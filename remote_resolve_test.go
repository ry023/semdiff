package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ry023/semdiff/internal/config"
	"github.com/ry023/semdiff/internal/gitdiff"
	"github.com/ry023/semdiff/internal/model"
	"github.com/ry023/semdiff/internal/reviews"
)

func TestRemoteResolvePullsSelectedReviewOnlyWhenRequested(t *testing.T) {
	t.Setenv("LANG", "C")
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
	git(repo, "commit", "--allow-empty", "-qm", "base")
	base := git(repo, "rev-parse", "HEAD")
	git(repo, "commit", "--allow-empty", "-qm", "reviewed")
	head := git(repo, "rev-parse", "HEAD")
	source := filepath.Join(t.TempDir(), "groups.json")
	data := []byte(`{"format":"semdiff.groups","format_version":"1.0.0","base_sha":"` + base + `","head_sha":"` + head + `","groups":[]}`)
	if err := os.WriteFile(source, data, 0644); err != nil {
		t.Fatal(err)
	}
	store := reviews.Store{Dir: repo, Config: config.ReviewStore{Repository: remote, Branch: "semdiff/reviews"}}
	if _, err := store.Publish(ctx, source, model.GroupsFile{BaseSHA: base, HeadSHA: head}); err != nil {
		t.Fatal(err)
	}
	git(repo, "commit", "--allow-empty", "-qm", "next")
	local := localReviewPath(repo, base, head)
	if err := saveRemoteArtifact(local, []byte("local authored artifact")); err != nil {
		t.Fatal(err)
	}
	output, err := captureStdout(t, func() error {
		return runRemote(ctx, gitdiff.Runner{Dir: repo}, []string{"resolve", base + "..HEAD", "--repository", remote, "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var result reviewResolveOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Found || result.Exact || result.CommitsBehind != 1 || result.ReviewHeadSHA != head || result.GroupsPath != "" || result.RemotePath != reviews.Path(base, head) {
		t.Fatalf("unexpected output: %s", output)
	}
	original, err := os.ReadFile(local)
	if err != nil || string(original) != "local authored artifact" {
		t.Fatalf("local review changed: %s, %v", original, err)
	}
	if _, err := runRemoteResolveOutput(t, ctx, repo, base+"..HEAD", remote, "--pull", "--no-clobber"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("no-clobber should reject local file: %v", err)
	}
	if _, err := runRemoteResolveOutput(t, ctx, repo, base+"..HEAD", remote, "--pull"); err == nil || !(strings.Contains(err.Error(), "--force or --no-clobber") || strings.Contains(err.Error(), "上書きされていません") || strings.Contains(err.Error(), "pull cancelled")) {
		t.Fatalf("overwrite without approval should fail: %v", err)
	}
	for _, flag := range []string{"--force", "--no-clobber"} {
		if _, err := runRemoteResolveOutput(t, ctx, repo, base+"..HEAD", remote, flag); err == nil || !strings.Contains(err.Error(), "require --pull") {
			t.Fatalf("%s without --pull: %v", flag, err)
		}
	}
	if _, err := runRemoteResolveOutput(t, ctx, repo, base+"..HEAD", remote, "--pull", "--force", "--no-clobber"); err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("conflicting flags: %v", err)
	}
	output, err = runRemoteResolveOutput(t, ctx, repo, base+"..HEAD", remote, "--pull", "--force")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if result.GroupsPath != local {
		t.Fatalf("pulled path = %q, want %q", result.GroupsPath, local)
	}
	pulled, err := os.ReadFile(local)
	if err != nil || string(pulled) != string(data) {
		t.Fatalf("pulled artifact: %s, %v", pulled, err)
	}
	if err := os.Remove(local); err != nil {
		t.Fatal(err)
	}
	output, err = runRemoteResolveOutput(t, ctx, repo, base+"..HEAD", remote, "--pull")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil || result.GroupsPath != local {
		t.Fatalf("fresh pull: %s, %v", output, err)
	}
	output, err = captureStdout(t, func() error {
		return runRemote(ctx, gitdiff.Runner{Dir: repo}, []string{"resolve", base + "..HEAD", "--repository", remote, "--exact", "--json"})
	})
	if err != nil || !strings.Contains(output, `"found": false`) {
		t.Fatalf("exact output: %s, %v", output, err)
	}
}

func runRemoteResolveOutput(t *testing.T, ctx context.Context, repo, rangeSpec, remote string, flags ...string) (string, error) {
	t.Helper()
	args := append([]string{"resolve", rangeSpec, "--repository", remote, "--json"}, flags...)
	return captureStdout(t, func() error { return runRemote(ctx, gitdiff.Runner{Dir: repo}, args) })
}
