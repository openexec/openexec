# Exhausted native task integration discovery

US-007 / T-US-007-002. Discovery only; no production behavior changed.
The requested outcome is bounded verification of an explicitly corrected retained
candidate without resetting retries, losing evidence or widening effects. The
native task queue owns reconciliation; reuse its writer lock, task ledger,
receipt binding and completion guard. No new abstraction is needed for this
stage: zero new production concepts, state, transitions, decisions or loops.
The live Console Project context was read (accepted portfolio Goal revision 4);
its broader roadmap does not expand the selected task. The Simple Loop contract
keeps implementation in OpenExec and Goal review/delivery in Console.

## Observed baseline

The authoritative machine observation is
[the baseline result](verification/exhausted-task-discovery-result.json);
[the reproduction record](verification/exhausted-task-discovery.md) describes
its preserved source and synthetic provenance. Run
`scripts/verify-exhausted-task-discovery.sh` to reproduce it without future
implementation-story artifacts. It executes a real shell timeout, native repair
creation and completion, final failed attempt, authorized local correction,
queue refusal, database close/reopen and a second refusal. The corrected check
passes but the native queue dispatches nothing. Full task/receipt snapshots and
completed prerequisite remain unchanged. Numeric results belong only in the
machine record; the verifier checks them against the fixture contract.
This proves the retained-failure restart path, not a deployed owner incident or
an implemented reconciliation mechanism. Source-inspected boundaries below are
not claimed as newly executed behavior tests.

## Authority

`RunOptions.StoryIDs` bounds `ExecuteTasks`/`executeTaskQueue`; tasks retain
`story_id`, metadata `mode`, `decision_reason`, `decision_ref`, and `needs_review`.
The fixture's acceptance stored as `decision_ref` and its metadata
`correction_authorization` are explicit
synthetic owner facts, not authority consumed by the baseline. A receipt proves
failure, not permission. Reusing a prior grant or simply changing status cannot
establish fresh bounded correction authority. The production representation and
atomic consumption of that authorization remain an implementation boundary.

## Candidate binding

`Manager.start` supplies `Config.WorkDir` to the pipeline. Native
`SQLiteStore.CreateFailureRepair` preserves `git_branch`, `git_pr_number`,
`git_pr_url` and refuses disagreeing task/story branches. `resolveRecaptureCommand`
checks registered artifact path, digest-addressed evidence, command argv and cwd
against the same workdir. Tasks also retain `git_commits`; the fixture snapshots
these fields. A branch alone does not attest corrected bytes or authorization:
exact candidate content binding for exhausted correction remains unresolved.

## Bounded admission

`SQLiteStore.CreateFailureRepair` first handles an existing evidence-specific
repair, then bounds original `attempt_count` against `max_attempts` and prior
repair count. `runnableTasks` requires pending AFK work, remaining attempts, live
story and completed dependencies. `recaptureTaskFailure` uses a conditional SQL
claim bound to task, receipt and attempt; it spends an ordinary remaining attempt.
It cannot admit this exhausted diagnostic-bearing case. `retryWithStopReason`
stores `previous_attempt_stop` and only retries a changed reason within remaining
attempts. Future correction admission must be bounded without refunding history.

## Accepted validation

`Manager.SetTaskStatus` delegates to `UpdateTask`; entering done invokes
`SQLiteStore.CanCompleteTask`. The latest accepted `validation_plan_revisions`
row binds `task_id`, `generation_id`, `worktree_state_hash`; `graph_generations`
must remain current with the same hash. Accepted required/blocking
`validation_items` need supported `completion_claims` with predicate
`validation_item_passed` and matching `repository_state_hash`. No accepted plan
preserves legacy completion behavior. A successful recheck alone cannot bypass
these obligations or establish Goal Ready.

## Receipt retention

`persistTaskVerificationFailure` validates artifacts and persists references
before publishing repair eligibility. `Store.RecordTaskFailureStep` atomically
inserts `run_steps` and sets tasks metadata `verification_failure_evidence`,
conditioned on in-progress status and exact attempt. `run_steps` retains `id`,
`run_id`, `trace_id`, `phase`, `agent`, `iteration`, `status`, `metadata`;
metadata contains `verification_failure_receipt`, `verification_failure_digest`,
`stage_output`, `stage_error` and optionally `stage_diagnostics`.
`repairTaskFromRetainedFailure` checks ownership, failed status, deterministic
agent and digest. These checks do not authenticate arbitrary worker input;
the trusted executor boundary remains necessary. Fresh attempts clear only the
current task pointer, keeping the old step and repair evidence.

