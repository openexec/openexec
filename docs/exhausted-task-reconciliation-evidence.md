# Exhausted-task reconciliation verification

US-008 / T-US-008-003; candidate verification on 2026-10-01. This record concerns
native exhausted-task correction, not older identity or retention stories with
reused task numbers. Production source is unchanged in this stage. The outcome
is evidence that an authorized corrected retained candidate can finish through
the native queue without resetting attempts, losing receipts or bypassing
completion/effect controls. The existing queue, SQLite ledger, accepted validation
plan and deterministic executor own that path; no new runtime abstraction,
persistent state, transition, owner decision or execution loop is added.

## Reproducible gate and inventory

`scripts/verify-exhausted-task-reconciliation.sh` now runs Python verifier controls,
fresh instrumented acceptance/related regressions, whole-function coverage and
source-overlay removal proof. Existing `admission-completion`, `native-queue` and
`exhaustion-controls` modes remain executable. `coverage` and `removal-sensitive`
can run independently. A missing/skipped/failed required test or missing reopened
persistence evidence refuses verification.

The discovery inventory retains all 26 original functions and adds 22 final
production bodies changed since `ca2bdf8c` (the pre-implementation discovery
revision). This includes correction APIs, consumption/disposition, candidate
identity, queue integration, event handling and task metadata preservation.
The AST helper compares complete bodies against that fixed revision, including
untracked production files, and refuses missing changed functions. Discovery's
own fixed original inventory check and frozen reproduction remain independent:
neither imports nor invokes the new coverage/removal tooling. Its missing-entry
control now removes the first original entry, rather than assuming the last
entry still belongs to the original denominator.

Coverage uses Go `-covermode=count`, a fresh profile and `-coverpkg` across
`pkg/manager`, `internal/release` and `pkg/db/state`. Expected instrumentation is
independently generated with `go tool cover` for every inventoried full body.
Every expected block must have a matching profile record and statement count.
Missing functions, missing records, malformed profiles and empty denominators
refuse. The threshold is strictly greater than 90% of statements across the
48 inventoried whole functions, not package coverage or changed-line coverage;
individual function measurements are reported below without claiming that every
function independently exceeds the aggregate threshold. Exactly 90% refuses.

## Native acceptance and negative controls

The existing success/refusal journeys execute real native deterministic commands,
close/reopen SQLite and re-enter the queue. They retain original attempt counts,
candidate identity, the completed prerequisite, original receipt and dependent
state, and prevent reauthorization from minting another pass. Continuing failures
reload the fresh command evidence and terminal disposition. Cancellation, Stop,
denied effects, agent-only claims, candidate drift and unmet validation remain
refusals, with independent work draining through the original queue.

Added `TestCorrectionDiagnosticQueueSuccess` exercises the same success/reopen
journey with a retained diagnostic-bearing timeout receipt matching discovery's
incident shape. The existing diagnostic-free success test remains. This receipt
is synthetic test data; the independent frozen discovery test actually executes
a shell timeout. No owner database or live deployment is implied.

Additional tests assert candidate bytes, executable modes, deletion and internal
symlink targets change identity; missing/non-Git/subdirectory/detached/unborn or
escaping/broken/directory-link candidates refuse. Plan and authority tests cover
missing/unsupported/empty/unaccepted plans, wrong graph hash, named check mapping,
invalid pre-consumed grants, lost stores, missing stories and competing writers.
Evidence insert/link/completion triggers verify unfinished state persists after
failure and cannot renew authority. Repair rollback, bulk creation, reserved
metadata, branch inheritance, task reassignment and legacy HITL boundaries are
also exercised. Late events cannot undo Stop.

No existing behavioral assertion was weakened. The receipt comparison helper
accepts the explicit diagnostic fixture's expected bytes and still compares the
entire persisted original receipt after reopening. Existing discovery controls
were adapted to the expanded inventory as described above. During development,
a new test incorrectly used schema status `draft`; corrected to `proposed`.
Another new closed-store test bypassed the fixture's once-only shutdown and
panicked during cleanup; corrected to use its existing `closeState` helper.

## Removal-sensitive proof

