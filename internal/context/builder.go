// Package context provides automatic context gathering and injection for AI agent sessions.
package context

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// BuilderResult represents the result of building context with token budgeting.
type BuilderResult struct {
	// Context is the final assembled context string.
	Context string `json:"context"`
	// TotalTokens is the total tokens used by the included context.
	TotalTokens int `json:"total_tokens"`
	// IncludedItems contains the context items that were included.
	IncludedItems []*ContextItem `json:"included_items"`
	// ExcludedItems contains items that were excluded due to budget constraints.
	ExcludedItems []*ContextItem `json:"excluded_items"`
	// Errors contains any errors encountered during gathering.
	Errors []GatherError `json:"errors,omitempty"`
	// Budget is the budget configuration used.
	Budget *ContextBudget `json:"budget"`
	// TokensAvailable is the token budget that was available for context.
	TokensAvailable int `json:"tokens_available"`
	// TokensRemaining is how many tokens remain unused.
	TokensRemaining int `json:"tokens_remaining"`
}

// GatherError represents an error from a specific gatherer.
type GatherError struct {
	// Type is the context type that failed.
	Type ContextType `json:"type"`
	// Name is the gatherer name.
	Name string `json:"name"`
	// Error is the error message.
	Error string `json:"error"`
}

// ContextBuilder assembles context from multiple gatherers while respecting token budgets.
type ContextBuilder struct {
	registry     *GathererRegistry
	budget       *ContextBudget
	maxPerType   map[ContextType]int
	formatHeader bool
	separator    string
}

// NewContextBuilder creates a new ContextBuilder with the given registry and budget.
func NewContextBuilder(registry *GathererRegistry, budget *ContextBudget) *ContextBuilder {
	if budget == nil {
		budget = DefaultContextBudget()
	}
	if registry == nil {
		registry = DefaultRegistry()
	}

	builder := &ContextBuilder{
		registry:     registry,
		budget:       budget,
		maxPerType:   make(map[ContextType]int),
		formatHeader: true,
		separator:    "\n\n---\n\n",
	}

	// Parse max per type from budget
	builder.parseMaxPerType()

	return builder
}

// parseMaxPerType parses the MaxPerType JSON from the budget.
func (b *ContextBuilder) parseMaxPerType() {
	if b.budget.MaxPerType == "" {
		return
	}

	var maxPerType map[string]int
	if err := json.Unmarshal([]byte(b.budget.MaxPerType), &maxPerType); err != nil {
		return
	}

	for typeStr, max := range maxPerType {
		b.maxPerType[ContextType(typeStr)] = max
	}
}

// WithFormatHeader sets whether to include headers for each context section.
func (b *ContextBuilder) WithFormatHeader(enabled bool) *ContextBuilder {
	b.formatHeader = enabled
	return b
}

// WithSeparator sets the separator between context sections.
func (b *ContextBuilder) WithSeparator(sep string) *ContextBuilder {
	b.separator = sep
	return b
}

// WithMaxPerType sets the maximum tokens allowed for a specific context type.
func (b *ContextBuilder) WithMaxPerType(contextType ContextType, maxTokens int) *ContextBuilder {
	b.maxPerType[contextType] = maxTokens
	return b
}

// Build gathers context from all registered gatherers and assembles it within budget.
func (b *ContextBuilder) Build(ctx context.Context, projectPath string) (*BuilderResult, error) {
	// Gather all context items
	results := b.registry.RunAll(ctx, projectPath)

	// Collect items and errors
	var items []*ContextItem
	var errors []GatherError

	for _, result := range results {
		if result.Error != nil {
			// Find the gatherer to get its info
			for _, g := range b.registry.GetAll() {
				if g.Type() == result.Item.Type || (result.Item != nil && g.Type() == result.Item.Type) {
					errors = append(errors, GatherError{
						Type:  g.Type(),
						Name:  g.Name(),
						Error: result.Error.Error(),
					})
					break
				}
			}
			// If we couldn't find the gatherer, still log the error
			if result.Item == nil {
				continue
			}
		}
		if result.Item != nil {
			items = append(items, result.Item)
		}
	}

	// Build context with budgeting
	return b.buildWithBudget(items, errors)
}

