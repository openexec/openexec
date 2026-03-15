// Package dcp provides architecture verification tests for the Deterministic Control Plane.
//
// These tests enforce the architectural invariant that DCP is a thin tool-routing layer
// that MUST NOT have any dependencies on Pipeline or Loop packages. This separation
// ensures a single source of truth for orchestration state in Pipeline/Loop.
package dcp

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/router"
)

// =============================================================================
// T-US-002-CHS-01: DCP Import Boundary Test
// =============================================================================
//
// BEHAVIORAL NARRATIVE:
// Given the DCP package exists as a thin tool-routing layer
// When we analyze all imports in the DCP package
// Then we find NO imports from internal/pipeline or internal/loop packages
//
// RATIONALE:
// The single orchestration plane architecture requires that:
// - Pipeline/Loop are the ONLY orchestration entry points
// - DCP is a pure tool-routing adapter with no state management
// - This separation prevents accidental coupling of concerns
//
// If this test fails, it indicates an architectural violation that must be
// immediately addressed to maintain the single orchestration plane invariant.

// TestDCP_NeverImportsPipelineOrLoop verifies the architectural boundary between
// the tool-routing plane (DCP) and the orchestration plane (Pipeline/Loop).
//
// This is a compile-time invariant check that parses all Go source files in the
// DCP package and fails if any forbidden imports are found.
func TestDCP_NeverImportsPipelineOrLoop(t *testing.T) {
	// Forbidden imports that would violate the single orchestration plane
	forbiddenImports := []string{
		"github.com/openexec/openexec/internal/pipeline",
		"github.com/openexec/openexec/internal/loop",
	}

	// Get the current directory (internal/dcp)
	dcpDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	// Walk all .go files in the DCP package (excluding test files)
	var violations []string
	err = filepath.Walk(dcpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip non-Go files
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip test files (they may have broader dependencies for testing)
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Parse the file for imports only (fast path)
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Errorf("failed to parse %s: %v", path, err)
			return nil
		}

		// Check each import against forbidden list
		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			for _, forbidden := range forbiddenImports {
				if importPath == forbidden {
					relPath, _ := filepath.Rel(dcpDir, path)
					violations = append(violations, relPath+": imports "+forbidden)
				}
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk DCP directory: %v", err)
	}

	if len(violations) > 0 {
		t.Errorf("ARCHITECTURAL VIOLATION: DCP imports orchestration packages\n\n"+
			"The DCP package is a thin tool-routing layer and MUST NOT import:\n"+
			"  - internal/pipeline (owns phase state machine)\n"+
			"  - internal/loop (owns iteration execution)\n\n"+
			"Violations found:\n  %s\n\n"+
			"Resolution: Move the orchestration logic to Pipeline/Loop where it belongs.",
			strings.Join(violations, "\n  "))
	}
}

// =============================================================================
// T-US-002-CHS-02: DCP Statelessness Test
// =============================================================================
//
// BEHAVIORAL NARRATIVE:
// Given the DCP Coordinator is a stateless tool-routing adapter
// When ProcessQuery is called multiple times with the same input
// Then the same output is produced each time (modulo tool side-effects)
//
// This test verifies that DCP maintains no internal state between queries,
// ensuring that the orchestration plane (Pipeline/Loop) is the single source
// of truth for all state management.

