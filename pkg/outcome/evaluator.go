// Package outcome supplies bounded judgments at existing execution boundaries.
// It owns no Goal, queue, scheduler, permission or recovery loop. Callers supply
// authoritative facts and apply judgments through their existing machinery.
package outcome

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type Condition struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

// Contract is a projection of an accepted revision, never acceptance authority.
type Contract struct {
	GoalID        string      `json:"goal_id"`
	GoalRevision  string      `json:"goal_revision"`
	ReadyRevision string      `json:"ready_revision"`
	Goal          string      `json:"goal"`
	Conditions    []Condition `json:"conditions"`
}

type Identity struct {
	Repository string `json:"repository"`
	Workspace  string `json:"workspace"`
	Candidate  string `json:"candidate"`
	Deployment string `json:"deployment,omitempty"`
}

type ConditionReview struct {
	ConditionID string `json:"condition_id"`
	State       string `json:"state"` // satisfied, unsatisfied, unknown
	Evidence    string `json:"evidence"`
	Gap         string `json:"gap,omitempty"`
}

type Review struct {
	Ready      bool              `json:"ready"`
	Conditions []ConditionReview `json:"conditions"`
	Summary    string            `json:"summary"`
	Identity   Identity          `json:"identity"`
}

type Disposition string

const (
	Continue      Disposition = "continue"
	Reassess      Disposition = "reassess"
	Recover       Disposition = "recover"
	OwnerBoundary Disposition = "owner_boundary"
)

type Evaluation struct {
	Disposition  Disposition `json:"disposition"`
	Advancement  string      `json:"advancement"` // baseline, advanced, unchanged, unknown
	Reason       string      `json:"reason"`
	ConditionIDs []string    `json:"condition_ids,omitempty"`
}

// Record belongs in the existing evidence ledger. An observation of an old
// candidate is history; it cannot establish the current product's readiness.
type Record struct {
	GoalID        string     `json:"goal_id"`
	GoalRevision  string     `json:"goal_revision"`
	ReadyRevision string     `json:"ready_revision"`
	Round         int        `json:"round"`
	PlanID        string     `json:"plan_id"`
	TaskIDs       []string   `json:"task_ids"`
	At            string     `json:"at"`
	Review        Review     `json:"review"`
	Evaluation    Evaluation `json:"evaluation"`
	Reassessment  string     `json:"reassessment,omitempty"`
}

// Facts must be collected by the host, not copied from a proposed decision.
// A missing executable binding is not evidence of an owner Stop. An explicit
// current Stop remains a hard boundary even when execution state is defective.
type Facts struct {
	ExecutionGoalID    string   `json:"execution_goal_id"`
	ExecutionAvailable bool     `json:"execution_available"`
	HardBoundaryGoalID string   `json:"hard_boundary_goal_id,omitempty"`
	HardBoundary       string   `json:"hard_boundary,omitempty"`
	MachineRecovery    bool     `json:"machine_recovery,omitempty"`
	ExecutionOutcomes  []string `json:"execution_outcomes,omitempty"`
	RecoveryEvents     []string `json:"recovery_events,omitempty"`
}

type Input struct {
	Contract           Contract `json:"accepted_contract"`
	Current            Identity `json:"current_product"`
	Facts              Facts    `json:"observed_facts"`
	History            []Record `json:"independent_reviews"`
	CurrentReview      *Record  `json:"current_review,omitempty"`
	PreviousHypotheses []string `json:"previous_hypotheses_not_facts,omitempty"`
	NextGoalID         string   `json:"proposed_next_goal_id"`
	NextAction         string   `json:"proposed_next_action"`
	OwnerDecision      string   `json:"proposed_owner_decision,omitempty"`
}

type Judge interface {
	Complete(context.Context, string) (string, error)
}

const MaxHistory = 5

func (c Contract) Validate() error {
	if strings.TrimSpace(c.GoalID) == "" || strings.TrimSpace(c.GoalRevision) == "" || strings.TrimSpace(c.ReadyRevision) == "" || strings.TrimSpace(c.Goal) == "" || len(c.Conditions) == 0 {
		return fmt.Errorf("accepted Goal/Ready identity and conditions required")
	}
	seen := map[string]bool{}
	for _, v := range c.Conditions {
		if strings.TrimSpace(v.ID) == "" || seen[v.ID] || strings.TrimSpace(v.Description) == "" {
			return fmt.Errorf("accepted conditions must have unique IDs and descriptions")
		}
		seen[v.ID] = true
	}
	return nil
}

func (i Identity) Valid() bool { return i.Repository != "" && i.Workspace != "" && i.Candidate != "" }

