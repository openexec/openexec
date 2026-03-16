package dcp

import (
	"context"
	"errors"
	"testing"

	"github.com/openexec/openexec/internal/mode"
	"github.com/openexec/openexec/internal/router"
)

// mockRouterForRoute implements router.Router for Route tests
type mockRouterForRoute struct {
	intent *router.Intent
	err    error
}

func (m *mockRouterForRoute) ParseIntent(ctx context.Context, query string) (*router.Intent, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.intent, nil
}

func (m *mockRouterForRoute) RegisterTool(name, desc string, schema string) error {
	return nil
}

func TestCoordinator_Route_ModeClassification(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		expectedMode mode.Mode
	}{
		{
			name:         "chat mode for questions",
			query:        "what is the purpose of this function?",
			expectedMode: mode.ModeChat,
		},
		{
			name:         "chat mode for explanation requests",
			query:        "explain how the authentication works",
			expectedMode: mode.ModeChat,
		},
		{
			name:         "task mode for modifications",
			query:        "add a new endpoint for user registration",
			expectedMode: mode.ModeTask,
		},
		{
			name:         "task mode for fixes",
			query:        "fix the bug in login handler",
			expectedMode: mode.ModeTask,
		},
		{
			name:         "run mode for complex operations",
			query:        "implement the entire user management feature",
			expectedMode: mode.ModeRun,
		},
		{
			name:         "run mode for refactoring",
			query:        "refactor the database layer",
			expectedMode: mode.ModeRun,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockRouter := &mockRouterForRoute{
				intent: &router.Intent{
					ToolName:   "read_file",
					Args:       map[string]interface{}{},
					Confidence: 0.9,
				},
			}

			coord := NewCoordinator(mockRouter, nil)
			plan, err := coord.Route(context.Background(), tc.query)

			if err != nil {
				t.Fatalf("Route failed: %v", err)
			}

			if plan.Mode != tc.expectedMode {
				t.Errorf("Mode = %s, want %s", plan.Mode, tc.expectedMode)
			}
		})
	}
}

func TestCoordinator_Route_ToolsetSelection(t *testing.T) {
	tests := []struct {
		name            string
		query           string
		expectedToolset string
	}{
		{
			name:            "read operations use repo_readonly",
			query:           "search code for authentication patterns",
			expectedToolset: "repo_readonly",
		},
		{
			name:            "implementation tasks use coding_backend",
			query:           "implement api for user profiles",
			expectedToolset: "coding_backend",
		},
		{
			name:            "frontend tasks use coding_frontend",
			query:           "style the react ui component",
			expectedToolset: "coding_frontend",
		},
		{
			name:            "CI tasks use debug_ci",
			query:           "fix the ci pipeline test failure",
			expectedToolset: "debug_ci",
		},
		{
			name:            "research tasks use docs_research",
			query:           "research topic: best practices for security",
			expectedToolset: "docs_research",
		},
		{
			name:            "release tasks use release_ops",
			query:           "tag release version 2.0",
			expectedToolset: "release_ops",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockRouter := &mockRouterForRoute{
				intent: &router.Intent{
					ToolName:   "read_file",
					Args:       map[string]interface{}{},
					Confidence: 0.9,
				},
			}

			coord := NewCoordinator(mockRouter, nil)
			plan, err := coord.Route(context.Background(), tc.query)

			if err != nil {
				t.Fatalf("Route failed: %v", err)
			}

			if plan.Toolset != tc.expectedToolset {
				t.Errorf("Toolset = %s, want %s", plan.Toolset, tc.expectedToolset)
			}
		})
	}
}

func TestCoordinator_Route_SensitivityDetection(t *testing.T) {
	tests := []struct {
		name                string
		query               string
		expectedSensitivity Sensitivity
	}{
		{
			name:                "high sensitivity for passwords",
			query:               "update the password hashing function",
			expectedSensitivity: SensitivityHigh,
		},
		{
			name:                "high sensitivity for API keys",
			query:               "add the api_key validation",
			expectedSensitivity: SensitivityHigh,
		},
		{
			name:                "medium sensitivity for user data",
			query:               "update user profile handling",
			expectedSensitivity: SensitivityMedium,
		},
		{
			name:                "medium sensitivity for configs",
			query:               "modify the config settings",
			expectedSensitivity: SensitivityMedium,
		},
		{
			name:                "low sensitivity for general code",
			query:               "add a helper function for date formatting",
			expectedSensitivity: SensitivityLow,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockRouter := &mockRouterForRoute{
				intent: &router.Intent{
					ToolName:   "read_file",
					Args:       map[string]interface{}{},
					Confidence: 0.9,
				},
			}

			coord := NewCoordinator(mockRouter, nil)
			plan, err := coord.Route(context.Background(), tc.query)

			if err != nil {
				t.Fatalf("Route failed: %v", err)
			}

			if plan.Sensitivity != tc.expectedSensitivity {
				t.Errorf("Sensitivity = %s, want %s", plan.Sensitivity, tc.expectedSensitivity)
			}
		})
	}
}

