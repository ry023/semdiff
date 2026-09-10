package viewer

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	pathpkg "path"
	"sort"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	categorydraft "github.com/ry023/semdiff/internal/categories"
	"github.com/ry023/semdiff/internal/model"
	"github.com/ry023/semdiff/internal/questions"
	"github.com/yuin/goldmark"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
)

//go:embed dist/viewer.css dist/viewer.js
var assets embed.FS

const defaultContextLines = 5

type FragmentView struct {
	model.MaterializedFragment
	Description      string            `json:"description"`
	DescriptionHTML  template.HTML     `json:"description_html"`
	ReviewLevel      model.ReviewLevel `json:"review_level"`
	RangeLabel       string            `json:"range_label"`
	Directory        string            `json:"directory"`
	Name             string            `json:"name"`
	Status           string            `json:"status"`
	Additions        int               `json:"additions"`
	Deletions        int               `json:"deletions"`
	Diffstat         []string          `json:"diffstat"`
	HeaderHTML       template.HTML     `json:"header_html"`
	HunkHTML         template.HTML     `json:"hunk_html"`
	UpperContextHTML template.HTML     `json:"upper_context_html"`
	LowerContextHTML template.HTML     `json:"lower_context_html"`
}

type syntaxHighlighter struct {
	lexer chroma.Lexer
	lines map[int]template.HTML
}
type FileView struct {
	Path        string            `json:"path"`
	AnchorID    string            `json:"anchor_id"`
	Directory   string            `json:"directory"`
	Name        string            `json:"name"`
	Status      string            `json:"status"`
	Additions   int               `json:"additions"`
	Deletions   int               `json:"deletions"`
	Diffstat    []string          `json:"diffstat"`
	HeaderHTML  template.HTML     `json:"header_html"`
	Fragments   []FragmentView    `json:"fragments"`
	ReviewLevel model.ReviewLevel `json:"review_level"`
}
type GroupView struct {
	ID            string           `json:"id"`
	Title         string           `json:"title"`
	Summary       string           `json:"summary"`
	Importance    model.Importance `json:"importance"`
	AnchorID      string           `json:"anchor_id"`
	SummaryHTML   template.HTML    `json:"summary_html"`
	Order         *int             `json:"order,omitempty"`
	Files         []FileView       `json:"files"`
	Categories    []CategoryView   `json:"categories"`
	Steps         []ReviewStepView `json:"steps"`
	FragmentCount int              `json:"fragment_count"`
}

type ReviewStepView struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Summary     string         `json:"summary"`
	SummaryHTML template.HTML  `json:"summary_html"`
	AnchorID    string         `json:"anchor_id"`
	Number      int            `json:"number"`
	Fragments   []FragmentView `json:"fragments"`
}
type CategoryView struct {
	Name     string     `json:"name"`
	Icon     string     `json:"icon"`
	Standard bool       `json:"standard"`
	Files    []FileView `json:"files"`
	Added    int        `json:"added"`
	Updated  int        `json:"updated"`
	Deleted  int        `json:"deleted"`
}

type ReviewDrift struct {
	CurrentBaseSHA string         `json:"current_base_sha"`
	CurrentHeadSHA string         `json:"current_head_sha"`
	Commits        []model.Commit `json:"commits"`
	Paths          []string       `json:"paths"`
}

type Page struct {
	BaseSHA            string             `json:"base_sha"`
	HeadSHA            string             `json:"head_sha"`
	Drift              *ReviewDrift       `json:"drift,omitempty"`
	Groups             []GroupView        `json:"groups"`
	SidebarDirectories []SidebarDirectory `json:"sidebar_directories"`
	SidebarFiles       []SidebarFile      `json:"sidebar_files"`
	FragmentCount      int                `json:"fragment_count"`
	FileCount          int                `json:"file_count"`
}

type SidebarOccurrence struct {
	GroupID       string            `json:"group_id"`
	GroupTitle    string            `json:"group_title"`
	FileAnchorID  string            `json:"file_anchor_id"`
	FragmentCount int               `json:"fragment_count"`
	ReviewLevel   model.ReviewLevel `json:"review_level"`
}

type SidebarFile struct {
	Path        string              `json:"path"`
	Name        string              `json:"name"`
	Status      string              `json:"status"`
	ReviewLevel model.ReviewLevel   `json:"review_level"`
	Occurrences []SidebarOccurrence `json:"occurrences"`
}

type SidebarDirectory struct {
	Name        string             `json:"name"`
	FileCount   int                `json:"file_count"`
	Directories []SidebarDirectory `json:"directories"`
	Files       []SidebarFile      `json:"files"`
}

