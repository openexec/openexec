package planner

import (
	"reflect"
	"strings"
	"testing"
)

func TestPlanStaleBaseRefIssues(t *testing.T) {
	t.Run("nil and clean plans have no issues", func(t *testing.T) {
		if got := PlanStaleBaseRefIssues(nil); len(got) != 0 {
			t.Fatalf("nil plan: got %v", got)
		}
		clean := &ProjectPlan{Stories: []Story{{
			ID:                 "US-001",
			VerificationScript: "git diff --exit-code origin/main...HEAD -- main",
			Tasks:              []Task{{ID: "T-US-001-001", VerificationScript: "git log origin/master..HEAD"}},
		}}}
		if got := PlanStaleBaseRefIssues(clean); len(got) != 0 {
			t.Fatalf("origin/ refs must pass, got %v", got)
		}
	})

	t.Run("goal-less plan: story and task scripts are keyed by owner", func(t *testing.T) {
		plan := &ProjectPlan{Stories: []Story{
			{ID: "US-001", VerificationScript: "git diff main...HEAD"},
			{ID: "US-002", VerificationScript: "git diff origin/main...HEAD", Tasks: []Task{
				{ID: "T-US-002-001", VerificationScript: "echo ok"},
				{ID: "T-US-002-002", VerificationScript: "git log master..HEAD"},
			}},
		}}
		issues := PlanStaleBaseRefIssues(plan)
		want := []string{"story US-001", "story US-002 task T-US-002-002"}
		if got := StaleBaseRefOwners(issues); !reflect.DeepEqual(got, want) {
			t.Fatalf("owners = %v, want %v", got, want)
		}
		for owner, issue := range issues {
			if !strings.Contains(issue, "origin/") {
				t.Errorf("%s: diagnostic must name the origin/ fix, got %q", owner, issue)
			}
		}
	})
}
