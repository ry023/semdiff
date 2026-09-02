package reviews

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ry023/semdiff/internal/config"
	"github.com/ry023/semdiff/internal/model"
)

const cacheRef = "refs/semdiff/reviews-cache"

type Store struct {
	Dir    string
	Config config.ReviewStore
}

type Entry struct {
	Path    string
	BaseSHA string
	HeadSHA string
}

func Path(baseSHA, headSHA string) string { return baseSHA + "..." + headSHA + "/groups.json" }

func LocalPath(baseSHA, headSHA string) string {
	return filepath.Join(".semdiff", "reviews", baseSHA+"..."+headSHA, "groups.json")
}

func (s Store) endpoint() string {
	if s.Config.Repository != "" {
		return s.Config.Repository
	}
	return s.Config.Remote
}

func (s Store) git(ctx context.Context, env []string, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, "git", append([]string{"-C", s.Dir}, args...)...)
	c.Env = append(os.Environ(), env...)
	b, err := c.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(b)))
	}
	return b, nil
}

// Fetch copies only the configured artifact branch into a private local ref;
// it never checks out or changes the caller's working tree.
func (s Store) Fetch(ctx context.Context) error {
	_, err := s.git(ctx, nil, "fetch", "--quiet", s.endpoint(), "refs/heads/"+s.Config.Branch+":"+cacheRef)
	return err
}

func (s Store) List(ctx context.Context) ([]Entry, error) {
	// ls-tree otherwise limits its output to s.Dir's path within the worktree.
	// Reviews are stored at the artifact tree root, so viewing from a repository
	// subdirectory would incorrectly produce an empty index.
	b, err := s.git(ctx, nil, "ls-tree", "-r", "--full-tree", "--name-only", cacheRef)
	if err != nil {
		return nil, err
	}
	var result []Entry
	for _, path := range strings.Fields(string(b)) {
		base, head, ok := artifactPath(path)
		if !ok {
			continue
		}
		result = append(result, Entry{Path: path, BaseSHA: base, HeadSHA: head})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path > result[j].Path })
	return result, nil
}

func (s Store) Read(ctx context.Context, path string) ([]byte, error) {
	if _, _, ok := artifactPath(path); !ok {
		return nil, fmt.Errorf("invalid review path")
	}
	return s.git(ctx, nil, "show", cacheRef+":"+path)
}

func artifactPath(path string) (string, string, bool) {
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "groups.json" {
		return "", "", false
	}
	shas := strings.Split(parts[0], "...")
	if len(shas) != 2 || len(shas[0]) != 40 || len(shas[1]) != 40 || !hexSHA(shas[0]) || !hexSHA(shas[1]) {
		return "", "", false
	}
	return shas[0], shas[1], true
}

func hexSHA(value string) bool {
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func (s Store) Publish(ctx context.Context, source string, groups model.GroupsFile) (string, error) {
	b, err := os.ReadFile(source)
	if err != nil {
		return "", err
	}
	path := Path(groups.BaseSHA, groups.HeadSHA)
	// A missing branch is normal on the first publish. Other fetch failures are
	// surfaced so an authentication error never silently replaces history.
	if err := s.Fetch(ctx); err != nil {
		out, remoteErr := s.git(ctx, nil, "ls-remote", "--heads", s.endpoint(), "refs/heads/"+s.Config.Branch)
		if remoteErr != nil || len(bytes.TrimSpace(out)) != 0 {
			return "", err
		}
	}
	gitPath, err := s.git(ctx, nil, "rev-parse", "--git-path", "semdiff-review-index")
	if err != nil {
		return "", err
	}
	index := strings.TrimSpace(string(gitPath))
	if !filepath.IsAbs(index) {
		index = filepath.Join(s.Dir, index)
	}
	defer os.Remove(index)
	if _, err := s.git(ctx, []string{"GIT_INDEX_FILE=" + index}, "read-tree", "--empty"); err != nil {
		return "", err
	}
	if _, err := s.git(ctx, []string{"GIT_INDEX_FILE=" + index}, "rev-parse", "--verify", cacheRef+"^{commit}"); err == nil {
		if _, err := s.git(ctx, []string{"GIT_INDEX_FILE=" + index}, "read-tree", cacheRef); err != nil {
			return "", err
		}
	}
	// hash-object reads standard input; use a dedicated command for the source.
	c := exec.CommandContext(ctx, "git", "-C", s.Dir, "hash-object", "-w", "--stdin")
	c.Stdin = bytes.NewReader(b)
	blob, err := c.Output()
	if err != nil {
		return "", fmt.Errorf("write review artifact: %w", err)
	}
	if _, err := s.git(ctx, []string{"GIT_INDEX_FILE=" + index}, "update-index", "--add", "--cacheinfo", "100644,"+strings.TrimSpace(string(blob))+","+path); err != nil {
		return "", err
	}
	tree, err := s.git(ctx, []string{"GIT_INDEX_FILE=" + index}, "write-tree")
	if err != nil {
		return "", err
	}
	args := []string{"commit-tree", strings.TrimSpace(string(tree)), "-m", "semdiff review " + groups.BaseSHA[:12] + "..." + groups.HeadSHA[:12]}
	if _, err := s.git(ctx, nil, "rev-parse", "--verify", cacheRef+"^{commit}"); err == nil {
		args = append(args, "-p", cacheRef)
	}
	commit, err := s.git(ctx, nil, args...)
	if err != nil {
		return "", err
	}
	if _, err := s.git(ctx, nil, "push", s.endpoint(), strings.TrimSpace(string(commit))+":refs/heads/"+s.Config.Branch); err != nil {
		return "", err
	}
	_, _ = s.git(ctx, nil, "update-ref", cacheRef, strings.TrimSpace(string(commit)))
	return path, nil
}
