package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/ry023/semdiff/internal/categories"
	"github.com/ry023/semdiff/internal/gitdiff"
	"github.com/ry023/semdiff/internal/groupingdraft"
	"github.com/ry023/semdiff/internal/groups"
	"github.com/ry023/semdiff/internal/model"
	"github.com/ry023/semdiff/internal/questions"
	"github.com/ry023/semdiff/internal/reviews"
	"github.com/ry023/semdiff/internal/viewer"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, localized("error:", "エラー:"), err)
		var compatibilityError *groups.CompatibilityError
		if errors.As(err, &compatibilityError) {
			fmt.Fprintln(os.Stderr, localized("semdiff version:", "semdiff バージョン:"), productVersion())
		}
		os.Exit(1)
	}
}

func usage() {
	writeUsage(os.Stderr)
}

func writeUsage(w io.Writer) {
	if japaneseLocale() {
		fmt.Fprintln(w, usageJA)
		return
	}
	fmt.Fprintln(w, `semdiff organizes a fixed Git range into semantic review groups.

Usage:
  semdiff <command> [arguments] [flags]
  semdiff --help

Create and read a review:
  grouping init [<base>..<head>]       Start a draft from Git changes.
  grouping inspect --suggestions      Inspect draft candidates; also accepts
                                      --unassigned, --group, or --fragment.
  grouping apply <operations-file|->  Apply draft operations; - reads stdin.
  grouping status                     Check draft progress and coverage.
  grouping finalize [<groups-file>]   Validate and save groups.json locally.
  view [<groups-file>]                 Serve the local review for a browser.
  view --html <path>                   Export a self-contained HTML review.
  resolve [<base>..<head>]            Find the exact or nearest compatible
                                      local review; --exact disables fallback.

Inspect Git changes and review data:
  commits <base>..<head>              List commits in the range.
  fragments <base>..<head>            List Git-derived fragment candidates.
  classify <base>..<head>             Suggest file categories by path.
  show [<groups-file>] <fragment-id>  Show a finalized fragment and its patch;
                                      --draft <path> reads a draft instead.
  validate [<groups-file>]           Check a finalized review against Git.

Answer viewer questions:
  questions session start            Start an answer session.
  questions wait                     Wait for a pending question.
  questions answer <id> --stdin       Attach an answer from stdin.

Share reviews through a Git artifact branch:
  remote view-index                   Serve the remote review index as HTML.
  remote view [<base>..<head>]        Serve one remote review without saving it.
  remote pull [<base>..<head>]        Save one remote review locally; asks
                                      before overwrite (--force/--no-clobber).
  remote push [<base>..<head>]        Publish a local review; defaults to the
                                      current draft or use --groups-file.

Other:
  --version                           Show the CLI version on one line.
  version [--json]                    Show CLI and groups schema versions.

For range-based commands, omission selects the current pull request or
default branch. remote push instead uses the current draft by default.
Drafts live under .semdiff/; finalized reviews under .semdiff/reviews/.
Use --json where available, --draft <path> to select a draft, and
--remote/--repository/--branch to override the remote review store.`)
}

