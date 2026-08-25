package gitdiff

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ry023/semdiff/internal/model"
)

type Runner struct{ Dir string }

func ParseRange(s string) (string, string, error) {
	parts := strings.Split(s, "..")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.Contains(s, "...") {
		return "", "", fmt.Errorf("range must have the form <base>..<head>")
	}
	return parts[0], parts[1], nil
}

func (r Runner) git(ctx context.Context, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, "git", append([]string{"-C", r.Dir}, args...)...)
	b, err := c.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(b)))
	}
	return b, nil
}

func (r Runner) Resolve(ctx context.Context, rev string) (string, error) {
	b, err := r.git(ctx, "rev-parse", "--verify", rev+"^{commit}")
	return strings.TrimSpace(string(b)), err
}

func (r Runner) Commits(ctx context.Context, rangeSpec string) ([]model.Commit, error) {
	base, head, err := ParseRange(rangeSpec)
	if err != nil {
		return nil, err
	}
	if _, err = r.Resolve(ctx, base); err != nil {
		return nil, err
	}
	if _, err = r.Resolve(ctx, head); err != nil {
		return nil, err
	}
	b, err := r.git(ctx, "log", "--reverse", "--format=%H%x00%s%x00%an%x00%aI", "--name-only", rangeSpec, "--")
	if err != nil {
		return nil, err
	}
	var out []model.Commit
	var files map[string]bool
	flush := func() {
		if len(out) > 0 && files != nil {
			out[len(out)-1].FilesChanged = len(files)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if strings.Contains(line, "\x00") {
			flush()
			fields := strings.Split(line, "\x00")
			if len(fields) != 4 {
				continue
			}
			out = append(out, model.Commit{SHA: fields[0], Subject: fields[1], Author: fields[2], Timestamp: fields[3]})
			files = map[string]bool{}
			continue
		}
		if line != "" && files != nil {
			files[line] = true
		}
	}
	flush()
	return out, nil
}

var hunkRE = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

func (r Runner) Changes(ctx context.Context, rangeSpec string) (model.ChangeMap, error) {
	base, head, err := ParseRange(rangeSpec)
	if err != nil {
		return model.ChangeMap{}, err
	}
	baseSHA, err := r.Resolve(ctx, base)
	if err != nil {
		return model.ChangeMap{}, err
	}
	headSHA, err := r.Resolve(ctx, head)
	if err != nil {
		return model.ChangeMap{}, err
	}
	b, err := r.git(ctx, "diff", "--find-renames", "--no-color", "--unified=0", baseSHA, headSHA, "--")
	if err != nil {
		return model.ChangeMap{}, err
	}
	frags, err := ParseUnified(b, baseSHA, headSHA)
	if err != nil {
		return model.ChangeMap{}, err
	}
	return model.ChangeMap{BaseSHA: baseSHA, HeadSHA: headSHA, Changes: frags}, nil
}

// SuggestedFragments converts Git's zero-context changed spans into editable
// fragment definitions. These are starting points only; groups.json stores
// the resulting ranges as its source of truth.
func SuggestedFragments(inv model.ChangeMap) []model.Fragment {
	result := make([]model.Fragment, 0, len(inv.Changes))
	metadataPaths := map[string]bool{}
	for _, change := range inv.Changes {
		if change.Metadata || change.OldLines == 0 && change.NewLines == 0 {
			metadataPaths[change.Path] = true
		}
	}
	for _, change := range inv.Changes {
		if change.Metadata {
			continue
		}
		fragment := model.Fragment{ID: change.ID, Path: change.Path}
		if change.OldLines > 0 || change.NewLines > 0 {
			span := model.FragmentRange{}
			if change.OldLines > 0 {
				span.Old = &model.Range{Start: change.OldStart, Lines: change.OldLines}
			}
			if change.NewLines > 0 {
				span.New = &model.Range{Start: change.NewStart, Lines: change.NewLines}
			}
			fragment.Ranges = []model.FragmentRange{span}
		} else {
			fragment.FileMetadata = true
		}
		result = append(result, fragment)
	}
	for path := range metadataPaths {
		merged := false
		for index := range result {
			if result[index].Path == path {
				result[index].FileMetadata = true
				merged = true
				break
			}
		}
		if merged {
			continue
		}
		for _, change := range inv.Changes {
			if change.Path == path {
				result = append(result, model.Fragment{ID: change.ID, Path: path, FileMetadata: true})
				break
			}
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Path == result[j].Path {
			return result[i].ID < result[j].ID
		}
		return result[i].Path < result[j].Path
	})
	return result
}

// Materialize renders range-defined fragments back into ordinary diff
// fragments for show and the viewer.
func Materialize(inv model.ChangeMap, definitions []model.Fragment) model.FragmentSet {
	result := model.FragmentSet{BaseSHA: inv.BaseSHA, HeadSHA: inv.HeadSHA}
	for _, definition := range definitions {
		fragment := model.MaterializedFragment{
			ID: definition.ID, BaseSHA: inv.BaseSHA, HeadSHA: inv.HeadSHA, Path: definition.Path,
		}
		setFragmentBounds(&fragment, definition.Ranges)
		var header string
		var hunks []string
		for _, change := range inv.Changes {
			if change.Path != definition.Path {
				continue
			}
			fileHeader, hunk := splitPatch(change.Patch)
			if header == "" {
				header = fileHeader
			}
			if change.Metadata || change.OldLines == 0 && change.NewLines == 0 {
				if definition.FileMetadata {
					fragment.Patch = change.Patch
				}
				continue
			}
			hunks = append(hunks, selectHunk(hunk, definition.Ranges)...)
		}
		if len(hunks) > 0 {
			fragment.Patch = header + strings.Join(hunks, "")
		}
		result.Fragments = append(result.Fragments, fragment)
	}
	return result
}

func setFragmentBounds(fragment *model.MaterializedFragment, ranges []model.FragmentRange) {
	oldStart, oldEnd, newStart, newEnd := 0, 0, 0, 0
	for _, span := range ranges {
		if span.Old != nil {
			if oldStart == 0 || span.Old.Start < oldStart {
				oldStart = span.Old.Start
			}
			oldEnd = max(oldEnd, span.Old.Start+span.Old.Lines)
		}
		if span.New != nil {
			if newStart == 0 || span.New.Start < newStart {
				newStart = span.New.Start
			}
			newEnd = max(newEnd, span.New.Start+span.New.Lines)
		}
	}
	fragment.OldStart, fragment.NewStart = oldStart, newStart
	if oldStart > 0 {
		fragment.OldLines = oldEnd - oldStart
	}
	if newStart > 0 {
		fragment.NewLines = newEnd - newStart
	}
}

func splitPatch(patch string) (string, string) {
	lines := strings.SplitAfter(patch, "\n")
	for index, line := range lines {
		if strings.HasPrefix(line, "@@ ") {
			return strings.Join(lines[:index], ""), strings.Join(lines[index:], "")
		}
	}
	return patch, ""
}

func selected(line int, ranges []model.FragmentRange, old bool) bool {
	for _, span := range ranges {
		candidate := span.New
		if old {
			candidate = span.Old
		}
		if candidate != nil && line >= candidate.Start && line < candidate.Start+candidate.Lines {
			return true
		}
	}
	return false
}

func selectHunk(hunk string, ranges []model.FragmentRange) []string {
	lines := strings.Split(strings.TrimSuffix(hunk, "\n"), "\n")
	if len(lines) == 0 {
		return nil
	}
	m := hunkRE.FindStringSubmatch(lines[0])
	if m == nil {
		return nil
	}
	oldLine, _ := strconv.Atoi(m[1])
	newLine, _ := strconv.Atoi(m[3])
	type segment struct {
		oldStart, newStart, oldLines, newLines int
		lines                                  []string
	}
	var segments []segment
	var current *segment
	flush := func() {
		if current != nil {
			segments = append(segments, *current)
			current = nil
		}
	}
	for _, line := range lines[1:] {
		if strings.HasPrefix(line, "\\") {
			if current != nil {
				current.lines = append(current.lines, line)
			}
			continue
		}
		isOld := strings.HasPrefix(line, "-")
		isNew := strings.HasPrefix(line, "+")
		isContext := strings.HasPrefix(line, " ")
		keep := (isOld && selected(oldLine, ranges, true)) || (isNew && selected(newLine, ranges, false))
		if !keep {
			flush()
		} else {
			if current == nil {
				current = &segment{oldStart: oldLine, newStart: newLine}
			}
			current.lines = append(current.lines, line)
			if isOld {
				current.oldLines++
			}
			if isNew {
				current.newLines++
			}
		}
		if isOld {
			oldLine++
		}
		if isNew {
			newLine++
		}
		if isContext {
			oldLine++
			newLine++
		}
	}
	flush()
	result := make([]string, 0, len(segments))
	for _, part := range segments {
		result = append(result, fmt.Sprintf("@@ -%d,%d +%d,%d @@\n%s\n", part.oldStart, part.oldLines, part.newStart, part.newLines, strings.Join(part.lines, "\n")))
	}
	return result
}

// FileContents loads the head version of each path, falling back to the base
// version for deleted files. The viewer uses these lines to expand context
// around each fragment without changing the fragment itself.
func (r Runner) FileContents(ctx context.Context, baseSHA, headSHA string, paths []string) map[string]string {
	contents := make(map[string]string, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		if seen[path] {
			continue
		}
		seen[path] = true
		b, err := r.git(ctx, "show", headSHA+":"+path)
		if err != nil {
			b, err = r.git(ctx, "show", baseSHA+":"+path)
		}
		if err == nil {
			contents[path] = string(b)
		}
	}
	return contents
}

func decodePath(s string) string {
	s = strings.TrimSpace(s)
	if s == "/dev/null" {
		return ""
	}
	if strings.HasPrefix(s, "\"") {
		if v, err := strconv.Unquote(s); err == nil {
			s = v
		}
	}
	if len(s) > 2 && s[1] == '/' {
		s = s[2:]
	}
	return s
}

func diffHeaderPath(line string) string {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "diff --git "))
	var tokens []string
	for rest != "" && len(tokens) < 2 {
		if rest[0] != '"' {
			fields := strings.Fields(rest)
			if len(fields) == 0 {
				break
			}
			tokens = append(tokens, fields[0])
			rest = strings.TrimSpace(strings.TrimPrefix(rest, fields[0]))
			continue
		}
		end, escaped := 1, false
		for end < len(rest) {
			if rest[end] == '"' && !escaped {
				end++
				break
			}
			if rest[end] == '\\' && !escaped {
				escaped = true
			} else {
				escaped = false
			}
			end++
		}
		tokens = append(tokens, rest[:end])
		rest = strings.TrimSpace(rest[end:])
	}
	if len(tokens) != 2 {
		return ""
	}
	return decodePath(tokens[1])
}

