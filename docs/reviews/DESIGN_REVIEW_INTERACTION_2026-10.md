# Design review 3/6 — interaction conformance

**Date:** 2026-10-24. **Task:** `344c839036f26546d63e6128de0f3ae9` (third step
of the OpenExec `design-review` run; step 1 is
[DESIGN_REVIEW_INTENT_2026-10.md](DESIGN_REVIEW_INTENT_2026-10.md), step 2 is
[DESIGN_REVIEW_DRIFT_2026-10.md](DESIGN_REVIEW_DRIFT_2026-10.md)).

**Reviewed revision:** native `openexec` `origin/main`
`72b6d53f6688df72f70a49f5ae2bf0091c89c979`. Every `path:line` below is at that
commit unless it carries a source label from the table.

This report asks three questions: do the shipped workflows match the declared
workflows, is context preserved as work passes between them, and are the
numbered invariants still true. It changes no code, intent, grouping or
configuration, raises no owner decision, and is not evidence of Goal Ready or
of served behaviour.

## Rule being applied

The review step's own contract (Agent Console
`internal/playbook/playbook.go:263-268`, read from the local Console checkout,
which step 1 records as behind its origin): "Every finding names the workflow
or invariant number it violates." The step description is "Do the shipped
workflows match layer 2? Is context preserved across them? Are the numbered
invariants still true?"

## Sources

The labels are step 1's (its "Sources and provenance" table). One label is
added here.

| ID | Source | On `origin/main`? |
|---|---|---|
| **N** | `PROJECT_INTENT.md` (last changed `75e3bd87`, 2026-08-22), owner interview words | yes |
| **L** | owner checkout `PROJECT_INTENT.md`, uncommitted edit dated 2026-09-05 | **no** |
| **G** | owner checkout `GOAL.md`, untracked | **no** |
| **C** | Agent Console project context (interpretation revision 10), sourced from L | **no** |
| **K** | `docs/OPENEXEC_SIMPLE_LOOP_ARCHITECTURE_CONTRACT.md` (last changed `593a9e18`, 2026-09-14), "owner-mandated architectural constraint" (K:3) | yes |
| **KV3** | `docs/KNOWLEDGE_V3_PLAN.md` (last changed `674b50f0`), "Non-negotiable invariants" (KV3:114-125) | yes |

This pass did not open the owner checkout. L, G and C are used only as step 1
and step 2 recorded them.

Classification, as in step 2: **product** (code or CLI behaviour departs from
the declaration), **document** (a document is stale, wrong or missing), or
**N↔L** (the finding rests only on L, G or C). Severity: **high** (defeats a
declared workflow), **medium** (behaviour the owner or an agent relies on
contradicts a declaration), **low** (wording, stale document, dead code or an
opt-in path).

## What this review checks against

**I0 — Native OpenExec has no layer-2 interaction document.**
- No document on `origin/main` states the owner's workflows, the screens or
  commands each one passes through, or numbered interaction invariants.
- Documents considered and why none is layer 2:
  - `docs/STATE_MACHINE.md` describes blueprint stages inside one run.
  - `docs/LIGHT_MODE.md` describes one lane (MCP backlog over `mcp-serve`).
  - `docs/CONVERSATIONAL_ORCHESTRATION.md` covers `chat` only, and says
    "not re-verified line by line since; treat details as indicative" (line 3).
  - `docs/TASK_ORIENTED_GOAL_ROUTE.md` is an implementation plan, "implementation
    in progress" (line 3).
  - `docs/AUTONOMOUS_HUMAN_BOUNDARIES_IMPLEMENTATION_STATUS.md` is a status
    record, "incomplete implementation" (line 3).
  - `docs/architecture/orchestration-map.md` is a generated code map. Its
    "Architecture invariants" (line 112) are package import rules, not
    interaction rules.
  - `docs/REPOSITORY_POINTER_GRAPH_PLAN.md:591-602` lists "Central invariants"
    for graph fallback. They are not numbered.
  - ADR-001 to ADR-019 each decide one mechanism; none states an owner
    workflow end to end.
- Step 2 found the same gap one layer up (its D0: no layer-1 jobs document).
- Classification: **document**. Severity: **medium**. Without a layer-2
  document this review, and step 4 after it, can only check against the
  nearest committed statements below.

**Workflows checked (W1-W6).** These are labels local to this report for the
workflows K states. They are not owner-accepted layer-2 workflows and not new
product concepts. K is used because it is the committed, owner-mandated
statement of the loops and boundaries; it says it "is not proof of
implementation" (K:4-5).

