package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	pagent "github.com/openexec/openexec/pkg/agent"
)

// mockOpenAIResponse builds a minimal OpenAI chat completion response JSON string.
func mockOpenAIResponse(content string) string {
	resp := map[string]interface{}{
		"id":      "test-id",
		"object":  "chat.completion",
		"model":   "gpt-4o",
		"created": 1700000000,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     10,
			"completion_tokens": 20,
			"total_tokens":      30,
		},
	}
	data, _ := json.Marshal(resp)
	return string(data)
}

// newTestProvider creates an OpenAI provider backed by an httptest server.
func newTestProvider(t *testing.T, handler http.HandlerFunc) (pagent.ProviderAdapter, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	provider, err := pagent.NewOpenAIProvider(pagent.OpenAIProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("failed to create test provider: %v", err)
	}
	return provider, server
}

func TestPlan_ReturnsValidWorkPlan(t *testing.T) {
	planJSON := `{
		"subtasks": [
			{"id": "1", "description": "Update auth module", "files": ["auth.go"], "dependencies": []},
			{"id": "2", "description": "Update API handler", "files": ["api.go"], "dependencies": ["1"]}
		]
	}`

	provider, server := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(mockOpenAIResponse(planJSON)))
	})
	defer server.Close()

	coord := NewTaskCoordinator(CoordinatorConfig{
		Provider:       provider,
		WorkerProvider: provider,
		MaxWorkers:     2,
		WorkDir:        t.TempDir(),
		Model:          "gpt-4o",
		WorkerModel:    "gpt-4o",
	})

	plan, err := coord.Plan(context.Background(), "refactor auth flow", []string{"auth.go", "api.go", "main.go"})
	if err != nil {
		t.Fatalf("Plan() error: %v", err)
	}

	if len(plan.Subtasks) != 2 {
		t.Fatalf("expected 2 subtasks, got %d", len(plan.Subtasks))
	}

	if plan.Subtasks[0].ID != "1" {
		t.Errorf("expected subtask ID '1', got %q", plan.Subtasks[0].ID)
	}
	if len(plan.Subtasks[1].Dependencies) != 1 || plan.Subtasks[1].Dependencies[0] != "1" {
		t.Errorf("expected subtask 2 to depend on 1, got %v", plan.Subtasks[1].Dependencies)
	}
}

func TestPlan_RejectsFileConflicts(t *testing.T) {
	planJSON := `{
		"subtasks": [
			{"id": "1", "description": "Task A", "files": ["shared.go"], "dependencies": []},
			{"id": "2", "description": "Task B", "files": ["shared.go"], "dependencies": []}
		]
	}`

	provider, server := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(mockOpenAIResponse(planJSON)))
	})
	defer server.Close()

	coord := NewTaskCoordinator(CoordinatorConfig{
		Provider: provider,
		MaxWorkers: 2,
		WorkDir:  t.TempDir(),
		Model:    "gpt-4o",
	})

	_, err := coord.Plan(context.Background(), "update shared", []string{"shared.go"})
	if err == nil {
		t.Fatal("Plan() should have returned error for file conflict")
	}
	if !contains(err.Error(), "file conflict") {
		t.Errorf("expected file conflict error, got: %v", err)
	}
}

func TestPlan_HandlesCodeFences(t *testing.T) {
	planJSON := "```json\n" + `{
		"subtasks": [
			{"id": "1", "description": "Do thing", "files": ["a.go"], "dependencies": []}
		]
	}` + "\n```"

	provider, server := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(mockOpenAIResponse(planJSON)))
	})
	defer server.Close()

	coord := NewTaskCoordinator(CoordinatorConfig{
		Provider: provider,
		MaxWorkers: 2,
		WorkDir:  t.TempDir(),
		Model:    "gpt-4o",
	})

	plan, err := coord.Plan(context.Background(), "do thing", []string{"a.go"})
	if err != nil {
		t.Fatalf("Plan() with code fences: %v", err)
	}
	if len(plan.Subtasks) != 1 {
		t.Fatalf("expected 1 subtask, got %d", len(plan.Subtasks))
	}
}

func TestTopologicalSort_Simple(t *testing.T) {
	subtasks := []Subtask{
		{ID: "3", Dependencies: []string{"1", "2"}},
		{ID: "1", Dependencies: nil},
		{ID: "2", Dependencies: []string{"1"}},
	}

	order, err := topologicalSort(subtasks)
	if err != nil {
		t.Fatalf("topologicalSort() error: %v", err)
	}

	// 1 must come before 2 and 3; 2 must come before 3
	indexOf := func(id string) int {
		for i, o := range order {
			if o == id {
				return i
			}
		}
		return -1
	}

	if indexOf("1") >= indexOf("2") {
		t.Error("1 should come before 2")
	}
	if indexOf("1") >= indexOf("3") {
		t.Error("1 should come before 3")
	}
	if indexOf("2") >= indexOf("3") {
		t.Error("2 should come before 3")
	}
}

