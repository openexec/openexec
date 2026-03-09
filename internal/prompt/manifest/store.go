package manifest

import (
	"fmt"
	"io/fs"
	"sync"

	"gopkg.in/yaml.v3"
)

// Store loads and caches agent manifests from a filesystem.
type Store struct {
	fs    fs.FS
	cache map[string]*Manifest
	mu    sync.RWMutex
}

// NewStore creates a new store that loads manifest YAML files from the given filesystem.
func NewStore(f fs.FS) *Store {
	return &Store{
		fs:    f,
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

	path := name + ".yaml"
	data, err := fs.ReadFile(s.fs, path)
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