| ID | Workflow | K lines |
|---|---|---|
| **W1** | Task convergence: intent/current state → task generation → independent plan review → repair incomplete plans → highest-priority runnable task → implement → verify → complete, or create higher-priority repair tasks → execute repairs → resume affected work → continue the original queue. Ordinary failures mean more work; "Runs are attempts; native tasks remain the durable truth of unfinished work." | K:18-28 |
| **W2** | Goal convergence: independent Goal review at a stable task boundary; gaps feed the same task generation and loop. Brownfield plans only missing work; planning happens at meaningful boundaries. | K:30-42 |
| **W3** | Small-task escape hatch: focused request → current-state inspection → compact planning → normally 1-3 tasks → implement → verify → done. | K:44-49 |
| **W4** | Deterministic delivery: exact candidate → provenance → required checks → independent review → effect gate → publication/deployment → verify running revision; software, not a general agent, performs known transitions. | K:51-60 |
| **W5** | Normal continuation and Stop: observe explicit Stop and hard boundaries; otherwise run the highest-priority runnable task; do not reconstruct work from old runs or conversations. | K:62-80 |
| **W6** | Owner attention: only for destination changes, product judgment, irreversible effects, hard resource boundaries, contradictions and Stop/Resume; not for next tasks, retries, repairs, sequencing or continuation. | K:82-86 |

**Numbered invariants checked (INV-K1 to INV-K5).** The only numbered
invariants on `origin/main` are KV3:114-125, and they cover the knowledge
index, not interaction:

| ID | Invariant | KV3 lines |
|---|---|---|
| **INV-K1** | No answer from a non-current generation without an explicit typed refusal | 116-117 |
| **INV-K2** | Every edge carries a `ResolutionStatus`; derived edges are never `compiler_exact` | 118-119 |
| **INV-K3** | Bounded lists disclose truncation and selection scope | 120-121 |
| **INV-K4** | Third-party indexers run as validated argv, never via a shell, under `internal/infra`'s deny-by-default posture | 122-123 |
| **INV-K5** | Legacy `.uaos/` and `.openexec/` projects keep loading; migrations are additive; `make compat-test` gates every phase | 124-125 |

## 1. Shipped workflows against W1-W6

**Checked:** the `run`, `plan`, `blueprint`, `chat`, `stop`, `release` and
`task` commands (`internal/cli`); the daemon routes in `pkg/api/server.go:100-112`
and `pkg/api/ws.go`; the scheduler, native task queue and repair code
(`pkg/manager`, `internal/release`); the standard blueprint
(`internal/blueprint/engine.go`); delivery code (`internal/release/automerge.go`,
`internal/git/tracker.go`).

### W1 — task convergence

**I1.1 — `openexec run` executes the legacy batch scheduler, not the W1 task
loop.**
- Workflow: W1 (K:20-28).
- Evidence:
  - `run` posts `runs:execute` with only `worker_count`, `mode` and
    `task_ids` (`internal/cli/start.go:305-311`).
  - `ExecuteTasks` takes the native queue only when `opts.TaskOriented` is
    true (`pkg/manager/scheduler.go:103-106`). The only non-test reads of
    `TaskOriented` are its declaration (`scheduler.go:17`) and that branch.
    The only non-test caller of `ExecuteTasks` is the `runs:execute` handler
    (`pkg/api/handlers.go:65-70`).
  - On the legacy path a failed task is set to `"error"`
    (`scheduler.go:368-371`), a status that is not a `TaskStatus`
    (`internal/release/models.go:220-227`). Its dependents are marked
    `"error"` and skipped (`scheduler.go:213-228`). No repair task is created.
    Only `"pending"` tasks are ever selected (`scheduler.go:123-129`), so the
    work stays unfinished.
  - The daemon logs the failure (`handlers.go:66-69`). `run` has already
    printed "Execution orchestrated by daemon" (`start.go:325`).
- What does match W1: the task-oriented queue (`pkg/manager/task_queue.go:13-168`)
  creates a same-story repair that runs first (`internal/release/failure_repair.go:283-294`),
  does not cascade failure, and is covered by passing tests (see "Checks run").
  It is reached over HTTP with `"task_oriented": true` and `story_ids`
  (`pkg/api/compact_plan_test.go:81`), which no native CLI command sends.
  `docs/TASK_ORIENTED_GOAL_ROUTE.md:130-133` records that "the legacy route
  remains available for compatibility" until equivalence is proven.
- Classification: **product**. Severity: **medium**. The owner's own command
  turns an ordinary failure into stopped work, which W1 says must not happen.
  It is not high because the W1 loop exists and is the documented route for
  the admitted caller.

