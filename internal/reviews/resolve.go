package reviews

import (
	"context"
	"fmt"
	"strings"

	"github.com/ry023/semdiff/internal/gitdiff"
	"github.com/ry023/semdiff/internal/groups"
)

// Resolution contains a validated remote artifact. A nil result means no match.
type Resolution struct {
	Entry
	Data          []byte
	CommitsBehind int
}

// Resolve searches only the configured remote store, preferring an exact match
// and then the nearest first-parent ancestor with the same base.
func (s Store) Resolve(ctx context.Context, runner gitdiff.Runner, base, head string, exact bool) (*Resolution, error) {
	if err := s.Fetch(ctx); err != nil {
		exists, probeErr := s.branchExists(ctx)
		if probeErr == nil && !exists {
			return nil, nil
		}
		return nil, err
	}
	entries, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	available := make(map[string]Entry)
	for _, entry := range entries {
		if entry.BaseSHA == base {
			available[entry.HeadSHA] = entry
		}
	}
	candidates := []string{head}
	if !exact {
		history, err := runner.FirstParentCommits(ctx, base+".."+head)
		if err != nil {
			return nil, fmt.Errorf("walk current branch history: %w", err)
		}
		for _, sha := range history {
			if sha != head {
				candidates = append(candidates, sha)
			}
		}
	}
	for behind, sha := range candidates {
		entry, ok := available[sha]
		if !ok {
			continue
		}
		data, err := s.Read(ctx, entry.Path)
		if err != nil {
			return nil, err
		}
		g, err := groups.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("remote review %s: %w", entry.Path, err)
		}
		if g.BaseSHA != base || g.HeadSHA != sha {
			return nil, fmt.Errorf("remote artifact range does not match requested range %s..%s", base, sha)
		}
		changes, err := runner.Changes(ctx, base+".."+sha)
		if err != nil {
			return nil, err
		}
		report := groups.ValidateReport(g, changes)
		if len(report.Errors) > 0 {
			return nil, fmt.Errorf("remote groups file is invalid: %s", strings.Join(report.Errors, "; "))
		}
		return &Resolution{Entry: entry, Data: data, CommitsBehind: behind}, nil
	}
	return nil, nil
}
