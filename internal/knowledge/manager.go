package knowledge

import (
	"fmt"
	"sync"
)

// Manager coordinates knowledge stores for multiple projects.
// This allows the UI to maintain context when switching between workspaces.
type Manager struct {
	mu     sync.RWMutex
	stores map[string]*Store // projectPath -> Store
}

func NewManager() *Manager {
	return &Manager{
		stores: make(map[string]*Store),
	}
}

// GetStore returns the knowledge store for a specific project.
// If the store isn't open, it initializes it.
func (m *Manager) GetStore(projectPath string) (*Store, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.stores[projectPath]; ok {
		return s, nil
	}

	s, err := NewStore(projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open store for project %s: %w", projectPath, err)
	}

	m.stores[projectPath] = s
	return s, nil
}

// CloseAll safely shuts down all active database connections.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for path, s := range m.stores {
		_ = s.Close()
		delete(m.stores, path)
	}
}
