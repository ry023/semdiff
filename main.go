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
	"strings"
	"syscall"
	"time"

	"github.com/ry023/semdiff/internal/categories"
	"github.com/ry023/semdiff/internal/gitdiff"
	"github.com/ry023/semdiff/internal/groupingdraft"
	"github.com/ry023/semdiff/internal/groups"
	"github.com/ry023/semdiff/internal/model"
	"github.com/ry023/semdiff/internal/questions"
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
	semdiff show <groups-file> <fragment-id> [--json]
	semdiff show --draft <path> <fragment-id> [--json]
	semdiff validate <groups-file> [--json]
	semdiff grouping init <base>..<head> [--draft <path>] [--json]
	semdiff grouping apply <operations-file|-> [--draft <path>] [--json]
	semdiff grouping status [--draft <path>] [--json]
	semdiff grouping inspect (--suggestions|--unassigned|--group <id>|--fragment <id>) [--draft <path>] [--json]
	semdiff grouping finalize <groups-file> [--draft <path>] [--json]
	semdiff questions wait <groups-file> [--json]
	semdiff questions answer <groups-file> <question-id> --stdin [--json]
	semdiff view <groups-file> [--addr 127.0.0.1:8080]
	semdiff publish <groups-file> [--remote origin|--repository <url>] [--branch semdiff/reviews]
	semdiff reviews view [--addr 127.0.0.1:8080] [--remote origin|--repository <url>] [--branch semdiff/reviews]`)
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
		if len(positional) != 2 {
			return errors.New("show requires <groups-file> <fragment-id>")
		}
		g, inv, report, err := loadAndValidate(ctx, r, positional[0])
		if err != nil {
			return err
		}
		if len(report.Errors) > 0 {
			return fmt.Errorf("groups file is invalid: %s", strings.Join(report.Errors, "; "))
		}
		return printMaterializedFragment(inv, groups.Fragments(g), positional[1], *jsonOut)
	case "validate":
		fs := flag.NewFlagSet("validate", flag.ContinueOnError)
		jsonOut := fs.Bool("json", false, "JSON output")
		positional, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
		}
		if len(positional) != 1 {
			return errors.New("validate requires <groups-file>")
		}
		g, inv, report, err := loadAndValidate(ctx, r, positional[0])
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
		addr := fs.String("addr", "127.0.0.1:8080", "listen address")
		positional, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
		}
		if len(positional) != 1 {
			return errors.New("view requires <groups-file>")
		}
		g, inv, report, err := loadAndValidate(ctx, r, positional[0])
		if err != nil {
			return err
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
		questionStore := questions.Store{Path: questions.DefaultPath(positional[0], g.BaseSHA, g.HeadSHA), BaseSHA: g.BaseSHA, HeadSHA: g.HeadSHA}
		h, err := viewer.HandlerWithQuestions(viewer.Build(g, inv, fileContents), questionStore)
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