The mutation removes the actual `reconcileTaskCorrection` and
`persistTaskExhaustion` branches from the retained-failure loop in
`pkg/manager/task_queue.go`. It writes replacement source and Go overlay JSON
inside a repository-local temporary directory and runs only the diagnostic
native regression against that overlay. The original candidate source is never
written. The native regression must execute and fail, and its test output must
contain a source-location line ending in exactly `task repair attempt limit reached`.
A compile failure, timeout, skip, surviving test or unrelated error is not proof.

Go's regression timeout is 45 seconds and the subprocess bound is 120 seconds.
TemporaryDirectory cleanup applies on success, refusal and timeout; Python
controls exercise failure and timeout cleanup and reread unchanged source bytes.
The verifier prints the original source digest after successful cleanup.

## Fresh results

Toolchain: Go 1.26.5 linux/amd64, Node v22.23.2. Final native gate exit 0:
**1,293 / 1,433 statements (90.2303%)** across all 48 inventoried full bodies.
All **84 required tests/subcases** executed with no skips; reopened persistence
payloads were parsed. Nine Python control tests passed. Independent discovery
exit 0: five controls and the fresh native frozen reproduction passed; its
single JSON evidence file was replaced and reread.

| Check | Actual result |
| --- | --- |
| Default acceptance (coverage plus removal proof) | Exit 0; fresh uncached Go runs |
| Independent discovery script | Exit 0; frozen source and persisted evidence verified |
| `make test` via host declared `test` | Exit 0; final rerun passed Go suite and 635 UI tests in 40 files |
| `make lint` via host declared `lint` | Exit 0; final rerun passed Go vet and UI ESLint |
| `make compat-test` | Exit 0; current `.openexec`, legacy `.uaos`, config and tasks JSON fallbacks |
| `make type-check` | Exit 0; Go build and UI tsc |
| `make build` | Exit 0 with writable Go cache; UI bundle and embedded-UI Go binary built; generated tracked binary restored |
| Python verifier controls, shell syntax, `git diff --check` | Exit 0 |

The final mutation's native regression exited 1 at `task_correction_test.go:173`
with the exact original refusal. The verifier itself exited 0 only after checking
that failure. Original `task_queue.go` SHA256:
`50a6e238d3b49c77c63b702542414f5522cfe5fde43cfaf808f96c83fb661302`.
A subsequent repository directory scan found no `.exhausted-*` artifacts.

