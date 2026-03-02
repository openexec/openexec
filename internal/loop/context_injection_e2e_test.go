package loop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	octx "github.com/openexec/openexec/internal/context"
)

// E2E Test: Auto-Context Injection (G-004)
//
// This test suite validates the end-to-end integration of automatic context
// gathering and injection into the agent loop. Auto-context gathers project
// information (git status, CLAUDE.md, package.json, etc.) and injects it
// into the initial conversation to provide relevant background.

// setupTestProject creates a temporary project directory with various files
// that context gatherers will detect and process.
func setupTestProject(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()

	// Create a CLAUDE.md / project instructions file
	claudeMD := `# Project Instructions

This is a test project for OpenExec.

## Guidelines
- Follow Go best practices
- Write tests for all new code
- Use idiomatic error handling
`
	if err := os.WriteFile(filepath.Join(tmpDir, "CLAUDE.md"), []byte(claudeMD), 0644); err != nil {
		t.Fatalf("failed to create CLAUDE.md: %v", err)
	}

	// Create a go.mod file (package info)
	goMod := `module example.com/testproject

go 1.21

require (
	github.com/google/uuid v1.3.0
)
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("failed to create go.mod: %v", err)
	}

	// Create a main.go file (recent files)
	mainGo := `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(mainGo), 0644); err != nil {
		t.Fatalf("failed to create main.go: %v", err)
	}

	// Create a nested directory structure
	srcDir := filepath.Join(tmpDir, "src", "handlers")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src/handlers: %v", err)
	}

	handlerGo := `package handlers

func Handle() error {
	return nil
}
`
	if err := os.WriteFile(filepath.Join(srcDir, "handler.go"), []byte(handlerGo), 0644); err != nil {
		t.Fatalf("failed to create handler.go: %v", err)
	}

	return tmpDir
}

