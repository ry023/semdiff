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
	return Parse(b)
}

func Parse(b []byte) (model.GroupsFile, error) {
	var result model.GroupsFile
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("decode groups file: %w", err)
	}
	for groupIndex := range result.Groups {
		for fragmentIndex := range result.Groups[groupIndex].Fragments {
			fragment := &result.Groups[groupIndex].Fragments[fragmentIndex]
			if fragment.ReviewLevel == "" {
				fragment.ReviewLevel = model.ReviewLevelNormal
			}
		}
	}
	return result, nil
}

type ValidationReport struct{ Errors, Warnings []string }

func Validate(g model.GroupsFile, changes model.ChangeMap) []string {
	return ValidateReport(g, changes).Errors
}

func Fragments(g model.GroupsFile) []model.Fragment {
	var result []model.Fragment
	for _, group := range g.Groups {
		result = append(result, group.Fragments...)
	}
	return result
}

type lineKey struct {
	path string
	old  bool
	line int
}

func ValidateReport(g model.GroupsFile, changes model.ChangeMap) ValidationReport {
	var report ValidationReport
	add := func(format string, args ...any) { report.Errors = append(report.Errors, fmt.Sprintf(format, args...)) }
	if g.Version != 3 {
		add("version must be 3 (got %d)", g.Version)
	}
	if g.BaseSHA != changes.BaseSHA {
		add("base_sha mismatch: groups=%s changes=%s", g.BaseSHA, changes.BaseSHA)
	}
	if g.HeadSHA != changes.HeadSHA {
		add("head_sha mismatch: groups=%s changes=%s", g.HeadSHA, changes.HeadSHA)
	}

	changedLines := map[lineKey]bool{}
	metadataPaths := map[string]bool{}
	for _, change := range changes.Changes {
		if change.Metadata || change.OldLines == 0 && change.NewLines == 0 {
			metadataPaths[change.Path] = true
			continue
		}
		for line := change.OldStart; line < change.OldStart+change.OldLines; line++ {
			changedLines[lineKey{path: change.Path, old: true, line: line}] = true
		}
		for line := change.NewStart; line < change.NewStart+change.NewLines; line++ {
			changedLines[lineKey{path: change.Path, line: line}] = true
		}
	}

	claims := map[lineKey][]string{}
	metadataClaims := map[string][]string{}
	groupIDs, fragmentIDs := map[string]bool{}, map[string]bool{}
	for _, group := range g.Groups {
		if strings.TrimSpace(group.ID) == "" {
			add("group id must not be empty")
		} else if groupIDs[group.ID] {
			add("duplicate group ID: %s", group.ID)
		}
		groupIDs[group.ID] = true
		if strings.TrimSpace(group.Title) == "" {
			add("group %s has an empty title", group.ID)
		}
		if strings.TrimSpace(group.Summary) == "" {
			add("group %s has an empty summary", group.ID)
		}
		if !group.Importance.Valid() {
			add("group %s has invalid importance %q (must be core, supporting, or side)", group.ID, group.Importance)
		}
		groupPaths := map[string]bool{}
		fragmentIDsInGroup := map[string]bool{}
		for _, fragment := range group.Fragments {
			validateFragment(fragment, group.ID, changedLines, metadataPaths, claims, metadataClaims, fragmentIDs, &report.Errors)
			fragmentIDsInGroup[fragment.ID] = true
			if fragment.Path != "" {
				groupPaths[fragment.Path] = true
			}
		}
		validateReviewSteps(group, fragmentIDsInGroup, &report.Errors)
		validateFileCategories(group, groupPaths, &report.Errors)
	}
	report.Errors = append(report.Errors, coverageErrors(changedLines, claims)...)
	for path := range metadataPaths {
		if len(metadataClaims[path]) == 0 {
			add("unassigned file metadata change: %s", path)
		} else if len(metadataClaims[path]) > 1 {
			add("file metadata change assigned to multiple fragments: %s (%s)", path, strings.Join(metadataClaims[path], ", "))
		}
	}
	sort.Strings(report.Errors)
	return report
}

func validateReviewSteps(group model.SemanticGroup, fragments map[string]bool, errors *[]string) {
	if len(group.ReviewSteps) == 0 {
		*errors = append(*errors, fmt.Sprintf("group %s has no review steps", group.ID))
		return
	}
	stepIDs, claimed := map[string]bool{}, map[string]string{}
	for _, step := range group.ReviewSteps {
		if strings.TrimSpace(step.ID) == "" {
			*errors = append(*errors, fmt.Sprintf("group %s has a review step with an empty ID", group.ID))
		} else if stepIDs[step.ID] {
			*errors = append(*errors, fmt.Sprintf("duplicate review step ID %s in group %s", step.ID, group.ID))
		}
		stepIDs[step.ID] = true
		if strings.TrimSpace(step.Title) == "" {
			*errors = append(*errors, fmt.Sprintf("review step %s in group %s has an empty title", step.ID, group.ID))
		}
		if strings.TrimSpace(step.Summary) == "" {
			*errors = append(*errors, fmt.Sprintf("review step %s in group %s has an empty summary", step.ID, group.ID))
		}
		if len(step.FragmentIDs) == 0 {
			*errors = append(*errors, fmt.Sprintf("review step %s in group %s has no fragments", step.ID, group.ID))
		}
		for _, id := range step.FragmentIDs {
			if !fragments[id] {
				*errors = append(*errors, fmt.Sprintf("review step %s in group %s references unknown fragment %s", step.ID, group.ID, id))
				continue
			}
			if prior, ok := claimed[id]; ok {
				*errors = append(*errors, fmt.Sprintf("fragment %s occurs in review steps %s and %s in group %s", id, prior, step.ID, group.ID))
			}
			claimed[id] = step.ID
		}
	}
	for id := range fragments {
		if claimed[id] == "" {
			*errors = append(*errors, fmt.Sprintf("fragment %s in group %s is not assigned to a review step", id, group.ID))
		}
	}
}

