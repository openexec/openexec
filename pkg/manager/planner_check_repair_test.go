package manager

import (
	"context"
	"strings"
	"testing"
)

// A plan fixed after its review that still fails the deterministic checks is
// repaired against those failures, not refused. Goal 4011a347's second round
// failed three times overnight on the masked-check lint (10-07, 01:17 to
// 05:38): each refusal ended the run and the next run planned from scratch.
func TestAFixedPlanFailingTheChecksIsRepairedNotRefused(t *testing.T) {
	e := newSchedulerTestEnv(t)
	e.mgr.cfg.MaxReviewCycles = 2
	masked := strings.Replace(replayPlanFixture, `"verification_script":"go test ./..."`, `"verification_script":"go test ./... || echo skipped"`, 1)
	clean := strings.Replace(replayPlanFixture, `"verification_script":"go test ./..."`, `"verification_script":"go test ./... || { echo \"tests failed\"; exit 1; }"`, 1)
	generated, reviewed := 0, 0
	var repairPrompt string
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(_ context.Context, prompt string) (string, error) {
		generated++
		switch generated {
		case 1:
			return replayPlanFixture, nil
		case 2:
			return masked, nil // the fix after review brings in a masked check
		default:
			repairPrompt = prompt
			return clean, nil
		}
	})
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) {
		reviewed++
		return rejectedReplayReview, nil
	})
	result, err := e.mgr.Plan(context.Background(), replayRequest())
	if err != nil {
		t.Fatalf("a plan the checks could name the fix for was refused: %v", err)
	}
	if !result.Valid || generated != 3 || reviewed != 1 {
		t.Fatalf("want one review, one fix and one check repair: valid=%v generated=%d reviewed=%d", result.Valid, generated, reviewed)
	}
	if !strings.Contains(repairPrompt, "masks failure") || !strings.Contains(repairPrompt, "change nothing else") {
		t.Fatalf("the repair was not given the check failures alone: %q", repairPrompt)
	}
	if !strings.Contains(result.Plan.Stories[0].VerificationScript, "exit 1") {
		t.Fatalf("the imported plan is not the repaired one: %q", result.Plan.Stories[0].VerificationScript)
	}
	// Replay after completion repeats nothing.
	if _, err := e.mgr.Plan(context.Background(), replayRequest()); err != nil || generated != 3 {
		t.Fatalf("completed repair repeated: generated=%d err=%v", generated, err)
	}
}

// The repair is bounded: a planner that keeps writing the masked check is
// refused after maxCheckRepairs repairs, with the checks' own findings.
func TestCheckRepairsAreBounded(t *testing.T) {
	e := newSchedulerTestEnv(t)
	e.mgr.cfg.MaxReviewCycles = 2
	generated := 0
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(context.Context, string) (string, error) {
		generated++
		if generated == 1 {
			return replayPlanFixture, nil
		}
		// Every answer still masks the check, worded differently each time so
		// it is a changed plan.
		return strings.Replace(replayPlanFixture, `"verification_script":"go test ./..."`, `"verification_script":"go test ./... || echo skip`+strings.Repeat("!", generated)+`"`, 1), nil
	})
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) {
		return rejectedReplayReview, nil
	})
	_, err := e.mgr.Plan(context.Background(), replayRequest())
	if err == nil || !strings.Contains(err.Error(), "the plan fixed after its review still fails required checks") || !strings.Contains(err.Error(), "masks failure") {
		t.Fatalf("an unrepairable plan was not refused with its findings: %v", err)
	}
	if generated != 2+maxCheckRepairs {
		t.Fatalf("plans generated %d, want the plan, its fix and %d repairs", generated, maxCheckRepairs)
	}
}

// A plan that also fails a human boundary is refused at once, not repaired:
// the planner could satisfy that lint by removing the owner's decision.
func TestABoundaryFailureIsNotRepaired(t *testing.T) {
	e := newSchedulerTestEnv(t)
	e.mgr.cfg.MaxReviewCycles = 2
	generated := 0
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(context.Context, string) (string, error) {
		generated++
		if generated == 1 {
			return replayPlanFixture, nil
		}
		fixed := strings.Replace(replayPlanFixture, `"verification_script":"go test ./..."`, `"verification_script":"go test ./... || echo skipped"`, 1)
		return strings.Replace(fixed, `"mode":"afk"`, `"mode":"hitl"`, 1), nil // an owner step with no decision reason
	})
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) {
		return rejectedReplayReview, nil
	})
	_, err := e.mgr.Plan(context.Background(), replayRequest())
	if err == nil || !strings.Contains(err.Error(), "human boundary lint refused") {
		t.Fatalf("a plan failing a human boundary was not refused: %v", err)
	}
	if generated != 2 {
		t.Fatalf("a plan failing a human boundary was sent to repair: %d plans generated", generated)
	}
}