| Whole function | Covered / statements |
| --- | --- |
| `pkg/manager/scheduler.go:(*Manager).ExecuteTasks` | 144 / 168 |
| `pkg/manager/task_queue.go:(*Manager).executeTaskQueue` | 106 / 124 |
| `pkg/manager/task_queue.go:(*Manager).waitTaskQueueRun` | 45 / 51 |
| `pkg/manager/task_queue.go:retryWithStopReason` | 12 / 13 |
| `pkg/manager/task_execution_lock.go:(*Manager).lockTaskExecution` | 17 / 19 |
| `pkg/manager/task_execution_lock.go:entryFinished` | 6 / 7 |
| `pkg/manager/task_execution_lock.go:(*Manager).reconcileInterruptedTasks` | 12 / 13 |
| `pkg/manager/task_execution_lock.go:isRepairTask` | 1 / 1 |
| `pkg/manager/task_failure.go:(*Manager).persistTaskVerificationFailure` | 38 / 40 |
| `pkg/manager/task_failure.go:(*Manager).repairTaskFromRetainedFailure` | 17 / 18 |
| `pkg/manager/task_recapture.go:(*Manager).diagnosticFreeReceipt` | 18 / 19 |
| `pkg/manager/task_recapture.go:recapturePhase` | 8 / 8 |
| `pkg/manager/task_recapture.go:(*Manager).resolveRecaptureCommand` | 25 / 25 |
| `pkg/manager/task_recapture.go:(*Manager).recaptureTaskFailure` | 79 / 85 |
| `pkg/manager/task_boundary.go:retainedTaskBoundary` | 20 / 20 |
| `pkg/manager/manager.go:(*Manager).start` | 84 / 100 |
| `pkg/manager/manager.go:(*Manager).Stop` | 10 / 10 |
| `pkg/manager/manager.go:(*Manager).Wait` | 10 / 10 |
| `internal/release/failure_repair.go:(*SQLiteStore).CreateFailureRepair` | 71 / 78 |
| `internal/release/failure_repair.go:runnableTasks` | 1 / 1 |
| `internal/release/failure_repair.go:(*Manager).CreateFailureRepair` | 12 / 16 |
| `internal/release/failure_repair.go:(*Manager).RecaptureEligible` | 9 / 9 |
| `internal/release/manager.go:(*Manager).UpdateTask` | 28 / 31 |
| `internal/release/manager.go:(*Manager).SetTaskStatus` | 7 / 8 |
| `internal/release/sqlite_store.go:(*SQLiteStore).CanCompleteTask` | 1 / 1 |
| `pkg/db/state/task_failure.go:(*Store).RecordTaskFailureStep` | 16 / 16 |
| `internal/release/failure_repair.go:selectRunnableTasks` | 62 / 62 |
| `internal/release/sqlite_store.go:(*SQLiteStore).createTaskInternal` | 26 / 29 |
| `internal/release/sqlite_store.go:(*SQLiteStore).UpdateTask` | 31 / 34 |
| `internal/release/sqlite_store.go:canCompleteTask` | 15 / 18 |
| `internal/release/sqlite_store.go:(*SQLiteStore).BulkCreateTasks` | 32 / 33 |
| `internal/release/task_correction.go:CorrectionForTask` | 7 / 7 |
| `internal/release/task_correction.go:correctionChanged` | 7 / 8 |
| `internal/release/task_correction.go:(*SQLiteStore).AuthorizeTaskCorrection` | 6 / 6 |
| `internal/release/task_correction.go:(*SQLiteStore).AdmitTaskCorrection` | 2 / 2 |
| `internal/release/task_correction.go:(*SQLiteStore).FinishTaskCorrection` | 1 / 1 |
| `internal/release/task_correction.go:(*SQLiteStore).FailTaskCorrection` | 1 / 1 |
| `internal/release/task_correction.go:(*SQLiteStore).finishTaskCorrection` | 20 / 22 |
| `internal/release/task_correction.go:(*Manager).CorrectionEligible` | 13 / 14 |
| `pkg/manager/events.go:(*Manager).consumeEvents` | 63 / 70 |
| `pkg/manager/events.go:updateInfo` | 27 / 27 |
| `pkg/manager/task_correction.go:(*Manager).CorrectionCandidate` | 66 / 76 |
| `pkg/manager/task_correction.go:(*Manager).AuthorizeTaskCorrection` | 10 / 12 |
| `pkg/manager/task_correction.go:(*Manager).checkCorrectionCandidate` | 10 / 11 |
| `pkg/manager/task_correction.go:(*Manager).correctionPlan` | 22 / 23 |
| `pkg/manager/task_correction.go:correctionCheck` | 5 / 5 |
| `pkg/manager/task_correction.go:(*Manager).reconcileTaskCorrection` | 68 / 79 |
| `pkg/manager/task_queue.go:(*Manager).persistTaskExhaustion` | 2 / 2 |

## Limitations and provenance

The initial unrestricted coverage experiment ran all tests in the three packages
with a 120-second package bound and timed out in legacy scheduler tests. Coverage
now selects the accepted journeys and related deterministic regression tests
explicitly, still requiring the entire inventory's statement denominator. Early
coverage runs correctly refused insufficient coverage; instrumentation was then
expanded across packages and missing behavior tests added, without lowering the
threshold or removing functions. A read-only default Go build cache refused the
first embedded build; rerunning with a writable `/tmp` cache succeeded. Go also
emitted a read-only module stat-cache warning; this was not a failed final check.
Host UI tests emitted existing React act warnings but all assertions passed.

These are candidate checkout results. Earlier Console test, Settings commit,
hooks drift and serving revision claims are historical context only. The supplied
Console revision `f7bf25d5` is not OpenExec deployment evidence. No Settings UI,
live Console authority transport, publication, independent review, owner Goal
acceptance, merge, deployment or D2 completion is claimed. The canonical full
gate runs later in the socket-capable repository runner; this stage uses the
task verifier, changed-code tests and declared host checks. The local commit
preserves stage work only and does not deliver the product.