func validateFragment(fragment model.Fragment, groupID string, changed map[lineKey]bool, metadata map[string]bool, claims map[lineKey][]string, metadataClaims map[string][]string, ids map[string]bool, errors *[]string) {
	add := func(format string, args ...any) { *errors = append(*errors, fmt.Sprintf(format, args...)) }
	if strings.TrimSpace(fragment.ID) == "" {
		add("group %s has a fragment with an empty ID", groupID)
	} else if ids[fragment.ID] {
		add("duplicate fragment ID: %s", fragment.ID)
	}
	ids[fragment.ID] = true
	if strings.TrimSpace(fragment.Path) == "" {
		add("fragment %s has an empty path", fragment.ID)
	}
	if strings.TrimSpace(fragment.Description) == "" {
		add("fragment %s in group %s has an empty description", fragment.ID, groupID)
	}
	if !fragment.ReviewLevel.Valid() {
		add("fragment %s in group %s has invalid review_level %q (must be careful, normal, or skim)", fragment.ID, groupID, fragment.ReviewLevel)
	}
	if len(fragment.Ranges) == 0 && !fragment.FileMetadata {
		add("fragment %s has neither ranges nor file_metadata", fragment.ID)
	}
	selected := map[lineKey]bool{}
	for index, span := range fragment.Ranges {
		if span.Old == nil && span.New == nil {
			add("fragment %s range %d has neither old nor new side", fragment.ID, index)
		}
		sides := []struct {
			old   bool
			value *model.Range
		}{{true, span.Old}, {false, span.New}}
		for _, side := range sides {
			if side.value == nil {
				continue
			}
			if side.value.Start < 1 || side.value.Lines < 1 {
				add("fragment %s range %d must have positive start and lines", fragment.ID, index)
				continue
			}
		}
	}
	for key := range changed {
		if key.path != fragment.Path {
			continue
		}
		for _, span := range fragment.Ranges {
			value := span.New
			if key.old {
				value = span.Old
			}
			if value != nil && key.line >= value.Start && key.line < value.Start+value.Lines {
				selected[key] = true
				break
			}
		}
	}
	if len(fragment.Ranges) > 0 && len(selected) == 0 {
		add("fragment %s ranges select no changed lines", fragment.ID)
	}
	for key := range selected {
		claims[key] = append(claims[key], fragment.ID)
	}
	if fragment.FileMetadata {
		if !metadata[fragment.Path] {
			add("fragment %s selects file metadata but %s has no metadata-only change", fragment.ID, fragment.Path)
		} else {
			metadataClaims[fragment.Path] = append(metadataClaims[fragment.Path], fragment.ID)
		}
	}
}

func coverageErrors(changed map[lineKey]bool, claims map[lineKey][]string) []string {
	keys := make([]lineKey, 0, len(changed))
	for key := range changed {
		if len(claims[key]) != 1 {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].path != keys[j].path {
			return keys[i].path < keys[j].path
		}
		if keys[i].old != keys[j].old {
			return keys[i].old
		}
		return keys[i].line < keys[j].line
	})
	var result []string
	for index := 0; index < len(keys); {
		start := keys[index]
		owners := strings.Join(claims[start], ", ")
		end := start.line
		index++
		for index < len(keys) && keys[index].path == start.path && keys[index].old == start.old && keys[index].line == end+1 && strings.Join(claims[keys[index]], ", ") == owners {
			end = keys[index].line
			index++
		}
		side := "new"
		if start.old {
			side = "old"
		}
		location := fmt.Sprintf("%s %s:%d", start.path, side, start.line)
		if end > start.line {
			location = fmt.Sprintf("%s %s:%d,%d", start.path, side, start.line, end-start.line+1)
		}
		if owners == "" {
			result = append(result, "unassigned changed range: "+location)
		} else {
			result = append(result, fmt.Sprintf("changed range assigned to multiple fragments: %s (%s)", location, owners))
		}
	}
	return result
}

func validateFileCategories(group model.SemanticGroup, paths map[string]bool, errors *[]string) {
	seen := map[string]bool{}
	for _, category := range group.FileCategories {
		path := strings.TrimSpace(category.Path)
		if path == "" {
			*errors = append(*errors, fmt.Sprintf("group %s has a file category with an empty path", group.ID))
			continue
		}
		if strings.TrimSpace(category.Category) == "" {
			*errors = append(*errors, fmt.Sprintf("file category for %s in group %s has an empty category", path, group.ID))
		}
		if seen[path] {
			*errors = append(*errors, fmt.Sprintf("file category for %s is repeated in group %s", path, group.ID))
		}
		seen[path] = true
		if !paths[path] {
			*errors = append(*errors, fmt.Sprintf("file category path %s is not referenced by group %s", path, group.ID))
		}
	}
	for path := range paths {
		if !seen[path] {
			*errors = append(*errors, fmt.Sprintf("group %s has no file category for %s", group.ID, path))
		}
	}
}
