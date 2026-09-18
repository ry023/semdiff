package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ry023/semdiff/internal/gitdiff"
	"github.com/ry023/semdiff/internal/groups"
	"github.com/ry023/semdiff/internal/model"
	"github.com/ry023/semdiff/internal/reviews"
)

func remoteFixture(t *testing.T) (string, string, string, string, gitdiff.Runner) {
	t.Helper()
	repo := t.TempDir()
	remote := filepath.Join(t.TempDir(), "artifacts.git")
	git := func(dir string, args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git(repo, "init", "-q")
	git(repo, "config", "user.email", "test@example.com")
	git(repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git(repo, "add", ".")
	git(repo, "commit", "-qm", "base")
	base := git(repo, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("new\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git(repo, "add", ".")
	git(repo, "commit", "-qm", "head")
	head := git(repo, "rev-parse", "HEAD")
	if out, err := exec.Command("git", "init", "--bare", "-q", remote).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v: %s", err, out)
	}
	runner := gitdiff.Runner{Dir: repo}
	changes, err := runner.Changes(context.Background(), base+".."+head)
	if err != nil {
		t.Fatal(err)
	}
	fragments := gitdiff.SuggestedFragments(changes)
	ids := make([]string, 0, len(fragments))
	for i := range fragments {
		fragments[i].Description = "Explains the change."
		fragments[i].ReviewLevel = model.ReviewLevelNormal
		ids = append(ids, fragments[i].ID)
	}
	g := groups.NewFile(base, head, []model.SemanticGroup{{
		ID: "logic", Title: "Logic", Summary: "Review the change.", Importance: model.ImportanceCore,
		FileCategories: []model.FileCategory{{Path: "file.txt", Category: "logic"}},
		ReviewSteps:    []model.ReviewStep{{ID: "logic", Title: "Review logic", Summary: "Read the changed file.", FragmentIDs: ids}},
		Fragments:      fragments,
	}})
	path := filepath.Join(repo, reviews.LocalPath(base, head))
	if err := saveJSONAtomic(path, g); err != nil {
		t.Fatal(err)
	}
	return repo, remote, base, head, runner
}

func inRepo(t *testing.T, repo string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
}

func TestRemotePushPullAndValidation(t *testing.T) {
	repo, remote, base, head, runner := remoteFixture(t)
	inRepo(t, repo)
	ctx := context.Background()
	rangeSpec := base + ".." + head
	path := reviews.LocalPath(base, head)
	if err := runRemotePush(ctx, runner, []string{rangeSpec, "--repository", remote}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	store, err := remoteStore("", remote, "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := remoteArtifact(ctx, runner, store, base, head)
	if err != nil {
		t.Fatal(err)
	}
	key := strings.TrimSuffix(reviews.Path(base, head), "/groups.json")
	request := httptest.NewRequest(http.MethodGet, "/review/"+key+"/", nil)
	response := httptest.NewRecorder()
	handler := reviewIndexHandler(ctx, runner, store)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "<title>Semantic Changes</title>") {
		t.Fatalf("remote review HTML: status=%d", response.Code)
	}
	indexResponse := httptest.NewRecorder()
	handler.ServeHTTP(indexResponse, httptest.NewRequest(http.MethodGet, "/", nil))
	if indexResponse.Code != http.StatusOK || !strings.Contains(indexResponse.Body.String(), key) {
		t.Fatalf("remote index HTML: status=%d", indexResponse.Code)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("view preparation saved a local review: %v", err)
	}
	if err := runRemotePull(ctx, runner, []string{rangeSpec, "--repository", remote}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("pulled file: %v", err)
	}
	if err := runRemotePull(ctx, runner, []string{rangeSpec, "--repository", remote, "--no-clobber"}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("no-clobber error = %v", err)
	}
	if err := runRemotePull(ctx, runner, []string{rangeSpec, "--repository", remote, "--force", "--no-clobber"}); err == nil {
		t.Fatal("conflicting flags should fail")
	}
	if err := os.WriteFile(path, []byte("local content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runRemotePull(ctx, runner, []string{rangeSpec, "--repository", remote, "--force"}); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("force did not restore remote artifact: %v", err)
	}
	if err := runRemotePull(ctx, runner, []string{base + "..missing-ref", "--repository", remote}); err == nil {
		t.Fatal("invalid range ref should fail")
	}
	if err := runRemotePush(ctx, runner, []string{rangeSpec, "--groups-file", path}); err == nil {
		t.Fatal("range and groups file should be exclusive")
	}
	if err := runRemotePush(ctx, runner, []string{"--groups-file", path, "--repository", remote}); err != nil {
		t.Fatal(err)
	}
	corrupt := filepath.Join(t.TempDir(), "groups.json")
	if err := os.WriteFile(corrupt, []byte(`{"version":3}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish(ctx, corrupt, model.GroupsFile{BaseSHA: base, HeadSHA: head}); err != nil {
		t.Fatal(err)
	}
	if err := runRemotePull(ctx, runner, []string{rangeSpec, "--repository", remote, "--force"}); err == nil {
		t.Fatal("invalid remote artifact should fail")
	}
	got, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("invalid remote artifact changed local file: %v", err)
	}
}

func TestConfirmOverwrite(t *testing.T) {
	for _, test := range []struct {
		name, answer        string
		terminal, wantError bool
	}{
		{"accept", "yes\n", true, false},
		{"reject", "n\n", true, true},
		{"default", "\n", true, true},
		{"noninteractive", "yes\n", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := confirmOverwrite("groups.json", strings.NewReader(test.answer), &output, test.terminal)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v", err)
			}
			if test.terminal && !strings.Contains(output.String(), "overwrite groups.json?") {
				t.Fatalf("prompt = %q", output.String())
			}
		})
	}
}

func TestDeprecatedResolveWarnsOnlyOnStderr(t *testing.T) {
	repo, _, base, head, _ := remoteFixture(t)
	inRepo(t, repo)
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout, oldStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdoutWriter, stderrWriter
	defer func() { os.Stdout, os.Stderr = oldStdout, oldStderr }()
	runErr := run(context.Background(), []string{"reviews", "resolve", base + ".." + head, "--json"})
	stdoutWriter.Close()
	stderrWriter.Close()
	stdout, _ := io.ReadAll(stdoutReader)
	stderr, _ := io.ReadAll(stderrReader)
	if runErr != nil {
		t.Fatal(runErr)
	}
	var result reviewResolveOutput
	if err := json.Unmarshal(stdout, &result); err != nil || !result.Found {
		t.Fatalf("JSON = %s, error = %v", stdout, err)
	}
	if !strings.Contains(string(stderr), "use semdiff resolve") {
		t.Fatalf("warning = %s", stderr)
	}
}

func TestDeprecatedCommandsAreHiddenAndWarn(t *testing.T) {
	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	defer func() { os.Stderr = oldStderr }()
	usage()
	if err := run(context.Background(), []string{"reviews", "view", "unexpected"}); err == nil {
		t.Fatal("legacy view should reject a positional argument")
	}
	if err := run(context.Background(), []string{"publish", "missing-groups.json"}); err == nil {
		t.Fatal("legacy publish should report a missing file")
	}
	writer.Close()
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	text := string(output)
	if strings.Contains(text, "\tsemdiff publish ") || strings.Contains(text, "\tsemdiff reviews ") {
		t.Fatalf("deprecated commands appear in help: %s", text)
	}
	for _, warning := range []string{"use semdiff remote view-index", "use semdiff remote push"} {
		if !strings.Contains(text, warning) {
			t.Fatalf("missing warning %q", warning)
		}
	}
}
