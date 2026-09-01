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
	"github.com/ry023/semdiff/internal/gitdiff"
	"github.com/ry023/semdiff/internal/groups"
	"github.com/ry023/semdiff/internal/model"
)

const Version = 4

type Draft struct {
	Version             int                     `json:"draft_version"`
	Revision            int                     `json:"revision"`
	BaseSHA             string                  `json:"base_sha"`
	HeadSHA             string                  `json:"head_sha"`
	Changes             []model.DiffChange      `json:"changes"`
	Suggestions         []model.Fragment        `json:"suggestions"`
	Fragments           []model.Fragment        `json:"fragments"`
	CategorySuggestions []categories.Suggestion `json:"category_suggestions,omitempty"`
	Groups              []DraftGroup            `json:"groups"`
}

type DraftGroup struct {
	ID             string               `json:"id"`
	Title          string               `json:"title,omitempty"`
	Summary        string               `json:"summary,omitempty"`
	Importance     model.Importance     `json:"importance,omitempty"`
	Order          *int                 `json:"order,omitempty"`
	Members        []string             `json:"members,omitempty"`
	FileCategories []model.FileCategory `json:"file_categories,omitempty"`
	ReviewSteps    []model.ReviewStep   `json:"review_steps,omitempty"`
}

type ApplyRequest struct {
	ExpectedRevision *int        `json:"expected_revision,omitempty"`
	Operations       []Operation `json:"operations"`
}

type Operation struct {
	Op          string             `json:"op"`
	GroupID     string             `json:"group_id,omitempty"`
	Title       *string            `json:"title,omitempty"`
	Summary     *string            `json:"summary,omitempty"`
	Importance  *model.Importance  `json:"importance,omitempty"`
	Order       *int               `json:"order,omitempty"`
	Fragment    *model.Fragment    `json:"fragment,omitempty"`
	Members     []string           `json:"members,omitempty"`
	Categories  map[string]string  `json:"categories,omitempty"`
	Paths       []string           `json:"paths,omitempty"`
	ReviewSteps []model.ReviewStep `json:"review_steps,omitempty"`
}

type Status struct {
	Revision               int           `json:"revision"`
	SuggestionCount        int           `json:"suggestion_count"`
	FragmentCount          int           `json:"fragment_count"`
	AssignedFragmentCount  int           `json:"assigned_fragment_count"`
	DescribedFragmentCount int           `json:"described_fragment_count"`
	UnassignedFragmentIDs  []string      `json:"unassigned_fragment_ids,omitempty"`
	UndescribedFragmentIDs []string      `json:"undescribed_fragment_ids,omitempty"`
	Groups                 []GroupStatus `json:"groups"`
	ReadyToFinalize        bool          `json:"ready_to_finalize"`
}

type GroupStatus struct {
	ID                       string   `json:"id"`
	FragmentCount            int      `json:"fragment_count"`
	MissingSummary           bool     `json:"missing_summary,omitempty"`
	MissingImportance        bool     `json:"missing_importance,omitempty"`
	MissingDescriptionIDs    []string `json:"missing_description_ids,omitempty"`
	MissingReviewLevelIDs    []string `json:"missing_review_level_ids,omitempty"`
	MissingFileCategoryPaths []string `json:"missing_file_category_paths,omitempty"`
	MissingReviewSteps       bool     `json:"missing_review_steps,omitempty"`
	MissingStepFragmentIDs   []string `json:"missing_step_fragment_ids,omitempty"`
}

type FragmentInspection struct {
	Fragment           model.Fragment `json:"fragment"`
	Assignments        []string       `json:"assignments,omitempty"`
	CategorySuggestion string         `json:"category_suggestion,omitempty"`
}

