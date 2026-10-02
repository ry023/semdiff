package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ry023/semdiff/internal/config"
	"github.com/ry023/semdiff/internal/gitdiff"
	"github.com/ry023/semdiff/internal/groups"
	"github.com/ry023/semdiff/internal/reviews"
)

func runPublish(ctx context.Context, runner gitdiff.Runner, args []string) error {
	options, err := parseCommand[publishArgs]("publish", args)
	if err != nil {
		return err
	}
	remote, repository, branch, draftPath := &options.Remote, &options.Repository, &options.Branch, &options.Draft
	positional := []string{}
	if options.GroupsFile != "" {
		positional = append(positional, options.GroupsFile)
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
	if options.Config != "" {
		translated = append(translated, "--config", options.Config)
	}
	return runRemotePush(ctx, runner, translated)
}

func runReviews(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("reviews requires the subcommand resolve")
	}
	if args[0] == "resolve" {
		fmt.Fprintln(os.Stderr, localized("warning: semdiff reviews resolve is deprecated; use semdiff resolve", "警告: semdiff reviews resolve は非推奨です。semdiff resolve を使ってください"))
		return runReviewsResolve(ctx, gitdiff.Runner{Dir: "."}, args[1:])
	}
	return fmt.Errorf("unknown reviews subcommand %q", args[0])
}

func runRemote(ctx context.Context, runner gitdiff.Runner, args []string) error {
	if len(args) == 0 {
		return errors.New("remote requires resolve, pull, or push")
	}
	switch args[0] {
	case "resolve":
		return runRemoteResolve(ctx, runner, args[1:])
	case "pull":
		return runRemotePull(ctx, runner, args[1:])
	case "push":
		return runRemotePush(ctx, runner, args[1:])
	default:
		return fmt.Errorf("unknown remote subcommand %q", args[0])
	}
}

func runRemoteResolve(ctx context.Context, runner gitdiff.Runner, args []string) error {
	options, err := parseCommand[remoteResolveArgs]("remote resolve", args)
	if err != nil {
		return err
	}
	if options.Force && options.NoClobber {
		return errors.New("remote resolve --force and --no-clobber cannot be used together")
	}
	if !options.Pull && (options.Force || options.NoClobber) {
		return errors.New("remote resolve --force and --no-clobber require --pull")
	}
	var positional []string
	if options.Range != "" {
		positional = append(positional, options.Range)
	}
	base, head, err := remoteRange(ctx, runner, positional)
	if err != nil {
		return err
	}
	cfg, err := loadRemoteConfig(ctx, runner, options.Config)
	if err != nil {
		return err
	}
	cfg, err = config.Override(cfg, options.Remote, options.Repository, options.Branch)
	if err != nil {
		return err
	}
	store := reviews.Store{Dir: runner.Dir, Config: cfg}
	match, err := store.Resolve(ctx, runner, base, head, options.Exact)
	if err != nil {
		return err
	}
	result := reviewResolveOutput{CurrentBaseSHA: base, CurrentHeadSHA: head}
	if match != nil {
		result.RemotePath = match.Path
		if options.Pull {
			path := localReviewPath(runner.Dir, base, match.HeadSHA)
			if err := checkRemoteOverwrite(path, options.Force, options.NoClobber); err != nil {
				return err
			}
			if err := saveRemoteArtifact(path, match.Data); err != nil {
				return err
			}
			result.GroupsPath = path
		}
		result.Found = true
		result.ReviewBaseSHA, result.ReviewHeadSHA = base, match.HeadSHA
		result.Exact, result.CommitsBehind = match.HeadSHA == head, match.CommitsBehind
	}
	return printReviewResolution(options.JSON, result)
}

func loadRemoteConfig(ctx context.Context, runner gitdiff.Runner, path string) (config.ReviewStore, error) {
	root, err := runner.Root(ctx)
	if err != nil {
		return config.ReviewStore{}, err
	}
	return config.Load(root, path)
}

