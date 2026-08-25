package viewer

import (
	"embed"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/ry023/semdiff/internal/model"
)

//go:embed index.html
var assets embed.FS

type FragmentView struct {
	model.DiffFragment
	HeaderHTML       template.HTML
	HunkHTML         template.HTML
	UpperContextHTML template.HTML
	LowerContextHTML template.HTML
}
type FileView struct {
	Path       string
	HeaderHTML template.HTML
	Fragments  []FragmentView
}
type GroupView struct {
	ID, Title, Summary string
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
		gv := GroupView{ID: group.ID, Title: group.Title, Summary: group.Summary, Order: group.Order}
		fileMap := map[string][]model.DiffFragment{}
		var paths []string
		for _, id := range group.FragmentIDs {
			f := byID[id]
			if _, ok := fileMap[f.Path]; !ok {
				paths = append(paths, f.Path)
			}
			fileMap[f.Path] = append(fileMap[f.Path], f)
			allFiles[f.Path] = true
			gv.FragmentCount++
		}
		sort.Strings(paths)
		for _, path := range paths {
			gv.Files = append(gv.Files, buildFileView(path, fileMap[path], fileContents[path], byPath[path]))
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
	view.UpperContextHTML = expandableContext(lines[upperStart:start], "up")
	view.LowerContextHTML = expandableContext(lines[end:lowerEnd], "down")
	return view
}

func buildFileView(path string, fragments []model.DiffFragment, content string, siblings []model.DiffFragment) FileView {
	sort.SliceStable(fragments, func(i, j int) bool {
		return fragmentStart(fragments[i]) < fragmentStart(fragments[j])
	})
	file := FileView{Path: path}
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
		previous.LowerContextHTML = expandableGap(lines[start:end])
		current.UpperContextHTML = ""
	}
	return file
}

func expandableContext(lines []string, direction string) template.HTML {
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
	for _, line := range lines {
		out.WriteString(`<span class="context-hidden" hidden> `)
		out.WriteString(template.HTMLEscapeString(line))
		out.WriteString(`</span>`)
	}
	if direction == "up" {
		out.WriteString(`<button class="expand-lines" type="button" data-direction="up">` + arrow + ` Show ` + strconv.Itoa(len(lines)) + ` lines above</button>`)
	}
	out.WriteString(`</span>`)
	return template.HTML(out.String())
}

func expandableGap(lines []string) template.HTML {
	if len(lines) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString(`<span class="context-expand context-gap">`)
	out.WriteString(`<button class="expand-lines" type="button" data-direction="down">↓ Show ` + strconv.Itoa(len(lines)) + ` lines below</button>`)
	for _, line := range lines {
		out.WriteString(`<span class="context-hidden" hidden> `)
		out.WriteString(template.HTMLEscapeString(line))
		out.WriteString(`</span>`)
	}
	out.WriteString(`<button class="expand-lines" type="button" data-direction="up">↑ Show ` + strconv.Itoa(len(lines)) + ` lines above</button>`)
	out.WriteString(`</span>`)
	return template.HTML(out.String())
}

func colorPatch(patch string) template.HTML {
	// The template escapes each line before these trusted presentation wrappers are added.
	escaped := template.HTMLEscapeString(patch)
	lines := []byte(escaped)
	out := make([]byte, 0, len(lines)+128)
	start := 0
	for i := 0; i <= len(lines); i++ {
		if i == len(lines) && start == len(lines) {
			break
		}
		if i < len(lines) && lines[i] != '\n' {
			continue
		}
		line := lines[start:i]
		class := "ctx"
		if len(line) > 0 {
			if line[0] == '+' && !(len(line) > 2 && string(line[:3]) == "+++") {
				class = "add"
			}
			if line[0] == '-' && !(len(line) > 2 && string(line[:3]) == "---") {
				class = "del"
			}
			if line[0] == '@' {
				class = "hunk"
			}
			if string(line[:min(5, len(line))]) == "diff " {
				class = "meta"
			}
		}
		out = append(out, []byte(`<span class="`+class+`">`)...)
		out = append(out, line...)
		// Each span is a block, so a literal newline here would create an
		// additional blank row inside the preformatted diff.
		out = append(out, []byte("</span>")...)
		start = i + 1
	}
	return template.HTML(out)
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