// BuildFromItems builds context from pre-gathered context items.
func (b *ContextBuilder) BuildFromItems(items []*ContextItem) (*BuilderResult, error) {
	return b.buildWithBudget(items, nil)
}

// buildWithBudget applies token budgeting and assembles the final context.
func (b *ContextBuilder) buildWithBudget(items []*ContextItem, errors []GatherError) (*BuilderResult, error) {
	// Calculate available tokens
	tokensAvailable := b.budget.AvailableForContext()

	// Filter by minimum priority
	var eligibleItems []*ContextItem
	for _, item := range items {
		if item.Priority >= b.budget.MinPriorityToInclude {
			eligibleItems = append(eligibleItems, item)
		}
	}

	// Sort by priority (highest first), then by token count (smallest first for efficiency)
	sort.Slice(eligibleItems, func(i, j int) bool {
		if eligibleItems[i].Priority != eligibleItems[j].Priority {
			return eligibleItems[i].Priority > eligibleItems[j].Priority
		}
		return eligibleItems[i].TokenCount < eligibleItems[j].TokenCount
	})

	// Track tokens used per type
	tokensPerType := make(map[ContextType]int)

	// Select items within budget
	var includedItems []*ContextItem
	var excludedItems []*ContextItem
	tokensUsed := 0

	for _, item := range eligibleItems {
		// Check if we have room in the overall budget
		if tokensUsed+item.TokenCount > tokensAvailable {
			excludedItems = append(excludedItems, item)
			continue
		}

		// Check per-type limits
		if maxForType, ok := b.maxPerType[item.Type]; ok {
			if tokensPerType[item.Type]+item.TokenCount > maxForType {
				excludedItems = append(excludedItems, item)
				continue
			}
		}

		// Include this item
		includedItems = append(includedItems, item)
		tokensUsed += item.TokenCount
		tokensPerType[item.Type] += item.TokenCount
	}

	// Build the final context string
	contextStr := b.formatContext(includedItems)

	return &BuilderResult{
		Context:         contextStr,
		TotalTokens:     tokensUsed,
		IncludedItems:   includedItems,
		ExcludedItems:   excludedItems,
		Errors:          errors,
		Budget:          b.budget,
		TokensAvailable: tokensAvailable,
		TokensRemaining: tokensAvailable - tokensUsed,
	}, nil
}

// formatContext formats the included items into a single context string.
func (b *ContextBuilder) formatContext(items []*ContextItem) string {
	if len(items) == 0 {
		return ""
	}

	var parts []string

	for _, item := range items {
		var section string
		if b.formatHeader {
			section = b.formatSection(item)
		} else {
			section = item.Content
		}
		parts = append(parts, section)
	}

	return strings.Join(parts, b.separator)
}

// formatSection formats a single context item with a header.
func (b *ContextBuilder) formatSection(item *ContextItem) string {
	header := formatContextHeader(item.Type)
	if item.Source != "" && item.Source != string(item.Type) {
		header = fmt.Sprintf("%s (%s)", header, item.Source)
	}
	return fmt.Sprintf("## %s\n\n%s", header, item.Content)
}

// formatContextHeader returns a human-readable header for a context type.
func formatContextHeader(t ContextType) string {
	switch t {
	case ContextTypeProjectInstructions:
		return "Project Instructions"
	case ContextTypeGitStatus:
		return "Git Status"
	case ContextTypeGitDiff:
		return "Git Diff"
	case ContextTypeGitLog:
		return "Git Log"
	case ContextTypeDirectoryStructure:
		return "Directory Structure"
	case ContextTypeRecentFiles:
		return "Recent Files"
	case ContextTypeOpenFiles:
		return "Open Files"
	case ContextTypePackageInfo:
		return "Package Info"
	case ContextTypeEnvironment:
		return "Environment"
	case ContextTypeSessionSummary:
		return "Session Summary"
	case ContextTypeCustom:
		return "Custom Context"
	default:
		return string(t)
	}
}

