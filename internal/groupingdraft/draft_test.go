package groupingdraft

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ry023/semdiff/internal/categories"
	"github.com/ry023/semdiff/internal/model"
)

func fixtureDraft() Draft {
	return New(model.Inventory{
		BaseSHA: "base",
		HeadSHA: "head",
		Fragments: []model.DiffFragment{
			{ID: "F2", Path: "b.test.ts", NewStart: 20, NewLines: 2},
			{ID: "F1", Path: "src/a.ts", NewStart: 10, NewLines: 3},
		},
	}, []categories.Suggestion{
		{Path: "src/a.ts", Category: "logic"},
		{Path: "b.test.ts", Category: "test"},
	})
}

func TestApplyBuildsDraftIncrementally(t *testing.T) {
	draft := fixtureDraft()
	updated, err := Apply(draft, ApplyRequest{Operations: []Operation{
		{Op: "upsert_group", GroupID: "logic", Title: stringPtr("Logic"), Summary: stringPtr("Explains the logic change.")},
		{Op: "assign_fragments", GroupID: "logic", FragmentIDs: []string{"F1"}},
		{Op: "describe_fragments", Descriptions: map[string]string{"F1": "Introduces the shared logic."}},
		{Op: "set_file_categories", GroupID: "logic", Categories: map[string]string{"src/a.ts": "logic"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 1 || len(updated.Groups) != 1 || len(updated.Groups[0].FragmentIDs) != 1 || updated.Descriptions["F1"] == "" {
		t.Fatalf("unexpected incremental draft: %+v", updated)
	}
	if draft.Revision != 0 || len(draft.Groups) != 0 {
		t.Fatalf("Apply mutated the original draft: %+v", draft)
	}
	status := updated.Status()
	if status.AssignedFragmentCount != 1 || len(status.UnassignedFragmentIDs) != 1 || len(status.UndescribedFragmentIDs) != 0 || status.ReadyToFinalize {
		t.Fatalf("unexpected partial status: %+v", status)
	}
}

func TestApplyRejectsInvalidBatchWithoutMutation(t *testing.T) {
	draft := fixtureDraft()
	updated, err := Apply(draft, ApplyRequest{Operations: []Operation{
		{Op: "upsert_group", GroupID: "logic", Title: stringPtr("Logic")},
		{Op: "assign_fragments", GroupID: "logic", FragmentIDs: []string{"missing"}},
	}})
	if err == nil || !strings.Contains(err.Error(), "unknown fragment ID missing") {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Revision != draft.Revision || len(updated.Groups) != len(draft.Groups) {
		t.Fatalf("failed batch mutated the draft: before=%+v after=%+v", draft, updated)
	}
}

func TestMoveAndFinalize(t *testing.T) {
	draft := fixtureDraft()
	var err error
	draft, err = Apply(draft, ApplyRequest{Operations: []Operation{
		{Op: "upsert_group", GroupID: "first", Title: stringPtr("First"), Summary: stringPtr("First summary")},
		{Op: "upsert_group", GroupID: "second", Title: stringPtr("Second"), Summary: stringPtr("Second summary")},
		{Op: "assign_fragments", GroupID: "first", FragmentIDs: []string{"F1", "F2"}},
		{Op: "move_fragments", GroupID: "second", FragmentIDs: []string{"F2"}},
		{Op: "describe_fragments", Descriptions: map[string]string{"F1": "Logic behavior.", "F2": "Test coverage."}},
		{Op: "set_file_categories", GroupID: "first", Categories: map[string]string{"src/a.ts": "logic"}},
		{Op: "set_file_categories", GroupID: "second", Categories: map[string]string{"b.test.ts": "test"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got := draft.FinalErrors(); len(got) != 0 {
		t.Fatalf("complete draft has errors: %v", got)
	}
	groups := draft.ToGroupsFile()
	if len(groups.Groups) != 2 || len(groups.Groups[0].Fragments) != 1 || groups.Groups[1].Fragments[0].Description != "Test coverage." {
		t.Fatalf("unexpected finalized groups: %+v", groups)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	draft := fixtureDraft()
	path := filepath.Join(t.TempDir(), "grouping-draft.json")
	if err := SaveAtomic(path, draft); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.BaseSHA != draft.BaseSHA || loaded.HeadSHA != draft.HeadSHA || len(loaded.Fragments) != len(draft.Fragments) {
		t.Fatalf("round trip changed draft: %+v", loaded)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func stringPtr(value string) *string { return &value }
