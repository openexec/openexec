# Design review 6/6 — consolidated findings, ordered by cost of leaving alone

**Date:** 2026-10-25. **Task:** `8a800117ba4225d07a97c5bc05111fc0` (sixth and
last step of the OpenExec `design-review` run).

**Source revision:** native `openexec` `origin/main`
`131b9ce8d3473c63c1aea1b0eab0f5b4dfa3db6b` (#141). This was current native
main when this pass started. Every `path:line` under "At 131b9ce8" is at this
commit. Main has since moved to `0e39075b` (#142). That commit changes only
`internal/server/graph_journey_prepare_test.go`, which no entry here cites, so
every citation also holds at `0e39075b`.

**This report is advisory. It grants no authority**: no implementation scope,
merge, deploy, spend, provider access or other effect. It does not rewrite any
intent, Goal, Ready, grouping or configuration. It is not evidence of Goal
Ready or of served behaviour. Each fix it describes would become a separate
native task through the normal route. Console-side corrections stay with
Console tasks 684d and 61e and the blocked originals, which this report only
references.

## The five reports merged here

| Step | Report | Merged as | Revision it reviewed |
|---|---|---|---|
| 1 | [DESIGN_REVIEW_INTENT_2026-10.md](DESIGN_REVIEW_INTENT_2026-10.md) | `f3e8d850` (#127) | `35e2125b` |
| 2 | [DESIGN_REVIEW_DRIFT_2026-10.md](DESIGN_REVIEW_DRIFT_2026-10.md) | `72b6d53f` (#128) | `f3e8d850` |
| 3 | [DESIGN_REVIEW_INTERACTION_2026-10.md](DESIGN_REVIEW_INTERACTION_2026-10.md) | `c8a3532b` (#129) | `72b6d53f` |
| 4 | [DESIGN_REVIEW_INTERFACE_2026-10.md](DESIGN_REVIEW_INTERFACE_2026-10.md) | `0a6849bb` (#138) | `5589b5e1` |
| 5 | [DESIGN_REVIEW_ACCESSIBILITY_2026-10.md](DESIGN_REVIEW_ACCESSIBILITY_2026-10.md) | `2c4bdd9d` (#140) | `ebf32d3e` |

**Finding IDs** keep the names their reports gave them. Two names collide.
Step 1's `U1`-`U7` and `X1`-`X5` are written here as `R1:U1` … `R1:X5`, so
they are not confused with step 4's `U1`-`U5`. Every other ID is unique across
the five reports and is written as is.

## How to read an entry

Each ranked entry has the same seven fields:

- **Observation.** What the code or document does, as the review found it,
  and what (if anything) has changed since.
- **Clause.** What the observation breaks. Every clause carries one of these
  labels:
  - **[committed]**: a clause in a file on `origin/main` at `131b9ce8`.
  - **[owner contract]**: rests only on the uncommitted intent (owner
    checkout `PROJECT_INTENT.md`, labelled L in step 1) or on the Console
    project contract sourced from it (C). Step 2 called these findings N↔L.
    Supervisor guidance of 2026-10-25 treats the accepted owner contract as
    authoritative alongside committed sources. The label stays so the reader
    can see the clause is not in a committed native file.
  - **[preference: WCAG 2.2]**: the WCAG 2.2 A/AA baseline step 5 chose. No
    committed native clause states it except where CR:389-393
    (`ui/src/components/chat/README.md`) is also cited.
  - **[preference]**: no clause; a convention or engineering judgement.
- **Benefit.** What fixing it gains.
- **Trade-off.** What fixing it costs or risks.
- **Confidence.** The label the source review used, unchanged:
  - **read from code**: nothing was run;
  - **run by hand**: the reviewer ran a probe or computed a value, outside the
    declared checks;
  - **checked automatically**: a repository test or declared check asserts
    it.

  This pass upgrades none of them. Re-reading a citation at `131b9ce8` is
  reading code, not running it.
- **At 131b9ce8.** The re-anchored `path:line`, and whether the observation
  still holds. Where a finding is an absence, the entry names the search
  instead of a line. "Not re-checked" marks an observation that needs a browser
  or a terminal to re-observe.

**Clause sources** use the earlier reports' labels: **K** =
`docs/OPENEXEC_SIMPLE_LOOP_ARCHITECTURE_CONTRACT.md` (unchanged since
`593a9e18`); **AG** = `AGENTS.md`; **CR** = `ui/src/components/chat/README.md`;
**AR** = `docs/API_REFERENCE.md`; **EP** = `docs/EXECUTION_PROVIDER_CONTRACT.md`;
**TA** = `docs/architecture/TOOL-ACTION-CONTRACT-V1.md`; **LM** =
`docs/LIGHT_MODE.md`; **A12** = `docs/architecture/ADR-012-APPROVAL-HUMAN-GATE.md`;
**KV3** = `docs/KNOWLEDGE_V3_PLAN.md`; **N** = committed `PROJECT_INTENT.md`.
Step 3's workflow labels W1-W6 are short names for K's workflows (W1
K:18-28, W2 K:30-42, W3 K:44-49, W4 K:51-60, W5 K:62-80, W6 K:82-86).

## Cost basis

One cost axis can hide the difference between how far a defect reaches and
how bad it is, so each tier states its basis. Within a tier, entries are
ordered by severity first, then by reach.

| Tier | What leaving it alone costs |
|---|---|
| 1. Authority and safety | An effect happens without the authority the owner holds, or a surface reports approval or success that never happened. This breaks the constraint "Consequential effects remain policy-controlled" [owner contract] and K:51-60. One occurrence can be irreversible. |
| 2. The loop needs the owner to keep it moving | Each ordinary failure, Stop or restart becomes owner scheduling work. This works against the project's stated purpose (the owner is not the "scheduler, task manager or execution bottleneck" [owner contract]) and K's W1/W5/W6. The cost repeats on every run. |
| 3. Contradicted non-goals and false signals | The owner or an agent reads a signal that is wrong (progress, intent, contracts, freshness, help text) and decides on it. No effect happens directly. |
| 4. Accessibility (WCAG 2.2 A/AA) | Users who rely on keyboard, assistive technology or contrast are blocked or misled on the web UI or TUI. Reach is limited today because the web chat's main path does not reach the daemon (entries 3 and 17). |
| 5. Documents | Agents and reviewers have nothing to check against, or follow a stale contract. The cost is in future work, not current behaviour. |

## Ranked findings

### Tier 1 — Authority and safety

**1. An agent with full shell access can approve its own work** — M1, F1.1
- Observation: infra apply tools need `danger-full-access`. In that mode
  `run_shell_command` is unrestricted and runs through `bash -c`, so the same
  agent session can run `openexec approve yes <id> --local`. It can also start
  an MCP server with `OPENEXEC_OPERATOR_SESSION=1`. The approval MCP tools are
  gated, but the shell route around them is not (M1). Separately,
  `openexec approve` without `--local` calls the daemon's approvals route,
  which the daemon never wires, so it can only return 503. ADR-012:21 says
  workspace writes are "Gated by local approval prompts" (F1.1). F1.1 merges
  here because both are about where the approval boundary actually sits.
- Clause: [committed] A12:25 "The agent is physically blocked from calling any
  approval-bypass tools"; LM:127 "an agent must never be able to approve its
  own requests"; AG:254 "agent sessions must never self-approve". [owner
  contract] "Consequential effects remain policy-controlled."
- Benefit: owner sign-off on infra applies becomes a real boundary. The
  `approve` command stops advertising a path that always fails.
- Trade-off: separating sessions (for example, refusing `approve` and the
  operator environment variable inside agent-spawned shells, or holding the
  approvals DB outside the agent's writable roots) adds friction to operator
  tooling. Each mechanism needs a negative test. The missing daemon gate is
  not itself drift: routine workspace writes should not need the owner
  ([owner contract] non-goal "Owner approval of navigator-created tasks").
  The fix for F1.1 is to correct ADR-012:21 and the command's default form.
- Confidence: read from code. No test covers the shell route. Step 4 did not
  try it.
- At 131b9ce8 (still holds): `internal/mcp/broker.go:195-197`, `:233-236`;
  `internal/mcp/server.go:208`, `:917`; `internal/cli/approve.go:184-186`;
  `internal/server/server.go:287` (no `WithApprovalGate`);
  `pkg/api/handlers.go:1030-1031` (503).

**2. Run handlers default to full access, while the API reference says
workspace-write** — A3, I3.2 remainder
- Observation: five daemon handlers set `mode = "danger-full-access"` when the
  caller sends no mode: create run, start run, parallel runs, blueprint run
  and session run. #135 fixed the chat CLI, which now always sends a mode, so
  chat no longer reaches the default. Any other caller that omits `mode` still
  gets the most permissive mode.
- Clause: [committed] AR:226 "`mode` | string | `workspace-write`". [owner
  contract] "Consequential effects remain policy-controlled."
- Benefit: a caller that trusts the reference gets the documented,
  least-surprising mode.
- Trade-off: changing the default may break callers that relied on full
  access silently. Changing AR instead keeps the permissive default. Step 4
  left the choice to the engineering task; a restrictive default with an
  explicit opt-in is the conventional answer.
- Confidence: read from code.
- At 131b9ce8 (still holds): `pkg/api/handlers.go:133-136`, `:202-205`,
  `:582-585`, `:666-669`, `:886-891`; `docs/API_REFERENCE.md:226`.

**3. The chat UI shows a tool as approved without the server ever confirming
it** — U1
- Observation: the web chat sends the owner's text as `send_message`, and
  approvals as `approve_tool` and `reject_tool`. The daemon's `/ws` handles
  neither and answers with a `notice`, which the UI only logs. Before sending,
  the UI marks the tool call `approvalStatus: 'approved'`, and nothing ever
  confirms or reverses that. Step 4 rated this its only high-severity finding.
- Clause: [committed] CR:372-378 (client→server protocol), CR:400 (optimistic
  approval updates presuppose a confirming `tool_call_update`).
- Benefit: the chat screen's main job, sending the owner's message, works. An
  ignored approval no longer looks approved.
- Trade-off: either the daemon implements CR's protocol or the client moves to
  the daemon's. Both touch the UI's mocked tests, which currently mock CR's
  protocol and so pass.
- Confidence: read from code. No Go test drives `/ws`.
- At 131b9ce8 (still holds): `ui/src/hooks/useChat.ts:445-446`, `:465`,
  `:467-473`; `pkg/api/ws.go:195-201`; `ui/src/hooks/useWebSocket.ts:331-332`.

**4. `apply_patch` reports success without applying anything** — T2
- Observation: the action checks that an `implementation` input is present,
  then returns `completed` with "Applied implementation changes to workspace".
  The comment says "Placeholder". No built-in blueprint uses it, so only a
  custom blueprint reaches it.
- Clause: [committed] TA:50 "`apply_patch` | Apply a structured patch artifact
  to the workspace. | `patch_applied`".
- Benefit: no stage can record a success that did not happen. That matters
  because evidence, not activity, decides Ready.
- Trade-off: making it fail closed (or unregistering it) is small. Building
  it properly is the larger TA question (entry 19).
- Confidence: read from code.
- At 131b9ce8 (still holds): `internal/actions/apply_patch.go:35-39`.

**5. Completion rules are enforced in one entry point only** — M2, M3
- Observation: `backlog_complete_story` refuses to complete `US-MAINT`, but
  `release.Manager.CompleteTask`'s auto-merge sets a complete story done with
  no maintenance check. That path is opt-in, behind `auto_merge_stories` (M2).
  `backlog_complete_task` sets a task done without checking the
  verification its contract names (M3).
- Clause: [committed] AG:241 (`US-MAINT` "can never be completed"); LM:44
  "Mark a task done after its verification passes".
- Benefit: the same rule holds whichever entry point is used.
- Trade-off: the M2 fix is small. For M3, LM does not say whether the tool or
  the client verifies. Either the wording changes, or the tool runs or checks
  the verification, which adds latency to light-mode completion.
- Confidence: read from code.
- At 131b9ce8 (still holds): `internal/mcp/backlog.go:532`, `:568-570`;
  `internal/release/automerge.go:71-72`, `:251`.

**6. Dead exported code still skips permissions** — E4
- Observation: `AgentCLIExecutor` accepts only `danger-full-access` and runs
  Claude with `--dangerously-skip-permissions`. It is not a `Provider`, so EP's
  statement holds for the two providers. No non-test caller exists outside
  `cli.go`.
- Clause: [committed] EP:77 "Neither provider uses a dangerous permission
  bypass."
- Benefit: removes an exported, ready-made bypass that a future caller could
  pick up.
- Trade-off: deleting it may break an external importer of `pkg/execution`.
  None is visible in this workspace.
- Confidence: read from code.
- At 131b9ce8 (still holds): `pkg/execution/cli.go:34-37`, `:49`.

**7. The working directory is not validated** — E1, E2
- Observation: `validateProviderRequest` checks only that the working
  directory is non-empty. It does not make it absolute, clean it, resolve
  symlinks or check the model (E1). `GatewayToolExecutor.ValidateAccess` never
  reads `workingDir` (E2), though it does refuse anything but read-only, which
  limits E2's reach.
- Clause: [committed] EP:15 ("canonical working directory … validated before
  starting"); EP:84-86 ("must validate the working directory").
- Benefit: the sandbox boundary is computed from a canonical path, not
  whatever string arrived.
- Trade-off: canonicalising can reject paths existing callers send, such as
  relative ones. Severity is low in step 4. The entry ranks in tier 1 because
  it is a sandbox boundary, and last because nothing shows it being bypassed.
- Confidence: read from code.
- At 131b9ce8 (still holds): `pkg/execution/agent_cli_provider.go:237-239`;
  `pkg/execution/gateway.go:97-105`.

### Tier 2 — The loop needs the owner to keep it moving

**8. `openexec run` uses the old batch scheduler, which never repairs** — I1.1
- Observation: `run` posts `runs:execute` without `task_oriented`, so
  `ExecuteTasks` takes the legacy path. A failed task is set to `"error"`, a
  status that is not a `TaskStatus`. Its dependents cascade to `"error"`, and
  no repair task is created. Only `pending` tasks are selected again, so the
  work stays stopped. The task-oriented queue that does repair (step 3's tests
  pass for it) is reachable only over HTTP. #133 added task selection to the
  legacy path but did not change its failure handling. Mixing `task_ids` with
  `task_oriented` is refused.
- Clause: [committed] K:20-28 (W1: "Ordinary failures mean more work"; "native
  tasks remain the durable truth of unfinished work").
- Benefit: an ordinary failure becomes a repair task instead of stopped work
  the owner has to notice and reset.
- Trade-off: `docs/TASK_ORIENTED_GOAL_ROUTE.md:130-133` keeps the legacy route
  "for compatibility" until equivalence is proven. Switching `run` needs that
  equivalence evidence and the compatibility gate.
- Confidence: read from code. The working route was checked automatically by
  step 3's listed tests.
- At 131b9ce8 (still holds): `pkg/manager/scheduler.go:222-224`, `:339-348`,
  `:489-491`; `internal/cli/start.go:357-363`.

**9. Stop is recorded as an error and cascades; only a full reset brings work
back** — I5.1
- Observation: a run the owner stops sends "task … stopped manually" into the
  error channel. The task is set to `"error"` and its dependents cascade. No
  command reopens one task: `openexec release reset` resets every story and
  task, including done ones. Stop exists only per run over the WebSocket or
  for the whole daemon. There is no HTTP stop route.
- Clause: [committed] K:72-73 (W5 "observe explicit Stop"); K:82-85 (W6:
  Stop/Resume is owner attention; retries and continuation are not).
- Benefit: Stop means "pause this", not "fail this and everything after it".
- Trade-off: needs a distinct stopped status on the legacy path, or the move
  in entry 8. A per-task reopen command adds CLI surface.
- Confidence: read from code.
- At 131b9ce8 (still holds): `pkg/manager/scheduler.go:479-483`, `:489-491`,
  `:339-348`; `internal/release/manager.go:153-165`.

**10. Nothing continues after a daemon restart** — I5.3
- Observation: `Server.Start` never calls `ExecuteTasks`, so work resumes only
  when someone runs `openexec run` again. The restart clean-up resets tasks in
  `running` or `starting`, statuses no non-test code writes, so that code is
  dead. Both routes recover from native tasks, not runs, which is what W5
  asks; the gap is that someone has to start them.
  `docs/AUTONOMOUS_HUMAN_BOUNDARIES_IMPLEMENTATION_STATUS.md:56-58` already
  lists restart recovery as remaining work.
- Clause: [committed] K:72-74 (W5); K:84-85 (W6, continuation is not owner
  work).
- Benefit: leaving and returning is safe without a manual restart step.
- Trade-off: auto-continuation on start must respect Stop (entry 9) and
  delegated budgets. Otherwise it would resume work the owner deliberately
  stopped.
- Confidence: read from code. The task-oriented reconciliation was checked
  automatically (`TestFreshTaskQueueReconcilesOnlyScopedInterruptedWork`, step
  3).
- At 131b9ce8 (still holds): `internal/server/server.go:670-742`;
  `pkg/manager/manager.go:214`.

**11. Planning ignores current state and repeats on every bare `run`** — I2.2
- Observation: a bare `run` re-plans every time unless `--no-auto-plan` is
  passed. Since #133, `run <id>` skips planning. The planner's input is the
  intent file alone (`GeneratePlan(ctx, intent, nil)`); no repository or
  backlog state enters generation. Import reconciliation remaps colliding
  IDs afterwards. `plan`'s help says `run` plans "if your plan is missing or
  stale", but no such check exists.
- Clause: [committed] K:39-42 (W2: brownfield plans only missing work;
  planning at meaningful boundaries); K:20 (W1 "Intent/current state").
- Benefit: repeated runs stop paying for, and risking duplicates from, a full
  re-plan.
- Trade-off: a staleness check needs a definition of "stale" (intent hash,
  backlog state). Feeding repository state to the planner raises prompt size.
- Confidence: read from code. Whether a repeated `run` actually duplicates
  work depends on model output and was not run.
- At 131b9ce8 (still holds, narrowed): `internal/cli/start.go:313`;
  `pkg/manager/planner.go:132`, `:146`; `internal/cli/plan.go:30`.

**12. No independent plan review on the `run` or `plan` route** — I1.3
- Observation: plan review is opt-in (`Review bool … Opt-in`), and the
  review-and-refine loop is reached only with a `request_id`. `run` sends
  only `intent_file` and `auto_import`. `plan` sends `IntentFile`,
  `NoValidate` and `AutoImport`.
- Clause: [committed] K:20-21 (W1 "independent task-plan review → repair
  incomplete plans").
- Benefit: incomplete plans are repaired before they become failing tasks.
- Trade-off: one more model call per plan. With entry 11 unfixed, that cost
  is paid on every bare `run`.
- Confidence: read from code.
- At 131b9ce8 (still holds): `pkg/manager/planner.go:34`, `:64-65`,
  `:160-173`; `internal/cli/start.go:314-317`; `internal/cli/plan.go:66-68`.

**13. Blueprint, session and chat runs leave no task record** — I3.3
- Observation: `runs:blueprint` and the session-run route create a run ID and
  call `Start`. `Manager.start` writes a `runs` row and no task row. Since
  #135, chat escalation goes through the session route, so chat runs also
  leave no task. A failed or interrupted run of this kind leaves nothing to
  resume.
- Clause: [committed] K:27-28 (W1 "native tasks remain the durable truth of
  unfinished work"); K:46-47 (W3).
- Benefit: every unit of work the owner started survives as resumable state.
- Trade-off: creating a task per blueprint or chat run adds rows to the
  backlog and interacts with phase computation. A maintenance-style story
  (as for `US-MAINT`) is one existing primitive to reuse.
- Confidence: read from code.
- At 131b9ce8 (still holds, wider than reported): `pkg/api/handlers.go:659`,
  `:690`, `:908`, `:941`; `pkg/manager/manager.go:523`.

**14. "Resume" restarts runs from the beginning** — I5.2
- Observation: `POST /runs/{id}/resume` sets a `ResumeFrom` checkpoint that no
  non-test code reads. The WebSocket `resume` calls `Start`, which refuses a
  pipeline that has not finished, including a paused one. The scheduler
  orders interrupted runs first "for resume" and then starts them fresh.
- Clause: [committed] K:62-80 (W5). W5 does not require checkpoint resume
  ("Runs are attempts", K:27), so the defect is the advertised behaviour.
- Benefit: the route name and the behaviour agree.
- Trade-off: either rename and document "restart", which is cheap, or
  implement checkpoint resume, which K does not ask for.
- Confidence: read from code.
- At 131b9ce8 (still holds): `pkg/manager/manager.go:314-319`, `:364-365`;
  `internal/pipeline/pipeline.go:72-73`; `pkg/api/ws.go:233-235`.

**15. Merges are checked by branch name only, and completion paths differ** —
I4.1, F2.3, F3.4
- Observation: `MergeStoryToRelease` checks that two branches exist and merges
  with `--no-ff`. No head SHA, check result or review is consulted (I4.1).
  The daemon's release manager has Git disabled, so this is reached only
  through `openexec task complete` with auto-merge on. `task complete`
  auto-merges, while `backlog_complete_task` only sets status, so the same
  completion merges or not depending on the entry point (F2.3). The opt-in
  `approval_enabled` setting puts every machine-created story and task in
  `ApprovalPending` and refuses merge until approval (F3.4). F2.3 and F3.4
  merge here because all three describe the native merge path.
- Clause: [committed] K:53-55 (W4: candidate provenance → required checks →
  independent review). [owner contract] non-goal "Owner approval of
  navigator-created tasks" (F3.4); the authority interpretation reserves
  merge to the owner.
- Benefit: no native path can merge an unreviewed candidate, and completion
  behaves the same everywhere.
- Trade-off: all three paths are opt-in and off by default. Removing native
  auto-merge in favour of the external exact-head delivery route is the
  simplest fix, but it drops a feature someone may use.
- Confidence: read from code.
- At 131b9ce8 (still holds): `internal/git/tracker.go:294-345`;
  `internal/release/automerge.go:47-49`, `:227`; `internal/cli/release.go:1358`;
  `internal/mcp/backlog.go:532`; `internal/release/manager.go:75`,
  `:519-523`, `:882-885`.

### Tier 3 — Contradicted non-goals and false signals

**16. Owner-facing progress is a task-count percentage, and "done" has three
definitions** — F5.1, F4.5
- Observation: `openexec status` prints "Progress: %d/%d tasks completed
  (%.1f%%)". `openexec tui` shows a percentage computed as
  `(completed * 100) / total` (F5.1). API progress counts `Delivered`, `story
  list` counts `done` only, and the TUI counts `completed`, which is not a
  `TaskStatus` (F4.5). F4.5 merges here because it feeds the same number.
- Clause: [owner contract] non-goal "Progress percentages derived from task or
  agent counts". [committed] K:33 ("Do not infer Ready from task
  completion"); `pkg/outcome/evaluator.go:252` ("Do not infer progress from
  activity, tasks …").
- Benefit: removes the clearest native contradiction of a declared non-goal.
  Return reviews show state, not a count.
- Trade-off: the owner loses a familiar number. An honest replacement
  (outcome state, next runnable task, blocked reason) needs design.
- Confidence: read from code.
- At 131b9ce8 (still holds): `internal/cli/status.go:175`;
  `internal/tui/app.go:166`; `internal/tui/file_source.go:214`, `:272-283`;
  `internal/release/failure_repair.go:189-193`; `internal/cli/release.go:653`.

**17. The chat UI and the daemon speak different message formats** — U2, U3
- Observation: the daemon sends only `event`, `step`, `pong` and `notice`. The
  UI's `message`, `streaming_chunk`, `tool_call_update` and `error` handlers
  never fire (U2). The UI moves loop state only on `loop.*` and
  `iteration.start`, while the daemon sends `iteration_start`, `paused`,
  `complete` and so on. No Go `LoopEventType` exists, though `types/chat.ts`
  says it "mirrors Go LoopEventType" (U3). The status badge and the action
  buttons therefore never move.
- Clause: [committed] CR:380-385 (server→client messages); CR:104-112 (badge
  and actions follow loop state); `ui/src/types/chat.ts:192`.
- Benefit: the chat view shows real run state. It is a prerequisite for the
  announcements in entry 33.
- Trade-off: the same protocol decision as entry 3. Fix them together.
- Confidence: read from code.
- At 131b9ce8 (still holds): `pkg/api/ws.go:173`, `:198`, `:215`, `:239`;
  `ui/src/hooks/useWebSocket.ts:286-314`; `internal/loop/event.go:7-52`;
  `ui/src/types/chat.ts:192`.

**18. "Intent" names two files, and the planner reads only the generated
one** — F4.1
- Observation: the planner's input is `INTENT.md`, written by `wizard`. No
  non-test Go file references `PROJECT_INTENT`. README calls the step
  "Define goal, generates INTENT.md". ADR-005:36-42 already records the split
  as target direction. Under K, Console owns Intent (K:67), so the native
  planner need not read `PROJECT_INTENT.md` itself. The word still names two
  artefacts with different authority.
- Clause: [committed] N:23 "split intent into tasks"; K:20 "Intent/current
  state → native task generation".
- Benefit: one meaning of "intent". Either the accepted intent flows into
  planning through the Console boundary, or the generated file is renamed.
- Trade-off: renaming touches the CLI, README and existing projects'
  `INTENT.md`, which is compatibility-sensitive (Anti-Regression Policy).
- Confidence: read from code.
- At 131b9ce8 (still holds): `internal/cli/start.go:315`;
  `internal/cli/plan.go:33`; `pkg/manager/planner.go:76`;
  `internal/cli/wizard.go:120`; `README.md:202`.

**19. Blueprint actions do not match their contract** — T1, T3, T4
- Observation: TA lists seven v1 actions. The registry holds five different
  ones, with only `apply_patch` in common, and no built-in blueprint uses it.
  A blueprint naming a TA action such as `run_tests` fails closed (T1). TA's
  request and result envelopes, policy step, statuses and timeout classes are
  not what the code implements (T3). The standard blueprint's `lint` and
  `test` stages are shell commands from project configuration, which TA calls
  an anti-pattern (T4). They are operator-owned and recorded by evidence
  hooks, so they are not opaque in the unrecorded sense.
- Clause: [committed] TA:45-55 (actions), TA:16-43 and TA:64 (lifecycle,
  envelopes, policy, timeouts), TA:12 and TA:67 (no shell snippets).
- Benefit: an agent writing a blueprint from TA gets stages that run.
- Trade-off: rewriting TA to describe what exists is cheap. Building TA as
  written is a large change. Neither changes runtime behaviour except T1's
  fail-closed refusal.
- Confidence: read from code.
- At 131b9ce8 (still holds): `internal/actions/registry.go:51-58`;
  `internal/blueprint/executor.go:113-129`, `:141-142`, `:179-183`;
  `internal/types/types.go:27-32`; `internal/pipeline/pipeline.go:484-491`.

**20. Knowledge answers without a current generation** — I6.2 remainder, I6.1
- Observation: #141 stopped serving `failed` and `building` generations from
  the fast path. A `partial` generation is still served, labelled
  `Freshness: partial`. KV3:116 says any non-current generation needs a typed
  refusal. `REPOSITORY_POINTER_GRAPH_PLAN.md:585` allows a partial graph with
  disclosed gaps. The two documents disagree (I6.2 remainder). After a
  refresh, the active generation is returned without re-checking its status.
  That is step 3's agent's inference, not traced. The DCP coordinator's symbol
  tool and `GET /api/v1/knowledge/symbols` read the legacy `symbols` table
  with no generation or freshness check at all, and the HTTP route ignores
  errors (I6.1). Both are DCP-only.
- Clause: [committed] KV3:116-117 (INV-K1).
- Benefit: agents never act on an index that the invariant says must refuse.
  One document states the partial-graph rule.
- Trade-off: refusing partial graphs reduces availability on large
  repositories. Allowing them means amending KV3:116 instead. The DCP path is
  opt-in.
- Confidence: read from code.
- At 131b9ce8 (I6.2 changed by #141; I6.1 still holds):
  `internal/knowledge/graph_freshness.go:95-102`;
  `internal/knowledge/graph_store.go:169`, `:195-201`;
  `internal/cli/start.go:52`; `internal/tools/symbol_reader.go:66`, `:105`;
  `internal/server/server.go:420-422`, `:486-492`.

**21. The standard blueprint's fix stages are unreachable, and
`STATE_MACHINE.md` says they run** — I1.4
- Observation: `lint` and `test` name `fix_lint`/`fix_tests` as `OnFailure`
  but set no `MaxRetries`. The engine follows `OnFailure` only while retries
  are below `MaxRetries`, so a failed check fails the run. On the
  task-oriented queue a failure becomes a repair task instead. On the legacy
  route (entry 8) nothing repairs it.
- Clause: [committed] `docs/STATE_MACHINE.md:3`, `:23`, `:25`; K:21-22 (W1).
- Benefit: the document matches the engine, or in-run repair works on the
  legacy route.
- Trade-off: enabling retries adds model calls per failed check. Deleting the
  dead stages is simpler once entry 8 moves `run` to the repairing queue.
- Confidence: read from code.
- At 131b9ce8 (still holds): `internal/blueprint/engine.go:154-161`,
  `:171-179`, `:496`, `:507-509`.

**22. The configuration guide documents loop settings nothing reads** — F3.1
- Observation: CONFIGURATION.md documents `review.require_review`,
  `execution.auto_fix`/`max_fix_iterations`, `retry.max_attempts`,
  `mode.default`, `gitflow.auto_merge` and `parallel.auto_merge`. The only
  `openexec.yaml` schema parses `project:` and `quality:`. Retry and
  iteration limits are fixed defaults. #131 corrected only the gate sections.
- Clause: [committed] K:64-65 (OpenExec owns retries, repairs and
  continuation; they are not owner configuration).
- Benefit: an owner following the guide stops editing settings that have no
  effect.
- Trade-off: docs-only. Removing the entries is cheaper than wiring them, and
  K argues against owner-tunable loop policy anyway.
- Confidence: read from code.
- At 131b9ce8 (still holds): `docs/CONFIGURATION.md:63`, `:87-89`, `:99`,
  `:207`, `:253`, `:281`; `internal/execution/gates/config.go:13-16`;
  `internal/config/defaults.go:7-9`.

**23. `goal verify` claims to confirm goals but checks story status** — F4.2
- Observation: the help says "confirm that project goals have been met".
  Without `--execute` it decides from story status alone and prints
  "implementations-complete". These are planner `G-NNN` records, not the
  owner's Goal/Ready. Nothing routes Ready through the command.
- Clause: [committed] K:32-33 "Do not infer Ready from task completion".
- Benefit: the command stops reading as a Ready signal.
- Trade-off: help-text change only.
- Confidence: read from code.
- At 131b9ce8 (still holds): `internal/cli/release.go:1590-1593`, `:1635`,
  `:1653`.

**24. `run --max-iterations` and `run --timeout` are still never read** — F2.1
remainder
- Observation: #130 made `--quickfix` and `--verify` refuse before anything
  runs. The two other dead flags step 2 named are still declared, and nothing
  reads them.
- Clause: [committed] K:44-49 (W3; flags that silently do nothing are the
  same false signal as F2.1's original).
- Benefit: no flag pretends to bound a run.
- Trade-off: removing them breaks scripts that pass them harmlessly. Refusing
  them, as #130 did, breaks the same scripts with a clear message.
- Confidence: read from code.
- At 131b9ce8 (partly fixed): `internal/cli/start.go:82-83`, `:917-918`.

**25. `init` and `wizard` route the owner through the deprecated `plan`** — F2.2
- Observation: `init` prints "Run 'openexec wizard'", then "Run 'openexec
  plan INTENT.md'", then "Run 'openexec run'". `plan` is labelled deprecated,
  and `run` already plans. `wizard` also points to `plan`. This pass found the
  `wizard` line while re-anchoring; it is part of the same finding.
- Clause: [committed] `internal/cli/plan.go:28` (the deprecation itself);
  K:44-49.
- Benefit: the first-run path names one command.
- Trade-off: output text only.
- Confidence: read from code.
- At 131b9ce8 (still holds): `internal/cli/init.go:254-256`;
  `internal/cli/wizard.go:125`.

**26. The stdio execution protocol can exit without a result envelope** — E3
- Observation: EP describes protocol version 1, but the code speaks 1-3. If
  building the provider fails, or an `execute` envelope has no request, the
  command returns a bare error with no result envelope.
- Clause: [committed] EP:54, EP:60, EP:63 ("exactly one result").
- Benefit: callers can always parse an outcome.
- Trade-off: small. The version wording is docs-only.
- Confidence: read from code.
- At 131b9ce8 (still holds): `internal/cli/execution_stdio.go:30-32`,
  `:280-283`, `:317-318`.

**27. The `stories.json` format has diverged between its writers and its
reader** — F4.6
- Observation: `release export` writes empty `goals`, while `ExportJSON`
  writes them. The planner's schema is `1.2`, and `story import` accepts only
  `1.0`, `1.1` or `legacy`. Only the explicit file import is affected.
- Clause: [committed] N:24-25 ("outcome is predictable").
- Benefit: export and import round-trip.
- Trade-off: touches a compatibility-sensitive migration path. It needs
  compatibility coverage.
- Confidence: read from code.
- At 131b9ce8 (still holds): `internal/cli/release.go:69-70`, `:927`;
  `internal/release/manager.go:1184-1203`; `internal/planner/planner.go:94`.

**28. Web UI surfaces that cannot work** — U5, plus two observations from
step 4's screens table (no IDs)
- Observation: the exported `restart/` and `diff/` components call
  `/restart/requests…` and `/backups…`, which no route serves, and no page
  mounts them (U5). The Knowledge Hub nav button always shows, but its routes
  exist only with DCP on, so without DCP the page shows "Some knowledge
  endpoints failed to load". Replay is a nav item whose view is a placeholder.
- Clause: U5 has no clause and is labelled a document gap by step 4.
  Knowledge Hub and Replay: [preference] (step 4: "no clause says a
  placeholder may not ship").
- Benefit: the nav offers only what works.
- Trade-off: hiding Knowledge Hub without DCP needs a capability signal from
  the daemon. Deleting dead components loses work someone may want to revive.
- Confidence: read from code.
- At 131b9ce8 (still holds): `ui/src/hooks/useRestartApproval.ts:171-372`;
  `ui/src/hooks/useBackupHistory.ts:196`, `:241`; `ui/src/App.tsx:15-16`,
  `:99-104`, `:111-116`; `internal/server/server.go:420-424`, `:430-438`.

**29. The TUI opens SQLite without `sqlitecfg.DSN`** — TU3
- Observation: `sql.Open("sqlite", dbPath+"?mode=ro")` runs with no busy
  timeout. Any error makes the TUI fall back to `tasks.json` or show no
  progress, silently. `internal/cli/symbol_index.go:79` does the same.
- Clause: [committed] AG:289 ("always open via `sqlitecfg.DSN(path)` … fixed
  across all open sites").
- Benefit: the declared repository rule holds everywhere, and the TUI does
  not silently show wrong or empty progress under write contention.
- Trade-off: two-line change per site. Read-only mode must be expressed in
  modernc syntax.
- Confidence: read from code.
- At 131b9ce8 (still holds): `internal/tui/file_source.go:265`;
  `internal/cli/symbol_index.go:79`.

**30. INV-K4's scope over existing compilers is unstated** — I6.5
- Observation: the TypeScript compiler runs as argv with no shell, so the
  argv half holds. Nothing in `internal/knowledge` imports `internal/infra`.
  Whether the `internal/infra` half applies to compilers already running, or
  only to future third-party indexers, is not stated.
- Clause: [committed] KV3:122-123 (INV-K4).
- Benefit: the invariant can be checked.
- Trade-off: docs-only, unless the answer is "applies", in which case the
  compiler call moves under `internal/infra`.
- Confidence: read from code.
- At 131b9ce8 (still holds): `internal/knowledge/ts_compiler.go:97`, `:196`.

### Tier 4 — Accessibility (WCAG 2.2 A/AA)

The missing automated check comes first, because every later accessibility
fix regresses silently without it.

**31. No automated accessibility check, and the "Accessibility" e2e tests
assert nothing** — A0.1, A0.2
- Observation: ESLint has no `jsx-a11y`, and there is no axe package. No unit
  test asserts focus, live regions or reduced motion (A0.1). The generic
  Playwright "Accessibility" tests end in assertions a count always satisfies
  (`>= 0`), or never assert what they count. Neither `make test` nor CI runs
  that suite (A0.2).
- Clause: [committed] CR:389-393 (the declared accessibility clause has no
  check). [preference: WCAG 2.2].
- Benefit: the medium findings below cannot regress unnoticed once they are
  fixed.
- Trade-off: a new devDependency and lint rule set, so some existing code
  will fail lint on day one. Vacuous tests should be fixed or deleted, not
  kept as false assurance.
- Confidence: read from code; the absence was confirmed by search.
- At 131b9ce8 (still holds): `ui/eslint.config.js:1-5`;
  `ui/package.json:26-47`; `ui/e2e/app.spec.ts:74`;
  `ui/e2e/multi-project-sessions.spec.ts:645`, `:668-671`;
  `ui/e2e/provider-agnostic-loop.spec.ts:1150`; `Makefile:60-86`.

**32. The focus indicator is removed from the chat input and 12 other
controls** — WF5
- Observation: inline `outline: 'none'` overrides the global `:focus-visible`
  outline on 13 controls, including the chat textarea, the project combobox
  and the session search. Step 5 measured `outline: none 0px` on keyboard
  focus for three of them.
- Clause: [preference: WCAG 2.2] 2.4.7 Focus Visible (AA). [committed] CR:389.
- Benefit: keyboard users can see where they are on the view's primary
  control.
- Trade-off: visual change on 13 controls.
- Confidence: run by hand (Chromium probe, step 5), backed by reading code.
- At 131b9ce8 (still holds): `ui/index.html:36-39`;
  `ui/src/components/chat/input/InputTextarea.tsx:124`. The other sites are
  listed in step 5 and are at the same lines.

**33. Nothing in the chat UI is announced; there is no live region** — WR1
- Observation: there is no `aria-live`, `role="status"` or `role="log"` in
  any `.tsx`. Streamed output, new messages, loop-status changes, approval
  errors and new events are not announced. Against a real daemon there is no
  streamed text yet (entry 17), but the gap remains once that is fixed.
- Clause: [preference: WCAG 2.2] 4.1.3 Status Messages (AA). [committed]
  CR:392.
- Benefit: screen-reader users learn that the agent replied or failed.
- Trade-off: chatty announcements are their own barrier, so they need
  throttling.
- Confidence: read from code; the probe counted zero live regions (run by
  hand).
- At 131b9ce8 (still holds): `ui/src/pages/KnowledgeHub.tsx:100` (the only
  `role="alert"`); `ui/src/components/chat/blueprint/ApprovalsPanel.tsx:241-246`.

**34. Several form controls have no programmatic name or label** — WS3
- Observation: the filter toggle is an unnamed `button`. The Status and Sort
  comboboxes have labels that are not associated with them. The new-session
  modal's labels are not associated with their controls. The Project
  Wizard's close button is named only "×". Some fields are named only by
  placeholder.
- Clause: [preference: WCAG 2.2] 4.1.2, 1.3.1, 3.3.2. [committed] CR:390.
- Benefit: assistive technology can say what each control is.
- Trade-off: small markup changes.
- Confidence: run by hand (ARIA snapshot) and read from code.
- At 131b9ce8 (still holds):
  `ui/src/components/chat/session/SessionFilters.tsx:91-99`, `:106-107`,
  `:121-122`; `SessionSidebar.tsx:211-212`; `ProjectWizard.tsx:125`.

**35. Muted text and white-on-colour buttons fall below contrast minimums** —
WC1, WC2, WC3
- Observation: `#6e7681` text at 11-12px is 4.12:1 and 3.77:1. The colour is
  used at many inline sites: step 5 counted 45, and this pass's re-check
  counted 46 in unchanged files. `#484f58` text is 2.28:1 (WC1). White on
  `#f0883e` (paused badge) is 2.53:1, and white on `#a371f7` (fork button) is
  3.35:1 (WC2). The chat textarea's only boundary is 1.55:1 (WC3).
- Clause: [preference: WCAG 2.2] 1.4.3 (AA), 1.4.11 (AA). [committed] CR:393
  "High contrast support for status indicators" (the paused badge).
- Benefit: readable secondary text and status.
- Trade-off: palette changes alter the look. Inline colours mean many edits.
  The unused token file (entry 44) is the natural single source.
- Confidence: run by hand (ratios computed from source hex values, not
  rendered pixels).
- At 131b9ce8 (still holds):
  `ui/src/components/chat/input/ChatInput.tsx:75`, `:103-106`;
  `ui/src/components/chat/header/LoopStatusBadge.tsx:35-37`;
  `ui/src/components/chat/session/SessionForkDialog.tsx:974-977`;
  `ui/src/components/chat/input/InputTextarea.tsx:120-121`.

**36. The fork dialog does not take, keep or return focus** — WF1
- Observation: it has correct `role="dialog"` and labelling, but the file
  contains no `focus` call. Focus stays on the trigger, Escape works only
  after tabbing in, Tab is not contained, and nothing returns focus on close.
- Clause: [preference: WCAG 2.2] 2.4.3 (A). [committed] CR:391.
- Benefit: the one mounted dialog behaves like a dialog for keyboard users.
- Trade-off: a focus-trap helper. Reuse one across entries 36, 37 and 41.
- Confidence: read from code. The probe could not open the dialog.
- At 131b9ce8 (still holds):
  `ui/src/components/chat/session/SessionForkDialog.tsx:190`, `:217-221`,
  `:348-352`.

**37. The other mounted modals have no dialog semantics, lose focus and ignore
Escape** — WF2, WK3
- Observation: the new-session modal, `ProjectInitModal` and `ProjectWizard`
  are plain `div` overlays with no role, no `aria-modal` and no focus
  handling. Focus ended on `BODY` after close (WF2). The new-session modal
  stayed open after Escape (WK3).
- Clause: [preference: WCAG 2.2] 2.4.3, 4.1.2. WK3: [preference] (no A/AA
  criterion requires Escape) and [committed] CR:389.
- Benefit: consistent modal behaviour.
- Trade-off: same helper as entry 36.
- Confidence: run by hand (new-session modal) and read from code (the other
  two).
- At 131b9ce8 (still holds):
  `ui/src/components/chat/session/SessionSidebar.tsx:204-205`;
  `ProjectInitModal.tsx:41-42`; `ProjectWizard.tsx:121-122`.

**38. The active view is shown only visually** — WS1
- Observation: the active nav button gets a colour and an underline. There is
  no `aria-current`, `aria-pressed` or `aria-selected`.
- Clause: [preference: WCAG 2.2] 1.3.1, 4.1.2.
- Benefit: screen-reader users know which view is open.
- Trade-off: one attribute per button.
- Confidence: run by hand (probe) and read from code.
- At 131b9ce8 (still holds): `ui/src/App.tsx:77-80`, `:94`.

**39. The TUI's selected row and help text have low contrast** — TC1, TC2
- Observation: the selected row is colour 255 on colour 39, 2.11:1 whatever
  the terminal theme (TC1). The help text uses `Dim` (colour 59), 3.29:1 on
  black, while the defined `Help` style (6.92:1) is unused (TC2).
- Clause: [preference: WCAG 2.2] 1.4.3 by analogy. The TUI has no committed
  accessibility clause.
- Benefit: the TUI's focus is readable.
- Trade-off: colour choice only. Terminal palettes vary.
- Confidence: run by hand (ratios from standard xterm values).
- At 131b9ce8 (still holds): `internal/tui/styles.go:25-29`, `:47-52`;
  `internal/tui/help.go:32`.

**40. Keyboard gaps: hover-only row actions and a clickable `div`** — WK1, WK2
- Observation: session row actions appear only on mouse hover, though the
  header's fork button covers the one wired action (WK1). Directory entries
  in the init picker are `div`s with only `onClick`, though a typed path field
  is offered (WK2).
- Clause: [preference: WCAG 2.2] 2.1.1 (A), 4.1.2. [committed] CR:389.
- Benefit: full keyboard parity.
- Trade-off: small. Both have workarounds today.
- Confidence: run by hand (WK1) and read from code (WK2).
- At 131b9ce8 (still holds):
  `ui/src/components/chat/session/SessionListItem.tsx:109-110`, `:138`;
  `DirectoryPicker.tsx:65-72`.

**41. `ApprovalsPanel` drops focus** — WF4
- Observation: Escape closes the focused reject field with no focus target. A
  resolved approval's focused button disappears the same way.
- Clause: [preference: WCAG 2.2] 2.4.3.
- Benefit: approval review by keyboard keeps its place.
- Trade-off: small.
- Confidence: read from code.
- At 131b9ce8 (still holds):
  `ui/src/components/chat/blueprint/ApprovalsPanel.tsx:139`, `:260-269`,
  `:372`, `:376-378`.

**42. Pointer targets: a covered toggle and undersized controls** — WF6, WT1
- Observation: at 1280×720 the filter toggle's centre is covered by the
  header's cost text. Keyboard focus still reaches it (WF6). "Init" and
  "Wizard" are 22px tall, and the search input is 15px (WT1).
- Clause: [preference: WCAG 2.2] 2.5.8 (AA); 2.4.11 is a candidate, not
  established.
- Benefit: reliable pointer use.
- Trade-off: layout spacing.
- Confidence: run by hand (probe). Only the centre point was measured, and the
  spacing exception was not evaluated.
- At 131b9ce8 (code unchanged; the measurement was not re-checked):
  `ui/src/components/chat/session/SessionFilters.tsx:72`, `:91-99`,
  `:188-196`; `ProjectSelector.tsx:96-102`.

**43. Session status is a coloured dot** — WS4
- Observation: visually, status is colour only. The text exists only in
  `title`, which assistive technology does receive.
- Clause: [preference: WCAG 2.2] 1.4.1 (A). [committed] CR:393.
- Benefit: colour-blind users can read status.
- Trade-off: small.
- Confidence: run by hand (accessible name) and read from code.
- At 131b9ce8 (still holds):
  `ui/src/components/chat/session/SessionListItem.tsx:92-99`, `:121-127`.

**44. Accessibility work that exists but is not wired** — WF3, WC0, WM1
- Observation: the unmounted `PatchPreviewModal`, `RollbackModal` and
  `ModelPicker` repeat the dialog focus defects (WF3). `variables.css`, with
  colour tokens and `--touch-target-min: 44px`, is imported nowhere (WC0).
  `usePrefersReducedMotion` is defined and used nowhere, and the `pulse`
  and `blink` animations it would govern have no keyframes (WM1).
- Clause: WF3 [preference: WCAG 2.2] 2.4.3. WC0 and WM1: [preference] (dead
  code; step 5 found no A/AA failure in WM1's evidence).
- Benefit: the pieces a fix to entries 35 and 37 needs already exist.
- Trade-off: decide whether to wire them or delete them, together with the
  dead surface in entry 28.
- Confidence: read from code.
- At 131b9ce8 (still holds):
  `ui/src/components/chat/diff/PatchPreviewModal.tsx:123`, `:140`;
  `ui/src/styles/variables.css:139`; `ui/src/hooks/useBreakpoint.ts:79-80`.

**45. Advisory structure and motion items** — WS2, WR2, and the TUI's
one-second refresh (step 5 §2.4, no ID)
- Observation: headings and landmarks differ by view. Knowledge Hub and Replay
  have no `main`, and the chat layout nests an unnamed `aside` inside another
  (WS2). Decorative SVGs are exposed as unnamed images (WR2). The TUI
  re-fetches and redraws every second with no pause control.
- Clause: [preference] (step 5: no A/AA criterion is failed; 2.2.2 may apply
  to the TUI by analogy).
- Benefit: cleaner screen-reader output.
- Trade-off: low value until entries 31-38 are done.
- Confidence: run by hand (probe, WS2 and WR2) and read from code (TUI).
- At 131b9ce8 (still holds): `ui/src/pages/KnowledgeHub.tsx:95`;
  `ui/src/components/chat/messages/StreamingMessage.tsx:52-60`;
  `internal/tui/app.go:78-79`, `:262-265`.

### Tier 5 — Documents

**46. Documents that contradict the controlling loop contract** — I2.1, F5.2,
F5.3
- Observation: K assigns independent Goal review, outcome lifecycle and
  scoped Stop to Agent Console. AG tells agents to read K before changing
  these loops. `docs/AUTONOMOUS_HUMAN_BOUNDARIES_IMPLEMENTATION_STATUS.md`
  (2026-09-27) says that assignment is "superseded" and plans to move Goal
  review, Stop policy and restart recovery into OpenExec (I2.1). The
  2026-08-17 operating model plans Goal triage and Done authority inside
  OpenExec (F5.2). ADR-019 (Proposed, aspirational) makes a Git-backed second
  backlog and agent merges to `main` the `init` default (F5.3).
- Clause: [committed] K:27-28, K:30-37, K:67-69; AG "Autonomy architecture"
  (K is controlling). [owner contract] authority interpretation (merge is
  the owner's).
- How it is resolved here: supervisor guidance of 2026-10-25 states that
  the owner's recorded architecture assigns native planning, execution and
  repair to OpenExec, the outer Goal review to Console and known delivery to
  deterministic code, and that the current AG/K contract controls. A contrary
  document is a documentation finding, not an open product question. Step 3
  left I2.1 unresolved because that guidance did not exist yet.
- Benefit: an agent following AG builds against one ownership model.
- Trade-off: docs-only. The status record's other content stays useful
  history, so mark it superseded rather than deleting it.
- Confidence: read from code (documents).
- At 131b9ce8 (still holds): `docs/OPENEXEC_SIMPLE_LOOP_ARCHITECTURE_CONTRACT.md:30-37`;
  `docs/AUTONOMOUS_HUMAN_BOUNDARIES_IMPLEMENTATION_STATUS.md:5-8`, `:56-58`;
  `docs/EXPERIENCE_FIRST_OPERATING_MODEL.md:899`, `:913`, `:955`;
  `docs/architecture/ADR-019-GIT-BACKED-BACKLOG-SCAFFOLDING.md:5`, `:25`, `:57`.

**47. No jobs, interaction or screen document, and no declared UI
anti-patterns** — D0, I0, S0.1, S0.3
- Observation: native `origin/main` has no layer-1 jobs document (D0), no
  layer-2 workflow or interaction-invariant document (I0) and no layer-3
  screen document (S0.1). No anti-pattern is declared for the UI or the API;
  the explicit prohibitions that do exist are in TA, EP, A12, LM and AG
  (S0.3). Steps 2-4 therefore checked against K and component contracts.
- Clause: [preference] (review method). The review playbook expects these
  layers; no committed native clause requires them.
- Benefit: later reviews and agents have a declared target. Today every
  finding in tiers 2-4 rests on the nearest available statement.
- Trade-off: writing jobs is owner-authored destination work, which machines
  may draft but not adopt. Interaction and screen documents can be derived
  from K and current behaviour, but they risk freezing defects as design.
- Confidence: read from code; the absence was confirmed by search at
  `131b9ce8` (no `docs/*JOB*`, `docs/*STOR*`, interaction or screen
  document).
- At 131b9ce8 (still holds): absence; `docs/` listing.

**48. The API reference is stale** — A1, A2, A4
- Observation: AR documents blueprint, toolset, session-mode and legacy FWU
  routes that are not registered (A1). Its WebSocket section disagrees with
  `ws.go` on version fields, the event envelope and the subscribe key, and
  its table of contents links to missing sections (A2). Much of the
  registered API, including runs, approvals, health, usage and the
  `runs:plan` fields, is undocumented (A4). #133 added a `task_ids` request
  field, which widens A4.
- Clause: [committed] AR:26-27, AR:273, AR:442-563, AR:600, AR:619-622,
  AR:739-764, AR:822. AR admits partial coverage (AR:3-5).
- Benefit: API callers, including the web UI's authors, have a correct
  reference. Entry 2 shows what a wrong one costs.
- Trade-off: docs-only. Generating the reference from routes would prevent
  drift but is new tooling.
- Confidence: read from code.
- At 131b9ce8 (still holds): `pkg/api/server.go:93-159` (`:97` "Legacy FWU
  routes REMOVED"); `internal/server/server.go:415-443`;
  `pkg/api/ws.go:160`, `:173-178`, `:202-205`;
  `pkg/manager/scheduler.go:17-23`.

**49. The chat component README describes a different tree** — U4
- Observation: CR describes a Zustand store, virtualized lists, a
  `/api/chat/ws` endpoint and Ctrl+Enter to send. None exists: state lives in
  hooks, the UI connects to `/ws` and sends on Enter. CR's tree also omits
  shipped folders.
- Clause: [committed] CR:73, CR:189, CR:247-341, CR:365, CR:397.
- Benefit: the only UI contract matches the UI.
- Trade-off: docs-only. Decide it after entries 3 and 17, because CR's
  protocol section depends on that choice.
- Confidence: read from code.
- At 131b9ce8 (still holds): `ui/src/App.tsx:40`;
  `ui/src/components/chat/input/InputTextarea.tsx:86-87`; no `ui/src/store/`.

**50. The TUI's documented keys and log view do nothing** — TU1, TU2, TK1
- Observation: Enter "View project logs" shows only TUI errors, because
  nothing builds a `ProjectUpdateMsg` (TU1). `?` "Toggle help" is an empty
  handler (TU2). `g`/`G` set a scroll position the log view never reads
  (TK1).
- Clause: the TUI's own help text, not a design document:
  `internal/tui/help.go:23`, `:28`, `:42`; `internal/cli/tui.go:14`.
  [committed].
- Benefit: the TUI does what its help says.
- Trade-off: either implement or drop the help lines. `internal/tui` has no
  tests.
- Confidence: read from code. The TUI was never run.
- At 131b9ce8 (still holds): `internal/tui/app.go:135-144`, `:196-199`,
  `:205-206`, `:215-223`.

**51. The route plan describes original-task resumption; the code closes the
original** — I1.5
- Observation: `TASK_ORIENTED_GOAL_ROUTE.md` says the original resumes after
  repair. The code closes it and lets the fix chain deliver it. Behaviour
  satisfies W1's "resume affected work". A code comment ("refuses a repair of
  a repair") is also stale.
- Clause: [committed] K:22-23 (W1); `docs/TASK_ORIENTED_GOAL_ROUTE.md:73`,
  `:96`.
- Benefit: documents match the tested mechanism.
- Trade-off: docs and comment only.
- Confidence: read from code; the behaviour was checked automatically
  (`TestTaskQueueFailedCheckClosesTaskAndItsFixDeliversIt`, step 3).
- At 131b9ce8 (still holds): `internal/release/failure_repair.go:19-20`,
  `:84-88`; `pkg/manager/task_execution_lock.go:74-76`.

**52. `docs/ARCHITECTURE.md` is a ledger study artefact** — F1.2
- Observation: it describes one repair study with ledger IDs ("US-015 /
  T-US-015-001 … accepted G-007"). The planner rule that study stories write
  this file overwrites the general architecture document.
- Clause: [owner contract] "without reconstructing execution state" (the
  newer intent's leave-and-return experience).
- Benefit: a reader outside the ledger can orient.
- Trade-off: the planner rule (AG "Once-heavy-then-light" (1)) would need a
  different target file.
- Confidence: read from code.
- At 131b9ce8 (still holds): `docs/ARCHITECTURE.md:3`, `:35-36`.

**53. Split vocabulary: storage, phase and task** — F1.3, F4.3, F4.4
- Observation: owner output says "imported to SQLite" (F1.3). "Phase" means
  both `new/planned/building/maintaining` and the operating model's
  Explore/Build/Finish/Ship (F4.3). "Task" is a ledger unit, an execution
  mode and free text, and README says Chat has "None" side effects (F4.4).
- Clause: [committed] N:23-25 ("process is reviewable").
- Benefit: one word, one meaning.
- Trade-off: wording only. Renames in CLI output can break scripts that parse
  it.
- Confidence: read from code.
- At 131b9ce8 (still holds): `internal/cli/plan.go:82`;
  `internal/cli/release.go:871`; `internal/release/models.go:242-257`;
  `README.md:87-88`; `internal/cli/start.go:232`, `:438`.

**54. The intent the console serves is not the committed intent** — R1:X1,
R1:X2
- Observation: committed `PROJECT_INTENT.md` is still the 2026-08-22
  interview ("solve the problem that ai cant code production quality data").
  The console serves the owner checkout's uncommitted 2026-09-05 synthesis,
  which states the portfolio-stewardship purpose (R1:X1). Anything that reads
  `origin/main` sees the August text, and the two disagree on all six
  answers (R1:X2).
- Clause: [committed] N:3-11 against the [owner contract] statement of
  purpose. AGENTS working agreements: the project context names the owner
  checkout file as its source.
- Benefit: repository readers and the console agree on what the project is
  for.
- Trade-off: committing the newer text is the owner's act on the owner's
  checkout. Machines may not author or adopt it. Supervisor guidance says
  work need not wait for it: the accepted owner contract is authoritative in
  the meantime. See "Needs owner judgment" below.
- Confidence: read from code (files and revisions).
- At 131b9ce8 (still holds): `PROJECT_INTENT.md:3-11`, last changed
  `75e3bd87`. The owner checkout was not re-opened by this pass.

**55. Intent questions only the owner can answer** — R1:U1, R1:U2, R1:U7
- Observation: the broader non-goal exclusions remain unanswered by the newer
  text's own wording (R1:U1). Whose needs may be deliberately secondary is
  not addressed (R1:U2). The meaning of "production quality data" in the
  committed purpose is not clarified, and the newer text replaces that
  purpose (R1:U7).
- Clause: [committed] `docs/OPENEXEC_PROJECT_INTENT_QUESTIONS.md` prompts
  (Q:27-28, Q:51-52); N:11, N:36.
- Benefit: later reviews can cite declared boundaries instead of nearest
  statements.
- Trade-off: owner-authored only. This report raises no question for them.
  Step 1's contract and supervisor guidance both say not to ask the owner to
  author intent through this chain.
- Confidence: read from code.
- At 131b9ce8 (still holds): `PROJECT_INTENT.md:11`, `:34-36`.

**56. No compatibility boundary in the intent** — R1:U5
- Observation: authority, evidence and cost are answered in the newer record.
  A compatibility boundary appears only in the Console's interpretation
  constraints, not in N, L or G.
- Clause: [committed] Q:44-45 (the template asks for compatibility).
  AGENTS.md's Anti-Regression Policy is the committed engineering rule that
  covers it today.
- Benefit: the protected-compatibility rule would rest on declared intent, not
  only on contributor guidance.
- Trade-off: owner-authored if it goes into intent. The existing AGENTS rule
  already enforces it in practice.
- Confidence: read from code.
- At 131b9ce8 (still holds): `AGENTS.md` "Anti-Regression Policy";
  `PROJECT_INTENT.md` (no compatibility clause).

**57. The Goal's validation conditions point to a file that was not found** —
R1:U6, R1:X3
- Observation: the untracked `GOAL.md` links the ten validation conditions to
  Agent Console `docs/GOAL_V5_PROFESSIONAL_PORTFOLIO_STEWARDSHIP.md`. That
  file was absent from the local Console checkout step 1 read, which was
  stale.
- Clause: [owner contract] Ready interpretation (V5D1-V5D10).
- Benefit: Ready conditions are readable where they are cited.
- Trade-off: Console-side. Not re-checked against current Console `origin/main`
  by this pass; owned by Console tasks, not native work.
- Confidence: read from code (stale checkout).
- At 131b9ce8: not re-checked (outside this repository).

**58. A Console plan line still says OpenExec has no intent** — R1:X4
- Observation: Agent Console `docs/EXPERIENCE_FIRST_DOGFOOD_PLAN.md:195` says
  "OpenExec has no owner-authored root `PROJECT_INTENT.md`". Native main has
  had one since `90569c7f` (2026-08-22). The supervisor's 2026-10-24 note
  makes the same correction.
- Clause: [committed] native `PROJECT_INTENT.md` exists (N).
- Benefit: no future triage inherits the obsolete claim.
- Trade-off: Console-side document; referenced, not proposed as native work.
- Confidence: read from code (stale Console checkout).
- At 131b9ce8: not re-checked (outside this repository).

**59. Repository grouping, recorded only** — R1:X5
- Observation: Agent Console declares its own `PROJECT_INTENT.md` while the
  owner contract groups both repositories as one portfolio project.
- Clause: [owner contract] constraint "Repositories have exclusive portfolio
  membership" and non-goal "Separate Agent Console and OpenExec portfolio
  projects".
- Benefit: none proposed. Supervisor guidance (2026-10-24) says the grouping
  is intentional and not for the owner to settle again.
- Trade-off: none. Recorded so the cross-check is complete.
- Confidence: read from code.
- At 131b9ce8: not re-checked (Console repository).

**60. `make compat-test` does not gate the graph schema tests** — INV-K5 note
- Observation: step 3 found no INV-K5 violation. It noted that `make
  compat-test` runs only `./internal/validation/... -run Compatibility`, so
  the graph schema tests are gated by CI's `go test ./...`, not by
  `compat-test` as KV3:125 implies.
- Clause: [committed] KV3:124-125.
- Benefit: the invariant names the gate that actually runs.
- Trade-off: one-line doc change. No behaviour gap, since CI runs the tests.
- Confidence: read from code; step 3's `make compat-test` run passed (checked
  automatically).
- At 131b9ce8: `Makefile` `compat-test` target (unchanged scope); not
  re-checked line by line.

**61. Step 3's statement that native OpenExec has no screens** — S0.2
- Observation: step 3 said the owner surface is "a CLI and daemon API rather
  than screens". The daemon serves an embedded React UI, and `openexec tui`
  ships. Step 4 corrected this, and step 5 audited both.
- Clause: [preference] (review-chain accuracy).
- Benefit: none outstanding; the correction is already on main in step 4.
  `docs/CONVERSATIONAL_ORCHESTRATION.md:121-126` still gives a `/chat` path
  the UI does not have.
- Trade-off: none.
- Confidence: read from code.
- At 131b9ce8 (correction holds): `internal/server/server.go:442`;
  `ui/src/App.tsx:92-128`; `internal/cli/tui.go:11-14`.

## Resolved since review

None of these is offered as new work. Partial fixes show their remainder in
the ranked entry named.

| Finding | Fixed by | What changed (at 131b9ce8) | Remainder |
|---|---|---|---|
| F2.1 | #130 `6e133023` | `run --quickfix`/`--verify` are refused before anything runs (`internal/cli/start.go:248-255`) | `--max-iterations`, `--timeout`: entry 24 |
| F3.2 | #131 `c835d583` | CONFIGURATION.md no longer claims built-in gates (`docs/CONFIGURATION.md:545-549`); `init` declares `lint` only with an inferred command (`internal/project/quality_gate.go:24-37`) | none |
| F3.3 | #132 `ca56ce21` | The daemon reads `ReviewEnabled` from project config (`internal/server/server.go:235-241`) | none |
| I3.1 | #133 `882c7938` | `run <id>` runs only that task, or is refused with 400 (`pkg/api/handlers.go:73-88`; `pkg/manager/scheduler.go:131-207`) | none for I3.1. The route is still the legacy scheduler: entry 8 |
| I1.2 | #134 `03c295d0` | `run` stops on a failed or invalid plan and says nothing was executed (`internal/cli/start.go:323-353`) | none |
| I3.2 | #135 `dcb64f33` | Chat posts the request text and its resolved mode to the session route (`internal/cli/chat.go:242-247`) | handler defaults: entry 2. Chat runs leave no task: entry 13 |
| I6.3 | #136 `5589b5e1` | Imports keep their own resolution; derived edges are capped below `compiler_exact` (`internal/knowledge/graph_scan.go:1256-1262`, `:1279-1284`) | none |
| I6.4 | #137 `190f13ac` | Edge and depth truncation are disclosed (`internal/knowledge/graph_query.go:52`, `:95`, `:131`, `:176`) | none |
| I6.2 | #141 `131b9ce8` (partly) | `failed` and `building` generations are no longer served from the fast path (`internal/knowledge/graph_freshness.go:95-102`) | `partial` is still served; documents disagree: entry 20 |

Whether these fixes are running in the installed daemon and CLI is not
established here (see After deploy).

## Needs owner judgment, not machine repair

These are recorded as findings. Nothing in this chain waits on them, and no
owner decision is requested.

- **The served intent is an uncommitted edit** (entry 54). `origin/main`
  still holds the August intent, which states a different purpose. Only the
  owner can commit, revise or withdraw that text. Supervisor guidance of
  2026-10-25 says the accepted owner contract is authoritative meanwhile, so
  machine work does not stall on it. This report does not ask the owner to
  rewrite intent or change grouping.
- **Two native documents named different owners for Goal review and Stop**
  (I2.1, entry 46). Step 3 left this open. The supervisor's 2026-10-25
  clarification settles the direction (K and AG control), which makes the
  remaining work a documentation correction an agent can make. It stays in
  this section only so the owner sees that the question was closed by
  recorded direction, not by this report.

## Cross-check: every finding ID and where it went

Each ID has exactly one disposition: a ranked entry, the resolved table, or
(for step 1's two answered items and INV-K5's "no finding") a stated
non-finding. The counts are: step 1, 12 IDs (10 open, 2 answered); step 2, 20;
step 3, 20 plus INV-K5; step 4, 26; step 5, 26.

| Report | ID → disposition |
|---|---|
| Step 1 (intent) | R1:U1 → 55 · R1:U2 → 55 · R1:U3 → answered by L/C (step 1), not open · R1:U4 → answered by L/C (step 1), not open · R1:U5 → 56 · R1:U6 → 57 · R1:U7 → 55 · R1:X1 → 54 · R1:X2 → 54 · R1:X3 → 57 · R1:X4 → 58 · R1:X5 → 59 |
| Step 2 (drift) | D0 → 47 · F1.1 → 1 · F1.2 → 52 · F1.3 → 53 · F2.1 → resolved (#130), remainder 24 · F2.2 → 25 · F2.3 → 15 · F3.1 → 22 · F3.2 → resolved (#131) · F3.3 → resolved (#132) · F3.4 → 15 · F4.1 → 18 · F4.2 → 23 · F4.3 → 53 · F4.4 → 53 · F4.5 → 16 · F4.6 → 27 · F5.1 → 16 · F5.2 → 46 · F5.3 → 46 |
| Step 3 (interaction) | I0 → 47 · I1.1 → 8 · I1.2 → resolved (#134) · I1.3 → 12 · I1.4 → 21 · I1.5 → 51 · I2.1 → 46 · I2.2 → 11 · I3.1 → resolved (#133) · I3.2 → resolved (#135), remainder 2 · I3.3 → 13 · I4.1 → 15 · I5.1 → 9 · I5.2 → 14 · I5.3 → 10 · I6.1 → 20 · I6.2 → resolved in part (#141), remainder 20 · I6.3 → resolved (#136) · I6.4 → resolved (#137) · I6.5 → 30 · INV-K5 → no finding (step 3); its gate note → 60 |
| Step 4 (interface) | S0.1 → 47 · S0.2 → 61 · S0.3 → 47 · U1 → 3 · U2 → 17 · U3 → 17 · U4 → 49 · U5 → 28 · A1 → 48 · A2 → 48 · A3 → 2 · A4 → 48 · E1 → 7 · E2 → 7 · E3 → 26 · E4 → 6 · T1 → 19 · T2 → 4 · T3 → 19 · T4 → 19 · M1 → 1 · M2 → 5 · M3 → 5 · TU1 → 50 · TU2 → 50 · TU3 → 29 |
| Step 5 (accessibility) | A0.1 → 31 · A0.2 → 31 · WK1 → 40 · WK2 → 40 · WK3 → 37 · WF1 → 36 · WF2 → 37 · WF3 → 44 · WF4 → 41 · WF5 → 32 · WF6 → 42 · WC0 → 44 · WC1 → 35 · WC2 → 35 · WC3 → 35 · WS1 → 38 · WS2 → 45 · WS3 → 34 · WS4 → 43 · WR1 → 33 · WR2 → 45 · WT1 → 42 · WM1 → 44 · TK1 → 50 · TC1 → 39 · TC2 → 39 |

Step 4's two screens-table observations (Knowledge Hub without DCP, Replay
placeholder) have no IDs and are in entry 28. Step 5's TUI refresh advisory
has no ID and is in entry 45.

## How this pass checked

- **Re-anchoring.** Every ranked entry's citations were re-located at
  `131b9ce8` by content (`grep -n`, `sed -n`) in a clean worktree whose HEAD
  is `131b9ce8`. Four read-only sub-agents did most of this, one per report;
  each returned a status per ID (still holds, changed or fixed).
- **Spot-checks by this pass directly.** These tier 1-2 citations were read
  line by line: entry 1 (`internal/mcp/broker.go:195-197`, `:233-236`;
  `internal/mcp/server.go:208`, `:917`; `internal/cli/approve.go:184-186`);
  entry 2 (`pkg/api/handlers.go:886-891`; `docs/API_REFERENCE.md:226`);
  entry 3 (`ui/src/hooks/useChat.ts:465-473`; `pkg/api/ws.go:195-201`);
  entry 4 (`internal/actions/apply_patch.go:35-39`); entry 7
  (`pkg/execution/agent_cli_provider.go:237-239`;
  `pkg/execution/gateway.go:97-105`); entries 8-9
  (`pkg/manager/scheduler.go:222-224`, `:339-348`, `:479-491`); entry 24
  (`internal/cli/start.go:82-83`, `:917-918`). The committed clauses quoted in
  tier 1 (A12:21, A12:25, LM:44, LM:127, AG:241, AG:254, AG:289, EP:15,
  EP:77, EP:85, TA:50, CR:389-393, CR:400) were also read at `131b9ce8`.
- **Nothing was run.** No daemon, CLI binary, browser, terminal UI or model.
  No audit was re-run, and no extra model spend was used beyond this pass's
  sub-agents.

## Limitations

- **Almost every product finding was read from code and never run.** The
  labels above are the reviews' own and are not upgraded. U1-U3, the
  highest-cost interface findings, still need a browser or WebSocket
  reproduction.
- **Re-anchoring is reading, not reproduction.** "Still holds" means the
  cited code still reads as described at `131b9ce8`. Step 5's
  browser measurements (WF6, WT1 and the probe-based observations) were not
  re-measured; the code they rest on is unchanged.
- **Most line re-anchoring was delegated** to read-only sub-agents. Only the
  citations listed under "Spot-checks" were read by this pass directly.
- **One cost axis.** Tiers order by cost of leaving alone, with severity then
  reach inside each tier. A low-severity item in tier 1 (entry 7) still ranks
  above a medium item in tier 3, because its tier's cost is an authority
  boundary.
- **Clauses that rest only on the uncommitted intent or the Console
  contract** are labelled [owner contract]. If that contract changes, those
  entries (15, 16, 52 and parts of 1 and 2) need re-checking.
- **The Console repository was not re-read.** Entries 57-59 rest on step 1's
  stale Console checkout. Console tasks 684d and 61e and the blocked
  originals own any Console correction; their contents were not available to
  this pass.
- **No fix is proposed as authorised.** Each entry's benefit and trade-off is
  advice. Filing native tasks, and in which order, is route work for the
  navigator under existing authority.

## After deploy

These are not acceptance for this report. They are the measurements the
reviews could not make:

- Confirm that the installed daemon and CLI include #130-#137 and #141. For
  example, `openexec run --quickfix x` should refuse, and `openexec run <id>`
  should run only that task.
- Reproduce U1-U3 (entries 3 and 17) in a browser against the served UI, or
  over the daemon's `/ws`.
- Check focus and announcements with a real screen reader (NVDA, VoiceOver or
  Orca) on the served UI, including the fork dialog (WF1, entry 36) and live
  announcements (WR1, entry 33).
- Run `openexec tui` in a real terminal and check TU1, TU2 and TK1 (entry 50),
  contrast under the user's theme (entry 39), and how a terminal screen reader
  reads the one-second redraw (entry 45).
