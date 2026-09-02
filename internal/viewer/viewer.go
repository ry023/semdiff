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

	categorydraft "github.com/ry023/semdiff/internal/categories"
	"github.com/ry023/semdiff/internal/model"
	"github.com/ry023/semdiff/internal/questions"
	"github.com/yuin/goldmark"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
)

//go:embed index.html importance.css importance.js questions.css questions.js answers.js review-drift.css
var assets embed.FS

const defaultContextLines = 5

const reviewDriftMarkup = `{{with .Drift}}<section class="review-drift" role="status"><strong>This semantic review is {{len .Commits}} unreviewed {{if eq (len .Commits) 1}}commit{{else}}commits{{end}} behind HEAD.</strong><p>Groups cover <code>{{$.BaseSHA}}...{{$.HeadSHA}}</code>. The current range is <code>{{.CurrentBaseSHA}}...{{.CurrentHeadSHA}}</code>.</p><details><summary>Changes since this review</summary><div class="review-drift-columns"><div><h3>Commits</h3><ul>{{range .Commits}}<li><code>{{printf "%.12s" .SHA}}</code> {{.Subject}} <small>({{.FilesChanged}} files)</small></li>{{end}}</ul></div><div><h3>Files</h3><ul>{{range .Paths}}<li><code>{{.}}</code></li>{{end}}</ul></div></div></details></section>{{end}}`

func addReviewDriftMarkup(source string) string {
	const marker = `<div class="stats">{{len .Groups}} groups · {{.FileCount}} files · {{.FragmentCount}} fragments</div>`
	return strings.Replace(source, marker, marker+reviewDriftMarkup, 1)
}

const guidedReviewMarkup = `<div class="review-mode-toolbar" role="group" aria-label="Review layout"><button class="review-mode-toggle" type="button" data-review-mode="guided" aria-pressed="true">Guided</button><button class="review-mode-toggle" type="button" data-review-mode="files" aria-pressed="false">Files</button></div><section class="guided-view">{{range .Groups}}{{$group := .}}<details id="guided-{{.AnchorID}}" class="group main-group guided-group" data-group-id="{{.ID}}" open><summary><h2>{{if .Order}}{{.Order}}. {{end}}{{.Title}}</h2><span class="count">{{len .Steps}} steps · {{.FragmentCount}} fragments</span></summary><div class="summary guided-group-summary">{{.SummaryHTML}}</div>{{range .Steps}}<details id="{{.AnchorID}}" class="review-step" data-step-id="{{.ID}}"><summary><h3>{{.Title}}</h3><div class="step-summary">{{.SummaryHTML}}</div></summary>{{range .Fragments}}<details class="guided-file main-guided-fragment" data-group-id="{{$group.ID}}" data-file-path="{{.Path}}" open><summary><span class="file-heading"><h3>{{.Path}}</h3></span><div class="file-fragment-description guided-fragment-description"><span class="fragment-note"><span class="fragment-note-id">{{.ID}} · {{.RangeLabel}}</span>{{if .Description}}<span class="fragment-note-description">{{.Description}}</span>{{end}}</span></div></summary><pre>{{.HeaderHTML}}{{.UpperContextHTML}}{{.HunkHTML}}{{.LowerContextHTML}}</pre></details>{{end}}</details>{{end}}</details>{{end}}</section><section class="files-view">`