func Build(g model.GroupsFile, inv model.FragmentSet, contents ...map[string]string) Page {
	var fileContents map[string]string
	if len(contents) > 0 {
		fileContents = contents[0]
	}
	byID := map[string]model.MaterializedFragment{}
	byPath := map[string][]model.MaterializedFragment{}
	for _, f := range inv.Fragments {
		byID[f.ID] = f
		byPath[f.Path] = append(byPath[f.Path], f)
	}
	for path := range byPath {
		sort.SliceStable(byPath[path], func(i, j int) bool {
			return fragmentStart(byPath[path][i]) < fragmentStart(byPath[path][j])
		})
	}
	highlighters := map[string]*syntaxHighlighter{}
	highlighterFor := func(path string) *syntaxHighlighter {
		if highlighter, ok := highlighters[path]; ok {
			return highlighter
		}
		highlighter := newSyntaxHighlighter(path, fileContents[path])
		highlighters[path] = highlighter
		return highlighter
	}
	p := Page{BaseSHA: g.BaseSHA, HeadSHA: g.HeadSHA}
	allFiles := map[string]bool{}
	for groupIndex, group := range g.Groups {
		gv := GroupView{ID: group.ID, Title: group.Title, Summary: group.Summary, Importance: group.Importance, SummaryHTML: renderMarkdown(group.Summary), Order: group.Order, AnchorID: fmt.Sprintf("group-%d", groupIndex)}
		fileMap := map[string][]model.MaterializedFragment{}
		descriptions := map[string]string{}
		rangeLabels := map[string]string{}
		reviewLevels := map[string]model.ReviewLevel{}
		var paths []string
		for _, reference := range group.Fragments {
			id := reference.ID
			f := byID[id]
			if _, ok := fileMap[f.Path]; !ok {
				paths = append(paths, f.Path)
			}
			fileMap[f.Path] = append(fileMap[f.Path], f)
			descriptions[id] = reference.Description
			rangeLabels[id] = formatRanges(reference)
			reviewLevels[id] = reference.ReviewLevel
			allFiles[f.Path] = true
			gv.FragmentCount++
		}
		sort.Strings(paths)
		for fileIndex, path := range paths {
			file := buildFileView(path, fileMap[path], fileContents[path], byPath[path], highlighterFor(path))
			file.AnchorID = fmt.Sprintf("file-%d-%d", groupIndex, fileIndex)
			for i := range file.Fragments {
				file.Fragments[i].Description = descriptions[file.Fragments[i].ID]
				file.Fragments[i].DescriptionHTML = renderInlineMarkdown(file.Fragments[i].Description)
				file.Fragments[i].RangeLabel = rangeLabels[file.Fragments[i].ID]
				file.Fragments[i].ReviewLevel = reviewLevels[file.Fragments[i].ID]
				file.ReviewLevel = strongerReviewLevel(file.ReviewLevel, file.Fragments[i].ReviewLevel)
			}
			gv.Files = append(gv.Files, file)
		}
		for stepIndex, step := range group.ReviewSteps {
			sv := ReviewStepView{ID: step.ID, Title: step.Title, Summary: step.Summary, SummaryHTML: renderMarkdown(step.Summary), AnchorID: fmt.Sprintf("step-%d-%d", groupIndex, stepIndex), Number: stepIndex + 1}
			for _, id := range step.FragmentIDs {
				fragment := byID[id]
				view := buildFragmentView(fragment, fileContents[fragment.Path], nil, byPath[fragment.Path], highlighterFor(fragment.Path))
				file := buildFileView(fragment.Path, []model.MaterializedFragment{fragment}, fileContents[fragment.Path], byPath[fragment.Path], highlighterFor(fragment.Path))
				view.Description = descriptions[id]
				view.DescriptionHTML = renderInlineMarkdown(view.Description)
				view.RangeLabel = rangeLabels[id]
				view.ReviewLevel = reviewLevels[id]
				view.Directory = file.Directory
				view.Name = file.Name
				view.Status = file.Status
				view.Additions = file.Additions
				view.Deletions = file.Deletions
				view.Diffstat = file.Diffstat
				sv.Fragments = append(sv.Fragments, view)
			}
			gv.Steps = append(gv.Steps, sv)
		}
		gv.Categories = buildCategoryViews(gv.Files, group.FileCategories)
		p.Groups = append(p.Groups, gv)
		p.FragmentCount += gv.FragmentCount
	}
	sort.SliceStable(p.Groups, func(i, j int) bool {
		if p.Groups[i].Order == nil {
			return false
		}
		if p.Groups[j].Order == nil {
			return true
		}
		return *p.Groups[i].Order < *p.Groups[j].Order
	})
	p.FileCount = len(allFiles)
	p.buildSidebar()
	return p
}

type sidebarDirectoryBuilder struct {
	name        string
	directories map[string]*sidebarDirectoryBuilder
	files       []SidebarFile
}