func remoteStore(ctx context.Context, runner gitdiff.Runner, configPath, remote, repository, branch string) (reviews.Store, error) {
	storeConfig, err := loadRemoteConfig(ctx, runner, configPath)
	if err != nil {
		return reviews.Store{}, err
	}
	storeConfig, err = config.Override(storeConfig, remote, repository, branch)
	if err != nil {
		return reviews.Store{}, err
	}
	return reviews.Store{Dir: runner.Dir, Config: storeConfig}, nil
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

func confirmOverwrite(path string, input io.Reader, output io.Writer, terminal bool) error {
	if !terminal {
		return fmt.Errorf(localized("%s already exists; use --force or --no-clobber in non-interactive mode", "%s は既に存在します。非対話環境では --force または --no-clobber を指定してください"), path)
	}
	fmt.Fprintf(output, localized("overwrite %s? [y/N] ", "%s を上書きしますか？ [y/N] "), path)
	var answer string
	if _, err := fmt.Fscanln(input, &answer); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if answer != "y" && answer != "Y" && !strings.EqualFold(answer, "yes") {
		return fmt.Errorf(localized("pull cancelled: %s was not overwritten", "pull を中止しました: %s は上書きされていません"), path)
	}
	return nil
}

func checkRemoteOverwrite(path string, force, noClobber bool) error {
	exists, err := reviewFileExists(path)
	if err != nil {
		return err
	}
	if exists && noClobber {
		return fmt.Errorf("%s already exists", path)
	}
	if exists && !force {
		stat, err := os.Stdin.Stat()
		if err != nil {
			return err
		}
		return confirmOverwrite(path, os.Stdin, os.Stderr, stat.Mode()&os.ModeCharDevice != 0)
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
	options, err := parseCommand[remotePullArgs]("remote pull", args)
	if err != nil {
		return err
	}
	remote, repository, branch := &options.Remote, &options.Repository, &options.Branch
	force, noClobber := &options.Force, &options.NoClobber
	positional := []string{}
	if options.Range != "" {
		positional = append(positional, options.Range)
	}
	if *force && *noClobber {
		return errors.New("remote pull --force and --no-clobber cannot be used together")
	}
	baseSHA, headSHA, err := remoteRange(ctx, runner, positional)
	if err != nil {
		return err
	}
	path := reviews.LocalPath(baseSHA, headSHA)
	if err := checkRemoteOverwrite(path, *force, *noClobber); err != nil {
		return err
	}
	store, err := remoteStore(ctx, runner, options.Config, *remote, *repository, *branch)
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
	fmt.Printf(localized("pulled %s\n", "取得しました: %s\n"), path)
	return nil
}

func runRemotePush(ctx context.Context, runner gitdiff.Runner, args []string) error {
	options, err := parseCommand[remotePushArgs]("remote push", args)
	if err != nil {
		return err
	}
	remote, repository, branch, draftPath, groupsFile := &options.Remote, &options.Repository, &options.Branch, &options.Draft, &options.GroupsFile
	positional := []string{}
	if options.Range != "" {
		positional = append(positional, options.Range)
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
	store, err := remoteStore(ctx, runner, options.Config, *remote, *repository, *branch)
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
	fmt.Printf(localized("published %s to %s:%s\n", "%s を %s:%s に共有しました\n"), path, store.Config.Endpoint(), store.Config.Branch)
	return nil
}

type reviewResolveOutput struct {
	Found          bool   `json:"found"`
	GroupsPath     string `json:"groups_path,omitempty"`
	RemotePath     string `json:"remote_path,omitempty"`
	CurrentBaseSHA string `json:"current_base_sha"`
	CurrentHeadSHA string `json:"current_head_sha"`
	ReviewBaseSHA  string `json:"review_base_sha,omitempty"`
	ReviewHeadSHA  string `json:"review_head_sha,omitempty"`
	Exact          bool   `json:"exact"`
	CommitsBehind  int    `json:"commits_behind"`
}

func runReviewsResolve(ctx context.Context, runner gitdiff.Runner, args []string) error {
	options, err := parseCommand[resolveArgs]("resolve", args)
	if err != nil {
		return err
	}
	jsonOut, exactOnly := &options.JSON, &options.Exact
	positional := []string{}
	if options.Range != "" {
		positional = append(positional, options.Range)
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
		fmt.Printf(localized("no compatible finalized review for %s..%s\n", "%s..%s に対応する確定済みレビューはありません\n"), result.CurrentBaseSHA, result.CurrentHeadSHA)
		return nil
	}
	state := localized("ancestor", "祖先")
	if result.Exact {
		state = localized("exact", "完全一致")
	}
	path := result.GroupsPath
	if path == "" {
		path = result.RemotePath
	}
	fmt.Printf(localized("%s review: %s (%s..%s; %d first-parent commits behind)\n", "%sのレビュー: %s (%s..%s; first-parent 上で %d commit 前)\n"), state, path, result.ReviewBaseSHA, result.ReviewHeadSHA, result.CommitsBehind)
	return nil
}
