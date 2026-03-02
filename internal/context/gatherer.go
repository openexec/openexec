// Package context provides automatic context gathering and injection for AI agent sessions.
package context

import (
	"context"
	"encoding/json"
	"time"
)

// Gatherer is the interface that all context gatherers must implement.
// Gatherers are responsible for collecting specific types of context from the project workspace.
type Gatherer interface {
	// Type returns the context type this gatherer produces.
	Type() ContextType

	// Name returns a human-readable name for the gatherer.
	Name() string

	// Description returns a description of what this gatherer collects.
	Description() string

	// Gather collects context and returns a ContextItem.
	// The ctx parameter can be used for cancellation.
	// The projectPath is the root directory of the project to gather context from.
	Gather(ctx context.Context, projectPath string) (*ContextItem, error)

	// Configure applies a GathererConfig to this gatherer.
	Configure(config *GathererConfig) error
}

// GathererFunc is a function type that implements the Gather method.
type GathererFunc func(ctx context.Context, projectPath string) (*ContextItem, error)

// BaseGatherer provides common functionality for all gatherers.
type BaseGatherer struct {
	contextType  ContextType
	name         string
	description  string
	maxTokens    int
	priority     Priority
	filePaths    []string
	filePatterns []string
	options      map[string]interface{}
}

// NewBaseGatherer creates a new BaseGatherer with the given parameters.
func NewBaseGatherer(contextType ContextType, name, description string) *BaseGatherer {
	return &BaseGatherer{
		contextType: contextType,
		name:        name,
		description: description,
		maxTokens:   4000, // Default max tokens
		priority:    DefaultPriorityForType(contextType),
		options:     make(map[string]interface{}),
	}
}

// Type returns the context type.
func (b *BaseGatherer) Type() ContextType {
	return b.contextType
}

// Name returns the gatherer name.
func (b *BaseGatherer) Name() string {
	return b.name
}

// Description returns the gatherer description.
func (b *BaseGatherer) Description() string {
	return b.description
}

// MaxTokens returns the maximum tokens for this gatherer.
func (b *BaseGatherer) MaxTokens() int {
	return b.maxTokens
}

// Priority returns the injection priority.
func (b *BaseGatherer) Priority() Priority {
	return b.priority
}

// FilePaths returns the configured file paths.
func (b *BaseGatherer) FilePaths() []string {
	return b.filePaths
}

// FilePatterns returns the configured file patterns.
func (b *BaseGatherer) FilePatterns() []string {
	return b.filePatterns
}

// Options returns the gatherer options.
func (b *BaseGatherer) Options() map[string]interface{} {
	return b.options
}

// GetOption retrieves an option value with a default.
func (b *BaseGatherer) GetOption(key string, defaultValue interface{}) interface{} {
	if val, ok := b.options[key]; ok {
		return val
	}
	return defaultValue
}

// GetIntOption retrieves an integer option with a default.
func (b *BaseGatherer) GetIntOption(key string, defaultValue int) int {
	val := b.GetOption(key, defaultValue)
	switch v := val.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case int64:
		return int(v)
	default:
		return defaultValue
	}
}

// GetStringOption retrieves a string option with a default.
func (b *BaseGatherer) GetStringOption(key string, defaultValue string) string {
	val := b.GetOption(key, defaultValue)
	if s, ok := val.(string); ok {
		return s
	}
	return defaultValue
}

// GetBoolOption retrieves a boolean option with a default.
func (b *BaseGatherer) GetBoolOption(key string, defaultValue bool) bool {
	val := b.GetOption(key, defaultValue)
	if bv, ok := val.(bool); ok {
		return bv
	}
	return defaultValue
}

// GetStringSliceOption retrieves a string slice option with a default.
func (b *BaseGatherer) GetStringSliceOption(key string, defaultValue []string) []string {
	val := b.GetOption(key, defaultValue)
	switch v := val.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return defaultValue
	}
}