// TestDCP_ProcessQueryIsStateless verifies that ProcessQuery is a pure function
// with respect to DCP's internal state. Same input should produce same output.
func TestDCP_ProcessQueryIsStateless(t *testing.T) {
	ctx := context.Background()

	t.Run("Same input produces same output", func(t *testing.T) {
		// GIVEN a Coordinator with a deterministic router and tool
		store, cleanup := newTestStore(t)
		defer cleanup()

		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "deterministic_tool",
				Args:       map[string]interface{}{"input": "test"},
				Confidence: 0.9,
			},
		}
		coord := NewCoordinator(mockRouter, store)

		// Register a deterministic tool that always returns the same value
		coord.RegisterTool(&mockCountingTool{name: "deterministic_tool"})

		// WHEN we call ProcessQuery multiple times with the same input
		results := make([]interface{}, 3)
		for i := 0; i < 3; i++ {
			result, err := coord.ProcessQuery(ctx, "test query")
			if err != nil {
				t.Fatalf("ProcessQuery[%d]: %v", i, err)
			}
			results[i] = result
		}

		// THEN all results should be identical
		for i := 1; i < len(results); i++ {
			if results[i] != results[0] {
				t.Errorf("ProcessQuery produced different results: %v vs %v", results[0], results[i])
			}
		}
	})

	t.Run("No internal state accumulates between queries", func(t *testing.T) {
		// GIVEN a Coordinator
		store, cleanup := newTestStore(t)
		defer cleanup()

		queryCount := 0
		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "counting_tool",
				Args:       map[string]interface{}{},
				Confidence: 0.9,
			},
		}
		coord := NewCoordinator(mockRouter, store)

		// Register a tool that tracks external call count (not internal state)
		coord.RegisterTool(&mockCountingTool{
			name: "counting_tool",
			onExecute: func() {
				queryCount++
			},
		})

		// WHEN we process many queries
		for i := 0; i < 100; i++ {
			_, err := coord.ProcessQuery(ctx, fmt.Sprintf("query %d", i))
			if err != nil {
				t.Fatalf("ProcessQuery[%d]: %v", i, err)
			}
		}

		// THEN the tool was called 100 times (no internal batching or caching)
		// This proves DCP doesn't accumulate state or batch operations
		if queryCount != 100 {
			t.Errorf("Expected 100 tool calls, got %d (DCP may be caching)", queryCount)
		}
	})

	t.Run("Each query is independent", func(t *testing.T) {
		// GIVEN a Coordinator with tools that can succeed or fail
		store, cleanup := newTestStore(t)
		defer cleanup()

		callIndex := 0
		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "alternating_tool",
				Args:       map[string]interface{}{},
				Confidence: 0.9,
			},
		}
		coord := NewCoordinator(mockRouter, store)

		// Tool alternates between success and error
		coord.RegisterTool(&mockAlternatingTool{
			name:      "alternating_tool",
			callIndex: &callIndex,
		})

		// WHEN we call ProcessQuery multiple times
		// THEN each query is independent - previous errors don't affect next success
		for i := 0; i < 6; i++ {
			_, err := coord.ProcessQuery(ctx, "test")
			if i%2 == 0 {
				// Even calls succeed
				if err != nil {
					t.Errorf("Query %d should succeed: %v", i, err)
				}
			} else {
				// Odd calls fail
				if err == nil {
					t.Errorf("Query %d should fail", i)
				}
			}
		}
	})
}

// mockAlternatingTool alternates between success and failure
type mockAlternatingTool struct {
	name      string
	callIndex *int
}

func (m *mockAlternatingTool) Name() string        { return m.name }
func (m *mockAlternatingTool) Description() string { return "Alternating tool" }
func (m *mockAlternatingTool) InputSchema() string { return "{}" }
func (m *mockAlternatingTool) Execute(ctx context.Context, args map[string]interface{}) (any, error) {
	idx := *m.callIndex
	*m.callIndex++
	if idx%2 == 0 {
		return "success", nil
	}
	return nil, fmt.Errorf("intentional failure at call %d", idx)
}

// TestDCP_AllowedDependencies documents the expected dependency graph for DCP.
// This test serves as documentation and validates the thin adapter pattern.
func TestDCP_AllowedDependencies(t *testing.T) {
	// These are the ONLY internal packages DCP should depend on:
	expectedDependencies := map[string]string{
		"github.com/openexec/openexec/internal/knowledge": "symbol/code knowledge store",
		"github.com/openexec/openexec/internal/router":    "BitNet intent routing",
		"github.com/openexec/openexec/internal/tools":     "tool interface and implementations",
		"github.com/openexec/openexec/internal/logging":   "structured logging",
		"github.com/openexec/openexec/internal/mode":      "mode classification (chat/task/run)",
		"github.com/openexec/openexec/internal/toolset":   "toolset registry and selection",
		"github.com/openexec/openexec/pkg/util":           "utility functions (PII scrubbing, etc.)",
	}

	dcpDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	// Find all internal imports in DCP production code
	internalImports := make(map[string]bool)
	err = filepath.Walk(dcpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return nil
		}

		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(importPath, "openexec") {
				internalImports[importPath] = true
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk DCP directory: %v", err)
	}

	// Verify all internal imports are expected
	for imp := range internalImports {
		if _, ok := expectedDependencies[imp]; !ok {
			// Check if it's a forbidden import
			if strings.Contains(imp, "internal/pipeline") || strings.Contains(imp, "internal/loop") {
				t.Errorf("FORBIDDEN: DCP imports %s (orchestration package)", imp)
			} else {
				// New dependency - document it but don't fail
				t.Logf("INFO: DCP has undocumented dependency on %s - consider documenting it", imp)
			}
		}
	}

	// Log the documented dependencies for clarity
	t.Log("DCP's documented internal dependencies:")
	for dep, purpose := range expectedDependencies {
		if internalImports[dep] {
			t.Logf("  ✓ %s (%s)", dep, purpose)
		} else {
			t.Logf("  - %s (%s) [not used]", dep, purpose)
		}
	}
}
