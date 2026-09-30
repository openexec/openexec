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

## Current Console adoption and precise remaining work

Read-only source inspection distinguishes the local Console checkout
`1d8aa1d8f120c8347d0431210c92475085182a2a` from the owner-supplied serving
revision `4c819427` (reported process start 2026-09-30T08:52:05Z). The stale
checkout uses `VerificationCommandFailure` and pins an older dependency; it is
not evidence of serving behavior. Inspected serving-revision source with
`git show 4c819427:go.mod` and
`git show 4c819427:internal/server/openexec_checks.go` in the Console repository.
That revision pins OpenExec `v0.13.2-0.20260930061734-4290c0d23b00` and calls
`VerificationCommandFailureWithOutput`. Inspection of that dependency's
`pkg/runtime/execution.go` and `internal/execution/gates/failure.go` confirms
joined command text and combined output enter CheckFailure and its digest.
Console uses one `openExecCheckOutput` for both streams and includes joined argv
in public StageResult.Output. It does retain an output tail; claiming it has no
output attachment would be incorrect. This is source-backed adoption evidence,
not independent attestation of the running binary or a live persisted repair.

Remaining Console integration is concrete and **not completed by this note**:

1. Upgrade the Console dependency to the delivered OpenExec revision exposing
   the private-reference APIs. Replace its WithOutput call; this candidate does
   not expose that dependency's WithOutput API, so a dependency-only bump is
   insufficient.
2. In `executeOpenExecCheck`, use separate runtime EvidenceBuffers, retain exact
   argv/cwd/exit and explicitly collected allowlisted version context privately,
   then attach the returned reference with VerificationCommandFailureWithEvidence.
   Preserve admission, candidate locking, process-tree cancellation and refusal
   checks; storage failure must not authorize repair.
3. Replace public joined argv/raw output with PublicVerificationStream summaries
   using known admission/configuration secrets. Keep command/output out of the
   public classification receipt and digest.
4. Exercise the real Console adapter on the upgraded candidate: silent and noisy
   failures through persisted native repair/reload, nil result, cancellation,
   launch/storage refusal, and secret exclusion. Attest the serving build and
   dependency after Console-owned delivery. This workspace grants no Console
   source writes or deployment; no integration completion is claimed here.

F2 public attachment is repaired in OpenExec. F1 recapture remains US-013, private
storage US-014, aggregate evidence US-015; independent story-wide coverage and
external delivery remain with their assigned stages. Canonical gates, review,
publication and merge remain Console-owned after the queue.

Compatibility: unchanged schemas, loaders, legacy receipt parsing and migration
fallbacks. Capture changes only the retained bytes on overflow; bounds, private
permissions, hashes and reference resolution remain intact. Complexity delta:
no persistent concepts, states, transitions, owner decisions or execution loops
added; existing buffer, typed failure, run-step persistence and repair reused.