func (p *Page) buildSidebar() {
	byPath := map[string]*SidebarFile{}
	for _, group := range p.Groups {
		for _, file := range group.Files {
			sidebarFile := byPath[file.Path]
			if sidebarFile == nil {
				sidebarFile = &SidebarFile{Path: file.Path, Name: file.Name, Status: file.Status}
				byPath[file.Path] = sidebarFile
			}
			sidebarFile.Occurrences = append(sidebarFile.Occurrences, SidebarOccurrence{
				GroupID: group.ID, GroupTitle: group.Title,
				FileAnchorID: file.AnchorID, FragmentCount: len(file.Fragments), ReviewLevel: file.ReviewLevel,
			})
			sidebarFile.ReviewLevel = strongerReviewLevel(sidebarFile.ReviewLevel, file.ReviewLevel)
		}
	}
	files := make([]SidebarFile, 0, len(byPath))
	for _, file := range byPath {
		files = append(files, *file)
	}
	p.SidebarDirectories, p.SidebarFiles = buildSidebarTree(files)
}

func strongerReviewLevel(left, right model.ReviewLevel) model.ReviewLevel {
	rank := map[model.ReviewLevel]int{model.ReviewLevelSkim: 1, model.ReviewLevelNormal: 2, model.ReviewLevelCareful: 3}
	if rank[right] > rank[left] {
		return right
	}
	return left
}

func buildSidebarTree(files []SidebarFile) ([]SidebarDirectory, []SidebarFile) {
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	root := &sidebarDirectoryBuilder{directories: map[string]*sidebarDirectoryBuilder{}}
	for _, file := range files {
		parts := strings.Split(file.Path, "/")
		directory := root
		for _, part := range parts[:len(parts)-1] {
			child := directory.directories[part]
			if child == nil {
				child = &sidebarDirectoryBuilder{name: part, directories: map[string]*sidebarDirectoryBuilder{}}
				directory.directories[part] = child
			}
			directory = child
		}
		directory.files = append(directory.files, file)
	}
	return materializeSidebarDirectory(root)
}

func materializeSidebarDirectory(builder *sidebarDirectoryBuilder) ([]SidebarDirectory, []SidebarFile) {
	names := make([]string, 0, len(builder.directories))
	for name := range builder.directories {
		names = append(names, name)
	}
	sort.Strings(names)
	directories := make([]SidebarDirectory, 0, len(names))
	for _, name := range names {
		child := builder.directories[name]
		grandchildren, files := materializeSidebarDirectory(child)
		directory := SidebarDirectory{Name: child.name, Directories: grandchildren, Files: files, FileCount: len(files)}
		if len(files) == 0 && len(grandchildren) == 1 {
			grandchild := grandchildren[0]
			directory.Name += "/" + grandchild.Name
			directory.Directories = grandchild.Directories
			directory.Files = grandchild.Files
			directory.FileCount = grandchild.FileCount
			directories = append(directories, directory)
			continue
		}
		for _, grandchild := range grandchildren {
			directory.FileCount += grandchild.FileCount
		}
		directories = append(directories, directory)
	}
	return directories, builder.files
}

func formatRanges(fragment model.Fragment) string {
	var parts []string
	for _, span := range fragment.Ranges {
		oldSide, newSide := "∅", "∅"
		if span.Old != nil {
			oldSide = strconv.Itoa(span.Old.Start) + "," + strconv.Itoa(span.Old.Lines)
		}
		if span.New != nil {
			newSide = strconv.Itoa(span.New.Start) + "," + strconv.Itoa(span.New.Lines)
		}
		parts = append(parts, "-"+oldSide+" +"+newSide)
	}
	if fragment.FileMetadata {
		parts = append(parts, "metadata")
	}
	return strings.Join(parts, "; ")
}

func buildCategoryViews(files []FileView, declared []model.FileCategory) []CategoryView {
	declaredByPath := make(map[string]string, len(declared))
	for _, fileCategory := range declared {
		if path := strings.TrimSpace(fileCategory.Path); path != "" {
			declaredByPath[path] = canonicalCategory(fileCategory.Category)
		}
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	drafts := categorydraft.ClassifyPaths(paths)
	draftByPath := make(map[string]string, len(drafts))
	for _, draft := range drafts {
		draftByPath[draft.Path] = draft.Category
	}
	byName := make(map[string][]FileView)
	for _, file := range files {
		name := declaredByPath[file.Path]
		if name == "" {
			name = draftByPath[file.Path]
		}
		if name == "" {
			name = "unknown"
		}
		byName[name] = append(byName[name], file)
	}
	categoryNames := make([]string, 0, len(byName))
	for name := range byName {
		categoryNames = append(categoryNames, name)
	}
	sort.Slice(categoryNames, func(i, j int) bool {
		left, leftOK := categoryRank(categoryNames[i])
		right, rightOK := categoryRank(categoryNames[j])
		if leftOK && rightOK {
			return left < right
		}
		if leftOK != rightOK {
			return leftOK
		}
		return strings.ToLower(categoryNames[i]) < strings.ToLower(categoryNames[j])
	})
	result := make([]CategoryView, 0, len(categoryNames))
	for _, name := range categoryNames {
		icon, standard := categoryIconName(name)
		category := CategoryView{Name: name, Icon: icon, Standard: standard, Files: byName[name]}
		for _, file := range category.Files {
			switch file.Status {
			case "new":
				category.Added++
			case "deleted":
				category.Deleted++
			default:
				category.Updated++
			}
		}
		result = append(result, category)
	}
	return result
}

func categoryRank(name string) (int, bool) {
	for i, standard := range []string{"logic", "component", "config", "implementation", "test", "docs", "unknown"} {
		if strings.EqualFold(name, standard) {
			return i, true
		}
	}
	return 0, false
}

func canonicalCategory(name string) string {
	trimmed := strings.TrimSpace(name)
	for _, standard := range []string{"logic", "component", "config", "implementation", "test", "docs", "unknown"} {
		if strings.EqualFold(trimmed, standard) {
			return standard
		}
	}
	return trimmed
}

func categoryIconName(name string) (string, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "implementation", "test", "component", "logic", "config", "docs", "unknown":
		return name, true
	default:
		return "custom", false
	}
}

