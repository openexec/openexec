# Named-check recapture — US-013 / T-US-013-001

The selected implementation stage fixes the accepted F1 resolver defect in the
existing queue. One validated receipt supplies a missing phase; a conflicting
phase or multiple checks still refuses. Registered historical shell references
remain authoritative and malformed, foreign, unregistered or empty-command
references cannot fall back to a current check. Verify scripts retain their
existing role. With no historical reference, lint/test are dispatched by name
through the admitted executor, or resolved from the native blueprint's current
project configuration. Missing native commands refuse rather than auto-pass.

The terminal receipt records `recapture_definition=current-check-definition`.
It describes current execution, never recovered historical argv. The original
receipt remains in run_steps; no loader, schema, task type, retry mechanism,
owner decision or workflow transition was added. Existing RecaptureStage,
attempt claims, admission, queue iteration and repair insertion are reused.
Complexity delta: one phase-validation helper and a named-stage branch; persistent
concepts, transitions and owner decisions added 0; existing metadata gains one
provenance value.

## Evidence

`pkg/manager/named_recapture_test.go` defines an independent admitted executor
using public runtime APIs. Its only command selector is stage.Name. It runs a
real sh process exiting 2 with stderr, retains exact argv/cwd/exit and attaches
the reference. Four cases seed empty/matching phases for lint/test with empty
verification scripts and no diagnostics/private refs. Each reopens SQLite,
walks ExecuteTasks → executeTaskQueue → recapture → next-iteration repair,
then closes/reopens SQLite again. Assertions require exactly one check execution,
a fresh phase-correct receipt, unchanged original receipt metadata and phase,
registered readable command evidence with exact argv/stdout/stderr and current
provenance, one same-story repair containing the stderr marker and receipt ID,
recapture outcome failed (not unresolved), and Settings pending with zero attempts.
The fixture intentionally stops when the generated repair reaches ordinary work;
it does not claim that repair execution or the overall Goal is complete.

`internal/pipeline/named_recapture_test.go` executes configured native lint/test
commands and verifies their typed failure, stderr and current provenance; missing
native definitions refuse. Existing recapture tests exercise success, attempts,
restart, ownership, dependency/refusal boundaries and protected project formats.

## Current verification and repair diagnosis

On 2026-09-30 at 06:27 UTC, repair
`repair-7e6a9ed05d68f1a437dfc9fc2e1dccfd` resumed from candidate baseline
`c9447aa2` and reran the checks below. The Console project context was read
again; this stage retains the supplied repair scope and delivery boundary.
Read-only SQLite inspection confirmed that T-US-013-001 still specifies
`bash scripts/verify-verification-repair.sh --case legacy-incident` and that
its run used this candidate as `project_path` (`worktree_path` is null).
The run is keyed by `runs.id = T-US-013-001`; its `task_id` is null, so
filtering runs by task_id alone would incorrectly report no run. The run error is
`stage "test" failed: exit status 2`.

The selected receipt,
`verification-failure-f13d13cc4ea054bdaa2b09622c97c7976f34357007d6e6daebc898c8d989c0ac`,
has an empty phase and only the test/exit-2 receipt and its digest in metadata.
It contains no command, stdout/stderr, or private evidence reference. Thus the
stored failure cannot establish that the task verification script was the
command that failed, or identify a source defect. The original task, receipt,
plan and runtime database were not modified. The task is currently pending.
A fresh read of the test-phase and failure run_steps found only the running
`T-US-013-001-11` test-stage start marker and two diagnostic-free receipts.
The selected receipt still has no terminal command output. Its persisted start
is 2026-09-30 05:45:28 UTC, after dispatcher fix `cd9171a3` at
2026-09-29 20:31:07 UTC. The earlier receipt starts at 20:29:05 UTC,
before that fix. Both receipts have the same check/digest, but neither binds
the executed source revision or command; timing alone cannot establish that
the later execution used the fix or that it merely repeated stale evidence.
The local
`.openexec/openexec.yaml` declares only a custom `go vet ./...` lint gate,
so it supplies no missing test-command mapping. The checkpoint file contains
only gather-context markers and receipt/digest entries, without diagnostics.

The Console check interface refused execution during this US-013 attempt.
That availability limitation is superseded: T-US-014-001 ran the declared host
checks and reproduced and corrected a current test failure; see
[private storage verification](private-storage.md). The historical receipts
still do not identify their executed command or revision, so they cannot prove
that the current failure was the historical one.

Fresh checks in this attempt (all exit 0):

- `bash scripts/verify-verification-repair.sh --case legacy-incident`: all four
  lint/test × empty/matching cases passed, including real failing subprocess,
  exact retained command/streams, unchanged original receipt, SQLite reopen,
  persisted same-story repair and pending Settings assertions. Both isolated
  resolver mutations failed at the required unresolved/needs_review assertion
  with zero executions. The verifier rejects compilation failures, skips and
  unrelated failures as mutation evidence.
- `python3 -m unittest discover -s scripts/verification -p test_named_recapture_dispatch.py -v`:
  one test passed, invoking the public command from /tmp and checking the
  baseline and both mutation reports.
- `go test ./pkg/manager ./internal/pipeline ./internal/release ./internal/validation -run 'Test(NamedRecapture|LegacyRecapture|Recapture)|Compatibility' -count=1 -timeout=60s`:
  all four packages passed using `GOCACHE=/tmp/openexec-retention-go-cache`,
  set through the subprocess environment. This includes native command
  definitions and protected .openexec/.uaos/tasks.json journeys.
- `bash -n scripts/verify-verification-repair.sh` and `git diff --check`: passed.

The earlier repair `repair-2d2a8484fad9be3b0170c225126befce` fixed a
reproduced dispatcher defect: the retained `legacy-incident` case returned
unknown-case exit 2 while `named-recapture` passed. Both names now invoke the
same incident verifier. That fix is already present in the baseline and does
not explain this later receipt. No fresh failure was reproduced by the task's
own verification or the targeted tests, so this attempt makes no speculative
runtime or verifier change and does not claim the new failure repaired.
Diagnosing the failed test stage still requires its actual command and output
from the execution owner. Canonical repository gates remain runner-owned;
no sandbox gate refusal is being treated as a blocker.

Compatibility evaluation: this attempt changes documentation only; no loader,
schema, migration, execution or check behavior changes. Complexity delta: zero.
The added phase helper remains in the US-013 full-function scope manifest;
no new statement-coverage result is claimed.

## Review and delivery boundary

F1's empty-phase and named-command defect is accepted and repaired here. The
literal suggestion to retain needs_review *only* for ambiguity/HITL/exhaustion is
rejected: cancellation, admission refusal, unsafe evidence and stale ownership
must preserve their existing boundaries. These are not permission to repair code.
F2's public typed-error helper already exists in this candidate from US-012;
Console adoption is separate and not established by this fixture. F3 storage
implementation and verification are now recorded in [US-014](private-storage.md). Remaining US-013 story-wide coverage/boundary proof,
US-015 aggregate evidence and D2 are not claimed complete. No Console source,
process, deployment or merge status was inferred from this local test.

Per the supplied stage restriction, ordinary local git commits persist this work.
Console owns publication, canonical socket-capable gates and review resolution
when the queue finishes. This stage neither publishes nor resolves the whole review.
