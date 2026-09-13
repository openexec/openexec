# OpenExec-first Goal route

Status: implementation in progress; not deployed or a productive-autonomy claim.
Owner direction: 2026-09-13. Existing Goal/Ready, historical stops, resource
ceilings and effect approvals are not changed by this implementation plan.

## Inspection result

| Responsibility | Existing implementation | Reuse / demonstrated gap |
| --- | --- | --- |
| Requirement mapping, study, vertical slices, verification, AFK/HITL | `internal/planner`, public `pkg/runtime` | Reuse prompts, schema and lint. `Manager.Plan` previously generated/imported without invoking `StoryReviewPrompt`. |
| Durable stories/tasks and Git lineage | `internal/release`, SQLite `.openexec/openexec.db` | Reuse. Do not create Console copies of execution tasks. |
| Replanning import | `pkg/manager/planner.go`, `planner.RemapPlanIDs` | Reuse collision handling, but identity/title deduplication alone does not prove semantic equivalence. |
| Task execution | `pkg/manager.Start`, `internal/pipeline`, `standard_task` blueprint | Reuse implementation/lint/test/review stages and existing effects; do not replace with a new worker loop. |
| Task scheduling | `pkg/manager.ExecuteTasks` | Current invocation snapshots pending tasks, ignores task priority in its ready ordering, and cascades errors. New repair tasks cannot join that snapshot. |
| Resume | `WithResumeCheckpoint`, API resume handler | Scheduler's resumable-ID priority does not itself pass a checkpoint. Consuming `ResumeFrom` in the blueprint path remains a verification obligation. |
| Console adapter | `internal/providers/openexec.go` | `execution-stdio` is a single execution-provider call, not this task engine. Switching the provider does not switch controllers. |
| Console recovery | task strategies, typed results, predecessor selection, navigation cycles | Migrate execution responsibility to OpenExec for the new path; retain old histories and old path until equivalence is proven. |
| Resource/effect enforcement | Console execution protocol and reservations | The manager pipeline lacks the same aggregate token ceiling and caller-owned effect gateway. Must bridge these explicitly before live handover. |

The older loop's reported productive history is a comparison baseline, not
evidence that every present entry point is connected or safe. In particular,
`ExecuteTasks` over every pending repository task is not an authorized Goal
handover: unrelated work may exist in that repository.

## Dependency-ordered first slice

1. Connect OpenExec's existing review/refinement prompts through its public
   planning seam. A rejected or malformed review cannot import executable work.
   Preserve the existing caller path unless reviewed planning is selected.
2. Reuse existing SQLite tasks for an atomic failure/repair dependency. Keep
   the original story, task and candidate. Evidence identity makes duplicate
   failure handling idempotent. Never turn Stop, HITL or an effect refusal
   into an autonomous implementation retry.
3. Add a sequential, scoped live-queue path in the existing manager. Re-read
   task state after useful outcomes; select by prerequisites and priority.
   No second scheduler or task batch ledger. No-runnable is a boundary, not Ready.
4. Bridge the existing Console admission/effect/Stop controls to the existing
   OpenExec stage execution boundary. Resource checks govern starts, not task
   identity. Keep run-to-window accounting explicit across internal attempts.
5. Opt in one authorized brownfield Goal. Console projects accepted Goal/Ready
   and current evidence into OpenExec planning, observes task-set boundaries,
   then runs fresh Goal review. Gaps feed the same planner/backlog. Do not run
   a Goal evaluator after every provider response.
6. Prove the complete failure/repair/resume/remaining-gap/Ready journey, then
   a real unattended brownfield run. Only then remove superseded Console
   strategy branches for this execution path.

## Migration and deletion contract

Retain Goal/Ready provenance, owner Stop/revocation, exact effect authority,
candidate/review evidence, immutable usage, and process supervision. Historical
task-scoped authority must not become repository-wide or Goal-wide permission.

Demote Console NavigatorTasks to retained provenance/Goal handover references
where an OpenExec task owns execution. Do not synchronize two writable backlogs.
Remove per-worker Goal-gap derivation, ordinary task-repair strategy selection
and resource-generation continuation selection from the opted-in task loop.
Do not mechanically delete historical readers or active legacy execution paths.

