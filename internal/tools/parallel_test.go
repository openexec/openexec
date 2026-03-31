package tools

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestParallelExecutor(t *testing.T) {
	config := DefaultParallelConfig()
	executor := NewParallelExecutor(config)

	t.Run("Execute Single Tool", func(t *testing.T) {
		tool := NewSimpleTool("test", func(ctx context.Context) (interface{}, error) {
			return "result", nil
		})

		result, err := executor.Execute(context.Background(), []ParallelTool{tool})
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if result.SuccessCount != 1 {
			t.Errorf("expected 1 success, got %d", result.SuccessCount)
		}
		if len(result.Results) != 1 {
			t.Errorf("expected 1 result, got %d", len(result.Results))
		}
	})

	t.Run("Execute Multiple Tools", func(t *testing.T) {
		tools := []ParallelTool{
			NewSimpleTool("tool1", func(ctx context.Context) (interface{}, error) {
				return "result1", nil
			}),
			NewSimpleTool("tool2", func(ctx context.Context) (interface{}, error) {
				return "result2", nil
			}),
			NewSimpleTool("tool3", func(ctx context.Context) (interface{}, error) {
				return "result3", nil
			}),
		}

		result, err := executor.Execute(context.Background(), tools)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if result.SuccessCount != 3 {
			t.Errorf("expected 3 successes, got %d", result.SuccessCount)
		}
		if len(result.Results) != 3 {
			t.Errorf("expected 3 results, got %d", len(result.Results))
		}
	})

	t.Run("Execute With Error", func(t *testing.T) {
		tools := []ParallelTool{
			NewSimpleTool("tool1", func(ctx context.Context) (interface{}, error) {
				return "result1", nil
			}),
			NewSimpleTool("tool2", func(ctx context.Context) (interface{}, error) {
				return nil, errors.New("tool2 failed")
			}),
		}

		result, err := executor.Execute(context.Background(), tools)
		if err == nil {
			t.Error("expected error for failed tool")
		}

		if result.ErrorCount != 1 {
			t.Errorf("expected 1 error, got %d", result.ErrorCount)
		}
		if result.SuccessCount != 1 {
			t.Errorf("expected 1 success, got %d", result.SuccessCount)
		}
	})

	t.Run("Execute Empty", func(t *testing.T) {
		result, err := executor.Execute(context.Background(), []ParallelTool{})
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if len(result.Results) != 0 {
			t.Errorf("expected 0 results, got %d", len(result.Results))
		}
	})

	t.Run("Results By Name", func(t *testing.T) {
		tools := []ParallelTool{
			NewSimpleTool("tool1", func(ctx context.Context) (interface{}, error) {
				return "result1", nil
			}),
			NewSimpleTool("tool2", func(ctx context.Context) (interface{}, error) {
				return "result2", nil
			}),
		}

		result, _ := executor.Execute(context.Background(), tools)
		byName := result.ResultsByName()

		if len(byName) != 2 {
			t.Errorf("expected 2 results by name, got %d", len(byName))
		}

		if r, ok := byName["tool1"]; !ok {
			t.Error("expected tool1 in results")
		} else if r.Result != "result1" {
			t.Errorf("expected result1, got %v", r.Result)
		}
	})

	t.Run("Get Result", func(t *testing.T) {
		tools := []ParallelTool{
			NewSimpleTool("tool1", func(ctx context.Context) (interface{}, error) {
				return "result1", nil
			}),
		}

		result, _ := executor.Execute(context.Background(), tools)

		r, ok := result.GetResult("tool1")
		if !ok {
			t.Error("expected to find tool1")
		}
		if r.Result != "result1" {
			t.Errorf("expected result1, got %v", r.Result)
		}

		_, ok = result.GetResult("nonexistent")
		if ok {
			t.Error("should not find nonexistent tool")
		}
	})

	t.Run("Has Errors", func(t *testing.T) {
		tests := []struct {
			name     string
			tools    []ParallelTool
			expected bool
		}{
			{
				"no errors",
				[]ParallelTool{NewSimpleTool("tool1", func(ctx context.Context) (interface{}, error) {
					return "ok", nil
				})},
				false,
			},
			{
				"with errors",
				[]ParallelTool{NewSimpleTool("tool1", func(ctx context.Context) (interface{}, error) {
					return nil, errors.New("fail")
				})},
				true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, _ := executor.Execute(context.Background(), tt.tools)
				if result.HasErrors() != tt.expected {
					t.Errorf("expected HasErrors=%v, got %v", tt.expected, result.HasErrors())
				}
			})
		}
	})
}

func TestParallelConfig(t *testing.T) {
	config := DefaultParallelConfig()

	if config.MaxConcurrency != 4 {
		t.Errorf("expected MaxConcurrency 4, got %d", config.MaxConcurrency)
	}
	if config.Timeout != 30*time.Second {
		t.Errorf("expected Timeout 30s, got %v", config.Timeout)
	}
	if config.FailFast {
		t.Error("expected FailFast to be false")
	}
}

func TestParallelToolSet(t *testing.T) {
	set := NewParallelToolSet(DefaultParallelConfig())

	set.Add(NewSimpleTool("tool1", func(ctx context.Context) (interface{}, error) {
		return "result1", nil
	}))
	set.Add(NewSimpleTool("tool2", func(ctx context.Context) (interface{}, error) {
		return "result2", nil
	}))

	result, err := set.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.SuccessCount != 2 {
		t.Errorf("expected 2 successes, got %d", result.SuccessCount)
	}
}

func TestFileReadTool(t *testing.T) {
	tool := NewFileReadTool("/path/to/file.go")

	if tool.Name() != "read_file:/path/to/file.go" {
		t.Errorf("unexpected name: %s", tool.Name())
	}

	result, err := tool.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result != "content of /path/to/file.go" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestGrepTool(t *testing.T) {
	tool := NewGrepTool("pattern", "/path")

	if tool.Name() != "grep:pattern:/path" {
		t.Errorf("unexpected name: %s", tool.Name())
	}

	result, err := tool.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	matches, ok := result.([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", result)
	}

	if len(matches) != 2 {
		t.Errorf("expected 2 matches, got %d", len(matches))
	}
}

func TestToolResult(t *testing.T) {
	result := ToolResult{
		ToolName:  "test-tool",
		Result:    "test-result",
		Error:     nil,
		Duration:  10 * time.Millisecond,
		StartedAt: time.Now(),
	}

	if result.ToolName != "test-tool" {
		t.Errorf("expected tool name 'test-tool', got %s", result.ToolName)
	}
	if result.Result != "test-result" {
		t.Errorf("expected result 'test-result', got %v", result.Result)
	}
}

func TestExecutionResultError(t *testing.T) {
	result := &ExecutionResult{
		Results: []ToolResult{
			{ToolName: "tool1", Error: errors.New("error1")},
			{ToolName: "tool2", Error: errors.New("error2")},
		},
		ErrorCount: 2,
	}

	errMsg := result.Error()
	if errMsg == "" {
		t.Error("expected error message")
	}

	if !contains(errMsg, "tool1") || !contains(errMsg, "tool2") {
		t.Error("expected error message to contain tool names")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
