package groups

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ry023/semdiff/internal/model"
)

func lineRange(start, lines int) *model.Range { return &model.Range{Start: start, Lines: lines} }

func fixture() (model.GroupsFile, model.ChangeMap) {
	changes := model.ChangeMap{BaseSHA: "aaa", HeadSHA: "bbb", Changes: []model.DiffChange{
		{Path: "a.go", OldStart: 10, OldLines: 2, NewStart: 10, NewLines: 3},
		{Path: "new.go", NewStart: 1, NewLines: 4},
	}}
	fileCategories := []model.FileCategory{{Path: "a.go", Category: "logic"}, {Path: "new.go", Category: "logic"}}
	groups := model.GroupsFile{Version: 1, BaseSHA: "aaa", HeadSHA: "bbb", Groups: []model.SemanticGroup{{
		ID: "logic", Title: "Logic", Summary: "Explains the change.", Importance: model.ImportanceCore, FileCategories: fileCategories,
		Fragments: []model.Fragment{
			{ID: "replace", Path: "a.go", Ranges: []model.FragmentRange{{Old: lineRange(10, 2), New: lineRange(10, 3)}}, Description: "Replaces the behavior.", Importance: model.ImportanceCore},
			{ID: "new-top", Path: "new.go", Ranges: []model.FragmentRange{{New: lineRange(1, 2)}}, Description: "Adds the first concern.", Importance: model.ImportanceSupporting},
			{ID: "new-bottom", Path: "new.go", Ranges: []model.FragmentRange{{New: lineRange(3, 2)}}, Description: "Adds the second concern.", Importance: model.ImportanceSupporting},
		},
	}}}
	return groups, changes
}

func TestValidateRangeCoverage(t *testing.T) {
	g, changes := fixture()
	if report := ValidateReport(g, changes); len(report.Errors) != 0 {
		t.Fatal(report.Errors)
	}
}

func TestValidateRequiresKnownImportanceAtBothLevels(t *testing.T) {
	g, changes := fixture()
	g.Groups[0].Importance = "urgent"
	g.Groups[0].Fragments[0].Importance = ""
	errors := strings.Join(Validate(g, changes), "\n")
	if !strings.Contains(errors, `group logic has invalid importance "urgent"`) || !strings.Contains(errors, `fragment replace in group logic has invalid importance ""`) {
		t.Fatal(errors)
	}
}

func TestValidateDetectsGapAndOverlap(t *testing.T) {
	g, changes := fixture()
	g.Groups[0].Fragments[1].Ranges[0].New.Lines = 1
	errors := strings.Join(Validate(g, changes), "\n")
	if !strings.Contains(errors, "unassigned changed range: new.go new:2") {
		t.Fatal(errors)
	}

	g, changes = fixture()
	g.Groups[0].Fragments[1].Ranges[0].New.Lines = 3
	errors = strings.Join(Validate(g, changes), "\n")
	if !strings.Contains(errors, "assigned to multiple fragments: new.go new:3") {
		t.Fatal(errors)
	}
}

func TestValidateAllowsDiscontiguousRanges(t *testing.T) {
	g, changes := fixture()
	g.Groups[0].Fragments[1].Ranges = append(g.Groups[0].Fragments[1].Ranges, model.FragmentRange{New: lineRange(4, 1)})
	g.Groups[0].Fragments[2].Ranges[0] = model.FragmentRange{New: lineRange(3, 1)}
	if errors := Validate(g, changes); len(errors) != 0 {
		t.Fatal(errors)
	}
}

func TestValidateMetadataOnlyChange(t *testing.T) {
	changes := model.ChangeMap{BaseSHA: "a", HeadSHA: "b", Changes: []model.DiffChange{{Path: "renamed.go"}}}
	g := model.GroupsFile{Version: 1, BaseSHA: "a", HeadSHA: "b", Groups: []model.SemanticGroup{{
		ID: "rename", Title: "Rename", Summary: "Renames the file.", Importance: model.ImportanceIncidental, FileCategories: []model.FileCategory{{Path: "renamed.go", Category: "logic"}},
		Fragments: []model.Fragment{{ID: "rename-file", Path: "renamed.go", FileMetadata: true, Description: "Renames the file.", Importance: model.ImportanceCore}},
	}}}
	if errors := Validate(g, changes); len(errors) != 0 {
		t.Fatal(errors)
	}
}

func TestLoadRejectsOldFragmentIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "groups.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"base_sha":"a","head_sha":"b","groups":[{"id":"g","title":"G","summary":"S","fragment_ids":["F1"]}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "unknown field \"fragment_ids\"") {
		t.Fatalf("old schema was accepted: %v", err)
	}
}
