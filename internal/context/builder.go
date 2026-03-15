// Package context provides automatic context gathering and injection for AI agent sessions.
package context

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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
	// Timestamp is when the result was generated (V1 TTL)
	Timestamp time.Time `json:"timestamp"`
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
			atomic.AddUint64(&cacheHits, 1)
			return cached.(*BuilderResult), nil
		}
		
		// Try disk cache
		if result, err := loadContextPack(projectPath, cacheKey); err == nil {
			atomic.AddUint64(&cacheHits, 1)
			globalContextCache.Store(cacheKey, result)
			return result, nil
		}
		atomic.AddUint64(&cacheMisses, 1)
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
	if err == nil {
		result.Timestamp = time.Now()
		if cacheKey != "" {
			globalContextCache.Store(cacheKey, result)
			
			// V5: Persist to artifact store for cross-process reuse
			persistContextPack(projectPath, cacheKey, result)
		}
	}
	return result, err
}

func persistContextPack(projectPath, key string, result *BuilderResult) {
	dir := filepath.Join(projectPath, ".openexec", "artifacts", "context")
	_ = os.MkdirAll(dir, 0750)
	
	// Create content-addressed hash of the key
	h := sha256.Sum256([]byte(key))
	hashStr := hex.EncodeToString(h[:])
	
	path := filepath.Join(dir, "context_" + hashStr + ".json")
	data, _ := json.Marshal(result)
	_ = os.WriteFile(path, data, 0644)
}

func loadContextPack(projectPath, key string) (*BuilderResult, error) {
	h := sha256.Sum256([]byte(key))
	hashStr := hex.EncodeToString(h[:])
	path := filepath.Join(projectPath, ".openexec", "artifacts", "context", "context_" + hashStr + ".json")
	
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	
	var result BuilderResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	// TTL Check: 24 hours
	if !result.Timestamp.IsZero() && time.Since(result.Timestamp) > 24*time.Hour {
		return nil, fmt.Errorf("context pack expired")
	}

	return &result, nil
}

var (
	globalContextCache sync.Map
	cacheHits          uint64
	cacheMisses        uint64
)

// GetCacheMetrics returns the hit and miss counts for the context cache.
func GetCacheMetrics() (hits, misses uint64) {
	return atomic.LoadUint64(&cacheHits), atomic.LoadUint64(&cacheMisses)
}

