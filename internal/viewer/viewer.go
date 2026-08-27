package viewer

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
	pathpkg "path"
	"sort"
	"strconv"
	"strings"

	categorydraft "github.com/ry023/semdiff/internal/categories"
	"github.com/ry023/semdiff/internal/model"
	"github.com/yuin/goldmark"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
)

//go:embed index.html
var assets embed.FS

type FragmentView struct {
	model.MaterializedFragment
	Description      string
	RangeLabel       string
	HeaderHTML       template.HTML
	HunkHTML         template.HTML
	UpperContextHTML template.HTML
	LowerContextHTML template.HTML
}
type FileView struct {
	Path       string
	Directory  string
	Name       string
	Status     string
	StatusIcon template.HTML
	Additions  int
	Deletions  int
	Diffstat   []string
	HeaderHTML template.HTML
	Fragments  []FragmentView
}
type GroupView struct {
	ID, Title, Summary string
	SummaryHTML        template.HTML
	Order              *int
	Files              []FileView
	Categories         []CategoryView
	FragmentCount      int
}
type CategoryView struct {
	Name      string
	Icon      template.HTML
	IconClass string
	Standard  bool
	Files     []FileView
	Added     int
	Updated   int
	Deleted   int
}
type Page struct {
	BaseSHA, HeadSHA         string
	Groups                   []GroupView
	FragmentCount, FileCount int
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
	p := Page{BaseSHA: g.BaseSHA, HeadSHA: g.HeadSHA}
	allFiles := map[string]bool{}
	for _, group := range g.Groups {
		gv := GroupView{ID: group.ID, Title: group.Title, Summary: group.Summary, SummaryHTML: renderMarkdown(group.Summary), Order: group.Order}
		fileMap := map[string][]model.MaterializedFragment{}
		descriptions := map[string]string{}
		rangeLabels := map[string]string{}
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
			allFiles[f.Path] = true
			gv.FragmentCount++
		}
		sort.Strings(paths)
		for _, path := range paths {
			file := buildFileView(path, fileMap[path], fileContents[path], byPath[path])
			for i := range file.Fragments {
				file.Fragments[i].Description = descriptions[file.Fragments[i].ID]
				file.Fragments[i].RangeLabel = rangeLabels[file.Fragments[i].ID]
			}
			gv.Files = append(gv.Files, file)
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
	return p
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
		icon, iconClass, standard := categoryIcon(name)
		category := CategoryView{Name: name, Icon: icon, IconClass: iconClass, Standard: standard, Files: byName[name]}
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

func categoryIcon(name string) (template.HTML, string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "implementation":
		return lucideIcon(`<path d="m18 16 4-4-4-4"/><path d="m6 8-4 4 4 4"/><path d="m14.5 4-5 16"/>`), "implementation", true
	case "test":
		return lucideIcon(`<circle cx="12" cy="12" r="9"/><path d="m9 12 2 2 4-4"/>`), "test", true
	case "component":
		return lucideIcon(`<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M3 9h18"/><path d="M9 21V9"/>`), "component", true
	case "logic":
		return lucideIcon(`<line x1="6" x2="6" y1="3" y2="15"/><circle cx="18" cy="6" r="3"/><circle cx="6" cy="18" r="3"/><path d="M18 9a9 9 0 0 1-9 9"/>`), "logic", true
	case "config":
		return lucideIcon(`<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.38a2 2 0 0 0-.73-2.73l-.15-.09a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/>`), "config", true
	case "docs":
		return lucideIcon(`<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/><path d="M8 13h8"/><path d="M8 17h8"/>`), "docs", true
	case "unknown":
		return lucideIcon(`<circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><path d="M12 17h.01"/>`), "unknown", true
	default:
		return lucideIcon(`<path d="M12.586 2.586A2 2 0 0 0 11.172 2H4a2 2 0 0 0-2 2v7.172a2 2 0 0 0 .586 1.414l8.828 8.828a2 2 0 0 0 2.828 0l6.172-6.172a2 2 0 0 0 0-2.828z"/><circle cx="7.5" cy="7.5" r=".5" fill="currentColor"/>`), "custom", false
	}
}

func lucideIcon(content string) template.HTML {
	return template.HTML(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">` + content + `</svg>`)
}

func renderMarkdown(source string) template.HTML {
	var rendered bytes.Buffer
	markdown := goldmark.New(goldmark.WithRendererOptions(goldmarkhtml.WithHardWraps()))
	if err := markdown.Convert([]byte(source), &rendered); err != nil {
		return template.HTML(template.HTMLEscapeString(source))
	}
	return template.HTML(rendered.String())
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

func buildFragmentView(f model.MaterializedFragment, content string, siblings []model.MaterializedFragment) FragmentView {
	header, hunk := splitPatch(f.Patch)
	lines := sourceLines(content)
	view := FragmentView{MaterializedFragment: f, HeaderHTML: colorPatch(header), HunkHTML: colorPatchWithContext(hunk, lines)}
	start := fragmentStart(f) - 1
	if start < 0 || start > len(lines) {
		return view
	}
	end := min(len(lines), start+fragmentLines(f))
	upperStart, lowerEnd := 0, len(lines)
	for i, sibling := range siblings {
		if sibling.ID != f.ID {
			continue
		}
		if i > 0 {
			previous := siblings[i-1]
			upperStart = min(start, fragmentStart(previous)-1+fragmentLines(previous))
		}
		if i+1 < len(siblings) {
			lowerEnd = max(end, fragmentStart(siblings[i+1])-1)
		}
		break
	}
	upperStart = max(0, min(upperStart, start))
	lowerEnd = max(end, min(lowerEnd, len(lines)))
	view.UpperContextHTML = expandableContext(lines[upperStart:start], upperStart+1, "up")
	view.LowerContextHTML = expandableContext(lines[end:lowerEnd], end+1, "down")
	return view
}

// colorPatchWithContext keeps each materialized hunk as an independently
// expandable range. A multi-range fragment has one outer set of controls, but
// without controls between its hunks the source lines in those gaps can never
// be revealed.
func colorPatchWithContext(patch string, lines []string) template.HTML {
	blocks := splitHunkBlocks(patch)
	if len(blocks) < 2 || len(lines) == 0 {
		return colorPatch(patch)
	}
	var out strings.Builder
	for i, block := range blocks {
		if i > 0 {
			_, previousEnd := hunkNewBounds(blocks[i-1])
			currentStart, _ := hunkNewBounds(block)
			previousEnd = max(0, min(previousEnd, len(lines)))
			currentStart = max(previousEnd, min(currentStart, len(lines)))
			out.WriteString(string(expandableGap(lines[previousEnd:currentStart], previousEnd+1)))
		}
		out.WriteString(string(colorPatch(block)))
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

func hunkNewBounds(hunk string) (int, int) {
	header := strings.SplitN(hunk, "\n", 2)[0]
	fields := strings.Fields(header)
	if len(fields) < 3 {
		return 0, 0
	}
	start, count := rangeBounds(fields[2])
	start = max(0, start-1)
	return start, start + count
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

func buildFileView(path string, fragments []model.MaterializedFragment, content string, siblings []model.MaterializedFragment) FileView {
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
	file.StatusIcon = fileStatusIcon(file.Status)
	file.Diffstat = diffstatBlocks(file.Additions, file.Deletions)
	for _, fragment := range fragments {
		file.Fragments = append(file.Fragments, buildFragmentView(fragment, content, siblings))
	}
	if len(file.Fragments) == 0 {
		return file
	}
	file.HeaderHTML = file.Fragments[0].HeaderHTML
	for i := range file.Fragments {
		file.Fragments[i].HeaderHTML = ""
	}
	globalIndex := map[string]int{}
	for i, fragment := range siblings {
		globalIndex[fragment.ID] = i
	}
	lines := sourceLines(content)
	for i := 1; i < len(file.Fragments); i++ {
		previous := &file.Fragments[i-1]
		current := &file.Fragments[i]
		if globalIndex[current.ID] != globalIndex[previous.ID]+1 {
			continue
		}
		start := fragmentStart(previous.MaterializedFragment) - 1 + fragmentLines(previous.MaterializedFragment)
		end := fragmentStart(current.MaterializedFragment) - 1
		start = max(0, min(start, len(lines)))
		end = max(start, min(end, len(lines)))
		previous.LowerContextHTML = expandableGap(lines[start:end], start+1)
		current.UpperContextHTML = ""
	}
	return file
}

func fileStatusIcon(status string) template.HTML {
	switch status {
	case "new":
		return lucideIcon(`<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><path d="M14 2v7h7"/><path d="M12 18v-6"/><path d="M9 15h6"/>`)
	case "deleted":
		return lucideIcon(`<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><path d="M14 2v7h7"/><path d="M9 15h6"/>`)
	default:
		return lucideIcon(`<path d="M12 22h6a2 2 0 0 0 2-2V7l-5-5H6a2 2 0 0 0-2 2v16"/><path d="M14 2v5h5"/><path d="m10.4 12.6 2.9 2.9L19 9.8"/>`)
	}
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

func appendDiffRow(out *strings.Builder, line, class, oldNumber, newNumber string, hidden bool) {
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
	escaped := template.HTMLEscapeString(line)
	out.WriteString(`<span class="line-number unified-cell">` + unifiedNumber + `</span><span class="line-code unified-cell">`)
	out.WriteString(escaped)
	out.WriteString(`</span>`)
	if oldNumber == "" && newNumber == "" {
		out.WriteString(`<span class="split-wide">` + escaped + `</span>`)
	} else {
		oldCode, newCode := "", ""
		switch class {
		case "add":
			newCode = escaped
		case "del":
			oldCode = escaped
		default:
			oldCode, newCode = escaped, escaped
		}
		out.WriteString(`<span class="line-number split-cell old-number">` + oldNumber + `</span><span class="line-code split-cell old-code">` + oldCode + `</span>`)
		out.WriteString(`<span class="line-number split-cell new-number">` + newNumber + `</span><span class="line-code split-cell new-code">` + newCode + `</span>`)
	}
	out.WriteString(`</span>`)
}

func expandableContext(lines []string, firstLine int, direction string) template.HTML {
	if len(lines) == 0 {
		return ""
	}
	arrow := "↑"
	if direction == "down" {
		arrow = "↓"
	}
	var out strings.Builder
	out.WriteString(`<span class="context-expand context-` + direction + `">`)
	if direction == "down" {
		out.WriteString(`<button class="expand-lines" type="button" data-direction="down">` + arrow + ` Show ` + strconv.Itoa(len(lines)) + ` lines below</button>`)
	}
	for i, line := range lines {
		number := strconv.Itoa(firstLine + i)
		appendDiffRow(&out, " "+line, "ctx", number, number, true)
	}
	if direction == "up" {
		out.WriteString(`<button class="expand-lines" type="button" data-direction="up">` + arrow + ` Show ` + strconv.Itoa(len(lines)) + ` lines above</button>`)
	}
	out.WriteString(`</span>`)
	return template.HTML(out.String())
}

func expandableGap(lines []string, firstLine int) template.HTML {
	if len(lines) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString(`<span class="context-expand context-gap">`)
	out.WriteString(`<button class="expand-lines" type="button" data-direction="down">↓ Show ` + strconv.Itoa(len(lines)) + ` lines below</button>`)
	for i, line := range lines {
		number := strconv.Itoa(firstLine + i)
		appendDiffRow(&out, " "+line, "ctx", number, number, true)
	}
	out.WriteString(`<button class="expand-lines" type="button" data-direction="up">↑ Show ` + strconv.Itoa(len(lines)) + ` lines above</button>`)
	out.WriteString(`</span>`)
	return template.HTML(out.String())
}

func colorPatch(patch string) template.HTML {
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
			class = "hunk"
			oldLine, newLine = hunkStarts(line)
			inHunk = true
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
		appendDiffRow(&out, line, class, oldNumber, newNumber, false)
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
	t, err := template.ParseFS(assets, "index.html")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		_ = t.Execute(w, page)
	})
	return mux, nil
}
