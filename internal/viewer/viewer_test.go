package viewer

import (
	"net/http"
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
	if strings.Contains(upper, "Show ") || !strings.Contains(lower, "Show 8 lines below") {
		t.Fatalf("missing directional context controls: upper=%q lower=%q", upper, lower)
	}
	if strings.Contains(upper, "context-hidden") {
		t.Fatal("fewer than five upper context lines should all be initially visible")
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
	if strings.Contains(body, "@@ -5,3 +5,3 @@") {
		t.Fatal("hunk headers should not be rendered")
	}
	if !strings.Contains(body, `<details id="group-1" class="group main-group" data-group-id="first" open>`) {
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
	if got, want := formatRanges(fragment), "-10,2 +10,4; -∅ +40,3; metadata"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
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

func TestSidebarBuildsGroupAndFileCentricNavigation(t *testing.T) {
	inv := model.FragmentSet{Fragments: []model.MaterializedFragment{
		{ID: "F1", Path: "docs/design/a.go", Patch: "@@ -1,1 +1,1 @@\n-old\n+new\n"},
		{ID: "F2", Path: "docs/design/a.go", Patch: "@@ -3,1 +3,1 @@\n-old\n+new\n"},
		{ID: "F3", Path: "docs/design/a.go", Patch: "@@ -5,1 +5,1 @@\n-old\n+new\n"},
		{ID: "F4", Path: "root.go", Patch: "@@ -1,1 +1,1 @@\n-old\n+new\n"},
	}}
	groups := model.GroupsFile{Groups: []model.SemanticGroup{
		{ID: "schema", Title: "Schema", Fragments: []model.Fragment{{ID: "F1", Path: "docs/design/a.go"}, {ID: "F2", Path: "docs/design/a.go"}, {ID: "F4", Path: "root.go"}}},
		{ID: "cleanup", Title: "Cleanup", Fragments: []model.Fragment{{ID: "F3", Path: "docs/design/a.go"}}},
	}}
	page := Build(groups, inv)
	if len(page.SidebarDirectories) != 1 || page.SidebarDirectories[0].Name != "docs/design" || page.SidebarDirectories[0].FileCount != 1 {
		t.Fatalf("unexpected sidebar root directories: %+v", page.SidebarDirectories)
	}
	design := page.SidebarDirectories[0]
	if len(design.Files) != 1 || design.Files[0].Path != "docs/design/a.go" {
		t.Fatalf("nested file tree was not preserved: %+v", design)
	}
	occurrences := design.Files[0].Occurrences
	if len(occurrences) != 2 || occurrences[0].GroupID != "schema" || occurrences[0].FragmentCount != 2 || occurrences[1].GroupID != "cleanup" || occurrences[1].FragmentCount != 1 {
		t.Fatalf("file occurrences were not grouped by semantic group: %+v", occurrences)
	}
	if len(page.SidebarFiles) != 1 || page.SidebarFiles[0].Path != "root.go" {
		t.Fatalf("root-level file missing from sidebar: %+v", page.SidebarFiles)
	}
	handler, err := Handler(page)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
	body := response.Body.String()
	for _, want := range []string{
		`<aside class="sidebar" aria-label="Change navigation">`,
		`class="sidebar-pane sidebar-groups"`,
		`class="sidebar-pane sidebar-files"`,
		`if(sidebarPanes.files)sidebarPanes.files.remove()`,
		`data-group-id="cleanup" data-file-path="docs/design/a.go"`,
		`var mainFiles=Array.from(document.querySelectorAll('.main-file'))`,
		`function buildGroupTrees()`,
		`directory.className='nav-directory nav-group-directory'`,
		`function compressGroupDirectories(root)`,
		`parent.dataset.userCollapsed!=='true'`,
		`.nav-folder-icon{display:none}`,
		`diff.className='nav-file-diff'`,
		`.main-group>summary{position:sticky`,
		`--group-header-height`,
		`--category-header-height`,
		`.category[open]>summary{position:sticky`,
		`data-category-action="open"`,
		`data-category-action="close"`,
		`data-group-action="open"`,
		`data-group-action="close"`,
		`file-review-toggle`,
		`if(file)file.open=false`,
		`function updateCategoryReview(category)`,
		`ResizeObserver`,
		`--sidebar-width:360px`,
		`semdiff-sidebar-width`,
		`sidebar.setPointerCapture(event.pointerId)`,
		`syncSidebar(fileAtViewport(),false)`,
		`openAncestors(activeGroupFile)`,
		`focusInPane(fileTarget,filePane)`,
		`var current=Array.isArray(mainFiles)?fileAtViewport():null`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered sidebar is missing %q", want)
		}
	}
}

func TestFileImportanceUsesStrongestFragment(t *testing.T) {
	page := Build(model.GroupsFile{Groups: []model.SemanticGroup{{
		ID: "g", Title: "Group", Importance: model.ImportanceCore,
		Fragments: []model.Fragment{
			{ID: "F1", Path: "a.go", Importance: model.ImportanceIncidental},
			{ID: "F2", Path: "a.go", Importance: model.ImportanceSupporting},
		},
	}}}, model.FragmentSet{Fragments: []model.MaterializedFragment{{ID: "F1", Path: "a.go"}, {ID: "F2", Path: "a.go"}}})
	if got := page.Groups[0].Files[0].Importance; got != model.ImportanceSupporting {
		t.Fatalf("file importance = %q", got)
	}
	if got := page.SidebarFiles[0].Importance; got != model.ImportanceSupporting {
		t.Fatalf("sidebar file importance = %q", got)
	}
}

func TestHandlerServesImportanceUIAssetsAndData(t *testing.T) {
	page := Build(model.GroupsFile{Groups: []model.SemanticGroup{{
		ID: "behavior", Title: "Behavior", Importance: model.ImportanceCore,
		Fragments: []model.Fragment{{ID: "F1", Path: "a.go", Description: "Adapts the caller.", Importance: model.ImportanceSupporting}},
	}}}, model.FragmentSet{Fragments: []model.MaterializedFragment{{ID: "F1", Path: "a.go", Patch: "@@ -1,1 +1,1 @@\n-old\n+new\n"}}})
	handler, err := Handler(page)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"/":                `<script src="/importance.js"></script>`,
		"/importance.css":  ".importance-core",
		"/importance.js":   "group-importance",
		"/importance.json": `"behavior":"core"`,
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), want) {
			t.Errorf("%s: status=%d body does not contain %q", path, response.Code, want)
		}
	}
}