func deriveCacheKey(projectPath string, opts *ContextBuilderOptions) string {
	// Base key is the project path
	key := projectPath

	// Add git state if available.
	// For non-git repos: silently fall back to path-only key with TTL invalidation.
	// This is expected behavior - no error logging needed.
	if data, err := os.ReadFile(filepath.Join(projectPath, ".git", "HEAD")); err == nil {
		key += ":" + strings.TrimSpace(string(data))

		// Add dirty status to key to invalidate if files changed (V1 Hardening)
		// Use short timeout to avoid hangs on slow/unresponsive git
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
		cmd.Dir = projectPath
		out, err := cmd.Output()
		cancel()
		if err == nil && len(out) > 0 {
			// Hash the porcelain output to keep the key stable but unique per dirty state
			h := sha256.Sum256(out)
			key += ":dirty:" + hex.EncodeToString(h[:8])
		}
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

// =============================================================================
// Two-Stage Context Assembly (Converged Architecture)
// =============================================================================
//
// Stage 1: Deterministic gathering based on routing plan zones
// Stage 2: Ranking and filtering based on knowledge source priorities

// ContextPack is the assembled context for a single routing request.
// It contains prioritized, sanitized context items ready for model consumption.
type ContextPack struct {
	// Items contains the ranked and filtered context items.
	Items []*ContextItem `json:"items"`

	// TotalTokens is the total estimated token count of all items.
	TotalTokens int `json:"total_tokens"`

	// ContextHash is a hash of the context for cache keying.
	ContextHash string `json:"context_hash"`

	// Sensitivity indicates the detected sensitivity level.
	Sensitivity string `json:"sensitivity"`

	// RedactedKeys lists any keys that were redacted for security.
	RedactedKeys []string `json:"redacted_keys,omitempty"`

	// AssembledAt is when this pack was created.
	AssembledAt time.Time `json:"assembled_at"`

	// Zones lists the repo zones that were used for filtering.
	Zones []string `json:"zones,omitempty"`

	// KnowledgeSources lists the sources used for ranking.
	KnowledgeSources []string `json:"knowledge_sources,omitempty"`
}

// DeterministicContext holds the raw gathered context before ranking.
type DeterministicContext struct {
	// ChangedFiles lists files modified in the workspace.
	ChangedFiles []string `json:"changed_files,omitempty"`

	// CurrentDiff is the git diff of uncommitted changes.
	CurrentDiff string `json:"current_diff,omitempty"`

	// RepoRules contains .claude/rules, CLAUDE.md content.
	RepoRules []string `json:"repo_rules,omitempty"`

	// TestFailures lists recent test failure messages.
	TestFailures []string `json:"test_failures,omitempty"`

	// GatheredItems are the raw context items from gatherers.
	GatheredItems []*ContextItem `json:"gathered_items,omitempty"`
}

// RoutingAwareBuilder builds context using a routing plan.
type RoutingAwareBuilder struct {
	builder     *ContextBuilder
	projectPath string
	maxTokens   int
	sensitiveKws []string
}

// NewRoutingAwareBuilder creates a builder that uses routing plans.
func NewRoutingAwareBuilder(projectPath string, budget *ContextBudget) *RoutingAwareBuilder {
	registry := DefaultRegistry()
	return &RoutingAwareBuilder{
		builder:     NewContextBuilder(registry, budget),
		projectPath: projectPath,
		maxTokens:   budget.AvailableForContext(),
		sensitiveKws: []string{
			"password", "secret", "api_key", "apikey", "token",
			"private_key", "credential", "ssh_key", "auth",
		},
	}
}

// BuildWithPlan performs two-stage context assembly from routing parameters.
// repoZones filters context to specific directories.
// knowledgeSources ranks items by source relevance.
// sensitivity determines redaction level ("low", "medium", "high").
func (b *RoutingAwareBuilder) BuildWithPlan(
	ctx context.Context,
	repoZones []string,
	knowledgeSources []string,
	sensitivity string,
) (*ContextPack, error) {
	// Stage 1: Deterministic gathering
	det, err := b.gatherDeterministic(ctx, repoZones)
	if err != nil {
		return nil, err
	}

	// Stage 2: Ranking by knowledge source priorities
	ranked := b.rankByKnowledgeSources(det.GatheredItems, knowledgeSources)

	// Stage 3: Apply token budget and redact sensitive content
	filtered, redactedKeys := b.applyBudgetAndRedact(ranked, sensitivity)

	// Compute hash for caching
	hash := b.computeContextHash(filtered)

	return &ContextPack{
		Items:            filtered,
		TotalTokens:      sumTokens(filtered),
		ContextHash:      hash,
		Sensitivity:      sensitivity,
		RedactedKeys:     redactedKeys,
		AssembledAt:      time.Now(),
		Zones:            repoZones,
		KnowledgeSources: knowledgeSources,
	}, nil
}

// gatherDeterministic performs Stage 1: gather context without LLM involvement.
func (b *RoutingAwareBuilder) gatherDeterministic(ctx context.Context, repoZones []string) (*DeterministicContext, error) {
	det := &DeterministicContext{
		GatheredItems: make([]*ContextItem, 0),
	}

	// Build context using the standard builder
	result, err := b.builder.Build(ctx, b.projectPath)
	if err != nil {
		return nil, err
	}

	// Collect all items
	det.GatheredItems = append(det.GatheredItems, result.IncludedItems...)

	// Extract specific context types
	for _, item := range det.GatheredItems {
		switch item.Type {
		case ContextTypeGitDiff:
			det.CurrentDiff = item.Content
		case ContextTypeGitStatus:
			det.ChangedFiles = parseGitStatusFiles(item.Content)
		case ContextTypeProjectInstructions:
			det.RepoRules = append(det.RepoRules, item.Content)
		}
	}

	// Filter by repo zones if specified
	if len(repoZones) > 0 {
		det.GatheredItems = filterItemsByZones(det.GatheredItems, repoZones)
	}

	return det, nil
}

// filterItemsByZones keeps only items matching the specified zones.
func filterItemsByZones(items []*ContextItem, zones []string) []*ContextItem {
	filtered := make([]*ContextItem, 0)

	// Always include essential types regardless of zone
	essentialTypes := map[ContextType]bool{
		ContextTypeProjectInstructions: true,
		ContextTypeEnvironment:         true,
		ContextTypePackageInfo:         true,
	}

	for _, item := range items {
		// Always include essential types
		if essentialTypes[item.Type] {
			filtered = append(filtered, item)
			continue
		}

		// Check if source matches any zone
		for _, zone := range zones {
			if sourceMatchesZone(item.Source, zone) {
				filtered = append(filtered, item)
				break
			}
		}
	}

	return filtered
}

// sourceMatchesZone checks if a source path matches a zone pattern.
func sourceMatchesZone(source, zone string) bool {
	// Direct prefix match
	if strings.HasPrefix(source, zone) {
		return true
	}

	// Normalize zone to directory form
	if !strings.HasSuffix(zone, "/") {
		zone = zone + "/"
	}

	return strings.Contains(source, zone)
}

// rankByKnowledgeSources ranks items by knowledge source priorities.
func (b *RoutingAwareBuilder) rankByKnowledgeSources(items []*ContextItem, knowledgeSources []string) []*ContextItem {
	// Build priority map: earlier in list = higher priority
	priority := make(map[string]int)
	for i, source := range knowledgeSources {
		priority[source] = len(knowledgeSources) - i
	}

	// Score each item
	type scoredItem struct {
		item  *ContextItem
		score int
	}

	scored := make([]scoredItem, 0, len(items))
	for _, item := range items {
		score := b.scoreItemBySource(item, priority)
		scored = append(scored, scoredItem{item: item, score: score})
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Extract items
	result := make([]*ContextItem, 0, len(scored))
	for _, si := range scored {
		result = append(result, si.item)
	}

	return result
}

// scoreItemBySource calculates relevance score based on knowledge sources.
func (b *RoutingAwareBuilder) scoreItemBySource(item *ContextItem, sourcePriority map[string]int) int {
	score := 0

	// Base score from item priority
	score += int(item.Priority) * 10

	// Map context type to knowledge source
	sourceType := contextTypeToKnowledgeSource(item.Type)
	if p, ok := sourcePriority[sourceType]; ok {
		score += p * 100
	}

	// Bonus for recent items
	if !item.GatheredAt.IsZero() {
		age := time.Since(item.GatheredAt)
		if age < time.Minute {
			score += 50
		} else if age < time.Hour {
			score += 20
		}
	}

	return score
}

// contextTypeToKnowledgeSource maps context types to knowledge source names.
func contextTypeToKnowledgeSource(ct ContextType) string {
	switch ct {
	case ContextTypeGitLog:
		return "git_history"
	case ContextTypeGitDiff, ContextTypeGitStatus:
		return "git_history"
	case ContextTypeProjectInstructions:
		return "local_docs"
	case ContextTypeDirectoryStructure, ContextTypeRecentFiles:
		return "code_symbols"
	case ContextTypePackageInfo:
		return "dependencies"
	default:
		return "code_symbols"
	}
}

// applyBudgetAndRedact applies token budget and redacts sensitive content.
func (b *RoutingAwareBuilder) applyBudgetAndRedact(items []*ContextItem, sensitivity string) ([]*ContextItem, []string) {
	result := make([]*ContextItem, 0)
	redactedKeys := make([]string, 0)
	tokensUsed := 0

	for _, item := range items {
		// Check budget
		if tokensUsed+item.TokenCount > b.maxTokens {
			// Try to truncate
			remaining := b.maxTokens - tokensUsed
			if remaining > 100 {
				newItem := *item
				newItem.Content = TruncateToTokenLimit(item.Content, remaining)
				newItem.TokenCount = EstimateTokens(newItem.Content)
				result = append(result, &newItem)
				tokensUsed += newItem.TokenCount
			}
			break
		}

		// Redact if high sensitivity
		if sensitivity == "high" {
			newItem := *item
			newItem.Content, redactedKeys = b.redactSensitive(item.Content, redactedKeys)
			newItem.TokenCount = EstimateTokens(newItem.Content)
			result = append(result, &newItem)
			tokensUsed += newItem.TokenCount
		} else {
			result = append(result, item)
			tokensUsed += item.TokenCount
		}
	}

	return result, redactedKeys
}

// redactSensitive detects and logs sensitive content.
func (b *RoutingAwareBuilder) redactSensitive(content string, existingKeys []string) (string, []string) {
	lower := strings.ToLower(content)
	keys := append([]string{}, existingKeys...)

	for _, kw := range b.sensitiveKws {
		if strings.Contains(lower, kw) {
			keys = append(keys, kw)
		}
	}

	// For now, just detect - actual redaction would be more sophisticated
	return content, keys
}

// computeContextHash generates a hash for cache keying.
func (b *RoutingAwareBuilder) computeContextHash(items []*ContextItem) string {
	h := sha256.New()
	for _, item := range items {
		h.Write([]byte(item.ID))
		h.Write([]byte(item.Content))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// parseGitStatusFiles extracts file paths from git status output.
func parseGitStatusFiles(gitStatus string) []string {
	files := make([]string, 0)
	lines := strings.Split(gitStatus, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 3 {
			continue
		}

		// Git status format: XY filename
		if len(line) > 2 && line[2] == ' ' {
			file := strings.TrimSpace(line[3:])
			if file != "" {
				// Handle renamed files
				if idx := strings.Index(file, " -> "); idx != -1 {
					file = file[idx+4:]
				}
				files = append(files, file)
			}
		}
	}

	return files
}

// sumTokens calculates total tokens across items.
func sumTokens(items []*ContextItem) int {
	total := 0
	for _, item := range items {
		total += item.TokenCount
	}
	return total
}

// BuildContextWithRouting is a convenience function for routing-aware context building.
func BuildContextWithRouting(
	ctx context.Context,
	projectPath string,
	totalTokens int,
	repoZones []string,
	knowledgeSources []string,
	sensitivity string,
) (*ContextPack, error) {
	budget, err := NewContextBudget(totalTokens)
	if err != nil {
		return nil, err
	}

	builder := NewRoutingAwareBuilder(projectPath, budget)
	return builder.BuildWithPlan(ctx, repoZones, knowledgeSources, sensitivity)
}
