package actions

import (
	"fmt"
	"sync"
)

// Registry stores and retrieves registered actions.
type Registry struct {
	mu      sync.RWMutex
	actions map[string]Action
}

// NewRegistry creates a new empty action registry.
func NewRegistry() *Registry {
	return &Registry{
		actions: make(map[string]Action),
	}
}

// Register adds an action to the registry.
func (r *Registry) Register(a Action) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.actions[a.Name()]; exists {
		return fmt.Errorf("action %q already registered", a.Name())
	}

	r.actions[a.Name()] = a
	return nil
}

// Get retrieves an action by name.
func (r *Registry) Get(name string) (Action, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	a, ok := r.actions[name]
	return a, ok
}

// DefaultRegistry returns a registry populated with built-in actions.
func DefaultRegistry(projectDir string) *Registry {
	r := NewRegistry()
	_ = r.Register(NewRunGatesAction(projectDir))
	_ = r.Register(NewBuildContextAction(projectDir))
	_ = r.Register(NewApplyPatchAction(projectDir))
	return r
}
