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

func Validate(g model.GroupsFile, inv model.Inventory) []string {
	var errs []string
	if g.Version != 1 {
		errs = append(errs, fmt.Sprintf("version must be 1 (got %d)", g.Version))
	}
	if g.BaseSHA != inv.BaseSHA {
		errs = append(errs, fmt.Sprintf("base_sha mismatch: groups=%s inventory=%s", g.BaseSHA, inv.BaseSHA))
	}
	if g.HeadSHA != inv.HeadSHA {
		errs = append(errs, fmt.Sprintf("head_sha mismatch: groups=%s inventory=%s", g.HeadSHA, inv.HeadSHA))
	}
	known := map[string]bool{}
	for _, f := range inv.Fragments {
		known[f.ID] = true
	}
	groupIDs, assigned := map[string]bool{}, map[string][]string{}
	for _, group := range g.Groups {
		if group.ID == "" {
			errs = append(errs, "group id must not be empty")
		} else if groupIDs[group.ID] {
			errs = append(errs, fmt.Sprintf("duplicate group ID: %s", group.ID))
		}
		groupIDs[group.ID] = true
		if strings.TrimSpace(group.Title) == "" {
			errs = append(errs, fmt.Sprintf("group %s has an empty title", group.ID))
		}
		seen := map[string]bool{}
		for _, id := range group.FragmentIDs {
			if seen[id] {
				errs = append(errs, fmt.Sprintf("fragment %s is repeated in group %s", id, group.ID))
			}
			seen[id] = true
			if !known[id] {
				errs = append(errs, fmt.Sprintf("unknown fragment ID %s in group %s", id, group.ID))
			}
			assigned[id] = append(assigned[id], group.ID)
		}
	}
	for _, f := range inv.Fragments {
		if len(assigned[f.ID]) == 0 {
			errs = append(errs, fmt.Sprintf("unassigned fragment: %s (%s)", f.ID, f.Path))
		}
		if len(assigned[f.ID]) > 1 {
			errs = append(errs, fmt.Sprintf("fragment %s is assigned to multiple groups: %s", f.ID, strings.Join(assigned[f.ID], ", ")))
		}
	}
	sort.Strings(errs)
	return errs
}
