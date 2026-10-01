# Exhausted native task integration discovery

US-010 / T-US-010-001. Current review discovery; no production behavior changed.
Source inspected at b3d5236c53097c73057b8f7a13a5dfa5387774b1 on 2026-10-01.
The frozen US-007 reproduction remains historical baseline evidence.
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
establish fresh bounded correction authority. Current production authority is metadata `task_correction`, decoded by
`release.CorrectionForTask`. It requires decision/task/receipt identity,
candidate path/digest/branch and plan/state hash. The store authorizes only
failed tasks exactly at their positive attempt limit with matching branch and
receipt and no previous correction. Admission atomically marks consumed/running.

Production caller search:
`rg -n 'AuthorizeTaskCorrection' --glob '*.go' --glob '!**/*_test.go'`
found the manager and store declarations/comments and the manager-to-store call,
but no caller of `Manager.AuthorizeTaskCorrection`. Thus the shipped CLI, HTTP
and MCP paths do not expose that method in this checkout. This is a local source
finding, not proof that a separately built Console cannot call the exported API.

`internal/cli/approve_local.go` resolves infra approvals in
`.openexec/approvals.db` as cli-operator without a daemon; it does not authorize
task correction. `internal/mcp/server.go` captures
`OPENEXEC_OPERATOR_SESSION=1` at construction and advertises approval tools only
for operator sessions with a wired store. `internal/mcp/approvals.go` checks that
guard again on invocation. A correction entry point should reuse this trusted
operator separation, never infer authority from a receipt or a worker-supplied
decision string. The local CLI precedent relies on terminal access; the MCP
guard does not automatically secure an arbitrary new CLI command.

## Candidate binding

`Manager.start` supplies `Config.WorkDir` to the pipeline. Native
`SQLiteStore.CreateFailureRepair` preserves `git_branch`, `git_pr_number`,
`git_pr_url` and refuses disagreeing task/story branches. `resolveRecaptureCommand`
checks registered artifact path, digest-addressed evidence, command argv and cwd
against the same workdir. Tasks also retain `git_commits`; the fixture snapshots
these fields. A branch alone does not attest corrected bytes or authorization:
current `CorrectionCandidate` binds canonical root, branch, HEAD, tracked
and unignored untracked bytes and modes, including symlink targets/content.
`checkCorrectionCandidate` also compares the scan-manifest state hash.
Ignored runtime output is excluded. Binding exists; its interaction with
independent edits/commits and stale authorization is the unresolved defect.

## Bounded admission

`SQLiteStore.CreateFailureRepair` first handles an existing evidence-specific
repair, then bounds original `attempt_count` against `max_attempts` and prior
repair count. `runnableTasks` requires pending AFK work, remaining attempts, live
story and completed dependencies. `recaptureTaskFailure` uses a conditional SQL
claim bound to task, receipt and attempt; it spends an ordinary remaining attempt.
It cannot admit this exhausted diagnostic-bearing case. `retryWithStopReason`
stores `previous_attempt_stop` and only retries a changed reason within remaining
attempts. Current `AdmitTaskCorrection` provides a single extra verification pass without
incrementing or refunding attempts, conditioned on authority, failed status,
mode and review state. `CorrectionEligible` reuses story/dependency selection.
Pre-admission refusals currently have no persisted disposition (see review
assessment); admitted failure uses `FailTaskCorrection`.

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
snapshot equality and persistence, not just visible status. Current correction tests cover consumed admission/restart and duplicate
authorization refusal; that does not prove pre-admission refusal convergence.

## Completed repairs

Repair metadata `repair_of`, `failure_evidence`, `diagnosis` identifies an
existing evidence-specific disposition. `CreateFailureRepair` adds the repair
to original `depends_on`, retains branch identity and reopens only failed work
with budget remaining. Completed repairs do not refund attempts or justify a
replacement candidate. The fixture includes an already completed prerequisite;
current correction fixtures also assert its retention; expanded refusal journeys must retain that invariant.