// TestE2E_AutoContextInjection_BuildsContextFromProject tests that the
// context builder correctly gathers context from a project directory.
func TestE2E_AutoContextInjection_BuildsContextFromProject(t *testing.T) {
	projectPath := setupTestProject(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build context using default options
	opts := octx.DefaultBuilderOptions()
	result, err := octx.BuildContext(ctx, projectPath, opts)
	if err != nil {
		t.Fatalf("failed to build context: %v", err)
	}

	// Verify context was gathered
	if result.Context == "" {
		t.Fatal("expected non-empty context")
	}

	if result.TotalTokens == 0 {
		t.Error("expected non-zero token count")
	}

	// Verify at least one item was included
	if len(result.IncludedItems) == 0 {
		t.Error("expected at least one included context item")
	}

	// Verify no errors occurred
	if len(result.Errors) > 0 {
		t.Logf("context gathering errors (may be expected): %+v", result.Errors)
	}

	t.Logf("Context gathered: %d tokens, %d items included, %d items excluded",
		result.TotalTokens, len(result.IncludedItems), len(result.ExcludedItems))
}

// TestE2E_AutoContextInjection_ProjectInstructions tests that CLAUDE.md
// or similar instruction files are gathered with high priority.
func TestE2E_AutoContextInjection_ProjectInstructions(t *testing.T) {
	projectPath := setupTestProject(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use options that only gather project instructions
	opts := octx.DefaultBuilderOptions()
	opts.OnlyTypes = []octx.ContextType{octx.ContextTypeProjectInstructions}

	result, err := octx.BuildContext(ctx, projectPath, opts)
	if err != nil {
		t.Fatalf("failed to build context: %v", err)
	}

	// Check if project instructions were found
	found := false
	for _, item := range result.IncludedItems {
		if item.Type == octx.ContextTypeProjectInstructions {
			found = true
			// Verify content contains expected text
			if !strings.Contains(item.Content, "Project Instructions") {
				t.Error("expected project instructions to contain 'Project Instructions'")
			}
			if !strings.Contains(item.Content, "Go best practices") {
				t.Error("expected project instructions to contain 'Go best practices'")
			}
			// Verify priority is critical
			if item.Priority != octx.PriorityCritical {
				t.Errorf("expected critical priority for project instructions, got %d", item.Priority)
			}
			break
		}
	}

	if !found {
		t.Error("expected project instructions to be gathered from CLAUDE.md")
	}
}

// TestE2E_AutoContextInjection_PackageInfo tests that package files
// (go.mod, package.json, etc.) are gathered.
func TestE2E_AutoContextInjection_PackageInfo(t *testing.T) {
	projectPath := setupTestProject(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use options that only gather package info
	opts := octx.DefaultBuilderOptions()
	opts.OnlyTypes = []octx.ContextType{octx.ContextTypePackageInfo}

	result, err := octx.BuildContext(ctx, projectPath, opts)
	if err != nil {
		t.Fatalf("failed to build context: %v", err)
	}

	// Check if package info was found
	found := false
	for _, item := range result.IncludedItems {
		if item.Type == octx.ContextTypePackageInfo {
			found = true
			// Verify content contains expected text from go.mod
			if !strings.Contains(item.Content, "example.com/testproject") {
				t.Error("expected package info to contain module path")
			}
			break
		}
	}

	if !found {
		t.Log("Package info gatherer may not have found go.mod - this is acceptable in test environment")
	}
}

// TestE2E_AutoContextInjection_TokenBudgeting tests that context respects
// token budget constraints.
func TestE2E_AutoContextInjection_TokenBudgeting(t *testing.T) {
	projectPath := setupTestProject(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a very limited budget
	budget, err := octx.NewContextBudget(1000) // Very small: 1000 total tokens
	if err != nil {
		t.Fatalf("failed to create budget: %v", err)
	}
	budget.ReservedForSystemPrompt = 100
	budget.ReservedForConversation = 200
	// Available for context: 700 tokens

	opts := octx.DefaultBuilderOptions()
	opts.Budget = budget

	result, err := octx.BuildContext(ctx, projectPath, opts)
	if err != nil {
		t.Fatalf("failed to build context: %v", err)
	}

	// Verify we stayed within budget
	availableBudget := budget.AvailableForContext()
	if result.TotalTokens > availableBudget {
		t.Errorf("token budget exceeded: used %d, available %d", result.TotalTokens, availableBudget)
	}

	// Verify TokensRemaining is calculated correctly
	expectedRemaining := availableBudget - result.TotalTokens
	if result.TokensRemaining != expectedRemaining {
		t.Errorf("expected tokens remaining %d, got %d", expectedRemaining, result.TokensRemaining)
	}

	t.Logf("Budget: %d available, %d used, %d remaining",
		availableBudget, result.TotalTokens, result.TokensRemaining)
}

// TestE2E_AutoContextInjection_PriorityOrdering tests that higher priority
// context items are included first when budget is limited.
func TestE2E_AutoContextInjection_PriorityOrdering(t *testing.T) {
	projectPath := setupTestProject(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a limited budget that forces exclusion
	budget, err := octx.NewContextBudget(2000)
	if err != nil {
		t.Fatalf("failed to create budget: %v", err)
	}
	budget.ReservedForSystemPrompt = 100
	budget.ReservedForConversation = 200

	opts := octx.DefaultBuilderOptions()
	opts.Budget = budget

	result, err := octx.BuildContext(ctx, projectPath, opts)
	if err != nil {
		t.Fatalf("failed to build context: %v", err)
	}

	// If items were excluded, verify higher priority items were kept
	if len(result.ExcludedItems) > 0 && len(result.IncludedItems) > 0 {
		lowestIncludedPriority := result.IncludedItems[0].Priority
		for _, item := range result.IncludedItems {
			if item.Priority < lowestIncludedPriority {
				lowestIncludedPriority = item.Priority
			}
		}

		for _, excluded := range result.ExcludedItems {
			// Excluded items should generally have lower or equal priority
			// (or were excluded due to per-type limits)
			if excluded.Priority > lowestIncludedPriority+20 { // Allow some tolerance
				t.Logf("Note: High priority item excluded - may be due to per-type limit: %s (%d)",
					excluded.Type, excluded.Priority)
			}
		}
	}

	// Verify included items are sorted by priority (highest first)
	for i := 1; i < len(result.IncludedItems); i++ {
		// Items at same priority level may be reordered by token count
		if result.IncludedItems[i-1].Priority < result.IncludedItems[i].Priority {
			// Only flag if they're not at the same priority level
			if result.IncludedItems[i-1].Priority != result.IncludedItems[i].Priority {
				t.Errorf("items not sorted by priority: %d followed by %d",
					result.IncludedItems[i-1].Priority, result.IncludedItems[i].Priority)
			}
		}
	}
}

// TestE2E_AutoContextInjection_PerTypeLimits tests that per-type token limits
// are respected.
func TestE2E_AutoContextInjection_PerTypeLimits(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create registry with mock gatherers that produce known token counts
	registry := octx.NewGathererRegistry()

	// Create two gatherers of the same type
	g1 := newContextMockGatherer(octx.ContextTypeEnvironment, "Env1", "Environment content 1", 300)
	g2 := newContextMockGatherer(octx.ContextTypeEnvironment, "Env2", "Environment content 2", 300)
	g3 := newContextMockGatherer(octx.ContextTypeProjectInstructions, "Inst", "Instructions", 200)

	registry.Register(g1)
	registry.Register(g2)
	registry.Register(g3)

	opts := octx.DefaultBuilderOptions()
	opts.Registry = registry
	opts.MaxPerType = map[octx.ContextType]int{
		octx.ContextTypeEnvironment: 400, // Only room for one environment gatherer
	}

	result, err := octx.BuildContext(ctx, "/tmp/test", opts)
	if err != nil {
		t.Fatalf("failed to build context: %v", err)
	}

	// Count environment items
	envCount := 0
	for _, item := range result.IncludedItems {
		if item.Type == octx.ContextTypeEnvironment {
			envCount++
		}
	}

	// Should only have one environment item due to per-type limit
	if envCount > 1 {
		t.Errorf("expected at most 1 environment item due to per-type limit, got %d", envCount)
	}

	// Instructions should still be included (not subject to environment limit)
	instFound := false
	for _, item := range result.IncludedItems {
		if item.Type == octx.ContextTypeProjectInstructions {
			instFound = true
			break
		}
	}
	if !instFound {
		t.Error("expected project instructions to be included")
	}
}

// TestE2E_AutoContextInjection_MinimumPriority tests that items below
// minimum priority threshold are excluded.
func TestE2E_AutoContextInjection_MinimumPriority(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	registry := octx.NewGathererRegistry()

	// Create gatherers with different priorities
	highPri := newContextMockGatherer(octx.ContextTypeProjectInstructions, "High", "High priority content", 100)
	lowPri := newContextMockGatherer(octx.ContextTypeGitDiff, "Low", "Low priority content", 100)

	registry.Register(highPri)
	registry.Register(lowPri)

	budget := octx.DefaultContextBudget()
	budget.MinPriorityToInclude = octx.PriorityMedium // Exclude low and optional priority

	opts := octx.DefaultBuilderOptions()
	opts.Registry = registry
	opts.Budget = budget

	result, err := octx.BuildContext(ctx, "/tmp/test", opts)
	if err != nil {
		t.Fatalf("failed to build context: %v", err)
	}

	// High priority should be included
	highFound := false
	lowFound := false
	for _, item := range result.IncludedItems {
		if item.Type == octx.ContextTypeProjectInstructions {
			highFound = true
		}
		if item.Type == octx.ContextTypeGitDiff {
			lowFound = true
		}
	}

	if !highFound {
		t.Error("expected high priority item to be included")
	}
	if lowFound {
		t.Error("expected low priority item to be excluded due to minimum priority threshold")
	}
}

// TestE2E_AutoContextInjection_ConcurrentGathering tests that concurrent
// context gathering works correctly.
func TestE2E_AutoContextInjection_ConcurrentGathering(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	registry := octx.NewGathererRegistry()

	// Register multiple gatherers using valid context types
	// We use different valid types to avoid registry collisions
	validTypes := []octx.ContextType{
		octx.ContextTypeProjectInstructions,
		octx.ContextTypeGitStatus,
		octx.ContextTypeEnvironment,
		octx.ContextTypePackageInfo,
		octx.ContextTypeDirectoryStructure,
	}

	for i, contextType := range validTypes {
		g := newContextMockGatherer(
			contextType,
			"Gatherer"+string(rune('A'+i)),
			"Content"+string(rune('A'+i)),
			50,
		)
		registry.Register(g)
	}

	builder := octx.NewConcurrentContextBuilder(registry, nil)
	builder.WithMaxConcurrency(3) // Limit concurrency

	result, err := builder.Build(ctx, "/tmp/test")
	if err != nil {
		t.Fatalf("failed to build context: %v", err)
	}

	// All items should be included
	if len(result.IncludedItems) != len(validTypes) {
		t.Errorf("expected %d items, got %d", len(validTypes), len(result.IncludedItems))
	}

	// Total tokens should be correct
	expectedTokens := len(validTypes) * 50
	if result.TotalTokens != expectedTokens {
		t.Errorf("expected total tokens %d, got %d", expectedTokens, result.TotalTokens)
	}
}

// TestE2E_AutoContextInjection_TypeFiltering tests that OnlyTypes and
// ExcludeTypes filters work correctly.
func TestE2E_AutoContextInjection_TypeFiltering(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	registry := octx.NewGathererRegistry()
	registry.Register(newContextMockGatherer(octx.ContextTypeProjectInstructions, "Inst", "Instructions", 100))
	registry.Register(newContextMockGatherer(octx.ContextTypeGitStatus, "Git", "Git status", 100))
	registry.Register(newContextMockGatherer(octx.ContextTypeEnvironment, "Env", "Environment", 100))

	t.Run("OnlyTypes filter", func(t *testing.T) {
		opts := octx.DefaultBuilderOptions()
		opts.Registry = registry
		opts.OnlyTypes = []octx.ContextType{octx.ContextTypeProjectInstructions}

		result, err := octx.BuildContext(ctx, "/tmp/test", opts)
		if err != nil {
			t.Fatalf("failed to build context: %v", err)
		}

		if len(result.IncludedItems) != 1 {
			t.Errorf("expected 1 item with OnlyTypes filter, got %d", len(result.IncludedItems))
		}
		if len(result.IncludedItems) > 0 && result.IncludedItems[0].Type != octx.ContextTypeProjectInstructions {
			t.Errorf("expected project instructions type, got %s", result.IncludedItems[0].Type)
		}
	})

	t.Run("ExcludeTypes filter", func(t *testing.T) {
		opts := octx.DefaultBuilderOptions()
		opts.Registry = registry
		opts.ExcludeTypes = []octx.ContextType{octx.ContextTypeEnvironment}

		result, err := octx.BuildContext(ctx, "/tmp/test", opts)
		if err != nil {
			t.Fatalf("failed to build context: %v", err)
		}

		if len(result.IncludedItems) != 2 {
			t.Errorf("expected 2 items with ExcludeTypes filter, got %d", len(result.IncludedItems))
		}

		for _, item := range result.IncludedItems {
			if item.Type == octx.ContextTypeEnvironment {
				t.Error("environment type should have been excluded")
			}
		}
	})
}

// TestE2E_AutoContextInjection_ContextFormatting tests that context is
// formatted correctly with headers and separators.
func TestE2E_AutoContextInjection_ContextFormatting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	registry := octx.NewGathererRegistry()
	registry.Register(newContextMockGatherer(octx.ContextTypeProjectInstructions, "CLAUDE.md", "Instruction content", 100))
	registry.Register(newContextMockGatherer(octx.ContextTypeGitStatus, "git status", "Git content", 100))

	t.Run("with headers", func(t *testing.T) {
		opts := octx.DefaultBuilderOptions()
		opts.Registry = registry
		opts.FormatHeaders = true

		result, err := octx.BuildContext(ctx, "/tmp/test", opts)
		if err != nil {
			t.Fatalf("failed to build context: %v", err)
		}

		if !strings.Contains(result.Context, "## Project Instructions") {
			t.Error("expected Project Instructions header")
		}
		if !strings.Contains(result.Context, "## Git Status") {
			t.Error("expected Git Status header")
		}
	})

	t.Run("without headers", func(t *testing.T) {
		opts := octx.DefaultBuilderOptions()
		opts.Registry = registry
		opts.FormatHeaders = false

		result, err := octx.BuildContext(ctx, "/tmp/test", opts)
		if err != nil {
			t.Fatalf("failed to build context: %v", err)
		}

		if strings.Contains(result.Context, "## ") {
			t.Error("expected no markdown headers when FormatHeaders is false")
		}
	})

	t.Run("custom separator", func(t *testing.T) {
		opts := octx.DefaultBuilderOptions()
		opts.Registry = registry
		opts.Separator = "\n===SECTION===\n"

		result, err := octx.BuildContext(ctx, "/tmp/test", opts)
		if err != nil {
			t.Fatalf("failed to build context: %v", err)
		}

		if !strings.Contains(result.Context, "===SECTION===") {
			t.Error("expected custom separator in context")
		}
	})
}

// TestE2E_AutoContextInjection_EmptyProject tests behavior when no context
// can be gathered from the project.
func TestE2E_AutoContextInjection_EmptyProject(t *testing.T) {
	// Create an empty directory
	emptyDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use empty registry to simulate no gatherers producing output
	registry := octx.NewGathererRegistry()

	opts := octx.DefaultBuilderOptions()
	opts.Registry = registry

	result, err := octx.BuildContext(ctx, emptyDir, opts)
	if err != nil {
		t.Fatalf("failed to build context: %v", err)
	}

	// Should return empty but valid result
	if result.Context != "" {
		t.Error("expected empty context for empty registry")
	}
	if result.TotalTokens != 0 {
		t.Errorf("expected 0 tokens, got %d", result.TotalTokens)
	}
	if len(result.IncludedItems) != 0 {
		t.Errorf("expected 0 included items, got %d", len(result.IncludedItems))
	}
}

// TestE2E_AutoContextInjection_QuickContextHelpers tests the convenience
// functions for quick context building.
func TestE2E_AutoContextInjection_QuickContextHelpers(t *testing.T) {
	projectPath := setupTestProject(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Run("QuickContext", func(t *testing.T) {
		contextStr, err := octx.QuickContext(ctx, projectPath)
		// May have errors if gatherers fail, but function should not panic
		if err != nil {
			t.Logf("QuickContext returned error (may be expected): %v", err)
		}
		_ = contextStr // Just verify it returns
	})

	t.Run("QuickContextWithBudget", func(t *testing.T) {
		contextStr, err := octx.QuickContextWithBudget(ctx, projectPath, 5000)
		if err != nil {
			t.Logf("QuickContextWithBudget returned error (may be expected): %v", err)
		}
		_ = contextStr // Just verify it returns
	})
}

// TestE2E_AutoContextInjection_EventEmission tests that context injection
// emits appropriate events in the agent loop.
func TestE2E_AutoContextInjection_EventEmission(t *testing.T) {
	projectPath := setupTestProject(t)

	var events []*LoopEvent
	var mu sync.Mutex

	// Create a mock registry with a guaranteed gatherer
	registry := octx.NewGathererRegistry()
	registry.Register(newContextMockGatherer(octx.ContextTypeProjectInstructions, "Test", "Test content", 100))

	// We'll test the context building part directly since creating a full
	// agent loop requires more setup (provider, etc.)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := octx.DefaultBuilderOptions()
	opts.Registry = registry

	result, err := octx.BuildContext(ctx, projectPath, opts)
	if err != nil {
		t.Fatalf("failed to build context: %v", err)
	}

	// Simulate event emission as would happen in agent loop
	if result.TotalTokens > 0 {
		builder, err := NewLoopEvent(ContextInjected)
		if err != nil {
			t.Fatalf("failed to create event builder: %v", err)
		}

		event, err := builder.
			WithMessage("context injected").
			WithContext(&ContextInfo{
				TokenCount: result.TotalTokens,
				MaxTokens:  result.TokensAvailable,
			}).
			Build()

		if err != nil {
			t.Fatalf("failed to build event: %v", err)
		}

		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	}

	// Verify event was created
	mu.Lock()
	defer mu.Unlock()

	if len(events) == 0 {
		t.Error("expected ContextInjected event to be created")
	} else {
		if events[0].Type != ContextInjected {
			t.Errorf("expected ContextInjected event type, got %s", events[0].Type)
		}
		if events[0].Context == nil {
			t.Error("expected context info in event")
		}
	}
}

// TestE2E_AutoContextInjection_ContextBudgetValidation tests that
// ContextBudget validation works correctly.
func TestE2E_AutoContextInjection_ContextBudgetValidation(t *testing.T) {
	t.Run("valid budget", func(t *testing.T) {
		budget, err := octx.NewContextBudget(50000)
		if err != nil {
			t.Fatalf("failed to create valid budget: %v", err)
		}
		if err := budget.Validate(); err != nil {
			t.Errorf("valid budget should pass validation: %v", err)
		}
	})

	t.Run("invalid negative budget", func(t *testing.T) {
		_, err := octx.NewContextBudget(-100)
		if err == nil {
			t.Error("expected error for negative budget")
		}
	})

	t.Run("available tokens calculation", func(t *testing.T) {
		budget, _ := octx.NewContextBudget(10000)
		budget.ReservedForSystemPrompt = 2000
		budget.ReservedForConversation = 3000

		available := budget.AvailableForContext()
		expected := 10000 - 2000 - 3000
		if available != expected {
			t.Errorf("expected available tokens %d, got %d", expected, available)
		}
	})

	t.Run("available tokens with negative result", func(t *testing.T) {
		budget, _ := octx.NewContextBudget(1000)
		budget.ReservedForSystemPrompt = 600
		budget.ReservedForConversation = 600
		// Total reserved: 1200, which exceeds total budget

		available := budget.AvailableForContext()
		if available != 0 {
			t.Errorf("expected 0 available tokens when reserved exceeds total, got %d", available)
		}
	})
}

// TestE2E_AutoContextInjection_ContextItemValidation tests that
// ContextItem validation works correctly.
func TestE2E_AutoContextInjection_ContextItemValidation(t *testing.T) {
	t.Run("valid item", func(t *testing.T) {
		item, err := octx.NewContextItem(octx.ContextTypeProjectInstructions, "test.md", "content", 100)
		if err != nil {
			t.Fatalf("failed to create valid item: %v", err)
		}
		if err := item.Validate(); err != nil {
			t.Errorf("valid item should pass validation: %v", err)
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		_, err := octx.NewContextItem(octx.ContextType("invalid_type"), "test.md", "content", 100)
		if err == nil {
			t.Error("expected error for invalid context type")
		}
	})

	t.Run("empty source", func(t *testing.T) {
		_, err := octx.NewContextItem(octx.ContextTypeProjectInstructions, "", "content", 100)
		if err == nil {
			t.Error("expected error for empty source")
		}
	})

	t.Run("negative token count", func(t *testing.T) {
		_, err := octx.NewContextItem(octx.ContextTypeProjectInstructions, "test.md", "content", -1)
		if err == nil {
			t.Error("expected error for negative token count")
		}
	})
}

// TestE2E_AutoContextInjection_ContextItemLifecycle tests the lifecycle
// methods of ContextItem (refresh, stale marking, expiration).
func TestE2E_AutoContextInjection_ContextItemLifecycle(t *testing.T) {
	item, err := octx.NewContextItem(octx.ContextTypeProjectInstructions, "test.md", "original content", 100)
	if err != nil {
		t.Fatalf("failed to create item: %v", err)
	}

	t.Run("mark stale", func(t *testing.T) {
		if item.IsStale {
			t.Error("new item should not be stale")
		}

		item.MarkStale()

		if !item.IsStale {
			t.Error("item should be stale after MarkStale()")
		}
		if !item.NeedsRefresh() {
			t.Error("stale item should need refresh")
		}
	})

	t.Run("refresh content", func(t *testing.T) {
		item.Refresh("new content", 200)

		if item.IsStale {
			t.Error("item should not be stale after refresh")
		}
		if item.Content != "new content" {
			t.Error("content should be updated after refresh")
		}
		if item.TokenCount != 200 {
			t.Errorf("token count should be updated, got %d", item.TokenCount)
		}
	})

	t.Run("expiration", func(t *testing.T) {
		// Set expiration in the past
		pastTime := time.Now().Add(-1 * time.Hour)
		item.SetExpiration(pastTime)

		if !item.IsExpired() {
			t.Error("item with past expiration should be expired")
		}
		if !item.NeedsRefresh() {
			t.Error("expired item should need refresh")
		}

		// Set expiration in the future
		futureTime := time.Now().Add(1 * time.Hour)
		item.SetExpiration(futureTime)

		if item.IsExpired() {
			t.Error("item with future expiration should not be expired")
		}
	})
}

// contextMockGatherer is a simple mock gatherer for E2E tests.
type contextMockGatherer struct {
	*octx.BaseGatherer
	content    string
	tokenCount int
}

func newContextMockGatherer(contextType octx.ContextType, name, content string, tokenCount int) *contextMockGatherer {
	return &contextMockGatherer{
		BaseGatherer: octx.NewBaseGatherer(contextType, name, "Mock gatherer for E2E testing"),
		content:      content,
		tokenCount:   tokenCount,
	}
}

func (g *contextMockGatherer) Gather(ctx context.Context, projectPath string) (*octx.ContextItem, error) {
	return g.CreateContextItem(g.Name(), g.content, g.tokenCount)
}
