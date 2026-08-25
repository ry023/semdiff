package groupingdraft

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ry023/semdiff/internal/categories"
	"github.com/ry023/semdiff/internal/groups"
	"github.com/ry023/semdiff/internal/model"
)

const Version = 1

// Draft is the resumable, intentionally incomplete representation used while
// an agent is deciding how to group a range. It is converted to GroupsFile by
// Finalize; the draft itself is never used by the viewer.
type Draft struct {
	Version             int                     `json:"draft_version"`
	Revision            int                     `json:"revision"`
	BaseSHA             string                  `json:"base_sha"`
	HeadSHA             string                  `json:"head_sha"`
	Fragments           []model.DiffFragment    `json:"fragments"`
	CategorySuggestions []categories.Suggestion `json:"category_suggestions,omitempty"`
	Groups              []DraftGroup            `json:"groups"`
	Descriptions        map[string]string       `json:"descriptions,omitempty"`
}

type DraftGroup struct {
	ID             string               `json:"id"`
	Title          string               `json:"title,omitempty"`
	Summary        string               `json:"summary,omitempty"`
	Order          *int                 `json:"order,omitempty"`
	FragmentIDs    []string             `json:"fragment_ids,omitempty"`
	FileCategories []model.FileCategory `json:"file_categories,omitempty"`
}

type ApplyRequest struct {
	ExpectedRevision *int        `json:"expected_revision,omitempty"`
	Operations       []Operation `json:"operations"`
}

type Operation struct {
	Op           string            `json:"op"`
	GroupID      string            `json:"group_id,omitempty"`
	Title        *string           `json:"title,omitempty"`
	Summary      *string           `json:"summary,omitempty"`
	Order        *int              `json:"order,omitempty"`
	FragmentIDs  []string          `json:"fragment_ids,omitempty"`
	Descriptions map[string]string `json:"descriptions,omitempty"`
	Categories   map[string]string `json:"categories,omitempty"`
	Paths        []string          `json:"paths,omitempty"`
}

type Status struct {
	Revision               int           `json:"revision"`
	FragmentCount          int           `json:"fragment_count"`
	AssignedFragmentCount  int           `json:"assigned_fragment_count"`
	DescribedFragmentCount int           `json:"described_fragment_count"`
	UnassignedFragmentIDs  []string      `json:"unassigned_fragment_ids,omitempty"`
	UndescribedFragmentIDs []string      `json:"undescribed_fragment_ids,omitempty"`
	OrphanDescriptionIDs   []string      `json:"orphan_description_ids,omitempty"`
	Groups                 []GroupStatus `json:"groups"`
	ReadyToFinalize        bool          `json:"ready_to_finalize"`
}

type GroupStatus struct {
	ID                       string   `json:"id"`
	FragmentCount            int      `json:"fragment_count"`
	MissingSummary           bool     `json:"missing_summary,omitempty"`
	MissingDescriptionIDs    []string `json:"missing_description_ids,omitempty"`
	MissingFileCategoryPaths []string `json:"missing_file_category_paths,omitempty"`
}

type FragmentInspection struct {
	Fragment           model.DiffFragment `json:"fragment"`
	Assignments        []string           `json:"assignments,omitempty"`
	Description        string             `json:"description,omitempty"`
	CategorySuggestion string             `json:"category_suggestion,omitempty"`
}

func New(inv model.Inventory, suggestions []categories.Suggestion) Draft {
	fragments := append([]model.DiffFragment(nil), inv.Fragments...)
	for i := range fragments {
		fragments[i].Patch = ""
	}
	sort.SliceStable(fragments, func(i, j int) bool { return fragments[i].ID < fragments[j].ID })
	suggestions = append([]categories.Suggestion(nil), suggestions...)
	sort.SliceStable(suggestions, func(i, j int) bool { return suggestions[i].Path < suggestions[j].Path })
	return Draft{
		Version:             Version,
		BaseSHA:             inv.BaseSHA,
		HeadSHA:             inv.HeadSHA,
		Fragments:           fragments,
		CategorySuggestions: suggestions,
		Groups:              []DraftGroup{},
		Descriptions:        map[string]string{},
	}
}

