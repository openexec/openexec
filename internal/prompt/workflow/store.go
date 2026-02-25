package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Store loads and caches workflow templates from a directory.
type Store struct {
	dir   string
	cache map[string]*Template
	mu    sync.RWMutex
}

// NewStore creates a new store that loads workflow files from dir.
func NewStore(dir string) *Store {
	return &Store{
		dir:   dir,
		cache: make(map[string]*Template),
	}
}

// Get returns the workflow template for the given ID, loading and caching it
// on first access.
func (s *Store) Get(id string) (*Template, error) {
	s.mu.RLock()
	if t, ok := s.cache[id]; ok {
		s.mu.RUnlock()
		return t, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock.
	if t, ok := s.cache[id]; ok {
		return t, nil
	}

	path := filepath.Join(s.dir, id+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("workflow %q: %w", id, err)
	}

	t, err := parse(string(data), id)
	if err != nil {
		return nil, fmt.Errorf("workflow %q: %w", id, err)
	}

	s.cache[id] = t
	return t, nil
}

// parse extracts params from frontmatter and instructions/process from body.
func parse(content, id string) (*Template, error) {
	t := &Template{ID: id}

	// Parse YAML frontmatter for params.
	t.Params = parseParams(content)

	// Extract body content (after frontmatter).
	body := stripFrontmatter(content)

	t.Instructions = extractTag(body, "instructions")
	t.Process = extractTag(body, "process")

	if t.Instructions == "" && t.Process == "" {
		return nil, fmt.Errorf("no <instructions> or <process> tags found")
	}

	return t, nil
}

// parseParams extracts the params map from YAML frontmatter.
// Returns nil if no params declared.
func parseParams(content string) map[string]string {
	const sep = "---"
	if !strings.HasPrefix(content, sep) {
		return nil
	}

	rest := content[len(sep):]
	end := strings.Index(rest, sep)
	if end < 0 {
		return nil
	}

	fm := rest[:end]

	// Find "params:" line.
	inParams := false
	params := make(map[string]string)
	for _, line := range strings.Split(fm, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "params:" {
			inParams = true
			continue
		}
		if inParams {
			// Param lines are indented: "  name: description"
			if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') && strings.Contains(trimmed, ":") {
				k, v, _ := strings.Cut(trimmed, ":")
				k = strings.TrimSpace(k)
				v = strings.TrimSpace(v)
				v = strings.Trim(v, "\"")
				if k != "" {
					params[k] = v
				}
			} else {
				// No longer in params block.
				break
			}
		}
	}

	if len(params) == 0 {
		return nil
	}
	return params
}

// stripFrontmatter returns content after the YAML frontmatter.
func stripFrontmatter(content string) string {
	const sep = "---"
	if !strings.HasPrefix(content, sep) {
		return content
	}

	rest := content[len(sep):]
	end := strings.Index(rest, sep)
	if end < 0 {
		return content
	}

	return rest[end+len(sep):]
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
