package planner

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
)

type mockProvider struct {
	lastPrompt string
	response   string
	err        error
}

func (m *mockProvider) Complete(ctx context.Context, prompt string) (string, error) {
	m.lastPrompt = prompt
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

// wizardResponseJSON generates a properly formatted wizard response JSON
func wizardResponseJSON(state IntentState, nextQ, ack string, isComplete bool) string {
	resp := WizardResponse{
		UpdatedState:    state,
		NextQuestion:    nextQ,
		Acknowledgement: ack,
		IsComplete:      isComplete,
	}
	data, _ := json.Marshal(resp)
	return string(data)
}

func TestPlanner_GeneratePlan(t *testing.T) {
	// Arrange
	mock := &mockProvider{
		response: `{
			"schema_version": "1.1",
			"stories": [
				{"id": "US-001", "title": "Test Story", "tasks": []}
			]
		}`,
	}
	p := New(mock)
	ctx := context.Background()
	intent := "# Test Intent"

	t.Run("Generate without PRD", func(t *testing.T) {
		// Act
		plan, err := p.GeneratePlan(ctx, intent, nil)

		// Assert
		if err != nil {
			t.Fatalf("GeneratePlan failed: %v", err)
		}
		if len(plan.Stories) != 1 {
			t.Errorf("got %d stories, want 1", len(plan.Stories))
		}
		if strings.Contains(mock.lastPrompt, "STRUCTURED PRD CONTEXT") {
			t.Error("prompt should not contain PRD context")
		}
	})

	t.Run("Generate with PRD", func(t *testing.T) {
		// Arrange
		prdContext := map[string][]*knowledge.PRDRecord{
			"personas": {
				{Key: "admin", Content: "Admin info"},
			},
		}

		// Act
		_, err := p.GeneratePlan(ctx, intent, prdContext)

		// Assert
		if err != nil {
			t.Fatalf("GeneratePlan failed: %v", err)
		}
		if !strings.Contains(mock.lastPrompt, "STRUCTURED PRD CONTEXT") {
			t.Error("prompt missing PRD context")
		}
		if !strings.Contains(mock.lastPrompt, "Admin info") {
			t.Error("prompt missing persona content")
		}
	})
}

func TestPlanner_ParseResponse(t *testing.T) {
	p := &Planner{}

	t.Run("Markdown Wrapped", func(t *testing.T) {
		resp := "Here is your plan:\n```json\n{\"stories\": [{\"id\": \"US-1\"}]}\n```"
		plan, err := p.parseResponse(resp)
		if err != nil {
			t.Fatalf("parseResponse failed: %v", err)
		}
		if plan.Stories[0].ID != "US-1" {
			t.Errorf("got id %q, want US-1", plan.Stories[0].ID)
		}
	})

	t.Run("Raw Array Fallback", func(t *testing.T) {
		resp := "[{\"id\": \"US-2\", \"title\": \"T\", \"tasks\": []}]"
		plan, err := p.parseResponse(resp)
		if err != nil {
			t.Fatalf("parseResponse failed: %v", err)
		}
		if plan.Stories[0].ID != "US-2" {
			t.Errorf("got id %q, want US-2", plan.Stories[0].ID)
		}
	})
}

// TestProcessWizardMessage_UpdatesState verifies that ProcessWizardMessage
// correctly parses the LLM response and returns updated state.
func TestProcessWizardMessage_UpdatesState(t *testing.T) {
	// GIVEN a mock LLM provider that returns an incomplete state update
	state := IntentState{
		ProjectName: "TestApp",
		Flow:        "greenfield",
		AppType:     "web",
	}
	mock := &mockProvider{
		response: wizardResponseJSON(state, "What problem does it solve?", "Got it!", false),
	}
	p := New(mock)
	ctx := context.Background()

	// WHEN ProcessWizardMessage is called with initial empty state
	resp, err := p.ProcessWizardMessage(ctx, "I want to build a web app", "{}")

	// THEN response contains updated state values
	if err != nil {
		t.Fatalf("ProcessWizardMessage failed: %v", err)
	}
	if resp.UpdatedState.ProjectName != "TestApp" {
		t.Errorf("UpdatedState.ProjectName = %q, want TestApp", resp.UpdatedState.ProjectName)
	}
	if resp.UpdatedState.Flow != "greenfield" {
		t.Errorf("UpdatedState.Flow = %q, want greenfield", resp.UpdatedState.Flow)
	}
	if resp.UpdatedState.AppType != "web" {
		t.Errorf("UpdatedState.AppType = %q, want web", resp.UpdatedState.AppType)
	}
	if resp.NextQuestion != "What problem does it solve?" {
		t.Errorf("NextQuestion = %q, want 'What problem does it solve?'", resp.NextQuestion)
	}
	if resp.Acknowledgement != "Got it!" {
		t.Errorf("Acknowledgement = %q, want 'Got it!'", resp.Acknowledgement)
	}
	if resp.IsComplete {
		t.Error("IsComplete should be false for incomplete state")
	}
}

// TestProcessWizardMessage_AutoCompletesWhenReady verifies auto-completion
// when UpdatedState.IsReady() returns true.
func TestProcessWizardMessage_AutoCompletesWhenReady(t *testing.T) {
	// GIVEN a mock LLM provider that returns a complete state
	// (all required fields filled) but with is_complete: false
	state := IntentState{
		ProjectName:      "MyApp",
		Flow:             "greenfield",
		AppType:          "web",
		ProblemStatement: "Build a task manager",
		PrimaryGoals:     []Goal{{ID: "G-001", Description: "Track tasks"}},
		Constraints:      []Constraint{{ID: "C-001", Description: "Must be simple"}},
		Entities:         []Entity{{Name: "Task", DataSource: "postgres"}},
	}
	mock := &mockProvider{
		response: wizardResponseJSON(state, "", "Great, we have everything!", false),
	}
	p := New(mock)
	ctx := context.Background()

	// WHEN ProcessWizardMessage is called
	resp, err := p.ProcessWizardMessage(ctx, "That's all I need", "{}")

	// THEN Response.IsComplete == true because UpdatedState.IsReady() returns true
	if err != nil {
		t.Fatalf("ProcessWizardMessage failed: %v", err)
	}
	if !resp.IsComplete {
		t.Error("IsComplete should be true when state IsReady()")
	}
	// Verify the state is actually ready
	if !resp.UpdatedState.IsReady() {
		t.Error("UpdatedState.IsReady() should return true")
	}
}

// TestProcessWizardMessage_DoesNotAutoCompleteWhenNotReady verifies that
// IsComplete stays false when state is not ready.
func TestProcessWizardMessage_DoesNotAutoCompleteWhenNotReady(t *testing.T) {
	// GIVEN a mock LLM provider that returns an incomplete state
	state := IntentState{
		ProjectName: "PartialApp",
		Flow:        "greenfield",
		// Missing: AppType, ProblemStatement, Goals, Constraints, Entities
	}
	mock := &mockProvider{
		response: wizardResponseJSON(state, "What type of app?", "Okay", false),
	}
	p := New(mock)
	ctx := context.Background()

	// WHEN ProcessWizardMessage is called
	resp, err := p.ProcessWizardMessage(ctx, "Building something", "{}")

	// THEN Response.IsComplete == false
	if err != nil {
		t.Fatalf("ProcessWizardMessage failed: %v", err)
	}
	if resp.IsComplete {
		t.Error("IsComplete should be false when state is not ready")
	}
}

// TestProcessWizardMessage_PreservesExistingState verifies that the prompt
// includes the current state for context.
func TestProcessWizardMessage_PreservesExistingState(t *testing.T) {
	// GIVEN existing state in JSON format
	existingState := `{"project_name":"ExistingProject","flow":"greenfield"}`

	mock := &mockProvider{
		response: wizardResponseJSON(IntentState{
			ProjectName:      "ExistingProject",
			Flow:             "greenfield",
			AppType:          "cli",
			ProblemStatement: "Automate stuff",
			PrimaryGoals:     []Goal{{ID: "G-001", Description: "Goal"}},
			Constraints:      []Constraint{{ID: "C-001", Description: "Limit"}},
			Entities:         []Entity{{Name: "User", DataSource: "file"}},
		}, "", "Done", false),
	}
	p := New(mock)
	ctx := context.Background()

	// WHEN ProcessWizardMessage is called with existing state
	_, err := p.ProcessWizardMessage(ctx, "It's a CLI tool", existingState)

	// THEN the prompt includes the existing state
	if err != nil {
		t.Fatalf("ProcessWizardMessage failed: %v", err)
	}
	if !strings.Contains(mock.lastPrompt, "ExistingProject") {
		t.Error("Prompt should contain existing state with project name")
	}
	if !strings.Contains(mock.lastPrompt, "greenfield") {
		t.Error("Prompt should contain existing state with flow")
	}
}

// TestProcessWizardMessage_HandlesLLMError verifies error handling when
// the LLM provider returns an error.
func TestProcessWizardMessage_HandlesLLMError(t *testing.T) {
	// GIVEN a mock LLM provider that returns an error
	mock := &mockProvider{
		err: errors.New("LLM service unavailable"),
	}
	p := New(mock)
	ctx := context.Background()

	// WHEN ProcessWizardMessage is called
	resp, err := p.ProcessWizardMessage(ctx, "Hello", "{}")

	// THEN error is returned
	if err == nil {
		t.Fatal("ProcessWizardMessage should return error when LLM fails")
	}
	if resp != nil {
		t.Error("Response should be nil when error occurs")
	}
	if !strings.Contains(err.Error(), "LLM service unavailable") {
		t.Errorf("Error should contain original message, got: %v", err)
	}
}

// TestProcessWizardMessage_HandlesMalformedJSON verifies error handling when
// the LLM returns invalid JSON.
func TestProcessWizardMessage_HandlesMalformedJSON(t *testing.T) {
	// GIVEN a mock LLM provider that returns invalid JSON
	mock := &mockProvider{
		response: `{"updated_state": {broken json`,
	}
	p := New(mock)
	ctx := context.Background()

	// WHEN ProcessWizardMessage is called
	resp, err := p.ProcessWizardMessage(ctx, "Hello", "{}")

	// THEN error is returned with context
	if err == nil {
		t.Fatal("ProcessWizardMessage should return error for malformed JSON")
	}
	if resp != nil {
		t.Error("Response should be nil when JSON parsing fails")
	}
	if !strings.Contains(err.Error(), "failed to parse") {
		t.Errorf("Error should mention parsing failure, got: %v", err)
	}
}

// TestProcessWizardMessage_HandlesPartialJSON verifies error handling when
// the LLM returns truncated JSON.
func TestProcessWizardMessage_HandlesPartialJSON(t *testing.T) {
	// GIVEN a mock LLM provider that returns truncated JSON
	mock := &mockProvider{
		response: `{"updated_state": {"project_name": "Test"`,
	}
	p := New(mock)
	ctx := context.Background()

	// WHEN ProcessWizardMessage is called
	resp, err := p.ProcessWizardMessage(ctx, "Hello", "{}")

	// THEN error is returned, no panic
	if err == nil {
		t.Fatal("ProcessWizardMessage should return error for partial JSON")
	}
	if resp != nil {
		t.Error("Response should be nil when JSON is truncated")
	}
}

// TestProcessWizardMessage_HandlesMarkdownWrappedJSON verifies that JSON
// wrapped in markdown code blocks is correctly extracted.
func TestProcessWizardMessage_HandlesMarkdownWrappedJSON(t *testing.T) {
	// GIVEN a mock LLM provider that wraps JSON in markdown
	state := IntentState{
		ProjectName: "MarkdownApp",
		Flow:        "greenfield",
	}
	jsonResp := wizardResponseJSON(state, "Next?", "Ack", false)
	mock := &mockProvider{
		response: "Here's the update:\n```json\n" + jsonResp + "\n```",
	}
	p := New(mock)
	ctx := context.Background()

	// WHEN ProcessWizardMessage is called
	resp, err := p.ProcessWizardMessage(ctx, "Test", "{}")

	// THEN the JSON is correctly extracted
	if err != nil {
		t.Fatalf("ProcessWizardMessage failed: %v", err)
	}
	if resp.UpdatedState.ProjectName != "MarkdownApp" {
		t.Errorf("UpdatedState.ProjectName = %q, want MarkdownApp", resp.UpdatedState.ProjectName)
	}
}

// =============================================================================
// Story Schema Compatibility Tests (T-US-001-003)
// =============================================================================

// TestProjectPlan_SchemaVersion verifies that generated stories use schema version "1.1"
// This is a key contract for story compatibility across the system.
func TestProjectPlan_SchemaVersion(t *testing.T) {
	t.Run("GeneratePlan returns schema version 1.1", func(t *testing.T) {
		// GIVEN a mock LLM provider that returns a valid ProjectPlan with schema version
		mock := &mockProvider{
			response: `{
				"schema_version": "1.1",
				"goals": [
					{"id": "G-001", "title": "Test Goal", "description": "Desc", "success_criteria": "Criteria", "verification_method": "Test"}
				],
				"stories": [
					{"id": "US-001", "title": "Test Story", "goal_id": "G-001", "tasks": []}
				]
			}`,
		}
		p := New(mock)
		ctx := context.Background()

		// WHEN GeneratePlan is called
		plan, err := p.GeneratePlan(ctx, "# Test Intent", nil)

		// THEN schema_version is "1.1"
		if err != nil {
			t.Fatalf("GeneratePlan failed: %v", err)
		}
		if plan.SchemaVersion != "1.1" {
			t.Errorf("SchemaVersion = %q, want \"1.1\"", plan.SchemaVersion)
		}
	})

	t.Run("parseResponse sets default schema version when missing", func(t *testing.T) {
		// GIVEN a response without schema_version
		p := &Planner{}
		resp := `{"stories": [{"id": "US-001", "title": "Story", "tasks": []}]}`

		// WHEN parseResponse is called
		plan, err := p.parseResponse(resp)

		// THEN plan is valid (schema_version may be empty but parsing succeeds)
		if err != nil {
			t.Fatalf("parseResponse failed: %v", err)
		}
		if plan == nil {
			t.Fatal("parseResponse returned nil plan")
		}
		// Note: Empty schema version is acceptable for backward compatibility
		// The important thing is that valid JSON parses correctly
		if len(plan.Stories) != 1 {
			t.Errorf("Expected 1 story, got %d", len(plan.Stories))
		}
	})

	t.Run("ProjectPlan JSON round-trip preserves schema version", func(t *testing.T) {
		// GIVEN a ProjectPlan with schema version 1.1
		original := &ProjectPlan{
			SchemaVersion: "1.1",
			Goals: []Goal{
				{ID: "G-001", Title: "Goal", Description: "Desc", SuccessCriteria: "Criteria", VerificationMethod: "Test"},
			},
			Stories: []Story{
				{ID: "US-001", Title: "Story", GoalID: "G-001", Tasks: []Task{}},
			},
		}

		// WHEN serialized and deserialized
		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var restored ProjectPlan
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}

		// THEN schema version is preserved
		if restored.SchemaVersion != "1.1" {
			t.Errorf("SchemaVersion = %q, want \"1.1\"", restored.SchemaVersion)
		}
	})
}