## Stop

`Manager.Stop` marks the in-memory pipeline stopped and cancels the attempt,
including deterministic checks. It is not a join or a persisted correction grant.
`entryFinished` and `Manager.Wait` use the attempt's done channel. Current correction completion serializes its final check/write with Stop under
the manager mutex, checks context and pipeline completion, and defers a terminal
disposition after admission. This is source evidence; fresh race proof belongs
to the correction verification stage.

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
Current uncorrected exhaustion persists metadata.exhaustion and proceeds to
runnable selection. However, a pre-admission correction error returns before
selection and repeats on restart. Failed tasks are projected as BoundaryFailed,
even at the attempt limit, and TaskBoundary has no evidence_id field.
Existing successful correction tests are not proof of refusal queue draining.

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
The current implementation narrows the remaining gaps to pre-admission refusal
persistence/re-authorization, a trusted production authority surface, native
verification-script/no-plan support, and evidence-bearing exhaustion projection.
The review assessment below is discovery, not a declaration that repairs passed.

## Review assessment: observations versus proposed repairs

All four findings are supported by this checkout's source. Their severity is
credible; the reported production incident and every proposed scenario have not
been executed in this stage. No review finding is marked repaired or resolved.

1. **Pre-admission refusal (accepted diagnosis).** In
   `pkg/manager/task_correction.go:reconcileTaskCorrection`, decoding, receipt,
   candidate, plan and branch/eligibility checks precede admission and the
   deferred failure write. `task_queue.go:executeTaskQueue` returns their errors.
   Authorization is write-once and exhaustion skips existing correction records.
   Existing `TestCorrectionNativeQueueRefusals` checks an error and zero check
   calls for candidate/task_binding/stale_plan, without independent work or
   repeat-run disposition assertions in those cases.
   Proposed proof must cover edits, commits and regenerated unignored output;
   graph staleness/new accepted revisions; malformed authority/receipts; branch
   disagreement; restart and independent work changing the candidate. Persist a
   conditional consumed, never-admitted refusal, preserve failed status/attempts,
   and permit a fresh explicit decision only for that disposition, without fresh
   evidence. Distinguish store/IO failures from policy refusals.
   **Qualification of the suggested fix:** returning nil alone is insufficient
   if the next queue iteration selects the same failed correction forever:
   `processedFailure` restarts the loop. Terminal consumed refusals need an
   explicit skip/wait path. Restoring bytes is insufficient after stale-plan or
   task-binding damage; re-authorization must derive fresh valid bindings.
   Require independent completion, retained A at 3/3, unchanged original receipt,
   zero correction checks, second-run boundary/no dispatch, database reopen,
   and new-decision re-authorization. Negative control: restore the raw refusal
   return; typed-boundary and independent-completion assertions must fail.

2. **Authority reachability (accepted locally; cross-repository claim open).**
   The production search above establishes missing OpenExec transport. It cannot
   establish absence in another repository, which was not inspected.
   Reuse a trusted operator command to compute candidate/manifest, retained
   receipt, branch and latest accepted plan, then call the existing manager API;
   consume through the same scoped queue. Prove unauthorized/agent refusal and
   valid CLI authorization followed by A and dependent Settings completion,
   reopened DB, attempt_count 3 and byte-identical legacy receipt. Exercise a
   scratch CLI journey as well as automated coverage. Negative control: omit
   authorization or corrupt its persisted decision binding; completion must
   fail. An arbitrary different but internally consistent DecisionRef is not
   necessarily a valid negative control unless the test asserts the exact grant.

