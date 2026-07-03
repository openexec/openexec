package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/openexec/openexec/internal/governance"
)

// Triage runs the planner AI over a change record and applies the result.
//
// It builds the triage prompt, calls the completer, parses the PlannerOutput,
// and writes Summary/Kind/Risk/AcceptanceCriteria/VerificationPlan/Plan onto the
// change record. It then enforces the Phase 6 versioning invariants:
// ProposalVersion is incremented (every material plan revision is a new version)
// and ApprovedVersion is cleared to 0 (a changed proposal makes any prior
// approval stale). Status moves to plan_ready, the record is persisted, and a
// DecisionEvent{Decision: proposed, ActorType: ai} is recorded at the new
// version. The applied PlannerOutput is returned.
func Triage(ctx context.Context, completer Completer, store governance.Store, ch *governance.ChangeRecord, repoContext, actor string) (*PlannerOutput, error) {
	if completer == nil || store == nil || ch == nil {
		return nil, fmt.Errorf("governance/ai: triage requires completer, store, and change record")
	}

	raw, err := completer.Complete(ctx, TriagePrompt(ch, repoContext))
	if err != nil {
		return nil, fmt.Errorf("governance/ai: triage completion failed: %w", err)
	}
	out, err := ParsePlannerOutput(raw)
	if err != nil {
		return nil, err
	}

	ch.Summary = out.Summary
	if out.Kind != "" {
		ch.Kind = out.Kind
	}
	if out.Risk != "" {
		ch.Risk = out.Risk
	}
	ch.AcceptanceCriteria = strings.Join(out.AcceptanceCriteria, "\n")
	ch.VerificationPlan = strings.Join(out.VerificationPlan, "\n")
	ch.Plan = out.ImplementationNotes

	// Versioning invariants: a new proposal supersedes any prior approval.
	ch.ProposalVersion++
	ch.ApprovedVersion = 0
	ch.Status = governance.ChangeStatusPlanReady

	if err := store.UpdateChangeRecord(ctx, ch); err != nil {
		return nil, fmt.Errorf("governance/ai: persist triaged change: %w", err)
	}

	ev := &governance.DecisionEvent{
		ID:              uuid.New().String(),
		ReleaseID:       ch.ReleaseID,
		ChangeID:        ch.ID,
		ProposalVersion: ch.ProposalVersion,
		Actor:           actor,
		ActorType:       governance.ActorTypeAI,
		Decision:        governance.DecisionProposed,
		Comment:         out.Summary,
	}
	if err := store.CreateDecisionEvent(ctx, ev); err != nil {
		return nil, fmt.Errorf("governance/ai: record proposed decision: %w", err)
	}

	return out, nil
}