func renderMarkdown(source string) template.HTML {
	var rendered bytes.Buffer
	markdown := goldmark.New(goldmark.WithRendererOptions(goldmarkhtml.WithHardWraps()))
	if err := markdown.Convert([]byte(source), &rendered); err != nil {
		return template.HTML(template.HTMLEscapeString(source))
	}
	return template.HTML(rendered.String())
}

func renderInlineMarkdown(source string) template.HTML {
	// Fragment descriptions are inline prose. Escape raw HTML before parsing so
	// angle-bracketed identifiers remain visible instead of becoming HTML nodes.
	rendered := string(renderMarkdown(template.HTMLEscapeString(source)))
	if strings.HasPrefix(rendered, "<p>") && strings.HasSuffix(rendered, "</p>\n") {
		rendered = strings.TrimSuffix(strings.TrimPrefix(rendered, "<p>"), "</p>\n")
	}
	return template.HTML(rendered)
}

func fragmentStart(f model.MaterializedFragment) int {
	if f.NewLines > 0 {
		return f.NewStart
	}
	return f.OldStart
}

func fragmentLines(f model.MaterializedFragment) int {
	if f.NewLines > 0 {
		return f.NewLines
	}
	return f.OldLines
}

func splitPatch(patch string) (string, string) {
	lines := strings.Split(strings.TrimSuffix(patch, "\n"), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "@@ ") {
			return strings.Join(lines[:i], "\n") + "\n", strings.Join(lines[i:], "\n") + "\n"
		}
	}
	return patch, ""
}

func sourceLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func buildFragmentView(f model.MaterializedFragment, content string, boundaries, highlights []model.MaterializedFragment, highlighter *syntaxHighlighter) FragmentView {
	header, hunk := splitPatch(f.Patch)
	lines := sourceLines(content)
	view := FragmentView{MaterializedFragment: f, HeaderHTML: colorPatch(header, highlighter), HunkHTML: colorPatchWithContext(hunk, lines, highlighter)}
	start := fragmentStart(f) - 1
	if start < 0 || start > len(lines) {
		return view
	}
	end := min(len(lines), start+fragmentLines(f))
	upperStart, lowerEnd := 0, len(lines)
	for i, sibling := range boundaries {
		if sibling.ID != f.ID {
			continue
		}
		if i > 0 {
			previous := boundaries[i-1]
			upperStart = min(start, fragmentStart(previous)-1+fragmentLines(previous))
		}
		if i+1 < len(boundaries) {
			lowerEnd = max(end, fragmentStart(boundaries[i+1])-1)
		}
		break
	}
	upperStart = max(0, min(upperStart, start))
	lowerEnd = max(end, min(lowerEnd, len(lines)))
	upperOldFirst, lowerOldFirst := upperStart+1, end+1
	blocks := splitHunkBlocks(hunk)
	if len(blocks) > 0 {
		first := parseHunkBounds(blocks[0])
		last := parseHunkBounds(blocks[len(blocks)-1])
		upperOldFirst = max(1, first.oldStart-(start-upperStart)+1)
		lowerOldFirst = last.oldEnd + 1
	}
	view.UpperContextHTML = expandableContext(lines[upperStart:start], upperOldFirst, upperStart+1, "up", highlights, highlighter)
	view.LowerContextHTML = expandableContext(lines[end:lowerEnd], lowerOldFirst, end+1, "down", highlights, highlighter)
	return view
}

