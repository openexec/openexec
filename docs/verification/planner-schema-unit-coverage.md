# Planner schema recovery unit coverage

US-008 / T-US-008-002, verified 2026-10-01 in the candidate worktree.
The outcome is regression protection for native schema correction, including
persisted limits and rejected evidence. The existing parser, RefinePlan and
Manager.Plan reviewed route own this behavior; this task changes tests and
verification only. Complexity delta: no production functions, persistent state,
transitions, owner decisions or runtime abstractions added.

## Complete production scope

The machine-readable [scope](planner-schema-coverage-scope.json) is checked
against Go AST function bodies changed since
`c13c3ec4c035a0febd724a28825dfe7dfe2c74f7` (before schema recovery).
New untracked Go files are included. Any added, modified or removed function
that disagrees with the manifest fails verification. Full function bodies,
including closures and failure paths, enter the denominator; there are no
coverage exclusions. The removed `Story.UnmarshalJSON` is recorded explicitly:
its array coercion was replaced by standard scalar JSON decoding. The modified
`RequirementIdentityRule` constant has no executable statements; its inclusion
in generation, compact, review and refinement prompts is asserted directly.

| Function | Responsibility | Covered / statements |
| --- | --- | --- |
| `Planner.parseResponse` | Envelopes, strict decode, empty-plan rejection | 22 / 22 |
| `ResponseDecodeError.Error` | Original response and concrete diagnostic | 1 / 1 |
| `ResponseDecodeError.Unwrap` | Typed JSON error propagation | 1 / 1 |
| `Planner.RefinePlan` | Public correction entry point | 1 / 1 |
| `Planner.RefinePlanWithSchemaCorrection` | Admission, bounded retry, evidence, human boundaries | 30 / 31 |
| `Manager.replayReviewedPlan` | Durable reservations, review, replay and import | 167 / 183 |

Aggregate: **222 / 239 statements, 92.887029%**. All 17 uncovered statements
remain counted, including defensive serialization and replay error returns.
The gate uses integer arithmetic (`10 * covered > 9 * statements`), so exactly
90% fails. Fresh instrumentation must match every scoped coverage block;
missing functions or missing profile blocks cannot shrink the denominator.

## Tests and executed evidence

- Extended `schema_recovery_test.go` with `TestSchemaSupportedEnvelopes`,
  `TestSchemaScalarPromptRequirements`, and `TestSchemaCorrectionFailurePaths`.
  They cover array/object/fenced/prose envelopes, empty/null plans, malformed
  syntax, partial decode rejection, scalar instructions, invalid review/plan/
  evidence and errors from both provider calls. Existing assertions were not
  weakened or replaced.
- Existing dedicated diagnostic, admission-refusal, bounded-retry and public
  runtime tests remain mandatory. Their checks retain exact malformed output,
  typed diagnostics, intent, original plan and rejected-review evidence.
- All six `TestReviewedPlanSchemaCorrection` scenarios are pinned individually:
  approval, rejection, malformed correction, interruption, human-boundary
  removal and remaining-budget exhaustion. These execute actual Manager.Plan
  with controlled providers and SQLite, close/reopen the database, verify spent
  reservations and retained limits, refuse unapproved imports and count the
  single approved import and persisted tasks.
- `scripts/verify-planner-schema-recovery.sh --case recovery`: exit 0. Discovers
  every required top-level test exactly once using `go test -list`, checks all
  dedicated test declarations against the manifest, runs fresh instrumented Go
  tests and requires package/test passes with no skips. Writes replaceable
  local evidence under `.openexec/planner-schema-checks/`; no generated artifacts
  are committed. Three Python verifier tests passed, proving rejection of
  omitted changed functions, missing/skipped tests, missing profile blocks,
  zero coverage and the exact threshold.
- `scripts/verify-planner-schema-recovery.sh --case discovery`: exit 0.
- Host `run_declared_check(check="test")`: exit 0, Go suite and 40 UI test files
  / 635 UI tests passed. Host `run_declared_check(check="lint")`: exit 0,
  Go vet and UI ESLint. Nonfatal UI warnings were emitted by the test suite.

Compatibility evaluation: this task changes no runtime behavior, plan schema,
project loading or migration. Existing `.openexec`, `.uaos` and tasks.json
support cannot change through these test/verifier edits. The native reviewed
journey is exercised with deterministic adapters, not a live model or deployed
service. Publication, independent review and canonical delivery remain Console
responsibilities.
