package persona

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Store loads and caches persona definitions from a directory.
type Store struct {
	dir   string
	cache map[string]*Persona
	mu    sync.RWMutex
}

// NewStore creates a new store that loads persona files from dir.
func NewStore(dir string) *Store {
	return &Store{
		dir:   dir,
		cache: make(map[string]*Persona),
	}
}

// Get returns the persona for the given name, loading and caching it on first
// access. If the persona declares extends, the base persona is loaded and merged.
func (s *Store) Get(name string) (*Persona, error) {
	s.mu.RLock()
	if p, ok := s.cache[name]; ok {
		s.mu.RUnlock()
		return p, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock.
	if p, ok := s.cache[name]; ok {
		return p, nil
	}

	p, extends, err := s.load(name)
	if err != nil {
		return nil, err
	}

	if extends != "" {
		base, _, err := s.load(extends)
		if err != nil {
			return nil, fmt.Errorf("persona %q: loading base %q: %w", name, extends, err)
		}
		p = merge(base, p)
	}

	s.cache[name] = p
	return p, nil
}

// load reads and parses a persona file without merging.
func (s *Store) load(name string) (*Persona, string, error) {
	path := filepath.Join(s.dir, name+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("persona %q: %w", name, err)
	}
	return parse(string(data))
}

// parse extracts the extends field and persona tags from file content.
func parse(content string) (*Persona, string, error) {
	extends := parseFrontmatterField(content, "extends")

	p := &Persona{
		Role:               extractTag(content, "role"),
		Identity:           extractTag(content, "identity"),
		CommunicationStyle: extractTag(content, "communication_style"),
		Principles:         extractTag(content, "principles"),
	}

	return p, extends, nil
}

// merge combines a base persona with an agent persona.
// Principles: base prepended to agent. Other fields: agent only.
func merge(base, agent *Persona) *Persona {
	merged := &Persona{
		Role:               agent.Role,
		Identity:           agent.Identity,
		CommunicationStyle: agent.CommunicationStyle,
		Principles:         agent.Principles,
	}

	if base.Principles != "" && agent.Principles != "" {
		merged.Principles = base.Principles + "\n" + agent.Principles
	} else if base.Principles != "" {
		merged.Principles = base.Principles
	}

	return merged
}

// parseFrontmatterField extracts a single field value from YAML frontmatter.
func parseFrontmatterField(content, field string) string {
	const sep = "---"
	if !strings.HasPrefix(content, sep) {
		return ""
	}

	rest := content[len(sep):]
	end := strings.Index(rest, sep)
	if end < 0 {
		return ""
	}

	fm := rest[:end]
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if k, v, ok := strings.Cut(line, ":"); ok {
			if strings.TrimSpace(k) == field {
				v = strings.TrimSpace(v)
				v = strings.Trim(v, "\"")
				return v
			}
		}
	}
	return ""
}

// extractTag extracts content between <tag> and </tag> markers.
func extractTag(content, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"

	start := strings.Index(content, open)
	if start < 0 {
		return ""
	}
	start += len(open)

	end := strings.Index(content[start:], close)
	if end < 0 {
		return ""
	}

	return strings.TrimSpace(content[start : start+end])
}
