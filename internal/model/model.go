package model

type Range struct {
	Start int `json:"start"`
	Lines int `json:"lines"`
}

// FragmentRange selects a region from either or both sides of a file diff.
// A nil side is used for pure additions or deletions.
type FragmentRange struct {
	Old *Range `json:"old,omitempty"`
	New *Range `json:"new,omitempty"`
}

type DiffChange struct {
	ID       string `json:"id"`
	BaseSHA  string `json:"base_sha"`
	HeadSHA  string `json:"head_sha"`
	Path     string `json:"path"`
	OldStart int    `json:"old_start"`
	OldLines int    `json:"old_lines"`
	NewStart int    `json:"new_start"`
	NewLines int    `json:"new_lines"`
	Metadata bool   `json:"metadata,omitempty"`
	Patch    string `json:"patch,omitempty"`
}

type MaterializedFragment struct {
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
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	Summary        string         `json:"summary"`
	Order          *int           `json:"order,omitempty"`
	FileCategories []FileCategory `json:"file_categories,omitempty"`
	Fragments      []Fragment     `json:"fragments"`
}

type FileCategory struct {
	Path     string `json:"path"`
	Category string `json:"category"`
}

// Fragment is a semantic, file-local selection of changed lines. Ranges are
// the source of truth; ID is only a stable handle within a draft/groups file.
type Fragment struct {
	ID           string          `json:"id"`
	Path         string          `json:"path,omitempty"`
	Ranges       []FragmentRange `json:"ranges,omitempty"`
	FileMetadata bool            `json:"file_metadata,omitempty"`
	Description  string          `json:"description,omitempty"`
}

type GroupsFile struct {
	Version int             `json:"version"`
	BaseSHA string          `json:"base_sha"`
	HeadSHA string          `json:"head_sha"`
	Groups  []SemanticGroup `json:"groups"`
}

type ChangeMap struct {
	BaseSHA string       `json:"base_sha"`
	HeadSHA string       `json:"head_sha"`
	Changes []DiffChange `json:"changes"`
}

type FragmentSet struct {
	BaseSHA   string                 `json:"base_sha"`
	HeadSHA   string                 `json:"head_sha"`
	Fragments []MaterializedFragment `json:"fragments"`
}
