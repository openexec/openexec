# Verification failure execution-boundary inspection

Story US-007; task T-US-007-001. Inspected 2026-09-29 at source revision
`e5d026ab61966b06330530622ea21266879b63ce`. This is an inspection deliverable,
not implementation of the broader failure-evidence/recovery Goal.

## Context and authority

Read root `AGENTS.md`, `AGENTS.local.md`, `docs/AGENTS.md`, `NOTES.md`,
`PROJECT_INTENT.md`, and `docs/OPENEXEC_SIMPLE_LOOP_ARCHITECTURE_CONTRACT.md`.
Read Console project context through `openexec_get_project(project="openexec")`:
accepted Goal/Ready revision 4 and interpretation revision 10 describe
Professional Portfolio Stewardship. The supplied task narrows this stage to
inspection of failure evidence and bounded native recovery. Project-context
limitations and old working-memory entries are claims/history, not current
delivery evidence. No owner-only NOTES items were executed.

Outcome: retain usable failed verification evidence and recover legacy empty
failures without weakening execution boundaries. Observed source already has
the admitted executor seam, durable task attempts and trusted receipt repairs;
it also has the loss points below. OpenExec's existing task loop owns repairs,
retries and continuation. Console owns admission/effects and outer Goal review.
Reuse `StageResult`, events, `run_steps`, native tasks and `CreateFailureRepair`;
no new controller, recovery queue, grant or storage concept is justified here.
Complexity delta for this inspection: no concepts, persistent runtime state,
transitions, owner decisions, failure modes or replacement machinery added.

Actual permission for this stage is workspace inspection, local documentation,
focused fixture execution and an ordinary candidate git commit. Writes are
restricted to the supplied candidate, its enumerated git paths and temporary
storage. No live ledger mutation, provider spending, daemon start, PR publication,
merge or deployment is authorized by this inspection. Console owns later
delivery. Tool availability and repository membership do not expand authority.
The supplied Console revision/start observation identifies that process only;
neither its deployment nor a running OpenExec product revision was independently
verified here. Console implementation is outside this checkout's traced source.

## Source trace and production interfaces

| Boundary | Current source and behavior |
| --- | --- |
| Public admission seam | `pkg/runtime/execution.go`: public aliases for `StageExecutor`, `Stage`, `StageInput`, `StageResult`; `VerificationCommandFailure` classifies actual command errors. `Execute(context.Context, *Stage, *StageInput) (*StageResult, error)` is execution, not scheduling. |
| Native entry | `pkg/manager/scheduler.go:ExecuteTasks` selects the task-oriented route using `RunOptions{TaskOriented:true, StoryIDs:...}`. `pkg/manager/task_queue.go:executeTaskQueue` requires explicit story scope, sequential ownership and durable storage; it re-reads runnable tasks and counts the attempt before `m.start`. |
| Adapter wiring | `pkg/manager/manager.go:Config`, `start`: inject `StageExecutor` into `pipeline.Config`. Injected execution skips constructor-time legacy cleanup and native gate runner installation. `internal/pipeline/pipeline.go` selects the admitted wrapper instead of `DefaultExecutor`; local routing/context preparation, quality gates and checkpoints have nil-executor guards. |
| Admission to engine | `internal/pipeline/admitted_executor.go:Execute` forwards the result/error and separately extracts trusted error receipts. Pipeline `OnStageStart` clears the prior receipt. `internal/blueprint/engine.go:Engine.Execute` invokes the executor and adds the result to both `Run.Results` and `StageInput.PreviousStages` before callbacks. |
| Result loss | `Engine.Execute` unconditionally replaces a returned result with `NewStageResult` when `err != nil`, then calls `Fail(err.Error())`. Existing output, diagnostics, artifacts and timing can disappear. This is source-verified; the current admitted fixture returns **nil** with its typed error, so it does not test preservation of a populated result plus error. |
| Stage events | Pipeline `OnStageComplete` emits stage status, attempt, output, error and artifacts; it does not include `StageResult.Diagnostics` or stage timing. `pkg/manager/events.go:updateInfo` deliberately keeps a failed stage nonterminal because the engine may retry. |
| Terminal failure | Pipeline's post-`engine.Execute` branch emits `EventBlueprintFailed` with engine error and `gateRunnerAction.terminalEvidence`. That helper requires a currently failed deterministic stage and a valid private receipt; it never promotes worker artifacts to repair authority. Cancellation excludes terminal receipt propagation. |
| Durable repair evidence | `pkg/manager/events.go:consumeEvents` calls `persistTaskVerificationFailure` before publishing terminal status. `pkg/manager/task_failure.go` accepts only valid blueprint-failure receipts, hashes task/start/artifacts for the record ID, writes synchronously and reads it back. `pkg/db/state/task_failure.go:RecordTaskFailureStep` atomically inserts `run_steps` metadata and binds `tasks.metadata.verification_failure_evidence` to the matching in-progress attempt. |
| Other storage limitations | `pkg/manager/events.go:writeRunStepAsync` accepts stage-start/stage-complete/iteration-start/complete events, **not stage-failed events**. Its step metadata contains text, prompt hash and review cycle, not error/diagnostics. Optional audit logging includes text/error/artifacts but is not the synchronous repair handoff. `runs` terminal status/error is separate from task repair evidence. |
| Repair creation | `pkg/manager/task_failure.go:repairTaskFromRetainedFailure` re-fetches the receipt, checks task ownership/status/agent and validates its shape, then passes gate names/exit codes in the diagnosis to `internal/release/failure_repair.go:CreateFailureRepair`. A transaction creates an idempotent same-story prerequisite and returns the original task to pending. Candidate branch/PR identity and execution mode are retained. `RunnableTasks` prioritizes recognized repairs before normal priority ordering. |

