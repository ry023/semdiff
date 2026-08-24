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
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Order       *int     `json:"order,omitempty"`
	FragmentIDs []string `json:"fragment_ids"`
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
