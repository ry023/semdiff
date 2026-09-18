package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
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
	translated := []string{}
	if len(positional) == 1 {
		translated = append(translated, "--groups-file", positional[0])
	}
	if *draftPath != defaultGroupingDraftPath {
		translated = append(translated, "--draft", *draftPath)
	}
	if *remote != "" {
		translated = append(translated, "--remote", *remote)
	}
	if *repository != "" {
		translated = append(translated, "--repository", *repository)
	}
	if *branch != "" {
		translated = append(translated, "--branch", *branch)
	}
	return runRemotePush(ctx, runner, translated)
}

func runReviews(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("reviews requires the subcommand resolve or view")
	}
	if args[0] == "resolve" {
		fmt.Fprintln(os.Stderr, "warning: semdiff reviews resolve is deprecated; use semdiff resolve")
		return runReviewsResolve(ctx, gitdiff.Runner{Dir: "."}, args[1:])
	}
	if args[0] != "view" {
		return fmt.Errorf("unknown reviews subcommand %q", args[0])
	}
	fmt.Fprintln(os.Stderr, "warning: semdiff reviews view is deprecated; use semdiff remote view-index")
	return runRemoteViewIndex(ctx, gitdiff.Runner{Dir: "."}, args[1:])
}

func runRemote(ctx context.Context, runner gitdiff.Runner, args []string) error {
	if len(args) == 0 {
		return errors.New("remote requires view-index, view, pull, or push")
	}
	switch args[0] {
	case "view-index":
		return runRemoteViewIndex(ctx, runner, args[1:])
	case "view":
		return runRemoteView(ctx, runner, args[1:])
	case "pull":
		return runRemotePull(ctx, runner, args[1:])
	case "push":
		return runRemotePush(ctx, runner, args[1:])
	default:
		return fmt.Errorf("unknown remote subcommand %q", args[0])
	}
}

func remoteStore(remote, repository, branch string) (reviews.Store, error) {
	storeConfig, err := config.Load(".")
	if err != nil {
		return reviews.Store{}, err
	}
	storeConfig, err = config.Override(storeConfig, remote, repository, branch)
	if err != nil {
		return reviews.Store{}, err
	}
	return reviews.Store{Dir: ".", Config: storeConfig}, nil
}

func runRemoteViewIndex(ctx context.Context, runner gitdiff.Runner, args []string) error {
	fs := flag.NewFlagSet("remote view-index", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:7363", "listen address")
	remote := fs.String("remote", "", "Git remote name")
	repository := fs.String("repository", "", "artifact repository URL or path")
	branch := fs.String("branch", "", "artifact branch")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(positional) != 0 {
		return errors.New("remote view-index does not accept positional arguments")
	}
	store, err := remoteStore(*remote, *repository, *branch)
	if err != nil {
		return err
	}
	if err := store.Fetch(ctx); err != nil {
		return err
	}
	h := reviewIndexHandler(ctx, runner, store)
	srv := &http.Server{Addr: *addr, Handler: h, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("Semantic Diff Reviews: http://%s", *addr)
	return srv.ListenAndServe()
}

func remoteRange(ctx context.Context, runner gitdiff.Runner, positional []string) (string, string, error) {
	if len(positional) > 1 {
		return "", "", errors.New("accepts at most one <base>..<head>")
	}
	rangeSpec := ""
	if len(positional) == 1 {
		rangeSpec = positional[0]
	} else {
		var err error
		rangeSpec, err = runner.DefaultRange(ctx)
		if err != nil {
			return "", "", fmt.Errorf("infer current review range: %w", err)
		}
	}
	base, head, err := gitdiff.ParseRange(rangeSpec)
	if err != nil {
		return "", "", err
	}
	baseSHA, err := runner.Resolve(ctx, base)
	if err != nil {
		return "", "", err
	}
	headSHA, err := runner.Resolve(ctx, head)
	if err != nil {
		return "", "", err
	}
	return baseSHA, headSHA, nil
}

func remoteArtifact(ctx context.Context, runner gitdiff.Runner, store reviews.Store, baseSHA, headSHA string) ([]byte, error) {
	if err := store.Fetch(ctx); err != nil {
		return nil, err
	}
	data, err := store.Read(ctx, reviews.Path(baseSHA, headSHA))
	if err != nil {
		return nil, err
	}
	g, err := groups.Parse(data)
	if err != nil {
		return nil, err
	}
	if g.BaseSHA != baseSHA || g.HeadSHA != headSHA {
		return nil, fmt.Errorf("remote artifact range does not match requested range %s..%s", baseSHA, headSHA)
	}
	changes, err := runner.Changes(ctx, baseSHA+".."+headSHA)
	if err != nil {
		return nil, err
	}
	report := groups.ValidateReport(g, changes)
	if len(report.Errors) > 0 {
		return nil, fmt.Errorf("remote groups file is invalid: %s", strings.Join(report.Errors, "; "))
	}
	return data, nil
}

func runRemoteView(ctx context.Context, runner gitdiff.Runner, args []string) error {
	fs := flag.NewFlagSet("remote view", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:7363", "listen address")
	remote := fs.String("remote", "", "Git remote name")
	repository := fs.String("repository", "", "artifact repository URL or path")
	branch := fs.String("branch", "", "artifact branch")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	baseSHA, headSHA, err := remoteRange(ctx, runner, positional)
	if err != nil {
		return err
	}
	store, err := remoteStore(*remote, *repository, *branch)
	if err != nil {
		return err
	}
	if _, err := remoteArtifact(ctx, runner, store, baseSHA, headSHA); err != nil {
		return err
	}
	key := strings.TrimSuffix(reviews.Path(baseSHA, headSHA), "/groups.json")
	path := "/review/" + key + "/"
	index := reviewIndexHandler(ctx, runner, store)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, path, http.StatusSeeOther)
			return
		}
		index.ServeHTTP(w, r)
	})
	srv := &http.Server{Addr: *addr, Handler: h, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("Semantic Diff Review: http://%s%s", *addr, path)
	return srv.ListenAndServe()
}

