package viewer

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ry023/semdiff/internal/model"
	"github.com/ry023/semdiff/internal/questions"
)

func diffItemsText(items []DiffItem) string {
	var out strings.Builder
	for _, item := range items {
		if item.Kind == "expand" {
			direction := "below"
			if item.Direction == "up" {
				direction = "above"
			}
			out.WriteString("Show " + strconv.Itoa(item.Count) + " lines " + direction)
			continue
		}
		out.WriteString(item.Text)
	}
	return out.String()
}

func diffItemsClassText(items []DiffItem) string {
	var out strings.Builder
	for _, item := range items {
		out.WriteString(item.Class)
	}
	return out.String()
}

func diffItemsCount(items []DiffItem, kind string) int {
	count := 0
	for _, item := range items {
		if item.Kind == kind {
			count++
		}
	}
	return count
}

func bootstrapFromHTML(t *testing.T, document []byte) viewerBootstrap {
	t.Helper()
	const start = `<script id="semdiff-data" type="application/json">`
	before, data, found := bytes.Cut(document, []byte(start))
	if !found || !bytes.Contains(before, []byte(`<div id="root"></div>`)) {
		t.Fatal("viewer shell or bootstrap data is missing")
	}
	data, _, found = bytes.Cut(data, []byte(`</script>`))
	if !found {
		t.Fatal("bootstrap script is not closed")
	}
	var bootstrap viewerBootstrap
	if err := json.Unmarshal(data, &bootstrap); err != nil {
		t.Fatalf("decode bootstrap data: %v", err)
	}
	return bootstrap
}

