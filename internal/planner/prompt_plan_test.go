package planner

import (
	"context"
	"strings"
	"testing"
)

func TestReviewAndFixPromptsShortenOnlyContinuedTasks(t *testing.T) {
	finding := strings.Repeat("a review finding carried by a fix task. ", 8000) // ~320 KB
	fresh := strings.Repeat("new work the planner may change. ", 100)
	plan := &ProjectPlan{
		Stories: []Story{{ID: "US-014", Title: "Self-watch", Tasks: []Task{
			{ID: "repair-1", Title: "Fix: SR6", Description: finding, VerificationScript: "go test ./..."},
			{ID: "T-US-014-006", Title: "New", Description: fresh + finding, VerificationScript: "go test ./..."},
		}}},
		Continues: []string{"US-014", "repair-1"},
	}
	provider := &mockProvider{response: `{"approved":false,"assessment":"fix the lint"}`}
	review, err := New(provider).ReviewPlan(context.Background(), "intent", plan)
	if err != nil {
		t.Fatal(err)
	}
	reviewPrompt := provider.lastPrompt
	provider.response = `[{"id":"US-014","title":"Self-watch","tasks":[{"id":"repair-1","title":"Fix: SR6","verification_script":"go test ./..."}]}]`
	if _, err := New(provider).RefinePlan(context.Background(), "intent", plan, review); err != nil {
		t.Fatal(err)
	}
	for name, prompt := range map[string]string{"review": reviewPrompt, "fix": provider.lastPrompt} {
		if !strings.Contains(prompt, `"id":"repair-1"`) || !strings.Contains(prompt, "kept as imported") {
			t.Fatalf("%s prompt does not name the continued task as shortened", name)
		}
		// The continued task is shortened; the new task keeps every byte.
		if got, want := strings.Count(prompt, "a review finding carried by a fix task."), 8000+continuedDescriptionBytes/40+1; got > want {
			t.Fatalf("%s prompt carries %d copies of the continued finding, want at most %d", name, got, want)
		}
		if !strings.Contains(prompt, fresh) {
			t.Fatalf("%s prompt shortened a task the plan does not continue", name)
		}
	}
	if plan.Stories[0].Tasks[0].Description != finding {
		t.Fatal("shortening the prompt changed the plan itself")
	}
}
