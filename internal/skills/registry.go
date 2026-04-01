package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Registry manages a collection of skills loaded from various sources.
type Registry struct {
	mu     sync.RWMutex
	skills map[string]*Skill
}

// NewRegistry creates an empty skill registry.
func NewRegistry() *Registry {
	return &Registry{
		skills: make(map[string]*Skill),
	}
}

// LoadFromDir scans a directory for subdirectories containing SKILL.md files.
// Each subdirectory name becomes the skill name if the skill has no name in
// its frontmatter. source identifies the origin ("builtin", "user", "project", "imported").
func (r *Registry) LoadFromDir(dir, source string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Directory doesn't exist yet — not an error.
		}
		return fmt.Errorf("read skills dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillPath := filepath.Join(dir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillPath); err != nil {
			continue // No SKILL.md in this subdirectory.
		}

		skill, err := ParseSkillFile(skillPath)
		if err != nil {
			continue // Skip unparseable files.
		}

		if skill.Name == "" {
			skill.Name = entry.Name()
		}
		skill.Source = source

		r.mu.Lock()
		r.skills[skill.Name] = skill
		r.mu.Unlock()
	}

	return nil
}

// LoadAll loads skills from the standard locations in priority order:
//  1. ~/.openexec/skills/builtin/
//  2. ~/.openexec/skills/user/
//  3. ~/.openexec/skills/imported/
//  4. <projectDir>/.openexec/skills/ (project-specific)
//
// Later sources override earlier ones if names collide.
func (r *Registry) LoadAll(projectDir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	if home != "" {
		base := filepath.Join(home, ".openexec", "skills")
		_ = r.LoadFromDir(filepath.Join(base, "builtin"), "builtin")
		_ = r.LoadFromDir(filepath.Join(base, "user"), "user")
		_ = r.LoadFromDir(filepath.Join(base, "imported"), "imported")
	}

	if projectDir != "" {
		projectSkills := filepath.Join(projectDir, ".openexec", "skills")
		_ = r.LoadFromDir(projectSkills, "project")
	}

	return nil
}

// Get retrieves a skill by name.
func (r *Registry) Get(name string) (*Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[name]
	return s, ok
}

// List returns all registered skills sorted by name.
func (r *Registry) List() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Skill, 0, len(r.skills))
	for _, s := range r.skills {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// ListByCategory returns all skills matching the given category.
func (r *Registry) ListByCategory(category string) []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cat := strings.ToLower(category)
	var result []*Skill
	for _, s := range r.skills {
		for _, c := range s.Categories {
			if strings.ToLower(c) == cat {
				result = append(result, s)
				break
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// Search performs a text search across skill name, description, tags, and content.
// Returns skills that contain the query string (case-insensitive).
func (r *Registry) Search(query string) []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	q := strings.ToLower(query)
	var result []*Skill
	for _, s := range r.skills {
		if matchesQuery(s, q) {
			result = append(result, s)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// SelectForTask selects the most relevant skills for a given task description.
// It scores skills based on keyword matches in categories, tags, when_to_use,
// name, and description. Returns the top 3 enabled skills by score.
func (r *Registry) SelectForTask(query string) []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if query == "" {
		return nil
	}

	words := strings.Fields(strings.ToLower(query))

	type scored struct {
		skill *Skill
		score int
	}

	var candidates []scored
	for _, s := range r.skills {
		if !s.Enabled {
			continue
		}
		score := scoreSkill(s, words)
		if score > 0 {
			candidates = append(candidates, scored{skill: s, score: score})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].skill.Name < candidates[j].skill.Name
	})

	limit := 3
	if len(candidates) < limit {
		limit = len(candidates)
	}

	result := make([]*Skill, limit)
	for i := 0; i < limit; i++ {
		result[i] = candidates[i].skill
	}
	return result
}

// Enable enables a skill by name.
func (r *Registry) Enable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.skills[name]
	if !ok {
		return fmt.Errorf("skill %q not found", name)
	}
	s.Enabled = true
	return nil
}

// Disable disables a skill by name.
func (r *Registry) Disable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.skills[name]
	if !ok {
		return fmt.Errorf("skill %q not found", name)
	}
	s.Enabled = false
	return nil
}

// matchesQuery checks if a skill matches a search query (case-insensitive).
func matchesQuery(s *Skill, q string) bool {
	if strings.Contains(strings.ToLower(s.Name), q) {
		return true
	}
	if strings.Contains(strings.ToLower(s.Description), q) {
		return true
	}
	for _, tag := range s.Tags {
		if strings.Contains(strings.ToLower(tag), q) {
			return true
		}
	}
	if strings.Contains(strings.ToLower(s.Content), q) {
		return true
	}
	return false
}

// scoreSkill scores a skill against query words. Higher scores indicate
// better relevance. Categories and tags get higher weight than content.
func scoreSkill(s *Skill, words []string) int {
	score := 0
	nameLower := strings.ToLower(s.Name)
	descLower := strings.ToLower(s.Description)
	whenLower := strings.ToLower(s.WhenToUse)

	catsLower := make([]string, len(s.Categories))
	for i, c := range s.Categories {
		catsLower[i] = strings.ToLower(c)
	}
	tagsLower := make([]string, len(s.Tags))
	for i, t := range s.Tags {
		tagsLower[i] = strings.ToLower(t)
	}

	for _, w := range words {
		// Name match: highest weight
		if strings.Contains(nameLower, w) {
			score += 5
		}
		// Category match: high weight
		for _, c := range catsLower {
			if strings.Contains(c, w) {
				score += 4
				break
			}
		}
		// Tag match: high weight
		for _, t := range tagsLower {
			if strings.Contains(t, w) {
				score += 3
				break
			}
		}
		// WhenToUse match: medium weight
		if strings.Contains(whenLower, w) {
			score += 2
		}
		// Description match: medium weight
		if strings.Contains(descLower, w) {
			score += 2
		}
	}

	// Priority boost
	switch strings.ToLower(s.Priority) {
	case "high":
		score += 2
	case "medium":
		score += 1
	}

	return score
}
