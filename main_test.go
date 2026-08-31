package main

import (
	"context"
	"encoding/json"
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

func stringPtrForTest(value string) *string { return &value }