**I1.2 — `run` ignores a failed or rejected plan and executes anyway.**
- Workflow: W1 ("repair incomplete plans", K:20-21).
- Evidence:
  - `run` discards the `runs:plan` response; only a transport error stops it
    (`start.go:293-303`).
  - The handler returns 400 or 500 on planner failure
    (`handlers.go:42-51`). `Manager.Plan` returns `Valid: false` with HTTP 200
    for an invalid intent (`pkg/manager/planner.go:109-114`).
  - `run` then posts `runs:execute`, which runs whatever pending tasks already
    exist (`start.go:304-326`). The owner is not told planning failed.
- Contrast: `openexec plan` prints "PLANNING GATE FAILED" and exits non-zero
  (`internal/cli/plan.go:74-80`).
- Classification: **product**. Severity: **medium**. Read from code, not
  executed.

**I1.3 — Independent plan review is not on the `run` or `plan` route.**
- Workflow: W1 ("independent task-plan review → repair incomplete plans",
  K:20-21).
- Evidence:
  - Review is opt-in: `Review bool … // Opt-in task-oriented route`
    (`pkg/manager/planner.go:34`), run only `if req.Review`
    (`planner.go:160-173`).
  - The review-and-refine loop is in `replayReviewedPlan`, reached only when
    the caller supplies `request_id` (`planner.go:64-65`).
  - `run` sends `{"intent_file":"INTENT.md","auto_import":true}`
    (`start.go:294-297`); `plan` sends `IntentFile`, `NoValidate` and
    `AutoImport` (`plan.go:65-69`). Neither requests review.
- Not the same as step 2's F3.3, which is the code-review stage after
  implementation, not plan review.
- Classification: **product**. Severity: **medium**.

**I1.4 — The standard blueprint's in-run fix stages are unreachable, and
`docs/STATE_MACHINE.md` says they run.**
- Workflow: W1 ("verify → … repair", K:21-22).
- Evidence:
  - `lint` and `test` set `OnFailure` to `fix_lint`/`fix_tests` but no
    `MaxRetries` (`internal/blueprint/engine.go:160-179`).
  - The engine follows `OnFailure` only while
    `run.GetRetries(stage) < stage.MaxRetries` (`engine.go:496`), which is
    never true at 0. A failed check fails the run (`engine.go:507-509`).
  - `docs/STATE_MACHINE.md:3` says "The stage sequence it describes is the one
    the engine runs"; lines 15, 23 and 25 say `fix_lint`/`fix_tests` are
    "Triggered on … failure".
- On the task-oriented queue this does not break W1: a failed check becomes a
  native repair task instead. On the legacy route (I1.1) nothing repairs it.
- Classification: **document** (and dead definitions in **product**).
  Severity: **low**. Read from code, not executed.

**I1.5 — The route plan describes "original-task resumption"; the code
closes the original and lets its fix carry the work.**
- Workflow: W1 ("execute repairs → resume affected work", K:22-23).
- Evidence:
  - `docs/TASK_ORIENTED_GOAL_ROUTE.md:72-73`: "Original cannot resume until
    repair evidence meets task completion"; line 97: "same-story repair and
    original-task resumption".
  - The code: the failed task "is closed, not reopened … and is never run
    again" (`internal/release/failure_repair.go:19-21`). `fixed_by` is set on
    it (`failure_repair.go:145-150`) and `RunnableTasks` skips it
    (`failure_repair.go:253-257`). Dependents unblock through `Delivered`,
    which follows the fix chain (`failure_repair.go:189-205`, `:263-278`).
  - A code comment is stale in the same way:
    `pkg/manager/task_execution_lock.go:74-76` says "repair creation refuses a
    repair of a repair", but a failed fix is fixed for the same original
    (`failure_repair.go:84-89`).
- The behaviour satisfies W1's "resume affected work" (dependents resume, and
  `TestTaskQueueFailedCheckClosesTaskAndItsFixDeliversIt` passes). The
  documents describe a different mechanism.
- Classification: **document**. Severity: **low**.

### W2 — Goal convergence

