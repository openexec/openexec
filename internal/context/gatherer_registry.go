// Package context provides automatic context gathering and injection for AI agent sessions.
package context

import (
	"context"
	"fmt"
	"sync"
)

// GathererRegistry manages the registration and lookup of context gatherers.
type GathererRegistry struct {
	mu        sync.RWMutex
	gatherers map[ContextType]Gatherer
}

// NewGathererRegistry creates a new empty GathererRegistry.
func NewGathererRegistry() *GathererRegistry {
	return &GathererRegistry{
		gatherers: make(map[ContextType]Gatherer),
	}
}

// Register adds a gatherer to the registry.
// If a gatherer for the same type already exists, it will be replaced.
func (r *GathererRegistry) Register(g Gatherer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gatherers[g.Type()] = g
}

// Get retrieves a gatherer by its context type.
func (r *GathererRegistry) Get(contextType ContextType) (Gatherer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.gatherers[contextType]
	return g, ok
}

// GetAll returns all registered gatherers.
func (r *GathererRegistry) GetAll() []Gatherer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Gatherer, 0, len(r.gatherers))
	for _, g := range r.gatherers {
		result = append(result, g)
	}
	return result
}

// Types returns all registered context types.
func (r *GathererRegistry) Types() []ContextType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]ContextType, 0, len(r.gatherers))
	for t := range r.gatherers {
		types = append(types, t)
	}
	return types
}

// Remove removes a gatherer from the registry.
func (r *GathererRegistry) Remove(contextType ContextType) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.gatherers, contextType)
}

// Count returns the number of registered gatherers.
func (r *GathererRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.gatherers)
}

// RunAll executes all registered gatherers and returns the results.
func (r *GathererRegistry) RunAll(ctx context.Context, projectPath string) []GathererResult {
	r.mu.RLock()
	gatherers := make([]Gatherer, 0, len(r.gatherers))
	for _, g := range r.gatherers {
		gatherers = append(gatherers, g)
	}
	r.mu.RUnlock()

	runner := NewGathererRunner(gatherers...)
	return runner.RunAll(ctx, projectPath)
}

// RunByType executes a specific gatherer by type.
func (r *GathererRegistry) RunByType(ctx context.Context, projectPath string, contextType ContextType) (*GathererResult, error) {
	g, ok := r.Get(contextType)
	if !ok {
		return nil, fmt.Errorf("no gatherer registered for type: %s", contextType)
	}

	runner := NewGathererRunner()
	result := runner.Run(ctx, g, projectPath)
	return &result, nil
}

// DefaultRegistry returns a registry with all default gatherers registered.
func DefaultRegistry() *GathererRegistry {
	registry := NewGathererRegistry()

	// Register all default gatherers
	registry.Register(NewProjectInstructionsGatherer())
	registry.Register(NewGitStatusGatherer())
	registry.Register(NewGitDiffGatherer())
	registry.Register(NewGitLogGatherer())
	registry.Register(NewEnvironmentGatherer())
	registry.Register(NewDirectoryStructureGatherer())
	registry.Register(NewRecentFilesGatherer())
	registry.Register(NewPackageInfoGatherer())

	return registry
}

// ConfiguredRegistry returns a registry with gatherers configured from GathererConfigs.
func ConfiguredRegistry(configs []GathererConfig) *GathererRegistry {
	registry := NewGathererRegistry()

	for _, config := range configs {
		if !config.IsEnabled {
			continue
		}

		var g Gatherer
		switch config.Type {
		case ContextTypeProjectInstructions:
			g = NewProjectInstructionsGatherer()
		case ContextTypeGitStatus:
			g = NewGitStatusGatherer()
		case ContextTypeGitDiff:
			g = NewGitDiffGatherer()
		case ContextTypeGitLog:
			g = NewGitLogGatherer()
		case ContextTypeEnvironment:
			g = NewEnvironmentGatherer()
		case ContextTypeDirectoryStructure:
			g = NewDirectoryStructureGatherer()
		case ContextTypeRecentFiles:
			g = NewRecentFilesGatherer()
		case ContextTypePackageInfo:
			g = NewPackageInfoGatherer()
		default:
			continue
		}

		// Apply configuration
		if configurable, ok := g.(interface{ Configure(*GathererConfig) error }); ok {
			_ = configurable.Configure(&config)
		}

		registry.Register(g)
	}

	return registry
}

// GathererFactory creates gatherers based on context type.
type GathererFactory struct{}

// NewGathererFactory creates a new GathererFactory.
func NewGathererFactory() *GathererFactory {
	return &GathererFactory{}
}

// Create creates a new gatherer for the given context type.
func (f *GathererFactory) Create(contextType ContextType) (Gatherer, error) {
	switch contextType {
	case ContextTypeProjectInstructions:
		return NewProjectInstructionsGatherer(), nil
	case ContextTypeGitStatus:
		return NewGitStatusGatherer(), nil
	case ContextTypeGitDiff:
		return NewGitDiffGatherer(), nil
	case ContextTypeGitLog:
		return NewGitLogGatherer(), nil
	case ContextTypeEnvironment:
		return NewEnvironmentGatherer(), nil
	case ContextTypeDirectoryStructure:
		return NewDirectoryStructureGatherer(), nil
	case ContextTypeRecentFiles:
		return NewRecentFilesGatherer(), nil
	case ContextTypePackageInfo:
		return NewPackageInfoGatherer(), nil
	default:
		return nil, fmt.Errorf("unknown context type: %s", contextType)
	}
}

// CreateAll creates all available gatherers.
func (f *GathererFactory) CreateAll() []Gatherer {
	return []Gatherer{
		NewProjectInstructionsGatherer(),
		NewGitStatusGatherer(),
		NewGitDiffGatherer(),
		NewGitLogGatherer(),
		NewEnvironmentGatherer(),
		NewDirectoryStructureGatherer(),
		NewRecentFilesGatherer(),
		NewPackageInfoGatherer(),
	}
}

// AvailableTypes returns all context types that can be created.
func (f *GathererFactory) AvailableTypes() []ContextType {
	return []ContextType{
		ContextTypeProjectInstructions,
		ContextTypeGitStatus,
		ContextTypeGitDiff,
		ContextTypeGitLog,
		ContextTypeEnvironment,
		ContextTypeDirectoryStructure,
		ContextTypeRecentFiles,
		ContextTypePackageInfo,
	}
}
