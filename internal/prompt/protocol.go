package prompt

// SignalProtocol returns the signal protocol template text.
// Injected into every composed prompt to teach agents how to use openexec_signal.
func SignalProtocol() string {
	return `## OpenExec Signal Protocol

You have access to the ` + "`openexec_signal`" + ` MCP tool. Use it to communicate with the OpenExec
orchestrator throughout your work.

### Signal Types

- **progress**: Call when you make meaningful progress (e.g., tests passing, file created,
  design section completed). This resets the thrashing detector.
- **phase-complete**: Call when your workflow is fully complete. This ends the current phase.
- **blocked**: Call when you cannot proceed and need to stop (e.g., missing dependency,
  environment broken, contradictory requirements).
- **decision-point**: Call when you need operator intervention for a decision you cannot
  make autonomously (e.g., ambiguous requirement with no safe assumption).
- **planning-mismatch**: Call when you discover the FWU planning context doesn't match
  reality (e.g., interface contract describes a function that doesn't exist).
- **scope-discovery**: Call when you discover work that falls outside the FWU boundaries
  (e.g., upstream bug that needs fixing first).
- **route**: (Blade only) Call to route work to the next agent. Set target to "spark"
  (return for fixes) or "hon" (forward for refinement).

### Usage

Call ` + "`openexec_signal`" + ` with at minimum ` + "`" + `{"type": "<signal-type>"}` + "`" + `.
Add ` + "`reason`" + ` to explain why. Add ` + "`target`" + ` for route signals.
Add ` + "`metadata`" + ` for structured data (e.g., file list, test results summary).

### Important

- Call ` + "`progress`" + ` regularly — if the orchestrator sees no progress for several iterations,
  it will stop the pipeline assuming you are stuck.
- Call ` + "`phase-complete`" + ` exactly once, when your workflow is done.
- Prefer ` + "`blocked`" + ` over silently failing — the orchestrator can help.`
}

// ConsultProtocol returns the consultation protocol template text.
// Injected into every composed prompt to teach agents the three-tier resolution hierarchy.
func ConsultProtocol() string {
	return `## Consultation Protocol

When you encounter uncertainty or need clarification during your work, follow this
three-tier resolution hierarchy:

### Tier 1: Self-Resolve (Preferred)
Make a reasonable decision based on available context. Document it immediately:
- Record a DesignDecision via ` + "`tract_record`" + ` with your decision, rationale, and context.
- Proceed with implementation based on the decision.
- This is preferred for non-critical ambiguities where a reasonable default exists.

### Tier 2: Subagent Consultation
For questions that need another agent's expertise (e.g., design clarification from
the architect), use the Claude Code Task tool to spawn a lightweight consultation:
- Describe the specific question and relevant context.
- The subagent will respond with guidance.
- Record the outcome as a DesignDecision via ` + "`tract_record`" + `.

### Tier 3: Operator Escalation
For genuine blockers where no safe assumption exists:
- Call ` + "`openexec_signal`" + ` with type "decision-point" and a clear description of what you need.
- The pipeline will pause until the operator provides guidance.
- Use sparingly — only when you truly cannot proceed safely.`
}