## Historical attempts

Task `attempt_count`, `max_attempts`, `status`, dependencies and commit history
are durable facts; pipeline state is attempt-local. `reconcileInterruptedTasks`
handles stale in-progress work without resetting counts. Old failure receipts
remain history even after correction. The baseline fixture checks complete task
snapshot equality and persistence, not just visible status. Later coverage must
prove duplicate authorization/restart cannot mint repeated extra admission.

## Completed repairs

Repair metadata `repair_of`, `failure_evidence`, `diagnosis` identifies an
existing evidence-specific disposition. `CreateFailureRepair` adds the repair
to original `depends_on`, retains branch identity and reopens only failed work
with budget remaining. Completed repairs do not refund attempts or justify a
replacement candidate. The fixture includes an already completed prerequisite;
future reconciliation must preserve its completion and historical evidence.

## Stop

`Manager.Stop` marks the in-memory pipeline stopped and cancels the attempt,
including deterministic checks. It is not a join or a persisted correction grant.
`entryFinished` and `Manager.Wait` use the attempt's done channel. Later tests
must prove a concurrent Stop wins over correction completion and cannot be
undone by an obsolete attempt's write.

## Cancellation

`waitTaskQueueRun` stops and drains the captured attempt on context cancellation.
`executeTaskQueue` leaves ordinary interrupted work for restart reconciliation;
`recaptureTaskFailure` records `recapture_outcome` with conditional ownership.
Pause drains before conditionally changing an in-progress task to needs_review.
Cancellation is distinct from the fixture's real timeout subprocess exit,
which is a retained deterministic check failure.

## Effects

`Manager.start` reuses the configured stage executor and native pipeline;
`recaptureTaskFailure` enters it with a deterministic stage and bounded timeout.
Repair creation grants no effects and runnable selection is not authorization.
Keep caller-supplied resource/effect restrictions, accepted scope and review
boundaries. No publication, merge, deployment or external provider was exercised
by discovery. End-to-end Console grant transport and exact candidate attestation
need later integration evidence; do not infer either from fixture metadata.

## Queue draining

`executeTaskQueue` holds `taskQueueActive` and `lockTaskExecution` until return.
`waitTaskQueueRun` waits for runner cleanup and event persistence before releasing
that writer ownership. With no runnable work, `retainedTaskBoundary` projects
remaining tasks and decision metadata; task completion is not Goal completion.
The exhausted retained receipt currently refuses before that boundary. Later
coverage must exercise success continuing the original queue, refusal retaining
work, cancellation while writing, and reopen after each disposition.

## Reconciliation function inventory

The machine inventory is
[`exhausted-task-inventory.json`](../scripts/verification/exhausted-task-inventory.json).
Each entry identifies a whole function by source path, receiver and declaration.
This is the starting denominator for later coverage measurement, not measured
coverage. Include newly changed functions when implementing; never substitute
package aggregate coverage for the affected functions. Inventory:

- `pkg/manager/scheduler.go`: `(*Manager).ExecuteTasks`.
- `pkg/manager/task_queue.go`: `(*Manager).executeTaskQueue`.
- `pkg/manager/task_queue.go`: `(*Manager).waitTaskQueueRun`.
- `pkg/manager/task_queue.go`: `retryWithStopReason`.
- `pkg/manager/task_execution_lock.go`: `(*Manager).lockTaskExecution`.
- `pkg/manager/task_execution_lock.go`: `entryFinished`.
- `pkg/manager/task_execution_lock.go`: `(*Manager).reconcileInterruptedTasks`.
- `pkg/manager/task_execution_lock.go`: `isRepairTask`.
- `pkg/manager/task_failure.go`: `(*Manager).persistTaskVerificationFailure`.
- `pkg/manager/task_failure.go`: `(*Manager).repairTaskFromRetainedFailure`.
- `pkg/manager/task_recapture.go`: `(*Manager).diagnosticFreeReceipt`.
- `pkg/manager/task_recapture.go`: `recapturePhase`.
- `pkg/manager/task_recapture.go`: `(*Manager).resolveRecaptureCommand`.
- `pkg/manager/task_recapture.go`: `(*Manager).recaptureTaskFailure`.
- `pkg/manager/task_boundary.go`: `retainedTaskBoundary`.
- `pkg/manager/manager.go`: `(*Manager).start`.
- `pkg/manager/manager.go`: `(*Manager).Stop`.
- `pkg/manager/manager.go`: `(*Manager).Wait`.
- `internal/release/failure_repair.go`: `(*SQLiteStore).CreateFailureRepair`.
- `internal/release/failure_repair.go`: `runnableTasks`.
- `internal/release/failure_repair.go`: `(*Manager).CreateFailureRepair`.
- `internal/release/failure_repair.go`: `(*Manager).RecaptureEligible`.
- `internal/release/manager.go`: `(*Manager).UpdateTask`.
- `internal/release/manager.go`: `(*Manager).SetTaskStatus`.
- `internal/release/sqlite_store.go`: `(*SQLiteStore).CanCompleteTask`.
- `pkg/db/state/task_failure.go`: `(*Store).RecordTaskFailureStep`.

