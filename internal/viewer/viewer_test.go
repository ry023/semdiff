package viewer

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ry023/semdiff/internal/model"
)

func TestBuildAndHandler(t *testing.T) {
	one, two := 1, 2
	g := model.GroupsFile{BaseSHA: "aaa", HeadSHA: "bbb", Groups: []model.SemanticGroup{{ID: "later", Title: "Later", Order: &two, Fragments: []model.Fragment{{ID: "F2", Path: "b.go"}}}, {ID: "first", Title: "First", Summary: "Start here\nMore context.", Order: &one, FileCategories: []model.FileCategory{{Path: "a.go", Category: "logic"}}, Fragments: []model.Fragment{{ID: "F1", Path: "a.go", Description: "Explains the <safe> change."}}}}}
	inv := model.FragmentSet{Fragments: []model.MaterializedFragment{{ID: "F1", Path: "a.go", NewStart: 5, NewLines: 3, Patch: "diff --git a/a.go b/a.go\n@@ -5,3 +5,3 @@\n-unsafe <tag>\n+safe\n context\n"}, {ID: "F2", Path: "b.go", Patch: "patch\n"}}}
	p := Build(g, inv, map[string]string{"a.go": strings.Repeat("source line\n", 20)})
	if p.Groups[0].ID != "first" || p.FileCount != 2 || p.FragmentCount != 2 {
		t.Fatalf("unexpected page: %+v", p)
	}
	if len(p.Groups[0].Categories) != 1 || p.Groups[0].Categories[0].Name != "logic" || len(p.Groups[0].Categories[0].Files) != 1 {
		t.Fatalf("unexpected group categories: %+v", p.Groups[0].Categories)
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
	if !strings.Contains(body, "Start here<br") || !strings.Contains(body, "More context.") {
		t.Fatal("group summary line break was not rendered")
	}
	if !strings.Contains(body, ".summary{color:var(--muted);margin:8px 0 0 24px;white-space:pre-line}") {
		t.Fatal("group summary line breaks are not preserved in the viewer")
	}
	if !strings.Contains(body, ".summary code{padding:2px 5px;border:1px solid var(--line)") {
		t.Fatal("inline Markdown code has no visual styling")
	}
	if !strings.Contains(body, ".summary code{padding:2px 5px;border:1px solid var(--line);border-radius:4px;background:#1f2630;color:#8b949e;") {
		t.Fatal("inline Markdown code should use a neutral gray color")
	}
	if !strings.Contains(body, `<details class="category">`) || strings.Contains(body, `<details class="category" open>`) || !strings.Contains(body, `class="category-icon logic"`) || !strings.Contains(body, `<svg viewBox="0 0 24 24"`) {
		t.Fatal("categorized file sections are not rendered collapsed with a standard icon")
	}
	if !strings.Contains(body, ".category-icon{display:inline-flex;width:18px;height:18px;margin-right:7px;vertical-align:-4px;color:#8b949e}") {
		t.Fatal("category icons should use a shared neutral color")
	}
	if !strings.Contains(body, ".category-stats{display:inline-flex;gap:10px;margin-left:10px;font-size:12px}.category-stat.added{color:#3fb950}.category-stat.updated{color:#d29922}.category-stat.deleted{color:#f85149}") {
		t.Fatal("category file status counts should be styled by status")
	}
	if !strings.Contains(body, ".file-status-icon.updated{color:#d29922}") {
		t.Fatal("updated file icons should use the update yellow")
	}
	if !strings.Contains(body, `<span class="category-stat updated">1 files updated</span>`) || strings.Contains(body, `class="category-stat added"`) || strings.Contains(body, `class="category-stat deleted"`) {
		t.Fatal("category file status counts should omit zero statuses")
	}
	if !strings.Contains(body, ".file h3{display:inline;font:400 14px ui-monospace,monospace;margin:0;color:var(--text)}.file-path{font-weight:400}.file-name{font-weight:700}") {
		t.Fatal("file paths should be regular white text with bold file names")
	}
	if !strings.Contains(body, "Explains the &lt;safe&gt; change.") {
		t.Fatal("missing or unsafe fragment description")
	}
	fileHeading := strings.Index(body, "<h3><span class=\"file-name\">a.go</span></h3>")
	if fileHeading < 0 {
		t.Fatal("missing file heading")
	}
	summaryStart := strings.LastIndex(body[:fileHeading], "<summary>")
	descriptionAt := strings.Index(body, "Explains the &lt;safe&gt; change.")
	if summaryStart < 0 {
		t.Fatal("missing file summary")
	}
	summaryEnd := strings.Index(body[summaryStart:], "</summary>")
	if summaryEnd < 0 || descriptionAt < summaryStart || descriptionAt >= summaryStart+summaryEnd {
		t.Fatal("fragment description is not visible in the collapsed file summary")
	}
	if strings.Count(body, "Explains the &lt;safe&gt; change.") != 2 {
		t.Fatal("fragment description should appear in both the file summary and expanded diff")
	}
	if !strings.Contains(body, `class="file-status-icon updated"`) || !strings.Contains(body, `class="stat-add">+1`) || !strings.Contains(body, `class="stat-del">-1`) {
		t.Fatal("missing file status or line-change statistics")
	}
	if strings.Count(body, `class="diffstat-block `) != 10 {
		t.Fatal("each file should render a five-block diffstat")
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
	if !strings.Contains(body, ".file[open]>summary{position:sticky;top:0") {
		t.Fatal("open file heading is not sticky")
	}
	if !strings.Contains(body, ".group{border:1px solid var(--line);border-radius:8px;background:var(--panel);margin:14px 0;overflow:clip}") {
		t.Fatal("group clipping prevents sticky file headings")
	}
	if !strings.Contains(body, "batch[0].before(button)") || !strings.Contains(body, "batch[batch.length-1].after(button)") {
		t.Fatal("context controls do not move to the expanded range boundary")
	}
	if !strings.Contains(body, "while(hunk&&!hunk.classList.contains('hunk'))") || !strings.Contains(body, "hunk.remove()") {
		t.Fatal("upper expansion does not remove the stale hunk header")
	}
	if !strings.Contains(body, `<details class="group" open>`) {
		t.Fatal("groups should be open by default")
	}
	if strings.Contains(body, `<details class="category" open>`) {
		t.Fatal("categories should be collapsed by default")
	}
	if strings.Contains(body, `<details class="file" open>`) {
		t.Fatal("files should be collapsed by default")
	}
	if strings.Contains(body, "unsafe <tag>") || !strings.Contains(body, "unsafe &lt;tag&gt;") {
		t.Fatal("patch was not safely escaped")
	}
}

func TestFormatRangesPreservesDiscontiguousDefinition(t *testing.T) {
	fragment := model.Fragment{FileMetadata: true, Ranges: []model.FragmentRange{
		{Old: &model.Range{Start: 10, Lines: 2}, New: &model.Range{Start: 10, Lines: 4}},
		{New: &model.Range{Start: 40, Lines: 3}},
	}}
	if got, want := formatRanges(fragment), "-10,2 +10,4; -∅ +40,3; metadata"; got != want { t.Fatalf("got %q, want %q", got, want) }
}

func TestDiffstatBlocks(t *testing.T) {
	tests := []struct {
		additions, deletions int
		want                 string
	}{
		{102, 84, "added,added,deleted,deleted,neutral"},
		{8, 31, "added,deleted,deleted,deleted,neutral"},
		{172, 0, "added,added,added,added,added"},
		{0, 0, "neutral,neutral,neutral,neutral,neutral"},
	}
	for _, tt := range tests {
		if got := strings.Join(diffstatBlocks(tt.additions, tt.deletions), ","); got != tt.want {
			t.Errorf("diffstatBlocks(%d, %d) = %q, want %q", tt.additions, tt.deletions, got, tt.want)
		}
	}
}

func TestFileStatusAndLineCounts(t *testing.T) {
	tests := []struct {
		name, patch, status, iconFragment string
		additions, deletions              int
	}{
		{"new", "diff --git a/new.go b/new.go\nnew file mode 100644\n--- /dev/null\n+++ b/new.go\n@@ -0,0 +1,2 @@\n+one\n+two\n", "new", "M12 18v-6", 2, 0},
		{"updated", "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-old\n+new\n", "updated", "m10.4 12.6", 1, 1},
		{"deleted", "diff --git a/old.go b/old.go\ndeleted file mode 100644\n--- a/old.go\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-one\n-two\n", "deleted", "M9 15h6", 0, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fragment := model.MaterializedFragment{ID: "F1", Path: "a.go", Patch: tt.patch}
			file := buildFileView("a.go", []model.MaterializedFragment{fragment}, "", []model.MaterializedFragment{fragment})
			if file.Status != tt.status || !strings.Contains(string(file.StatusIcon), tt.iconFragment) || file.Additions != tt.additions || file.Deletions != tt.deletions {
				t.Fatalf("unexpected file metadata: %+v", file)
			}
			if !strings.Contains(string(file.StatusIcon), `<svg viewBox="0 0 24 24"`) {
				t.Fatalf("status icon is not an inline SVG: %s", file.StatusIcon)
			}
		})
	}
}

func TestFileViewSplitsDirectoryAndName(t *testing.T) {
	fragment := model.MaterializedFragment{ID: "F1", Path: "web/src/Button.tsx", Patch: "diff --git a/web/src/Button.tsx b/web/src/Button.tsx\n"}
	file := buildFileView(fragment.Path, []model.MaterializedFragment{fragment}, "", []model.MaterializedFragment{fragment})
	if file.Directory != "web/src/" || file.Name != "Button.tsx" {
		t.Fatalf("unexpected path split: directory=%q name=%q", file.Directory, file.Name)
	}
	page := Build(model.GroupsFile{Groups: []model.SemanticGroup{{ID: "g", Title: "Group", Fragments: []model.Fragment{{ID: "F1", Path: fragment.Path}}}}}, model.FragmentSet{Fragments: []model.MaterializedFragment{fragment}})
	handler, err := Handler(page)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(response.Body.String(), `<h3><span class="file-path">web/src/</span><span class="file-name">Button.tsx</span></h3>`) {
		t.Fatal("nested file heading did not render separate path and name spans")
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

func TestRenderMarkdown(t *testing.T) {
	html := string(renderMarkdown("# Heading\n\n**important** and `foobar`\n\n- first\n- second\n\n<script>alert(1)</script>"))
	for _, want := range []string{"<h1>Heading</h1>", "<strong>important</strong>", "<code>foobar</code>", "<li>first</li>", "<li>second</li>"} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered markdown is missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, "<script>") {
		t.Fatalf("raw HTML must not be executable: %s", html)
	}
}

func TestCategoryViewsUseRequestedOrder(t *testing.T) {
	files := []FileView{{Path: "a.ts"}, {Path: "b.tsx"}, {Path: "c.yml"}, {Path: "d.go"}, {Path: "e.test.ts"}, {Path: "f.md"}, {Path: "g.ts"}}
	declared := []model.FileCategory{
		{Path: "a.ts", Category: "logic"},
		{Path: "b.tsx", Category: "component"},
		{Path: "c.yml", Category: "config"},
		{Path: "d.go", Category: "implementation"},
		{Path: "e.test.ts", Category: "test"},
		{Path: "f.md", Category: "unknown"},
		{Path: "g.ts", Category: "custom"},
	}
	views := buildCategoryViews(files, declared)
	want := []string{"logic", "component", "config", "implementation", "test", "unknown", "custom"}
	if len(views) != len(want) {
		t.Fatalf("got %d categories, want %d: %+v", len(views), len(want), views)
	}
	for i, category := range views {
		if category.Name != want[i] {
			t.Errorf("category %d = %q, want %q", i, category.Name, want[i])
		}
	}
}

func TestCategoryViewsCountFileStatuses(t *testing.T) {
	files := []FileView{
		{Path: "added.ts", Status: "new"},
		{Path: "updated.ts", Status: "updated"},
		{Path: "deleted.ts", Status: "deleted"},
		{Path: "also-added.ts", Status: "new"},
	}
	views := buildCategoryViews(files, []model.FileCategory{{Path: "added.ts", Category: "logic"}, {Path: "updated.ts", Category: "logic"}, {Path: "deleted.ts", Category: "logic"}, {Path: "also-added.ts", Category: "logic"}})
	if len(views) != 1 || views[0].Added != 2 || views[0].Updated != 1 || views[0].Deleted != 1 {
		t.Fatalf("unexpected category status counts: %+v", views)
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
	f1 := model.MaterializedFragment{ID: "F1", Path: "a.go", NewStart: 10, NewLines: 5, Patch: "@@ -10,5 +10,5 @@\n-old\n+new\n"}
	f2 := model.MaterializedFragment{ID: "F2", Path: "a.go", NewStart: 30, NewLines: 5, Patch: "@@ -30,5 +30,5 @@\n-old\n+new\n"}
	group := model.GroupsFile{Groups: []model.SemanticGroup{{ID: "g", Title: "Group", Fragments: []model.Fragment{{ID: "F1", Path: "a.go"}, {ID: "F2", Path: "a.go"}}}}}
	page := Build(group, model.FragmentSet{Fragments: []model.MaterializedFragment{f1, f2}}, map[string]string{"a.go": strings.Repeat("line\n", 50)})
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
