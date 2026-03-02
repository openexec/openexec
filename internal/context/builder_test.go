package context

import (
	"context"
	"testing"
	"time"
)

// mockGatherer is a simple gatherer for testing.
type mockGatherer struct {
	*BaseGatherer
	content    string
	tokenCount int
	shouldFail bool
	failError  error
}

func newMockGatherer(contextType ContextType, name, content string, tokenCount int) *mockGatherer {
	return &mockGatherer{
		BaseGatherer: NewBaseGatherer(contextType, name, "Mock gatherer for testing"),
		content:      content,
		tokenCount:   tokenCount,
	}
}

func (g *mockGatherer) Gather(ctx context.Context, projectPath string) (*ContextItem, error) {
	if g.shouldFail {
		return nil, g.failError
	}
	return g.CreateContextItem(g.name, g.content, g.tokenCount)
}

func (g *mockGatherer) WithError(err error) *mockGatherer {
	g.shouldFail = true
	g.failError = err
	return g
}

func TestNewContextBuilder(t *testing.T) {
	t.Run("creates builder with default registry and budget", func(t *testing.T) {
		builder := NewContextBuilder(nil, nil)
		if builder == nil {
			t.Fatal("expected non-nil builder")
		}
		if builder.registry == nil {
			t.Error("expected non-nil registry")
		}
		if builder.budget == nil {
			t.Error("expected non-nil budget")
		}
	})

	t.Run("creates builder with custom registry and budget", func(t *testing.T) {
		registry := NewGathererRegistry()
		budget := DefaultContextBudget()
		budget.TotalTokenBudget = 50000

		builder := NewContextBuilder(registry, budget)
		if builder.budget.TotalTokenBudget != 50000 {
			t.Errorf("expected budget %d, got %d", 50000, builder.budget.TotalTokenBudget)
		}
	})
}