**I2.1 — Two committed documents name opposite owners for Goal review,
recovery and Stop policy.**
- Workflow: W2 (K:30-37) and W5 (K:67-69).
- Evidence:
  - K:30-37 and K:67-69 give Agent Console independent Goal review, outcome
    lifecycle and scoped Stop. `AGENTS.md:82-83` tells agents to read K
    before changing these loops.
  - `docs/AUTONOMOUS_HUMAN_BOUNDARIES_IMPLEMENTATION_STATUS.md:6-9`: "The
    owner assigns durable accepted Goal/Ready, execution policy, Goal review,
    recovery and human decision scheduling to OpenExec … The old Console
    ownership in the Simple Loop contract is superseded". Its "Still required"
    item 3 (lines 56-58) is to "Transfer the outer Goal loop … resource/Stop
    policy and restart recovery from Console to one engine owner".
  - K was not changed to record that reassignment (last changed `593a9e18`,
    2026-09-14; the status record is dated 2026-09-27). The status record
    cites no owner record for it.
- Related to step 2's F5.2 (the 2026-08-17 operating model plans Goal triage
  in OpenExec, against K). F5.2 is an older document predating K; this is a
  newer document overriding K. No duplicate fix is proposed: which document
  is right is not settled by this review.
- Classification: **document**. Severity: **medium**. An agent following
  AGENTS.md reads K and builds against the ownership the newer record says
  is superseded.

