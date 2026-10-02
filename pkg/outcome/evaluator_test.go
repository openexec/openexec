package outcome

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type completion func(context.Context, string) (string, error)

func (f completion) Complete(c context.Context, p string) (string, error) { return f(c, p) }

func fixture() Input {
	c := Contract{GoalID: "semantic-convergence", GoalRevision: "1", ReadyRevision: "1", Goal: "Autonomously reassess failing strategies", Conditions: []Condition{{"D3", "Deployed restart recovery works"}}}
	id := Identity{Repository: "openexec", Workspace: "candidate", Candidate: "sha:dirty-content", Deployment: "deployed-sha"}
	r := Record{GoalID: c.GoalID, GoalRevision: c.GoalRevision, ReadyRevision: c.ReadyRevision, Round: 1, At: "2026-10-02T07:23:23Z", PlanID: "plan-1", Review: Review{Conditions: []ConditionReview{{"D3", "unsatisfied", "Callback returns 500 after redirect", "Restart recovery fails"}}, Summary: "D3 still fails", Identity: id}}
	return Input{Contract: c, Current: id, Facts: Facts{ExecutionGoalID: c.GoalID, ExecutionAvailable: true}, NextGoalID: c.GoalID, CurrentReview: &r}
}

func TestPR437IncidentCannotBecomeOwnerCoordination(t *testing.T) {
	in := fixture()
	in.Facts.ExecutionGoalID = "historical-admission-resume"
	in.Facts.ExecutionAvailable = false
	in.Facts.HardBoundaryGoalID = "historical-admission-resume"
	in.Facts.HardBoundary = "selected Goal is stopped or no longer active"
	in.Facts.RecoveryEvents = []string{"PR #437 retained admission/resume work; does not implement semantic convergence", "preflight used the historical Goal ID and refused"}
	in.NextGoalID = "historical-admission-resume"
	in.NextAction = "Continue retained PR #437"
	in.OwnerDecision = "Implement semantic Goal convergence or finish retained PR #437 first?"
	judge := completion(func(context.Context, string) (string, error) {
		t.Fatal("deterministic incident needs no model or owner")
		return "", nil
	})
	e, err := Evaluate(context.Background(), judge, in)
	if err != nil || e.Disposition != Recover {
		t.Fatalf("coordination leak: %+v %v", e, err)
	}
	// Reconstructing the accepted binding is sufficient; no historical Stop or
	// unfinished PR becomes authority over this outcome.
	in.Facts.ExecutionGoalID = in.Contract.GoalID
	in.Facts.ExecutionAvailable = true
	in.NextGoalID = in.Contract.GoalID
	in.OwnerDecision = ""
	e, err = Evaluate(context.Background(), judge, in)
	if err != nil || e.Disposition != Continue {
		t.Fatalf("historical hijack: %+v %v", e, err)
	}
	// A real Stop scoped to THIS Goal survives the recovery diagnosis.
	in.Facts.HardBoundaryGoalID = in.Contract.GoalID
	e, err = Evaluate(context.Background(), judge, in)
	if err != nil || e.Disposition != OwnerBoundary {
		t.Fatalf("current authority bypass: %+v %v", e, err)
	}
}

func TestExecutionAndHistoricalRouteAreIndependentChecks(t *testing.T) {
	for _, mode := range []string{"missing", "mismatched", "historical_next", "recovery"} {
		t.Run(mode, func(t *testing.T) {
			in := fixture()
			switch mode {
			case "missing":
				in.Facts.ExecutionAvailable = false
			case "mismatched":
				in.Facts.ExecutionGoalID = "old"
			case "historical_next":
				in.NextGoalID = "old"
			case "recovery":
				in.Facts.MachineRecovery = true
			}
			e, err := Evaluate(context.Background(), nil, in)
			if err != nil || e.Disposition != Recover {
				t.Fatal(e, err)
			}
		})
	}
}

func TestInvalidSemanticJudgmentsCannotAuthorizeContinuation(t *testing.T) {
	for _, answer := range []string{`{`, `{}`, `{"disposition":"invented","advancement":"unchanged","reason":"x"}`, `{"disposition":"continue","advancement":"73%","reason":"x"}`, `{"disposition":"continue","advancement":"unchanged","reason":"x","condition_ids":["unknown"]}`, `{"disposition":"continue","advancement":"unchanged","reason":"x"}`} {
		in := fixture()
		prior := *in.CurrentReview
		in.History = []Record{prior}
		in.CurrentReview.Round = 2
		_, err := Evaluate(context.Background(), completion(func(context.Context, string) (string, error) { return answer, nil }), in)
		if err == nil {
			t.Fatalf("invalid evaluation accepted: %s", answer)
		}
	}
}

func TestGenuineOwnerBoundaryAndEngineeringReassessment(t *testing.T) {
	for _, disposition := range []Disposition{OwnerBoundary, Reassess} {
		in := fixture()
		in.CurrentReview = nil
		in.OwnerDecision = "Does this require authority or merely a different implementation?"
		e, err := Evaluate(context.Background(), completion(func(context.Context, string) (string, error) {
			raw, _ := json.Marshal(Evaluation{Disposition: disposition, Advancement: "unknown", Reason: "Independent bounded evidence judgment"})
			return string(raw), nil
		}), in)
		if err != nil || e.Disposition != disposition {
			t.Fatal(e, err)
		}
	}
}