func TestTopologicalSort_Cycle(t *testing.T) {
	subtasks := []Subtask{
		{ID: "1", Dependencies: []string{"2"}},
		{ID: "2", Dependencies: []string{"1"}},
	}

	_, err := topologicalSort(subtasks)
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
}

func TestTopologicalSort_NoDependencies(t *testing.T) {
	subtasks := []Subtask{
		{ID: "a", Dependencies: nil},
		{ID: "b", Dependencies: nil},
		{ID: "c", Dependencies: nil},
	}

	order, err := topologicalSort(subtasks)
	if err != nil {
		t.Fatalf("topologicalSort() error: %v", err)
	}
	if len(order) != 3 {
		t.Fatalf("expected 3 items in order, got %d", len(order))
	}
}

func TestBuildWaves(t *testing.T) {
	subtasks := []Subtask{
		{ID: "1", Dependencies: nil},
		{ID: "2", Dependencies: nil},
		{ID: "3", Dependencies: []string{"1", "2"}},
	}

	order, _ := topologicalSort(subtasks)
	waves := buildWaves(order, subtasks)

	if len(waves) != 2 {
		t.Fatalf("expected 2 waves, got %d", len(waves))
	}

	// First wave should have 1 and 2 (independent)
	if len(waves[0]) != 2 {
		t.Errorf("expected 2 subtasks in wave 0, got %d", len(waves[0]))
	}

	// Second wave should have 3
	if len(waves[1]) != 1 || waves[1][0].ID != "3" {
		t.Errorf("expected subtask 3 in wave 1, got %v", waves[1])
	}
}

func TestMerge_CombinesOutputs(t *testing.T) {
	coord := NewTaskCoordinator(CoordinatorConfig{
		MaxWorkers: 2,
		WorkDir:    t.TempDir(),
	})

	results := []*WorkerResult{
		{SubtaskID: "1", Status: "completed", Output: "Updated auth module"},
		{SubtaskID: "2", Status: "completed", Output: "Updated API handler"},
	}

	summary, err := coord.Merge(context.Background(), results)
	if err != nil {
		t.Fatalf("Merge() error: %v", err)
	}

	if !contains(summary, "Subtask 1: completed") {
		t.Error("summary should contain subtask 1 result")
	}
	if !contains(summary, "Subtask 2: completed") {
		t.Error("summary should contain subtask 2 result")
	}
}

func TestMerge_ReportsFailures(t *testing.T) {
	coord := NewTaskCoordinator(CoordinatorConfig{
		MaxWorkers: 2,
		WorkDir:    t.TempDir(),
	})

	results := []*WorkerResult{
		{SubtaskID: "1", Status: "completed", Output: "Done"},
		{SubtaskID: "2", Status: "failed", Error: "compilation error"},
	}

	summary, err := coord.Merge(context.Background(), results)
	if err != nil {
		t.Fatalf("Merge() error: %v", err)
	}

	if !contains(summary, "FAILED") {
		t.Error("summary should mention failed subtask")
	}
	if !contains(summary, "1 subtask(s) failed") {
		t.Error("summary should include failure count")
	}
}

func TestValidateNoFileConflicts_NoConflict(t *testing.T) {
	plan := &WorkPlan{
		Subtasks: []Subtask{
			{ID: "1", Files: []string{"a.go", "b.go"}},
			{ID: "2", Files: []string{"c.go", "d.go"}},
		},
	}
	if err := validateNoFileConflicts(plan); err != nil {
		t.Errorf("expected no conflict, got: %v", err)
	}
}

func TestValidateNoFileConflicts_WithConflict(t *testing.T) {
	plan := &WorkPlan{
		Subtasks: []Subtask{
			{ID: "1", Files: []string{"a.go", "shared.go"}},
			{ID: "2", Files: []string{"b.go", "shared.go"}},
		},
	}
	if err := validateNoFileConflicts(plan); err == nil {
		t.Error("expected conflict error")
	}
}

func TestStripCodeFences(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`{"a": 1}`, `{"a": 1}`},
		{"```json\n{\"a\": 1}\n```", `{"a": 1}`},
		{"```\n{\"a\": 1}\n```", `{"a": 1}`},
	}

	for _, tt := range tests {
		got := stripCodeFences(tt.input)
		if got != tt.expected {
			t.Errorf("stripCodeFences(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
