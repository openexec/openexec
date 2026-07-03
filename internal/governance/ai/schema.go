// Package ai implements the planner (triage) and reviewer AI actors for release
// governance. The two roles are deliberately separate actors in state even when
// they share an underlying model: the planner proposes a plan and the reviewer
// may only recommend — never approve. The actual LLM call is injected via the
// Completer interface so the package is fully unit-testable without a provider.
package ai

import "context"

// Completer is the minimal LLM call this package depends on. The CLI/MCP layers
// pass a concrete provider-backed implementation; tests pass a fake.
type Completer interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// PlannerOutput is the structured triage result produced by the planner AI.
// It mirrors the Phase 6 planner schema verbatim.
type PlannerOutput struct {
	Summary             string   `yaml:"summary" json:"summary"`
	Kind                string   `yaml:"kind" json:"kind"`
	Risk                string   `yaml:"risk" json:"risk"`
	AffectedProjects    []string `yaml:"affected_projects" json:"affected_projects"`
	AffectedAreas       []string `yaml:"affected_areas" json:"affected_areas"`
	AcceptanceCriteria  []string `yaml:"acceptance_criteria" json:"acceptance_criteria"`
	VerificationPlan    []string `yaml:"verification_plan" json:"verification_plan"`
	ImplementationNotes string   `yaml:"implementation_notes" json:"implementation_notes"`
	OpenQuestions       []string `yaml:"open_questions" json:"open_questions"`
	RecommendedRelease  string   `yaml:"recommended_release" json:"recommended_release"`
}

// ReviewerOutput is the structured review result produced by the reviewer AI.
// It mirrors the Phase 6 reviewer schema verbatim. Decision is one of
// approve | request_changes | human_required — but "approve" only ever maps to
// a recommendation (see MapReviewDecision); the reviewer AI never approves.
type ReviewerOutput struct {
	Decision                  string   `yaml:"decision" json:"decision"`
	Concerns                  []string `yaml:"concerns" json:"concerns"`
	MissingAcceptanceCriteria []string `yaml:"missing_acceptance_criteria" json:"missing_acceptance_criteria"`
	MissingTests              []string `yaml:"missing_tests" json:"missing_tests"`
	RiskComments              []string `yaml:"risk_comments" json:"risk_comments"`
	RecommendedPolicy         string   `yaml:"recommended_policy" json:"recommended_policy"`
}

// Reviewer decision tokens accepted from the model (the reviewer-facing
// vocabulary, distinct from the governance Decision* constants in state).
const (
	ReviewerApprove        = "approve"
	ReviewerRequestChanges = "request_changes"
	ReviewerHumanRequired  = "human_required"
)