func TestSidebarKeepsDirectoryBranchesWhileCompressingSingleChains(t *testing.T) {
	files := []SidebarFile{
		{Path: "web/src/entities/node/model/v0.0.6/Node.ts", Name: "Node.ts"},
		{Path: "web/src/entities/node/model/v0.0.6/MediaNode.ts", Name: "MediaNode.ts"},
		{Path: "web/test/node.test.ts", Name: "node.test.ts"},
	}
	directories, rootFiles := buildSidebarTree(files)
	if len(rootFiles) != 0 || len(directories) != 1 || directories[0].Name != "web" {
		t.Fatalf("top-level branch should remain explicit: directories=%+v root=%+v", directories, rootFiles)
	}
	if len(directories[0].Directories) != 2 {
		t.Fatalf("web branch was collapsed across a fork: %+v", directories[0])
	}
	if got := directories[0].Directories[0].Name; got != "src/entities/node/model/v0.0.6" {
		t.Fatalf("single directory chain was not compressed: %q", got)
	}
	if got := directories[0].Directories[1].Name; got != "test" {
		t.Fatalf("sibling directory was lost: %q", got)
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
	files := []FileView{{Path: "a.ts"}, {Path: "b.tsx"}, {Path: "c.yml"}, {Path: "d.go"}, {Path: "e.test.ts"}, {Path: "f.md"}, {Path: "g.ts"}, {Path: "h.bin"}}
	declared := []model.FileCategory{
		{Path: "a.ts", Category: "logic"},
		{Path: "b.tsx", Category: "component"},
		{Path: "c.yml", Category: "config"},
		{Path: "d.go", Category: "implementation"},
		{Path: "e.test.ts", Category: "test"},
		{Path: "f.md", Category: "docs"},
		{Path: "g.ts", Category: "custom"},
		{Path: "h.bin", Category: "unknown"},
	}
	views := buildCategoryViews(files, declared)
	want := []string{"logic", "component", "config", "implementation", "test", "docs", "unknown", "custom"}
	if len(views) != len(want) {
		t.Fatalf("got %d categories, want %d: %+v", len(views), len(want), views)
	}
	for i, category := range views {
		if category.Name != want[i] {
			t.Errorf("category %d = %q, want %q", i, category.Name, want[i])
		}
	}
	if !views[5].Standard || views[5].IconClass != "docs" || views[5].Icon == "" {
		t.Errorf("docs category should have a standard icon: %+v", views[5])
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
	if strings.Contains(html, "@@ -7,2 +7,2 @@") || strings.Contains(html, `class="diff-row hunk"`) {
		t.Fatalf("hunk header should only drive line numbering, not be rendered: %q", html)
	}
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
	if !strings.Contains(gap, "Show 5 lines below") || !strings.Contains(gap, "Show 5 lines above") {
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

func TestMultipleRangesHaveExpandableContextBetweenHunks(t *testing.T) {
	fragment := model.MaterializedFragment{
		ID: "F1", Path: "a.go", NewStart: 10, NewLines: 21,
		Patch: "@@ -10,1 +10,1 @@\n-old one\n+new one\n@@ -30,1 +30,1 @@\n-old two\n+new two\n",
	}
	group := model.GroupsFile{Groups: []model.SemanticGroup{{
		ID: "g", Title: "Group", Fragments: []model.Fragment{{
			ID: "F1", Path: "a.go", Ranges: []model.FragmentRange{
				{Old: &model.Range{Start: 10, Lines: 1}, New: &model.Range{Start: 10, Lines: 1}},
				{Old: &model.Range{Start: 30, Lines: 1}, New: &model.Range{Start: 30, Lines: 1}},
			},
		}},
	}}}
	page := Build(group, model.FragmentSet{Fragments: []model.MaterializedFragment{fragment}}, map[string]string{"a.go": strings.Repeat("line\n", 40)})
	view := page.Groups[0].Files[0].Fragments[0]
	between := string(view.HunkHTML)
	if !strings.Contains(between, "Show 9 lines below") || !strings.Contains(between, "Show 9 lines above") {
		t.Fatalf("multi-range fragment has no bidirectional expansion between ranges: %q", between)
	}
	if strings.Count(between, `class="context-expand context-gap"`) != 1 {
		t.Fatalf("got unexpected number of internal range gaps: %q", between)
	}
	if !strings.Contains(string(view.UpperContextHTML), "Show 4 lines above") || !strings.Contains(string(view.LowerContextHTML), "Show 5 lines below") {
		t.Fatalf("outer fragment context changed: upper=%q lower=%q", view.UpperContextHTML, view.LowerContextHTML)
	}
}

func TestShortRangeGapIsInitiallyVisibleWithoutExpandControls(t *testing.T) {
	html := string(expandableGap(strings.Split("one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine", "\n"), 11, 11))
	if strings.Contains(html, "expand-lines") || strings.Contains(html, "context-hidden") {
		t.Fatalf("a gap covered by five lines of context on each side should be fully visible: %q", html)
	}
	if strings.Count(html, `class="diff-row ctx"`) != 9 {
		t.Fatalf("got unexpected visible context rows: %q", html)
	}
}

func TestRangeGapUsesIndependentOldAndNewLineNumbers(t *testing.T) {
	patch := "@@ -128,0 +129,4 @@\n+one\n+two\n+three\n+four\n@@ -138,1 +142,1 @@\n-old\n+new\n"
	html := string(colorPatchWithContext(patch, sourceLines(strings.Repeat("line\n", 160))))
	oldContext := `<span class="line-number split-cell old-number">137</span>`
	newContext := `<span class="line-number split-cell new-number">141</span>`
	oldChange := `<span class="line-number split-cell old-number">138</span>`
	if !strings.Contains(html, oldContext) || !strings.Contains(html, newContext) {
		t.Fatalf("context did not preserve independent split-view line numbers: %q", html)
	}
	if strings.Index(html, oldContext) > strings.Index(html, oldChange) {
		t.Fatalf("old-side context line numbers run backward at the next range: %q", html)
	}
}