// ConcurrentContextBuilder gathers context concurrently for better performance.
type ConcurrentContextBuilder struct {
	*ContextBuilder
	maxConcurrency int
}

// NewConcurrentContextBuilder creates a new ConcurrentContextBuilder.
func NewConcurrentContextBuilder(registry *GathererRegistry, budget *ContextBudget) *ConcurrentContextBuilder {
	return &ConcurrentContextBuilder{
		ContextBuilder: NewContextBuilder(registry, budget),
		maxConcurrency: 4, // Default max concurrent gatherers
	}
}

// WithMaxConcurrency sets the maximum number of concurrent gatherers.
func (b *ConcurrentContextBuilder) WithMaxConcurrency(max int) *ConcurrentContextBuilder {
	if max > 0 {
		b.maxConcurrency = max
	}
	return b
}

// Build gathers context concurrently from all registered gatherers.
func (b *ConcurrentContextBuilder) Build(ctx context.Context, projectPath string) (*BuilderResult, error) {
	gatherers := b.registry.GetAll()
	if len(gatherers) == 0 {
		return &BuilderResult{
			Context:         "",
			TotalTokens:     0,
			IncludedItems:   nil,
			ExcludedItems:   nil,
			Budget:          b.budget,
			TokensAvailable: b.budget.AvailableForContext(),
			TokensRemaining: b.budget.AvailableForContext(),
		}, nil
	}

	// Create channels for work distribution
	type workItem struct {
		gatherer Gatherer
	}
	type resultItem struct {
		result GathererResult
	}

	workChan := make(chan workItem, len(gatherers))
	resultChan := make(chan resultItem, len(gatherers))

	// Determine actual concurrency
	concurrency := b.maxConcurrency
	if concurrency > len(gatherers) {
		concurrency = len(gatherers)
	}

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runner := NewGathererRunner()
			for work := range workChan {
				select {
				case <-ctx.Done():
					resultChan <- resultItem{
						result: GathererResult{
							Error: ctx.Err(),
						},
					}
				default:
					result := runner.Run(ctx, work.gatherer, projectPath)
					resultChan <- resultItem{result: result}
				}
			}
		}()
	}

	// Send work
	for _, g := range gatherers {
		workChan <- workItem{gatherer: g}
	}
	close(workChan)

	// Wait for all workers to finish and close result channel
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	var items []*ContextItem
	var errors []GatherError

	for res := range resultChan {
		if res.result.Error != nil {
			if res.result.Item != nil {
				errors = append(errors, GatherError{
					Type:  res.result.Item.Type,
					Name:  string(res.result.Item.Type),
					Error: res.result.Error.Error(),
				})
			}
			continue
		}
		if res.result.Item != nil {
			items = append(items, res.result.Item)
		}
	}

	// Build context with budgeting
	return b.buildWithBudget(items, errors)
}

// ContextBuilderOptions configures the context builder.
type ContextBuilderOptions struct {
	// Registry is the gatherer registry to use. If nil, DefaultRegistry is used.
	Registry *GathererRegistry
	// Budget is the token budget to apply. If nil, DefaultContextBudget is used.
	Budget *ContextBudget
	// FormatHeaders indicates whether to include section headers.
	FormatHeaders bool
	// Separator is the string used between sections.
	Separator string
	// MaxPerType limits tokens per context type.
	MaxPerType map[ContextType]int
	// Concurrent enables concurrent gathering.
	Concurrent bool
	// MaxConcurrency sets max concurrent gatherers (only if Concurrent is true).
	MaxConcurrency int
	// OnlyTypes limits gathering to specific types.
	OnlyTypes []ContextType
	// ExcludeTypes excludes specific types from gathering.
	ExcludeTypes []ContextType
}

// DefaultBuilderOptions returns sensible default options.
func DefaultBuilderOptions() *ContextBuilderOptions {
	return &ContextBuilderOptions{
		FormatHeaders:  true,
		Separator:      "\n\n---\n\n",
		MaxPerType:     make(map[ContextType]int),
		Concurrent:     true,
		MaxConcurrency: 4,
	}
}

