// Package comms provides pure, deterministic communication generators for
// release governance. Each generator takes already-loaded governance structs
// (from internal/governance) and returns markdown. There are NO database
// reads, NO AI calls, and NO network access here — these functions format
// real, stored records.
//
// This matches the implementation plan's invariant for Phase 8: "Handoff is
// generated from STORED records, not only from AI claims." The markdown house
// style mirrors internal/release/changelog.go (level-1 title, level-2 section
// headings, bold field labels, "ID: Title" item headers, [x]/[-]/[ ] status
// markers, and a generated-by footer).
//
// All output is deterministic: collections are sorted by a stable key (change
// ID) before rendering, so map iteration order never leaks into the result.
package comms

import (
	"fmt"
	"sort"
	"strings"

	"github.com/openexec/openexec/internal/governance"
)

// GenerateExecutorBrief renders the Phase 7 executor brief: the self-contained
// instructions an executor (Codex/Claude) needs to implement one approved
// change. It deliberately includes only the scope, acceptance criteria, and
// verification plan that were stored on the approved change record, plus the
// reporting contract the executor must satisfy.
func GenerateExecutorBrief(rel *governance.GovernanceRelease, ch *governance.ChangeRecord, repoPath string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Executor Brief: %s\n\n", valueOr(changeID(ch), "(unknown change)")))

	// Identity block.
	sb.WriteString(fmt.Sprintf("**Change:** %s\n", valueOr(changeID(ch), "-")))
	if ch != nil && ch.Title != "" {
		sb.WriteString(fmt.Sprintf("**Title:** %s\n", ch.Title))
	}
	sb.WriteString(fmt.Sprintf("**Release:** %s\n", releaseLabel(rel)))
	sb.WriteString(fmt.Sprintf("**Repo:** %s\n", valueOr(repoPath, "-")))
	if ch != nil {
		sb.WriteString(fmt.Sprintf("**Risk:** %s\n", valueOr(ch.Risk, "unspecified")))
		if ch.Branch != "" {
			sb.WriteString(fmt.Sprintf("**Branch:** `%s`\n", ch.Branch))
		}
		if ch.SourceURL != "" {
			sb.WriteString(fmt.Sprintf("**Source:** %s\n", ch.SourceURL))
		}
	}

	// Allowed scope: what the executor may change, with release-level guardrails.
	sb.WriteString("\n## Allowed scope\n\n")
	if ch != nil && ch.Summary != "" {
		sb.WriteString(ch.Summary + "\n")
	} else {
		sb.WriteString("_No scope summary recorded._\n")
	}
	if rel != nil && len(rel.OutOfScope) > 0 {
		sb.WriteString("\n**Out of scope (do not touch):**\n")
		for _, item := range rel.OutOfScope {
			if s := strings.TrimSpace(item); s != "" {
				sb.WriteString(fmt.Sprintf("- %s\n", s))
			}
		}
	}

	// Acceptance criteria (stored single-field, may be multi-line).
	sb.WriteString("\n## Acceptance criteria\n\n")
	writeBulletField(&sb, fieldOr(ch, func(c *governance.ChangeRecord) string { return c.AcceptanceCriteria }), "_No acceptance criteria recorded._")

	// Verification plan.
	sb.WriteString("\n## Verification\n\n")
	writeBulletField(&sb, fieldOr(ch, func(c *governance.ChangeRecord) string { return c.VerificationPlan }), "_No verification plan recorded._")

	// Required reporting contract (fixed — what the executor must return).
	sb.WriteString("\n## Required reporting\n\n")
	sb.WriteString("- PR URL\n")
	sb.WriteString("- Tests run (commands and results)\n")
	sb.WriteString("- Known risks\n")

	writeFooter(&sb)
	return sb.String()
}

