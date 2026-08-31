package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
	semdiff commits <base>..<head> [--json]
	semdiff fragments <base>..<head> [--json]
	semdiff classify <base>..<head> [--json]
	semdiff show [<groups-file>] <fragment-id> [--json]
	semdiff show --draft <path> <fragment-id> [--json]
	semdiff validate [<groups-file>] [--draft <path>] [--json]
	semdiff grouping init [<base>..<head>] [--from <groups-file>] [--draft <path>] [--json]
	semdiff grouping apply <operations-file|-> [--draft <path>] [--json]
	semdiff grouping status [--draft <path>] [--json]
	semdiff grouping inspect (--suggestions|--unassigned|--group <id>|--fragment <id>) [--draft <path>] [--json]
	semdiff grouping finalize [<groups-file>] [--draft <path>] [--json]
	semdiff questions wait [<groups-file>] [--session <session-id>] [--draft <path>] [--json]
	semdiff questions session start [<groups-file>] [--draft <path>] [--json]
	semdiff questions answer [<groups-file>] <question-id> --stdin [--draft <path>] [--json]
	semdiff view [<groups-file>] [--draft <path>] [--exact] [--addr 127.0.0.1:7363]
	semdiff view [<groups-file>] [--draft <path>] [--exact] --html <path> [--include-answers]
	semdiff publish [<groups-file>] [--draft <path>] [--remote origin|--repository <url>] [--branch semdiff/reviews]
	semdiff reviews resolve [<base>..<head>] [--exact] [--json]
	semdiff reviews view [--addr 127.0.0.1:7363] [--remote origin|--repository <url>] [--branch semdiff/reviews]`)
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("command is required")
	}
	r := gitdiff.Runner{Dir: "."}
	switch args[0] {
	case "grouping":
		return runGrouping(ctx, r, args[1:])
	case "questions":
		return runQuestions(ctx, args[1:])
	case "publish":
		return runPublish(ctx, r, args[1:])
	case "reviews":
		return runReviews(ctx, args[1:])
	case "commits":
		fs := flag.NewFlagSet("commits", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "JSON output")
		positional, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
		}
		if len(positional) != 1 {
			return errors.New("commits requires <base>..<head>")
		}
		cs, err := r.Commits(ctx, positional[0])
		if err != nil {
			return err
		}
		if *jsonOut {
			return printJSON(cs)
		}
		for _, c := range cs {
			fmt.Printf("%.12s  %s  %s  (%d files)\n", c.SHA, c.Subject, c.Author, c.FilesChanged)
		}
		return nil
	case "fragments":
		fs := flag.NewFlagSet("fragments", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "JSON output")
		positional, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
		}
		if len(positional) != 1 {
			return errors.New("fragments requires <base>..<head>")
		}
		inv, err := r.Changes(ctx, positional[0])
		if err != nil {
			return err
		}
		if *jsonOut {
			return printJSON(gitdiff.SuggestedFragments(inv))
		}
		for _, f := range gitdiff.SuggestedFragments(inv) {
			fmt.Printf("%s  %s  %s\n", f.ID, f.Path, formatFragmentRanges(f))
		}
		return nil
	case "classify":
		fs := flag.NewFlagSet("classify", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "JSON output")
		positional, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
		}
		if len(positional) != 1 {
			return errors.New("classify requires <base>..<head>")
		}
		inv, err := r.Changes(ctx, positional[0])
		if err != nil {
			return err
		}
		paths := make([]string, 0, len(inv.Changes))
		for _, fragment := range inv.Changes {
			paths = append(paths, fragment.Path)
		}
		suggestions := categories.ClassifyPaths(paths)
		if *jsonOut {
			return printJSON(suggestions)
		}
		for _, suggestion := range suggestions {
			fmt.Printf("%s  %s\n", suggestion.Path, suggestion.Category)
		}
		return nil
	case "show":
		fs := flag.NewFlagSet("show", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "JSON output")
		draftPath := fs.String("draft", "", "grouping draft path")
		positional, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
		}
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
		fs := flag.NewFlagSet("validate", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "JSON output")
		draftPath := fs.String("draft", defaultGroupingDraftPath, "draft path used to locate the default groups file")
		positional, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
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
				fmt.Fprintln(os.Stderr, "warning:", warning)
			}
			fmt.Printf("valid: %d fragments assigned exactly once across %d groups\n", len(inv.Fragments), len(g.Groups))
		} else {
			for _, warning := range report.Warnings {
				fmt.Fprintln(os.Stderr, "warning:", warning)
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
		fs := flag.NewFlagSet("view", flag.ContinueOnError)
		addr := fs.String("addr", "127.0.0.1:7363", "listen address")
		htmlPath := fs.String("html", "", "write a self-contained HTML file instead of serving the viewer")
		includeAnswers := fs.Bool("include-answers", false, "include answered question threads in an HTML export")
		draftPath := fs.String("draft", "", "use a grouping draft to locate the groups file instead of the current range")
		exact := fs.Bool("exact", false, "require a finalized review for the current range")
		positional, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
		}
		if len(positional) > 1 {
			return errors.New("view accepts at most one <groups-file>")
		}
		if *includeAnswers && *htmlPath == "" {
			return errors.New("view --include-answers requires --html")
		}
		addrSet := false
		draftSet := false
		fs.Visit(func(item *flag.Flag) {
			if item.Name == "addr" {
				addrSet = true
			}
			if item.Name == "draft" {
				draftSet = true
			}
		})
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
			log.Printf("warning: %s", warning)
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
			fmt.Printf("wrote %s\n", *htmlPath)
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

func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a != "-" && strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			name := strings.TrimLeft(strings.SplitN(a, "=", 2)[0], "-")
			f := fs.Lookup(name)
			if f != nil && !strings.Contains(a, "=") && i+1 < len(args) {
				boolFlag := false
				if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok {
					boolFlag = bf.IsBoolFlag()
				}
				if !boolFlag {
					i++
					flags = append(flags, args[i])
				}
			}
		} else {
			pos = append(pos, a)
		}
	}
	return pos, fs.Parse(flags)
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
			return viewSelection{GroupsPath: candidatePath, CurrentBaseSHA: baseSHA, CurrentHeadSHA: headSHA, ReviewHeadSHA: candidateHead}, nil
		}
	}
	return viewSelection{}, noReviewError{CurrentBaseSHA: baseSHA, CurrentHeadSHA: headSHA}
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