func New(inv model.ChangeMap, suggestions []categories.Suggestion) Draft {
	changes := append([]model.DiffChange(nil), inv.Changes...)
	for index := range changes {
		changes[index].Patch = ""
	}
	fragmentSuggestions := gitdiff.SuggestedFragments(inv)
	return Draft{Version: Version, BaseSHA: inv.BaseSHA, HeadSHA: inv.HeadSHA, Changes: changes, Suggestions: fragmentSuggestions, Fragments: []model.Fragment{}, CategorySuggestions: suggestions, Groups: []DraftGroup{}}
}

// NewFromGroups creates a draft for inv that carries forward the authored
// semantic decisions from source. The caller must verify that source belongs
// to a compatible earlier review range. The new draft always receives a fresh
// change map and fresh Git-derived suggestions from inv.
func NewFromGroups(inv model.ChangeMap, suggestions []categories.Suggestion, source model.GroupsFile) Draft {
	draft := New(inv, suggestions)
	for _, group := range source.Groups {
		copied := DraftGroup{
			ID:             group.ID,
			Title:          group.Title,
			Summary:        group.Summary,
			Importance:     group.Importance,
			Order:          group.Order,
			FileCategories: append([]model.FileCategory(nil), group.FileCategories...),
			ReviewSteps:    append([]model.ReviewStep(nil), group.ReviewSteps...),
		}
		for _, fragment := range group.Fragments {
			draft.Fragments = append(draft.Fragments, fragment)
			copied.Members = append(copied.Members, fragment.ID)
		}
		draft.Groups = append(draft.Groups, copied)
	}
	return draft
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
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".grouping-draft-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
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
	return os.Rename(name, path)
}

func (d Draft) ChangeMap() model.ChangeMap {
	return model.ChangeMap{BaseSHA: d.BaseSHA, HeadSHA: d.HeadSHA, Changes: append([]model.DiffChange(nil), d.Changes...)}
}

func (d Draft) ToGroupsFile() model.GroupsFile {
	byID := map[string]model.Fragment{}
	for _, fragment := range d.Fragments {
		byID[fragment.ID] = fragment
	}
	result := model.GroupsFile{Version: 3, BaseSHA: d.BaseSHA, HeadSHA: d.HeadSHA}
	for _, group := range d.Groups {
		semantic := model.SemanticGroup{ID: group.ID, Title: group.Title, Summary: group.Summary, Importance: group.Importance, Order: group.Order, FileCategories: append([]model.FileCategory(nil), group.FileCategories...), ReviewSteps: append([]model.ReviewStep(nil), group.ReviewSteps...)}
		for _, id := range group.Members {
			semantic.Fragments = append(semantic.Fragments, byID[id])
		}
		result.Groups = append(result.Groups, semantic)
	}
	return result
}

func (d Draft) Status() Status {
	assigned := map[string]bool{}
	for _, group := range d.Groups {
		for _, id := range group.Members {
			assigned[id] = true
		}
	}
	status := Status{Revision: d.Revision, SuggestionCount: len(d.Suggestions), FragmentCount: len(d.Fragments), Groups: []GroupStatus{}}
	byID := map[string]model.Fragment{}
	for _, fragment := range d.Fragments {
		byID[fragment.ID] = fragment
		if assigned[fragment.ID] {
			status.AssignedFragmentCount++
		} else {
			status.UnassignedFragmentIDs = append(status.UnassignedFragmentIDs, fragment.ID)
		}
		if strings.TrimSpace(fragment.Description) != "" {
			status.DescribedFragmentCount++
		} else if assigned[fragment.ID] {
			status.UndescribedFragmentIDs = append(status.UndescribedFragmentIDs, fragment.ID)
		}
	}
	for _, group := range d.Groups {
		item := GroupStatus{ID: group.ID, FragmentCount: len(group.Members), MissingSummary: strings.TrimSpace(group.Summary) == "", MissingImportance: !group.Importance.Valid(), MissingReviewSteps: len(group.ReviewSteps) == 0}
		paths := map[string]bool{}
		categorized := map[string]bool{}
		stepped := map[string]bool{}
		for _, id := range group.Members {
			fragment := byID[id]
			paths[fragment.Path] = true
			if strings.TrimSpace(fragment.Description) == "" {
				item.MissingDescriptionIDs = append(item.MissingDescriptionIDs, id)
			}
			if !fragment.ReviewLevel.Valid() {
				item.MissingReviewLevelIDs = append(item.MissingReviewLevelIDs, id)
			}
		}
		for _, category := range group.FileCategories {
			categorized[category.Path] = true
		}
		for _, step := range group.ReviewSteps {
			for _, id := range step.FragmentIDs {
				stepped[id] = true
			}
		}
		for _, id := range group.Members {
			if !stepped[id] {
				item.MissingStepFragmentIDs = append(item.MissingStepFragmentIDs, id)
			}
		}
		sort.Strings(item.MissingStepFragmentIDs)
		for path := range paths {
			if !categorized[path] {
				item.MissingFileCategoryPaths = append(item.MissingFileCategoryPaths, path)
			}
		}
		status.Groups = append(status.Groups, item)
	}
	sort.Strings(status.UnassignedFragmentIDs)
	sort.Strings(status.UndescribedFragmentIDs)
	status.ReadyToFinalize = len(d.FinalErrors()) == 0
	return status
}

