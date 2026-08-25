package viewer

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ry023/semdiff/internal/model"
)

func TestBuildAndHandler(t *testing.T) {
	one, two := 1, 2
	g := model.GroupsFile{BaseSHA: "aaa", HeadSHA: "bbb", Groups: []model.SemanticGroup{{ID: "later", Title: "Later", Order: &two, FragmentIDs: []string{"F2"}}, {ID: "first", Title: "First", Summary: "Start here", Order: &one, Fragments: []model.FragmentReference{{ID: "F1", Description: "Explains the <safe> change."}}}}}
	inv := model.Inventory{Fragments: []model.DiffFragment{{ID: "F1", Path: "a.go", NewStart: 5, NewLines: 3, Patch: "diff --git a/a.go b/a.go\n@@ -5,3 +5,3 @@\n-unsafe <tag>\n+safe\n context\n"}, {ID: "F2", Path: "b.go", Patch: "patch\n"}}}
	p := Build(g, inv, map[string]string{"a.go": strings.Repeat("source line\n", 20)})
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
	if !strings.Contains(body, "Explains the &lt;safe&gt; change.") {
		t.Fatal("missing or unsafe fragment description")
	}
	if !strings.Contains(body, `data-view="unified"`) || !strings.Contains(body, `data-view="split"`) || !strings.Contains(body, "semdiff-view") {
		t.Fatal("missing page-wide unified/split view controls")
	}
	fragment := p.Groups[0].Files[0].Fragments[0]
	upper, lower := string(fragment.UpperContextHTML), string(fragment.LowerContextHTML)
	if !strings.Contains(upper, "Show 4 lines above") || !strings.Contains(lower, "Show 13 lines below") {
		t.Fatalf("missing directional context controls: upper=%q lower=%q", upper, lower)
	}
	if strings.Index(upper, "context-hidden") > strings.Index(upper, `class="expand-lines"`) {
		t.Fatal("upper expansion rows must be before their button")
	}
	if strings.Index(lower, `class="expand-lines"`) > strings.Index(lower, "context-hidden") {
		t.Fatal("lower expansion rows must be after their button")
	}
	if !strings.Contains(body, ".context-hidden[hidden]{display:none!important}") {
		t.Fatal("hidden context rows are not explicitly hidden by CSS")
	}
	if !strings.Contains(body, "batch[0].before(button)") || !strings.Contains(body, "batch[batch.length-1].after(button)") {
		t.Fatal("context controls do not move to the expanded range boundary")
	}
	if !strings.Contains(body, "while(hunk&&!hunk.classList.contains('hunk'))") || !strings.Contains(body, "hunk.remove()") {
		t.Fatal("upper expansion does not remove the stale hunk header")
	}
	if strings.Contains(body, `<details class="group" open>`) || !strings.Contains(body, `<details class="group">`) || !strings.Contains(body, `<details class="file">`) {
		t.Fatalf("groups and files should be collapsed by default: %s", body)
	}
	if strings.Contains(body, "unsafe <tag>") || !strings.Contains(body, "unsafe &lt;tag&gt;") {
		t.Fatal("patch was not safely escaped")
	}
}

func TestColorPatchDoesNotAddBlankRows(t *testing.T) {
	html := string(colorPatch("+one\n+two\n"))
	if strings.Contains(html, "</span>\n") {
		t.Fatalf("colorPatch added a newline after a block: %q", html)
	}
	if strings.Count(html, `<span class="diff-row`) != 2 {
		t.Fatalf("got unexpected rendered rows: %q", html)
	}
}

func TestColorPatchLineNumbers(t *testing.T) {
	html := string(colorPatch("@@ -7,2 +7,2 @@\n-old\n+new\n context\n"))
	if strings.Count(html, `<span class="line-number unified-cell">7</span>`) != 2 {
		t.Fatalf("deletion and addition should use line 7: %q", html)
	}
	if !strings.Contains(html, `<span class="line-number unified-cell">8</span>`) {
		t.Fatalf("context line should advance to line 8: %q", html)
	}
	if !strings.Contains(html, `<span class="line-number split-cell old-number">7</span>`) || !strings.Contains(html, `<span class="line-number split-cell new-number">7</span>`) {
		t.Fatalf("split view should contain old and new line numbers: %q", html)
	}
}

func TestMultipleFragmentsHaveIndependentContext(t *testing.T) {
	f1 := model.DiffFragment{ID: "F1", Path: "a.go", NewStart: 10, NewLines: 5, Patch: "@@ -10,5 +10,5 @@\n-old\n+new\n"}
	f2 := model.DiffFragment{ID: "F2", Path: "a.go", NewStart: 30, NewLines: 5, Patch: "@@ -30,5 +30,5 @@\n-old\n+new\n"}
	group := model.GroupsFile{Groups: []model.SemanticGroup{{ID: "g", Title: "Group", FragmentIDs: []string{"F1", "F2"}}}}
	page := Build(group, model.Inventory{Fragments: []model.DiffFragment{f1, f2}}, map[string]string{"a.go": strings.Repeat("line\n", 50)})
	fragments := page.Groups[0].Files[0].Fragments
	if len(fragments) != 2 {
		t.Fatalf("got %d fragments, want 2", len(fragments))
	}
	gap := string(fragments[0].LowerContextHTML)
	if !strings.Contains(gap, "Show 15 lines below") || !strings.Contains(gap, "Show 15 lines above") {
		t.Fatalf("fragments do not share a bidirectional context gap: %q", gap)
	}
	if fragments[1].UpperContextHTML != "" {
		t.Fatalf("second fragment duplicated the shared context gap: %q", fragments[1].UpperContextHTML)
	}
	handler, err := Handler(page)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
	if got := strings.Count(response.Body.String(), "<pre>"); got != 1 {
		t.Fatalf("same-file fragments rendered in %d diff views, want 1", got)
	}
}
