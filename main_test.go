package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ry023/semdiff/internal/gitdiff"
	"github.com/ry023/semdiff/internal/groupingdraft"
	"github.com/ry023/semdiff/internal/groups"
	"github.com/ry023/semdiff/internal/model"
	"github.com/ry023/semdiff/internal/reviews"
)

func TestParseInterspersedKeepsStdinMarker(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "JSON output")
	positional, err := parseInterspersed(fs, []string{"-", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if len(positional) != 1 || positional[0] != "-" || !*jsonOut {
		t.Fatalf("unexpected parsed arguments: positional=%v json=%v", positional, *jsonOut)
	}
}

func TestGroupingApplyAndFinalizeCommands(t *testing.T) {
	repo := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	runGit("init", "-q")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "src.ts"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")
	runGit("commit", "-qm", "base")
	baseBytes, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSpace(string(baseBytes))
	if err := os.WriteFile(filepath.Join(repo, "src.ts"), []byte("new\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")
	runGit("commit", "-qm", "head")
	draftRunner := gitdiff.Runner{Dir: repo}
	inv, err := draftRunner.Changes(context.Background(), base+"..HEAD")
	if err != nil {
		t.Fatal(err)
	}
	draftPath := filepath.Join(t.TempDir(), "draft.json")
	groupsPath := reviews.LocalPath(inv.BaseSHA, inv.HeadSHA)
	draft := groupingdraft.New(inv, nil)
	if err := groupingdraft.SaveAtomic(draftPath, draft); err != nil {
		t.Fatal(err)
	}
	resolvedGroupsPath, err := defaultGroupsPath(draftPath)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedGroupsPath != groupsPath {
		t.Fatalf("default groups path = %q, want %q", resolvedGroupsPath, groupsPath)
	}
	operationsPath := filepath.Join(t.TempDir(), "operations.json")
	core := model.ImportanceCore
	request := groupingdraft.ApplyRequest{Operations: []groupingdraft.Operation{
		{Op: "upsert_group", GroupID: "logic", Title: stringPtrForTest("Logic"), Summary: stringPtrForTest("A complete summary."), Importance: &core},
	}}
	var fragmentIDs []string
	fileCategories := map[string]string{}
	for _, fragment := range inv.Changes {
		fragmentIDs = append(fragmentIDs, fragment.ID)
		fileCategories[fragment.Path] = "logic"
		var definition model.Fragment
		for _, suggestion := range draft.Suggestions {
			if suggestion.ID == fragment.ID {
				definition = suggestion
				break
			}
		}
		definition.Description = "Adds the logic contract."
		definition.ReviewLevel = model.ReviewLevelCareful
		request.Operations = append(request.Operations, groupingdraft.Operation{Op: "add_fragment", Fragment: &definition})
	}
	request.Operations = append(request.Operations,
		groupingdraft.Operation{Op: "assign_fragments", GroupID: "logic", Members: fragmentIDs},
		groupingdraft.Operation{Op: "set_file_categories", GroupID: "logic", Categories: fileCategories},
	)
	b, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(operationsPath, b, 0644); err != nil {
		t.Fatal(err)
	}
	if err := runGroupingApply([]string{operationsPath, "--draft", draftPath}); err != nil {
		t.Fatal(err)
	}
	oldWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWorkingDirectory)
	currentDraft, err := groupingdraft.Load(draftPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := groupingdraft.SaveAtomic(defaultGroupingDraftPath, currentDraft); err != nil {
		t.Fatal(err)
	}
	if err := runGroupingFinalize(context.Background(), draftRunner, []string{"--draft", draftPath}); err != nil {
		t.Fatal(err)
	}
	result, err := groups.Load(filepath.Join(repo, groupsPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Groups) != 1 || len(result.Groups[0].Fragments) != len(inv.Changes) || result.Groups[0].Fragments[0].Description == "" {
		t.Fatalf("unexpected finalized groups: %+v", result)
	}
	if err := run(context.Background(), []string{"validate", "--json"}); err != nil {
		t.Fatalf("validate default groups file: %v", err)
	}
	if err := run(context.Background(), []string{"show", result.Groups[0].Fragments[0].ID, "--json"}); err != nil {
		t.Fatalf("show default groups file: %v", err)
	}
}

func TestResolveViewForRangePrefersExactThenNearestFirstParentReview(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", repo}, args...)...)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	commit := func(name string) string {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte(name+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		git("add", ".")
		git("commit", "-qm", name)
		return git("rev-parse", "HEAD")
	}
	writeReview := func(base, head string) string {
		t.Helper()
		path := filepath.Join(repo, reviews.LocalPath(base, head))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(`{"version":2,"base_sha":"`+base+`","head_sha":"`+head+`","groups":[]}`), 0644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	git("init", "-q")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test")
	base := commit("base")
	olderHead := commit("older")
	reviewedHead := commit("reviewed")
	currentHead := commit("current")
	writeReview(base, olderHead)
	reviewedPath := writeReview(base, reviewedHead)
	runner := gitdiff.Runner{Dir: repo}

	selection, err := resolveViewForRange(context.Background(), runner, base+".."+currentHead, false)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Exact || selection.GroupsPath != reviewedPath || selection.CurrentBaseSHA != base || selection.CurrentHeadSHA != currentHead || selection.ReviewHeadSHA != reviewedHead {
		t.Fatalf("unexpected ancestor selection: %+v", selection)
	}
	if _, err := resolveViewForRange(context.Background(), runner, base+".."+currentHead, true); err == nil || !strings.Contains(err.Error(), "no finalized review for current range") {
		t.Fatalf("exact lookup should reject an ancestor review, got %v", err)
	}
	resolution, err := resolveReview(context.Background(), runner, base+".."+currentHead, false)
	if err != nil {
		t.Fatal(err)
	}
	if !resolution.Found || resolution.Exact || resolution.GroupsPath != reviewedPath || resolution.ReviewHeadSHA != reviewedHead || resolution.CommitsBehind != 1 {
		t.Fatalf("unexpected review resolution: %+v", resolution)
	}

	exactPath := writeReview(base, currentHead)
	selection, err = resolveViewForRange(context.Background(), runner, base+".."+currentHead, false)
	if err != nil {
		t.Fatal(err)
	}
	if !selection.Exact || selection.GroupsPath != exactPath {
		t.Fatalf("unexpected exact selection: %+v", selection)
	}
	resolution, err = resolveReview(context.Background(), runner, base+".."+currentHead, false)
	if err != nil {
		t.Fatal(err)
	}
	if !resolution.Exact || resolution.CommitsBehind != 0 {
		t.Fatalf("unexpected exact review resolution: %+v", resolution)
	}
}

func TestViewWithoutDraftFallsBackToAncestorReviewAndShowsDrift(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", repo}, args...)...)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	commit := func(path, content, subject string) string {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		git("add", ".")
		git("commit", "-qm", subject)
		return git("rev-parse", "HEAD")
	}

	git("init", "-q", "-b", "main")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test")
	base := commit("app.txt", "base\n", "base")
	git("switch", "-qc", "feature")
	reviewedHead := commit("app.txt", "reviewed\n", "reviewed")
	runner := gitdiff.Runner{Dir: repo}
	changes, err := runner.Changes(context.Background(), base+".."+reviewedHead)
	if err != nil {
		t.Fatal(err)
	}
	fragments := gitdiff.SuggestedFragments(changes)
	for index := range fragments {
		fragments[index].Description = "Explains the reviewed change."
		fragments[index].ReviewLevel = model.ReviewLevelNormal
	}
	groupsFile := model.GroupsFile{Version: 2, BaseSHA: base, HeadSHA: reviewedHead, Groups: []model.SemanticGroup{{
		ID: "reviewed", Title: "Reviewed", Summary: "The reviewed snapshot.", Importance: model.ImportanceCore,
		FileCategories: []model.FileCategory{{Path: "app.txt", Category: "logic"}}, Fragments: fragments,
	}}}
	groupsPath := filepath.Join(repo, reviews.LocalPath(base, reviewedHead))
	if err := saveJSONAtomic(groupsPath, groupsFile); err != nil {
		t.Fatal(err)
	}
	currentHead := commit("follow-up.txt", "later\n", "follow-up")
	refreshDraftPath := filepath.Join(t.TempDir(), "refresh-draft.json")
	if err := runGroupingInit(context.Background(), runner, []string{base + ".." + currentHead, "--from", groupsPath, "--draft", refreshDraftPath}); err != nil {
		t.Fatal(err)
	}
	refreshDraft, err := groupingdraft.Load(refreshDraftPath)
	if err != nil {
		t.Fatal(err)
	}
	if refreshDraft.BaseSHA != base || refreshDraft.HeadSHA != currentHead || len(refreshDraft.Groups) != 1 || len(refreshDraft.Fragments) != len(fragments) {
		t.Fatalf("refresh draft did not carry forward the source review: %+v", refreshDraft)
	}
	if refreshDraft.Status().ReadyToFinalize {
		t.Fatal("refresh draft should require grouping the follow-up commit")
	}
	git("update-ref", "refs/remotes/origin/main", base)
	git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	git("config", "branch.feature.remote", "origin")

	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	oldWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWorkingDirectory)
	htmlPath := filepath.Join(t.TempDir(), "review.html")
	if err := run(context.Background(), []string{"view", "--html", htmlPath}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"This semantic review is 1 unreviewed commit behind HEAD.", reviewedHead, currentHead, "follow-up", "follow-up.txt"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("exported ancestor review is missing %q", want)
		}
	}
	if _, err := os.Stat(defaultGroupingDraftPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("view should not require or create a grouping draft, stat error = %v", err)
	}
}

func stringPtrForTest(value string) *string { return &value }
