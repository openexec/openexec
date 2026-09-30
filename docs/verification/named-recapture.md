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

On 2026-09-30, this implementation stage resumed from candidate baseline
`0c0f4b49`. The Console project context and the local architecture contract were
read; the supplied task scope and delivery boundary remain authoritative.
The requested resolver, independent admitted fixture, native command handling,
provenance and dispatcher fix are already present. No additional runtime change
is necessary.

The earlier repair corrected the public dispatcher: `legacy-incident` formerly
returned unknown-case exit 2; it now invokes the same verifier as
`named-recapture`. Historical diagnostic-free receipts do not identify their
executed command or revision and cannot establish a current failure. Fresh host
checks are available now; the old check-availability limitation is removed.

Current checks:

- `bash scripts/verify-verification-repair.sh --case legacy-incident`: exit 0.
  All four lint/test × empty/matching phase cases passed through real shell
  execution, SQLite reload and persisted repair creation. Both isolated resolver
  negative controls were rejected at unresolved/needs_review with zero
  executions. Compilation failures, skips and unrelated failures are not
  accepted as negative-control evidence.
- `python3 -m unittest discover -s scripts/verification -p test_named_recapture_dispatch.py -v`:
  exit 0; the public-command regression passed from outside the repository,
  checking the baseline and both mutation reports.
- `go test ./pkg/manager ./internal/pipeline ./internal/release ./internal/validation -run 'Test(NamedRecapture|LegacyRecapture|Recapture)|Compatibility' -count=1 -timeout=60s`:
  exit 0 for all four packages, with the Go cache set to
  `/tmp/openexec-retention-go-cache` through the subprocess environment. This
  exercises native definitions, queue boundaries and protected project formats.
- Host `run_declared_check(lint)`: exit 0, including Go vet and UI ESLint.
- Host `run_declared_check(test)`: exit 0; `go test ./...` passed and UI
  Vitest passed all 635 tests in 40 files. UI emitted non-failing React test
  warnings. The earlier test-stage exit 2 did not recur.
- `bash -n scripts/verify-verification-repair.sh` and `git diff --check`: exit 0.

The journey deliberately stops at ordinary repair execution. It proves exactly
one named check, actionable retained evidence, a non-unresolved recapture outcome,
one persisted repair, and Settings pending; it does not claim the overall Goal
or repair implementation is complete. No Console process, deployment, or merge
claim follows from these checks.

Compatibility evaluation: this continuation updates evidence only. Existing
`.openexec`, `.uaos` and tasks.json behavior is unchanged. Complexity delta: zero
new concepts, persistent state, transitions, owner decisions or failure modes;
no existing execution machinery replaced.

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