`internal/blueprint/stage.go:StageResult` already has Output, Error, Artifacts,
Diagnostics, Attempt and timestamps. The trusted receipt in
`internal/execution/gates/failure.go:CheckFailure` carries only gate and exit code;
its SHA-256 validates integrity, not provenance. It does not carry command output
or diagnostics. Preserving a result alone therefore cannot prove durable,
useful diagnostic delivery to the repair task.

Standalone execution is a distinct existing route:
`internal/blueprint/executor.go:executeDeterministic` captures shell output and
returns a failed result with **nil** Go error; configured verification stage
identity is pointer-bound and `OnVerificationFailure` separately captures the
typed error. Injected execution must not fall back to these host commands.
`gates.NewCommandFailure` only classifies direct `*exec.ExitError` with codes
1–125 and a live context. Missing executable, cancellation, unknown errors and
mixed error trees do not authorize repair. Worker prose or a correctly hashed
worker artifact is insufficient.

## Retry controls and reachable legacy state

- Engine retries follow each stage's `OnFailure` and `MaxRetries` plus
  `EngineConfig.MaxTotalRetries` (default 10). `Run.StageRetries` tracks stage
  retries; the total counter is local to `Engine.Execute`. A synthesized error
  result gets the computed attempt; an executor-supplied result keeps its own
  attempt. Do not confuse these with durable task attempts or agent-loop config.
- Queue dispatch increments persisted `Task.AttemptCount` before starting.
  `RunnableTasks` excludes spent attempts, HITL tasks, unresolved dependencies
  and out-of-scope stories. `CreateFailureRepair` refuses recursive repair,
  exhausted attempts, excessive prior repairs, completed stories and branch
  mismatch. Repair budget is `maxAttempts - attempts`; no reset/refund occurs.
- `pkg/manager/task_execution_lock.go:reconcileInterruptedTasks` already reopens
  in-progress tasks and eligible failed tasks **once at fresh queue entry**,
  under the workspace lock. Failed tasks need attempts left, no receipt metadata
  and non-HITL mode. This is broader than diagnostic-free failures; do not claim
  that legacy re-entry is entirely absent or introduce a competing retry budget.
- Inside the active queue, an unclassified failure has no receipt to repair from
  and returns an error. `waitTaskQueueRun` surfaces `PipelineInfo.Error` if present,
  otherwise the explicit “executor recorded no reason” message. This message
  does not itself recover the task. Receipt-free legacy rows are reachable via
  existing persisted failures and are exercised by restart fixtures. The source
  comment about four historical failures was not independently reproduced.
- Cancellation drains the exact attempt before releasing writer ownership;
  pause, Stop, HITL and retained boundary handling must remain distinct from
  recoverable implementation failure. See `task_queue_cancellation_test.go`,
  `task_pause_boundary_test.go`, `task_boundary.go` and `task_boundary_test.go`.

## Fixture seams and verification

