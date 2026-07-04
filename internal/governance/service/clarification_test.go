package service

import (
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/ai"
)

func TestRenderClarificationComment(t *testing.T) {
	ch := &governance.ChangeRecord{ID: "CHANGE-1", Status: governance.ChangeStatusChangesRequested}
	out := &ai.ReviewerOutput{
		Decision:                  ai.ReviewerRequestChanges,
		Concerns:                  []string{"Placeholder privacy policy with a newsletter form is a GDPR risk"},
		MissingAcceptanceCriteria: []string{"Approved TikTok/Instagram profile URLs"},
		MissingTests:              []string{"A render test asserting the footer links"},
		RecommendedPolicy:         "Require human approval of legal-page content before merge",
	}

	body := renderClarificationComment(ch, out)

	for _, want := range []string{
		"CHANGE-1",
		"needs your input",
		"GDPR risk",
		"Approved TikTok/Instagram profile URLs",
		"A render test asserting the footer links",
		"Require human approval",
		"/openexec revise",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("clarification comment missing %q\n---\n%s", want, body)
		}
	}
}

func TestRenderClarificationComment_OmitsEmptySections(t *testing.T) {
	ch := &governance.ChangeRecord{ID: "CHANGE-2", Status: governance.ChangeStatusChangesRequested}
	out := &ai.ReviewerOutput{
		Decision: ai.ReviewerRequestChanges,
		Concerns: []string{"One concern"},
	}
	body := renderClarificationComment(ch, out)
	if strings.Contains(body, "Missing acceptance criteria") || strings.Contains(body, "Missing tests") {
		t.Errorf("empty sections should be omitted:\n%s", body)
	}
}