func (d Draft) FinalErrors() []string {
	result := d.structuralErrors()
	for _, fragment := range d.Fragments {
		if d.assignment(fragment.ID) == "" {
			result = append(result, fmt.Sprintf("unassigned fragment: %s (%s)", fragment.ID, fragment.Path))
		}
	}
	if len(result) == 0 {
		result = append(result, groups.Validate(d.ToGroupsFile(), d.ChangeMap())...)
	}
	return sortedUnique(result)
}

func Apply(d Draft, request ApplyRequest) (Draft, error) {
	if len(request.Operations) == 0 {
		return d, errors.New("at least one operation is required")
	}
	if request.ExpectedRevision != nil && *request.ExpectedRevision != d.Revision {
		return d, fmt.Errorf("draft revision mismatch: expected %d, current %d", *request.ExpectedRevision, d.Revision)
	}
	working := clone(d)
	for index, operation := range request.Operations {
		if err := working.applyOperation(operation); err != nil {
			return d, fmt.Errorf("operation %d (%s): %w", index, operation.Op, err)
		}
	}
	if err := working.normalizeAndValidate(); err != nil {
		return d, err
	}
	working.Revision++
	return working, nil
}

func (d *Draft) applyOperation(operation Operation) error {
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
		if operation.Importance != nil {
			group.Importance = *operation.Importance
		}
		if operation.Order != nil {
			value := *operation.Order
			group.Order = &value
		}
	case "delete_group":
		index := d.groupIndex(operation.GroupID)
		if index < 0 {
			return fmt.Errorf("group %s does not exist", operation.GroupID)
		}
		if len(d.Groups[index].Members) > 0 {
			return fmt.Errorf("group %s still has assigned fragments", operation.GroupID)
		}
		d.Groups = append(d.Groups[:index], d.Groups[index+1:]...)
	case "add_fragment":
		if operation.Fragment == nil {
			return errors.New("fragment is required")
		}
		if d.fragmentIndex(operation.Fragment.ID) >= 0 {
			return fmt.Errorf("fragment %s already exists", operation.Fragment.ID)
		}
		d.Fragments = append(d.Fragments, *operation.Fragment)
	case "update_fragment":
		if operation.Fragment == nil {
			return errors.New("fragment is required")
		}
		index := d.fragmentIndex(operation.Fragment.ID)
		if index < 0 {
			return fmt.Errorf("fragment %s does not exist", operation.Fragment.ID)
		}
		d.Fragments[index] = *operation.Fragment
	case "merge_fragments":
		if operation.Fragment == nil {
			return errors.New("fragment is required")
		}
		if len(operation.Members) == 0 {
			return errors.New("members must contain at least one suggestion or fragment ID")
		}
		merged := *operation.Fragment
		if strings.TrimSpace(merged.ID) == "" {
			return errors.New("merged fragment ID is required")
		}
		sourceIDs := map[string]bool{}
		assignments := map[string]bool{}
		var sources []model.Fragment
		for _, id := range operation.Members {
			if sourceIDs[id] {
				return fmt.Errorf("source fragment %s is repeated", id)
			}
			sourceIDs[id] = true
			source, ok := d.fragmentOrSuggestion(id)
			if !ok {
				return fmt.Errorf("source fragment or suggestion %s does not exist", id)
			}
			sources = append(sources, source)
			if assignment := d.assignment(id); assignment != "" {
				assignments[assignment] = true
			}
		}
		if len(assignments) > 1 {
			return errors.New("source fragments belong to different groups; move them to one group before merging")
		}
		path := sources[0].Path
		for _, source := range sources {
			if source.Path != path {
				return errors.New("merge_fragments sources must use the same path")
			}
		}
		if merged.Path == "" {
			merged.Path = path
		} else if merged.Path != path {
			return fmt.Errorf("merged fragment path %s does not match source path %s", merged.Path, path)
		}
		if len(merged.Ranges) == 0 {
			for _, source := range sources {
				merged.Ranges = append(merged.Ranges, source.Ranges...)
			}
			sort.SliceStable(merged.Ranges, func(left, right int) bool {
				return fragmentRangeStart(merged.Ranges[left]) < fragmentRangeStart(merged.Ranges[right])
			})
		}
		for _, source := range sources {
			merged.FileMetadata = merged.FileMetadata || source.FileMetadata
		}
		if existing := d.fragmentIndex(merged.ID); existing >= 0 && !sourceIDs[merged.ID] {
			return fmt.Errorf("fragment %s already exists", merged.ID)
		}
		for _, suggestion := range d.Suggestions {
			if suggestion.ID == merged.ID && !sourceIDs[merged.ID] {
				return fmt.Errorf("suggestion %s already uses the merged fragment ID", merged.ID)
			}
		}
		filtered := d.Fragments[:0]
		for _, fragment := range d.Fragments {
			if !sourceIDs[fragment.ID] {
				filtered = append(filtered, fragment)
			}
		}
		d.Fragments = filtered
		for id := range sourceIDs {
			d.removeFragment(id)
		}
		d.Fragments = append(d.Fragments, merged)
		for groupID := range assignments {
			group, _ := d.group(groupID)
			group.Members = append(group.Members, merged.ID)
		}
	case "delete_fragments":
		for _, id := range operation.Members {
			index := d.fragmentIndex(id)
			if index < 0 {
				return fmt.Errorf("fragment %s does not exist", id)
			}
			d.removeFragment(id)
			d.Fragments = append(d.Fragments[:index], d.Fragments[index+1:]...)
		}
	case "assign_fragments", "move_fragments":
		group, err := d.group(operation.GroupID)
		if err != nil {
			return err
		}
		for _, id := range operation.Members {
			if d.fragmentIndex(id) < 0 {
				return fmt.Errorf("unknown fragment ID %s", id)
			}
			current := d.assignment(id)
			if operation.Op == "assign_fragments" && current != "" && current != operation.GroupID {
				return fmt.Errorf("fragment %s is already assigned to group %s; use move_fragments", id, current)
			}
			if operation.Op == "move_fragments" {
				d.removeFragment(id)
				group, _ = d.group(operation.GroupID)
			}
			if !contains(group.Members, id) {
				group.Members = append(group.Members, id)
			}
		}
	case "unassign_fragments":
		for _, id := range operation.Members {
			if d.fragmentIndex(id) < 0 {
				return fmt.Errorf("unknown fragment ID %s", id)
			}
			d.removeFragment(id)
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
			found := false
			for index := range group.FileCategories {
				if group.FileCategories[index].Path == path {
					group.FileCategories[index].Category = category
					found = true
				}
			}
			if !found {
				group.FileCategories = append(group.FileCategories, model.FileCategory{Path: path, Category: category})
			}
		}
	case "set_review_steps":
		group, err := d.group(operation.GroupID)
		if err != nil {
			return err
		}
		members := map[string]bool{}
		for _, id := range group.Members {
			members[id] = true
		}
		seenSteps, seenFragments := map[string]bool{}, map[string]bool{}
		for _, step := range operation.ReviewSteps {
			if strings.TrimSpace(step.ID) == "" || strings.TrimSpace(step.Title) == "" || strings.TrimSpace(step.Summary) == "" {
				return errors.New("review steps require non-empty id, title, and summary")
			}
			if seenSteps[step.ID] {
				return fmt.Errorf("review step %s is repeated", step.ID)
			}
			seenSteps[step.ID] = true
			for _, id := range step.FragmentIDs {
				if !members[id] {
					return fmt.Errorf("fragment %s is not assigned to group %s", id, group.ID)
				}
				if seenFragments[id] {
					return fmt.Errorf("fragment %s occurs in multiple review steps", id)
				}
				seenFragments[id] = true
			}
		}
		group.ReviewSteps = append([]model.ReviewStep(nil), operation.ReviewSteps...)
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
	index := d.fragmentIndex(id)
	if index < 0 {
		return FragmentInspection{}, fmt.Errorf("fragment %s does not exist", id)
	}
	result := FragmentInspection{Fragment: d.Fragments[index]}
	for _, group := range d.Groups {
		if contains(group.Members, id) {
			result.Assignments = append(result.Assignments, group.ID)
		}
	}
	for _, suggestion := range d.CategorySuggestions {
		if suggestion.Path == result.Fragment.Path {
			result.CategorySuggestion = suggestion.Category
		}
	}
	return result, nil
}