func TestThreeReviewsTriggerFreshReassessmentDespiteActivity(t *testing.T) {
	in := fixture()
	calls := 0
	judge := completion(func(_ context.Context, p string) (string, error) {
		calls++
		if strings.Contains(p, "Independently determine why") {
			return "Current callback evidence disproves wake-up diagnosis. Inspect repository-runner preflight and repair its candidate binding.", nil
		}
		return `{"disposition":"continue","advancement":"unchanged","reason":"Both observations fail at provider callback before login completes","condition_ids":["D3"]}`, nil
	})
	for round := 1; round <= 3; round++ {
		in.CurrentReview.Round = round
		in.CurrentReview.Review.Conditions[0].Evidence = []string{"Callback returns 500 after redirect", "Authentication fails when provider returns control", "Login callback still fails before a session exists"}[round-1]
		in.Facts.ExecutionOutcomes = append(in.Facts.ExecutionOutcomes, "task done, progress emitted, files changed, tests passed")
		in.PreviousHypotheses = []string{"strategy A", "strategy A prime", "strategy A double prime"}
		e, err := Evaluate(context.Background(), judge, in)
		if err != nil {
			t.Fatal(err)
		}
		if round < 3 && e.Disposition != Continue {
			t.Fatalf("premature stall round %d: %+v", round, e)
		}
		if round == 3 && e.Disposition != Reassess {
			t.Fatalf("failed to detect stall: %+v", e)
		}
		in.CurrentReview.Evaluation = e
		// JSON round trip simulates a process restart: no hidden in-memory count.
		b, _ := json.Marshal(in.CurrentReview)
		var retained Record
		_ = json.Unmarshal(b, &retained)
		in.History = append(in.History, retained)
	}
	in.CurrentReview = nil
	diagnosis, err := Reassessment(context.Background(), judge, in)
	if err != nil || !strings.Contains(diagnosis, "disproves") || calls != 3 {
		t.Fatalf("no fresh strategy: %s %v calls=%d", diagnosis, err, calls)
	}
}

func TestSameStateDownstreamEvidenceIsAdvancement(t *testing.T) {
	in := fixture()
	prior := *in.CurrentReview
	in.History = []Record{prior}
	in.CurrentReview.Round = 2
	in.CurrentReview.Review.Conditions = []ConditionReview{{"D3", "unsatisfied", "Payment succeeds; fulfilment callback now fails", "Fulfilment remains"}}
	judge := completion(func(context.Context, string) (string, error) {
		return `{"disposition":"continue","advancement":"advanced","reason":"Journey now completes payment; remaining failure is downstream","condition_ids":["D3"]}`, nil
	})
	e, err := Evaluate(context.Background(), judge, in)
	if err != nil || e.Disposition != Continue || e.Advancement != "advanced" {
		t.Fatal(e, err)
	}
}

func TestRevisionAndCandidateProvenance(t *testing.T) {
	for _, mode := range []string{"goal", "ready", "candidate", "deployment", "workspace", "old_history"} {
		t.Run(mode, func(t *testing.T) {
			in := fixture()
			switch mode {
			case "goal":
				in.CurrentReview.GoalRevision = "old"
			case "ready":
				in.CurrentReview.ReadyRevision = "old"
			case "candidate":
				in.Current.Candidate = "new-local-head"
			case "deployment":
				in.Current.Deployment = "new-deployment"
			case "workspace":
				in.Current.Workspace = "other-worktree"
			case "old_history":
				r := *in.CurrentReview
				r.GoalRevision = "old"
				r.Evaluation.Advancement = "unchanged"
				in.History = []Record{r, r}
			}
			e, err := Evaluate(context.Background(), nil, in)
			if err != nil {
				t.Fatal(err)
			}
			want := Recover
			if mode == "old_history" {
				want = Continue
			}
			if e.Disposition != want || e.Advancement == "advanced" {
				t.Fatalf("stale evidence counted: %+v", e)
			}
		})
	}
}

func TestConditionReviewCannotWeakenReady(t *testing.T) {
	for _, mode := range []string{"omit", "unknown_id", "unknown_state", "missing_evidence", "false_ready", "satisfied_gap"} {
		t.Run(mode, func(t *testing.T) {
			in := fixture()
			r := in.CurrentReview.Review
			switch mode {
			case "omit":
				r.Conditions = nil
			case "unknown_id":
				r.Conditions[0].ConditionID = "D4"
			case "unknown_state":
				r.Conditions[0].State = "progress"
			case "missing_evidence":
				r.Conditions[0].Evidence = ""
			case "false_ready":
				r.Ready = true
			case "satisfied_gap":
				r.Conditions[0].State = "satisfied"
				r.Ready = true
			}
			if ValidateReview(in.Contract, r) == nil {
				t.Fatal("weakened review accepted")
			}
		})
	}
}

func TestFreshDiagnosisIsNotAnchoredToPriorModelReasoning(t *testing.T) {
	in := fixture()
	r := *in.CurrentReview
	r.Reassessment = "PRIVATE OLD REASONING"
	r.Evaluation.Reason = "PRIVATE OLD REASONING"
	in.History = []Record{r, r}
	in.CurrentReview = nil
	_, err := Reassessment(context.Background(), completion(func(_ context.Context, p string) (string, error) {
		if strings.Contains(p, "PRIVATE OLD REASONING") || !strings.Contains(p, "verification observing the wrong target") {
			t.Fatal("anchored or shallow reassessment")
		}
		return "Evidence binds wrong deployment; correct observer target before implementing another fix.", nil
	}), in)
	if err != nil {
		t.Fatal(err)
	}
}