func Load(path string) (Draft, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Draft{}, err
	}
	var draft Draft
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&draft); err != nil {
		return draft, fmt.Errorf("decode grouping draft: %w", err)
	}
	if err := draft.normalizeAndValidate(); err != nil {
		return draft, err
	}
	return draft, nil
}

func SaveAtomic(path string, draft Draft) error {
	if err := draft.normalizeAndValidate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(draft, "", "  ")
	if err != nil {
		return fmt.Errorf("encode grouping draft: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".grouping-draft-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0644); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func (d Draft) Inventory() model.Inventory {
	fragments := append([]model.DiffFragment(nil), d.Fragments...)
	return model.Inventory{BaseSHA: d.BaseSHA, HeadSHA: d.HeadSHA, Fragments: fragments}
}

func (d Draft) ToGroupsFile() model.GroupsFile {
	result := model.GroupsFile{Version: 1, BaseSHA: d.BaseSHA, HeadSHA: d.HeadSHA}
	for _, group := range d.Groups {
		semantic := model.SemanticGroup{
			ID:             group.ID,
			Title:          group.Title,
			Summary:        group.Summary,
			Order:          group.Order,
			FileCategories: append([]model.FileCategory(nil), group.FileCategories...),
		}
		for _, id := range group.FragmentIDs {
			semantic.Fragments = append(semantic.Fragments, model.FragmentReference{ID: id, Description: d.Descriptions[id]})
		}
		result.Groups = append(result.Groups, semantic)
	}
	return result
}

func (d Draft) Status() Status {
	fragmentPaths := map[string]string{}
	for _, fragment := range d.Fragments {
		fragmentPaths[fragment.ID] = fragment.Path
	}
	assignments := map[string][]string{}
	for _, group := range d.Groups {
		for _, id := range group.FragmentIDs {
			assignments[id] = append(assignments[id], group.ID)
		}
	}
	status := Status{Revision: d.Revision, FragmentCount: len(d.Fragments), Groups: []GroupStatus{}}
	for _, fragment := range d.Fragments {
		assigned := len(assignments[fragment.ID]) > 0
		if assigned {
			status.AssignedFragmentCount++
		} else {
			status.UnassignedFragmentIDs = append(status.UnassignedFragmentIDs, fragment.ID)
		}
		if strings.TrimSpace(d.Descriptions[fragment.ID]) != "" {
			status.DescribedFragmentCount++
		} else if assigned {
			status.UndescribedFragmentIDs = append(status.UndescribedFragmentIDs, fragment.ID)
		}
	}
	for id := range d.Descriptions {
		if _, ok := fragmentPaths[id]; !ok {
			status.OrphanDescriptionIDs = append(status.OrphanDescriptionIDs, id)
		}
	}
	for _, group := range d.Groups {
		groupStatus := GroupStatus{ID: group.ID, FragmentCount: len(group.FragmentIDs), MissingSummary: strings.TrimSpace(group.Summary) == ""}
		groupPaths := map[string]bool{}
		for _, id := range group.FragmentIDs {
			if path, ok := fragmentPaths[id]; ok {
				groupPaths[path] = true
			}
			if strings.TrimSpace(d.Descriptions[id]) == "" {
				groupStatus.MissingDescriptionIDs = append(groupStatus.MissingDescriptionIDs, id)
			}
		}
		categorized := map[string]bool{}
		for _, category := range group.FileCategories {
			categorized[category.Path] = true
		}
		for path := range groupPaths {
			if !categorized[path] {
				groupStatus.MissingFileCategoryPaths = append(groupStatus.MissingFileCategoryPaths, path)
			}
		}
		sort.Strings(groupStatus.MissingDescriptionIDs)
		sort.Strings(groupStatus.MissingFileCategoryPaths)
		status.Groups = append(status.Groups, groupStatus)
	}
	sort.Strings(status.UnassignedFragmentIDs)
	sort.Strings(status.UndescribedFragmentIDs)
	sort.Strings(status.OrphanDescriptionIDs)
	sort.SliceStable(status.Groups, func(i, j int) bool { return status.Groups[i].ID < status.Groups[j].ID })
	status.ReadyToFinalize = len(d.FinalErrors()) == 0
	return status
}

func (d Draft) FinalErrors() []string {
	var errs []string
	if d.Version != Version {
		errs = append(errs, fmt.Sprintf("draft_version must be %d (got %d)", Version, d.Version))
	}
	if strings.TrimSpace(d.BaseSHA) == "" || strings.TrimSpace(d.HeadSHA) == "" {
		errs = append(errs, "base_sha and head_sha must not be empty")
	}
	for _, group := range d.Groups {
		if strings.TrimSpace(group.Title) == "" {
			errs = append(errs, fmt.Sprintf("group %s has an empty title", group.ID))
		}
		if strings.TrimSpace(group.Summary) == "" {
			errs = append(errs, fmt.Sprintf("group %s has an empty summary", group.ID))
		}
	}
	structural := d.structuralErrors()
	errs = append(errs, structural...)
	if len(structural) > 0 {
		return sortedUnique(errs)
	}
	assigned := map[string]int{}
	for _, group := range d.Groups {
		paths := map[string]bool{}
		for _, id := range group.FragmentIDs {
			assigned[id]++
			for _, fragment := range d.Fragments {
				if fragment.ID == id {
					paths[fragment.Path] = true
					break
				}
			}
			if strings.TrimSpace(d.Descriptions[id]) == "" {
				errs = append(errs, fmt.Sprintf("fragment %s has an empty description", id))
			}
		}
		categorized := map[string]bool{}
		for _, category := range group.FileCategories {
			categorized[category.Path] = true
		}
		for path := range paths {
			if !categorized[path] {
				errs = append(errs, fmt.Sprintf("group %s has no file category for %s", group.ID, path))
			}
		}
		for path := range categorized {
			if !paths[path] {
				errs = append(errs, fmt.Sprintf("file category path %s is not referenced by group %s", path, group.ID))
			}
		}
	}
	for _, fragment := range d.Fragments {
		if assigned[fragment.ID] == 0 {
			errs = append(errs, fmt.Sprintf("unassigned fragment: %s (%s)", fragment.ID, fragment.Path))
		} else if assigned[fragment.ID] > 1 {
			errs = append(errs, fmt.Sprintf("fragment %s is assigned to multiple groups", fragment.ID))
		}
	}
	report := groups.ValidateReport(d.ToGroupsFile(), d.Inventory())
	errs = append(errs, report.Errors...)
	for _, warning := range report.Warnings {
		errs = append(errs, warning)
	}
	return sortedUnique(errs)
}

func (d Draft) structuralErrors() []string {
	var errs []string
	known := map[string]bool{}
	for _, fragment := range d.Fragments {
		if fragment.ID == "" {
			errs = append(errs, "fragment ID must not be empty")
		}
		if known[fragment.ID] {
			errs = append(errs, fmt.Sprintf("duplicate fragment ID: %s", fragment.ID))
		}
		known[fragment.ID] = true
	}
	groupIDs := map[string]bool{}
	assigned := map[string]string{}
	for _, group := range d.Groups {
		if group.ID == "" {
			errs = append(errs, "group ID must not be empty")
		}
		if groupIDs[group.ID] {
			errs = append(errs, fmt.Sprintf("duplicate group ID: %s", group.ID))
		}
		groupIDs[group.ID] = true
		seen := map[string]bool{}
		for _, id := range group.FragmentIDs {
			if !known[id] {
				errs = append(errs, fmt.Sprintf("unknown fragment ID %s in group %s", id, group.ID))
			}
			if seen[id] {
				errs = append(errs, fmt.Sprintf("fragment %s is repeated in group %s", id, group.ID))
			}
			seen[id] = true
			if previous, ok := assigned[id]; ok && previous != group.ID {
				errs = append(errs, fmt.Sprintf("fragment %s is assigned to multiple groups: %s, %s", id, previous, group.ID))
			}
			assigned[id] = group.ID
		}
		categoryPaths := map[string]bool{}
		for _, category := range group.FileCategories {
			if strings.TrimSpace(category.Path) == "" {
				errs = append(errs, fmt.Sprintf("group %s has a file category with an empty path", group.ID))
			}
			if strings.TrimSpace(category.Category) == "" {
				errs = append(errs, fmt.Sprintf("file category for %s in group %s has an empty category", category.Path, group.ID))
			}
			if categoryPaths[category.Path] {
				errs = append(errs, fmt.Sprintf("file category for %s is repeated in group %s", category.Path, group.ID))
			}
			categoryPaths[category.Path] = true
		}
	}
	return sortedUnique(errs)
}

func Apply(d Draft, request ApplyRequest) (Draft, error) {
	if len(request.Operations) == 0 {
		return d, errors.New("at least one operation is required")
	}
	if request.ExpectedRevision != nil && *request.ExpectedRevision != d.Revision {
		return d, fmt.Errorf("draft revision mismatch: expected %d, current %d", *request.ExpectedRevision, d.Revision)
	}
	working := clone(d)
	if working.Descriptions == nil {
		working.Descriptions = map[string]string{}
	}
	for i, operation := range request.Operations {
		if err := working.applyOperation(operation); err != nil {
			return d, fmt.Errorf("operation %d (%s): %w", i, operation.Op, err)
		}
	}
	if err := working.normalizeAndValidate(); err != nil {
		return d, err
	}
	working.Revision++
	return working, nil
}

func (d *Draft) applyOperation(operation Operation) error {
	known := d.knownFragmentIDs()
	switch operation.Op {
	case "upsert_group":
		if strings.TrimSpace(operation.GroupID) == "" {
			return errors.New("group_id is required")
		}
		index := d.groupIndex(operation.GroupID)
		if index < 0 {
			d.Groups = append(d.Groups, DraftGroup{ID: operation.GroupID})
			index = len(d.Groups) - 1
		}
		group := &d.Groups[index]
		if operation.Title != nil {
			group.Title = *operation.Title
		}
		if operation.Summary != nil {
			group.Summary = *operation.Summary
		}
		if operation.Order != nil {
			order := *operation.Order
			group.Order = &order
		}
	case "delete_group":
		index := d.groupIndex(operation.GroupID)
		if index < 0 {
			return fmt.Errorf("group %s does not exist", operation.GroupID)
		}
		if len(d.Groups[index].FragmentIDs) > 0 {
			return fmt.Errorf("group %s still has assigned fragments", operation.GroupID)
		}
		d.Groups = append(d.Groups[:index], d.Groups[index+1:]...)
	case "assign_fragments":
		group, err := d.group(operation.GroupID)
		if err != nil {
			return err
		}
		for _, id := range operation.FragmentIDs {
			if !known[id] {
				return fmt.Errorf("unknown fragment ID %s", id)
			}
			if current := d.assignment(id); current != "" && current != operation.GroupID {
				return fmt.Errorf("fragment %s is already assigned to group %s; use move_fragments", id, current)
			}
			if !contains(group.FragmentIDs, id) {
				group.FragmentIDs = append(group.FragmentIDs, id)
			}
		}
	case "move_fragments":
		if _, err := d.group(operation.GroupID); err != nil {
			return err
		}
		for _, id := range operation.FragmentIDs {
			if !known[id] {
				return fmt.Errorf("unknown fragment ID %s", id)
			}
			d.removeFragment(id)
			group, _ := d.group(operation.GroupID)
			group.FragmentIDs = append(group.FragmentIDs, id)
		}
	case "unassign_fragments":
		for _, id := range operation.FragmentIDs {
			if !known[id] {
				return fmt.Errorf("unknown fragment ID %s", id)
			}
			d.removeFragment(id)
		}
	case "describe_fragments":
		for id, description := range operation.Descriptions {
			if !known[id] {
				return fmt.Errorf("unknown fragment ID %s", id)
			}
			d.Descriptions[id] = description
		}
	case "set_file_categories":
		group, err := d.group(operation.GroupID)
		if err != nil {
			return err
		}
		for path, category := range operation.Categories {
			path, category = strings.TrimSpace(path), strings.TrimSpace(category)
			if path == "" || category == "" {
				return errors.New("category paths and values must not be empty")
			}
			updated := false
			for i := range group.FileCategories {
				if group.FileCategories[i].Path == path {
					group.FileCategories[i].Category = category
					updated = true
					break
				}
			}
			if !updated {
				group.FileCategories = append(group.FileCategories, model.FileCategory{Path: path, Category: category})
			}
		}
	case "remove_file_categories":
		group, err := d.group(operation.GroupID)
		if err != nil {
			return err
		}
		for _, path := range operation.Paths {
			filtered := group.FileCategories[:0]
			for _, category := range group.FileCategories {
				if category.Path != path {
					filtered = append(filtered, category)
				}
			}
			group.FileCategories = filtered
		}
	default:
		return fmt.Errorf("unknown operation %q", operation.Op)
	}
	return nil
}

func (d Draft) FragmentInspection(id string) (FragmentInspection, error) {
	var result FragmentInspection
	for _, fragment := range d.Fragments {
		if fragment.ID == id {
			result.Fragment = fragment
			result.Description = d.Descriptions[id]
			for _, group := range d.Groups {
				if contains(group.FragmentIDs, id) {
					result.Assignments = append(result.Assignments, group.ID)
				}
			}
			for _, suggestion := range d.CategorySuggestions {
				if suggestion.Path == fragment.Path {
					result.CategorySuggestion = suggestion.Category
					break
				}
			}
			return result, nil
		}
	}
	return result, fmt.Errorf("fragment %s does not exist", id)
}

func (d Draft) UnassignedFragments() []model.DiffFragment {
	assigned := map[string]bool{}
	for _, group := range d.Groups {
		for _, id := range group.FragmentIDs {
			assigned[id] = true
		}
	}
	var result []model.DiffFragment
	for _, fragment := range d.Fragments {
		if !assigned[fragment.ID] {
			result = append(result, fragment)
		}
	}
	return result
}

func (d Draft) Group(groupID string) (DraftGroup, error) {
	index := d.groupIndex(groupID)
	if index < 0 {
		return DraftGroup{}, fmt.Errorf("group %s does not exist", groupID)
	}
	return d.Groups[index], nil
}

func (d *Draft) normalizeAndValidate() error {
	if d.Version == 0 {
		d.Version = Version
	}
	if d.Descriptions == nil {
		d.Descriptions = map[string]string{}
	}
	if d.Groups == nil {
		d.Groups = []DraftGroup{}
	}
	if d.Fragments == nil {
		d.Fragments = []model.DiffFragment{}
	}
	for i := range d.Fragments {
		d.Fragments[i].Patch = ""
	}
	for i := range d.Groups {
		sort.SliceStable(d.Groups[i].FileCategories, func(left, right int) bool {
			return d.Groups[i].FileCategories[left].Path < d.Groups[i].FileCategories[right].Path
		})
	}
	sort.SliceStable(d.CategorySuggestions, func(left, right int) bool {
		return d.CategorySuggestions[left].Path < d.CategorySuggestions[right].Path
	})
	return errors.Join(errorList(d.structuralErrors())...)
}

func (d *Draft) group(groupID string) (*DraftGroup, error) {
	index := d.groupIndex(groupID)
	if index < 0 {
		return nil, fmt.Errorf("group %s does not exist", groupID)
	}
	return &d.Groups[index], nil
}

func (d Draft) groupIndex(groupID string) int {
	for i := range d.Groups {
		if d.Groups[i].ID == groupID {
			return i
		}
	}
	return -1
}

func (d Draft) knownFragmentIDs() map[string]bool {
	known := make(map[string]bool, len(d.Fragments))
	for _, fragment := range d.Fragments {
		known[fragment.ID] = true
	}
	return known
}

func (d Draft) assignment(id string) string {
	for _, group := range d.Groups {
		if contains(group.FragmentIDs, id) {
			return group.ID
		}
	}
	return ""
}

func (d *Draft) removeFragment(id string) {
	for i := range d.Groups {
		filtered := d.Groups[i].FragmentIDs[:0]
		for _, current := range d.Groups[i].FragmentIDs {
			if current != id {
				filtered = append(filtered, current)
			}
		}
		d.Groups[i].FragmentIDs = filtered
	}
}

func clone(d Draft) Draft {
	b, _ := json.Marshal(d)
	var copy Draft
	_ = json.Unmarshal(b, &copy)
	return copy
}

func contains(values []string, value string) bool {
	for _, current := range values {
		if current == value {
			return true
		}
	}
	return false
}

func errorList(values []string) []error {
	result := make([]error, 0, len(values))
	for _, value := range values {
		result = append(result, errors.New(value))
	}
	return result
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