func (d Draft) InspectableFragments() []model.Fragment {
	result := append([]model.Fragment(nil), d.Fragments...)
	authored := map[string]bool{}
	for _, fragment := range d.Fragments {
		authored[fragment.ID] = true
	}
	for _, suggestion := range d.Suggestions {
		if !authored[suggestion.ID] {
			result = append(result, suggestion)
		}
	}
	return result
}

func (d Draft) UnassignedFragments() []model.Fragment {
	assigned := map[string]bool{}
	for _, group := range d.Groups {
		for _, id := range group.Members {
			assigned[id] = true
		}
	}
	var result []model.Fragment
	for _, fragment := range d.Fragments {
		if !assigned[fragment.ID] {
			result = append(result, fragment)
		}
	}
	return result
}
func (d Draft) Group(id string) (DraftGroup, error) {
	index := d.groupIndex(id)
	if index < 0 {
		return DraftGroup{}, fmt.Errorf("group %s does not exist", id)
	}
	return d.Groups[index], nil
}

func (d *Draft) normalizeAndValidate() error {
	if d.Version == 0 {
		d.Version = Version
	}
	if d.Version != Version {
		return fmt.Errorf("draft_version must be %d", Version)
	}
	if d.Changes == nil {
		d.Changes = []model.DiffChange{}
	}
	if d.Suggestions == nil {
		d.Suggestions = []model.Fragment{}
	}
	if d.Fragments == nil {
		d.Fragments = []model.Fragment{}
	}
	for index := range d.Fragments {
		if d.Fragments[index].ReviewLevel == "" {
			d.Fragments[index].ReviewLevel = model.ReviewLevelNormal
		}
	}
	if d.Groups == nil {
		d.Groups = []DraftGroup{}
	}
	for index := range d.Changes {
		d.Changes[index].Patch = ""
	}
	for index := range d.Groups {
		sort.SliceStable(d.Groups[index].FileCategories, func(a, b int) bool {
			return d.Groups[index].FileCategories[a].Path < d.Groups[index].FileCategories[b].Path
		})
	}
	return errors.Join(errorList(d.structuralErrors())...)
}

