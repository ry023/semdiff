package viewer

import (
	"embed"
	"html/template"
	"net/http"
	"sort"

	"github.com/ry023/semdiff/internal/model"
)

//go:embed index.html
var assets embed.FS

type FragmentView struct {
	model.DiffFragment
	PatchHTML template.HTML
}
type FileView struct {
	Path      string
	Fragments []FragmentView
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

func Build(g model.GroupsFile, inv model.Inventory) Page {
	byID := map[string]model.DiffFragment{}
	for _, f := range inv.Fragments {
		byID[f.ID] = f
	}
	p := Page{BaseSHA: g.BaseSHA, HeadSHA: g.HeadSHA}
	allFiles := map[string]bool{}
	for _, group := range g.Groups {
		gv := GroupView{ID: group.ID, Title: group.Title, Summary: group.Summary, Order: group.Order}
		fileMap := map[string][]FragmentView{}
		var paths []string
		for _, id := range group.FragmentIDs {
			f := byID[id]
			if _, ok := fileMap[f.Path]; !ok {
				paths = append(paths, f.Path)
			}
			fileMap[f.Path] = append(fileMap[f.Path], FragmentView{DiffFragment: f, PatchHTML: colorPatch(f.Patch)})
			allFiles[f.Path] = true
			gv.FragmentCount++
		}
		sort.Strings(paths)
		for _, path := range paths {
			gv.Files = append(gv.Files, FileView{Path: path, Fragments: fileMap[path]})
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

func colorPatch(patch string) template.HTML {
	// The template escapes each line before these trusted presentation wrappers are added.
	escaped := template.HTMLEscapeString(patch)
	lines := []byte(escaped)
	out := make([]byte, 0, len(lines)+128)
	start := 0
	for i := 0; i <= len(lines); i++ {
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
		out = append(out, []byte("</span>\n")...)
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
