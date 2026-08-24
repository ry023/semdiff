package groups

import (
	"strings"
	"testing"

	"github.com/ry023/semdiff/internal/model"
)

func fixture() (model.GroupsFile, model.Inventory) {
	inv := model.Inventory{BaseSHA: "aaa", HeadSHA: "bbb", Fragments: []model.DiffFragment{{ID: "F1", Path: "a"}, {ID: "F2", Path: "b"}}}
	g := model.GroupsFile{Version: 1, BaseSHA: "aaa", HeadSHA: "bbb", Groups: []model.SemanticGroup{{ID: "g1", Title: "One", FragmentIDs: []string{"F1"}}, {ID: "g2", Title: "Two", FragmentIDs: []string{"F2"}}}}
	return g, inv
}

func TestValidateOK(t *testing.T) {
	g, inv := fixture()
	if errs := Validate(g, inv); len(errs) > 0 {
		t.Fatal(errs)
	}
}

func TestValidateFailures(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model.GroupsFile)
		want   string
	}{
		{"unassigned", func(g *model.GroupsFile) { g.Groups[1].FragmentIDs = nil }, "unassigned fragment"},
		{"multiple", func(g *model.GroupsFile) { g.Groups[1].FragmentIDs = []string{"F1", "F2"} }, "multiple groups"},
		{"unknown", func(g *model.GroupsFile) { g.Groups[0].FragmentIDs = []string{"F1", "missing"} }, "unknown fragment"},
		{"duplicate group", func(g *model.GroupsFile) { g.Groups[1].ID = "g1" }, "duplicate group ID"},
		{"base mismatch", func(g *model.GroupsFile) { g.BaseSHA = "wrong" }, "base_sha mismatch"},
		{"head mismatch", func(g *model.GroupsFile) { g.HeadSHA = "wrong" }, "head_sha mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, inv := fixture()
			tt.mutate(&g)
			errs := Validate(g, inv)
			if !strings.Contains(strings.Join(errs, "\n"), tt.want) {
				t.Fatalf("errors %v do not contain %q", errs, tt.want)
			}
		})
	}
}