// Configure applies a GathererConfig to this gatherer.
func (b *BaseGatherer) Configure(config *GathererConfig) error {
	if config == nil {
		return nil
	}

	b.maxTokens = config.MaxTokens
	b.priority = config.Priority

	// Parse file paths
	if config.FilePaths != "" {
		var paths []string
		if err := json.Unmarshal([]byte(config.FilePaths), &paths); err == nil {
			b.filePaths = paths
		}
	}

	// Parse file patterns
	if config.FilePatterns != "" {
		var patterns []string
		if err := json.Unmarshal([]byte(config.FilePatterns), &patterns); err == nil {
			b.filePatterns = patterns
		}
	}

	// Parse options
	if config.Options != "" {
		var opts map[string]interface{}
		if err := json.Unmarshal([]byte(config.Options), &opts); err == nil {
			b.options = opts
		}
	}

	return nil
}

// CreateContextItem creates a new ContextItem from gathered content.
func (b *BaseGatherer) CreateContextItem(source, content string, tokenCount int) (*ContextItem, error) {
	item, err := NewContextItem(b.contextType, source, content, tokenCount)
	if err != nil {
		return nil, err
	}
	item.SetPriority(b.priority)
	return item, nil
}

// EstimateTokens provides a simple token count estimation.
// Uses a rough heuristic of ~4 characters per token.
func EstimateTokens(content string) int {
	if content == "" {
		return 0
	}
	// Simple heuristic: ~4 characters per token on average
	return (len(content) + 3) / 4
}

// TruncateToTokenLimit truncates content to fit within a token limit.
func TruncateToTokenLimit(content string, maxTokens int) string {
	if maxTokens <= 0 {
		return content
	}

	estimated := EstimateTokens(content)
	if estimated <= maxTokens {
		return content
	}

	// Calculate approximate character limit
	maxChars := maxTokens * 4
	if maxChars >= len(content) {
		return content
	}

	// Truncate and add indicator
	truncated := content[:maxChars-20] // Leave room for truncation message
	return truncated + "\n... [truncated]"
}

// GathererResult holds the result of a gather operation along with metadata.
type GathererResult struct {
	// Item is the gathered context item.
	Item *ContextItem
	// Duration is how long the gather operation took.
	Duration time.Duration
	// Error contains any error that occurred.
	Error error
}

// GathererRunner executes gatherers and tracks their execution.
type GathererRunner struct {
	gatherers []Gatherer
}

// NewGathererRunner creates a new GathererRunner.
func NewGathererRunner(gatherers ...Gatherer) *GathererRunner {
	return &GathererRunner{
		gatherers: gatherers,
	}
}

// AddGatherer adds a gatherer to the runner.
func (r *GathererRunner) AddGatherer(g Gatherer) {
	r.gatherers = append(r.gatherers, g)
}

// Gatherers returns all registered gatherers.
func (r *GathererRunner) Gatherers() []Gatherer {
	return r.gatherers
}

// RunAll executes all gatherers and returns their results.
func (r *GathererRunner) RunAll(ctx context.Context, projectPath string) []GathererResult {
	results := make([]GathererResult, 0, len(r.gatherers))

	for _, g := range r.gatherers {
		result := r.Run(ctx, g, projectPath)
		results = append(results, result)
	}

	return results
}

// Run executes a single gatherer and returns the result.
func (r *GathererRunner) Run(ctx context.Context, g Gatherer, projectPath string) GathererResult {
	start := time.Now()

	item, err := g.Gather(ctx, projectPath)
	duration := time.Since(start)

	return GathererResult{
		Item:     item,
		Duration: duration,
		Error:    err,
	}
}

// RunByType executes gatherers of a specific type.
func (r *GathererRunner) RunByType(ctx context.Context, projectPath string, contextType ContextType) []GathererResult {
	results := make([]GathererResult, 0)

	for _, g := range r.gatherers {
		if g.Type() == contextType {
			result := r.Run(ctx, g, projectPath)
			results = append(results, result)
		}
	}

	return results
}