Implementation reconciliation (US-008 / T-US-008-003) adds these final source
bodies without changing the frozen discovery journey or its original denominator:

- `internal/release/failure_repair.go`: `selectRunnableTasks`.
- `internal/release/sqlite_store.go`: `(*SQLiteStore).createTaskInternal`.
- `internal/release/sqlite_store.go`: `(*SQLiteStore).UpdateTask`.
- `internal/release/sqlite_store.go`: `canCompleteTask`.
- `internal/release/sqlite_store.go`: `(*SQLiteStore).BulkCreateTasks`.
- `internal/release/task_correction.go`: `CorrectionForTask`.
- `internal/release/task_correction.go`: `correctionChanged`.
- `internal/release/task_correction.go`: `(*SQLiteStore).AuthorizeTaskCorrection`.
- `internal/release/task_correction.go`: `(*SQLiteStore).AdmitTaskCorrection`.
- `internal/release/task_correction.go`: `(*SQLiteStore).FinishTaskCorrection`.
- `internal/release/task_correction.go`: `(*SQLiteStore).FailTaskCorrection`.
- `internal/release/task_correction.go`: `(*SQLiteStore).finishTaskCorrection`.
- `internal/release/task_correction.go`: `(*Manager).CorrectionEligible`.
- `pkg/manager/events.go`: `(*Manager).consumeEvents`.
- `pkg/manager/events.go`: `updateInfo`.
- `pkg/manager/task_correction.go`: `(*Manager).CorrectionCandidate`.
- `pkg/manager/task_correction.go`: `(*Manager).AuthorizeTaskCorrection`.
- `pkg/manager/task_correction.go`: `(*Manager).checkCorrectionCandidate`.
- `pkg/manager/task_correction.go`: `(*Manager).correctionPlan`.
- `pkg/manager/task_correction.go`: `correctionCheck`.
- `pkg/manager/task_correction.go`: `(*Manager).reconcileTaskCorrection`.
- `pkg/manager/task_queue.go`: `(*Manager).persistTaskExhaustion`.

## Required OpenExec checks

- This task: `scripts/verify-exhausted-task-discovery.sh` runs the frozen native
  journey and verifier refusal controls, validates documentation/inventory and
  rereads persisted evidence. No implementation-story artifact is required.
- Root AGENTS.md / AGENTS.local.md: `make lint` (Go vet, optional golangci-lint,
  UI ESLint), `make test` (Go plus serial UI Vitest), `make type-check` (Go build
  and UI tsc). `make compat-test` is required for loading, migration, self-healing
  or legacy changes; docs/AGENTS.md identifies its Compatibility Go selection.
- Makefile defines those targets. `.openexec/openexec.yaml` enables quality gate
  lint with custom command `go vet ./...`; this narrower gate is not the full
  repository verification suite.
- `.github/workflows/ci.yml`: Linux/macOS Go 1.25, Node 22, npm ci/UI build,
  embedded UI Go build, `go test -v ./...`, `make compat-test`; golangci-lint is
  advisory there. Host declared `lint` and `test` are the available pipeline
  checks in this candidate environment. Full canonical runner gates and review
  remain Console-owned; this Makefile has no check or pr-gate targets.

## Evidence and unresolved boundaries

Fresh task verification is recorded in NOTES.md; the machine result is replaced
on each successful reproduction. Documentation/inventory checks validate source
symbols and required evidence, not semantic correctness of future code.
Compatibility evaluation: only docs, inventory and discovery tooling change;
production current/legacy loading and migration are untouched.

Supplied Console test results, the Settings commit and hooks drift are
historical context only, not checks performed here. The supplied Console serving
revision is not OpenExec deployment evidence. No claim is made that Settings,
hooks, a live deployment or owner acceptance were verified in this stage.
Unresolved implementation boundaries are authority consumption, exact corrected
candidate binding, one bounded admission, Stop races, unchanged accepted
validation and queue convergence. These are later implementation/verification
work within the accepted Goal, not a request for additional owner approval.