No new persistent queue, generation, grant, task batch or task-state vocabulary
is planned. The review result implements an existing prompt's output schema;
repair linkage uses existing task metadata and dependency fields. A new adapter
is justified only by the concrete difference between the one-turn resource/
effect contract and the manager's current stage executor.

## Acceptance and negative controls

- Real manager planning entry point: rejected/malformed review never imports;
  accepted review persists tasks. Removing refusal must fail the test.
- Failed task plus concrete evidence creates one same-story repair, which is
  runnable before original/remaining work. Replay and SQLite reopen retain one
  repair. Original cannot resume until repair evidence meets task completion.
- HITL, missing dependency, missing scope, Stop and effect/capacity refusal do
  not become an AFK repair loop. Known branch/PR preserved; never use checkout
  position as candidate identity.
- Newly persisted repair work joins the running queue; no replacement task
  ledger and no parallel writer.
- Real blueprint stage failure, repair, original retry and remaining tasks;
  fresh Goal review then discovers an intentionally omitted accepted condition,
  feeds it into the same planner and executes it before Ready.
- Across restart, task/provenance survives and consumed resources do not reset.
- Native model runs, deployment, and hours of useful brownfield work are
  separate delivery evidence. Controlled-provider tests are not that evidence.

## Delivery status

The initial OpenExec-only slice now provides:

- opt-in existing plan review before import, content-addressed review evidence,
  and task-ID remapping before review rather than after it;
- an explicitly scoped sequential queue over existing SQLite tasks, with fresh
  dependency/priority selection and dynamically inserted repair prerequisites;
- trusted deterministic-check failure evidence, atomic with its task-attempt
  disposition, without treating worker-supplied artifacts as trusted receipts;
- same-story repair and original-task resumption; no second task database;
- cancellation draining, workspace execution exclusion, and scoped recovery of
  interrupted tasks without resetting attempts or changing candidate records.

Controlled-provider integration tests execute the actual manager and blueprint
engine against temporary project files: the configured check fails, a repair
creates the missing file, the original task resumes, and the remaining task
changes another file. SQLite reopening before repair insertion also resumes
that sequence. Separate tests cover interruption before the failure receipt,
stale attempts, failed receipt writes, candidate-record preservation and
cross-manager exclusion. These establish code-path behavior, not model
capability or deployed safety equivalence. OS restart still requires the
supervisor to terminate the previous process tree; a file lock alone does not
prove orphaned subprocesses stopped.

Negative controls physically remove refusal/transition logic and must fail:
unreviewed import, incomplete admitted planning adapters, review-time identity
drift, stale attempt binding, transient task storage, cancellation draining,
competing queue start, and receipt-driven repair consumption. Full canonical
validation and final independent disposition must be recorded separately.

First-slice review disposition (2026-09-13): independent deletion-first reviewer
reported no remaining blocking findings after cancellation/pause draining,
atomic failure/task persistence, scoped restart recovery, and stale-attempt
pause protection were corrected. Review covered the composed source and is
not a deployed-product verdict. A paused older attempt is tested against a
newer durable attempt written during drain; restoring the old snapshot-based
update makes that test fail.

Local validation completed: `make compat-test`, `make type-check` (Go and UI),
focused planner/release/gates/blueprint/pipeline/manager tests, focused manager
race tests, and `go vet` on the changed packages. The full `make test` remains
the next publication gate; no full-suite pass is claimed here.

No Agent Console execution subsystem has been removed or switched yet. The
new queue replaces snapshot selection only when explicitly selected; the
legacy route remains available for compatibility. Removed responsibilities in
the new path are conversation reconstruction, budget-based task selection and
dependent-task failure cascading—not historical audit records.

Bridge inspection found that merely swapping the blueprint's agentic runner
does not preserve Console's sandbox: deterministic Go actions, configured
shell checks and asynchronous quality callbacks still execute on the host.
The first Console-backed route must use admitted provider execution plus its
existing typed candidate/gate/review effects, or establish equivalent process
containment. Never exempt generic server-origin `run_command`. Pre-PR focused
checks and aggregate accounting across stages remain concrete integration
obligations. Verify against the actual deployed Console revision, not an
unmerged resource-separation branch.

PR #177's accounting candidate is independent and preserved; this branch does
not merge, deploy or rewrite it. No live provider capacity is created or renewed
by this plan. Full Console integration, live Goal convergence and the long-run
comparison are pending; no subsystem-removal or Ready claim is made yet.
