package planner

import (
	"strings"
	"testing"
)

// validMinimalState returns the minimum IntentState that passes IsReady().
//
// CONTRACT:
// - validMinimalState().IsReady() == true (always)
// - Removing any required field causes IsReady() == false
// - Uses "greenfield" flow (no LegacyRepoPath required)
// - Uses "web" app type (no Platforms required)
func validMinimalState() IntentState {
	return IntentState{
		ProjectName:      "Test Project",
		Flow:             "greenfield",
		AppType:          "web",
		ProblemStatement: "Solve X",
		PrimaryGoals:     []Goal{{ID: "G-001", Description: "First goal"}},
		Constraints:      []Constraint{{ID: "C-001", Description: "Limit"}},
		Entities:         []Entity{{Name: "User", DataSource: "postgres"}},
	}
}

func TestIntentState_IsReady(t *testing.T) {
	tests := []struct {
		name     string
		state    IntentState
		expected bool
	}{
		// Positive test: valid minimal state is ready
		{
			name:     "valid_minimal_state_is_ready",
			state:    validMinimalState(),
			expected: true,
		},

		// Flow validation
		{
			name: "empty_flow_not_ready",
			state: func() IntentState {
				s := validMinimalState()
				s.Flow = ""
				return s
			}(),
			expected: false,
		},
		{
			name: "unknown_flow_not_ready",
			state: func() IntentState {
				s := validMinimalState()
				s.Flow = "unknown"
				return s
			}(),
			expected: false,
		},

		// AppType validation
		{
			name: "empty_app_type_not_ready",
			state: func() IntentState {
				s := validMinimalState()
				s.AppType = ""
				return s
			}(),
			expected: false,
		},
		{
			name: "unknown_app_type_not_ready",
			state: func() IntentState {
				s := validMinimalState()
				s.AppType = "unknown"
				return s
			}(),
			expected: false,
		},

		// ProblemStatement validation
		{
			name: "empty_problem_statement_not_ready",
			state: func() IntentState {
				s := validMinimalState()
				s.ProblemStatement = ""
				return s
			}(),
			expected: false,
		},

		// PrimaryGoals validation
		{
			name: "no_goals_not_ready",
			state: func() IntentState {
				s := validMinimalState()
				s.PrimaryGoals = nil
				return s
			}(),
			expected: false,
		},

		// Constraints validation
		{
			name: "no_constraints_not_ready",
			state: func() IntentState {
				s := validMinimalState()
				s.Constraints = nil
				return s
			}(),
			expected: false,
		},

		// Entities validation
		{
			name: "no_entities_not_ready",
			state: func() IntentState {
				s := validMinimalState()
				s.Entities = nil
				return s
			}(),
			expected: false,
		},
		{
			name: "entities_without_datasource_not_ready",
			state: func() IntentState {
				s := validMinimalState()
				s.Entities = []Entity{{Name: "User", DataSource: ""}}
				return s
			}(),
			expected: false,
		},

		// Platform validation for desktop/mobile
		{
			name: "desktop_without_platforms_not_ready",
			state: func() IntentState {
				s := validMinimalState()
				s.AppType = "desktop"
				s.Platforms = nil
				return s
			}(),
			expected: false,
		},
		{
			name: "mobile_without_platforms_not_ready",
			state: func() IntentState {
				s := validMinimalState()
				s.AppType = "mobile"
				s.Platforms = nil
				return s
			}(),
			expected: false,
		},
		{
			name: "desktop_with_platforms_is_ready",
			state: func() IntentState {
				s := validMinimalState()
				s.AppType = "desktop"
				s.Platforms = []string{"linux"}
				return s
			}(),
			expected: true,
		},
		{
			name: "mobile_with_platforms_is_ready",
			state: func() IntentState {
				s := validMinimalState()
				s.AppType = "mobile"
				s.Platforms = []string{"ios", "android"}
				return s
			}(),
			expected: true,
		},

		// Refactor flow validation
		{
			name: "refactor_without_legacy_path_not_ready",
			state: func() IntentState {
				s := validMinimalState()
				s.Flow = "refactor"
				s.LegacyRepoPath = ""
				return s
			}(),
			expected: false,
		},
		{
			name: "refactor_with_legacy_path_is_ready",
			state: func() IntentState {
				s := validMinimalState()
				s.Flow = "refactor"
				s.LegacyRepoPath = "/path/to/legacy"
				return s
			}(),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.state.IsReady()
			if got != tt.expected {
				t.Errorf("IsReady() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIntentState_RenderIntentMD(t *testing.T) {
	t.Run("renders_project_name_header", func(t *testing.T) {
		state := validMinimalState()
		state.ProjectName = "MyApp"

		output := state.RenderIntentMD()

		if !strings.Contains(output, "# Intent: MyApp") {
			t.Errorf("output missing project header, got:\n%s", output)
		}
	})

	t.Run("renders_goals_section", func(t *testing.T) {
		state := validMinimalState()
		state.ProblemStatement = "Build X"
		state.PrimaryGoals = []Goal{
			{ID: "G-001", Description: "First goal", SuccessCriteria: "Test 1", VerificationMethod: "Unit test"},
			{ID: "G-002", Description: "Second goal", SuccessCriteria: "Test 2", VerificationMethod: "E2E test"},
		}

		output := state.RenderIntentMD()

		checks := []string{
			"## Goals",
			"G-001: First goal",
			"G-002: Second goal",
			"Success Criteria: Test 1",
			"Success Criteria: Test 2",
			"Verification: Unit test",
			"Verification: E2E test",
		}
		for _, check := range checks {
			if !strings.Contains(output, check) {
				t.Errorf("output missing %q, got:\n%s", check, output)
			}
		}
	})

	t.Run("renders_requirements_section", func(t *testing.T) {
		state := validMinimalState()
		state.AppType = "web"

		output := state.RenderIntentMD()

		if !strings.Contains(output, "## Requirements") {
			t.Errorf("output missing Requirements section, got:\n%s", output)
		}
		if !strings.Contains(output, "Shape: web") {
			t.Errorf("output missing Shape, got:\n%s", output)
		}
	})

	t.Run("renders_constraints_section", func(t *testing.T) {
		state := validMinimalState()
		state.Constraints = []Constraint{
			{ID: "C-001", Description: "First constraint"},
			{ID: "C-002", Description: "Second constraint"},
		}

		output := state.RenderIntentMD()

		if !strings.Contains(output, "## Constraints") {
			t.Errorf("output missing Constraints section, got:\n%s", output)
		}
		if !strings.Contains(output, "C-001: First constraint") {
			t.Errorf("output missing first constraint, got:\n%s", output)
		}
		if !strings.Contains(output, "C-002: Second constraint") {
			t.Errorf("output missing second constraint, got:\n%s", output)
		}
	})

	t.Run("renders_entities_with_datasource", func(t *testing.T) {
		state := validMinimalState()
		state.Entities = []Entity{
			{Name: "User", DataSource: "postgres"},
			{Name: "Order", DataSource: "mysql"},
		}

		output := state.RenderIntentMD()

		if !strings.Contains(output, "User: Source of Truth: postgres") {
			t.Errorf("output missing User entity, got:\n%s", output)
		}
		if !strings.Contains(output, "Order: Source of Truth: mysql") {
			t.Errorf("output missing Order entity, got:\n%s", output)
		}
	})

	t.Run("renders_platforms_joined", func(t *testing.T) {
		state := validMinimalState()
		state.Platforms = []string{"linux", "macos"}

		output := state.RenderIntentMD()

		if !strings.Contains(output, "linux, macos") {
			t.Errorf("output missing joined platforms, got:\n%s", output)
		}
	})

	t.Run("renders_empty_platforms_gracefully", func(t *testing.T) {
		state := validMinimalState()
		state.Platforms = nil

		// Should not panic and should contain Platforms line (even if empty)
		output := state.RenderIntentMD()

		if !strings.Contains(output, "Platforms:") {
			t.Errorf("output missing Platforms line, got:\n%s", output)
		}
	})
}
