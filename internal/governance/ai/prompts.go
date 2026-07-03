package ai

import (
	"fmt"
	"strings"

	"github.com/openexec/openexec/internal/governance"
)

// TriagePrompt builds the planner prompt. The model is instructed to return a
// PlannerOutput as a single YAML object. repoContext is optional caller-supplied
// context (e.g. affected files, README excerpts) and may be empty.
func TriagePrompt(ch *governance.ChangeRecord, repoContext string) string {
	var sb strings.Builder
	sb.WriteString("You are the PLANNER for an AI-governed software release.\n")
	sb.WriteString("Triage the change below into an actionable, reviewable plan.\n")
	sb.WriteString("You PROPOSE only. You do not approve work and you do not implement it.\n\n")

	sb.WriteString("## Change\n")
	if ch != nil {
		writeField(&sb, "ID", ch.ID)
		writeField(&sb, "Title", ch.Title)
		writeField(&sb, "Source", ch.SourceURL)
		writeField(&sb, "Existing kind", ch.Kind)
		writeField(&sb, "Existing risk", ch.Risk)
		if strings.TrimSpace(ch.RawText) != "" {
			sb.WriteString("\nDescription:\n")
			sb.WriteString(ch.RawText)
			sb.WriteString("\n")
		}
	}
	if strings.TrimSpace(repoContext) != "" {
		sb.WriteString("\n## Repository context\n")
		sb.WriteString(repoContext)
		sb.WriteString("\n")
	}

	sb.WriteString("\n## Required output\n")
	sb.WriteString("Return ONE YAML object and nothing else. Do not wrap prose around it.\n")
	sb.WriteString("Use exactly these keys:\n\n")
	sb.WriteString("```yaml\n")
	sb.WriteString("summary: \"\"                 # one-line summary of the change\n")
	sb.WriteString("kind: bug | feature | docs | ops | support_question | security | reliability\n")
	sb.WriteString("risk: low | medium | high | critical\n")
	sb.WriteString("affected_projects: []        # project ids/names\n")
	sb.WriteString("affected_areas: []           # subsystems / modules\n")
	sb.WriteString("acceptance_criteria: []      # observable, testable conditions for done\n")
	sb.WriteString("verification_plan: []        # concrete steps/commands to verify\n")
	sb.WriteString("implementation_notes: \"\"     # how to implement; constraints; scope\n")
	sb.WriteString("open_questions: []           # blockers needing a human answer\n")
	sb.WriteString("recommended_release: \"\"      # target release id, or empty\n")
	sb.WriteString("```\n")
	return sb.String()
}

// ReviewPrompt builds the reviewer prompt over a previously produced plan. The
// model is instructed to return a ReviewerOutput as a single YAML object and is
// told explicitly that it may recommend but never approve.
func ReviewPrompt(ch *governance.ChangeRecord, plan *PlannerOutput) string {
	var sb strings.Builder
	sb.WriteString("You are the REVIEWER for an AI-governed software release.\n")
	sb.WriteString("You are a DIFFERENT actor from the planner. Critically assess the plan.\n")
	sb.WriteString("You may request changes or recommend approval, but you CANNOT approve\n")
	sb.WriteString("and you CANNOT implement. Approval is reserved for an authorized human.\n\n")

	sb.WriteString("## Change\n")
	if ch != nil {
		writeField(&sb, "ID", ch.ID)
		writeField(&sb, "Title", ch.Title)
		writeField(&sb, "Risk", ch.Risk)
	}

	sb.WriteString("\n## Proposed plan\n")
	if plan != nil {
		writeField(&sb, "Summary", plan.Summary)
		writeField(&sb, "Kind", plan.Kind)
		writeField(&sb, "Risk", plan.Risk)
		writeList(&sb, "Acceptance criteria", plan.AcceptanceCriteria)
		writeList(&sb, "Verification plan", plan.VerificationPlan)
		writeField(&sb, "Implementation notes", plan.ImplementationNotes)
		writeList(&sb, "Open questions", plan.OpenQuestions)
	}

	sb.WriteString("\n## Required output\n")
	sb.WriteString("Return ONE YAML object and nothing else. Use exactly these keys:\n\n")
	sb.WriteString("```yaml\n")
	sb.WriteString("decision: approve | request_changes | human_required\n")
	sb.WriteString("concerns: []                      # specific problems with the plan\n")
	sb.WriteString("missing_acceptance_criteria: []   # criteria that should be added\n")
	sb.WriteString("missing_tests: []                 # tests/verification that should be added\n")
	sb.WriteString("risk_comments: []                 # risk observations\n")
	sb.WriteString("recommended_policy: \"\"            # suggested approval policy, or empty\n")
	sb.WriteString("```\n")
	sb.WriteString("Choose `approve` only to RECOMMEND approval; a human still decides.\n")
	sb.WriteString("Choose `human_required` when the change exceeds what an AI should sign off.\n")
	return sb.String()
}

func writeField(sb *strings.Builder, label, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	sb.WriteString(fmt.Sprintf("- %s: %s\n", label, value))
}

func writeList(sb *strings.Builder, label string, items []string) {
	if len(items) == 0 {
		return
	}
	sb.WriteString(fmt.Sprintf("- %s:\n", label))
	for _, it := range items {
		if strings.TrimSpace(it) == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("  - %s\n", it))
	}
}
