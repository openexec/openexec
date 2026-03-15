package blueprint

import (
	"fmt"
	"sort"
	"sync"
)

// Registry stores and retrieves blueprints by ID.
type Registry struct {
	mu         sync.RWMutex
	blueprints map[string]*Blueprint
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		blueprints: make(map[string]*Blueprint),
	}
}

// Register adds a blueprint to the registry.
// It returns an error if the blueprint ID is already registered
// or if the blueprint is invalid.
func (r *Registry) Register(bp *Blueprint) error {
	if err := bp.Validate(); err != nil {
		return fmt.Errorf("invalid blueprint: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.blueprints[bp.ID]; exists {
		return fmt.Errorf("blueprint %q already registered", bp.ID)
	}

	r.blueprints[bp.ID] = bp
	return nil
}

// Get retrieves a blueprint by ID.
// Returns nil and false if the blueprint is not found.
func (r *Registry) Get(id string) (*Blueprint, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	bp, ok := r.blueprints[id]
	return bp, ok
}

// List returns all registered blueprints sorted by ID.
func (r *Registry) List() []*Blueprint {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Blueprint, 0, len(r.blueprints))
	for _, bp := range r.blueprints {
		result = append(result, bp)
	}

	// Sort by ID for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result
}

// MustRegister registers a blueprint and panics on error.
// This is useful for registering built-in blueprints at init time.
func (r *Registry) MustRegister(bp *Blueprint) {
	if err := r.Register(bp); err != nil {
		panic(err)
	}
}

// defaultRegistry is the singleton registry with built-in blueprints.
var (
	defaultRegistry     *Registry
	defaultRegistryOnce sync.Once
)

// DefaultRegistry returns the singleton registry with default blueprints registered.
// The registry is initialized lazily on first call.
func DefaultRegistry() *Registry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = NewRegistry()
		defaultRegistry.MustRegister(DefaultBlueprint)
		defaultRegistry.MustRegister(QuickFixBlueprint)
	})
	return defaultRegistry
}
