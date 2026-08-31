package reviews

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ry023/semdiff/internal/config"
	"github.com/ry023/semdiff/internal/model"
)

func TestPublishCreatesAndPreservesArtifactBranch(t *testing.T) {
	repo, remote := t.TempDir(), filepath.Join(t.TempDir(), "artifacts.git")
	git := func(dir string, args ...string) string {
		t.Helper()
		c := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git(repo, "init", "-q")
	git(repo, "config", "user.name", "Test")
	git(repo, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(repo, "file"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	git(repo, "add", ".")
	git(repo, "commit", "-qm", "base")
	if out, err := exec.Command("git", "init", "--bare", "-q", remote).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v: %s", err, out)
	}
	store := Store{Dir: repo, Config: config.ReviewStore{Repository: remote, Branch: "semdiff/reviews"}}
	first := model.GroupsFile{BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}
	firstPath := filepath.Join(repo, "first.json")
	if err := os.WriteFile(firstPath, []byte(`{"version":2}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish(context.Background(), firstPath, first); err != nil {
		t.Fatal(err)
	}
	second := model.GroupsFile{BaseSHA: strings.Repeat("c", 40), HeadSHA: strings.Repeat("d", 40)}
	secondPath := filepath.Join(repo, "second.json")
	if err := os.WriteFile(secondPath, []byte(`{"version":2,"groups":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish(context.Background(), secondPath, second); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(context.Background(), Path(first.BaseSHA, first.HeadSHA)); err != nil {
		t.Fatalf("read published artifact: %v", err)
	}
	tree := git(remote, "ls-tree", "-r", "--name-only", "semdiff/reviews")
	if !strings.Contains(tree, Path(first.BaseSHA, first.HeadSHA)) || !strings.Contains(tree, Path(second.BaseSHA, second.HeadSHA)) {
		t.Fatalf("artifact tree missing published files: %s", tree)
	}
}
