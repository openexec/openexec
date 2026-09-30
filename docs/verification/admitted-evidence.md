# Admitted evidence — US-012 / T-US-012-002

## Implementation and exercised path

The existing runtime helper `VerificationCommandFailureWithEvidence` attaches
private references independently of classification. The shared `EvidenceBuffer`
now drains every write while keeping at most 4096 bytes per stream: the first
2048 and last 2048 when truncated. Direct `RetainCommandEvidence` calls apply the
same bound. Untruncated streams remain exact. Public rendering removes cut lines
on both sides of the omitted middle and the incomplete final line, redacts known
secrets and credential assignments, and reserves room for `[truncated]`.
Private capture retains partial lines; public output conservatively omits them.

The public-runtime-only fixture executes real shell commands at the candidate
WorkDir. The silent exit-2 journey still has no StageResult artifact fallback.
The diagnostic variant emits more than 32 KiB on each stream, ending in distinct
stdout/stderr markers, with a private credential sentinel. Both lint and test
run with populated and nil results. The manager journey closes the producing
manager and SQLite connection, reopens the store, reads the failed step and
registered private artifact, derives one repair, rereads its persisted description,
and resolves the artifact again. Assertions require exact argv/cwd/exit, bounded
streams and diagnostic tails, truncation flags, and only the allowlisted toolchain
field. The repair contains the authoritative reference, never the raw secret.
A missing evidence identity is refused.

Pipeline journeys require the terminal typed reference, retained public markers
when a result exists, and no secret sentinel anywhere in the serialized event.
The classification receipt contains only gate and exit code; changing private
artifact identity or returning a nil result does not change its digest. Actual
launch failure and cancellation of a started process, plus admission refusal, preserve
nil result/error identity without granting repair authority. Existing public
classification cases cover reserved exits, mixed errors and success. Existing
manager cancellation/restart tests remain part of the full test check.

Toolchain filtering already existed and is exercised after artifact reload:
only go_version, node_version, npm_version, python_version and rustc_version
are accepted, each capped at 128 bytes. No environment dump is collected.

## Verification

Executed in this candidate on 2026-09-30:

- Declared host `test` before changes: exit 0 (Go suite and 635 UI tests).
- Declared host `lint` after implementation: exit 0 (Go vet and UI ESLint).
- Declared host `test` after implementation: exit 0 (all Go packages and 635 UI
  tests). The subsequent started-process cancellation refinement also passed
  the task verifier on the final test source.
- Python verifier suite: 75 passed. Its first run exposed a pre-existing stale
  study contract demanding pending US-014 after the storage repair was recorded.
  Restored canonical disposition/root-cause labels and made that status check
  accept the explicit recorded repair, without reintroducing a false pending claim.
- `git diff --check`: passed.
- Task verifier passed: `bash scripts/verify-verification-repair.sh --case admitted-tracer`.
  All 16 diagnostic-boundary completions and 16 silent/classification completions
  passed; both engine-discard and wrapper-discard mutants were rejected at the
  required assertions. It requires exact non-skipped diagnostic-boundary test completions, the silent
  public journeys, and independent engine/result and wrapper-discard controls.
  The initial extended manifest incorrectly expected intermediate Go subtest
  events for slash-separated names; corrected the manifest to the actual named
  tests. No production assertion was weakened.

## Console adapter — US-012 / T-US-012-003

The earlier claim that this candidate lacked WithOutput and required Console
source changes was stale. This rebased candidate exports both WithOutput and
WithEvidence. The owner supplied Console revision 4c819427 and dependency
a7327a560c4f; these are context, not a deployment verification by this stage.
No Console source or dependency change is required or made here.

WithOutput keeps its public signature and its existing command/6000-byte output
tail receipt. The native manager now recognizes a validated command-bearing
receipt as useful evidence even for silent exits and nil StageResults. It uses
existing synchronous run-step persistence and repair generation, without
inventing argv boundaries, cwd or private artifact references unavailable to
that API. WithEvidence continues to support separate private command artifacts.
CommandSecrets now extracts credential values only; a missing executable name
survives in previous_attempt_stop while credential values remain redacted.

The admitted-adapter verifier runs TestAdmittedAdapterFailureReloadAndRepair,
go build ./..., and go test ./pkg/runtime/... ./internal/execution/gates/...
./pkg/manager/.... The regression uses the public WithOutput API with the
Console argument shape (gate, process error, joined argv, combined output).
Eight journeys cover lint/test, silent exit 2/long diagnostic tails, and
populated/nil results. Each closes and reopens SQLite before deriving a repair,
checks the command and bounded tail, rereads the persisted repair context,
and refuses invalid-digest or missing evidence. Existing classification-only
receipts retain their recapture path. No schema, loading or migration changes.

Verification on 2026-09-30:

- Declared host `test`: exit 0, all Go packages and 635 UI tests in 40 files.
  Earlier runs failed; the final redaction change also required removing the
  old first-entry slice from both blueprint command callbacks.
- Declared host `lint`: exit 0 (Go vet and UI ESLint). A transient candidate
  origin lookup refusal cleared on retry without repository changes.
- `bash scripts/verify-verification-repair.sh --case admitted-adapter`: exit 0;
  all eight adapter journeys, build and the required package suites passed.
- Focused manager adapter/missing-command regressions, blueprint suite,
  runtime refusal cases and evidence redaction tests passed.
- Shell syntax and `git diff --check` passed.

Deployment, canonical gates, review and publication remain Console-owned. Complexity delta: no new
persistent concepts, transitions, owner decisions or execution loops; reuse of
existing receipts, run_steps and repair tasks.
