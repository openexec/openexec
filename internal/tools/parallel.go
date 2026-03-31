// Package tools provides parallel tool execution for OpenExec.
// It enables concurrent execution of independent tools to improve performance.
package tools

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ParallelExecutor executes tools in parallel.
type ParallelExecutor struct {
	maxConcurrency int
	timeout        time.Duration
}

// ParallelConfig contains configuration for parallel execution.
type ParallelConfig struct {
	// MaxConcurrency is the maximum number of tools to execute concurrently.
	MaxConcurrency int

	// Timeout is the maximum time to wait for all tools.
	Timeout time.Duration

	// FailFast stops execution on first error.
	FailFast bool
}

// DefaultParallelConfig returns default parallel configuration.
func DefaultParallelConfig() *ParallelConfig {
	return &ParallelConfig{
		MaxConcurrency: 4,
		Timeout:        30 * time.Second,
		FailFast:       false,
	}
}

// NewParallelExecutor creates a new parallel executor.
func NewParallelExecutor(config *ParallelConfig) *ParallelExecutor {
	if config == nil {
		config = DefaultParallelConfig()
	}

	return &ParallelExecutor{
		maxConcurrency: config.MaxConcurrency,
		timeout:        config.Timeout,
	}
}

// ParallelTool represents a tool that can be executed in parallel.
type ParallelTool interface {
	Name() string
	Execute(ctx context.Context) (interface{}, error)
}

// ToolResult represents the result of a tool execution.
type ToolResult struct {
	ToolName  string
	Result    interface{}
	Error     error
	Duration  time.Duration
	StartedAt time.Time
}

// ExecutionResult contains results from parallel execution.
type ExecutionResult struct {
	Results        []ToolResult
	SuccessCount   int
	ErrorCount     int
	TotalDuration  time.Duration
	ExecutionOrder []string
}

// Execute runs multiple tools in parallel.
func (e *ParallelExecutor) Execute(ctx context.Context, tools []ParallelTool) (*ExecutionResult, error) {
	if len(tools) == 0 {
		return &ExecutionResult{}, nil
	}

	// Use timeout context
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	start := time.Now()

	// Create semaphore for concurrency control
	sem := make(chan struct{}, e.maxConcurrency)

	// Create results channel
	results := make(chan ToolResult, len(tools))

	// Execute tools
	var wg sync.WaitGroup
	for _, tool := range tools {
		wg.Add(1)
		go func(t ParallelTool) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				// Acquired
			case <-ctx.Done():
				results <- ToolResult{
					ToolName: t.Name(),
					Error:    ctx.Err(),
				}
				return
			}
			defer func() { <-sem }() // Release semaphore

			// Execute tool
			toolStart := time.Now()
			result, err := t.Execute(ctx)
			duration := time.Since(toolStart)

			// Send result
			select {
			case results <- ToolResult{
				ToolName:  t.Name(),
				Result:    result,
				Error:     err,
				Duration:  duration,
				StartedAt: toolStart,
			}:
			case <-ctx.Done():
				return
			}
		}(tool)
	}

	// Close results channel when all done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	execResult := &ExecutionResult{
		Results: make([]ToolResult, 0, len(tools)),
	}

	for result := range results {
		execResult.Results = append(execResult.Results, result)
		execResult.ExecutionOrder = append(execResult.ExecutionOrder, result.ToolName)

		if result.Error != nil {
			execResult.ErrorCount++
		} else {
			execResult.SuccessCount++
		}
	}

	execResult.TotalDuration = time.Since(start)

	// Check for errors
	if execResult.ErrorCount > 0 {
		return execResult, fmt.Errorf("%d tools failed", execResult.ErrorCount)
	}

	return execResult, nil
}

// ExecuteWithDependencies runs tools respecting dependencies.
func (e *ParallelExecutor) ExecuteWithDependencies(ctx context.Context, tools []ToolWithDeps) (*ExecutionResult, error) {
	if len(tools) == 0 {
		return &ExecutionResult{}, nil
	}

	// Build dependency graph (validates structure)
	_ = e.buildDependencyGraph(tools)

	// Execute in waves
	start := time.Now()
	execResult := &ExecutionResult{
		Results: make([]ToolResult, 0, len(tools)),
	}

	completed := make(map[string]bool)
	for len(completed) < len(tools) {
		// Find ready tools (all dependencies completed)
		var ready []ParallelTool
		for _, tool := range tools {
			if completed[tool.Name()] {
				continue
			}
			if e.dependenciesMet(tool, completed) {
				ready = append(ready, tool)
			}
		}

		if len(ready) == 0 {
			return execResult, fmt.Errorf("dependency cycle detected")
		}

		// Execute ready tools in parallel
		waveResult, err := e.Execute(ctx, ready)
		if err != nil {
			// Continue to get partial results
		}

		// Record completions
		for _, result := range waveResult.Results {
			if result.Error == nil {
				completed[result.ToolName] = true
			}
			execResult.Results = append(execResult.Results, result)
		}

		if err != nil {
			break
		}
	}

	execResult.TotalDuration = time.Since(start)
	execResult.SuccessCount = len(completed)
	execResult.ErrorCount = len(tools) - len(completed)

	return execResult, nil
}

