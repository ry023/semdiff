package model

type Range struct {
	Start int `json:"start"`
	Lines int `json:"lines"`
}

type DiffFragment struct {
	ID       string `json:"id"`
	BaseSHA  string `json:"base_sha"`
	HeadSHA  string `json:"head_sha"`
	Path     string `json:"path"`
	OldStart int    `json:"old_start"`
	OldLines int    `json:"old_lines"`
	NewStart int    `json:"new_start"`
	NewLines int    `json:"new_lines"`
	Patch    string `json:"patch,omitempty"`
}

type Commit struct {
	SHA          string `json:"sha"`
	Subject      string `json:"subject"`
	Author       string `json:"author"`
	Timestamp    string `json:"timestamp"`
	FilesChanged int    `json:"files_changed"`
}

type SemanticGroup struct {
	ID             string              `json:"id"`
	Title          string              `json:"title"`
	Summary        string              `json:"summary"`
	Order          *int                `json:"order,omitempty"`
	FileCategories []FileCategory      `json:"file_categories,omitempty"`
	Fragments      []FragmentReference `json:"fragments,omitempty"`
	FragmentIDs    []string            `json:"fragment_ids,omitempty"`
}

type FileCategory struct {
	Path     string `json:"path"`
	Category string `json:"category"`
}

// FragmentReference is the semantic annotation attached to a fragment in a
// group. FragmentIDs remains supported for groups.json files created before
// descriptions were introduced.
type FragmentReference struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

func (g SemanticGroup) FragmentReferences() []FragmentReference {
	if len(g.Fragments) > 0 {
		return g.Fragments
	}
	refs := make([]FragmentReference, 0, len(g.FragmentIDs))
	for _, id := range g.FragmentIDs {
		refs = append(refs, FragmentReference{ID: id})
	}
	return refs
}

type GroupsFile struct {
	Version int             `json:"version"`
	BaseSHA string          `json:"base_sha"`
	HeadSHA string          `json:"head_sha"`
	Groups  []SemanticGroup `json:"groups"`
}

type Inventory struct {
	BaseSHA   string         `json:"base_sha"`
	HeadSHA   string         `json:"head_sha"`
	Fragments []DiffFragment `json:"fragments"`
}