func (d Draft) structuralErrors() []string {
	var result []string
	suggestionIDs := map[string]bool{}
	for _, suggestion := range d.Suggestions {
		if strings.TrimSpace(suggestion.ID) == "" {
			result = append(result, "suggestion ID must not be empty")
		} else if suggestionIDs[suggestion.ID] {
			result = append(result, fmt.Sprintf("duplicate suggestion ID: %s", suggestion.ID))
		}
		suggestionIDs[suggestion.ID] = true
	}
	ids := map[string]bool{}
	for _, fragment := range d.Fragments {
		if strings.TrimSpace(fragment.ID) == "" {
			result = append(result, "fragment ID must not be empty")
		} else if ids[fragment.ID] {
			result = append(result, fmt.Sprintf("duplicate fragment ID: %s", fragment.ID))
		}
		ids[fragment.ID] = true
		if !fragment.ReviewLevel.Valid() {
			result = append(result, fmt.Sprintf("fragment %s has invalid review_level %q", fragment.ID, fragment.ReviewLevel))
		}
	}
	groupsSeen, assigned := map[string]bool{}, map[string]string{}
	for _, group := range d.Groups {
		if strings.TrimSpace(group.ID) == "" {
			result = append(result, "group ID must not be empty")
		} else if groupsSeen[group.ID] {
			result = append(result, fmt.Sprintf("duplicate group ID: %s", group.ID))
		}
		groupsSeen[group.ID] = true
		if group.Importance != "" && !group.Importance.Valid() {
			result = append(result, fmt.Sprintf("group %s has invalid importance %q", group.ID, group.Importance))
		}
		for _, id := range group.Members {
			if !ids[id] {
				result = append(result, fmt.Sprintf("unknown fragment ID %s in group %s", id, group.ID))
			}
			if previous := assigned[id]; previous != "" && previous != group.ID {
				result = append(result, fmt.Sprintf("fragment %s is assigned to multiple groups", id))
			}
			assigned[id] = group.ID
		}
	}
	return sortedUnique(result)
}

