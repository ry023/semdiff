package viewer

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ry023/semdiff/internal/model"
)

func TestBuildAndHandler(t *testing.T) {
	one, two := 1, 2
	g := model.GroupsFile{BaseSHA: "aaa", HeadSHA: "bbb", Groups: []model.SemanticGroup{{ID: "later", Title: "Later", Order: &two, FragmentIDs: []string{"F2"}}, {ID: "first", Title: "First", Summary: "Start here", Order: &one, FragmentIDs: []string{"F1"}}}}
	inv := model.Inventory{Fragments: []model.DiffFragment{{ID: "F1", Path: "a.go", Patch: "@@ -1 +1 @@\n-unsafe <tag>\n+safe\n"}, {ID: "F2", Path: "b.go", Patch: "patch\n"}}}
	p := Build(g, inv)
	if p.Groups[0].ID != "first" || p.FileCount != 2 || p.FragmentCount != 2 {
		t.Fatalf("unexpected page: %+v", p)
	}
	h, err := Handler(p)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	body := w.Body.String()
	if !strings.Contains(body, "Semantic Changes") || !strings.Contains(body, "Start here") {
		t.Fatal("missing viewer content")
	}
	if strings.Contains(body, "unsafe <tag>") || !strings.Contains(body, "unsafe &lt;tag&gt;") {
		t.Fatal("patch was not safely escaped")
	}
}
