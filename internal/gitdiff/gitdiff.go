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

func (r Runner) Fragments(ctx context.Context, rangeSpec string) (model.Inventory, error) {
	base, head, err := ParseRange(rangeSpec)
	if err != nil {
		return model.Inventory{}, err
	}
	baseSHA, err := r.Resolve(ctx, base)
	if err != nil {
		return model.Inventory{}, err
	}
	headSHA, err := r.Resolve(ctx, head)
	if err != nil {
		return model.Inventory{}, err
	}
	b, err := r.git(ctx, "diff", "--find-renames", "--no-color", "--unified=3", baseSHA, headSHA, "--")
	if err != nil {
		return model.Inventory{}, err
	}
	frags, err := ParseUnified(b, baseSHA, headSHA)
	if err != nil {
		return model.Inventory{}, err
	}
	return model.Inventory{BaseSHA: baseSHA, HeadSHA: headSHA, Fragments: frags}, nil
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

func ParseUnified(data []byte, baseSHA, headSHA string) ([]model.DiffFragment, error) {
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
			cur = &fileDiff{}
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
	var out []model.DiffFragment
	for _, f := range files {
		if len(f.hunks) == 0 {
			f.hunks = [][]string{{}}
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
			out = append(out, model.DiffFragment{ID: "F-" + hex.EncodeToString(hash[:])[:12], BaseSHA: baseSHA, HeadSHA: headSHA, Path: f.path, OldStart: oldStart, OldLines: oldLines, NewStart: newStart, NewLines: newLines, Patch: patch})
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