// GenerateTesterHandoff renders the Phase 8 tester handoff. Per the plan it
// MUST include: release/version/environment, included changes, acceptance
// criteria, exact verification steps, known risks, links to PRs/issues, and
// what is out of scope. Evidence is keyed by change ID.
func GenerateTesterHandoff(rel *governance.GovernanceRelease, changes []*governance.ChangeRecord, evidence map[string][]*governance.Evidence, env, version string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Tester Handoff: %s\n\n", releaseLabel(rel)))
	sb.WriteString(fmt.Sprintf("**Release:** %s\n", releaseLabel(rel)))
	sb.WriteString(fmt.Sprintf("**Version:** %s\n", valueOr(version, "(unversioned)")))
	sb.WriteString(fmt.Sprintf("**Environment:** %s\n", valueOr(env, "(unspecified)")))
	if rel != nil && rel.Goal != "" {
		sb.WriteString(fmt.Sprintf("**Goal:** %s\n", rel.Goal))
	}

	sorted := sortedChanges(changes)

	// Included changes with their per-change detail.
	sb.WriteString("\n## Included changes\n\n")
	if len(sorted) == 0 {
		sb.WriteString("_No changes attached to this release._\n")
	}
	for _, ch := range sorted {
		sb.WriteString(fmt.Sprintf("### %s: %s\n\n", valueOr(changeID(ch), "-"), valueOr(ch.Title, "(untitled)")))
		sb.WriteString(fmt.Sprintf("%s **Status:** %s", statusMarker(ch.Status), valueOr(ch.Status, "-")))
		if ch.Risk != "" {
			sb.WriteString(fmt.Sprintf(" | **Risk:** %s", ch.Risk))
		}
		sb.WriteString("\n\n")

		if ch.Summary != "" {
			sb.WriteString(ch.Summary + "\n\n")
		}

		// Links to PRs/issues.
		var links []string
		if ch.PRURL != "" {
			links = append(links, fmt.Sprintf("[PR](%s)", ch.PRURL))
		}
		if ch.SourceURL != "" {
			links = append(links, fmt.Sprintf("[Issue](%s)", ch.SourceURL))
		}
		if len(links) > 0 {
			sb.WriteString(fmt.Sprintf("**Links:** %s\n\n", strings.Join(links, " · ")))
		}

		// Acceptance criteria.
		sb.WriteString("**Acceptance criteria:**\n")
		writeBulletField(&sb, ch.AcceptanceCriteria, "_None recorded._")

		// Exact verification steps.
		sb.WriteString("\n**Verification steps:**\n")
		writeBulletField(&sb, ch.VerificationPlan, "_None recorded._")

		// Evidence already recorded (helps testers avoid re-running settled checks).
		if ev := evidence[changeID(ch)]; len(ev) > 0 {
			sb.WriteString("\n**Recorded evidence:**\n")
			for _, e := range sortedEvidence(ev) {
				line := fmt.Sprintf("- [%s/%s] %s", valueOr(e.Kind, "?"), valueOr(e.Source, "?"), valueOr(e.Summary, "(no summary)"))
				if e.URL != "" {
					line += fmt.Sprintf(" (%s)", e.URL)
				}
				sb.WriteString(line + "\n")
			}
		}
		sb.WriteString("\n")
	}

	// Known risks: aggregate non-low change risks.
	sb.WriteString("## Known risks\n\n")
	wroteRisk := false
	if rel != nil && rel.Risk != "" && rel.Risk != governance.RiskLow {
		sb.WriteString(fmt.Sprintf("- Release risk tier: **%s**\n", rel.Risk))
		wroteRisk = true
	}
	for _, ch := range sorted {
		if ch.Risk == governance.RiskHigh || ch.Risk == governance.RiskCritical || ch.Risk == governance.RiskMedium {
			sb.WriteString(fmt.Sprintf("- %s (%s): %s risk\n", valueOr(changeID(ch), "-"), valueOr(ch.Title, "(untitled)"), ch.Risk))
			wroteRisk = true
		}
	}
	if !wroteRisk {
		sb.WriteString("_No elevated risks recorded._\n")
	}

	// Out of scope — explicit boundaries for the tester.
	sb.WriteString("\n## Out of scope\n\n")
	if rel != nil && len(rel.OutOfScope) > 0 {
		for _, item := range rel.OutOfScope {
			if s := strings.TrimSpace(item); s != "" {
				sb.WriteString(fmt.Sprintf("- %s\n", s))
			}
		}
	} else {
		sb.WriteString("_Nothing explicitly marked out of scope._\n")
	}

	writeFooter(&sb)
	return sb.String()
}