**I2.2 — Planning does not see current state and repeats on every `run`.**
- Workflow: W2 ("Brownfield assessment must identify existing behavior and
  plan only missing/change work … Planning and Goal review occur at
  meaningful boundaries, not after every model run", K:39-42) and W1
  ("Intent/current state → native task generation", K:20).
- Evidence:
  - `run` re-plans on every invocation unless `--no-auto-plan` is passed
    (`start.go:293`). `plan`'s help says `run` plans "if your plan is missing
    or stale" (`plan.go:29-30`); no such check exists.
  - The planner input is the intent file alone: `Manager.Plan` reads it
    (`pkg/manager/planner.go:132-135`) and calls `GeneratePlan(ctx, intent,
    nil)` (`planner.go:146`); the prompt is the PRD block plus the intent
    (`internal/planner/planner.go:163`). No repository or backlog state enters
    the generation prompt.
  - Import does reconcile with the ledger afterwards: identical IDs are
    continued (`planner.go:298-300`, `internal/planner/continue_retained.go:39`)
    and colliding new IDs are remapped and appended (`planner.go:302-326`).
- Whether a repeated `run` adds duplicate work depends on model output; this
  pass did not run it.
- Classification: **product** and **document** (`plan.go:29-30`). Severity:
  **medium**.

No finding on native Goal review itself: under K it is Console's, and
`openexec goal verify` was already recorded by step 2 (F4.2).

### W3 — small-task escape hatch

**I3.1 — `openexec run <task-id>` runs every pending task.**
- Workflow: W3 (K:46-47, "preferably one vertical task") and W6 (K:84-85).
- Evidence:
  - `run` puts the argument in `task_ids` (`start.go:309-311`).
  - `RunOptions` has no task-ID field: `TaskOriented`, `StoryIDs`,
    `MaxParallel`, `IsStudy`, `Mode`, `Yield` (`pkg/manager/scheduler.go:15-29`).
    The handler decodes with a plain `json.Decoder` (`handlers.go:58-62`), so
    the field is dropped and the whole pending backlog runs.
  - The help text promises "openexec run T-001 # Execute specific task"
    (`start.go:236`). The scheduler tells the operator that held `hitl` work
    should be run "individually when ready" (`scheduler.go:146`); this
    command cannot do that.
- Related to step 2's F2.1 (the dead `--quickfix` flag on the same command).
  This is a separate defect.
- Classification: **product**. Severity: **medium**. Read from code, not
  executed.

**I3.2 — A `chat` request that escalates to a run never reaches the
pipeline.**
- Workflow: W3 ("Focused request → … implement → verify → done", K:46-47).
- Evidence:
  - `chat` posts `/api/v1/runs` with `quickfix_title` set to the owner's line,
    then `/runs/{id}/start` with no body (`internal/cli/chat.go:190-209`).
  - `handleCreateRun` uses `quickfix_title` only for the run-ID prefix and the
    audit record (`pkg/api/handlers.go:100-102`, `:127-144`).
  - `handleStartRun` passes a task description only from its own body
    (`handlers.go:166-181`). `Manager.start` otherwise takes it from a ledger
    task with the run's ID (`pkg/manager/manager.go:435-437`); no task exists
    for a `T-QF-*` ID.
  - The `mode` the chat sent is also lost; the start handler defaults to
    `danger-full-access` (`handlers.go:176-179`).
- Classification: **product**. Severity: **medium**: the request the owner
  typed is the context lost. Read from code, not executed.

**I3.3 — `blueprint` and session runs leave no native task.**
- Workflow: W3 ("→ done") and W1 ("native tasks remain the durable truth of
  unfinished work", K:27-28).
- Evidence: `runs:blueprint` creates a `BP-…` run ID and calls `Mgr.Start`
  (`handlers.go:607-679`); `Manager.start` writes a `runs` row
  (`manager.go:522-524`) and no task row. A failed or interrupted blueprint
  run therefore leaves no unfinished task to resume.
- Step 2 found no duplication finding for `blueprint` (distinct mode). This
  is a context finding, not a duplication one.
- Classification: **product**. Severity: **low**.

What does match W3: the compact planner plus the task-oriented queue over HTTP
(`pkg/manager/planner.go:35`, `:143-144`), exercised by
`TestCompactPlanAndNativeExecutionThroughHTTP` (passes). No native CLI command
selects it.

### W4 — deterministic delivery

**I4.1 — The native merge path checks branch names, not candidate
provenance or review.**
- Workflow: W4 ("validate Goal/repository/candidate provenance → required
  checks → independent review", K:53-55).
- Evidence:
  - `CompleteTask` can auto-merge (`internal/release/automerge.go:49`). Its
    only caller is `openexec task complete` (`internal/cli/release.go:1358`).
  - `MergeStoryToRelease` checks that the two branches exist and merges with
    `--no-ff` (`internal/git/tracker.go:293-340`). No head SHA, check result
    or review is consulted.
  - The daemon's release manager uses `release.DefaultConfig()`
    (`pkg/manager/manager.go:640`), where `GitEnabled` is false
    (`internal/release/manager.go:74`), so the daemon route does not merge.
- Exact-head review and publication are held outside native code (C
  invariants; K:57-60). Native code has no PR-creation or deployment path to
  check against W4's later steps.
- Overlaps step 2's F2.3 (two completion paths) and F3.4 (opt-in approval).
  Not re-proposed.
- Classification: **product**, opt-in. Severity: **low**.

The review stage being skipped on the daemon route is step 2's F3.3 and is not
repeated here.

### W5 — continuation and Stop

**I5.1 — Stop is recorded as failure on the default route, and only a reset
of every task brings the work back.**
- Workflow: W5 ("observe explicit Stop", K:72-73) and W6 (Stop/Resume is
  owner attention; retries are not, K:82-85).
- Evidence:
  - A run stopped by the owner sends "task … stopped manually" into the error
    channel (`scheduler.go:357-361`) and the task is set to `"error"`
    (`scheduler.go:368-371`), the same as a failed one. Dependents cascade
    to `"error"` (`scheduler.go:213-228`).
  - No CLI command reopens one task. `openexec release reset` resets "all
    stories and tasks to pending" (`internal/cli/release.go:1675-1690`),
    including done ones (`internal/release/manager.go:153-165`).
  - Stop is available only per run over the WebSocket (`pkg/api/ws.go:231-237`)
    or for the whole daemon (`openexec stop`, `start.go:331-366`). There is no
    HTTP stop route (`pkg/api/server.go:100-112`).
- Classification: **product**. Severity: **medium**.

**I5.2 — "Resume" restarts runs from the beginning.**
- Workflow: W5.
- Evidence:
  - `POST /runs/{id}/resume` sets `cfg.ResumeFrom` through
    `WithResumeCheckpoint` (`pkg/manager/manager.go:310-325`). The pipeline
    declares the field (`internal/pipeline/pipeline.go:72-73`) and no
    non-test code reads it.
  - The WebSocket `resume` calls `Mgr.Start` (`ws.go:233-235`; its comment
    says "resume currently not in manager"). `Start` refuses a pipeline that
    is not finished (`manager.go:364-366`), which includes a paused one.
  - The scheduler orders tasks with interrupted runs first "for resume"
    (`scheduler.go:152-165`) but starts them fresh.
- W5 does not require checkpoint resume (runs are attempts, K:27), so the
  defect is the advertised behaviour, not the workflow.
- Classification: **product** and **document** (route name and comments).
  Severity: **low**. Read from code, not executed.

**I5.3 — After a daemon restart, nothing continues until someone runs
`openexec run` again.**
- Workflow: W5 ("otherwise execute the highest-priority runnable native
  task", K:72-74) and W6 (continuation is not owner work, K:84-85).
- Evidence:
  - `Server.Start` does not call `ExecuteTasks` (`internal/server/server.go:657-728`).
  - The restart clean-up resets tasks in status `"running"` or `"starting"`
    (`manager.go:208-223`). No non-test code writes either status to a task
    (writers: `scheduler.go:226,370,377`, `task_queue.go:108,120,132,163`,
    `internal/mcp/backlog.go:532`, `internal/cli/release.go:1165`). The legacy
    scheduler never marks a task in progress, so an interrupted task stays
    `pending` and is picked up again by the next `run`.
  - On the task-oriented queue, `reconcileInterruptedTasks` reopens
    interrupted work from task state (`pkg/manager/task_execution_lock.go:77-114`),
    tested by `TestFreshTaskQueueReconcilesOnlyScopedInterruptedWork` (passes).
  - `AUTONOMOUS_HUMAN_BOUNDARIES_IMPLEMENTATION_STATUS.md:56-58` lists
    restart recovery as still to be transferred.
- Recovery is from native tasks, not from runs, on both routes, which is what
  W5 asks. The gap is that continuation needs a person or an external caller.
- Classification: **product** (and dead clean-up code). Severity: **low**,
  because the gap is already recorded as remaining work.

### W6 — owner attention

No separate W6 finding beyond I3.1, I5.1 and I5.3, which each also name W6.
Checked and matching W6:
- `hitl` holds are planner- or caller-tagged (`scheduler.go:136-150`;
  `internal/release/failure_repair.go:250`), which K allows for genuine
  product judgment.
- Infra apply approval (`internal/mcp/infra.go`, `approve --local`) is an
  external effect, which W6 lists.
- The opt-in story/task approval is step 2's F3.4.

## 2. Context across hand-offs

| Hand-off | Route | Context preserved? | Evidence |
|---|---|---|---|
| Intent → plan | `run`, `plan` | Intent file only; no current repository or backlog state in generation; repeated on every `run` | I2.2 |
| Plan → run | `run` | Plan failure or rejection is dropped; run proceeds on old tasks | I1.2 |
| Plan → plan review | `run`, `plan` | No review on these routes; review exists for `request_id` callers | I1.3 |
| Run → repair → resume (W1) | task-oriented queue | Yes. The repair task's description carries the failure receipt ("How it failed (run step %s): %s", `pkg/manager/task_failure.go:140`); the fix runs first and dependents resume through the fix chain | `TestTaskQueueFailedCheckClosesTaskAndItsFixDeliversIt`, `TestFailureClosesTaskAndItsFixRunsFirstAcrossRestart` (pass) |
| Run → repair → resume (W1) | `run` (legacy) | No. Failure output is not carried anywhere; the task and its dependents end as `"error"` | I1.1 |
| Failure → restart → repair | task-oriented queue | Yes: a receipt persisted before a crash is repaired on the next queue entry (`task_queue.go:56-83`) | `TestTaskQueueRestartAfterFailureReceiptBeforeRepair` (passes) |
| Restart → continue (W5) | both | Recovered from native tasks, not runs; does not continue on its own | I5.3 |
| Stop → resume (W5) | `run` (legacy), WebSocket | Stop becomes failure; resume restarts from the start | I5.1, I5.2 |
| Focused request → run (W3) | `chat` | The request text is lost | I3.2 |
| Focused request → run (W3) | `run <task-id>` | The task ID is lost | I3.1 |
| Focused request → done (W3) | `blueprint` | No task record | I3.3 |
| Candidate → merge (W4) | `task complete` (opt-in) | Branch name only; no head or review | I4.1 |
| Leave → return | all | Native state survives in `.openexec/openexec.db`; `openexec status` shows task-count progress (step 2 F5.1) and ✗ for failed runs with no next step (`internal/cli/status.go:196-201`) | read from code |

Within one run, earlier stage output reaches later agentic stages only as
`truncate(prev.Output, 500)` without `prev.Error`
(`internal/blueprint/executor.go:449-455`). With the fix stages unreachable
(I1.4) this matters only for `review`. No separate finding.

## 3. Numbered invariants INV-K1 to INV-K5

The enforcement code and its tests exist for each invariant, and every test
listed in "Checks run" passed. The findings are entry points that bypass them.

**I6.1 — Legacy symbol routes answer with no generation at all.**
- Invariant: INV-K1.
- Evidence:
  - The DCP coordinator registers `tools.NewSymbolReaderTool(kStore)` with no
    repository root (`internal/cli/start.go:51`). Without a root the tool
    reads the legacy `symbols` table (`internal/tools/symbol_reader.go:66`,
    `:105`), with no freshness check.
  - `GET /api/v1/knowledge/symbols` (`internal/server/server.go:408`) calls
    `ListSymbols()` on the legacy table and ignores errors
    (`server.go:473-479`).
  - Both are registered only when DCP is enabled (`server.go:407`).
- KV3:129-147 says V3.0 connected the graph to agents on the MCP plane; this
  DCP path still uses the pre-graph table.
- Classification: **product**. Severity: **low** (DCP is opt-in).

**I6.2 — Partial, building and failed generations are served without a
refusal.**
- Invariant: INV-K1.
- Evidence: the gate refuses only `stale` and `superseded`
  (`internal/knowledge/graph_freshness.go:96-100`); `activeGeneration` falls
  back to `partial`, then any other status (`internal/knowledge/graph_store.go:169`);
  `partial`, `building` and `failed` are reported as `Freshness: partial`
  (`graph_store.go:200-201`).
- `REPOSITORY_POINTER_GRAPH_PLAN.md:585` says a partial graph may be used with
  missing coverage disclosed. KV3:116 says any non-current generation needs a
  typed refusal. The two documents disagree.
- Classification: **document** (contradiction), and **product** for `failed`
  and `building`, which neither document allows. Severity: **low**.

**I6.3 — Imports found lexically are stored as `compiler_exact`.**
- Invariant: INV-K2.
- Evidence:
  - `resolveImportNode` returns `ResolutionCompilerExact` for any target found
    in the module map, ignoring the import's own resolution
    (`internal/knowledge/graph_scan.go:1250-1253`).
  - Python imports are extracted as `ResolutionStaticLexical` after being
    rewritten to a repository path (`internal/knowledge/polyglot_extract.go:28-32`,
    `:190-206`), so they hit that branch.
  - Derived `tested_by` edges copy the same resolution
    (`graph_scan.go:922`).
- An agent asking for impact gets lexical guesses labelled as compiler facts,
  which is what INV-K2 forbids. No test asserts the resolution of these
  edges.
- Classification: **product**. Severity: **medium**. Read from code, not
  executed.

**I6.4 — Module dependency queries truncate edges without saying so.**
- Invariant: INV-K3.
- Evidence (`internal/knowledge/graph_query.go`):
  - Depth is clamped to `MaxDepth` with no limitation note (`:47-49`).
  - `loadImportEdges` is called with the remaining edge budget as its SQL
    `LIMIT` (`:62`, `:142`), and returns nothing once the budget is 0
    (`:135`). The in-loop `len(edges) >= MaxEdges` check (`:67-69`) therefore
    never fires, so `Truncated` is never set for edges. Node truncation is
    disclosed (`:77-79`).
- Contrast: `FindSymbolRelationships` discloses its depth clamp
  (`TestRepositoryGraphImpactDisclosesDepthCut` passes).
- Classification: **product**. Severity: **medium**. Read from code, not
  executed.

**I6.5 — Existing compiler invocations run outside `internal/infra`.**
- Invariant: INV-K4.
- Evidence:
  - No SCIP indexer exists yet (V3.2 is proposed, KV3:193).
  - The TypeScript compiler runs as argv `exec.CommandContext(commandCtx,
    node, scriptPath, compiler, root)` (`internal/knowledge/ts_compiler.go:97`),
    with no shell. Nothing in `internal/knowledge` imports `internal/infra`.
    The compiler is preferentially the repository's own
    `node_modules/typescript` (`findTypeScriptCompiler`,
    `ts_compiler.go:196`).
- The argv half of INV-K4 holds. Whether the `internal/infra` half applies to
  compilers that already run, or only to future third-party indexers, is not
  stated.
- Classification: **document** (scope of the invariant) and possibly
  **product**. Severity: **low**.

**INV-K5 — no finding.** `make compat-test` passes. No non-test Go or SQL file
drops or renames schema (the graph schema helper is "additive",
`TestEnsureKnowledgeGraphSchemaIsAdditive` passes). Note: `make compat-test`
runs only `./internal/validation/... -run Compatibility` (Makefile), so the
graph schema tests are gated by CI's `go test -v ./...`
(`.github/workflows/ci.yml:56`), not by `compat-test` as KV3:125 implies.

## Checks run

All commands ran in this task's worktree at `72b6d53f`. All exited 0.

| Command | Result |
|---|---|
| `make compat-test` | PASS: `TestCompatibility_ExistingProjects_StatusCLI` (both subtests), `TestCompatibility_LegacyProjectConfigFallback`, `TestCompatibility_LegacyTasksJSONFallback`; `ok internal/validation 3.963s` |
| `go test ./internal/knowledge/... ./internal/mcp/...` | `ok internal/knowledge 71.843s`; `ok internal/mcp 25.279s` |
| `go test ./pkg/manager/ ./internal/release/ ./pkg/api/ -run '<W1/W3/W5 tests>' -count=1 -v` | PASS: `TestATaskClosedWithItsFixIsNeverReopened`, `TestTaskQueueHumanWaitRunsIndependentWorkAndSurvivesRestart`, `TestTaskOrientedQueueFailureDoesNotCascadeOrInventRepair`, `TestTaskOrientedQueueDiscoversTaskAddedAfterFirstDispatch`, `TestTaskQueueFailedCheckClosesTaskAndItsFixDeliversIt`, `TestTaskQueueRestartAfterFailureReceiptBeforeRepair`, `TestFreshTaskQueueReconcilesOnlyScopedInterruptedWork`, `TestFailureClosesTaskAndItsFixRunsFirstAcrossRestart`, `TestCompactPlanAndNativeExecutionThroughHTTP` |
| `go test ./internal/knowledge ./internal/server ./internal/mcp ./internal/cli ./internal/tools ./pkg/db/state -count=1 -run '<INV-K tests>' -v` | PASS: `TestEveryGraphReadRefusesDriftWithoutRefresh`, `TestNoRefreshOnReadRefusesInsteadOfScanning`, `TestRepositoryContextRefusesGenerationSupersededMidBuild`, `TestRepositoryIdentityAndLegacyMigration`, `TestGoTypedReferencesDegradeWhenTheModuleDoesNotBuild`, `TestScanRepositoryUsesTypeScriptCompilerWhenProjectProvidesIt`, `TestChangedImpactAnalysisEnumeratesTraversalBounds`, `TestRepositoryGraphRefusesStalePointersWhenRefreshIsUnsafe`, `TestRepositoryEvidenceRefusesStaleGraphWhenRefreshIsDisabled`, `TestRepositoryGraphImpactDisclosesDepthCut`, `TestSymbolFindDisclosesTruncationAndLimitations`, `TestSymbolRelationsClaimsOnlyWhatTheGraphKnows`, `TestServableGraphRequiresAGeneration`, `TestEnsureKnowledgeGraphSchemaIsAdditive`; `internal/tools` had no matching test |

These tests show the W1 loop and the INV-K enforcement work where they are
wired. None exercises the defects above: every I-finding is **read from code,
not executed**. No daemon, CLI binary or model was run.

## Overlaps with step 2

| Step 2 | Relation here |
|---|---|
| F2.1 dead `run --quickfix` | Same command as I3.1; separate defect |
| F2.3 two completion paths | Context for I4.1 |
| F3.3 `init` review answer discarded | Code-review stage; I1.3 is plan review. Not repeated |
| F3.4 opt-in approval gate | Context for I4.1 and W6. Not repeated |
| F4.2 `goal verify` | W2; not repeated |
| F5.1 task-count progress | Leave→return row of §2; not repeated |
| F5.2 operating model puts Goal triage in OpenExec | Related to I2.1 (newer document, opposite direction); no duplicate fix |

The Console-side corrections owned by tasks 684d and 61e and the blocked
originals are not re-proposed. Every finding here is in native source or
native documents.

## Limitations

- **No layer-2 document exists** (I0). W1-W6 are report-local labels for K's
  workflows, not owner-accepted interaction design. If the owner writes a
  layer-2 document, these findings need re-checking against it.
- **The numbered invariants cover only the knowledge index.** No numbered
  interaction invariant exists to check.
- **Nothing ran against a live daemon, CLI binary or model.** Each I-finding
  is read from code. The tests that ran cover the paths that work.
- **L, G and C were not opened.** No finding here rests on them alone, so no
  finding is N↔L.
- **I2.1 is not resolved here.** Whether K or the 2026-09-27 status record
  reflects the owner's current assignment is outside this review's
  authority.
- **The Agent Console checkout is stale** (step 1). It was read only for the
  playbook rule and dependency order.
- **Fixes are out of scope.** The product findings are engineering defects
  inside accepted work and can be filed as native tasks without an owner
  question.

## Downstream

The playbook order (Console `internal/playbook/playbook.go:251-301`) is:
- drift (`DESIGN_REVIEW_DRIFT_2026-10.md`, on `origin/main`) → interaction
  (this report);
- → interface (`bb19d2e4…`), which depends on interaction;
- → accessibility (`869ef988…`);
- → the consolidating report (`8a800117…`).

Each keeps its dependency on the step before it. Step 4 will face the same
absence of a layer-3 document that I0 records for layer 2, since native
OpenExec's owner surface is a CLI and daemon API rather than screens; this
report does not decide how step 4 handles that.

Consolidated, re-checked at `131b9ce8`: [DESIGN_REVIEW_CONSOLIDATED_2026-10.md](DESIGN_REVIEW_CONSOLIDATED_2026-10.md).
