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
a fresh phase-correct receipt, registered readable command evidence and current
provenance, one same-story repair containing the stderr marker and receipt ID,
recapture outcome failed (not unresolved), and Settings pending with zero attempts.
The fixture intentionally stops when the generated repair reaches ordinary work;
it does not claim that repair execution or the overall Goal is complete.

`internal/pipeline/named_recapture_test.go` executes configured native lint/test
commands and verifies their typed failure, stderr and current provenance; missing
native definitions refuse. Existing recapture tests exercise success, attempts,
restart, ownership, dependency/refusal boundaries and protected project formats.

Verified 2026-09-29 with a writable Go cache under /tmp:

- `bash scripts/verify-verification-repair.sh --case legacy-incident`: passed all
  four incident cases. In isolated source copies, separately removing phase
  inference and named fallback each failed the incident assertion with calls=0,
  status=needs_review and recapture_outcome=unresolved. Compiler errors, skips,
  unrelated assertions or missing test completions are rejected by the verifier.
- `go test ./pkg/manager ./internal/pipeline ./internal/release ./internal/validation -run 'Test(NamedRecapture|LegacyRecapture|Recapture)|Compatibility' -count=1 -timeout=60s`:
  passed, including native definitions and .openexec/.uaos/tasks.json journeys.
- `python3 -m unittest discover -s scripts/verification -p 'test_*.py' -q`: 75 tests passed.
- `bash scripts/verify-verification-repair.sh --case study`: passed the source,
  ownership and saved-document checks (not a product-completion certificate).
- `bash -n scripts/verify-verification-repair.sh` and `git diff --check`: passed.

The added phase helper is included in the US-013 full-function scope manifest;
no statement-coverage result is claimed in this implementation stage.

## Failed task verification repair

Repair `repair-2d2a8484fad9be3b0170c225126befce` preserves T-US-013-001's
accepted command from the retained plan: `--case legacy-incident`. Before this
repair that exact command returned exit 2 with `unimplemented or unknown
verification case: legacy-incident`; the previously documented `named-recapture`
command passed. The supplied test/exit-2 receipt contains no command diagnostics,
so it alone does not prove a Go defect. The concrete reproduced defect was the
missing dispatcher entry for the task's verification script.

Both names now invoke the same unchanged incident verifier, preserving its real
queue/reload/repair assertions and both resolver mutations. A new public-command
regression test invokes the retained command from /tmp and checks its baseline
and mutation results. Removing only the dispatcher alias made this test fail on
the original unknown-case exit-2 diagnostic; restoring it passed. Unknown and
unimplemented cases still fail closed. No task plan or runtime behavior changed;
complexity delta: one command alias and its test, no persistent state,
transitions, owner decisions or new execution machinery.

## Review and delivery boundary

F1's empty-phase and named-command defect is accepted and repaired here. The
literal suggestion to retain needs_review *only* for ambiguity/HITL/exhaustion is
rejected: cancellation, admission refusal, unsafe evidence and stale ownership
must preserve their existing boundaries. These are not permission to repair code.
F2's public typed-error helper already exists in this candidate from US-012;
Console adoption is separate and not established by this fixture. F3 storage
privacy remains US-014 work. Remaining US-013 story-wide coverage/boundary proof,
US-015 aggregate evidence and D2 are not claimed complete. No Console source,
process, deployment or merge status was inferred from this local test.

Per the supplied stage restriction, ordinary local git commits persist this work.
Console owns publication, canonical socket-capable gates and review resolution
when the queue finishes. This stage neither publishes nor resolves the whole review.
