package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"html"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/ry023/semdiff/internal/config"
	"github.com/ry023/semdiff/internal/gitdiff"
	"github.com/ry023/semdiff/internal/groups"
	"github.com/ry023/semdiff/internal/questions"
	"github.com/ry023/semdiff/internal/reviews"
	"github.com/ry023/semdiff/internal/viewer"
)

func runPublish(ctx context.Context, runner gitdiff.Runner, args []string) error {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	remote := fs.String("remote", "", "Git remote name")
	repository := fs.String("repository", "", "artifact repository URL or path")
	branch := fs.String("branch", "", "artifact branch")
	draftPath := fs.String("draft", defaultGroupingDraftPath, "draft path used to locate the default groups file")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(positional) > 1 {
		return errors.New("publish accepts at most one <groups-file>")
	}
	storeConfig, err := config.Load(".")
	if err != nil {
		return err
	}
	storeConfig, err = config.Override(storeConfig, *remote, *repository, *branch)
	if err != nil {
		return err
	}
	store := reviews.Store{Dir: ".", Config: storeConfig}
	groupsPath := ""
	if len(positional) == 1 {
		groupsPath = positional[0]
	} else {
		groupsPath, err = defaultGroupsPath(*draftPath)
		if err != nil {
			return fmt.Errorf("locate default groups file from draft: %w", err)
		}
	}
	g, _, report, err := loadAndValidate(ctx, runner, groupsPath)
	if err != nil {
		return err
	}
	if len(report.Errors) > 0 {
		return fmt.Errorf("groups file is invalid: %s", strings.Join(report.Errors, "; "))
	}
	path, err := store.Publish(ctx, groupsPath, g)
	if err != nil {
		return err
	}
	fmt.Printf("published %s to %s:%s\n", path, store.Config.Endpoint(), store.Config.Branch)
	return nil
}

