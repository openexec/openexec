package planner

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
)

type mockProvider struct {
	lastPrompt string
	response   string
}

func (m *mockProvider) Complete(ctx context.Context, prompt string) (string, error) {
	m.lastPrompt = prompt
	return m.response, nil
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