// colorPatchWithContext keeps each materialized hunk as an independently
// expandable range. A multi-range fragment has one outer set of controls, but
// without controls between its hunks the source lines in those gaps can never
// be revealed.
func colorPatchWithContext(patch string, lines []string, highlighter *syntaxHighlighter) template.HTML {
	blocks := splitHunkBlocks(patch)
	if len(blocks) < 2 || len(lines) == 0 {
		return colorPatch(patch, highlighter)
	}
	var out strings.Builder
	for i, block := range blocks {
		if i > 0 {
			previous := parseHunkBounds(blocks[i-1])
			current := parseHunkBounds(block)
			previousEnd := previous.newEnd
			currentStart := current.newStart
			previousEnd = max(0, min(previousEnd, len(lines)))
			currentStart = max(previousEnd, min(currentStart, len(lines)))
			gapLines := lines[previousEnd:currentStart]
			oldFirst := max(1, current.oldStart-len(gapLines)+1)
			out.WriteString(string(expandableGap(gapLines, oldFirst, previousEnd+1, nil, highlighter)))
		}
		out.WriteString(string(colorPatch(block, highlighter)))
	}
	return template.HTML(out.String())
}

func splitHunkBlocks(patch string) []string {
	trimmed := strings.TrimSuffix(patch, "\n")
	if trimmed == "" {
		return nil
	}
	var blocks []string
	var current []string
	for _, line := range strings.Split(trimmed, "\n") {
		if strings.HasPrefix(line, "@@ ") && len(current) > 0 {
			blocks = append(blocks, strings.Join(current, "\n")+"\n")
			current = nil
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		blocks = append(blocks, strings.Join(current, "\n")+"\n")
	}
	return blocks
}

type hunkBounds struct {
	oldStart int
	oldEnd   int
	newStart int
	newEnd   int
}

func parseHunkBounds(hunk string) hunkBounds {
	header := strings.SplitN(hunk, "\n", 2)[0]
	fields := strings.Fields(header)
	if len(fields) < 3 {
		return hunkBounds{}
	}
	oldStart, oldCount := rangeBounds(fields[1])
	newStart, newCount := rangeBounds(fields[2])
	oldStart = max(0, oldStart-1)
	newStart = max(0, newStart-1)
	return hunkBounds{oldStart: oldStart, oldEnd: oldStart + oldCount, newStart: newStart, newEnd: newStart + newCount}
}

func rangeBounds(field string) (int, int) {
	field = strings.TrimLeft(field, "+-")
	parts := strings.SplitN(field, ",", 2)
	start, _ := strconv.Atoi(parts[0])
	count := 1
	if len(parts) == 2 {
		count, _ = strconv.Atoi(parts[1])
	}
	return start, count
}

func buildFileView(path string, fragments []model.MaterializedFragment, content string, siblings []model.MaterializedFragment, highlighter *syntaxHighlighter) FileView {
	sort.SliceStable(fragments, func(i, j int) bool {
		return fragmentStart(fragments[i]) < fragmentStart(fragments[j])
	})
	directory, name := pathpkg.Dir(path), pathpkg.Base(path)
	if directory == "." {
		directory = ""
	} else {
		directory += "/"
	}
	file := FileView{Path: path, Directory: directory, Name: name, Status: "updated"}
	for _, fragment := range siblings {
		if strings.Contains(fragment.Patch, "new file mode ") || strings.Contains(fragment.Patch, "--- /dev/null") {
			file.Status = "new"
		}
		if strings.Contains(fragment.Patch, "deleted file mode ") || strings.Contains(fragment.Patch, "+++ /dev/null") {
			file.Status = "deleted"
		}
		for _, line := range strings.Split(fragment.Patch, "\n") {
			switch {
			case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
				file.Additions++
			case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
				file.Deletions++
			}
		}
	}
	file.Diffstat = diffstatBlocks(file.Additions, file.Deletions)
	for _, fragment := range fragments {
		file.Fragments = append(file.Fragments, buildFragmentView(fragment, content, fragments, siblings, highlighter))
	}
	if len(file.Fragments) == 0 {
		return file
	}
	file.HeaderHTML = file.Fragments[0].HeaderHTML
	for i := range file.Fragments {
		file.Fragments[i].HeaderHTML = ""
	}
	lines := sourceLines(content)
	for i := 1; i < len(file.Fragments); i++ {
		previous := &file.Fragments[i-1]
		current := &file.Fragments[i]
		start := fragmentStart(previous.MaterializedFragment) - 1 + fragmentLines(previous.MaterializedFragment)
		end := fragmentStart(current.MaterializedFragment) - 1
		start = max(0, min(start, len(lines)))
		end = max(start, min(end, len(lines)))
		gapLines := lines[start:end]
		oldFirst := start + 1
		_, currentHunk := splitPatch(current.Patch)
		currentBlocks := splitHunkBlocks(currentHunk)
		if len(currentBlocks) > 0 {
			bounds := parseHunkBounds(currentBlocks[0])
			oldFirst = max(1, bounds.oldStart-len(gapLines)+1)
		}
		previous.LowerContextHTML = expandableGap(gapLines, oldFirst, start+1, siblings, highlighter)
		current.UpperContextHTML = ""
	}
	return file
}

func diffstatBlocks(additions, deletions int) []string {
	const blockCount = 5
	blocks := make([]string, 0, blockCount)
	total := additions + deletions
	if total == 0 {
		for len(blocks) < blockCount {
			blocks = append(blocks, "neutral")
		}
		return blocks
	}
	addedBlocks := additions * blockCount / total
	deletedBlocks := deletions * blockCount / total
	for range addedBlocks {
		blocks = append(blocks, "added")
	}
	for range deletedBlocks {
		blocks = append(blocks, "deleted")
	}
	for len(blocks) < blockCount {
		blocks = append(blocks, "neutral")
	}
	return blocks
}

func appendDiffRow(out *strings.Builder, line, class, oldNumber, newNumber string, hidden bool, highlighter *syntaxHighlighter) {
	out.WriteString(`<span class="diff-row ` + class)
	if hidden {
		out.WriteString(` context-hidden" hidden>`)
	} else {
		out.WriteString(`">`)
	}
	unifiedNumber := newNumber
	if unifiedNumber == "" {
		unifiedNumber = oldNumber
	}
	escaped := highlighter.highlight(line, class, newNumber)
	out.WriteString(`<span class="line-number unified-cell">` + unifiedNumber + `</span><span class="line-code unified-cell">`)
	out.WriteString(string(escaped))
	out.WriteString(`</span>`)
	if oldNumber == "" && newNumber == "" {
		out.WriteString(`<span class="split-wide">` + string(escaped) + `</span>`)
	} else {
		oldCode, newCode := template.HTML(""), template.HTML("")
		switch class {
		case "add":
			newCode = escaped
		case "del":
			oldCode = escaped
		default:
			oldCode, newCode = escaped, escaped
		}
		out.WriteString(`<span class="line-number split-cell old-number">` + oldNumber + `</span><span class="line-code split-cell old-code">` + string(oldCode) + `</span>`)
		out.WriteString(`<span class="line-number split-cell new-number">` + newNumber + `</span><span class="line-code split-cell new-code">` + string(newCode) + `</span>`)
	}
	out.WriteString(`</span>`)
}

func newSyntaxHighlighter(path, source string) *syntaxHighlighter {
	highlighter := &syntaxHighlighter{lexer: lexers.Match(path), lines: map[int]template.HTML{}}
	if highlighter.lexer == nil || source == "" {
		return highlighter
	}
	iterator, err := highlighter.lexer.Tokenise(nil, source)
	if err != nil {
		return highlighter
	}
	line := 1
	var out strings.Builder
	for token := iterator(); token != chroma.EOF; token = iterator() {
		for i, part := range strings.Split(token.Value, "\n") {
			if i > 0 {
				highlighter.lines[line] = template.HTML(out.String())
				out.Reset()
				line++
			}
			out.WriteString(highlightedToken(token.Type, part))
		}
	}
	if out.Len() > 0 {
		highlighter.lines[line] = template.HTML(out.String())
	}
	return highlighter
}

func (highlighter *syntaxHighlighter) highlight(line, class, newNumber string) template.HTML {
	if highlighter == nil || class == "meta" {
		return template.HTML(template.HTMLEscapeString(line))
	}
	prefix, source := "", line
	if (class == "add" || class == "del" || strings.HasPrefix(class, "ctx")) && len(source) > 0 {
		prefix, source = source[:1], source[1:]
	}
	if class != "del" {
		if number, err := strconv.Atoi(newNumber); err == nil {
			if highlighted, ok := highlighter.lines[number]; ok {
				return template.HTML(template.HTMLEscapeString(prefix) + string(highlighted))
			}
		}
	}
	if highlighter.lexer == nil {
		return template.HTML(template.HTMLEscapeString(line))
	}
	iterator, err := highlighter.lexer.Tokenise(nil, source)
	if err != nil {
		return template.HTML(template.HTMLEscapeString(line))
	}
	var out strings.Builder
	out.WriteString(template.HTMLEscapeString(prefix))
	for token := iterator(); token != chroma.EOF; token = iterator() {
		out.WriteString(highlightedToken(token.Type, token.Value))
	}
	return template.HTML(out.String())
}

func highlightedToken(token chroma.TokenType, value string) string {
	escaped := template.HTMLEscapeString(value)
	if class := syntaxClass(token); class != "" {
		return `<span class="` + class + `">` + escaped + `</span>`
	}
	return escaped
}

func syntaxClass(token chroma.TokenType) string {
	switch {
	case token >= chroma.Keyword && token < chroma.Name:
		return "syntax-keyword"
	case token == chroma.NameFunction || token == chroma.NameFunctionMagic:
		return "syntax-function"
	case token == chroma.NameClass || token == chroma.NameBuiltin || token == chroma.NameBuiltinPseudo:
		return "syntax-type"
	case token >= chroma.LiteralString && token < chroma.LiteralNumber:
		return "syntax-string"
	case token >= chroma.LiteralNumber && token < chroma.Operator:
		return "syntax-number"
	case token >= chroma.Comment && token < chroma.Generic:
		return "syntax-comment"
	case token >= chroma.Operator && token < chroma.Punctuation:
		return "syntax-operator"
	default:
		return ""
	}
}

func expandableContext(lines []string, firstOldLine, firstNewLine int, direction string, highlights []model.MaterializedFragment, highlighter *syntaxHighlighter) template.HTML {
	if len(lines) == 0 {
		return ""
	}
	hiddenStart, hiddenEnd := 0, len(lines)
	if direction == "up" {
		hiddenEnd = max(0, len(lines)-defaultContextLines)
	} else {
		hiddenStart = min(defaultContextLines, len(lines))
	}
	hiddenCount := hiddenEnd - hiddenStart
	arrow := "↑"
	if direction == "down" {
		arrow = "↓"
	}
	var out strings.Builder
	out.WriteString(`<span class="context-expand context-` + direction + `">`)
	for i, line := range lines {
		if direction == "down" && i == hiddenStart && hiddenCount > 0 {
			out.WriteString(`<button class="expand-lines" type="button" data-direction="down">` + arrow + ` Show ` + strconv.Itoa(hiddenCount) + ` lines below</button>`)
		}
		if direction == "up" && i == hiddenEnd && hiddenCount > 0 {
			out.WriteString(`<button class="expand-lines" type="button" data-direction="up">` + arrow + ` Show ` + strconv.Itoa(hiddenCount) + ` lines above</button>`)
		}
		oldNumber := strconv.Itoa(firstOldLine + i)
		newNumber := strconv.Itoa(firstNewLine + i)
		appendDiffRow(&out, " "+line, contextRowClass(firstNewLine+i, highlights), oldNumber, newNumber, i >= hiddenStart && i < hiddenEnd, highlighter)
	}
	out.WriteString(`</span>`)
	return template.HTML(out.String())
}

func expandableGap(lines []string, firstOldLine, firstNewLine int, highlights []model.MaterializedFragment, highlighter *syntaxHighlighter) template.HTML {
	if len(lines) == 0 {
		return ""
	}
	hiddenStart := min(defaultContextLines, len(lines))
	hiddenEnd := max(hiddenStart, len(lines)-defaultContextLines)
	hiddenCount := hiddenEnd - hiddenStart
	var out strings.Builder
	out.WriteString(`<span class="context-expand context-gap">`)
	for i, line := range lines {
		if i == hiddenStart && hiddenCount > 0 {
			out.WriteString(`<button class="expand-lines" type="button" data-direction="down">↓ Show ` + strconv.Itoa(hiddenCount) + ` lines below</button>`)
		}
		if i == hiddenEnd && hiddenCount > 0 {
			out.WriteString(`<button class="expand-lines" type="button" data-direction="up">↑ Show ` + strconv.Itoa(hiddenCount) + ` lines above</button>`)
		}
		oldNumber := strconv.Itoa(firstOldLine + i)
		newNumber := strconv.Itoa(firstNewLine + i)
		appendDiffRow(&out, " "+line, contextRowClass(firstNewLine+i, highlights), oldNumber, newNumber, i >= hiddenStart && i < hiddenEnd, highlighter)
	}
	out.WriteString(`</span>`)
	return template.HTML(out.String())
}

func contextRowClass(line int, fragments []model.MaterializedFragment) string {
	for _, fragment := range fragments {
		start, count := fragmentStart(fragment), fragmentLines(fragment)
		if count > 0 && line >= start && line < start+count {
			return "ctx other-change"
		}
	}
	return "ctx"
}

func colorPatch(patch string, highlighter *syntaxHighlighter) template.HTML {
	patch = strings.TrimSuffix(patch, "\n")
	if patch == "" {
		return ""
	}
	var out strings.Builder
	oldLine, newLine, inHunk := 0, 0, false
	for _, line := range strings.Split(patch, "\n") {
		class, oldNumber, newNumber := "ctx", "", ""
		switch {
		case strings.HasPrefix(line, "@@ "):
			oldLine, newLine = hunkStarts(line)
			inHunk = true
			continue
		case strings.HasPrefix(line, "diff "):
			class = "meta"
		case inHunk && strings.HasPrefix(line, "+"):
			class, newNumber = "add", strconv.Itoa(newLine)
			newLine++
		case inHunk && strings.HasPrefix(line, "-"):
			class, oldNumber = "del", strconv.Itoa(oldLine)
			oldLine++
		case inHunk && strings.HasPrefix(line, " "):
			oldNumber, newNumber = strconv.Itoa(oldLine), strconv.Itoa(newLine)
			oldLine++
			newLine++
		}
		appendDiffRow(&out, line, class, oldNumber, newNumber, false, highlighter)
	}
	return template.HTML(out.String())
}

func hunkStarts(header string) (int, int) {
	fields := strings.Fields(header)
	if len(fields) < 3 {
		return 0, 0
	}
	return rangeStart(fields[1]), rangeStart(fields[2])
}

func rangeStart(field string) int {
	field = strings.TrimLeft(field, "+-")
	field = strings.SplitN(field, ",", 2)[0]
	start, _ := strconv.Atoi(field)
	return start
}

func Handler(page Page) (http.Handler, error) {
	return handlerAt(page, nil, "/")
}

func HandlerWithQuestions(page Page, store questions.Store) (http.Handler, error) {
	return handlerAt(page, &store, "/")
}

// HandlerWithQuestionsAt serves a viewer below basePath, including its assets
// and question API. This allows a review index to host independent artifacts.
func HandlerWithQuestionsAt(page Page, store questions.Store, basePath string) (http.Handler, error) {
	return handlerAt(page, &store, basePath)
}

// ExportHTML renders a self-contained, read-only viewer. When threads are
// provided, only answered turns are included in the exported snapshot.
func ExportHTML(page Page, threads []questions.Thread) ([]byte, error) {
	answered := answeredThreads(threads)
	return renderReactHTML(page, answered, false, "/")
}

func answeredThreads(threads []questions.Thread) []questions.Thread {
	var result []questions.Thread
	for _, thread := range threads {
		copy := thread
		copy.Turns = nil
		for _, turn := range thread.Turns {
			if turn.Status == questions.StatusAnswered {
				copy.Turns = append(copy.Turns, turn)
			}
		}
		if len(copy.Turns) > 0 {
			result = append(result, copy)
		}
	}
	return result
}

func handlerAt(page Page, questionStore *questions.Store, basePath string) (http.Handler, error) {
	if !strings.HasPrefix(basePath, "/") || !strings.HasSuffix(basePath, "/") {
		return nil, fmt.Errorf("viewer base path must start and end with /")
	}
	index, err := renderReactHTML(page, nil, questionStore != nil, basePath)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	if questionStore != nil {
		sessionStore := questionStore.Sessions()
		validGroups := map[string]bool{}
		validFragments := map[string]string{}
		validSteps := map[string]bool{}
		for _, group := range page.Groups {
			validGroups[group.ID] = true
			for _, step := range group.Steps {
				validSteps[group.ID+"\x00"+step.ID] = true
			}
			for _, file := range group.Files {
				for _, fragment := range file.Fragments {
					validFragments[fragment.ID] = group.ID
				}
			}
		}
		mux.HandleFunc("/api/questions", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.Method {
			case http.MethodGet:
				items, err := questionStore.List()
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				_ = json.NewEncoder(w).Encode(buildViewerThreads(items))
			case http.MethodPost:
				active, err := sessionStore.IsActive()
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				if !active {
					http.Error(w, "answer mode is not active", http.StatusConflict)
					return
				}
				var request struct {
					Anchor   questions.Anchor `json:"anchor"`
					Question string           `json:"question"`
					ThreadID string           `json:"thread_id,omitempty"`
				}
				body := http.MaxBytesReader(w, r.Body, 64<<10)
				decoder := json.NewDecoder(body)
				decoder.DisallowUnknownFields()
				if err := decoder.Decode(&request); err != nil && err != io.EOF {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				var item questions.Thread
				if request.ThreadID == "" {
					valid := request.Anchor.Type == "group" && validGroups[request.Anchor.GroupID]
					if request.Anchor.Type == "fragment" {
						valid = validFragments[request.Anchor.FragmentID] == request.Anchor.GroupID
					}
					if request.Anchor.Type == "step" {
						valid = validSteps[request.Anchor.GroupID+"\x00"+request.Anchor.StepID]
					}
					if !valid {
						http.Error(w, "unknown question anchor", http.StatusBadRequest)
						return
					}
				}
				err = sessionStore.WithActive(func() error {
					var mutateErr error
					if request.ThreadID != "" {
						item, mutateErr = questionStore.FollowUp(request.ThreadID, request.Question)
					} else {
						item, mutateErr = questionStore.Add(request.Anchor, request.Question)
					}
					return mutateErr
				})
				if err != nil {
					if errors.Is(err, questions.ErrNoActiveSession) {
						http.Error(w, "answer mode is not active", http.StatusConflict)
						return
					}
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(buildViewerThread(item))
			default:
				w.Header().Set("Allow", "GET, POST")
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		})
		mux.HandleFunc("/api/questions/session", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.Method {
			case http.MethodGet:
				session, found, err := sessionStore.Get()
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				if !found {
					_ = json.NewEncoder(w).Encode(struct {
						Status questions.SessionStatus `json:"status"`
					}{Status: questions.SessionStopped})
					return
				}
				_ = json.NewEncoder(w).Encode(session)
			case http.MethodPost:
				session, err := sessionStore.Stop()
				if err != nil {
					http.Error(w, err.Error(), http.StatusConflict)
					return
				}
				_ = json.NewEncoder(w).Encode(session)
			default:
				w.Header().Set("Allow", "GET, POST")
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		})
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
	return mux, nil
}
