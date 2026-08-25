package categories

import (
	"path"
	"sort"
	"strings"
)

// Suggestion is a deterministic, path-only category draft. It is intended as
// input for a later semantic review, not as a definitive classification.
type Suggestion struct {
	Path     string `json:"path"`
	Category string `json:"category"`
}

// ClassifyPaths returns one stable suggestion for each non-empty path.
func ClassifyPaths(paths []string) []Suggestion {
	seen := make(map[string]bool, len(paths))
	unique := make([]string, 0, len(paths))
	for _, raw := range paths {
		p := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		unique = append(unique, p)
	}
	sort.Strings(unique)
	out := make([]Suggestion, 0, len(unique))
	for _, p := range unique {
		out = append(out, Suggestion{Path: p, Category: classifyPath(p)})
	}
	return out
}

func classifyPath(filePath string) string {
	normalized := strings.ToLower(strings.Trim(strings.ReplaceAll(filePath, "\\", "/"), "/"))
	base := path.Base(normalized)
	segments := strings.Split(normalized, "/")

	// Test naming is the strongest signal and must win over component or
	// language-extension rules (for example, Button.test.tsx).
	if isTestPath(base, segments) {
		return "test"
	}
	if isConfigPath(base, segments) {
		return "config"
	}
	if hasExtension(base, ".tsx", ".jsx", ".vue", ".svelte", ".astro") || (isSourceFile(base) && hasSegment(segments, "component", "components", "ui", "views")) {
		return "component"
	}
	if hasExtension(base, ".ts", ".js", ".mjs", ".cjs") || (isSourceFile(base) && hasSegment(segments, "domain", "core", "logic", "service", "services", "util", "utils", "lib", "libs")) {
		return "logic"
	}
	if hasExtension(base, ".go", ".py", ".rb", ".rs", ".java", ".kt", ".swift", ".c", ".h", ".cc", ".cpp", ".cxx", ".cs", ".php", ".sh", ".bash", ".ex", ".exs", ".hs", ".scala") {
		return "implementation"
	}
	return "unknown"
}

func isTestPath(base string, segments []string) bool {
	if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") || strings.HasSuffix(base, "_test.go") || strings.HasPrefix(base, "test_") {
		return true
	}
	return hasSegment(segments, "test", "tests", "spec", "specs", "__tests__")
}

func isConfigPath(base string, segments []string) bool {
	known := map[string]bool{
		"dockerfile":          true,
		"makefile":            true,
		"package.json":        true,
		"go.mod":              true,
		"go.sum":              true,
		"cargo.toml":          true,
		"package-lock.json":   true,
		"yarn.lock":           true,
		"pnpm-lock.yaml":      true,
		"docker-compose.yml":  true,
		"docker-compose.yaml": true,
		"pyproject.toml":      true,
		"tsconfig.json":       true,
	}
	if known[base] || base == ".env" || strings.HasPrefix(base, ".env.") || hasSegment(segments, ".github", ".circleci", ".buildkite", "config", "configs", "configuration") {
		return true
	}
	if strings.HasPrefix(base, ".") && (strings.Contains(base, "config") || strings.HasSuffix(base, "rc")) {
		return true
	}
	return strings.Contains(base, ".config.") || strings.HasPrefix(base, "config.") || strings.HasSuffix(base, ".config")
}

func hasExtension(base string, extensions ...string) bool {
	for _, extension := range extensions {
		if strings.HasSuffix(base, extension) {
			return true
		}
	}
	return false
}

func isSourceFile(base string) bool {
	return hasExtension(base, ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".vue", ".svelte", ".astro", ".go", ".py", ".rb", ".rs", ".java", ".kt", ".swift", ".c", ".h", ".cc", ".cpp", ".cxx", ".cs", ".php", ".sh", ".bash", ".ex", ".exs", ".hs", ".scala")
}

func hasSegment(segments []string, wanted ...string) bool {
	for _, segment := range segments {
		for _, candidate := range wanted {
			if segment == candidate {
				return true
			}
		}
	}
	return false
}