const guidedReviewStyle = `<style>.review-mode-toolbar{display:flex;justify-content:flex-end;margin:-16px 0 18px}.review-mode-toggle{padding:6px 12px;border:1px solid var(--line);background:var(--panel);color:var(--muted);cursor:pointer}.review-mode-toggle:first-child{border-radius:6px 0 0 6px}.review-mode-toggle:last-child{border-radius:0 6px 6px 0}.review-mode-toggle[aria-pressed=true]{background:#1f4675;color:#fff;border-color:#3977b9}.files-view{display:none}.guided-view .group{margin-top:0}.guided-group>summary{padding:14px 18px!important;background:#202b38!important;border-bottom:1px solid #36516f!important;border-left:4px solid var(--accent)!important;color:#f0f6fc!important;box-shadow:0 3px 10px rgba(0,0,0,.28)!important}.guided-group>summary h2{color:#f0f6fc}.guided-group>summary .count{color:#b6c5d6}.guided-group-summary{margin:12px 18px 2px;padding:10px 12px;border:1px solid #284560;border-left:3px solid #3977b9;border-radius:6px;background:#172638;color:#d7e5f4;line-height:1.5}.guided-group-summary p{margin:0 0 8px}.guided-group-summary p:last-child{margin-bottom:0}.guided-group-summary code{color:#b7d8ff}.review-step{border-top:1px solid var(--line);padding:14px 16px;overflow-anchor:none}.review-step>summary,.guided-file>summary{cursor:pointer;list-style:none}.review-step>summary::-webkit-details-marker,.guided-file>summary::-webkit-details-marker{display:none}.review-step>summary:before,.guided-file>summary:before{content:'▶';display:inline-block;margin-right:8px;font-size:10px}.review-step[open]>summary:before,.guided-file[open]>summary:before{transform:rotate(90deg)}.guided-group .review-step[open]>summary{position:sticky;top:var(--group-header-height,0px);z-index:11;margin:-8px -8px 8px;padding:8px;background:#202733;border:1px solid var(--line);border-radius:6px;box-shadow:0 4px 10px rgba(0,0,0,.3)}.guided-group .review-step[open] .guided-file[open]>summary{position:sticky;top:calc(var(--group-header-height,0px) + var(--step-header-height,0px));z-index:10;margin:-4px -4px 8px;padding:7px;background:#202733;border:1px solid var(--line);border-radius:6px;box-shadow:0 3px 8px rgba(0,0,0,.25)}.review-step h3{display:inline;font-size:15px}.step-summary{margin:7px 0 0 19px;color:var(--muted);line-height:1.45}.step-summary p{margin:0}.guided-file{margin:14px 0 22px;border:1px solid var(--line);border-radius:6px;padding:10px;background:#111821;overflow-anchor:none}.guided-file .file-heading{max-width:none}.guided-file .file-heading h3{font:400 14px ui-monospace,monospace;margin:0;color:var(--text)}.guided-fragment-description{margin:6px 0 0 19px}.guided-file .fragment-note{border:0;border-radius:0;background:transparent;padding:0}.guided-file .fragment-note .ask-button{margin-left:8px}@media(max-width:850px){.review-mode-toolbar{margin-top:0}}</style><script>(function(){function start(){function setMode(mode){document.body.dataset.reviewMode=mode;document.querySelectorAll('.review-mode-toggle').forEach(function(button){button.setAttribute('aria-pressed',String(button.dataset.reviewMode===mode))});document.querySelector('.guided-view').style.display=mode==='guided'?'block':'none';document.querySelector('.files-view').style.display=mode==='files'?'block':'none'}function updateStepHeaderHeight(step){var summary=step.querySelector(':scope > summary');if(summary)step.style.setProperty('--step-header-height',summary.offsetHeight+'px')}var observer=typeof ResizeObserver==='function'?new ResizeObserver(function(entries){entries.forEach(function(entry){var step=entry.target.closest('.review-step');if(step)updateStepHeaderHeight(step)})}):null;var closingTops=new WeakMap();document.addEventListener('click',function(event){var summary=event.target.closest('.guided-file>summary,.review-step>summary');if(!summary)return;var details=summary.parentElement;if(details.open)closingTops.set(details,summary.getBoundingClientRect().top)},true);document.addEventListener('toggle',function(event){var details=event.target;if(!(details instanceof HTMLDetailsElement)||details.open)return;var top=closingTops.get(details);if(top===undefined)return;closingTops.delete(details);requestAnimationFrame(function(){window.scrollBy(0,details.querySelector(':scope > summary').getBoundingClientRect().top-top)})},true);document.querySelectorAll('.review-step').forEach(function(step){var summary=step.querySelector(':scope > summary');updateStepHeaderHeight(step);if(observer&&summary)observer.observe(summary)});window.addEventListener('resize',function(){document.querySelectorAll('.review-step').forEach(updateStepHeaderHeight)});document.querySelectorAll('.review-mode-toggle').forEach(function(button){button.addEventListener('click',function(){setMode(button.dataset.reviewMode)})});setMode('guided')}if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',start);else start()})();</script>`