// GeneratePMSummary renders a PM status update grouped by change status, with a
// blockers section. It surfaces the full release pipeline (planned → approved →
// implementing → ready-for-test → deployed/done) so a PM can see progress at a
// glance.
func GeneratePMSummary(rel *governance.GovernanceRelease, changes []*governance.ChangeRecord) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# PM Summary: %s\n\n", releaseLabel(rel)))
	if rel != nil {
		sb.WriteString(fmt.Sprintf("**Status:** %s\n", valueOr(rel.Status, "-")))
		if rel.Owner != "" {
			sb.WriteString(fmt.Sprintf("**Owner:** %s\n", rel.Owner))
		}
		if rel.Risk != "" {
			sb.WriteString(fmt.Sprintf("**Risk:** %s\n", rel.Risk))
		}
		if rel.Goal != "" {
			sb.WriteString(fmt.Sprintf("**Goal:** %s\n", rel.Goal))
		}
	}
	sb.WriteString(fmt.Sprintf("**Total items:** %d\n", len(changes)))

	sorted := sortedChanges(changes)

	// Group by status, rendered in pipeline order.
	byStatus := make(map[string][]*governance.ChangeRecord)
	for _, ch := range sorted {
		byStatus[ch.Status] = append(byStatus[ch.Status], ch)
	}

	sb.WriteString("\n## By status\n\n")
	rendered := make(map[string]bool)
	for _, st := range pmStatusOrder {
		group := byStatus[st]
		if len(group) == 0 {
			continue
		}
		rendered[st] = true
		sb.WriteString(fmt.Sprintf("### %s (%d)\n\n", pmStatusLabel(st), len(group)))
		for _, ch := range group {
			sb.WriteString(fmt.Sprintf("- %s %s: %s\n", statusMarker(ch.Status), valueOr(changeID(ch), "-"), valueOr(ch.Title, "(untitled)")))
		}
		sb.WriteString("\n")
	}
	// Any statuses not in the known pipeline order (forward-compat).
	var unknown []string
	for st := range byStatus {
		if !rendered[st] {
			unknown = append(unknown, st)
		}
	}
	sort.Strings(unknown)
	for _, st := range unknown {
		group := byStatus[st]
		sb.WriteString(fmt.Sprintf("### %s (%d)\n\n", pmStatusLabel(st), len(group)))
		for _, ch := range group {
			sb.WriteString(fmt.Sprintf("- %s %s: %s\n", statusMarker(ch.Status), valueOr(changeID(ch), "-"), valueOr(ch.Title, "(untitled)")))
		}
		sb.WriteString("\n")
	}

	// Blockers: anything blocked or needing changes.
	sb.WriteString("## Blockers\n\n")
	wroteBlocker := false
	for _, ch := range sorted {
		if ch.Status == governance.ChangeStatusBlocked || ch.Status == governance.ChangeStatusChangesRequested {
			reason := "blocked"
			if ch.Status == governance.ChangeStatusChangesRequested {
				reason = "changes requested"
			}
			sb.WriteString(fmt.Sprintf("- %s %s: %s (%s)\n", statusMarker(ch.Status), valueOr(changeID(ch), "-"), valueOr(ch.Title, "(untitled)"), reason))
			wroteBlocker = true
		}
	}
	if rel != nil && rel.Status == governance.ReleaseStatusBlocked {
		sb.WriteString("- Release is in **blocked** status\n")
		wroteBlocker = true
	}
	if !wroteBlocker {
		sb.WriteString("_No blockers._\n")
	}

	writeFooter(&sb)
	return sb.String()
}

// GenerateCustomerSummary renders a customer-SAFE release note.
//
// SAFETY RULE (conservative allow-list, not a block-list): a change is only
// included when ALL hold:
//   - its status is done (shipped, not in-flight work), AND
//   - its kind is customer-facing (feature, bug, or docs) — internal kinds
//     (security, ops, reliability, support_question) are excluded entirely, AND
//   - it has a non-empty Summary (the curated, user-facing description).
//
// Only the change Summary is ever emitted. Internal fields — Plan,
// Branch, PRURL, SourceURL, VerificationPlan, AcceptanceCriteria, Risk, RawText
// — are NEVER rendered, because they can leak implementation detail, attack
// surface, or unreleased context. When in doubt, a change is omitted.
func GenerateCustomerSummary(rel *governance.GovernanceRelease, changes []*governance.ChangeRecord) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s\n\n", valueOr(releaseName(rel), "Release Notes")))
	sb.WriteString("Here's what changed in this release.\n")

	sorted := sortedChanges(changes)

	var features, fixes, other []*governance.ChangeRecord
	for _, ch := range sorted {
		if !customerSafe(ch) {
			continue
		}
		switch ch.Kind {
		case governance.KindFeature:
			features = append(features, ch)
		case governance.KindBug:
			fixes = append(fixes, ch)
		default: // docs (and any other kind that passed customerSafe)
			other = append(other, ch)
		}
	}

	writeCustomerSection(&sb, "What's New", features)
	writeCustomerSection(&sb, "Fixes", fixes)
	writeCustomerSection(&sb, "Improvements", other)

	if len(features)+len(fixes)+len(other) == 0 {
		sb.WriteString("\n_No customer-facing changes to report yet._\n")
	}

	writeFooter(&sb)
	return sb.String()
}

// --- helpers ---------------------------------------------------------------

// pmStatusOrder is the pipeline order used to render the PM summary.
var pmStatusOrder = []string{
	governance.ChangeStatusCandidate,
	governance.ChangeStatusPlanned,
	governance.ChangeStatusPlanReady,
	governance.ChangeStatusChangesRequested,
	governance.ChangeStatusApprovedForAI,
	governance.ChangeStatusImplementing,
	governance.ChangeStatusPROpen,
	governance.ChangeStatusReadyForTest,
	governance.ChangeStatusDone,
	governance.ChangeStatusDeferred,
	governance.ChangeStatusRejected,
	governance.ChangeStatusBlocked,
}