func ValidateReview(c Contract, r Review) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if !r.Identity.Valid() || strings.TrimSpace(r.Summary) == "" || len(r.Conditions) != len(c.Conditions) {
		return fmt.Errorf("product review requires provenance, summary and every accepted condition")
	}
	seen := map[string]bool{}
	for _, v := range c.Conditions {
		seen[v.ID] = false
	}
	ready := true
	for _, v := range r.Conditions {
		done, exists := seen[v.ConditionID]
		if !exists || done || strings.TrimSpace(v.Evidence) == "" {
			return fmt.Errorf("invalid or duplicate reviewed condition %q", v.ConditionID)
		}
		seen[v.ConditionID] = true
		switch v.State {
		case "satisfied":
			if strings.TrimSpace(v.Gap) != "" {
				return fmt.Errorf("satisfied condition has a gap")
			}
		case "unsatisfied", "unknown":
			ready = false
			if strings.TrimSpace(v.Gap) == "" {
				return fmt.Errorf("unproven condition requires a gap")
			}
		default:
			return fmt.Errorf("invalid condition state %q", v.State)
		}
	}
	if r.Ready != ready {
		return fmt.Errorf("readiness conflicts with condition evidence")
	}
	return nil
}

func sameContract(c Contract, r Record) bool {
	return r.GoalID == c.GoalID && r.GoalRevision == c.GoalRevision && r.ReadyRevision == c.ReadyRevision
}

// Evaluate runs deterministic provenance, authority and routing checks first.
// It asks a fresh independent judge only for semantic comparisons or decisions.
func Evaluate(ctx context.Context, judge Judge, in Input) (Evaluation, error) {
	result := func(d Disposition, a, reason string) (Evaluation, error) {
		return Evaluation{Disposition: d, Advancement: a, Reason: reason}, nil
	}
	if err := in.Contract.Validate(); err != nil {
		return Evaluation{}, err
	}
	if in.Facts.HardBoundary != "" && in.Facts.HardBoundaryGoalID == in.Contract.GoalID {
		return result(OwnerBoundary, "unknown", in.Facts.HardBoundary)
	}
	if !in.Facts.ExecutionAvailable || in.Facts.ExecutionGoalID != in.Contract.GoalID {
		return result(Recover, "unknown", "Accepted Goal and executable Goal disagree; recover the accepted binding without selecting historical work or requesting owner coordination.")
	}
	if in.NextGoalID != in.Contract.GoalID {
		return result(Recover, "unknown", "Proposed work belongs to another Goal; history does not replace the accepted current outcome.")
	}
	if in.Facts.MachineRecovery {
		return result(Recover, "unknown", "Execution/recovery is machine work within the accepted outcome, not an owner product decision.")
	}
	if !in.Current.Valid() {
		return result(Recover, "unknown", "Current product identity is unavailable; refresh evidence before evaluating convergence.")
	}
	if len(in.History) > MaxHistory {
		in.History = in.History[len(in.History)-MaxHistory:]
	}
	valid := []Record{}
	for _, r := range in.History {
		if !sameContract(in.Contract, r) {
			continue
		}
		if err := ValidateReview(in.Contract, r.Review); err != nil {
			return result(Recover, "unknown", "Retained review is invalid; refresh product evidence.")
		}
		if len(valid) > 0 && r.Round <= valid[len(valid)-1].Round {
			return Evaluation{}, fmt.Errorf("review rounds must be strictly increasing")
		}
		valid = append(valid, r)
	}
	in.History = valid
	if in.CurrentReview != nil {
		r := in.CurrentReview
		if !sameContract(in.Contract, *r) || r.Review.Identity != in.Current {
			return result(Recover, "unknown", "Review does not describe the current Goal/Ready and product identity; refresh it without counting progress or a stall.")
		}
		if err := ValidateReview(in.Contract, r.Review); err != nil {
			return Evaluation{}, err
		}
		if len(valid) > 0 {
			p := valid[len(valid)-1]
			if r.Round <= p.Round {
				return Evaluation{}, fmt.Errorf("current review does not follow retained review")
			}
			if p.Review.Identity.Repository != in.Current.Repository || p.Review.Identity.Workspace != in.Current.Workspace {
				return result(Recover, "unknown", "Review lineage changed repository/workspace; establish a fresh comparison baseline.")
			}
			// Closing a condition is deterministic progress only without regression.
			old := map[string]string{}
			for _, c := range p.Review.Conditions {
				old[c.ConditionID] = c.State
			}
			advanced, regressed := false, false
			for _, c := range r.Review.Conditions {
				advanced = advanced || (c.State == "satisfied" && old[c.ConditionID] != "satisfied")
				regressed = regressed || (c.State != "satisfied" && old[c.ConditionID] == "satisfied")
			}
			if advanced && !regressed && in.OwnerDecision == "" {
				return result(Continue, "advanced", "Independent current evidence satisfies an additional accepted condition without regression.")
			}
		} else if in.OwnerDecision == "" {
			return result(Continue, "baseline", "First valid independent product review for this accepted revision.")
		}
	}
	if judge == nil {
		return Evaluation{}, fmt.Errorf("independent outcome evaluator required for semantic judgment")
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return Evaluation{}, err
	}
	if len(raw) > 64000 {
		return Evaluation{}, fmt.Errorf("outcome evidence exceeds bounded evaluator input")
	}
	response, err := judge.Complete(ctx, `Evaluate the autonomous organization's trajectory against the immutable accepted Goal/Ready. Facts and independent product observations are authoritative inputs; previous strategies/diagnoses are hypotheses, not facts. Do not infer progress from activity, tasks, tests, commits, tools, tokens or claims. Equivalent failures may have different wording; the same unsatisfied condition may progress to a later observable journey point. Inspect concrete evidence. Distinguish wrong-target work, wrong-route persistence, stale evidence, coordination leaks and genuine owner boundaries. Engineering strategy, missing executable binding, retries and historical PR selection are machine work. Only a material destination, value or authority question can be owner_boundary. Do not change acceptance or grant authority. Return only JSON {"disposition":"continue|reassess|recover|owner_boundary","advancement":"advanced|unchanged|unknown","reason":"concrete evidence and rationale","condition_ids":["accepted IDs"]}. Do not count rounds; software applies the stall threshold.
`+string(raw))
	if err != nil {
		return Evaluation{}, err
	}
	var e Evaluation
	if err = json.Unmarshal([]byte(response), &e); err != nil {
		return Evaluation{}, fmt.Errorf("invalid outside evaluation: %w", err)
	}
	if strings.TrimSpace(e.Reason) == "" {
		return Evaluation{}, fmt.Errorf("outside evaluation requires evidence rationale")
	}
	switch e.Disposition {
	case Continue, Reassess, Recover, OwnerBoundary:
	default:
		return Evaluation{}, fmt.Errorf("unknown outside disposition")
	}
	switch e.Advancement {
	case "advanced", "unchanged", "unknown":
	default:
		return Evaluation{}, fmt.Errorf("unknown advancement judgment")
	}
	ids := map[string]bool{}
	for _, c := range in.Contract.Conditions {
		ids[c.ID] = true
	}
	for _, id := range e.ConditionIDs {
		if !ids[id] {
			return Evaluation{}, fmt.Errorf("evaluator changed accepted condition identity")
		}
	}
	if in.CurrentReview != nil && (e.Advancement == "advanced" || e.Advancement == "unchanged") && len(e.ConditionIDs) == 0 {
		return Evaluation{}, fmt.Errorf("semantic comparison requires condition evidence references")
	}
	// Three reviews / two unchanged transitions. A completed reassessment resets
	// this window; its subsequent effectiveness remains visible in the history.
	if e.Disposition == Continue && e.Advancement == "unchanged" && in.CurrentReview != nil && len(valid) > 0 {
		last := valid[len(valid)-1]
		if last.Evaluation.Advancement == "unchanged" && last.Reassessment == "" {
			e.Disposition = Reassess
			e.Reason = "Repeated independent reviews show no material advancement. " + e.Reason
		}
	}
	return e, nil
}

