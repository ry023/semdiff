package gitdiff

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
	if len(got) != 5 {
		t.Fatalf("got %d fragments, want 5", len(got))
	}
	wantPaths := []string{"a.go", "a.go", "after.txt", "new.txt", "old.txt"}
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
	if got[2].OldLines != 0 || got[2].NewLines != 0 {
		t.Fatalf("rename should be metadata-only: %+v", got[2])
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
	inv, err := r.Fragments(context.Background(), base+"..HEAD")
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, f := range inv.Fragments {
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