// BuildContext is a convenience function that builds context with the given options.
// It includes a simple in-memory cache to reuse context within the same process
// when project state hasn't changed.
func BuildContext(ctx context.Context, projectPath string, opts *ContextBuilderOptions) (*BuilderResult, error) {
	if opts == nil {
		opts = DefaultBuilderOptions()
	}

	// Try cache if enabled (default true for performance)
	cacheKey := deriveCacheKey(projectPath, opts)
	if cacheKey != "" {
		if cached, ok := globalContextCache.Load(cacheKey); ok {
			return cached.(*BuilderResult), nil
		}
	}

	// Create or use provided registry
	registry := opts.Registry
	if registry == nil {
		registry = DefaultRegistry()
	}

	// Filter registry if type restrictions are specified
	if len(opts.OnlyTypes) > 0 || len(opts.ExcludeTypes) > 0 {
		registry = filterRegistry(registry, opts.OnlyTypes, opts.ExcludeTypes)
	}

	// Create builder
	var builder interface {
		Build(context.Context, string) (*BuilderResult, error)
	}

	if opts.Concurrent {
		cb := NewConcurrentContextBuilder(registry, opts.Budget)
		cb.WithMaxConcurrency(opts.MaxConcurrency)
		cb.WithFormatHeader(opts.FormatHeaders)
		cb.WithSeparator(opts.Separator)
		for t, max := range opts.MaxPerType {
			cb.WithMaxPerType(t, max)
		}
		builder = cb
	} else {
		b := NewContextBuilder(registry, opts.Budget)
		b.WithFormatHeader(opts.FormatHeaders)
		b.WithSeparator(opts.Separator)
		for t, max := range opts.MaxPerType {
			b.WithMaxPerType(t, max)
		}
		builder = b
	}

	result, err := builder.Build(ctx, projectPath)
	if err == nil && cacheKey != "" {
		globalContextCache.Store(cacheKey, result)
	}
	return result, err
}

var globalContextCache sync.Map

func deriveCacheKey(projectPath string, opts *ContextBuilderOptions) string {
	// Base key is the project path
	key := projectPath

	// Add git state if available
	if data, err := os.ReadFile(filepath.Join(projectPath, ".git", "HEAD")); err == nil {
		key += ":" + strings.TrimSpace(string(data))
	}

	// Add options hash to differentiate between different gathering settings
	if opts != nil {
		optsJSON, _ := json.Marshal(opts)
		hash := sha256.Sum256(optsJSON)
		key += ":" + hex.EncodeToString(hash[:])
	}

	return key
}

// filterRegistry creates a new registry with filtered gatherers.
func filterRegistry(original *GathererRegistry, onlyTypes, excludeTypes []ContextType) *GathererRegistry {
	filtered := NewGathererRegistry()

	// Create lookup maps
	onlyMap := make(map[ContextType]bool)
	for _, t := range onlyTypes {
		onlyMap[t] = true
	}

	excludeMap := make(map[ContextType]bool)
	for _, t := range excludeTypes {
		excludeMap[t] = true
	}

	for _, g := range original.GetAll() {
		// Skip if excluded
		if excludeMap[g.Type()] {
			continue
		}

		// Skip if only specific types are allowed and this isn't one
		if len(onlyMap) > 0 && !onlyMap[g.Type()] {
			continue
		}

		filtered.Register(g)
	}

	return filtered
}

// QuickContext is a convenience function for quickly building context with defaults.
func QuickContext(ctx context.Context, projectPath string) (string, error) {
	result, err := BuildContext(ctx, projectPath, nil)
	if err != nil {
		return "", err
	}
	return result.Context, nil
}

// QuickContextWithBudget builds context with a specified token budget.
func QuickContextWithBudget(ctx context.Context, projectPath string, totalTokens int) (string, error) {
	budget, err := NewContextBudget(totalTokens)
	if err != nil {
		return "", err
	}

	opts := DefaultBuilderOptions()
	opts.Budget = budget

	result, err := BuildContext(ctx, projectPath, opts)
	if err != nil {
		return "", err
	}
	return result.Context, nil
}