3. **Native checks (accepted diagnosis).** `correctionPlan` requires an accepted
   plan with at least one accepted required/blocking item; `correctionCheck`
   supports bare lint/test or shell -c argv only. The correction loop never reads
   task.VerificationScript. `planner_replay.go:reviewedPlanRows` and
   `internal/release/reviewed_plan_import.go` persist verification_script without
   creating accepted validation plans. This differs from CanCompleteTask's
   no-plan legacy rule and resolveRecaptureCommand's verify fallback.
   Proposed repair must always execute native task verification alongside all
   accepted obligations, allow candidate-bound no-plan tasks, and record fresh
   decision-tagged verify evidence even without an item. It must also change
   store decoding/finalization, which currently require plan/hash and query a
   plan on success. Test no-plan `test -f corrected.go` success and `exit 3`
   failure, accepted-plan plus native-script failure, supported/unsupported argv,
   stale/new plan refusal, persistence and unchanged attempts. Negative control:
   remove native-script inclusion; zero-check success and failed-script completion
   must be detected. **Qualification:** current admitted failures become
   needs_review, while the review asks for failed/continuing_failure. That
   lifecycle choice needs an explicit test expectation and terminal skip behavior;
   merely changing the status risks the same consumed-record queue lock.

4. **Exhaustion projection (accepted diagnosis).** `retainedTaskBoundary`
   assigns attempt_limit only to pending tasks; failed tasks always say failed.
   The store retains exhaustion.evidence_id but the report omits it. Project
   attempt-limit exhaustion and evidence_id without exposing reason in Error().
   Prove the no-authority A boundary contains attempt_limit and legacy evidence,
   including reopen. Negative control: restore the unconditional failed branch;
   kind/evidence assertions must fail.

These are completion checklists for subsequent implementation and verification,
not partially completed repairs. Existing tests were not weakened or changed.
The requested review-resolution/publication operations are outside this selected
discovery stage; delivery stays with Console after native preparation.

## Planning, story completion and delivery

`internal/planner/prompt.go` separates repository scope, candidate commit
ownership and human boundaries. AFK covers preparation, checks and ordinary
repairs; HITL represents an actual unresolved human requirement. Keep preparation
runnable before any retained acceptance task. Do not create native tasks for
Console publication, merge or deployment. This stage's explicit ordinary-Git
instruction governs its preparation commit.

Reviewed planning completion means an approved plan and durable import receipt,
not execution or product acceptance. `reviewedPlanRows` emits feature stories
and pending tasks; its verification_script is not a validation-plan revision.
`internal/release/models.go:ComputePhase` derives planned/building/maintaining
from non-maintenance story status and counts approved or done as completed.
That display/dispatch phase is not a Ready verdict. `SetStoryStatus` persists
story lifecycle, and `MergeStoryToRelease` has a separate approval/Git path
that marks a story done; neither establishes this Goal's default-branch D2.
The native queue returns nil for an all-done scoped task set, or a typed retained
boundary; it explicitly does not evaluate Goal/Ready.

This discovery finishes with source-backed documentation, its own verifier,
persisted evidence and a local commit. The full native preparation can finish
after all accepted repairs, regression/negative controls and required checks,
with one retained HITL acceptance task only if owner acceptance is required.
D1 needs those demonstrated repairs/checks; discovery alone does not meet D1.
D2 additionally needs Console evidence linking the verified candidate to the
actual OpenExec default-branch merge through existing promotion controls.
Neither a local commit, green fixture, accepted plan nor owner decision alone
proves that merge. Missing merge evidence is uncertainty, not proof of non-delivery.

## Hypotheses and cross-repository uncertainty

Hypothesis: the four local changes above suffice to reconcile the reported
exhausted shape. Falsify through the full operator-to-queue journey and each
refusal/restart/negative-control case; source inspection cannot settle it.
The actual T-US-005-002 database, Settings repair and Console authority adapter
were not inspected. A separate Console build could invoke the exported method;
confirm its interface, revision, grant transport and admitted executor independently
before claiming end-to-end integration or its absence. Do not read another
checkout from this task. Supplied Console revision f7bf25d5 and start time
2026-10-01T14:36:52Z identify a reported Console process, not an attested
OpenExec build or deployment. Earlier delivery notes and test summaries remain
historical claims; this document supersedes their implication of complete
production reachability without rewriting their historical test results.