func run(ctx context.Context, args []string) (err error) {
	defer func() {
		if errors.Is(err, errCommandHelp) {
			err = nil
		}
	}()
	if len(args) == 0 {
		usage()
		return errors.New("command is required")
	}
	if err := validateProductVersion(); err != nil {
		return err
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		if len(args) != 1 {
			return errors.New("help does not accept arguments")
		}
		writeUsage(os.Stdout)
		return nil
	}
	if args[0] == "--version" {
		if len(args) != 1 {
			return errors.New("--version does not accept arguments")
		}
		fmt.Printf("semdiff %s\n", productVersion())
		return nil
	}
	r := gitdiff.Runner{Dir: "."}
	switch args[0] {
	case "version":
		options, err := parseCommand[versionArgs]("version", args[1:])
		if err != nil {
			return err
		}
		output := currentVersionOutput()
		if options.JSON {
			return printJSON(output)
		}
		fmt.Printf("semdiff %s\n", output.Version)
		for _, supported := range output.GroupsSchema.ReadRanges {
			fmt.Printf(localized("groups schema read: >=%s, <%s\n", "読み込み可能な groups schema: >=%s, <%s\n"), supported.Min, supported.MaxExclusive)
		}
		fmt.Printf(localized("groups schema write: %s %s\n", "書き込む groups schema: %s %s\n"), output.GroupsSchema.Format, output.GroupsSchema.Write)
		return nil
	case "grouping":
		return runGrouping(ctx, r, args[1:])
	case "questions":
		return runQuestions(ctx, args[1:])
	case "publish":
		fmt.Fprintln(os.Stderr, localized("warning: semdiff publish is deprecated; use semdiff remote push", "警告: semdiff publish は非推奨です。semdiff remote push を使ってください"))
		return runPublish(ctx, r, args[1:])
	case "reviews":
		return runReviews(ctx, args[1:])
	case "resolve":
		return runReviewsResolve(ctx, r, args[1:])
	case "remote":
		return runRemote(ctx, r, args[1:])
	case "commits":
		options, err := parseCommand[rangeJSONArgs]("commits", args[1:])
		if err != nil {
			return err
		}
		if options.Range == "" {
			return errors.New("commits requires <base>..<head>")
		}
		cs, err := r.Commits(ctx, options.Range)
		if err != nil {
			return err
		}
		if options.JSON {
			return printJSON(cs)
		}
		for _, c := range cs {
			fmt.Printf(localized("%.12s  %s  %s  (%d files)\n", "%.12s  %s  %s  (%d ファイル)\n"), c.SHA, c.Subject, c.Author, c.FilesChanged)
		}
		return nil
	case "fragments":
		options, err := parseCommand[rangeJSONArgs]("fragments", args[1:])
		if err != nil {
			return err
		}
		if options.Range == "" {
			return errors.New("fragments requires <base>..<head>")
		}
		inv, err := r.Changes(ctx, options.Range)
		if err != nil {
			return err
		}
		if options.JSON {
			return printJSON(gitdiff.SuggestedFragments(inv))
		}
		for _, f := range gitdiff.SuggestedFragments(inv) {
			fmt.Printf("%s  %s  %s\n", f.ID, f.Path, formatFragmentRanges(f))
		}
		return nil
	case "classify":
		options, err := parseCommand[rangeJSONArgs]("classify", args[1:])
		if err != nil {
			return err
		}
		if options.Range == "" {
			return errors.New("classify requires <base>..<head>")
		}
		inv, err := r.Changes(ctx, options.Range)
		if err != nil {
			return err
		}
		paths := make([]string, 0, len(inv.Changes))
		for _, fragment := range inv.Changes {
			paths = append(paths, fragment.Path)
		}
		suggestions := categories.ClassifyPaths(paths)
		if options.JSON {
			return printJSON(suggestions)
		}
		for _, suggestion := range suggestions {
			fmt.Printf("%s  %s\n", suggestion.Path, suggestion.Category)
		}
		return nil
	case "show":
		options, err := parseCommand[showArgs]("show", args[1:])
		if err != nil {
			return err
		}
		jsonOut, draftPath, positional := &options.JSON, &options.Draft, options.Args
		if *draftPath != "" {
			if len(positional) != 1 {
				return errors.New("show --draft requires <fragment-id>")
			}
			draft, err := groupingdraft.Load(*draftPath)
			if err != nil {
				return err
			}
			changes, err := r.Changes(ctx, draft.BaseSHA+".."+draft.HeadSHA)
			if err != nil {
				return err
			}
			inspectable := draft.InspectableFragments()
			return printMaterializedFragment(gitdiff.Materialize(changes, inspectable), inspectable, positional[0], *jsonOut)
		}
		if len(positional) < 1 || len(positional) > 2 {
			return errors.New("show requires [<groups-file>] <fragment-id>")
		}
		groupsPath, fragmentID := "", ""
		if len(positional) == 2 {
			groupsPath, fragmentID = positional[0], positional[1]
		} else {
			groupsPath, err = defaultGroupsPath(defaultGroupingDraftPath)
			if err != nil {
				return fmt.Errorf("locate default groups file from draft: %w", err)
			}
			fragmentID = positional[0]
		}
		g, inv, report, err := loadAndValidate(ctx, r, groupsPath)
		if err != nil {
			return err
		}
		if len(report.Errors) > 0 {
			return fmt.Errorf("groups file is invalid: %s", strings.Join(report.Errors, "; "))
		}
		return printMaterializedFragment(inv, groups.Fragments(g), fragmentID, *jsonOut)
	case "validate":
		options, err := parseCommand[validateArgs]("validate", args[1:])
		if err != nil {
			return err
		}
		jsonOut, draftPath := &options.JSON, &options.Draft
		positional := []string{}
		if options.GroupsFile != "" {
			positional = append(positional, options.GroupsFile)
		}
		if len(positional) > 1 {
			return errors.New("validate accepts at most one <groups-file>")
		}
		groupsPath := ""
		if len(positional) == 1 {
			groupsPath = positional[0]
		} else {
			groupsPath, err = defaultGroupsPath(*draftPath)
			if err != nil {
				return fmt.Errorf("locate default groups file from draft: %w", err)
			}
		}
		g, inv, report, err := loadAndValidate(ctx, r, groupsPath)
		if err != nil {
			return err
		}
		if *jsonOut {
			result := struct {
				Valid         bool     `json:"valid"`
				FragmentCount int      `json:"fragment_count"`
				GroupCount    int      `json:"group_count"`
				Errors        []string `json:"errors"`
				Warnings      []string `json:"warnings"`
			}{len(report.Errors) == 0, len(inv.Fragments), len(g.Groups), report.Errors, report.Warnings}
			_ = printJSON(result)
		} else if len(report.Errors) == 0 {
			for _, warning := range report.Warnings {
				fmt.Fprintln(os.Stderr, localized("warning:", "警告:"), warning)
			}
			fmt.Printf(localized("valid: %d fragments assigned exactly once across %d groups\n", "検証成功: %d 個の Fragment が %d 個の Group に重複なく割り当てられています\n"), len(inv.Fragments), len(g.Groups))
		} else {
			for _, warning := range report.Warnings {
				fmt.Fprintln(os.Stderr, localized("warning:", "警告:"), warning)
			}
			for _, p := range report.Errors {
				fmt.Fprintln(os.Stderr, "-", p)
			}
		}
		if len(report.Errors) > 0 {
			return fmt.Errorf("validation failed with %d error(s)", len(report.Errors))
		}
		return nil
	case "view":
		options, err := parseCommand[viewArgs]("view", args[1:])
		if err != nil {
			return err
		}
		addrValue := "127.0.0.1:7363"
		if options.Addr != nil {
			addrValue = *options.Addr
		}
		addr := &addrValue
		htmlPath, includeAnswers, exact := &options.HTML, &options.IncludeAnswers, &options.Exact
		draftValue := ""
		if options.Draft != nil {
			draftValue = *options.Draft
		}
		draftPath := &draftValue
		positional := []string{}
		if options.GroupsFile != "" {
			positional = append(positional, options.GroupsFile)
		}
		if len(positional) > 1 {
			return errors.New("view accepts at most one <groups-file>")
		}
		if *includeAnswers && *htmlPath == "" {
			return errors.New("view --include-answers requires --html")
		}
		addrSet, draftSet := options.Addr != nil, options.Draft != nil
		if *htmlPath != "" && addrSet {
			return errors.New("view --html and --addr cannot be used together")
		}
		if *exact && (len(positional) == 1 || draftSet) {
			return errors.New("view --exact cannot be used with an explicit groups file or --draft")
		}
		groupsPath := ""
		var selection *viewSelection
		if len(positional) == 1 {
			groupsPath = positional[0]
		} else if draftSet {
			groupsPath, err = defaultGroupsPath(*draftPath)
			if err != nil {
				return fmt.Errorf("locate groups file from draft: %w", err)
			}
		} else {
			resolved, resolveErr := resolveCurrentView(ctx, r, *exact)
			err = resolveErr
			if err != nil {
				return err
			}
			selection = &resolved
			groupsPath = resolved.GroupsPath
		}
		g, inv, report, err := loadAndValidate(ctx, r, groupsPath)
		if err != nil {
			return err
		}
		if selection != nil && (g.BaseSHA != selection.CurrentBaseSHA || g.HeadSHA != selection.ReviewHeadSHA) {
			return fmt.Errorf("review artifact path does not match its range: expected %s..%s, got %s..%s", selection.CurrentBaseSHA, selection.ReviewHeadSHA, g.BaseSHA, g.HeadSHA)
		}
		if len(report.Errors) > 0 {
			return fmt.Errorf("groups file is invalid: %s", strings.Join(report.Errors, "; "))
		}
		for _, warning := range report.Warnings {
			log.Printf(localized("warning: %s", "警告: %s"), warning)
		}
		paths := make([]string, 0, len(inv.Fragments))
		for _, fragment := range inv.Fragments {
			paths = append(paths, fragment.Path)
		}
		fileContents := r.FileContents(ctx, inv.BaseSHA, inv.HeadSHA, paths)
		questionStore := questions.Store{Path: questions.DefaultPath(groupsPath, g.BaseSHA, g.HeadSHA), SessionPath: questions.DefaultSessionPath(groupsPath, g.BaseSHA, g.HeadSHA), BaseSHA: g.BaseSHA, HeadSHA: g.HeadSHA}
		page := viewer.Build(g, inv, fileContents)
		if selection != nil && !selection.Exact {
			drift, err := reviewDrift(ctx, r, selection.ReviewHeadSHA, selection.CurrentBaseSHA, selection.CurrentHeadSHA)
			if err != nil {
				return fmt.Errorf("inspect changes since review: %w", err)
			}
			page.Drift = &drift
		}
		if *htmlPath != "" {
			var threads []questions.Thread
			if *includeAnswers {
				threads, err = questionStore.List()
				if err != nil {
					return fmt.Errorf("load answers: %w", err)
				}
			}
			content, err := viewer.ExportHTML(page, threads)
			if err != nil {
				return fmt.Errorf("render HTML export: %w", err)
			}
			if err := os.WriteFile(*htmlPath, content, 0644); err != nil {
				return fmt.Errorf("write HTML export: %w", err)
			}
			fmt.Printf(localized("wrote %s\n", "%s を書き出しました\n"), *htmlPath)
			return nil
		}
		h, err := viewer.HandlerWithQuestions(page, questionStore)
		if err != nil {
			return err
		}
		srv := &http.Server{Addr: *addr, Handler: h, ReadHeaderTimeout: 5 * time.Second}
		stopCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stop()
		go func() {
			<-stopCtx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		}()
		log.Printf("Semantic Diff Viewer: http://%s", *addr)
		err = srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printJSON(v any) error {
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", "  ")
	return e.Encode(v)
}

func printMaterializedFragment(set model.FragmentSet, definitions []model.Fragment, id string, jsonOut bool) error {
	for _, rendered := range set.Fragments {
		if rendered.ID != id {
			continue
		}
		if !jsonOut {
			fmt.Print(rendered.Patch)
			return nil
		}
		for _, definition := range definitions {
			if definition.ID == id {
				return printJSON(struct {
					model.Fragment
					Patch string `json:"patch"`
				}{definition, rendered.Patch})
			}
		}
	}
	return fmt.Errorf("fragment %s not found", id)
}
func loadAndValidate(ctx context.Context, r gitdiff.Runner, path string) (model.GroupsFile, model.FragmentSet, groups.ValidationReport, error) {
	g, err := groups.Load(path)
	if err != nil {
		return g, model.FragmentSet{}, groups.ValidationReport{}, err
	}
	changes, err := r.Changes(ctx, g.BaseSHA+".."+g.HeadSHA)
	if err != nil {
		return g, model.FragmentSet{}, groups.ValidationReport{}, err
	}
	report := groups.ValidateReport(g, changes)
	return g, gitdiff.Materialize(changes, groups.Fragments(g)), report, nil
}

type viewSelection struct {
	GroupsPath     string
	CurrentBaseSHA string
	CurrentHeadSHA string
	ReviewHeadSHA  string
	Exact          bool
}

func resolveCurrentView(ctx context.Context, r gitdiff.Runner, exactOnly bool) (viewSelection, error) {
	rangeSpec, err := r.DefaultRange(ctx)
	if err != nil {
		return viewSelection{}, fmt.Errorf("infer current review range: %w", err)
	}
	return resolveViewForRange(ctx, r, rangeSpec, exactOnly)
}

func resolveViewForRange(ctx context.Context, r gitdiff.Runner, rangeSpec string, exactOnly bool) (viewSelection, error) {
	base, head, err := gitdiff.ParseRange(rangeSpec)
	if err != nil {
		return viewSelection{}, err
	}
	baseSHA, err := r.Resolve(ctx, base)
	if err != nil {
		return viewSelection{}, err
	}
	headSHA, err := r.Resolve(ctx, head)
	if err != nil {
		return viewSelection{}, err
	}
	rangeSpec = baseSHA + ".." + headSHA
	exactPath := localReviewPath(r.Dir, baseSHA, headSHA)
	if found, err := reviewFileExists(exactPath); err != nil {
		return viewSelection{}, err
	} else if found {
		if err := ensureReviewCompatible(exactPath); err != nil {
			return viewSelection{}, err
		}
		return viewSelection{GroupsPath: exactPath, CurrentBaseSHA: baseSHA, CurrentHeadSHA: headSHA, ReviewHeadSHA: headSHA, Exact: true}, nil
	}
	if exactOnly {
		return viewSelection{}, noReviewError{CurrentBaseSHA: baseSHA, CurrentHeadSHA: headSHA, ExactOnly: true, ExpectedPath: exactPath}
	}

	history, err := r.FirstParentCommits(ctx, rangeSpec)
	if err != nil {
		return viewSelection{}, fmt.Errorf("walk current branch history: %w", err)
	}
	for _, candidateHead := range history {
		if candidateHead == headSHA {
			continue
		}
		candidatePath := localReviewPath(r.Dir, baseSHA, candidateHead)
		found, statErr := reviewFileExists(candidatePath)
		if statErr != nil {
			return viewSelection{}, statErr
		}
		if found {
			if err := ensureReviewCompatible(candidatePath); err != nil {
				return viewSelection{}, err
			}
			return viewSelection{GroupsPath: candidatePath, CurrentBaseSHA: baseSHA, CurrentHeadSHA: headSHA, ReviewHeadSHA: candidateHead}, nil
		}
	}
	return viewSelection{}, noReviewError{CurrentBaseSHA: baseSHA, CurrentHeadSHA: headSHA}
}

func ensureReviewCompatible(path string) error {
	if _, err := groups.Load(path); err != nil {
		return fmt.Errorf("review artifact %s is not compatible: %w", path, err)
	}
	return nil
}

type noReviewError struct {
	CurrentBaseSHA string
	CurrentHeadSHA string
	ExpectedPath   string
	ExactOnly      bool
}

func (e noReviewError) Error() string {
	if e.ExactOnly {
		return fmt.Sprintf("no finalized review for current range %s..%s (expected %s)", e.CurrentBaseSHA, e.CurrentHeadSHA, e.ExpectedPath)
	}
	return fmt.Sprintf("no finalized review for current range %s..%s or an earlier first-parent state with the same base", e.CurrentBaseSHA, e.CurrentHeadSHA)
}

func localReviewPath(dir, baseSHA, headSHA string) string {
	path := reviews.LocalPath(baseSHA, headSHA)
	if dir == "" || dir == "." {
		return path
	}
	return filepath.Join(dir, path)
}

func reviewFileExists(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return true, nil
	} else if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else {
		return false, fmt.Errorf("check review artifact %s: %w", path, err)
	}
}

func reviewDrift(ctx context.Context, r gitdiff.Runner, reviewHead, currentBase, currentHead string) (viewer.ReviewDrift, error) {
	rangeSpec := reviewHead + ".." + currentHead
	commits, err := r.Commits(ctx, rangeSpec)
	if err != nil {
		return viewer.ReviewDrift{}, err
	}
	changes, err := r.Changes(ctx, rangeSpec)
	if err != nil {
		return viewer.ReviewDrift{}, err
	}
	pathSet := map[string]bool{}
	for _, change := range changes.Changes {
		pathSet[change.Path] = true
	}
	paths := make([]string, 0, len(pathSet))
	for path := range pathSet {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return viewer.ReviewDrift{CurrentBaseSHA: currentBase, CurrentHeadSHA: currentHead, Commits: commits, Paths: paths}, nil
}