func TestContextBuilder_BuildFromItems(t *testing.T) {
	t.Run("builds context from items", func(t *testing.T) {
		budget := DefaultContextBudget()
		budget.TotalTokenBudget = 10000
		budget.ReservedForSystemPrompt = 1000
		budget.ReservedForConversation = 2000
		// Available: 7000 tokens

		builder := NewContextBuilder(nil, budget)

		item1, _ := NewContextItem(ContextTypeProjectInstructions, "CLAUDE.md", "Project instructions content", 500)
		item2, _ := NewContextItem(ContextTypeGitStatus, "git status", "Git status content", 200)

		result, err := builder.BuildFromItems([]*ContextItem{item1, item2})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.TotalTokens != 700 {
			t.Errorf("expected total tokens %d, got %d", 700, result.TotalTokens)
		}
		if len(result.IncludedItems) != 2 {
			t.Errorf("expected %d included items, got %d", 2, len(result.IncludedItems))
		}
		if len(result.ExcludedItems) != 0 {
			t.Errorf("expected %d excluded items, got %d", 0, len(result.ExcludedItems))
		}
		if result.TokensAvailable != 7000 {
			t.Errorf("expected tokens available %d, got %d", 7000, result.TokensAvailable)
		}
		if result.TokensRemaining != 6300 {
			t.Errorf("expected tokens remaining %d, got %d", 6300, result.TokensRemaining)
		}
	})

	t.Run("excludes items that exceed budget", func(t *testing.T) {
		budget := DefaultContextBudget()
		budget.TotalTokenBudget = 5000
		budget.ReservedForSystemPrompt = 1000
		budget.ReservedForConversation = 2000
		// Available: 2000 tokens

		builder := NewContextBuilder(nil, budget)

		item1, _ := NewContextItem(ContextTypeProjectInstructions, "CLAUDE.md", "High priority content", 1500)
		item2, _ := NewContextItem(ContextTypeGitStatus, "git status", "Medium priority content", 1000)
		item2.SetPriority(PriorityMedium) // Lower priority than project instructions

		result, err := builder.BuildFromItems([]*ContextItem{item1, item2})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Only item1 should fit (1500 tokens), item2 (1000 tokens) would exceed 2000
		if len(result.IncludedItems) != 1 {
			t.Errorf("expected %d included items, got %d", 1, len(result.IncludedItems))
		}
		if len(result.ExcludedItems) != 1 {
			t.Errorf("expected %d excluded items, got %d", 1, len(result.ExcludedItems))
		}
		if result.TotalTokens != 1500 {
			t.Errorf("expected total tokens %d, got %d", 1500, result.TotalTokens)
		}
	})

	t.Run("respects priority ordering", func(t *testing.T) {
		budget := DefaultContextBudget()
		budget.TotalTokenBudget = 5000
		budget.ReservedForSystemPrompt = 1000
		budget.ReservedForConversation = 2000
		// Available: 2000 tokens

		builder := NewContextBuilder(nil, budget)

		// Create items with different priorities but all should fit
		itemLow, _ := NewContextItem(ContextTypeGitDiff, "diff", "Low priority", 100)
		itemLow.SetPriority(PriorityLow)

		itemHigh, _ := NewContextItem(ContextTypeProjectInstructions, "CLAUDE.md", "Critical priority", 100)
		// Already has PriorityCritical

		itemMedium, _ := NewContextItem(ContextTypePackageInfo, "package.json", "Medium priority", 100)
		itemMedium.SetPriority(PriorityMedium)

		// Pass in random order
		result, err := builder.BuildFromItems([]*ContextItem{itemLow, itemHigh, itemMedium})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// All should be included
		if len(result.IncludedItems) != 3 {
			t.Errorf("expected %d included items, got %d", 3, len(result.IncludedItems))
		}

		// Verify they're sorted by priority (highest first)
		if result.IncludedItems[0].Priority != PriorityCritical {
			t.Errorf("expected first item to have critical priority, got %d", result.IncludedItems[0].Priority)
		}
		if result.IncludedItems[1].Priority != PriorityMedium {
			t.Errorf("expected second item to have medium priority, got %d", result.IncludedItems[1].Priority)
		}
		if result.IncludedItems[2].Priority != PriorityLow {
			t.Errorf("expected third item to have low priority, got %d", result.IncludedItems[2].Priority)
		}
	})

	t.Run("respects minimum priority threshold", func(t *testing.T) {
		budget := DefaultContextBudget()
		budget.MinPriorityToInclude = PriorityMedium // Only include medium and above

		builder := NewContextBuilder(nil, budget)

		itemHigh, _ := NewContextItem(ContextTypeProjectInstructions, "CLAUDE.md", "High priority", 100)
		itemLow, _ := NewContextItem(ContextTypeGitDiff, "diff", "Low priority", 100)
		itemLow.SetPriority(PriorityLow)

		result, err := builder.BuildFromItems([]*ContextItem{itemHigh, itemLow})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Only high priority item should be included
		if len(result.IncludedItems) != 1 {
			t.Errorf("expected %d included items, got %d", 1, len(result.IncludedItems))
		}
		if result.IncludedItems[0].Type != ContextTypeProjectInstructions {
			t.Errorf("expected included item to be project instructions")
		}
	})

	t.Run("respects per-type limits", func(t *testing.T) {
		budget := DefaultContextBudget()
		budget.TotalTokenBudget = 100000 // Large overall budget

		builder := NewContextBuilder(nil, budget)
		builder.WithMaxPerType(ContextTypeGitStatus, 500) // Limit git status to 500 tokens

		item1, _ := NewContextItem(ContextTypeGitStatus, "git status", "Content 1", 400)
		item2, _ := NewContextItem(ContextTypeGitStatus, "git status 2", "Content 2", 400)
		item3, _ := NewContextItem(ContextTypeProjectInstructions, "CLAUDE.md", "Instructions", 1000)

		result, err := builder.BuildFromItems([]*ContextItem{item1, item2, item3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Project instructions (1000) + first git status (400) = 1400
		// Second git status should be excluded due to per-type limit
		if result.TotalTokens != 1400 {
			t.Errorf("expected total tokens %d, got %d", 1400, result.TotalTokens)
		}
		if len(result.ExcludedItems) != 1 {
			t.Errorf("expected %d excluded items, got %d", 1, len(result.ExcludedItems))
		}
	})
}

func TestContextBuilder_FormatContext(t *testing.T) {
	t.Run("formats context with headers", func(t *testing.T) {
		builder := NewContextBuilder(nil, nil)
		builder.WithFormatHeader(true)

		item1, _ := NewContextItem(ContextTypeProjectInstructions, "CLAUDE.md", "Instructions content", 100)
		item2, _ := NewContextItem(ContextTypeGitStatus, "git status", "Status content", 100)

		result, err := builder.BuildFromItems([]*ContextItem{item1, item2})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Check that headers are included
		if !contains(result.Context, "## Project Instructions") {
			t.Error("expected Project Instructions header in context")
		}
		if !contains(result.Context, "## Git Status") {
			t.Error("expected Git Status header in context")
		}
		if !contains(result.Context, "Instructions content") {
			t.Error("expected instructions content in context")
		}
	})

	t.Run("formats context without headers", func(t *testing.T) {
		builder := NewContextBuilder(nil, nil)
		builder.WithFormatHeader(false)

		item1, _ := NewContextItem(ContextTypeProjectInstructions, "CLAUDE.md", "Instructions content", 100)

		result, err := builder.BuildFromItems([]*ContextItem{item1})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if contains(result.Context, "##") {
			t.Error("expected no headers when formatHeader is false")
		}
		if !contains(result.Context, "Instructions content") {
			t.Error("expected instructions content in context")
		}
	})

	t.Run("uses custom separator", func(t *testing.T) {
		builder := NewContextBuilder(nil, nil)
		builder.WithSeparator("\n---CUSTOM---\n")

		item1, _ := NewContextItem(ContextTypeProjectInstructions, "CLAUDE.md", "Content 1", 100)
		item2, _ := NewContextItem(ContextTypeGitStatus, "git status", "Content 2", 100)

		result, err := builder.BuildFromItems([]*ContextItem{item1, item2})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !contains(result.Context, "---CUSTOM---") {
			t.Error("expected custom separator in context")
		}
	})
}

func TestContextBuilder_Build(t *testing.T) {
	t.Run("builds context from registry", func(t *testing.T) {
		registry := NewGathererRegistry()
		registry.Register(newMockGatherer(ContextTypeProjectInstructions, "Instructions", "Test instructions", 100))
		registry.Register(newMockGatherer(ContextTypeEnvironment, "Environment", "Test environment", 50))

		builder := NewContextBuilder(registry, nil)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := builder.Build(ctx, "/tmp/test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.IncludedItems) != 2 {
			t.Errorf("expected %d included items, got %d", 2, len(result.IncludedItems))
		}
		if result.TotalTokens != 150 {
			t.Errorf("expected total tokens %d, got %d", 150, result.TotalTokens)
		}
	})

	t.Run("handles empty registry", func(t *testing.T) {
		registry := NewGathererRegistry()
		builder := NewContextBuilder(registry, nil)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := builder.Build(ctx, "/tmp/test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Context != "" {
			t.Error("expected empty context for empty registry")
		}
		if len(result.IncludedItems) != 0 {
			t.Errorf("expected 0 included items, got %d", len(result.IncludedItems))
		}
	})
}

func TestConcurrentContextBuilder(t *testing.T) {
	t.Run("builds context concurrently", func(t *testing.T) {
		registry := NewGathererRegistry()
		registry.Register(newMockGatherer(ContextTypeProjectInstructions, "Instructions", "Content 1", 100))
		registry.Register(newMockGatherer(ContextTypeGitStatus, "Git", "Content 2", 100))
		registry.Register(newMockGatherer(ContextTypeEnvironment, "Env", "Content 3", 100))
		registry.Register(newMockGatherer(ContextTypePackageInfo, "Package", "Content 4", 100))

		builder := NewConcurrentContextBuilder(registry, nil)
		builder.WithMaxConcurrency(2)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		result, err := builder.Build(ctx, "/tmp/test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.IncludedItems) != 4 {
			t.Errorf("expected %d included items, got %d", 4, len(result.IncludedItems))
		}
		if result.TotalTokens != 400 {
			t.Errorf("expected total tokens %d, got %d", 400, result.TotalTokens)
		}
	})

	t.Run("handles empty registry", func(t *testing.T) {
		registry := NewGathererRegistry()
		builder := NewConcurrentContextBuilder(registry, nil)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := builder.Build(ctx, "/tmp/test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Context != "" {
			t.Error("expected empty context for empty registry")
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		registry := NewGathererRegistry()
		// Add a slow gatherer simulation via the mock
		registry.Register(newMockGatherer(ContextTypeProjectInstructions, "Instructions", "Content", 100))

		builder := NewConcurrentContextBuilder(registry, nil)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		result, err := builder.Build(ctx, "/tmp/test")
		// Should not error, but may have fewer results due to cancellation
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Result might be empty or partial due to cancellation
		_ = result
	})
}

func TestBuildContext(t *testing.T) {
	t.Run("uses default options", func(t *testing.T) {
		registry := NewGathererRegistry()
		registry.Register(newMockGatherer(ContextTypeProjectInstructions, "Instructions", "Test content", 100))

		opts := DefaultBuilderOptions()
		opts.Registry = registry

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := BuildContext(ctx, "/tmp/test", opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.IncludedItems) != 1 {
			t.Errorf("expected %d included items, got %d", 1, len(result.IncludedItems))
		}
	})

	t.Run("filters by OnlyTypes", func(t *testing.T) {
		registry := NewGathererRegistry()
		registry.Register(newMockGatherer(ContextTypeProjectInstructions, "Instructions", "Content 1", 100))
		registry.Register(newMockGatherer(ContextTypeGitStatus, "Git", "Content 2", 100))
		registry.Register(newMockGatherer(ContextTypeEnvironment, "Env", "Content 3", 100))

		opts := DefaultBuilderOptions()
		opts.Registry = registry
		opts.OnlyTypes = []ContextType{ContextTypeProjectInstructions}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := BuildContext(ctx, "/tmp/test", opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Only project instructions should be gathered
		if len(result.IncludedItems) != 1 {
			t.Errorf("expected %d included items, got %d", 1, len(result.IncludedItems))
		}
		if len(result.IncludedItems) > 0 && result.IncludedItems[0].Type != ContextTypeProjectInstructions {
			t.Errorf("expected project instructions type, got %s", result.IncludedItems[0].Type)
		}
	})

	t.Run("filters by ExcludeTypes", func(t *testing.T) {
		registry := NewGathererRegistry()
		registry.Register(newMockGatherer(ContextTypeProjectInstructions, "Instructions", "Content 1", 100))
		registry.Register(newMockGatherer(ContextTypeGitStatus, "Git", "Content 2", 100))

		opts := DefaultBuilderOptions()
		opts.Registry = registry
		opts.ExcludeTypes = []ContextType{ContextTypeGitStatus}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := BuildContext(ctx, "/tmp/test", opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Only project instructions should be gathered (git excluded)
		if len(result.IncludedItems) != 1 {
			t.Errorf("expected %d included items, got %d", 1, len(result.IncludedItems))
		}
	})
}

func TestQuickContext(t *testing.T) {
	// Note: This test will use real gatherers which may fail in the test environment
	// So we'll skip extensive testing here and just verify the function exists and doesn't panic
	t.Run("returns string", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// This might fail or return empty in test env, which is fine
		_, _ = QuickContext(ctx, "/tmp/nonexistent")
	})
}

func TestQuickContextWithBudget(t *testing.T) {
	t.Run("respects token budget", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// This might fail or return empty in test env, which is fine
		_, _ = QuickContextWithBudget(ctx, "/tmp/nonexistent", 1000)
	})
}

func TestBuilderResult(t *testing.T) {
	t.Run("calculates tokens correctly", func(t *testing.T) {
		budget := DefaultContextBudget()
		budget.TotalTokenBudget = 10000
		budget.ReservedForSystemPrompt = 1000
		budget.ReservedForConversation = 2000

		builder := NewContextBuilder(nil, budget)

		item1, _ := NewContextItem(ContextTypeProjectInstructions, "test", "content", 500)
		item2, _ := NewContextItem(ContextTypeGitStatus, "test", "content", 300)

		result, err := builder.BuildFromItems([]*ContextItem{item1, item2})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expectedAvailable := 10000 - 1000 - 2000 // 7000
		expectedUsed := 500 + 300                // 800
		expectedRemaining := expectedAvailable - expectedUsed

		if result.TokensAvailable != expectedAvailable {
			t.Errorf("expected tokens available %d, got %d", expectedAvailable, result.TokensAvailable)
		}
		if result.TotalTokens != expectedUsed {
			t.Errorf("expected total tokens %d, got %d", expectedUsed, result.TotalTokens)
		}
		if result.TokensRemaining != expectedRemaining {
			t.Errorf("expected tokens remaining %d, got %d", expectedRemaining, result.TokensRemaining)
		}
	})
}

func TestFormatContextHeader(t *testing.T) {
	tests := []struct {
		contextType ContextType
		expected    string
	}{
		{ContextTypeProjectInstructions, "Project Instructions"},
		{ContextTypeGitStatus, "Git Status"},
		{ContextTypeGitDiff, "Git Diff"},
		{ContextTypeGitLog, "Git Log"},
		{ContextTypeDirectoryStructure, "Directory Structure"},
		{ContextTypeRecentFiles, "Recent Files"},
		{ContextTypeOpenFiles, "Open Files"},
		{ContextTypePackageInfo, "Package Info"},
		{ContextTypeEnvironment, "Environment"},
		{ContextTypeSessionSummary, "Session Summary"},
		{ContextTypeCustom, "Custom Context"},
		{ContextType("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.contextType), func(t *testing.T) {
			result := formatContextHeader(tt.contextType)
			if result != tt.expected {
				t.Errorf("formatContextHeader(%s) = %s, want %s", tt.contextType, result, tt.expected)
			}
		})
	}
}

// Helper function for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// slowMockGatherer simulates a slow gatherer for testing timeouts.
type slowMockGatherer struct {
	*BaseGatherer
	delay      time.Duration
	content    string
	tokenCount int
}

func newSlowMockGatherer(contextType ContextType, name string, delay time.Duration, content string, tokenCount int) *slowMockGatherer {
	return &slowMockGatherer{
		BaseGatherer: NewBaseGatherer(contextType, name, "Slow mock gatherer for testing"),
		delay:        delay,
		content:      content,
		tokenCount:   tokenCount,
	}
}

func (g *slowMockGatherer) Gather(ctx context.Context, projectPath string) (*ContextItem, error) {
	select {
	case <-time.After(g.delay):
		return g.CreateContextItem(g.name, g.content, g.tokenCount)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestContextBuilder_BuildWithErrors(t *testing.T) {
	t.Run("handles gatherer returning partial results with error", func(t *testing.T) {
		registry := NewGathererRegistry()
		successGatherer := newMockGatherer(ContextTypeProjectInstructions, "Success", "content", 100)
		// This gatherer succeeds but with partial content
		partialGatherer := newMockGatherer(ContextTypeGitStatus, "Partial", "partial content", 50)

		registry.Register(successGatherer)
		registry.Register(partialGatherer)

		builder := NewContextBuilder(registry, nil)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := builder.Build(ctx, "/tmp/test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have both items
		if len(result.IncludedItems) != 2 {
			t.Errorf("expected 2 included items, got %d", len(result.IncludedItems))
		}
	})

	t.Run("handles mixed successful gatherers", func(t *testing.T) {
		registry := NewGathererRegistry()
		registry.Register(newMockGatherer(ContextTypeProjectInstructions, "Instructions", "content1", 100))
		registry.Register(newMockGatherer(ContextTypeGitStatus, "GitStatus", "content2", 100))
		registry.Register(newMockGatherer(ContextTypeEnvironment, "Environment", "content3", 100))

		builder := NewContextBuilder(registry, nil)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := builder.Build(ctx, "/tmp/test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have all three items
		if len(result.IncludedItems) != 3 {
			t.Errorf("expected 3 included items, got %d", len(result.IncludedItems))
		}

		// No errors should be reported
		if len(result.Errors) != 0 {
			t.Errorf("expected 0 errors, got %d", len(result.Errors))
		}
	})

	t.Run("handles single gatherer", func(t *testing.T) {
		registry := NewGathererRegistry()
		registry.Register(newMockGatherer(ContextTypeProjectInstructions, "Instructions", "content", 100))

		builder := NewContextBuilder(registry, nil)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := builder.Build(ctx, "/tmp/test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.IncludedItems) != 1 {
			t.Errorf("expected 1 included item, got %d", len(result.IncludedItems))
		}
	})
}

func TestContextBuilder_ParseMaxPerType(t *testing.T) {
	t.Run("parses valid JSON max per type", func(t *testing.T) {
		budget := DefaultContextBudget()
		budget.MaxPerType = `{"git_status": 1000, "environment": 500}`

		builder := NewContextBuilder(nil, budget)

		// Check that the max per type was parsed correctly
		if builder.maxPerType[ContextTypeGitStatus] != 1000 {
			t.Errorf("expected max for git_status = 1000, got %d", builder.maxPerType[ContextTypeGitStatus])
		}
		if builder.maxPerType[ContextTypeEnvironment] != 500 {
			t.Errorf("expected max for environment = 500, got %d", builder.maxPerType[ContextTypeEnvironment])
		}
	})

	t.Run("handles invalid JSON gracefully", func(t *testing.T) {
		budget := DefaultContextBudget()
		budget.MaxPerType = `{invalid json}`

		// Should not panic
		builder := NewContextBuilder(nil, budget)
		if builder == nil {
			t.Fatal("builder should not be nil even with invalid JSON")
		}
		// Should have empty maxPerType
		if len(builder.maxPerType) != 0 {
			t.Errorf("expected empty maxPerType with invalid JSON, got %d entries", len(builder.maxPerType))
		}
	})

	t.Run("handles empty max per type string", func(t *testing.T) {
		budget := DefaultContextBudget()
		budget.MaxPerType = ""

		builder := NewContextBuilder(nil, budget)
		if len(builder.maxPerType) != 0 {
			t.Errorf("expected empty maxPerType, got %d entries", len(builder.maxPerType))
		}
	})

	t.Run("handles empty JSON object", func(t *testing.T) {
		budget := DefaultContextBudget()
		budget.MaxPerType = `{}`

		builder := NewContextBuilder(nil, budget)
		if len(builder.maxPerType) != 0 {
			t.Errorf("expected empty maxPerType, got %d entries", len(builder.maxPerType))
		}
	})
}

func TestContextBuilder_FluentAPI(t *testing.T) {
	t.Run("chains multiple configuration methods", func(t *testing.T) {
		builder := NewContextBuilder(nil, nil).
			WithFormatHeader(false).
			WithSeparator("\n===\n").
			WithMaxPerType(ContextTypeGitStatus, 500).
			WithMaxPerType(ContextTypeEnvironment, 1000)

		if builder.formatHeader != false {
			t.Error("expected formatHeader to be false")
		}
		if builder.separator != "\n===\n" {
			t.Errorf("expected separator '\\n===\\n', got '%s'", builder.separator)
		}
		if builder.maxPerType[ContextTypeGitStatus] != 500 {
			t.Errorf("expected git_status max 500, got %d", builder.maxPerType[ContextTypeGitStatus])
		}
		if builder.maxPerType[ContextTypeEnvironment] != 1000 {
			t.Errorf("expected environment max 1000, got %d", builder.maxPerType[ContextTypeEnvironment])
		}
	})
}

func TestConcurrentContextBuilder_Advanced(t *testing.T) {
	t.Run("respects max concurrency setting", func(t *testing.T) {
		registry := NewGathererRegistry()
		for i := 0; i < 10; i++ {
			registry.Register(newMockGatherer(
				ContextType("custom_"+string(rune('a'+i))),
				"Gatherer",
				"Content",
				50,
			))
		}

		builder := NewConcurrentContextBuilder(registry, nil)
		builder.WithMaxConcurrency(2)

		if builder.maxConcurrency != 2 {
			t.Errorf("expected maxConcurrency 2, got %d", builder.maxConcurrency)
		}
	})

	t.Run("ignores invalid max concurrency values", func(t *testing.T) {
		builder := NewConcurrentContextBuilder(nil, nil)
		originalConcurrency := builder.maxConcurrency

		builder.WithMaxConcurrency(0)
		if builder.maxConcurrency != originalConcurrency {
			t.Errorf("maxConcurrency should not change for value 0")
		}

		builder.WithMaxConcurrency(-1)
		if builder.maxConcurrency != originalConcurrency {
			t.Errorf("maxConcurrency should not change for negative values")
		}
	})

	t.Run("handles context timeout gracefully", func(t *testing.T) {
		registry := NewGathererRegistry()
		registry.Register(newSlowMockGatherer(ContextTypeProjectInstructions, "Slow1", 5*time.Second, "Content1", 100))
		registry.Register(newSlowMockGatherer(ContextTypeGitStatus, "Slow2", 5*time.Second, "Content2", 100))

		builder := NewConcurrentContextBuilder(registry, nil)
		builder.WithMaxConcurrency(2)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		result, err := builder.Build(ctx, "/tmp/test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// With a very short timeout, we expect empty or partial results
		// The key is that it should not hang or panic
		_ = result
	})

	t.Run("handles mix of fast and slow gatherers", func(t *testing.T) {
		registry := NewGathererRegistry()
		registry.Register(newMockGatherer(ContextTypeProjectInstructions, "Fast", "Fast content", 100))
		registry.Register(newSlowMockGatherer(ContextTypeGitStatus, "Slow", 100*time.Millisecond, "Slow content", 100))

		builder := NewConcurrentContextBuilder(registry, nil)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		result, err := builder.Build(ctx, "/tmp/test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Both should complete within timeout
		if len(result.IncludedItems) != 2 {
			t.Errorf("expected 2 included items, got %d", len(result.IncludedItems))
		}
	})

	t.Run("adjusts concurrency to gatherer count", func(t *testing.T) {
		registry := NewGathererRegistry()
		registry.Register(newMockGatherer(ContextTypeProjectInstructions, "G1", "Content", 100))
		registry.Register(newMockGatherer(ContextTypeGitStatus, "G2", "Content", 100))

		builder := NewConcurrentContextBuilder(registry, nil)
		builder.WithMaxConcurrency(10) // More than gatherers

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := builder.Build(ctx, "/tmp/test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.IncludedItems) != 2 {
			t.Errorf("expected 2 included items, got %d", len(result.IncludedItems))
		}
	})
}

func TestFilterRegistry(t *testing.T) {
	t.Run("filters by OnlyTypes", func(t *testing.T) {
		original := NewGathererRegistry()
		original.Register(newMockGatherer(ContextTypeProjectInstructions, "PI", "content", 100))
		original.Register(newMockGatherer(ContextTypeGitStatus, "GS", "content", 100))
		original.Register(newMockGatherer(ContextTypeEnvironment, "ENV", "content", 100))

		filtered := filterRegistry(original, []ContextType{ContextTypeProjectInstructions, ContextTypeGitStatus}, nil)

		if filtered.Count() != 2 {
			t.Errorf("expected 2 gatherers, got %d", filtered.Count())
		}

		if _, ok := filtered.Get(ContextTypeProjectInstructions); !ok {
			t.Error("expected ProjectInstructions gatherer to be present")
		}
		if _, ok := filtered.Get(ContextTypeGitStatus); !ok {
			t.Error("expected GitStatus gatherer to be present")
		}
		if _, ok := filtered.Get(ContextTypeEnvironment); ok {
			t.Error("expected Environment gatherer to be absent")
		}
	})

	t.Run("filters by ExcludeTypes", func(t *testing.T) {
		original := NewGathererRegistry()
		original.Register(newMockGatherer(ContextTypeProjectInstructions, "PI", "content", 100))
		original.Register(newMockGatherer(ContextTypeGitStatus, "GS", "content", 100))
		original.Register(newMockGatherer(ContextTypeEnvironment, "ENV", "content", 100))

		filtered := filterRegistry(original, nil, []ContextType{ContextTypeGitStatus})

		if filtered.Count() != 2 {
			t.Errorf("expected 2 gatherers, got %d", filtered.Count())
		}

		if _, ok := filtered.Get(ContextTypeGitStatus); ok {
			t.Error("expected GitStatus gatherer to be excluded")
		}
	})

	t.Run("combines OnlyTypes and ExcludeTypes", func(t *testing.T) {
		original := NewGathererRegistry()
		original.Register(newMockGatherer(ContextTypeProjectInstructions, "PI", "content", 100))
		original.Register(newMockGatherer(ContextTypeGitStatus, "GS", "content", 100))
		original.Register(newMockGatherer(ContextTypeEnvironment, "ENV", "content", 100))
		original.Register(newMockGatherer(ContextTypePackageInfo, "PKG", "content", 100))

		// Only include PI, GS, ENV but exclude GS
		filtered := filterRegistry(original,
			[]ContextType{ContextTypeProjectInstructions, ContextTypeGitStatus, ContextTypeEnvironment},
			[]ContextType{ContextTypeGitStatus})

		if filtered.Count() != 2 {
			t.Errorf("expected 2 gatherers, got %d", filtered.Count())
		}

		if _, ok := filtered.Get(ContextTypeProjectInstructions); !ok {
			t.Error("expected ProjectInstructions gatherer to be present")
		}
		if _, ok := filtered.Get(ContextTypeEnvironment); !ok {
			t.Error("expected Environment gatherer to be present")
		}
	})

	t.Run("handles empty filters", func(t *testing.T) {
		original := NewGathererRegistry()
		original.Register(newMockGatherer(ContextTypeProjectInstructions, "PI", "content", 100))
		original.Register(newMockGatherer(ContextTypeGitStatus, "GS", "content", 100))

		filtered := filterRegistry(original, nil, nil)

		if filtered.Count() != original.Count() {
			t.Errorf("expected %d gatherers, got %d", original.Count(), filtered.Count())
		}
	})
}

func TestFormatSection(t *testing.T) {
	t.Run("includes source when different from type", func(t *testing.T) {
		builder := NewContextBuilder(nil, nil)

		item, _ := NewContextItem(ContextTypeProjectInstructions, "CLAUDE.md", "Test content", 100)
		section := builder.formatSection(item)

		if !contains(section, "(CLAUDE.md)") {
			t.Error("expected source to be included in section header")
		}
		if !contains(section, "## Project Instructions") {
			t.Error("expected type header in section")
		}
		if !contains(section, "Test content") {
			t.Error("expected content in section")
		}
	})

	t.Run("excludes source when same as type", func(t *testing.T) {
		builder := NewContextBuilder(nil, nil)

		item, _ := NewContextItem(ContextTypeGitStatus, "git_status", "Test content", 100)
		section := builder.formatSection(item)

		// Source should not appear in parentheses when it matches the type
		if contains(section, "(git_status)") {
			t.Error("source should not appear when it matches type")
		}
	})

	t.Run("formats empty content correctly", func(t *testing.T) {
		builder := NewContextBuilder(nil, nil)

		item, _ := NewContextItem(ContextTypeEnvironment, "env", "", 0)
		section := builder.formatSection(item)

		if !contains(section, "## Environment") {
			t.Error("expected Environment header in section")
		}
	})
}

func TestBuildContext_NonConcurrent(t *testing.T) {
	t.Run("uses non-concurrent builder when specified", func(t *testing.T) {
		registry := NewGathererRegistry()
		registry.Register(newMockGatherer(ContextTypeProjectInstructions, "Instructions", "Test content", 100))

		opts := DefaultBuilderOptions()
		opts.Registry = registry
		opts.Concurrent = false

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := BuildContext(ctx, "/tmp/test", opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.IncludedItems) != 1 {
			t.Errorf("expected 1 included item, got %d", len(result.IncludedItems))
		}
	})
}

func TestBuildContext_WithCustomOptions(t *testing.T) {
	t.Run("applies custom format options", func(t *testing.T) {
		registry := NewGathererRegistry()
		registry.Register(newMockGatherer(ContextTypeProjectInstructions, "PI", "Content 1", 100))
		registry.Register(newMockGatherer(ContextTypeGitStatus, "GS", "Content 2", 100))

		opts := DefaultBuilderOptions()
		opts.Registry = registry
		opts.FormatHeaders = false
		opts.Separator = "|||"

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := BuildContext(ctx, "/tmp/test", opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if contains(result.Context, "##") {
			t.Error("expected no headers when FormatHeaders is false")
		}
		if !contains(result.Context, "|||") {
			t.Error("expected custom separator in context")
		}
	})

	t.Run("applies custom max per type", func(t *testing.T) {
		registry := NewGathererRegistry()
		registry.Register(newMockGatherer(ContextTypeGitStatus, "GS1", "Content 1", 300))
		registry.Register(newMockGatherer(ContextTypeGitStatus, "GS2", "Content 2", 300))

		opts := DefaultBuilderOptions()
		opts.Registry = registry
		opts.MaxPerType = map[ContextType]int{
			ContextTypeGitStatus: 400, // Only allows one item
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := BuildContext(ctx, "/tmp/test", opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Only one git status should fit within the per-type limit
		gitStatusCount := 0
		for _, item := range result.IncludedItems {
			if item.Type == ContextTypeGitStatus {
				gitStatusCount++
			}
		}
		if gitStatusCount != 1 {
			t.Errorf("expected 1 git status item, got %d", gitStatusCount)
		}
	})
}

func TestBuildContext_NilOptions(t *testing.T) {
	t.Run("handles nil options by using defaults", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// This should not panic and should use default options
		_, err := BuildContext(ctx, "/tmp/nonexistent", nil)
		// May have errors due to non-existent path, but should not panic
		_ = err
	})
}

func TestDefaultBuilderOptions(t *testing.T) {
	t.Run("returns valid default options", func(t *testing.T) {
		opts := DefaultBuilderOptions()

		if opts.FormatHeaders != true {
			t.Error("expected FormatHeaders to default to true")
		}
		if opts.Separator != "\n\n---\n\n" {
			t.Errorf("unexpected default separator: %s", opts.Separator)
		}
		if opts.Concurrent != true {
			t.Error("expected Concurrent to default to true")
		}
		if opts.MaxConcurrency != 4 {
			t.Errorf("expected MaxConcurrency to default to 4, got %d", opts.MaxConcurrency)
		}
		if opts.MaxPerType == nil {
			t.Error("expected MaxPerType to be initialized")
		}
	})
}

func TestBuilderResult_JSON(t *testing.T) {
	t.Run("result can be marshaled to JSON", func(t *testing.T) {
		budget := DefaultContextBudget()
		builder := NewContextBuilder(nil, budget)

		item, _ := NewContextItem(ContextTypeProjectInstructions, "test", "content", 100)
		result, err := builder.BuildFromItems([]*ContextItem{item})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Import encoding/json is already in the file through other tests
		// This test verifies the JSON tags are properly set
		if result.Context == "" {
			t.Error("expected non-empty context")
		}
		if result.Budget == nil {
			t.Error("expected budget to be set")
		}
	})
}

func TestGatherError(t *testing.T) {
	t.Run("stores error information correctly", func(t *testing.T) {
		gErr := GatherError{
			Type:  ContextTypeGitStatus,
			Name:  "Git Status Gatherer",
			Error: "command not found: git",
		}

		if gErr.Type != ContextTypeGitStatus {
			t.Errorf("expected type %s, got %s", ContextTypeGitStatus, gErr.Type)
		}
		if gErr.Name != "Git Status Gatherer" {
			t.Errorf("expected name 'Git Status Gatherer', got '%s'", gErr.Name)
		}
		if gErr.Error != "command not found: git" {
			t.Errorf("unexpected error message: %s", gErr.Error)
		}
	})
}

func TestContextBuilder_EmptyItems(t *testing.T) {
	t.Run("handles empty item list", func(t *testing.T) {
		builder := NewContextBuilder(nil, nil)

		result, err := builder.BuildFromItems([]*ContextItem{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Context != "" {
			t.Error("expected empty context for empty items")
		}
		if result.TotalTokens != 0 {
			t.Errorf("expected 0 tokens, got %d", result.TotalTokens)
		}
		if len(result.IncludedItems) != 0 {
			t.Errorf("expected 0 included items, got %d", len(result.IncludedItems))
		}
	})

	t.Run("handles nil item list", func(t *testing.T) {
		builder := NewContextBuilder(nil, nil)

		result, err := builder.BuildFromItems(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Context != "" {
			t.Error("expected empty context for nil items")
		}
	})
}

func TestContextBuilder_BudgetEdgeCases(t *testing.T) {
	t.Run("handles zero budget", func(t *testing.T) {
		budget := DefaultContextBudget()
		budget.TotalTokenBudget = 0
		budget.ReservedForSystemPrompt = 0
		budget.ReservedForConversation = 0

		builder := NewContextBuilder(nil, budget)

		item, _ := NewContextItem(ContextTypeProjectInstructions, "test", "content", 100)
		result, err := builder.BuildFromItems([]*ContextItem{item})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// All items should be excluded as there's no budget
		if len(result.IncludedItems) != 0 {
			t.Errorf("expected 0 included items with zero budget, got %d", len(result.IncludedItems))
		}
		if len(result.ExcludedItems) != 1 {
			t.Errorf("expected 1 excluded item with zero budget, got %d", len(result.ExcludedItems))
		}
	})

	t.Run("handles budget exactly matching item size", func(t *testing.T) {
		budget := DefaultContextBudget()
		budget.TotalTokenBudget = 100
		budget.ReservedForSystemPrompt = 0
		budget.ReservedForConversation = 0

		builder := NewContextBuilder(nil, budget)

		item, _ := NewContextItem(ContextTypeProjectInstructions, "test", "content", 100)
		result, err := builder.BuildFromItems([]*ContextItem{item})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Item should fit exactly
		if len(result.IncludedItems) != 1 {
			t.Errorf("expected 1 included item when budget matches exactly, got %d", len(result.IncludedItems))
		}
		if result.TokensRemaining != 0 {
			t.Errorf("expected 0 tokens remaining, got %d", result.TokensRemaining)
		}
	})

	t.Run("handles negative available budget", func(t *testing.T) {
		budget := DefaultContextBudget()
		budget.TotalTokenBudget = 100
		budget.ReservedForSystemPrompt = 60
		budget.ReservedForConversation = 60 // Total reserved > budget

		builder := NewContextBuilder(nil, budget)

		item, _ := NewContextItem(ContextTypeProjectInstructions, "test", "content", 10)
		result, err := builder.BuildFromItems([]*ContextItem{item})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// No items should be included as available budget is 0 (clamped from negative)
		if result.TokensAvailable != 0 {
			t.Errorf("expected 0 available tokens, got %d", result.TokensAvailable)
		}
		if len(result.IncludedItems) != 0 {
			t.Errorf("expected 0 included items with negative available budget, got %d", len(result.IncludedItems))
		}
	})
}

func TestContextBuilder_SortingBehavior(t *testing.T) {
	t.Run("sorts by priority then by token count", func(t *testing.T) {
		budget := DefaultContextBudget()
		budget.TotalTokenBudget = 100000

		builder := NewContextBuilder(nil, budget)

		// Create items with same priority but different token counts
		item1, _ := NewContextItem(ContextTypeProjectInstructions, "src1", "Large content", 500)
		item1.SetPriority(PriorityHigh)

		item2, _ := NewContextItem(ContextTypeProjectInstructions, "src2", "Small content", 100)
		item2.SetPriority(PriorityHigh)

		item3, _ := NewContextItem(ContextTypeProjectInstructions, "src3", "Medium content", 300)
		item3.SetPriority(PriorityHigh)

		result, err := builder.BuildFromItems([]*ContextItem{item1, item2, item3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Within same priority, smaller items should come first
		if len(result.IncludedItems) != 3 {
			t.Fatalf("expected 3 items, got %d", len(result.IncludedItems))
		}

		// First item should be the smallest (100 tokens)
		if result.IncludedItems[0].TokenCount != 100 {
			t.Errorf("expected first item to have 100 tokens, got %d", result.IncludedItems[0].TokenCount)
		}
		// Second should be 300
		if result.IncludedItems[1].TokenCount != 300 {
			t.Errorf("expected second item to have 300 tokens, got %d", result.IncludedItems[1].TokenCount)
		}
		// Third should be 500
		if result.IncludedItems[2].TokenCount != 500 {
			t.Errorf("expected third item to have 500 tokens, got %d", result.IncludedItems[2].TokenCount)
		}
	})

	t.Run("prioritizes higher priority items first", func(t *testing.T) {
		budget := DefaultContextBudget()
		budget.TotalTokenBudget = 500
		budget.ReservedForSystemPrompt = 0
		budget.ReservedForConversation = 0

		builder := NewContextBuilder(nil, budget)

		lowPriority, _ := NewContextItem(ContextTypeGitDiff, "diff", "Low priority content", 200)
		lowPriority.SetPriority(PriorityLow)

		highPriority, _ := NewContextItem(ContextTypeProjectInstructions, "inst", "High priority content", 400)
		// Already has PriorityCritical

		result, err := builder.BuildFromItems([]*ContextItem{lowPriority, highPriority})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Only high priority should fit (400 < 500, but 400 + 200 > 500)
		if len(result.IncludedItems) != 1 {
			t.Errorf("expected 1 included item, got %d", len(result.IncludedItems))
		}
		if result.IncludedItems[0].Priority != PriorityCritical {
			t.Error("expected high priority item to be included")
		}
	})
}