func ParseUnified(data []byte, baseSHA, headSHA string) ([]model.DiffChange, error) {
	type fileDiff struct {
		path   string
		header []string
		hunks  [][]string
	}
	var files []fileDiff
	var cur *fileDiff
	var hunk []string
	flushHunk := func() {
		if cur != nil && hunk != nil {
			cur.hunks = append(cur.hunks, hunk)
			hunk = nil
		}
	}
	flushFile := func() {
		flushHunk()
		if cur != nil {
			files = append(files, *cur)
			cur = nil
		}
	}
	s := bufio.NewScanner(bytes.NewReader(data))
	s.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "diff --git ") {
			flushFile()
			cur = &fileDiff{path: diffHeaderPath(line)}
			cur.header = append(cur.header, line)
			continue
		}
		if cur == nil {
			continue
		}
		if strings.HasPrefix(line, "@@ ") {
			flushHunk()
			hunk = []string{line}
			continue
		}
		if hunk != nil {
			hunk = append(hunk, line)
		} else {
			cur.header = append(cur.header, line)
		}
		if strings.HasPrefix(line, "+++ ") {
			p := decodePath(strings.TrimPrefix(line, "+++ "))
			if p != "" {
				cur.path = p
			}
		}
		if strings.HasPrefix(line, "--- ") && cur.path == "" {
			p := decodePath(strings.TrimPrefix(line, "--- "))
			if p != "" {
				cur.path = p
			}
		}
		if strings.HasPrefix(line, "rename to ") {
			cur.path = strings.TrimPrefix(line, "rename to ")
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	flushFile()
	var out []model.DiffChange
	for _, f := range files {
		metadata := hasFileMetadata(f.header)
		if len(f.hunks) == 0 {
			metadata = true
		}
		for _, h := range f.hunks {
			oldStart, oldLines, newStart, newLines := 0, 0, 0, 0
			if len(h) > 0 {
				m := hunkRE.FindStringSubmatch(h[0])
				if m == nil {
					return nil, fmt.Errorf("invalid hunk header %q", h[0])
				}
				oldStart, _ = strconv.Atoi(m[1])
				oldLines = 1
				if m[2] != "" {
					oldLines, _ = strconv.Atoi(m[2])
				}
				newStart, _ = strconv.Atoi(m[3])
				newLines = 1
				if m[4] != "" {
					newLines, _ = strconv.Atoi(m[4])
				}
			}
			patchLines := append(append([]string{}, f.header...), h...)
			patch := strings.Join(patchLines, "\n") + "\n"
			hash := sha256.Sum256([]byte(baseSHA + "\x00" + headSHA + "\x00" + f.path + "\x00" + fmt.Sprint(oldStart, oldLines, newStart, newLines) + "\x00" + patch))
			out = append(out, model.DiffChange{ID: "F-" + hex.EncodeToString(hash[:])[:12], BaseSHA: baseSHA, HeadSHA: headSHA, Path: f.path, OldStart: oldStart, OldLines: oldLines, NewStart: newStart, NewLines: newLines, Patch: patch})
		}
		if metadata {
			patch := strings.Join(f.header, "\n") + "\n"
			hash := sha256.Sum256([]byte(baseSHA + "\x00" + headSHA + "\x00" + f.path + "\x00metadata\x00" + patch))
			out = append(out, model.DiffChange{ID: "F-" + hex.EncodeToString(hash[:])[:12], BaseSHA: baseSHA, HeadSHA: headSHA, Path: f.path, Metadata: true, Patch: patch})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return out[i].NewStart < out[j].NewStart
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}

func hasFileMetadata(header []string) bool {
	for _, line := range header {
		for _, prefix := range []string{"new file mode ", "deleted file mode ", "old mode ", "new mode ", "rename from ", "rename to ", "Binary files ", "GIT binary patch"} {
			if strings.HasPrefix(line, prefix) {
				return true
			}
		}
	}
	return false
}
