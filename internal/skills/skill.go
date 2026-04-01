package skills

// Skill represents a reusable knowledge/instruction module that can be
// injected into pipeline context. Skills are parsed from SKILL.md files
// containing YAML frontmatter and a markdown body.
type Skill struct {
	Name          string   `yaml:"name" json:"name"`
	Description   string   `yaml:"description" json:"description"`
	Categories    []string `yaml:"categories" json:"categories"`
	Tags          []string `yaml:"tags" json:"tags"`
	WhenToUse     string   `yaml:"when_to_use" json:"when_to_use"`
	Priority      string   `yaml:"priority" json:"priority"`
	HasSearch     bool     `yaml:"has_search_engine" json:"has_search_engine"`
	SearchCommand string   `yaml:"search_command" json:"search_command"`
	Content       string   `json:"content"`     // Markdown body after frontmatter
	SourcePath    string   `json:"source_path"` // File path
	Source        string   `json:"source"`      // "builtin", "user", "project", "imported"
	Enabled       bool     `json:"enabled"`     // default true
}