func confirmOverwrite(path string, input io.Reader, output io.Writer, terminal bool) error {
	if !terminal {
		return fmt.Errorf("%s already exists; use --force or --no-clobber in non-interactive mode", path)
	}
	fmt.Fprintf(output, "overwrite %s? [y/N] ", path)
	var answer string
	if _, err := fmt.Fscanln(input, &answer); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if answer != "y" && answer != "Y" && !strings.EqualFold(answer, "yes") {
		return fmt.Errorf("pull cancelled: %s was not overwritten", path)
	}
	return nil
}

func saveRemoteArtifact(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".groups-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0644); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func runRemotePull(ctx context.Context, runner gitdiff.Runner, args []string) error {
	fs := flag.NewFlagSet("remote pull", flag.ContinueOnError)
	remote := fs.String("remote", "", "Git remote name")
	repository := fs.String("repository", "", "artifact repository URL or path")
	branch := fs.String("branch", "", "artifact branch")
	force := fs.Bool("force", false, "overwrite an existing groups file without asking")
	noClobber := fs.Bool("no-clobber", false, "fail if the groups file already exists")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if *force && *noClobber {
		return errors.New("remote pull --force and --no-clobber cannot be used together")
	}
	baseSHA, headSHA, err := remoteRange(ctx, runner, positional)
	if err != nil {
		return err
	}
	path := reviews.LocalPath(baseSHA, headSHA)
	exists, err := reviewFileExists(path)
	if err != nil {
		return err
	}
	if exists && *noClobber {
		return fmt.Errorf("%s already exists", path)
	}
	if exists && !*force {
		stat, err := os.Stdin.Stat()
		if err != nil {
			return err
		}
		if err := confirmOverwrite(path, os.Stdin, os.Stderr, stat.Mode()&os.ModeCharDevice != 0); err != nil {
			return err
		}
	}
	store, err := remoteStore(*remote, *repository, *branch)
	if err != nil {
		return err
	}
	data, err := remoteArtifact(ctx, runner, store, baseSHA, headSHA)
	if err != nil {
		return err
	}
	if err := saveRemoteArtifact(path, data); err != nil {
		return err
	}
	fmt.Printf("pulled %s\n", path)
	return nil
}

func runRemotePush(ctx context.Context, runner gitdiff.Runner, args []string) error {
	fs := flag.NewFlagSet("remote push", flag.ContinueOnError)
	remote := fs.String("remote", "", "Git remote name")
	repository := fs.String("repository", "", "artifact repository URL or path")
	branch := fs.String("branch", "", "artifact branch")
	draftPath := fs.String("draft", defaultGroupingDraftPath, "draft path used to locate the default groups file")
	groupsFile := fs.String("groups-file", "", "groups file to publish")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(positional) > 1 || len(positional) == 1 && *groupsFile != "" {
		return errors.New("remote push accepts either <base>..<head> or --groups-file <path>")
	}
	groupsPath := *groupsFile
	requestedBase, requestedHead := "", ""
	if len(positional) == 1 {
		baseSHA, headSHA, err := remoteRange(ctx, runner, positional)
		if err != nil {
			return err
		}
		groupsPath = reviews.LocalPath(baseSHA, headSHA)
		requestedBase, requestedHead = baseSHA, headSHA
	} else if groupsPath == "" {
		groupsPath, err = defaultGroupsPath(*draftPath)
		if err != nil {
			return fmt.Errorf("locate default groups file from draft: %w", err)
		}
	}
	store, err := remoteStore(*remote, *repository, *branch)
	if err != nil {
		return err
	}
	g, _, report, err := loadAndValidate(ctx, runner, groupsPath)
	if err != nil {
		return err
	}
	if len(report.Errors) > 0 {
		return fmt.Errorf("groups file is invalid: %s", strings.Join(report.Errors, "; "))
	}
	if requestedBase != "" && (g.BaseSHA != requestedBase || g.HeadSHA != requestedHead) {
		return fmt.Errorf("groups file range does not match requested range %s..%s", requestedBase, requestedHead)
	}
	path, err := store.Publish(ctx, groupsPath, g)
	if err != nil {
		return err
	}
	fmt.Printf("published %s to %s:%s\n", path, store.Config.Endpoint(), store.Config.Branch)
	return nil
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
	fs := flag.NewFlagSet("resolve", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "JSON output")
	exactOnly := fs.Bool("exact", false, "only resolve a review for the exact current range")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(positional) > 1 {
		return errors.New("resolve accepts at most one <base>..<head>")
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
			status := http.StatusInternalServerError
			var compatibilityError *groups.CompatibilityError
			if errors.As(err, &compatibilityError) {
				status = http.StatusConflict
			}
			http.Error(w, err.Error(), status)
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