// ReviewPlan runs the reviewer AI over the change record's current plan and
// records its assessment as a DecisionEvent — without ever granting approval.
//
// The reviewer is a separate actor: its "approve" verdict maps only to
// recommended_approval (see MapReviewDecision). This function MUST NOT set
// ApprovedVersion or otherwise change release approval; that authority belongs to
// an authorized human alone. If the reviewer requests changes, the change Status
// moves to changes_requested. The authority parameter gates recommendation: a
// reviewer lacking the recommend_approval permission cannot emit a
// recommended_approval decision (it is downgraded to a plain review note),
// preventing an under-privileged actor from manufacturing a recommendation.
func ReviewPlan(ctx context.Context, completer Completer, store governance.Store, ch *governance.ChangeRecord, authority *governance.ReviewAuthority, actor string) (*ReviewerOutput, error) {
	if completer == nil || store == nil || ch == nil {
		return nil, fmt.Errorf("governance/ai: review requires completer, store, and change record")
	}

	plan := planFromChange(ch)
	raw, err := completer.Complete(ctx, ReviewPrompt(ch, plan))
	if err != nil {
		return nil, fmt.Errorf("governance/ai: review completion failed: %w", err)
	}
	out, err := ParseReviewerOutput(raw)
	if err != nil {
		return nil, err
	}

	decision := MapReviewDecision(out.Decision)

	// Permission guards: a reviewer may only emit a decision it is authorized
	// for. Neither path can ever grant approval.
	//  - recommend_approval requires the recommend_approval permission;
	//  - changes_requested requires the request_changes permission.
	// An unauthorized reviewer is downgraded to a neutral review note rather
	// than being able to recommend or to disrupt the flow by demanding changes.
	if decision == governance.DecisionRecommendedApproval && !authorityCan(authority, governance.PermRecommendApproval) {
		decision = governance.DecisionReviewed
	}
	if decision == governance.DecisionChangesRequested && !authorityCan(authority, governance.PermRequestChanges) {
		decision = governance.DecisionReviewed
	}

	ev := &governance.DecisionEvent{
		ID:              uuid.New().String(),
		ReleaseID:       ch.ReleaseID,
		ChangeID:        ch.ID,
		ProposalVersion: ch.ProposalVersion,
		Actor:           actor,
		ActorType:       governance.ActorTypeAI,
		Decision:        decision,
		Comment:         reviewComment(out),
	}
	if err := store.CreateDecisionEvent(ctx, ev); err != nil {
		return nil, fmt.Errorf("governance/ai: record review decision: %w", err)
	}

	// Move to changes_requested only when the (permission-checked) decision is
	// changes_requested — an unauthorized reviewer downgraded above cannot move
	// the change's status.
	if decision == governance.DecisionChangesRequested {
		ch.Status = governance.ChangeStatusChangesRequested
		if err := store.UpdateChangeRecord(ctx, ch); err != nil {
			return nil, fmt.Errorf("governance/ai: persist changes-requested status: %w", err)
		}
	}

	return out, nil
}

// MapReviewDecision maps a reviewer output decision token to the governance
// Decision constant recorded in state. The crux of role separation lives here:
// "approve" becomes recommended_approval — NEVER approved — because the reviewer
// AI may recommend but cannot grant approval. Unknown tokens map to the neutral
// "reviewed" so an unrecognized verdict never escalates into a recommendation.
func MapReviewDecision(reviewerDecision string) string {
	switch reviewerDecision {
	case ReviewerApprove:
		return governance.DecisionRecommendedApproval
	case ReviewerRequestChanges:
		return governance.DecisionChangesRequested
	case ReviewerHumanRequired:
		return governance.DecisionReviewed
	default:
		return governance.DecisionReviewed
	}
}

// planFromChange reconstructs a PlannerOutput from the stored plan fields so the
// reviewer prompt can be built from the persisted record (the planner's output
// is flattened into the change record by Triage).
func planFromChange(ch *governance.ChangeRecord) *PlannerOutput {
	return &PlannerOutput{
		Summary:             ch.Summary,
		Kind:                ch.Kind,
		Risk:                ch.Risk,
		AcceptanceCriteria:  splitLines(ch.AcceptanceCriteria),
		VerificationPlan:    splitLines(ch.VerificationPlan),
		ImplementationNotes: ch.Plan,
	}
}

func authorityCan(authority *governance.ReviewAuthority, perm string) bool {
	if authority == nil {
		return false
	}
	for _, p := range authority.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(ln); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// reviewComment renders a human-readable summary of the reviewer output for the
// DecisionEvent comment field. For human_required it leads with an explicit note
// so the handoff reason is visible in decision history.
func reviewComment(out *ReviewerOutput) string {
	var sb strings.Builder
	if out.Decision == ReviewerHumanRequired {
		sb.WriteString("Human review required.\n")
	}
	appendBullets(&sb, "Concerns", out.Concerns)
	appendBullets(&sb, "Missing acceptance criteria", out.MissingAcceptanceCriteria)
	appendBullets(&sb, "Missing tests", out.MissingTests)
	appendBullets(&sb, "Risk comments", out.RiskComments)
	if strings.TrimSpace(out.RecommendedPolicy) != "" {
		sb.WriteString("Recommended policy: " + out.RecommendedPolicy + "\n")
	}
	return strings.TrimSpace(sb.String())
}

func appendBullets(sb *strings.Builder, label string, items []string) {
	if len(items) == 0 {
		return
	}
	sb.WriteString(label + ":\n")
	for _, it := range items {
		if t := strings.TrimSpace(it); t != "" {
			sb.WriteString("- " + t + "\n")
		}
	}
}