func pmStatusLabel(status string) string {
	switch status {
	case governance.ChangeStatusCandidate:
		return "Candidate"
	case governance.ChangeStatusPlanned:
		return "Planned"
	case governance.ChangeStatusPlanReady:
		return "Plan ready"
	case governance.ChangeStatusChangesRequested:
		return "Changes requested"
	case governance.ChangeStatusApprovedForAI:
		return "Approved"
	case governance.ChangeStatusImplementing:
		return "Implementing"
	case governance.ChangeStatusPROpen:
		return "PR open"
	case governance.ChangeStatusReadyForTest:
		return "Ready for test"
	case governance.ChangeStatusDone:
		return "Done"
	case governance.ChangeStatusDeferred:
		return "Deferred"
	case governance.ChangeStatusRejected:
		return "Rejected"
	case governance.ChangeStatusBlocked:
		return "Blocked"
	case "":
		return "Unknown"
	default:
		return status
	}
}

// customerSafe applies the conservative allow-list for customer summaries.
func customerSafe(ch *governance.ChangeRecord) bool {
	if ch == nil {
		return false
	}
	if ch.Status != governance.ChangeStatusDone {
		return false
	}
	switch ch.Kind {
	case governance.KindFeature, governance.KindBug, governance.KindDocs:
		// customer-facing kinds only
	default:
		return false
	}
	return strings.TrimSpace(ch.Summary) != ""
}

func writeCustomerSection(sb *strings.Builder, heading string, changes []*governance.ChangeRecord) {
	if len(changes) == 0 {
		return
	}
	sb.WriteString(fmt.Sprintf("\n## %s\n\n", heading))
	for _, ch := range changes {
		// Summary only — never internal fields.
		sb.WriteString(fmt.Sprintf("- %s\n", strings.TrimSpace(ch.Summary)))
	}
}

// sortedChanges returns a copy sorted by change ID for deterministic output.
func sortedChanges(changes []*governance.ChangeRecord) []*governance.ChangeRecord {
	out := make([]*governance.ChangeRecord, 0, len(changes))
	for _, ch := range changes {
		if ch != nil {
			out = append(out, ch)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// sortedEvidence returns a copy sorted by (Kind, Source, ID) for determinism.
func sortedEvidence(ev []*governance.Evidence) []*governance.Evidence {
	out := make([]*governance.Evidence, 0, len(ev))
	for _, e := range ev {
		if e != nil {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// writeBulletField renders a stored multi-line text field as a bullet list.
// Lines are split on "\n"; existing "- " / "* " markers are normalized away.
// If the field is empty, the provided empty placeholder is written.
func writeBulletField(sb *strings.Builder, field, empty string) {
	field = strings.TrimSpace(field)
	if field == "" {
		sb.WriteString(empty + "\n")
		return
	}
	for _, line := range strings.Split(field, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		sb.WriteString(fmt.Sprintf("- %s\n", strings.TrimSpace(line)))
	}
}

// statusMarker mirrors changelog.go's statusEmoji checkbox markers.
func statusMarker(status string) string {
	switch status {
	case governance.ChangeStatusDone, "deployed", "completed":
		return "[x]"
	case governance.ChangeStatusImplementing, governance.ChangeStatusPROpen, governance.ChangeStatusReadyForTest:
		return "[-]"
	case governance.ChangeStatusBlocked, governance.ChangeStatusRejected, governance.ChangeStatusChangesRequested:
		return "[!]"
	default:
		return "[ ]"
	}
}

func writeFooter(sb *strings.Builder) {
	sb.WriteString("\n---\n")
	sb.WriteString("*Generated by OpenExec release governance*\n")
}

func changeID(ch *governance.ChangeRecord) string {
	if ch == nil {
		return ""
	}
	return ch.ID
}

func releaseLabel(rel *governance.GovernanceRelease) string {
	if rel == nil {
		return "(no release)"
	}
	if rel.Name != "" {
		return fmt.Sprintf("%s (%s)", rel.Name, valueOr(rel.ID, "-"))
	}
	return valueOr(rel.ID, "(no release)")
}

func releaseName(rel *governance.GovernanceRelease) string {
	if rel == nil {
		return ""
	}
	if rel.Name != "" {
		return rel.Name
	}
	return rel.ID
}

func fieldOr(ch *governance.ChangeRecord, get func(*governance.ChangeRecord) string) string {
	if ch == nil {
		return ""
	}
	return get(ch)
}

func valueOr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
