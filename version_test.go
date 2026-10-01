package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/ry023/semdiff/internal/groups"
	"github.com/ry023/semdiff/internal/versioning"
)

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prior := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = prior }()
	runErr := fn()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	return string(output), runErr
}

func TestVersionCommands(t *testing.T) {
	t.Setenv("LANG", "C")
	expectedVersion := "semdiff " + productVersion()
	short, err := captureStdout(t, func() error { return run(context.Background(), []string{"--version"}) })
	if err != nil {
		t.Fatal(err)
	}
	if short != expectedVersion+"\n" {
		t.Fatalf("--version output = %q", short)
	}

	text, err := captureStdout(t, func() error { return run(context.Background(), []string{"version"}) })
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{expectedVersion, "groups schema read: >=1.0.0, <1.1.0", "groups schema write: semdiff.groups 1.0.0"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("version output is missing %q: %s", expected, text)
		}
	}

	raw, err := captureStdout(t, func() error { return run(context.Background(), []string{"version", "--json"}) })
	if err != nil {
		t.Fatal(err)
	}
	var output versionOutput
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		t.Fatal(err)
	}
	if output.Version != productVersion() || output.GroupsSchema.Format != groups.Format || output.GroupsSchema.Write != groups.WriteVersion || len(output.GroupsSchema.ReadRanges) != 1 {
		t.Fatalf("unexpected JSON version output: %+v", output)
	}
}

func TestProductAndPluginVersionsStayInSync(t *testing.T) {
	type manifest struct {
		Version string `json:"version"`
		Plugins []struct {
			Version string `json:"version"`
		} `json:"plugins"`
	}
	for _, path := range []string{".codex-plugin/plugin.json", ".claude-plugin/plugin.json", ".claude-plugin/marketplace.json"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var parsed manifest
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		actual := parsed.Version
		if len(parsed.Plugins) == 1 {
			actual = parsed.Plugins[0].Version
		}
		if actual != productVersion() {
			t.Fatalf("%s version = %q, want %q", path, actual, productVersion())
		}
	}
}

func TestBundledSkillsCheckCLICompatibility(t *testing.T) {
	version, err := versioning.Parse(productVersion())
	if err != nil {
		t.Fatal(err)
	}
	wantRange := fmt.Sprintf(">=%d.%d.0, <%d.%d.0", version.Major, version.Minor, version.Major, version.Minor+1)
	wantPluginLine := fmt.Sprintf("plugin %d.%d.x", version.Major, version.Minor)
	ranges := regexp.MustCompile(`>=\d+\.\d+\.\d+, <\d+\.\d+\.\d+`)
	pluginLines := regexp.MustCompile(`plugin \d+\.\d+\.x`)
	for _, path := range []string{"skills/semantic-grouping/SKILL.md", "skills/answer-semdiff/SKILL.md"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		body := string(data)
		if !strings.Contains(body, "semdiff version --json") {
			t.Fatalf("%s is missing CLI version check", path)
		}
		if actual := ranges.FindAllString(body, -1); len(actual) != 1 || actual[0] != wantRange {
			t.Fatalf("%s CLI ranges = %q, want [%q]", path, actual, wantRange)
		}
		if actual := pluginLines.FindAllString(body, -1); len(actual) != 1 || actual[0] != wantPluginLine {
			t.Fatalf("%s plugin lines = %q, want [%q]", path, actual, wantPluginLine)
		}
	}
}