Existing fixtures use production `Manager.ExecuteTasks`, the actual pipeline,
engine and SQLite ledger with injected executors or controlled subprocesses:

- `pkg/manager/admitted_execution_test.go`: typed command failure creates repair
  and resumes A; forged artifacts are refused; native host-command marker must
  remain absent; cancellation/resumption and foreign-run preservation.
- `pkg/manager/task_repair_journey_test.go`: A fails a real file check, one repair
  creates `feature.txt`, A resumes, B writes `remaining.txt`; receipts and task
  attempts are re-read. Restart test closes manager/database before repair.
- `pkg/manager/task_failure_atomic_test.go`: committed pair, injected receipt
  write failure rollback, and stale-attempt refusal.
- `internal/release/failure_repair_test.go`: reopened-store dependency/candidate
  retention, idempotency/concurrency, budget/recursive/branch guards, rollback.
- `internal/blueprint/verification_failure_test.go`,
  `internal/pipeline/verification_failure_test.go`,
  `internal/execution/gates/failure_test.go`: configured command identity,
  forged/stale receipts, ordinary prose, mixed errors and cancellation refusals.
- `pkg/manager/task_restart_boundary_test.go`: persisted failed task retries to
  completion with attempt 2; spent, human and out-of-scope work stays retained.

Read the candidate SQLite ledger using URI `mode=ro`; this task's
`verification_script` is the empty string. No task-specific script was available
to execute. The ledger was inspected, not changed. The focused command below
was run against the inspected source (temporary log `/tmp/T-US-007-001-tests.log`):

```sh
go test ./internal/blueprint ./internal/pipeline ./internal/execution/gates ./internal/release ./pkg/manager -run 'Test(Engine_Execute|ConfiguredVerificationCommandEvidence|TerminalVerificationEvidence|VerificationFailureEvidence|FailureRepair|RunnableTasksHonors|InjectedExecutorUsesRealQueueAndTrustedRepair|CancelledInjectedTask|InjectedManagerDoesNotReap|TaskFailureReceipt|TaskQueueRestartAfterFailureReceipt|TaskQueueFailedCheck|FreshTaskQueue|TaskQueueBoundaryKeeps|TaskOrientedQueueFailureDoesNotCascade|CancelledQueueRetainsWriter|LiveWorkspaceOwner)' -count=1 -timeout=60s -v
```

Result: all 24 selected top-level tests passed across five packages, including
the controlled A → repair → resumed A → B file-writing journey, database reopen,
forged evidence refusal and atomic rollback. `git diff --check` also passed.
This exercises local controlled journeys, refusals and persisted-state re-reads;
it is not a live Console-to-product delivery test. The populated-result-plus-error
loss, durable diagnostics handoff and in-queue legacy recovery still need direct
regression coverage when implemented. No production code/schema changed, so
this inspection cannot drop `.openexec`, `.uaos` or tasks.json compatibility.

Available repository commands: `make build`, `make lint`, `make test`,
`make compat-test`, `make type-check`; UI build/lint/Vitest/Playwright commands
are listed in AGENTS and Makefile. Changes to recovery/legacy behavior require
targeted regressions plus `make test`, `make compat-test`, `make type-check`.
The supplied delivery instruction assigns canonical `make check`/`make pr-gate`
execution to the socket-capable repository runner; neither target exists in
this checkout's Makefile. Resolve that command mapping at delivery rather than
claim those gates ran here. Full repository gates were not run for this
documentation-only stage.

## Unresolved interface questions for implementation

1. What exactly does the current Console adapter return for populated failed
   results plus errors, nil/nil, cancellation and admission refusal? The public
   seam is verified; its live consumer/version and payloads are not.
2. Which existing event/run-step fields should retain bounded diagnostics, and
   how should the repair description retrieve them while keeping trusted receipt
   classification separate? Define truncation/scrubbing and attempt identity;
   arbitrary worker output must never become repair authority.
3. What precise persisted predicate identifies legacy diagnostic-free failure,
   and permits retry within the same loop without reopening Stop, authority
   refusal, HITL, exhausted or out-of-scope work? Reuse counted task attempts and
   the existing lock; current fresh-queue reconciliation alone does not answer it.
4. Which declared runner check corresponds to the canonical gate for this exact
   repository revision? Which paired Console/OpenExec revisions and bounded live
   journey will later establish deployment evidence? Historical notes cannot.
