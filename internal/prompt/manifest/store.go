package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Store loads and caches agent manifests from a directory.
type Store struct {
	dir   string
	cache map[string]*Manifest
	mu    sync.RWMutex
}

// NewStore creates a new store that loads manifest YAML files from dir.
func NewStore(dir string) *Store {
	return &Store{
		dir:   dir,
		cache: make(map[string]*Manifest),
	}
}

// Get returns the manifest for the given agent name, loading and caching it
// on first access.
func (s *Store) Get(name string) (*Manifest, error) {
	s.mu.RLock()
	if m, ok := s.cache[name]; ok {
		s.mu.RUnlock()
		return m, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock.
	if m, ok := s.cache[name]; ok {
		return m, nil
	}

	path := filepath.Join(s.dir, name+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("manifest %q: %w", name, err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("manifest %q: %w", name, err)
	}

	s.cache[name] = &m
	return &m, nil
}