func runReviews(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("reviews requires the subcommand resolve or view")
	}
	if args[0] == "resolve" {
		return runReviewsResolve(ctx, gitdiff.Runner{Dir: "."}, args[1:])
	}
	if args[0] != "view" {
		return fmt.Errorf("unknown reviews subcommand %q", args[0])
	}
	fs := flag.NewFlagSet("reviews view", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:7363", "listen address")
	remote := fs.String("remote", "", "Git remote name")
	repository := fs.String("repository", "", "artifact repository URL or path")
	branch := fs.String("branch", "", "artifact branch")
	positional, err := parseInterspersed(fs, args[1:])
	if err != nil {
		return err
	}
	if len(positional) != 0 {
		return errors.New("reviews view does not accept positional arguments")
	}
	storeConfig, err := config.Load(".")
	if err != nil {
		return err
	}
	storeConfig, err = config.Override(storeConfig, *remote, *repository, *branch)
	if err != nil {
		return err
	}
	store := reviews.Store{Dir: ".", Config: storeConfig}
	if err := store.Fetch(ctx); err != nil {
		return err
	}
	h := reviewIndexHandler(ctx, gitdiff.Runner{Dir: "."}, store)
	srv := &http.Server{Addr: *addr, Handler: h, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("Semantic Diff Reviews: http://%s", *addr)
	return srv.ListenAndServe()
}

type reviewResolveOutput struct {
	Found          bool   `json:"found"`
	GroupsPath     string `json:"groups_path,omitempty"`
	CurrentBaseSHA string `json:"current_base_sha"`
	CurrentHeadSHA string `json:"current_head_sha"`
	ReviewBaseSHA  string `json:"review_base_sha,omitempty"`
	ReviewHeadSHA  string `json:"review_head_sha,omitempty"`
	Exact          bool   `json:"exact"`
	CommitsBehind  int    `json:"commits_behind"`
}

func runReviewsResolve(ctx context.Context, runner gitdiff.Runner, args []string) error {
	fs := flag.NewFlagSet("reviews resolve", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "JSON output")
	exactOnly := fs.Bool("exact", false, "only resolve a review for the exact current range")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(positional) > 1 {
		return errors.New("reviews resolve accepts at most one <base>..<head>")
	}
	rangeSpec := ""
	if len(positional) == 1 {
		rangeSpec = positional[0]
	} else {
		rangeSpec, err = runner.DefaultRange(ctx)
		if err != nil {
			return fmt.Errorf("infer current review range: %w", err)
		}
	}
	result, err := resolveReview(ctx, runner, rangeSpec, *exactOnly)
	if err != nil {
		return err
	}
	return printReviewResolution(*jsonOut, result)
}

func resolveReview(ctx context.Context, runner gitdiff.Runner, rangeSpec string, exactOnly bool) (reviewResolveOutput, error) {
	selection, err := resolveViewForRange(ctx, runner, rangeSpec, exactOnly)
	if err != nil {
		var missing noReviewError
		if !errors.As(err, &missing) {
			return reviewResolveOutput{}, err
		}
		return reviewResolveOutput{CurrentBaseSHA: missing.CurrentBaseSHA, CurrentHeadSHA: missing.CurrentHeadSHA}, nil
	}
	firstParent, err := runner.FirstParentCommits(ctx, selection.ReviewHeadSHA+".."+selection.CurrentHeadSHA)
	if err != nil {
		return reviewResolveOutput{}, fmt.Errorf("count commits since review: %w", err)
	}
	return reviewResolveOutput{
		Found:          true,
		GroupsPath:     selection.GroupsPath,
		CurrentBaseSHA: selection.CurrentBaseSHA,
		CurrentHeadSHA: selection.CurrentHeadSHA,
		ReviewBaseSHA:  selection.CurrentBaseSHA,
		ReviewHeadSHA:  selection.ReviewHeadSHA,
		Exact:          selection.Exact,
		CommitsBehind:  len(firstParent),
	}, nil
}

func printReviewResolution(jsonOut bool, result reviewResolveOutput) error {
	if jsonOut {
		return printJSON(result)
	}
	if !result.Found {
		fmt.Printf("no compatible finalized review for %s..%s\n", result.CurrentBaseSHA, result.CurrentHeadSHA)
		return nil
	}
	state := "ancestor"
	if result.Exact {
		state = "exact"
	}
	fmt.Printf("%s review: %s (%s..%s; %d first-parent commits behind)\n", state, result.GroupsPath, result.ReviewBaseSHA, result.ReviewHeadSHA, result.CommitsBehind)
	return nil
}

func reviewIndexHandler(ctx context.Context, runner gitdiff.Runner, store reviews.Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		entries, err := store.List(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<!doctype html><title>Semantic Diff Reviews</title><style>body{font:16px system-ui;margin:3rem;max-width:55rem}code{font-family:ui-monospace}li{margin:.7rem 0}button{margin-left:1rem}</style><h1>Semantic Diff Reviews</h1><p>"+html.EscapeString(store.Config.Branch)+" <button onclick=\"location='/refresh'\">Refresh</button></p><ul>")
		for _, entry := range entries {
			label := entry.BaseSHA[:12] + "..." + entry.HeadSHA[:12]
			key := strings.TrimSuffix(entry.Path, "/groups.json")
			fmt.Fprintf(w, "<li><a href=\"/review/%s/\"><code>%s</code></a></li>", key, label)
		}
		fmt.Fprint(w, "</ul>")
	})
	mux.HandleFunc("/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := store.Fetch(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	mux.HandleFunc("/review/", func(w http.ResponseWriter, r *http.Request) {
		key := strings.Trim(strings.TrimPrefix(r.URL.Path, "/review/"), "/")
		if strings.Contains(key, "/") || key == "" {
			http.NotFound(w, r)
			return
		}
		path := key + "/groups.json"
		b, err := store.Read(r.Context(), path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		g, err := groups.Parse(b)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		changes, err := runner.Changes(r.Context(), g.BaseSHA+".."+g.HeadSHA)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		report := groups.ValidateReport(g, changes)
		if len(report.Errors) > 0 {
			http.Error(w, "groups file is invalid: "+strings.Join(report.Errors, "; "), http.StatusConflict)
			return
		}
		paths := make([]string, 0, len(changes.Changes))
		for _, fragment := range changes.Changes {
			paths = append(paths, fragment.Path)
		}
		questionPath := filepath.Join(".semdiff", "reviews", filepath.Dir(path), "groups.json")
		questionStore := questions.Store{Path: questions.DefaultPath(questionPath, g.BaseSHA, g.HeadSHA), SessionPath: questions.DefaultSessionPath(questionPath, g.BaseSHA, g.HeadSHA), BaseSHA: g.BaseSHA, HeadSHA: g.HeadSHA}
		basePath := "/review/" + key + "/"
		h, err := viewer.HandlerWithQuestionsAt(viewer.Build(g, gitdiff.Materialize(changes, groups.Fragments(g)), runner.FileContents(r.Context(), changes.BaseSHA, changes.HeadSHA, paths)), questionStore, basePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		copy := r.Clone(r.Context())
		copy.URL.Path = strings.TrimPrefix(r.URL.Path, basePath)
		if copy.URL.Path == "" {
			copy.URL.Path = "/"
		} else {
			copy.URL.Path = "/" + copy.URL.Path
		}
		h.ServeHTTP(w, copy)
	})
	return mux
}
