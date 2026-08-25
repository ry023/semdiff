package categories

import "testing"

func TestClassifyPaths(t *testing.T) {
	tests := []struct {
		path, want string
	}{
		{"web/Button.test.tsx", "test"},
		{"internal/gitdiff/gitdiff_test.go", "test"},
		{"src/config/app.config.ts", "config"},
		{".github/workflows/ci.yml", "config"},
		{"web/components/Button.tsx", "component"},
		{"src/domain/order.ts", "logic"},
		{"internal/gitdiff/gitdiff.go", "implementation"},
		{"docs/overview.md", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := classifyPath(tt.path)
			if got != tt.want {
				t.Fatalf("classifyPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestClassifyPathsDeduplicatesAndSorts(t *testing.T) {
	got := ClassifyPaths([]string{"b.ts", "a.test.ts", "b.ts", "", "a.test.ts"})
	if len(got) != 2 {
		t.Fatalf("got %d suggestions, want 2: %+v", len(got), got)
	}
	if got[0].Path != "a.test.ts" || got[0].Category != "test" || got[1].Path != "b.ts" || got[1].Category != "logic" {
		t.Fatalf("unexpected suggestions: %+v", got)
	}
}
