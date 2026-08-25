package viewer

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/ry023/semdiff/internal/model"
	"github.com/yuin/goldmark"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
)

//go:embed index.html
var assets embed.FS

type FragmentView struct {
	model.DiffFragment
	Description      string
	HeaderHTML       template.HTML
	HunkHTML         template.HTML
	UpperContextHTML template.HTML
	LowerContextHTML template.HTML
}
type FileView struct {
	Path       string
	Status     string
	StatusIcon string
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
	FragmentCount      int
}
type Page struct {
	BaseSHA, HeadSHA         string
	Groups                   []GroupView
	FragmentCount, FileCount int
}

func Build(g model.GroupsFile, inv model.Inventory, contents ...map[string]string) Page {
	var fileContents map[string]string
	if len(contents) > 0 {
		fileContents = contents[0]
	}
	byID := map[string]model.DiffFragment{}
	byPath := map[string][]model.DiffFragment{}
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
		fileMap := map[string][]model.DiffFragment{}
		descriptions := map[string]string{}
		var paths []string
		for _, reference := range group.FragmentReferences() {
			id := reference.ID
			f := byID[id]
			if _, ok := fileMap[f.Path]; !ok {
				paths = append(paths, f.Path)
			}
			fileMap[f.Path] = append(fileMap[f.Path], f)
			descriptions[id] = reference.Description
			allFiles[f.Path] = true
			gv.FragmentCount++
		}
		sort.Strings(paths)
		for _, path := range paths {
			file := buildFileView(path, fileMap[path], fileContents[path], byPath[path])
			for i := range file.Fragments {
				file.Fragments[i].Description = descriptions[file.Fragments[i].ID]
			}
			gv.Files = append(gv.Files, file)
		}
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

func renderMarkdown(source string) template.HTML {
	var rendered bytes.Buffer
	markdown := goldmark.New(goldmark.WithRendererOptions(goldmarkhtml.WithHardWraps()))
	if err := markdown.Convert([]byte(source), &rendered); err != nil {
		return template.HTML(template.HTMLEscapeString(source))
	}
	return template.HTML(rendered.String())
}

func fragmentStart(f model.DiffFragment) int {
	if f.NewLines > 0 {
		return f.NewStart
	}
	return f.OldStart
}

func fragmentLines(f model.DiffFragment) int {
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

func buildFragmentView(f model.DiffFragment, content string, siblings []model.DiffFragment) FragmentView {
	header, hunk := splitPatch(f.Patch)
	view := FragmentView{DiffFragment: f, HeaderHTML: colorPatch(header), HunkHTML: colorPatch(hunk)}
	lines := sourceLines(content)
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

func buildFileView(path string, fragments []model.DiffFragment, content string, siblings []model.DiffFragment) FileView {
	sort.SliceStable(fragments, func(i, j int) bool {
		return fragmentStart(fragments[i]) < fragmentStart(fragments[j])
	})
	file := FileView{Path: path, Status: "updated", StatusIcon: "~"}
	for _, fragment := range siblings {
		if strings.Contains(fragment.Patch, "new file mode ") || strings.Contains(fragment.Patch, "--- /dev/null") {
			file.Status, file.StatusIcon = "new", "+"
		}
		if strings.Contains(fragment.Patch, "deleted file mode ") || strings.Contains(fragment.Patch, "+++ /dev/null") {
			file.Status, file.StatusIcon = "deleted", "−"
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
		start := fragmentStart(previous.DiffFragment) - 1 + fragmentLines(previous.DiffFragment)
		end := fragmentStart(current.DiffFragment) - 1
		start = max(0, min(start, len(lines)))
		end = max(start, min(end, len(lines)))
		previous.LowerContextHTML = expandableGap(lines[start:end], start+1)
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