func TestCoordinator_Route_RepoZones(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		expectedZones []string
	}{
		{
			name:          "api-related query",
			query:         "fix the API endpoint handler",
			expectedZones: []string{"internal/api"},
		},
		{
			name:          "database query",
			query:         "update the database migration",
			expectedZones: []string{"internal/db"},
		},
		{
			name:          "test-related query",
			query:         "add tests for the auth module",
			expectedZones: []string{"internal/auth", "tests/"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockRouter := &mockRouterForRoute{
				intent: &router.Intent{
					ToolName:   "read_file",
					Args:       map[string]interface{}{},
					Confidence: 0.9,
				},
			}

			coord := NewCoordinator(mockRouter, nil)
			plan, err := coord.Route(context.Background(), tc.query)

			if err != nil {
				t.Fatalf("Route failed: %v", err)
			}

			// Check that all expected zones are present
			for _, expected := range tc.expectedZones {
				found := false
				for _, zone := range plan.RepoZones {
					if zone == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected zone %q not found in %v", expected, plan.RepoZones)
				}
			}
		})
	}
}

func TestCoordinator_Route_NeedsFrontier(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		toolName       string
		confidence     float64
		needsFrontier  bool
	}{
		{
			name:          "run mode always needs frontier",
			query:         "implement the full authentication system",
			toolName:      "write_file",
			confidence:    0.9,
			needsFrontier: true,
		},
		{
			name:          "high sensitivity needs frontier",
			query:         "update password storage",
			toolName:      "read_file",
			confidence:    0.9,
			needsFrontier: true,
		},
		{
			name:          "low confidence needs frontier",
			query:         "do something unclear",
			toolName:      "read_file",
			confidence:    0.3,
			needsFrontier: true,
		},
		{
			name:          "simple read in chat mode can be local",
			query:         "what is this function doing?",
			toolName:      "read_file",
			confidence:    0.95,
			needsFrontier: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockRouter := &mockRouterForRoute{
				intent: &router.Intent{
					ToolName:   tc.toolName,
					Args:       map[string]interface{}{},
					Confidence: tc.confidence,
				},
			}

			coord := NewCoordinator(mockRouter, nil)
			plan, err := coord.Route(context.Background(), tc.query)

			if err != nil {
				t.Fatalf("Route failed: %v", err)
			}

			if plan.NeedsFrontier != tc.needsFrontier {
				t.Errorf("NeedsFrontier = %v, want %v", plan.NeedsFrontier, tc.needsFrontier)
			}
		})
	}
}

func TestCoordinator_Route_FallbackOnRouterError(t *testing.T) {
	mockRouter := &mockRouterForRoute{
		err: errors.New("routing failed"),
	}

	coord := NewCoordinator(mockRouter, nil)
	plan, err := coord.Route(context.Background(), "some query")

	if err != nil {
		t.Fatalf("Route should not fail on router error: %v", err)
	}

	if plan.Intent == nil {
		t.Fatal("Intent should not be nil")
	}

	if !plan.Intent.IsFallback {
		t.Error("Intent should be marked as fallback")
	}

	if plan.Confidence >= 0.5 {
		t.Errorf("Confidence should be low for fallback, got %f", plan.Confidence)
	}
}

func TestCoordinator_Route_KnowledgeSources(t *testing.T) {
	tests := []struct {
		name            string
		query           string
		expectedSources []string
	}{
		{
			name:            "commit history query",
			query:           "what changed recently",
			expectedSources: []string{"git_history"},
		},
		{
			name:            "test-related query",
			query:           "show me the test cases",
			expectedSources: []string{"test_files"},
		},
		{
			name:            "function lookup",
			query:           "find the function definition",
			expectedSources: []string{"code_symbols"},
		},
		{
			name:            "documentation query",
			query:           "check the readme for setup instructions",
			expectedSources: []string{"local_docs"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockRouter := &mockRouterForRoute{
				intent: &router.Intent{
					ToolName:   "read_file",
					Args:       map[string]interface{}{},
					Confidence: 0.9,
				},
			}

			coord := NewCoordinator(mockRouter, nil)
			plan, err := coord.Route(context.Background(), tc.query)

			if err != nil {
				t.Fatalf("Route failed: %v", err)
			}

			// Check that expected sources are present
			for _, expected := range tc.expectedSources {
				found := false
				for _, source := range plan.KnowledgeSources {
					if source == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected source %q not found in %v", expected, plan.KnowledgeSources)
				}
			}
		})
	}
}
