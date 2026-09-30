package reviews

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ry023/semdiff/internal/config"
	"github.com/ry023/semdiff/internal/gitdiff"
	"github.com/ry023/semdiff/internal/model"
)

func TestResolveRemote(t *testing.T) {
	ctx := context.Background()
	repo, remote := t.TempDir(), t.TempDir()
	git := func(dir string, args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git(repo, "init", "-q")
	git(remote, "init", "--bare", "-q")
	git(repo, "config", "user.name", "Test")
	git(repo, "config", "user.email", "test@example.com")
	count := 0
	commit := func() string {
		count++
		git(repo, "commit", "--allow-empty", "-qm", fmt.Sprint("commit ", count))
		return git(repo, "rev-parse", "HEAD")
	}
	base, old, near, head := commit(), commit(), commit(), commit()
	runner := gitdiff.Runner{Dir: repo}
	store := Store{Dir: repo, Config: config.ReviewStore{Repository: remote, Branch: "semdiff/reviews"}}
	publish := func(b, h, data string) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "groups.json")
		if data == "" {
			data = `{"format":"semdiff.groups","format_version":"1.0.0","base_sha":"` + b + `","head_sha":"` + h + `","groups":[]}`
		}
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Publish(ctx, path, model.GroupsFile{BaseSHA: b, HeadSHA: h}); err != nil {
			t.Fatal(err)
		}
	}
	check := func(exact bool, want string, behind int) {
		t.Helper()
		result, err := store.Resolve(ctx, runner, base, head, exact)
		if err != nil {
			t.Fatal(err)
		}
		if want == "" {
			if result != nil {
				t.Fatalf("unexpected match: %+v", result)
			}
			return
		}
		if result == nil || result.HeadSHA != want || result.CommitsBehind != behind {
			t.Fatalf("match = %+v, want %s behind %d", result, want, behind)
		}
	}
	check(false, "", 0) // first run, branch absent
	publish(base, old, "")
	publish(base, near, "")
	publish(old, head, "") // different base must not match
	git(repo, "checkout", "-q", "--detach", base)
	side := commit()
	publish(base, side, "") // not on the target first-parent history
	check(false, near, 1)
	check(true, "", 0)
	publish(base, head, "")
	check(false, head, 0)
	check(true, head, 0)
	for _, data := range []string{`{"version":3}`, `not json`, `{"format":"semdiff.groups","format_version":"1.0.0","base_sha":"` + old + `","head_sha":"` + head + `","groups":[]}`} {
		publish(base, head, data)
		if _, err := store.Resolve(ctx, runner, base, head, false); err == nil {
			t.Fatalf("accepted invalid artifact: %s", data)
		}
	}
	// Coverage gaps must fail before an artifact is returned.
	git(repo, "checkout", "-q", "--detach", head)
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git(repo, "add", "new.txt")
	changed := commit()
	publish(base, changed, "")
	if _, err := store.Resolve(ctx, runner, base, changed, false); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("coverage gap: %v", err)
	}
	// A missing branch must not reuse the previous fetch's cache ref.
	git(remote, "update-ref", "-d", "refs/heads/semdiff/reviews")
	check(false, "", 0)
	store.Config.Repository = filepath.Join(t.TempDir(), "missing.git")
	if _, err := store.Resolve(ctx, runner, base, head, false); err == nil {
		t.Fatal("repository failure treated as missing review")
	}
}