func (d Draft) groupIndex(id string) int {
	for index := range d.Groups {
		if d.Groups[index].ID == id {
			return index
		}
	}
	return -1
}
func (d *Draft) group(id string) (*DraftGroup, error) {
	index := d.groupIndex(id)
	if index < 0 {
		return nil, fmt.Errorf("group %s does not exist", id)
	}
	return &d.Groups[index], nil
}
func (d Draft) fragmentIndex(id string) int {
	for index := range d.Fragments {
		if d.Fragments[index].ID == id {
			return index
		}
	}
	return -1
}
func (d Draft) fragmentOrSuggestion(id string) (model.Fragment, bool) {
	if index := d.fragmentIndex(id); index >= 0 {
		return d.Fragments[index], true
	}
	for _, suggestion := range d.Suggestions {
		if suggestion.ID == id {
			return suggestion, true
		}
	}
	return model.Fragment{}, false
}
func fragmentRangeStart(span model.FragmentRange) int {
	if span.New != nil {
		return span.New.Start
	}
	if span.Old != nil {
		return span.Old.Start
	}
	return 0
}
func (d Draft) assignment(id string) string {
	for _, group := range d.Groups {
		if contains(group.Members, id) {
			return group.ID
		}
	}
	return ""
}
func (d *Draft) removeFragment(id string) {
	for index := range d.Groups {
		filtered := d.Groups[index].Members[:0]
		for _, current := range d.Groups[index].Members {
			if current != id {
				filtered = append(filtered, current)
			}
		}
		d.Groups[index].Members = filtered
		for stepIndex := range d.Groups[index].ReviewSteps {
			step := &d.Groups[index].ReviewSteps[stepIndex]
			ids := step.FragmentIDs[:0]
			for _, current := range step.FragmentIDs {
				if current != id {
					ids = append(ids, current)
				}
			}
			step.FragmentIDs = ids
		}
	}
}
func clone(d Draft) Draft {
	bytes, _ := json.Marshal(d)
	var result Draft
	_ = json.Unmarshal(bytes, &result)
	return result
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
	var result []string
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
