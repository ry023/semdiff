package gitdiff

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ry023/semdiff/internal/model"
)

func TestDefaultRangeUsesDefaultBranchMergeBase(t *testing.T) {
	repo, commits := defaultRangeRepository(t)
	installFakeGH(t, "exit 1\n")

	got, err := (Runner{Dir: repo}).DefaultRange(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := commits["root"] + ".." + commits["feature"]
	if got != want {
		t.Fatalf("DefaultRange() = %q, want %q", got, want)
	}
}

func TestDefaultRangeUsesPullRequestBase(t *testing.T) {
	repo, commits := defaultRangeRepository(t)
	installFakeGH(t, fmt.Sprintf("printf '%%s\\n' '{\"baseRefName\":\"release\",\"baseRefOid\":\"%s\"}'\n", commits["release"]))

	got, err := (Runner{Dir: repo}).DefaultRange(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := commits["release"] + ".." + commits["feature"]
	if got != want {
		t.Fatalf("DefaultRange() = %q, want %q", got, want)
	}
}

func defaultRangeRepository(t *testing.T) (string, map[string]string) {
	t.Helper()
	repo := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", repo}, args...)...)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	commit := func(name string) string {
		t.Helper()
		path := filepath.Join(repo, name+".txt")
		if err := os.WriteFile(path, []byte(name+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		run("add", ".")
		run("commit", "-qm", name)
		return run("rev-parse", "HEAD")
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	commits := map[string]string{"root": commit("root")}
	run("switch", "-qc", "release")
	commits["release"] = commit("release")
	run("switch", "-qc", "feature")
	commits["feature"] = commit("feature")
	run("switch", "-q", "main")
	commit("main")
	run("update-ref", "refs/remotes/origin/main", "main")
	run("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	run("switch", "-q", "feature")
	run("config", "branch.feature.remote", "origin")
	return repo, commits
}

func installFakeGH(t *testing.T, body string) {
	t.Helper()
	bin := t.TempDir()
	path := filepath.Join(bin, "gh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestParseUnified(t *testing.T) {
	diff := `diff --git a/a.go b/a.go
index 111..222 100644
--- a/a.go
+++ b/a.go
@@ -1,2 +1,2 @@
-old
+new
 keep
@@ -10 +10,2 @@
 x
+y
diff --git a/new.txt b/new.txt
new file mode 100644
--- /dev/null
+++ b/new.txt
@@ -0,0 +1 @@
+hello
diff --git a/old.txt b/old.txt
deleted file mode 100644
--- a/old.txt
+++ /dev/null
@@ -1 +0,0 @@
-bye
diff --git a/before.txt b/after.txt
similarity index 100%
rename from before.txt
rename to after.txt
`
	got, err := ParseUnified([]byte(diff), "base", "head")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 7 {
		t.Fatalf("got %d changes, want 7", len(got))
	}
	wantPaths := []string{"a.go", "a.go", "after.txt", "new.txt", "new.txt", "old.txt", "old.txt"}
	for i, want := range wantPaths {
		if got[i].Path != want {
			t.Errorf("fragment %d path=%q want %q", i, got[i].Path, want)
		}
		if got[i].ID == "" {
			t.Errorf("fragment %d has empty ID", i)
		}
	}
	if got[0].OldStart != 1 || got[1].NewStart != 10 {
		t.Fatalf("unexpected hunk ranges: %+v %+v", got[0], got[1])
	}
	if !got[2].Metadata {
		t.Fatalf("rename should be metadata-only: %+v", got[2])
	}
}

func TestMaterializeSplitsNewFileAndCombinesRanges(t *testing.T) {
	diff := "diff --git a/new.txt b/new.txt\nnew file mode 100644\n--- /dev/null\n+++ b/new.txt\n@@ -0,0 +1,6 @@\n+one\n+two\n+three\n+four\n+five\n+six\n"
	changes, err := ParseUnified([]byte(diff), "base", "head")
	if err != nil {
		t.Fatal(err)
	}
	definitions := []model.Fragment{
		{ID: "odd", Path: "new.txt", FileMetadata: true, Ranges: []model.FragmentRange{{New: &model.Range{Start: 1, Lines: 2}}, {New: &model.Range{Start: 5, Lines: 2}}}},
		{ID: "middle", Path: "new.txt", Ranges: []model.FragmentRange{{New: &model.Range{Start: 3, Lines: 2}}}},
	}
	materialized := Materialize(model.ChangeMap{BaseSHA: "base", HeadSHA: "head", Changes: changes}, definitions)
	if len(materialized.Fragments) != 2 {
		t.Fatal(materialized.Fragments)
	}
	if strings.Contains(materialized.Fragments[0].Patch, "+three") || !strings.Contains(materialized.Fragments[0].Patch, "+six") {
		t.Fatalf("unexpected selected patch: %s", materialized.Fragments[0].Patch)
	}
	if !strings.Contains(materialized.Fragments[1].Patch, "+three") || strings.Contains(materialized.Fragments[1].Patch, "+one") {
		t.Fatalf("unexpected middle patch: %s", materialized.Fragments[1].Patch)
	}
}

func TestParseRange(t *testing.T) {
	if a, b, err := ParseRange("main..HEAD"); err != nil || a != "main" || b != "HEAD" {
		t.Fatalf("unexpected: %q %q %v", a, b, err)
	}
	for _, bad := range []string{"main", "main...HEAD", "..HEAD"} {
		if _, _, err := ParseRange(bad); err == nil {
			t.Errorf("ParseRange(%q) succeeded", bad)
		}
	}
}

func TestParseBinaryAndQuotedPath(t *testing.T) {
	diff := "diff --git \"a/image file.png\" \"b/image file.png\"\nnew file mode 100644\nindex 000..111\nBinary files /dev/null and b/image file.png differ\n"
	changes, err := ParseUnified([]byte(diff), "base", "head")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Path != "image file.png" || !changes[0].Metadata {
		t.Fatalf("unexpected binary change: %+v", changes)
	}
}

func TestRunnerAcrossCommitsAndFileOperations(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if b, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, b)
		}
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	write("change.txt", "one\nkeep\nten\n")
	write("delete.txt", "gone\n")
	write("rename.txt", "same\n")
	run("add", ".")
	run("commit", "-qm", "base")
	baseOut, _ := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	base := string(baseOut[:len(baseOut)-1])
	write("change.txt", "ONE\nkeep\nTEN\n")
	write("new.txt", "new\n")
	run("rm", "-q", "delete.txt")
	run("mv", "rename.txt", "renamed.txt")
	run("add", ".")
	run("commit", "-qm", "mixed operations")
	write("second.txt", "another commit\n")
	run("add", ".")
	run("commit", "-qm", "second commit")
	r := Runner{Dir: dir}
	inv, err := r.Changes(context.Background(), base+"..HEAD")
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, f := range inv.Changes {
		paths[f.Path] = true
	}
	for _, want := range []string{"change.txt", "delete.txt", "new.txt", "renamed.txt", "second.txt"} {
		if !paths[want] {
			t.Errorf("missing fragment for %s; got %v", want, paths)
		}
	}
	commits, err := r.Commits(context.Background(), base+"..HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("got %d commits, want 2", len(commits))
	}
	if commits[0].FilesChanged != 4 {
		t.Errorf("first commit changed %d files, want 4", commits[0].FilesChanged)
	}
	contents := r.FileContents(context.Background(), base, "HEAD", []string{"change.txt", "delete.txt"})
	if !strings.Contains(contents["change.txt"], "keep") || !strings.Contains(contents["delete.txt"], "gone") {
		t.Errorf("file contents did not load head and deleted paths: %q %q", contents["change.txt"], contents["delete.txt"])
	}
}
