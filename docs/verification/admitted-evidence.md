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

## Console adoption — US-012 / T-US-012-003 (incomplete)

### Source evidence

Read-only reinspection on 2026-09-30 at 09:30 UTC resolved the owner-reported Console revision
with `git -C /mnt/data1/projects/agent-console rev-parse 4c819427` to
`4c819427643655fc85d830b3a79f337488bd3fc0`. Files were read with
`git show <revision>:<path>`, independently of the stale main checkout at
`1d8aa1d8f120c8347d0431210c92475085182a2a`. SHA-256 of those exact blobs:

| Path | SHA-256 |
| --- | --- |
| go.mod | 7c03c602ad132a9ac794804a3e37e3c078f8165dd8331c8b9e3c685bbbb2c8c7 |
| go.sum | 7cac86654ac1fb3b805b01e37bbe30c805972245e9b0f08f92834d7d5970ff4c |
| internal/server/openexec_checks.go | a7094a264d44a369574e938cf74133c0cfc6ef652abdadc7287e35f5c7447017 |

`executeOpenExecCheck` uses one `openExecCheckOutput` for both streams and
`VerificationCommandFailureWithOutput`; public StageResult.Output includes
joined argv and combined output. It retains an output tail, so saying it has
no output attachment would be incorrect. `git grep` at that revision finds
its call in `internal/server/openexec_task_loop.go:221`, but no adoption of
runtime.EvidenceBuffer, RetainCommandEvidence or the evidence-bearing API in
internal/server. The expected dedicated openexec_checks_test.go does not exist
at that revision; this alone does not establish absence of indirect coverage.

### Dependency evidence

Console go.mod pins OpenExec `v0.13.2-0.20260930061734-4290c0d23b00`, with no
replace directive; go.sum contains both module and go.mod checksums. Reading
that dependency's pkg/runtime/execution.go confirms WithOutput passes command
and output into NewCommandFailureWithOutput. It lacks this candidate's
VerificationCommandFailureWithEvidence API. A dependency-only bump would leave
the Console call incompatible: this candidate does not expose WithOutput.

OpenExec prerequisite source inspected and tested here is candidate baseline
`36ba60dca1cb00191f1f51e64bc976a953e33bed`. Its public runtime exposes the
EvidenceBuffer alias, RetainCommandEvidence and
VerificationCommandFailureWithEvidence. Availability in this candidate is not
proof that Console has updated its dependency or binary.

### Runtime evidence

The owner reports a serving process start of 2026-09-30T08:52:05Z for the
Console revision above. This is an owner observation, not independently read
binary build-info or a deployment receipt. No deployment claim follows from
source inspection. The earlier sandbox `/proc/*/comm` scan supplied no independent serving-binary
evidence; it was not repeated or treated as current deployment verification.

Fresh checks rerun in this OpenExec candidate on 2026-09-30, completed by
09:31 UTC (the host test log uses local time):

- `run_declared_check(check="lint")`: exit 0, Go vet and UI ESLint.
- `run_declared_check(check="test")`: exit 0, Go suite and 635 UI tests across
  40 files. Non-failing React act/style warnings and a WebSocket port-in-use
  message remain in the output; the check returned success.
- `bash scripts/verify-verification-repair.sh --case admitted-tracer`: exit 0,
  16 diagnostic-boundary and 16 silent/classification completions; both engine
  discard controls and the wrapper discard control rejected at their expected
  assertions. These execute real shell failures through the public OpenExec
  runtime, persisted step/artifact reload and repair generation, requiring exact
  argv/cwd and late diagnostic markers as described above.

The declared checks execute this repository's actual lint/test commands on the
host. Their successful returns do not exercise a failed Console adapter or
establish its private-reference persistence. The tracer uses an admitted
OpenExec fixture, not executeOpenExecCheck. Actual Console silent exit 2,
long-tail lint/test failures and exact command/cwd reference reload remain
**unverified and incomplete**. No currently failing repository check was
reproduced: both declared checks passed.

The previous implement stage stopped on this cross-repository integration
boundary, not a failing lint/test command. Re-reading the exact Console blobs
and rerunning both declared checks and the tracer confirms that distinction;
there is no OpenExec check failure to repair in this stage.

### Required integration and effect boundary

The available Console source is outside this stage's writable roots. The only
repository write grant is this OpenExec candidate and its specified git paths;
/tmp does not grant authority to change or deliver a different product. No
Console source/dependency writes, copied substitute adapter, deployment or
approval request was attempted. This is a source-write boundary, not a missing
credential or a sandbox socket failure. Host checks close the OpenExec
verification gap but cannot update Console through this grant. Fresh OpenExec
project-context lookup lists agent-console with repositories:read and
repository-documents:write (docs/ only); it supplies no additional source-write
authority. The declared lint/test checks run this OpenExec candidate, not a
Console candidate. Repeating this stage with the same effect roots cannot
remove the confirmed dependency/adapter gap; the dependent integration needs
an authorized Console source workspace, not another OpenExec test retry.

Retain T-US-012-003 and its affected adoption criteria as incomplete:

1. Update Console to the delivered OpenExec dependency and replace WithOutput.
2. Use separate runtime EvidenceBuffers and RetainCommandEvidence with exact
   argv/cwd/exit and explicitly collected allowlisted toolchain context. Attach
   its returned reference with VerificationCommandFailureWithEvidence.
3. Render PublicVerificationStream summaries with known secrets; keep command
   and output out of classification/digests. Preserve candidate locking,
   admission, process-tree cancellation and refusal behavior. Storage failure
   must not authorize repair.
4. Exercise actual Console lint/test silent and noisy failures through native
   persistence, close/reopen and repair, plus nil results, cancellation,
   launch/storage refusal and secret exclusion. Record the adopted dependency
   separately from any serving-binary evidence.

Only this dependent Console integration waits for an authorized Console source
workspace. OpenExec prerequisite verification above is complete. This note is
not integration completion; delivery remains Console-owned after the queue.

F2 public attachment is repaired in OpenExec. F1 recapture remains US-013, private
storage US-014, aggregate evidence US-015; independent story-wide coverage and
external delivery remain with their assigned stages. Canonical gates, review,
publication and merge remain Console-owned after the queue.

Compatibility: unchanged schemas, loaders, legacy receipt parsing and migration
fallbacks. Capture changes only the retained bytes on overflow; bounds, private
permissions, hashes and reference resolution remain intact. Complexity delta:
no persistent concepts, states, transitions, owner decisions or execution loops
added; existing buffer, typed failure, run-step persistence and repair reused.