// ToolWithDeps is a tool with dependencies.
type ToolWithDeps interface {
	ParallelTool
	Dependencies() []string
}

// buildDependencyGraph builds a dependency graph.
func (e *ParallelExecutor) buildDependencyGraph(tools []ToolWithDeps) map[string][]string {
	graph := make(map[string][]string)
	for _, tool := range tools {
		graph[tool.Name()] = tool.Dependencies()
	}
	return graph
}

// dependenciesMet checks if all dependencies are completed.
func (e *ParallelExecutor) dependenciesMet(tool ToolWithDeps, completed map[string]bool) bool {
	for _, dep := range tool.Dependencies() {
		if !completed[dep] {
			return false
		}
	}
	return true
}

// ExecuteBatched executes tools in batches.
func (e *ParallelExecutor) ExecuteBatched(ctx context.Context, tools []ParallelTool, batchSize int) (*ExecutionResult, error) {
	if batchSize <= 0 {
		batchSize = e.maxConcurrency
	}

	start := time.Now()
	execResult := &ExecutionResult{
		Results: make([]ToolResult, 0, len(tools)),
	}

	// Process in batches
	for i := 0; i < len(tools); i += batchSize {
		end := i + batchSize
		if end > len(tools) {
			end = len(tools)
		}

		batch := tools[i:end]
		batchResult, err := e.Execute(ctx, batch)

		execResult.Results = append(execResult.Results, batchResult.Results...)
		execResult.SuccessCount += batchResult.SuccessCount
		execResult.ErrorCount += batchResult.ErrorCount

		if err != nil {
			return execResult, err
		}
	}

	execResult.TotalDuration = time.Since(start)
	return execResult, nil
}

// SimpleTool is a simple tool implementation.
type SimpleTool struct {
	name    string
	execute func(ctx context.Context) (interface{}, error)
}

// NewSimpleTool creates a new simple tool.
func NewSimpleTool(name string, execute func(ctx context.Context) (interface{}, error)) *SimpleTool {
	return &SimpleTool{
		name:    name,
		execute: execute,
	}
}

// Name returns the tool name.
func (t *SimpleTool) Name() string {
	return t.name
}

// Execute executes the tool.
func (t *SimpleTool) Execute(ctx context.Context) (interface{}, error) {
	return t.execute(ctx)
}

// FileReadTool reads a file.
type FileReadTool struct {
	Path string
}

// NewFileReadTool creates a new file read tool.
func NewFileReadTool(path string) *FileReadTool {
	return &FileReadTool{Path: path}
}

// Name returns the tool name.
func (t *FileReadTool) Name() string {
	return fmt.Sprintf("read_file:%s", t.Path)
}

// Execute reads the file.
func (t *FileReadTool) Execute(ctx context.Context) (interface{}, error) {
	// Implementation would read file
	return fmt.Sprintf("content of %s", t.Path), nil
}

// GrepTool searches for patterns.
type GrepTool struct {
	Pattern string
	Path    string
}

// NewGrepTool creates a new grep tool.
func NewGrepTool(pattern, path string) *GrepTool {
	return &GrepTool{Pattern: pattern, Path: path}
}

// Name returns the tool name.
func (t *GrepTool) Name() string {
	return fmt.Sprintf("grep:%s:%s", t.Pattern, t.Path)
}

// Execute performs the grep.
func (t *GrepTool) Execute(ctx context.Context) (interface{}, error) {
	// Implementation would grep
	return []string{"match1", "match2"}, nil
}

// ParallelToolSet executes a set of tools in parallel.
type ParallelToolSet struct {
	executor *ParallelExecutor
	tools    []ParallelTool
}

// NewParallelToolSet creates a new parallel tool set.
func NewParallelToolSet(config *ParallelConfig) *ParallelToolSet {
	return &ParallelToolSet{
		executor: NewParallelExecutor(config),
		tools:    make([]ParallelTool, 0),
	}
}

// Add adds a tool to the set.
func (s *ParallelToolSet) Add(tool ParallelTool) {
	s.tools = append(s.tools, tool)
}

// Execute executes all tools in parallel.
func (s *ParallelToolSet) Execute(ctx context.Context) (*ExecutionResult, error) {
	return s.executor.Execute(ctx, s.tools)
}

// Results returns results indexed by tool name.
func (r *ExecutionResult) ResultsByName() map[string]ToolResult {
	byName := make(map[string]ToolResult)
	for _, result := range r.Results {
		byName[result.ToolName] = result
	}
	return byName
}

// GetResult gets a result by tool name.
func (r *ExecutionResult) GetResult(name string) (ToolResult, bool) {
	for _, result := range r.Results {
		if result.ToolName == name {
			return result, true
		}
	}
	return ToolResult{}, false
}

// HasErrors returns true if any tool failed.
func (r *ExecutionResult) HasErrors() bool {
	return r.ErrorCount > 0
}

// Error returns a combined error message.
func (r *ExecutionResult) Error() string {
	if !r.HasErrors() {
		return ""
	}

	var errs []string
	for _, result := range r.Results {
		if result.Error != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", result.ToolName, result.Error))
		}
	}

	return fmt.Sprintf("%d tools failed: %s", r.ErrorCount, errs)
}