const summaryListStyle = `<style>.summary ul,.summary ol,.step-summary ul,.step-summary ol{margin:0!important;padding-left:20px!important;white-space:normal}.summary li,.step-summary li{margin:4px 0!important;white-space:normal}.summary li>p,.step-summary li>p{margin:0!important}.summary ul ul,.summary ul ol,.summary ol ul,.summary ol ol,.step-summary ul ul,.step-summary ul ol,.step-summary ol ul,.step-summary ol ol{margin:4px 0 0!important;padding-left:20px!important}</style>`

func addGuidedReviewMarkup(source string) string {
	const groupStart = `</div>{{range .Groups}}{{$group := .}}<details id="{{.AnchorID}}" class="group main-group"`
	if !strings.Contains(source, groupStart) {
		return source
	}
	source = strings.Replace(source, groupStart, `</div>`+guidedReviewMarkup+`{{range .Groups}}{{$group := .}}<details id="{{.AnchorID}}" class="group main-group"`, 1)
	source = strings.Replace(source, `</main>`, `</section></main>`, 1)
	return strings.Replace(source, `</head>`, summaryListStyle+guidedReviewStyle+`</head>`, 1)
}

type FragmentView struct {
	model.MaterializedFragment
	Description      string
	ReviewLevel      model.ReviewLevel
	RangeLabel       string
	HeaderHTML       template.HTML
	HunkHTML         template.HTML
	UpperContextHTML template.HTML
	LowerContextHTML template.HTML
}
type FileView struct {
	Path        string
	AnchorID    string
	Directory   string
	Name        string
	Status      string
	StatusIcon  template.HTML
	Additions   int
	Deletions   int
	Diffstat    []string
	HeaderHTML  template.HTML
	Fragments   []FragmentView
	ReviewLevel model.ReviewLevel
}
type GroupView struct {
	ID, Title, Summary string
	Importance         model.Importance
	AnchorID           string
	SummaryHTML        template.HTML
	Order              *int
	Files              []FileView
	Categories         []CategoryView
	Steps              []ReviewStepView
	FragmentCount      int
}

