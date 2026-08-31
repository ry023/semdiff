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

func TestNewKeepsSuggestionsSeparateFromAuthoredFragments(t *testing.T) {
	draft := fixtureDraft()
	if len(draft.Suggestions) != 2 || len(draft.Fragments) != 0 {
		t.Fatalf("suggestions were reified as fragments: %+v", draft)
	}
	status := draft.Status()
	if status.SuggestionCount != 2 || status.FragmentCount != 0 || status.ReadyToFinalize {
		t.Fatalf("unexpected initial status: %+v", status)
	}
}

func TestNewFromGroupsCarriesSemanticDecisionsIntoFreshInventory(t *testing.T) {
	source := model.GroupsFile{Version: 2, BaseSHA: "base", HeadSHA: "old-head", Groups: []model.SemanticGroup{{
		ID: "logic", Title: "Logic", Summary: "Explains the existing behavior.", Importance: model.ImportanceCore,
		FileCategories: []model.FileCategory{{Path: "src/a.ts", Category: "logic"}},
		Fragments:      []model.Fragment{{ID: "behavior", Path: "src/a.ts", Ranges: []model.FragmentRange{{New: &model.Range{Start: 3, Lines: 2}}}, Description: "Implements the existing behavior.", ReviewLevel: model.ReviewLevelCareful}},
	}}}
	inv := model.ChangeMap{BaseSHA: "base", HeadSHA: "new-head", Changes: []model.DiffChange{{ID: "new", Path: "new.ts", NewStart: 1, NewLines: 1}}}
	draft := NewFromGroups(inv, []categories.Suggestion{{Path: "new.ts", Category: "implementation"}}, source)
	if draft.BaseSHA != "base" || draft.HeadSHA != "new-head" || len(draft.Suggestions) != 1 || len(draft.Fragments) != 1 || len(draft.Groups) != 1 {
		t.Fatalf("unexpected refreshed draft: %+v", draft)
	}
	if draft.Groups[0].Members[0] != "behavior" || draft.Fragments[0].Description != "Implements the existing behavior." {
		t.Fatalf("semantic decisions were not carried forward: %+v", draft)
	}
}

func TestMergeSuggestionsCreatesMultiRangeFragment(t *testing.T) {
	draft := New(model.ChangeMap{BaseSHA: "base", HeadSHA: "head", Changes: []model.DiffChange{
		{ID: "S1", Path: "src/a.ts", OldStart: 10, OldLines: 2, NewStart: 10, NewLines: 3},
		{ID: "S2", Path: "src/a.ts", OldStart: 30, OldLines: 1, NewStart: 31, NewLines: 1},
	}}, nil)
	merged := model.Fragment{ID: "behavior", Description: "Updates one complete behavior across both locations."}
	updated, err := Apply(draft, ApplyRequest{Operations: []Operation{{Op: "merge_fragments", Members: []string{"S1", "S2"}, Fragment: &merged}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Fragments) != 1 || updated.Fragments[0].Path != "src/a.ts" || len(updated.Fragments[0].Ranges) != 2 {
		t.Fatalf("suggestions were not composed: %+v", updated.Fragments)
	}
	if len(updated.Suggestions) != 2 {
		t.Fatalf("mechanical suggestions were mutated: %+v", updated.Suggestions)
	}
}

func TestMergeAuthoredFragmentsPreservesSharedAssignment(t *testing.T) {
	draft := New(model.ChangeMap{BaseSHA: "base", HeadSHA: "head", Changes: []model.DiffChange{
		{ID: "S1", Path: "src/a.ts", NewStart: 10, NewLines: 1},
		{ID: "S2", Path: "src/a.ts", NewStart: 30, NewLines: 1},
	}}, nil)
	first, second := draft.Suggestions[0], draft.Suggestions[1]
	first.Description, second.Description = "First part.", "Second part."
	var err error
	draft, err = Apply(draft, ApplyRequest{Operations: []Operation{
		{Op: "upsert_group", GroupID: "logic", Title: stringPtr("Logic")},
		{Op: "add_fragment", Fragment: &first}, {Op: "add_fragment", Fragment: &second},
		{Op: "assign_fragments", GroupID: "logic", Members: []string{"S1", "S2"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	merged := model.Fragment{ID: "complete", Description: "Implements the complete behavior."}
	draft, err = Apply(draft, ApplyRequest{Operations: []Operation{{Op: "merge_fragments", Members: []string{"S1", "S2"}, Fragment: &merged}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(draft.Fragments) != 1 || draft.assignment("complete") != "logic" || draft.assignment("S1") != "" {
		t.Fatalf("assignment was not preserved: %+v", draft)
	}
}

func TestApplyEditsAndAssignsFragments(t *testing.T) {
	draft := fixtureDraft()
	suggestion, _ := draft.fragmentOrSuggestion("F1")
	updated, err := Apply(draft, ApplyRequest{Operations: []Operation{
		{Op: "upsert_group", GroupID: "logic", Title: stringPtr("Logic"), Summary: stringPtr("Explains the logic change.")},
		{Op: "add_fragment", Fragment: withDescription(suggestion, "Introduces the shared logic.")},
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
	updated, err := Apply(draft, ApplyRequest{Operations: []Operation{{Op: "add_fragment", Fragment: &first}, {Op: "add_fragment", Fragment: &second}}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.fragmentIndex("test-first") < 0 || updated.fragmentIndex("test-second") < 0 {
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
	for _, suggestion := range draft.Suggestions {
		suggestion.Description = "Describes the change."
		suggestion.ReviewLevel = model.ReviewLevelCareful
		draft.Fragments = append(draft.Fragments, suggestion)
	}
	core := model.ImportanceCore
	var err error
	draft, err = Apply(draft, ApplyRequest{Operations: []Operation{
		{Op: "upsert_group", GroupID: "all", Title: stringPtr("All"), Summary: stringPtr("Explains all changes."), Importance: &core},
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
	if len(loaded.Changes) != 2 || len(loaded.Suggestions) != 2 || len(loaded.Fragments) != 0 {
		t.Fatalf("round trip changed draft: %+v", loaded)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRejectsVersionOneDraft(t *testing.T) {
	path := filepath.Join(t.TempDir(), "draft.json")
	if err := os.WriteFile(path, []byte(`{"draft_version":1,"base_sha":"base","head_sha":"head","changes":[],"fragments":[],"groups":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "draft_version must be 3") {
		t.Fatalf("version 1 draft was accepted: %v", err)
	}
}

func stringPtr(value string) *string { return &value }
