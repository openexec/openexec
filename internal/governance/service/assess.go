package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/ai"
	"github.com/openexec/openexec/internal/governance/connectors/github"
)

// decisionAssessed is the audit token recorded when the AI evaluates a change's
// risk (file-level impact + operability). It puts the evaluation in the
// hash-chained decision trail so how the AI judged the risk is permanently
// tracked, not just how a human decided.
const decisionAssessed = "assessed"

// AssessChange runs the file-level impact and operability (rollback / DB
// migration / deploy risk) analyses for a change, stores the full reports, and
// records a hash-chained decision event summarizing the AI's risk evaluation —
// so every change carries its assessment in the audit trail even when it skipped
// deep triage (the lightweight lane). Requires a completer and repoContext (the
// file excerpts the model reads). Best-effort per analysis: one failing does not
// suppress the other; the audit event reflects whatever was produced.
func (s *Service) AssessChange(ctx context.Context, changeID, repoContext string) (*ai.ImpactReport, *ai.OperabilityReport, error) {
	if s.completer == nil {
		return nil, nil, errMissingCompleter
	}
	ch, err := s.getChange(ctx, changeID)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(repoContext) == "" {
		return nil, nil, fmt.Errorf("assessment needs repo context (files for the model to read); none provided")
	}
	intent := composeIntent(ch, "")

	impact, impErr := ai.AnalyzeImpact(ctx, s.completer, intent, repoContext)
	if impErr == nil && impact != nil {
		if raw, mErr := json.Marshal(impact); mErr == nil {
			_ = s.store.SetChangeImpact(ctx, ch.ID, string(raw))
		}
	}
	op, opErr := ai.AnalyzeOperability(ctx, s.completer, intent, repoContext)
	if opErr == nil && op != nil {
		if raw, mErr := json.Marshal(op); mErr == nil {
			_ = s.store.SetChangeOperability(ctx, ch.ID, string(raw))
		}
	}
	if impErr != nil && opErr != nil {
		return nil, nil, fmt.Errorf("assessment failed: impact: %v; operability: %v", impErr, opErr)
	}

	// Record the AI risk evaluation in the tamper-evident trail.
	ev := &governance.DecisionEvent{
		ID:              newID(),
		ReleaseID:       ch.ReleaseID,
		ChangeID:        ch.ID,
		ProposalVersion: ch.ProposalVersion,
		Actor:           "risk_assessor",
		ActorType:       governance.ActorTypeAI,
		Decision:        decisionAssessed,
		Comment:         assessmentSummary(impact, op),
	}
	if err := s.store.CreateDecisionEvent(ctx, ev); err != nil {
		return impact, op, fmt.Errorf("record assessment for change %q: %w", changeID, err)
	}
	return impact, op, nil
}

// PostPRAssessment renders the stored assessment for a change and posts it as a
// comment on the change's pull request, so a human reviewing the PR sees how the
// AI evaluated impact and operability. Requires a Runner and a recorded PR URL.
func (s *Service) PostPRAssessment(ctx context.Context, changeID string) error {
	if s.runner == nil {
		return errMissingRunner
	}
	ch, err := s.getChange(ctx, changeID)
	if err != nil {
		return err
	}
	repo, number, ok := repoFromPRURL(ch.PRURL)
	if !ok {
		return fmt.Errorf("change %s has no parseable PR URL (record the PR first)", changeID)
	}
	md, err := s.PRAssessmentMarkdown(ctx, changeID)
	if err != nil {
		return err
	}
	return github.PostPRComment(ctx, s.runner, repo, number, md)
}

// PRAssessmentMarkdown renders the change's stored impact + operability into a
// governance-assessment comment. Sections that were not produced are shown as
// "not assessed" rather than omitted, so a reviewer can tell the difference
// between "assessed as safe" and "never assessed".
func (s *Service) PRAssessmentMarkdown(ctx context.Context, changeID string) (string, error) {
	ch, err := s.getChange(ctx, changeID)
	if err != nil {
		return "", err
	}
	impact, _ := s.ChangeImpact(ctx, changeID)
	op, _ := s.ChangeOperability(ctx, changeID)
	return renderPRAssessment(ch, impact, op), nil
}

// assessmentSummary is the one-line risk digest stored on the decision event.
func assessmentSummary(impact *ai.ImpactReport, op *ai.OperabilityReport) string {
	files := 0
	if impact != nil {
		files = len(impact.Files)
	}
	if op == nil {
		return fmt.Sprintf("AI risk assessment: %d file(s) affected; operability not assessed", files)
	}
	return fmt.Sprintf("AI risk assessment: %d file(s) affected; rollback=%s, db_migration=%s, deploy_risk=%s",
		files, dash(op.RollbackSafe), dash(op.DBMigration), dash(op.DeployRisk))
}

func dash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "unknown"
	}
	return v
}

// renderPRAssessment builds the PR comment markdown from the stored reports.
func renderPRAssessment(ch *governance.ChangeRecord, impact *ai.ImpactReport, op *ai.OperabilityReport) string {
	var b strings.Builder
	lane := "standard"
	if ch.Light {
		lane = "lightweight (operator-approved, AI review waived)"
	}
	fmt.Fprintf(&b, "## 🔒 OpenExec governance assessment\n\n")
	fmt.Fprintf(&b, "**Change** `%s` · kind **%s** · risk **%s** · lane **%s**\n\n", ch.ID, dash(ch.Kind), dash(ch.Risk), lane)

	b.WriteString("### Impact — files affected\n\n")
	if impact == nil || len(impact.Files) == 0 {
		b.WriteString("_Not assessed._\n\n")
	} else {
		for _, f := range impact.Files {
			fmt.Fprintf(&b, "- `%s` (%s): %s\n", f.Path, dash(f.Action), f.Reason)
		}
		if strings.TrimSpace(impact.Notes) != "" {
			fmt.Fprintf(&b, "\n> %s\n", impact.Notes)
		}
		b.WriteString("\n")
	}

	b.WriteString("### Operability\n\n")
	if op == nil {
		b.WriteString("_Not assessed._\n\n")
	} else {
		fmt.Fprintf(&b, "- **Rollback safe:** %s\n", dash(op.RollbackSafe))
		fmt.Fprintf(&b, "- **DB migration:** %s\n", dash(op.DBMigration))
		fmt.Fprintf(&b, "- **Deploy risk:** %s\n", dash(op.DeployRisk))
		if len(op.Mitigations) > 0 {
			fmt.Fprintf(&b, "- **Mitigations:** %s\n", strings.Join(op.Mitigations, "; "))
		}
		if len(op.Monitoring) > 0 {
			fmt.Fprintf(&b, "- **Monitoring:** %s\n", strings.Join(op.Monitoring, "; "))
		}
		if strings.TrimSpace(op.Notes) != "" {
			fmt.Fprintf(&b, "\n> %s\n", op.Notes)
		}
		b.WriteString("\n")
	}

	b.WriteString("_This AI risk evaluation is recorded in the OpenExec governance audit trail._\n")
	return b.String()
}
