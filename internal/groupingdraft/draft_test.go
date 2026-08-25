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
	return New(model.ChangeMap{BaseSHA: "base", HeadSHA: "head", Changes: []model.DiffChange{
		{ID: "F1", Path: "src/a.ts", OldStart: 10, OldLines: 2, NewStart: 10, NewLines: 3},
		{ID: "F2", Path: "b.test.ts", NewStart: 1, NewLines: 4},
	}}, []categories.Suggestion{{Path: "src/a.ts", Category: "logic"}, {Path: "b.test.ts", Category: "test"}})
}

func withDescription(fragment model.Fragment, description string) *model.Fragment {
	fragment.Description = description
	return &fragment
}

func TestApplyEditsAndAssignsFragments(t *testing.T) {
	draft := fixtureDraft()
	updated, err := Apply(draft, ApplyRequest{Operations: []Operation{
		{Op: "upsert_group", GroupID: "logic", Title: stringPtr("Logic"), Summary: stringPtr("Explains the logic change.")},
		{Op: "update_fragment", Fragment: withDescription(draft.Fragments[0], "Introduces the shared logic.")},
		{Op: "assign_fragments", GroupID: "logic", Members: []string{"F1"}},
		{Op: "set_file_categories", GroupID: "logic", Categories: map[string]string{"src/a.ts": "logic"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 1 || len(updated.Groups[0].Members) != 1 || updated.Fragments[0].Description == "" {
		t.Fatalf("unexpected draft: %+v", updated)
	}
	if draft.Revision != 0 || len(draft.Groups) != 0 {
		t.Fatal("Apply mutated original")
	}
}

func TestSplitSuggestedNewFileFragment(t *testing.T) {
	draft := fixtureDraft()
	first := model.Fragment{ID: "test-first", Path: "b.test.ts", Ranges: []model.FragmentRange{{New: &model.Range{Start: 1, Lines: 2}}}, Description: "Adds first tests."}
	second := model.Fragment{ID: "test-second", Path: "b.test.ts", Ranges: []model.FragmentRange{{New: &model.Range{Start: 3, Lines: 2}}}, Description: "Adds second tests."}
	updated, err := Apply(draft, ApplyRequest{Operations: []Operation{{Op: "delete_fragments", Members: []string{"F2"}}, {Op: "add_fragment", Fragment: &first}, {Op: "add_fragment", Fragment: &second}}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.fragmentIndex("F2") >= 0 || updated.fragmentIndex("test-first") < 0 || updated.fragmentIndex("test-second") < 0 {
		t.Fatalf("split failed: %+v", updated.Fragments)
	}
}

func TestApplyRejectsInvalidBatchWithoutMutation(t *testing.T) {
	draft := fixtureDraft()
	updated, err := Apply(draft, ApplyRequest{Operations: []Operation{{Op: "upsert_group", GroupID: "logic", Title: stringPtr("Logic")}, {Op: "assign_fragments", GroupID: "logic", Members: []string{"missing"}}}})
	if err == nil || !strings.Contains(err.Error(), "unknown fragment ID missing") {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Revision != draft.Revision || len(updated.Groups) != len(draft.Groups) {
		t.Fatal("failed batch mutated draft")
	}
}

func TestFinalizeEmbedsDefinitions(t *testing.T) {
	draft := fixtureDraft()
	for index := range draft.Fragments {
		draft.Fragments[index].Description = "Describes the change."
	}
	var err error
	draft, err = Apply(draft, ApplyRequest{Operations: []Operation{
		{Op: "upsert_group", GroupID: "all", Title: stringPtr("All"), Summary: stringPtr("Explains all changes.")},
		{Op: "assign_fragments", GroupID: "all", Members: []string{"F1", "F2"}},
		{Op: "set_file_categories", GroupID: "all", Categories: map[string]string{"src/a.ts": "logic", "b.test.ts": "test"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if errors := draft.FinalErrors(); len(errors) != 0 {
		t.Fatal(errors)
	}
	result := draft.ToGroupsFile()
	if len(result.Groups[0].Fragments) != 2 || len(result.Groups[0].Fragments[0].Ranges) == 0 {
		t.Fatalf("definitions not embedded: %+v", result)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	draft := fixtureDraft()
	path := filepath.Join(t.TempDir(), "draft.json")
	if err := SaveAtomic(path, draft); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Changes) != 2 || len(loaded.Fragments) != 2 {
		t.Fatalf("round trip changed draft: %+v", loaded)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func stringPtr(value string) *string { return &value }
