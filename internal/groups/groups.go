package groups

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ry023/semdiff/internal/model"
)

func Load(path string) (model.GroupsFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return model.GroupsFile{}, err
	}
	var g model.GroupsFile
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err := d.Decode(&g); err != nil {
		return g, fmt.Errorf("decode groups file: %w", err)
	}
	return g, nil
}

type ValidationReport struct {
	Errors   []string
	Warnings []string
}

func Validate(g model.GroupsFile, inv model.Inventory) []string {
	return ValidateReport(g, inv).Errors
}

func ValidateReport(g model.GroupsFile, inv model.Inventory) ValidationReport {
	var report ValidationReport
	errs := &report.Errors
	warnings := &report.Warnings
	if g.Version != 1 {
		*errs = append(*errs, fmt.Sprintf("version must be 1 (got %d)", g.Version))
	}
	if g.BaseSHA != inv.BaseSHA {
		*errs = append(*errs, fmt.Sprintf("base_sha mismatch: groups=%s inventory=%s", g.BaseSHA, inv.BaseSHA))
	}
	if g.HeadSHA != inv.HeadSHA {
		*errs = append(*errs, fmt.Sprintf("head_sha mismatch: groups=%s inventory=%s", g.HeadSHA, inv.HeadSHA))
	}
	known := map[string]bool{}
	for _, f := range inv.Fragments {
		known[f.ID] = true
	}
	groupIDs, assigned := map[string]bool{}, map[string][]string{}
	for _, group := range g.Groups {
		if group.ID == "" {
			*errs = append(*errs, "group id must not be empty")
		} else if groupIDs[group.ID] {
			*errs = append(*errs, fmt.Sprintf("duplicate group ID: %s", group.ID))
		}
		groupIDs[group.ID] = true
		if strings.TrimSpace(group.Title) == "" {
			*errs = append(*errs, fmt.Sprintf("group %s has an empty title", group.ID))
		}
		if len(group.Fragments) > 0 && len(group.FragmentIDs) > 0 {
			*errs = append(*errs, fmt.Sprintf("group %s must use either fragments or fragment_ids, not both", group.ID))
		}
		seen := map[string]bool{}
		groupPaths := map[string]bool{}
		for _, fragment := range group.FragmentReferences() {
			id := fragment.ID
			if id == "" {
				*errs = append(*errs, fmt.Sprintf("group %s has a fragment with an empty ID", group.ID))
				continue
			}
			if len(group.Fragments) > 0 && strings.TrimSpace(fragment.Description) == "" {
				*errs = append(*errs, fmt.Sprintf("fragment %s in group %s has an empty description", id, group.ID))
			}
			if seen[id] {
				*errs = append(*errs, fmt.Sprintf("fragment %s is repeated in group %s", id, group.ID))
			}
			seen[id] = true
			if !known[id] {
				*errs = append(*errs, fmt.Sprintf("unknown fragment ID %s in group %s", id, group.ID))
			} else {
				for _, fragment := range inv.Fragments {
					if fragment.ID == id {
						groupPaths[fragment.Path] = true
						break
					}
				}
			}
			assigned[id] = append(assigned[id], group.ID)
		}
		validateFileCategories(group, groupPaths, errs, warnings)
	}
	for _, f := range inv.Fragments {
		if len(assigned[f.ID]) == 0 {
			*errs = append(*errs, fmt.Sprintf("unassigned fragment: %s (%s)", f.ID, f.Path))
		}
		if len(assigned[f.ID]) > 1 {
			*errs = append(*errs, fmt.Sprintf("fragment %s is assigned to multiple groups: %s", f.ID, strings.Join(assigned[f.ID], ", ")))
		}
	}
	sort.Strings(*errs)
	sort.Strings(*warnings)
	return report
}

func validateFileCategories(group model.SemanticGroup, groupPaths map[string]bool, errs, warnings *[]string) {
	if len(group.FileCategories) == 0 {
		*warnings = append(*warnings, fmt.Sprintf("group %s has no file_categories", group.ID))
		return
	}
	seen := map[string]bool{}
	for _, fileCategory := range group.FileCategories {
		path := strings.TrimSpace(fileCategory.Path)
		category := strings.TrimSpace(fileCategory.Category)
		if path == "" {
			*errs = append(*errs, fmt.Sprintf("group %s has a file category with an empty path", group.ID))
			continue
		}
		if category == "" {
			*errs = append(*errs, fmt.Sprintf("file category for %s in group %s has an empty category", path, group.ID))
		}
		if seen[path] {
			*errs = append(*errs, fmt.Sprintf("file category for %s is repeated in group %s", path, group.ID))
		}
		seen[path] = true
		if !groupPaths[path] {
			*errs = append(*errs, fmt.Sprintf("file category path %s is not referenced by group %s", path, group.ID))
		}
	}
	for path := range groupPaths {
		if !seen[path] {
			*errs = append(*errs, fmt.Sprintf("group %s has no file category for %s", group.ID, path))
		}
	}
}
