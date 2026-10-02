package reviews

import (
	"bytes"
	"context"
	"crypto/sha256"
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

func (s Store) sourceGit(ctx context.Context, env []string, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, "git", append([]string{"-C", s.Dir}, args...)...)
	c.Env = append(os.Environ(), env...)
	b, err := c.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(b)))
	}
	return b, nil
}

func (s Store) branchExists(ctx context.Context) (bool, error) {
	dir, fetchURL, _, err := s.cache(ctx)
	if err != nil {
		return false, err
	}
	out, err := runGit(ctx, dir, nil, "ls-remote", "--heads", fetchURL, "refs/heads/"+s.Config.Branch)
	if err != nil {
		return false, err
	}
	return len(bytes.TrimSpace(out)) != 0, nil
}

func runGit(ctx context.Context, dir string, env []string, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, "git", append([]string{"--git-dir", dir}, args...)...)
	c.Env = append(os.Environ(), env...)
	b, err := c.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(b)))
	}
	return b, nil
}

func (s Store) endpoints(ctx context.Context) (string, string, error) {
	if s.Config.Repository != "" {
		endpoint := s.Config.Repository
		if !filepath.IsAbs(endpoint) && !strings.Contains(endpoint, "://") && !strings.Contains(endpoint, ":") {
			endpoint = filepath.Join(s.Dir, endpoint)
		}
		return endpoint, endpoint, nil
	}
	fetchURL, err := s.sourceGit(ctx, nil, "remote", "get-url", s.Config.Remote)
	if err != nil {
		return "", "", err
	}
	pushURL, err := s.sourceGit(ctx, nil, "remote", "get-url", "--push", s.Config.Remote)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(string(fetchURL)), strings.TrimSpace(string(pushURL)), nil
}

func (s Store) cache(ctx context.Context) (string, string, string, error) {
	fetchURL, pushURL, err := s.endpoints(ctx)
	if err != nil {
		return "", "", "", err
	}
	root, err := s.sourceGit(ctx, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", "", err
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(fetchURL+"\x00"+pushURL+"\x00"+s.Config.Branch)))
	dir := filepath.Join(strings.TrimSpace(string(root)), ".semdiff", "cache", "review-stores", key, "repo.git")
	if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		return "", "", "", err
	}
	if _, err := os.Stat(filepath.Join(dir, "HEAD")); os.IsNotExist(err) {
		if out, initErr := exec.CommandContext(ctx, "git", "init", "--bare", "--quiet", dir).CombinedOutput(); initErr != nil {
			return "", "", "", fmt.Errorf("git init --bare: %w: %s", initErr, strings.TrimSpace(string(out)))
		}
	} else if err != nil {
		return "", "", "", err
	}
	// Preserve the effective identity used by commit-tree in the source repository.
	for _, name := range []string{"user.name", "user.email"} {
		if value, configErr := s.sourceGit(ctx, nil, "config", "--get", name); configErr == nil {
			if _, configErr = runGit(ctx, dir, nil, "config", name, strings.TrimSpace(string(value))); configErr != nil {
				return "", "", "", configErr
			}
		}
	}
	return dir, fetchURL, pushURL, nil
}

// Fetch copies only the configured artifact branch into a private local ref;
// it never checks out or changes the caller's working tree.
func (s Store) Fetch(ctx context.Context) error {
	dir, fetchURL, _, err := s.cache(ctx)
	if err != nil {
		return err
	}
	// Never retain a branch from an earlier fetch when the remote branch was deleted.
	_, _ = runGit(ctx, dir, nil, "update-ref", "-d", cacheRef)
	_, err = runGit(ctx, dir, nil, "fetch", "--quiet", fetchURL, "refs/heads/"+s.Config.Branch+":"+cacheRef)
	return err
}

func (s Store) List(ctx context.Context) ([]Entry, error) {
	// ls-tree otherwise limits its output to s.Dir's path within the worktree.
	// Reviews are stored at the artifact tree root, so viewing from a repository
	// subdirectory would incorrectly produce an empty index.
	dir, _, _, err := s.cache(ctx)
	if err != nil {
		return nil, err
	}
	b, err := runGit(ctx, dir, nil, "ls-tree", "-r", "--full-tree", "--name-only", cacheRef)
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
	dir, _, _, err := s.cache(ctx)
	if err != nil {
		return nil, err
	}
	return runGit(ctx, dir, nil, "show", cacheRef+":"+path)
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
	dir, _, pushURL, err := s.cache(ctx)
	if err != nil {
		return "", err
	}
	// A missing branch is normal on the first publish. Other fetch failures are
	// surfaced so an authentication error never silently replaces history.
	if err := s.Fetch(ctx); err != nil {
		exists, remoteErr := s.branchExists(ctx)
		if remoteErr != nil || exists {
			return "", err
		}
	}
	indexFile, err := os.CreateTemp(filepath.Dir(dir), "index-*")
	if err != nil {
		return "", err
	}
	index := indexFile.Name()
	if err := indexFile.Close(); err != nil {
		return "", err
	}
	// Git expects a missing index rather than an empty file.
	if err := os.Remove(index); err != nil {
		return "", err
	}
	defer os.Remove(index)
	if _, err := runGit(ctx, dir, []string{"GIT_INDEX_FILE=" + index}, "read-tree", "--empty"); err != nil {
		return "", err
	}
	if _, err := runGit(ctx, dir, []string{"GIT_INDEX_FILE=" + index}, "rev-parse", "--verify", cacheRef+"^{commit}"); err == nil {
		if _, err := runGit(ctx, dir, []string{"GIT_INDEX_FILE=" + index}, "read-tree", cacheRef); err != nil {
			return "", err
		}
	}
	// hash-object reads standard input; use a dedicated command for the source.
	c := exec.CommandContext(ctx, "git", "--git-dir", dir, "hash-object", "-w", "--stdin")
	c.Stdin = bytes.NewReader(b)
	blob, err := c.Output()
	if err != nil {
		return "", fmt.Errorf("write review artifact: %w", err)
	}
	if _, err := runGit(ctx, dir, []string{"GIT_INDEX_FILE=" + index}, "update-index", "--add", "--cacheinfo", "100644,"+strings.TrimSpace(string(blob))+","+path); err != nil {
		return "", err
	}
	tree, err := runGit(ctx, dir, []string{"GIT_INDEX_FILE=" + index}, "write-tree")
	if err != nil {
		return "", err
	}
	args := []string{"commit-tree", strings.TrimSpace(string(tree)), "-m", "semdiff review " + groups.BaseSHA[:12] + "..." + groups.HeadSHA[:12]}
	if _, err := runGit(ctx, dir, nil, "rev-parse", "--verify", cacheRef+"^{commit}"); err == nil {
		args = append(args, "-p", cacheRef)
	}
	commit, err := runGit(ctx, dir, nil, args...)
	if err != nil {
		return "", err
	}
	if _, err := runGit(ctx, dir, nil, "push", pushURL, strings.TrimSpace(string(commit))+":refs/heads/"+s.Config.Branch); err != nil {
		return "", err
	}
	_, _ = runGit(ctx, dir, nil, "update-ref", cacheRef, strings.TrimSpace(string(commit)))
	return path, nil
}