func TestQuestionAPIValidatesAnchorsAndReturnsAnswers(t *testing.T) {
	page := Page{BaseSHA: "base", HeadSHA: "head", Groups: []GroupView{{
		ID: "group", Files: []FileView{{Fragments: []FragmentView{{MaterializedFragment: model.MaterializedFragment{ID: "fragment"}}}}},
	}}}
	store := questions.Store{Path: filepath.Join(t.TempDir(), "questions.json"), BaseSHA: "base", HeadSHA: "head"}
	handler, err := HandlerWithQuestions(page, store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Sessions().Start(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/questions", bytes.NewBufferString(`{"anchor":{"type":"fragment","group_id":"group","fragment_id":"fragment"},"question":"Why?"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("POST status = %d: %s", response.Code, response.Body.String())
	}
	var thread questions.Thread
	if err := json.Unmarshal(response.Body.Bytes(), &thread); err != nil {
		t.Fatal(err)
	}
	item, err := store.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Answer(item.ID, "Because."); err != nil {
		t.Fatal(err)
	}
	followUp := httptest.NewRequest(http.MethodPost, "/api/questions", bytes.NewBufferString(`{"thread_id":"`+thread.ID+`","question":"Can you elaborate?"}`))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, followUp)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"turns"`) {
		t.Fatalf("follow-up response: %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/questions", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"question":"Why?"`) || !strings.Contains(response.Body.String(), `"answer":"Because."`) || strings.Contains(response.Body.String(), "_html") {
		t.Fatalf("unexpected GET response: %d %s", response.Code, response.Body.String())
	}

	invalid := httptest.NewRequest(http.MethodPost, "/api/questions", bytes.NewBufferString(`{"anchor":{"type":"group","group_id":"missing"},"question":"Why?"}`))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, invalid)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid anchor status = %d", response.Code)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/questions/session", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"stopped"`) {
		t.Fatalf("stop session response: %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/questions", bytes.NewBufferString(`{"anchor":{"type":"group","group_id":"group"},"question":"Too late?"}`)))
	if response.Code != http.StatusConflict {
		t.Fatalf("inactive answer mode accepted a question: %d %s", response.Code, response.Body.String())
	}
}

func TestExportHTMLIsSelfContainedAndOptionallyIncludesAnsweredTurns(t *testing.T) {
	page := Page{Groups: []GroupView{{ID: "group", Importance: model.ImportanceCore, Files: []FileView{{Fragments: []FragmentView{{MaterializedFragment: model.MaterializedFragment{ID: "fragment"}, ReviewLevel: model.ReviewLevelCareful}}}}}}}
	threads := []questions.Thread{{
		ID: "thread", Anchor: questions.Anchor{Type: "fragment", GroupID: "group", FragmentID: "fragment"},
		Turns: []questions.Turn{
			{ID: "answered", Question: "Why?", Status: questions.StatusAnswered, Answer: "Because."},
			{ID: "pending", Question: "Anything else?", Status: questions.StatusPending},
		},
	}}

	withoutAnswers, err := ExportHTML(page, nil)
	if err != nil {
		t.Fatal(err)
	}
	plain := string(withoutAnswers)
	if !strings.HasPrefix(plain, "<!doctype html>") || !strings.Contains(plain, `</script>`) || !strings.Contains(plain, `<script>`) {
		t.Fatalf("export contains an external asset or data request")
	}
	plainBootstrap := bootstrapFromHTML(t, withoutAnswers)
	if plainBootstrap.Capabilities.Questions != "disabled" || len(plainBootstrap.Threads) != 0 {
		t.Fatal("answers were included without requesting them")
	}

	withAnswers, err := ExportHTML(page, threads)
	if err != nil {
		t.Fatal(err)
	}
	answered := bootstrapFromHTML(t, withAnswers)
	if answered.Capabilities.Questions != "readonly" || len(answered.Threads) != 1 || len(answered.Threads[0].Turns) != 1 || answered.Threads[0].Turns[0].Answer != "Because." {
		t.Fatal("answered turn is missing from export")
	}
	if answered.Threads[0].Turns[0].Question == "Anything else?" {
		t.Fatal("export contains pending or interactive answer-mode content")
	}
}

func TestReactBootstrapEscapesRawTextBoundariesAndUsesBasePath(t *testing.T) {
	page := Page{BaseSHA: `</script><script>alert("x")</script>`, HeadSHA: "head"}
	document, err := renderReactHTML(page, nil, true, "/reviews/example/")
	if err != nil {
		t.Fatal(err)
	}
	bootstrap := bootstrapFromHTML(t, document)
	if bootstrap.Page.BaseSHA != page.BaseSHA {
		t.Fatalf("base SHA changed during embedding: %q", bootstrap.Page.BaseSHA)
	}
	if bootstrap.Capabilities.APIBase != "/reviews/example/api/questions" {
		t.Fatalf("API base = %q", bootstrap.Capabilities.APIBase)
	}
	prefix, _, _ := bytes.Cut(document, []byte(`</script>`))
	if bytes.Contains(prefix, []byte(`<script>alert`)) {
		t.Fatal("bootstrap data escaped its script element")
	}
}

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
	fragment := p.Groups[0].Files[0].Fragments[0]
	upper, lower := diffItemsText(fragment.UpperContext), diffItemsText(fragment.LowerContext)
	if strings.Contains(upper, "Show ") || !strings.Contains(lower, "Show 8 lines below") {
		t.Fatalf("missing directional context controls: upper=%q lower=%q", upper, lower)
	}
	if !strings.Contains(diffItemsText(fragment.Hunk), "unsafe <tag>") {
		t.Fatal("patch text was not preserved for browser escaping")
	}
	h, err := Handler(p)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	bootstrap := bootstrapFromHTML(t, w.Body.Bytes())
	if bootstrap.Capabilities.Questions != "disabled" || bootstrap.Page.BaseSHA != "aaa" || bootstrap.Page.HeadSHA != "bbb" {
		t.Fatalf("unexpected bootstrap: %+v", bootstrap)
	}
	if len(bootstrap.Page.Groups) != 2 || len(bootstrap.Page.Groups[0].Categories) != 1 || bootstrap.Page.Groups[0].Categories[0].Icon != "logic" {
		t.Fatalf("category icon semantics were not preserved: %+v", bootstrap.Page.Groups)
	}
	if strings.Contains(w.Body.String(), "status_icon_html") || strings.Contains(w.Body.String(), "icon_html") {
		t.Fatal("viewer bootstrap still contains rendered icon HTML")
	}
	if strings.Contains(w.Body.String(), "hunk_html") || strings.Contains(w.Body.String(), "description_html") || strings.Contains(w.Body.String(), "summary_html") || !strings.Contains(w.Body.String(), `"hunk"`) {
		t.Fatal("viewer bootstrap should carry structured diff items instead of rendered hunk HTML")
	}
	if got := bootstrap.Page.Groups[0].Files[0].Fragments[0].Description; got != "Explains the <safe> change." {
		t.Fatalf("missing or altered raw fragment description: %q", got)
	}
}

func TestHandlerShowsReviewDriftSeparatelyFromSemanticGroups(t *testing.T) {
	page := Page{
		BaseSHA: "review-base",
		HeadSHA: "review-head",
		Drift: &ReviewDrift{
			CurrentBaseSHA: "review-base",
			CurrentHeadSHA: "current-head",
			Commits:        []model.Commit{{SHA: "1234567890abcdef", Subject: "follow-up", FilesChanged: 2}},
			Paths:          []string{"a.go", "docs/readme.md"},
		},
	}
	h, err := Handler(page)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	bootstrap := bootstrapFromHTML(t, w.Body.Bytes())
	drift := bootstrap.Page.Drift
	if drift == nil || drift.CurrentHeadSHA != "current-head" || len(drift.Commits) != 1 || drift.Commits[0].Subject != "follow-up" || len(drift.Paths) != 2 {
		t.Fatalf("unexpected drift bootstrap: %+v", drift)
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
		name, patch, status  string
		additions, deletions int
	}{
		{"new", "diff --git a/new.go b/new.go\nnew file mode 100644\n--- /dev/null\n+++ b/new.go\n@@ -0,0 +1,2 @@\n+one\n+two\n", "new", 2, 0},
		{"updated", "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-old\n+new\n", "updated", 1, 1},
		{"deleted", "diff --git a/old.go b/old.go\ndeleted file mode 100644\n--- a/old.go\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-one\n-two\n", "deleted", 0, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fragment := model.MaterializedFragment{ID: "F1", Path: "a.go", Patch: tt.patch}
			file := buildFileView("a.go", []model.MaterializedFragment{fragment}, "", []model.MaterializedFragment{fragment})
			if file.Status != tt.status || file.Additions != tt.additions || file.Deletions != tt.deletions {
				t.Fatalf("unexpected file metadata: %+v", file)
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
}

func TestFileReviewLevelUsesMostCarefulFragment(t *testing.T) {
	page := Build(model.GroupsFile{Groups: []model.SemanticGroup{{
		ID: "g", Title: "Group", Importance: model.ImportanceCore,
		Fragments: []model.Fragment{
			{ID: "F1", Path: "a.go", ReviewLevel: model.ReviewLevelSkim},
			{ID: "F2", Path: "a.go", ReviewLevel: model.ReviewLevelNormal},
		},
	}}}, model.FragmentSet{Fragments: []model.MaterializedFragment{{ID: "F1", Path: "a.go"}, {ID: "F2", Path: "a.go"}}})
	if got := page.Groups[0].Files[0].ReviewLevel; got != model.ReviewLevelNormal {
		t.Fatalf("file review level = %q", got)
	}
	if got := page.SidebarFiles[0].ReviewLevel; got != model.ReviewLevelNormal {
		t.Fatalf("sidebar file review level = %q", got)
	}
}

func TestHandlerEmbedsViewerAssetsAndData(t *testing.T) {
	page := Build(model.GroupsFile{Groups: []model.SemanticGroup{{
		ID: "behavior", Title: "Behavior", Importance: model.ImportanceCore,
		Fragments: []model.Fragment{{ID: "F1", Path: "a.go", Description: "Adapts the caller.", ReviewLevel: model.ReviewLevelCareful}},
	}}}, model.FragmentSet{Fragments: []model.MaterializedFragment{{ID: "F1", Path: "a.go", Patch: "@@ -1,1 +1,1 @@\n-old\n+new\n"}}})
	handler, err := Handler(page)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `<style>`) || !strings.Contains(response.Body.String(), `<script>`) {
		t.Fatalf("viewer assets are not inline: status=%d", response.Code)
	}
	bootstrap := bootstrapFromHTML(t, response.Body.Bytes())
	fragment := bootstrap.Page.Groups[0].Files[0].Fragments[0]
	if bootstrap.Page.Groups[0].Importance != model.ImportanceCore || fragment.ReviewLevel != model.ReviewLevelCareful {
		t.Fatalf("importance data is missing: %+v", bootstrap.Page.Groups[0])
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

func TestDiffItemsDoNotAddBlankRows(t *testing.T) {
	items := diffItems("+one\n+two\n")
	if len(items) != 2 {
		t.Fatalf("got %d diff items, want 2: %+v", len(items), items)
	}
	if got := diffItemsText(items); got != "+one+two" {
		t.Fatalf("diff items changed patch text: %q", got)
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
	if !views[5].Standard || views[5].Icon != "docs" {
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

func TestDiffItemsLineNumbers(t *testing.T) {
	items := diffItems("@@ -7,2 +7,2 @@\n-old\n+new\n context\n")
	if strings.Contains(diffItemsText(items), "@@ -7,2 +7,2 @@") {
		t.Fatalf("hunk header should only drive line numbering, not be rendered: %+v", items)
	}
	if len(items) != 3 || items[0].OldNumber != "7" || items[1].NewNumber != "7" || items[2].OldNumber != "8" || items[2].NewNumber != "8" {
		t.Fatalf("unexpected line numbering: %+v", items)
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
	gap := diffItemsText(fragments[0].LowerContext)
	if !strings.Contains(gap, "Show 5 lines below") || !strings.Contains(gap, "Show 5 lines above") {
		t.Fatalf("fragments do not share a bidirectional context gap: %q", gap)
	}
	if len(fragments[1].UpperContext) != 0 {
		t.Fatalf("second fragment duplicated the shared context gap: %+v", fragments[1].UpperContext)
	}
}

func TestContextExpansionShowsOtherFragments(t *testing.T) {
	first := model.MaterializedFragment{ID: "F1", Path: "a.go", NewStart: 5, NewLines: 1, Patch: "@@ -5,1 +5,1 @@\n-old\n+first\n"}
	other := model.MaterializedFragment{ID: "F2", Path: "a.go", NewStart: 12, NewLines: 2, Patch: "@@ -12,2 +12,2 @@\n-old\n+other\n"}
	last := model.MaterializedFragment{ID: "F3", Path: "a.go", NewStart: 20, NewLines: 1, Patch: "@@ -20,1 +20,1 @@\n-old\n+last\n"}
	content := strings.Repeat("line\n", 30)
	file := buildFileView("a.go", []model.MaterializedFragment{first, last}, content, []model.MaterializedFragment{first, other, last})
	between := file.Fragments[0].LowerContext
	if !strings.Contains(diffItemsClassText(between), "ctx other-change") {
		t.Fatalf("context should mark other fragment lines: %+v", between)
	}
	if !strings.Contains(diffItemsText(between), "Show 4 lines below") {
		t.Fatalf("context should remain expandable across other fragment lines: %q", diffItemsText(between))
	}

	guided := buildFragmentView(first, content, nil, []model.MaterializedFragment{first, other, last})
	if !strings.Contains(diffItemsClassText(guided.LowerContext), "ctx other-change") {
		t.Fatalf("guided context should extend through other fragments: %+v", guided.LowerContext)
	}
}

func TestBuildPreservesReviewStepFragmentOrderAcrossFiles(t *testing.T) {
	order := 1
	group := model.GroupsFile{Groups: []model.SemanticGroup{{
		ID: "g", Title: "Guided", Order: &order, ReviewSteps: []model.ReviewStep{{ID: "first", Title: "Start", Summary: "Establish the prerequisite.", FragmentIDs: []string{"F2", "F1"}}},
		Fragments: []model.Fragment{{ID: "F1", Path: "a.go", Description: "First file."}, {ID: "F2", Path: "b.go", Description: "Second file."}},
	}}}
	page := Build(group, model.FragmentSet{Fragments: []model.MaterializedFragment{{ID: "F1", Path: "a.go", Patch: "patch"}, {ID: "F2", Path: "b.go", Patch: "patch"}}})
	step := page.Groups[0].Steps[0]
	if len(step.Fragments) != 2 || step.Fragments[0].ID != "F2" || step.Fragments[1].ID != "F1" {
		t.Fatalf("guided order = %+v", step.Fragments)
	}
	if step.Number != 1 {
		t.Fatalf("step number = %d, want 1", step.Number)
	}
	if page.Groups[0].Files[0].Path != "a.go" || page.Groups[0].Files[1].Path != "b.go" {
		t.Fatalf("files are no longer file ordered: %+v", page.Groups[0].Files)
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
	between := view.Hunk
	if !strings.Contains(diffItemsText(between), "Show 9 lines below") || !strings.Contains(diffItemsText(between), "Show 9 lines above") {
		t.Fatalf("multi-range fragment has no bidirectional expansion between ranges: %+v", between)
	}
	if diffItemsCount(between, "expand") != 2 {
		t.Fatalf("got unexpected number of internal range controls: %+v", between)
	}
	if !strings.Contains(diffItemsText(view.UpperContext), "Show 4 lines above") || !strings.Contains(diffItemsText(view.LowerContext), "Show 5 lines below") {
		t.Fatalf("outer fragment context changed: upper=%q lower=%q", diffItemsText(view.UpperContext), diffItemsText(view.LowerContext))
	}
}

func TestShortRangeGapIsInitiallyVisibleWithoutExpandControls(t *testing.T) {
	items := expandableGap(strings.Split("one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine", "\n"), 11, 11, nil)
	if diffItemsCount(items, "expand") != 0 || hasHiddenDiffItem(items) {
		t.Fatalf("a gap covered by five lines of context on each side should be fully visible: %+v", items)
	}
	if len(items) != 9 {
		t.Fatalf("got unexpected visible context rows: %+v", items)
	}
}

func TestRangeGapUsesIndependentOldAndNewLineNumbers(t *testing.T) {
	patch := "@@ -128,0 +129,4 @@\n+one\n+two\n+three\n+four\n@@ -138,1 +142,1 @@\n-old\n+new\n"
	items := diffItemsWithContext(patch, sourceLines(strings.Repeat("line\n", 160)))
	oldContext, newContext := false, false
	for _, item := range items {
		if item.OldNumber == "137" && item.NewNumber == "141" {
			oldContext, newContext = true, true
		}
	}
	if !oldContext || !newContext {
		t.Fatalf("context did not preserve independent split-view line numbers: %+v", items)
	}
	contextIndex, changeIndex := -1, -1
	for index, item := range items {
		if item.OldNumber == "137" {
			contextIndex = index
		}
		if item.OldNumber == "138" {
			changeIndex = index
		}
	}
	if contextIndex > changeIndex {
		t.Fatalf("old-side context line numbers run backward at the next range: %+v", items)
	}
}

func hasHiddenDiffItem(items []DiffItem) bool {
	for _, item := range items {
		if item.Hidden {
			return true
		}
	}
	return false
}