type ReviewStepView struct {
	ID, Title, Summary string
	SummaryHTML        template.HTML
	AnchorID           string
	Fragments          []FragmentView
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

type ReviewDrift struct {
	CurrentBaseSHA string
	CurrentHeadSHA string
	Commits        []model.Commit
	Paths          []string
}

type Page struct {
	BaseSHA, HeadSHA         string
	Drift                    *ReviewDrift
	Groups                   []GroupView
	SidebarDirectories       []SidebarDirectory
	SidebarFiles             []SidebarFile
	FragmentCount, FileCount int
}

type SidebarOccurrence struct {
	GroupID, GroupTitle string
	FileAnchorID        string
	FragmentCount       int
	ReviewLevel         model.ReviewLevel
}

type SidebarFile struct {
	Path, Name  string
	StatusIcon  template.HTML
	ReviewLevel model.ReviewLevel
	Occurrences []SidebarOccurrence
}

type SidebarDirectory struct {
	Name        string
	FileCount   int
	Directories []SidebarDirectory
	Files       []SidebarFile
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
			file := buildFileView(path, fileMap[path], fileContents[path], byPath[path])
			file.AnchorID = fmt.Sprintf("file-%d-%d", groupIndex, fileIndex)
			for i := range file.Fragments {
				file.Fragments[i].Description = descriptions[file.Fragments[i].ID]
				file.Fragments[i].RangeLabel = rangeLabels[file.Fragments[i].ID]
				file.Fragments[i].ReviewLevel = reviewLevels[file.Fragments[i].ID]
				file.ReviewLevel = strongerReviewLevel(file.ReviewLevel, file.Fragments[i].ReviewLevel)
			}
			gv.Files = append(gv.Files, file)
		}
		for stepIndex, step := range group.ReviewSteps {
			sv := ReviewStepView{ID: step.ID, Title: step.Title, Summary: step.Summary, SummaryHTML: renderMarkdown(step.Summary), AnchorID: fmt.Sprintf("step-%d-%d", groupIndex, stepIndex)}
			for _, id := range step.FragmentIDs {
				fragment := byID[id]
				view := buildFragmentView(fragment, fileContents[fragment.Path], byPath[fragment.Path])
				view.Description = descriptions[id]
				view.RangeLabel = rangeLabels[id]
				view.ReviewLevel = reviewLevels[id]
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
				sidebarFile = &SidebarFile{Path: file.Path, Name: file.Name, StatusIcon: file.StatusIcon}
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
	upperOldFirst, lowerOldFirst := upperStart+1, end+1
	blocks := splitHunkBlocks(hunk)
	if len(blocks) > 0 {
		first := parseHunkBounds(blocks[0])
		last := parseHunkBounds(blocks[len(blocks)-1])
		upperOldFirst = max(1, first.oldStart-(start-upperStart)+1)
		lowerOldFirst = last.oldEnd + 1
	}
	view.UpperContextHTML = expandableContext(lines[upperStart:start], upperOldFirst, upperStart+1, "up")
	view.LowerContextHTML = expandableContext(lines[end:lowerEnd], lowerOldFirst, end+1, "down")
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
			previous := parseHunkBounds(blocks[i-1])
			current := parseHunkBounds(block)
			previousEnd := previous.newEnd
			currentStart := current.newStart
			previousEnd = max(0, min(previousEnd, len(lines)))
			currentStart = max(previousEnd, min(currentStart, len(lines)))
			gapLines := lines[previousEnd:currentStart]
			oldFirst := max(1, current.oldStart-len(gapLines)+1)
			out.WriteString(string(expandableGap(gapLines, oldFirst, previousEnd+1)))
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
		gapLines := lines[start:end]
		oldFirst := start + 1
		_, currentHunk := splitPatch(current.Patch)
		currentBlocks := splitHunkBlocks(currentHunk)
		if len(currentBlocks) > 0 {
			bounds := parseHunkBounds(currentBlocks[0])
			oldFirst = max(1, bounds.oldStart-len(gapLines)+1)
		}
		previous.LowerContextHTML = expandableGap(gapLines, oldFirst, start+1)
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

func expandableContext(lines []string, firstOldLine, firstNewLine int, direction string) template.HTML {
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
		appendDiffRow(&out, " "+line, "ctx", oldNumber, newNumber, i >= hiddenStart && i < hiddenEnd)
	}
	out.WriteString(`</span>`)
	return template.HTML(out.String())
}

func expandableGap(lines []string, firstOldLine, firstNewLine int) template.HTML {
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
		appendDiffRow(&out, " "+line, "ctx", oldNumber, newNumber, i >= hiddenStart && i < hiddenEnd)
	}
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
	index, err := assets.ReadFile("index.html")
	if err != nil {
		return nil, err
	}
	importanceCSS, err := assets.ReadFile("importance.css")
	if err != nil {
		return nil, err
	}
	reviewDriftCSS, err := assets.ReadFile("review-drift.css")
	if err != nil {
		return nil, err
	}
	importanceJS, err := assets.ReadFile("importance.js")
	if err != nil {
		return nil, err
	}
	importanceJS = []byte(strings.Replace(string(importanceJS), "var importanceData=window.semdiffImportance?Promise.resolve(window.semdiffImportance):fetch('/importance.json').then(function(response){return response.json()});", "var importanceData=Promise.resolve(window.semdiffImportance);", 1))
	importanceJSON, err := json.Marshal(buildImportanceData(page))
	if err != nil {
		return nil, err
	}
	source := addGuidedReviewMarkup(addReviewDriftMarkup(string(index)))
	source = strings.Replace(source, "</head>", "<style>"+string(importanceCSS)+string(reviewDriftCSS)+"</style></head>", 1)
	scripts := "<script>window.semdiffImportance=" + string(importanceJSON) + ";</script><script>" + string(importanceJS) + "</script>"
	answered := answeredThreads(threads)
	if len(answered) > 0 {
		questionsCSS, readErr := assets.ReadFile("questions.css")
		if readErr != nil {
			return nil, readErr
		}
		answersJS, readErr := assets.ReadFile("answers.js")
		if readErr != nil {
			return nil, readErr
		}
		answersJSON, marshalErr := json.Marshal(answered)
		if marshalErr != nil {
			return nil, marshalErr
		}
		source = strings.Replace(source, "</head>", "<style>"+string(questionsCSS)+"</style></head>", 1)
		scripts += "<script>window.semdiffAnswers=" + string(answersJSON) + ";</script><script>" + string(answersJS) + "</script>"
	}
	source = strings.Replace(source, "</body>", scripts+"</body>", 1)
	t, err := template.New("index.html").Parse(source)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if err := t.Execute(&output, page); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
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

func buildImportanceData(page Page) struct {
	Groups    map[string]model.Importance  `json:"groups"`
	Fragments map[string]model.ReviewLevel `json:"fragments"`
} {
	data := struct {
		Groups    map[string]model.Importance  `json:"groups"`
		Fragments map[string]model.ReviewLevel `json:"fragments"`
	}{Groups: map[string]model.Importance{}, Fragments: map[string]model.ReviewLevel{}}
	for _, group := range page.Groups {
		data.Groups[group.ID] = group.Importance
		for _, file := range group.Files {
			for _, fragment := range file.Fragments {
				data.Fragments[fragment.ID] = fragment.ReviewLevel
			}
		}
	}
	return data
}

func handlerAt(page Page, questionStore *questions.Store, basePath string) (http.Handler, error) {
	if !strings.HasPrefix(basePath, "/") || !strings.HasSuffix(basePath, "/") {
		return nil, fmt.Errorf("viewer base path must start and end with /")
	}
	index, err := assets.ReadFile("index.html")
	if err != nil {
		return nil, err
	}
	source := addGuidedReviewMarkup(addReviewDriftMarkup(string(index)))
	source = strings.Replace(source, "</head>", `<link rel="stylesheet" href="`+basePath+`importance.css"></head>`, 1)
	source = strings.Replace(source, "</head>", `<link rel="stylesheet" href="`+basePath+`review-drift.css"></head>`, 1)
	source = strings.Replace(source, "</head>", `<link rel="stylesheet" href="`+basePath+`questions.css"></head>`, 1)
	source = strings.Replace(source, "</body>", `<script src="`+basePath+`importance.js"></script></body>`, 1)
	source = strings.Replace(source, "</body>", `<script src="`+basePath+`questions.js"></script></body>`, 1)
	t, err := template.New("index.html").Parse(source)
	if err != nil {
		return nil, err
	}
	importanceData := buildImportanceData(page)
	importanceJSON, err := json.Marshal(importanceData)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/importance.css", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		content, _ := assets.ReadFile("importance.css")
		_, _ = w.Write(content)
	})
	mux.HandleFunc("/review-drift.css", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		content, _ := assets.ReadFile("review-drift.css")
		_, _ = w.Write(content)
	})
	mux.HandleFunc("/importance.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		content, _ := assets.ReadFile("importance.js")
		_, _ = w.Write([]byte(strings.ReplaceAll(string(content), "'/importance.json'", "'"+basePath+"importance.json'")))
	})
	mux.HandleFunc("/questions.css", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		content, _ := assets.ReadFile("questions.css")
		_, _ = w.Write(content)
	})
	mux.HandleFunc("/questions.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		content, _ := assets.ReadFile("questions.js")
		_, _ = w.Write([]byte(strings.ReplaceAll(string(content), "'/api/questions'", "'"+basePath+"api/questions'")))
	})
	mux.HandleFunc("/importance.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(importanceJSON)
	})
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
				_ = json.NewEncoder(w).Encode(items)
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
				_ = json.NewEncoder(w).Encode(item)
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
		_ = t.Execute(w, page)
	})
	return mux, nil
}