func Reassessment(ctx context.Context, judge Judge, in Input) (string, error) {
	if judge == nil {
		return "", fmt.Errorf("fresh independent reassessor required")
	}
	if len(in.History) > MaxHistory {
		in.History = in.History[len(in.History)-MaxHistory:]
	}
	// Exclude prior model explanations/diagnoses. Retain observations and factual
	// attempted work only. No implementation conversation is resumed.
	failedRoutes := 0
	for i := range in.History {
		if in.History[i].Evaluation.Advancement == "advanced" {
			failedRoutes = 0
		}
		if in.History[i].Reassessment != "" {
			failedRoutes++
		}
		in.History[i].Reassessment = ""
		in.History[i].Evaluation = Evaluation{}
	}
	if in.CurrentReview != nil {
		r := *in.CurrentReview
		r.Evaluation = Evaluation{}
		r.Reassessment = ""
		in.CurrentReview = &r
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return "", err
	}
	if len(raw) > 64000 {
		return "", fmt.Errorf("reassessment evidence exceeds bounded input")
	}
	prompt := `Independently determine why the accepted outcome is not converging. Treat previous diagnoses and implementation strategies as hypotheses rather than facts. Start from current product and repository evidence. Identify assumptions disproved by evidence and derive the smallest materially different route toward the remaining Ready conditions. Do not weaken Goal/Ready and do not escalate engineering strategy selection to the owner. Preserve completed valid work. Return concise factual findings, failed assumptions and a materially different route for the existing reviewed planner; identify concrete authority boundaries only if required by evidence. No conversation history is authoritative.
`
	if failedRoutes >= 2 {
		prompt += "Repeated reassessments did not establish progress. Investigate verification observing the wrong target, candidate/deployment mismatch, semantically equivalent plans, unavailable capabilities and contradictory accepted conditions before proposing another repair.\n"
	}
	answer, err := judge.Complete(ctx, prompt+string(raw))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(answer) == "" || len(answer) > 12000 {
		return "", fmt.Errorf("fresh reassessment must be nonempty and bounded")
	}
	return answer, nil
}
