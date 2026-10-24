# Design review 2/6 — strategy and drift

**Date:** 2026-10-24. **Task:** `c2986112a1f4010ed4933578c755f6dc` (second step
of the OpenExec `design-review` run; step 1 is
[DESIGN_REVIEW_INTENT_2026-10.md](DESIGN_REVIEW_INTENT_2026-10.md)).

**Reviewed revision:** native `openexec` `origin/main`
`f3e8d8501667f469daa7526f7a55d9740f22e4b0`. Every `path:line` below is at that
commit unless it carries a source label from the table.

This report asks whether the native implementation still serves the declared
intent. It changes no code, intent, grouping or configuration, proposes no
owner decision, and is not evidence of Goal Ready or of served behaviour.

## Rule being applied

The review step's own contract (Agent Console
`internal/playbook/playbook.go:258-260`, read from the local Console checkout,
which step 1 records as behind its origin): "Every finding cites a clause of
PROJECT_INTENT.md or a layer-1 job, and says whether the drift is in the
product or in the document — the intent is allowed to be the thing that is
wrong. Finding nothing is a valid result and is stated plainly."

## Sources

The labels are step 1's (its "Sources and provenance" table); the inventory is
not repeated here.

| ID | Source | On `origin/main`? |
|---|---|---|
| **N** | `PROJECT_INTENT.md` at `f3e8d850` (last changed `75e3bd87`, 2026-08-22), owner interview words | yes |
| **L** | owner checkout `/mnt/data1/projects/openexec/PROJECT_INTENT.md`, uncommitted edit dated 2026-09-05, a non-verbatim synthesis | **no** |
| **G** | owner checkout `GOAL.md`, untracked | **no** |
| **C** | Agent Console project context (Goal, Non-goals, Constraints, interpretation revision 10), sourced from L | **no** (console record; has no line numbers, so C clauses are quoted) |
| **K** | `docs/OPENEXEC_SIMPLE_LOOP_ARCHITECTURE_CONTRACT.md` at `f3e8d850` (last changed `593a9e18`), "owner-mandated architectural constraint, 2026-09-14" (K:3) | yes |

K is not a `PROJECT_INTENT.md` clause. It is cited because it is the committed,
owner-mandated statement of which loop owns which job, and the task's scope
names it.

Classification used in every finding: **product** (code or CLI behaviour
departs from the declaration), **document** (a document is stale, wrong or
missing), or **N↔L** (the finding depends on a clause that exists only in the
uncommitted L/G/C record, so it is drift against the newer intent but not
against the committed one). Severity: **high** (defeats a declared job),
**medium** (owner-visible behaviour contradicts a declaration), **low**
(wording, stale document or opt-in path).

## Layer 1: jobs

**D0 — Native OpenExec has no layer-1 document.** No jobs, stories or
success-measure document exists on `origin/main` (no `docs/*JOBS*` or
`docs/*STORIES*`; step 1 notes the intent template has no "jobs" question).
The job statements used below are therefore the nearest declared ones:

- **J-N** (N:23-25): "split intent into tasks and then implement and test that
  deliverable works and outcome is predictable and process is reviewable."
- **J-L** (L:19-23): "The owner can leave, switch projects and return without
  reconstructing execution state or coordinating tasks, retries and
  repositories. Agent Console provides owner interaction and orientation;
  OpenExec owns navigation and execution."
- **J-K** (K:64-65): "OpenExec owns planning, plan review, native tasks,
  dependencies, priorities, implementation, verification, repairs, retries and
  task continuation/convergence." K:67-69 gives Console "Intent/Story,
  Goal/Ready … independent Goal review … owner-facing progress".

Classification: **document**. Severity: **medium**. Each later review step has
no jobs list to check against either. This report does not invent jobs.

## 1. Implementation concepts leaking into the interface

**Checked:** help text and output of every registered command in
`internal/cli/*.go` (`Use:`/`Short:`/`Long:`), README, GET_STARTED,
`docs/ARCHITECTURE.md`, ADR-001 to ADR-019.

**F1.1 — `openexec approve` advertises a Task-mode approval path that is not
wired.**
- Clause: J-N "process is reviewable"; L:29-30 "The owner holds … consequential
  authority".
- Evidence:
  - `internal/cli/approve.go:20-23` says: "Manage tool approval requests …
    In Task mode, certain operations require user approval before execution."
  - `internal/mcp/approval_gate.go:103-110`: generic approval applies only when
    `IsApprovalEnabledForServer` is true. The only call to
    `SetApprovalEnabledForServer` outside its definition is
    `internal/mcp/infra_test.go:438`, a test.
  - Without `--local`, the command calls the daemon's `/api/v1/approvals`
    (`approve.go:79`). The daemon constructs the API with
    `api.New(..., api.WithStateStore(stateStore))` (`internal/server/server.go:274`),
    never `WithApprovalGate`. The handler then returns
    `503 "approval gate not configured"` (`pkg/api/handlers.go:1004-1005`).
  - ADR-012:21 says "Medium-Risk (Workspace Writes) … Gated by local approval
    prompts."
- What does work: the `--local` form reads `.openexec/approvals.db`, which
  `mcp-serve` opens for infra apply tools (`internal/cli/mcp_serve.go:220-234`).
  That covers the external, high-risk effects ADR-012:22 describes.
- Classification: **product** (an owner command whose default form can only
  fail) and **document** (ADR-012:21 overstates). The missing gate itself is
  *not* drift: per C's non-goal "Owner approval of navigator-created tasks",
  routine workspace writes should not need the owner.
- Severity: **low**.

**F1.2 — `docs/ARCHITECTURE.md` is a ledger study artifact, not the
project's architecture.**
- Clause: J-L "without reconstructing execution state".
- Evidence: `docs/ARCHITECTURE.md:3` "Current validation: US-015 /
  T-US-015-001, 2026-09-30, accepted G-007". Lines 14-23 describe one repair
  study. Lines 34-38 cite "Console Project context … interpretation 10". The
  planner rule that study stories must write `docs/ARCHITECTURE.md` (AGENTS.md,
  "Once-heavy-then-light lifecycle support" (1)) overwrites the general
  document with ledger IDs. These IDs mean nothing outside the ledger.
- Classification: **document**. Severity: **low**.

**F1.3 — Storage vocabulary in owner output.**
- Clause: J-N.
- Evidence: `internal/cli/plan.go:82` prints "Stories generated and imported
  to SQLite". `internal/cli/release.go:870-871` (`story import`) is described
  as "Advanced: Import stories from JSON into SQLite (explicit migration)".
- Classification: **product**. Severity: **low**.

## 2. Duplicated workflows

**Checked:** every command that plans, executes, completes or merges work:
`run`, `plan`, `blueprint`, `chat`, `wizard`, `start`, `story *`, `task *`,
`release *`, and the MCP backlog tools.

**F2.1 — `run --quickfix` advertises the small-task path but is dead.**
- Clause: K:44-49 ("Focused request → … preferably one vertical task … No
  mandatory project-scale … ceremony"); J-N.
- Evidence:
  - `internal/cli/start.go:237` documents
    `openexec run --quickfix "Fix typo" --verify "make test"`.
  - The flags are declared at `start.go:864-865` into `runQuickfix` and
    `runVerify` (`start.go:85-86`). A grep of `internal/cli` finds no other
    reference to either variable. `--max-iterations` and `--timeout`
    (`start.go:860-861`) are also never read.
  - The `run` body then auto-plans from `INTENT.md` (`start.go:293-302`) and
    posts `runs:execute` with only `worker_count`, `mode` and an optional
    `task_ids` (`start.go:306-312`).
- Consequence: a quickfix invocation runs the plan-and-execute-everything
  path, which is the opposite of what it advertises. This was read from the
  code and not executed.
- Classification: **product**. Severity: **medium**.

**F2.2 — The `init` guidance routes the owner through a deprecated command.**
- Clause: J-L "without … coordinating tasks"; J-N "structured way".
- Evidence:
  - `internal/cli/init.go:246-248` prints three next steps: "Run 'openexec
    wizard'", then "Run 'openexec plan INTENT.md'", then "Run 'openexec run'".
  - `internal/cli/plan.go:28` labels `plan`: "Advanced: Generate a project
    plan manually (Deprecated: use 'openexec run' instead)".
  - `run` already plans (`start.go:293-302`).
  - README:201-203 lists `wizard` and `run` but not `plan`.
- Classification: **product** (CLI output) and **document**. Severity: **low**.

**F2.3 — Two task-completion paths with different side effects.**
- Clause: J-N "outcome is predictable".
- Evidence:
  - `openexec task complete` calls `mgr.CompleteTask` (`internal/cli/release.go:1358`).
    That function "triggers auto-merge if configured" (`internal/release/automerge.go:46-49`).
  - The MCP `backlog_complete_task` calls only `SetTaskStatus(..., TaskStatusDone)`
    (`internal/mcp/backlog.go:532`).
  - The same completion therefore merges or does not merge depending on the
    entry point.
- Classification: **product**. Severity: **low**, because auto-merge
  defaults off (F3.4).

No finding for the remaining candidate duplications:
- `blueprint` (`start.go:381`) and `chat` each start a run differently, but
  they serve distinct modes that README:84-90 documents.
- `knowledge graph journey` is hidden. It is described as "the reviewed graph
  deploy's live acceptance check, not an owner command"
  (`internal/cli/knowledge_journey.go:22-28`).

## 3. Configuration replacing automation

**Checked:** `docs/CONFIGURATION.md`, `openexec config set` keys
(`internal/cli/config.go`), `init` prompts and flags, `openexec.yaml` parsing,
and execution defaults.

**F3.1 — The configuration guide documents owner-tunable loop policy that
nothing reads.**
- Clause: J-K (OpenExec owns retries, repairs, continuation); J-L.
- Evidence:
  - The guide documents `review.require_review` (CONFIGURATION.md:63),
    `execution.auto_fix` / `max_fix_iterations` (:87-90), `retry.max_attempts`
    (:99), `mode.default` (:207), `gitflow.auto_merge` (:253) and
    `parallel.auto_merge` (:281).
  - The only YAML schema for `openexec.yaml` is
    `internal/execution/gates/config.go:13-16`, which parses `project:` and
    `quality:` only.
  - Retry, iteration and review-cycle limits are fixed defaults
    (`internal/config/defaults.go:7-9`).
- The product does what the intent asks: retries are automatic. The document
  presents them as owner configuration.
- Classification: **document**. Severity: **medium**, because an owner
  following the guide edits settings that have no effect.

**F3.2 — The documented built-in quality gates do not exist, and the
`init` default names one.**
- Clause: N:31-32 "handle coding like senior engineers do"; N:42 "ships
  tested and working code".
- Evidence:
  - CONFIGURATION.md:543-553 lists "Built-in Gates": `lint`, `typecheck`,
    `test`, `security`, `format`.
  - `project.Initialize` writes `quality: gates: [lint]` with no `custom:`
    entry (`internal/project/project.go:359-366`).
  - `GetGate` searches only `Quality.Custom` (`gates/config.go:64-71`).
  - `RunGate` returns "gate '%s' not found in config" with `Passed = false`
    (`internal/execution/gates/runner.go:108-112`), and `RunAll` marks the
    report failed (`runner.go:85-88`).
  - The daemon builds its manager without a `StageExecutor`
    (`internal/server/server.go:203-235`), so `pkg/manager/manager.go:460-463`
    attaches this gate runner on the native daemon route.
  - The admitted route, which sets a `StageExecutor`, does not attach it.
- So the owner must hand-write gate commands that the guide says are
  built in. This pass read the failure from code and did not run it. It did
  not trace how the pipeline acts on the failure.
- Classification: **product** and **document**. Severity: **medium**.

**F3.3 — The `init` review answer is stored but never reaches the daemon.**
- Clause: N:24-25 "process is reviewable"; L:31-32 "reviewable delivery".
- Evidence:
  - `init --review` and the interactive prompt set `ReviewEnabled`
    (`internal/cli/init.go:173`, `:208`, `:257`).
  - The daemon's `manager.Config{…}` (`internal/server/server.go:203-235`) has
    no `ReviewEnabled`, and no other non-test code reads the project field
    (`internal/project/project.go:144`).
  - The pipeline then sets the blueprint `test.OnSuccess = "complete"`
    (`internal/pipeline/pipeline.go:700-703`), skipping the review stage
    whatever the owner answered.
- Mitigation: delivery-time independent exact-head review is held outside this
  stage (C invariants; K:53-55). The owner's answer is still silently
  discarded.
- Classification: **product**. Severity: **medium**.

**F3.4 — `approval_enabled` is an opt-in owner gate on machine-created
stories and tasks.**
- Clause: C non-goal "Owner approval of navigator-created tasks"; J-L.
- Evidence:
  - The setting defaults to false (`internal/release/manager.go:76`,
    `internal/cli/config.go:220`).
  - `config init --approval` turns it on (`config.go:186-188`).
  - When it is on, every new story and task starts `ApprovalPending`
    (`internal/release/manager.go:519-523`, `:663-667`).
  - Story merge then refuses with "story %s is not approved"
    (`manager.go:882-885`), and needs task approval for `NeedsReview` tasks
    (`internal/release/automerge.go:216-229`).
  - `task complete` tells the owner "Run: openexec story approve"
    (`internal/cli/release.go:1367-1368`).
  - Dispatch is *not* gated. The scheduler holds back only planner-tagged
    `hitl` tasks (`pkg/manager/scheduler.go:36-44`).
- Classification: **product**, opt-in. It is an **N↔L** finding because the
  non-goal exists only in C, and N:36 has no non-goals.
- Severity: **low**: the default is off and merge authority is the owner's
  anyway (C authority interpretation).

## 4. Terminology that has split in two

**Checked:** Goal, Outcome, Story, Task, Intent, Phase and "done" across the
CLI, the `.openexec/stories.json` import/export structs, README, GET_STARTED,
CONFIGURATION and ARCHITECTURE.

**F4.1 — "Intent" is two files, and the planner reads only the generated one.**
- Clause: N:23 "split intent into tasks"; K:20 "Intent/current state →
  native task generation".
- Evidence:
  - The planner input is `INTENT.md` (`internal/cli/plan.go:33`,
    `internal/cli/start.go:295`, `internal/cli/doctor.go:61`,
    `pkg/manager/planner.go:76`). `wizard` writes it
    (`internal/cli/wizard.go:120`).
  - No non-test Go file references `PROJECT_INTENT` (grep).
  - README:202 describes `wizard` as "Define goal, generates INTENT.md". That
    is a third name for the same step.
  - ADR-005:36-42 already records the split as "target product direction, not
    a shipped runtime refusal".
- Under K, Console owns Intent (K:67), so the native planner need not read
  `PROJECT_INTENT.md` itself. The word still names two artifacts with
  different authority.
- Classification: **product** and **document** (already acknowledged in
  ADR-005). Severity: **medium**.

**F4.2 — "Goal" means planner records and the owner's Goal/Ready.**
- Clause: K:32-33 "Do not infer Ready from task completion"; L:31 "Evidence
  determines Ready".
- Evidence:
  - Planner goals are `G-NNN` records (`internal/planner/planner.go:12-16`,
    `internal/release/models.go:48-52`).
  - The top-level `openexec goal verify` (`internal/cli/release.go:1590-1598`,
    registered at `:1697`) says: "Run high-level verification scripts to
    confirm that project goals have been met". Without `--execute`, it
    decides from story status alone (`release.go:1651`) and prints
    "implementations-complete".
  - The owner's Goal/Ready is a different object (K:30-37;
    `pkg/outcome/evaluator.go:19-24`).
  - `docs/ARCHITECTURE.md:3` and `:35` use both meanings in one paragraph
    ("accepted G-007"; "accepted Goal/Ready revision 4").
- Answer to the candidate check: this command does not perform Goal
  convergence and nothing routes Ready through it. It is a planner-goal
  status report whose help text overclaims.
- Classification: **product** (help text) and **document**.
  Severity: **low**.

**F4.3 — "Phase" names two unrelated state sets.**
- Clause: J-L.
- Evidence:
  - `release.ComputePhase` gives `new/planned/building/maintaining`, derived
    from story counts as a lane hint (`internal/release/models.go:237-279`).
  - `docs/EXPERIENCE_FIRST_OPERATING_MODEL.md:458-466` defines
    Explore/Build/Finish/Ship "policy states, not estimated percentages".
- Classification: **document**. Severity: **low**.

**F4.4 — "Task" names a ledger unit, an execution mode and a free-text
request.**
- Clause: J-N.
- Evidence:
  - Ledger unit: `T-NNN` (`internal/release/models.go`, `release.go:1277`).
  - Execution mode: README:88 "**Task** | Scoped action".
  - Free text: `blueprint <task-description>` (`start.go:381`), next to
    `run [task-id]` (`start.go:227`).
  - README:87 also says Chat mode has "None" side effects. AGENTS.md says
    orchestrator bookkeeping may still change in chat.
- Classification: **document**. Severity: **low**.

**F4.5 — "Done" has three definitions.**
- Clause: J-N "outcome is predictable".
- Evidence:
  - API progress uses `release.Delivered`: done, approved, or failed with a
    delivered fix chain (`internal/release/failure_repair.go:189-192`;
    `pkg/api/handlers.go:498-513`).
  - `story list` counts `== TaskStatusDone` only (`internal/cli/release.go:650-657`).
  - The TUI counts `status IN ('completed', 'done')`
    (`internal/tui/file_source.go:278`), where `completed` is not a
    `TaskStatus` (`internal/release/models.go:220-227`).
- Classification: **product**. Severity: **low** (it feeds F5.1).

**F4.6 — The `.openexec/stories.json` format has diverged between its
writers and its reader.**
- Clause: J-N.
- Evidence:
  - `release export` writes `"goals": []` with the comment "goals not
    persisted here" (`internal/cli/release.go:68-71`).
  - The manager's `ExportJSON` writes the goals (`internal/release/manager.go:1198-1203`).
  - The planner's current schema is `"1.2"` (`internal/planner/planner.go:94`).
  - `story import` accepts only `1.0`, `1.1` or `legacy`
    (`internal/cli/release.go:925-926`).
- `run` and `plan` import in-process, so only the explicit file import is
  affected. This was not executed.
- Classification: **product**. Severity: **low**.

**No finding on "Outcome" or "Story" as owner vocabulary.** Native "outcome"
appears as an execution result status (`pkg/execution/execution.go`), a
plan-shape lint and the internal `pkg/outcome` evaluator. None is offered to
the owner as a work unit, and the CLI has no `outcome` command. "Story" is used
consistently as the planner's user-visible slice.

## 5. Contradictions of a declared non-goal

N declares no non-goals (N:36 `UNANSWERED — owner`). All non-goals checked here
are L:39-42 and C's list, so every finding in this section is also **N↔L**.

**Checked:** C's six non-goals against every registered command, the TUI, the
API progress response, ADR-012/019 and the operating model.

**F5.1 — The owner-facing CLI and TUI show progress as task-count
percentages.**
- Clause: C non-goal "Progress percentages derived from task or agent
  counts"; K:33; L:48-49 "Return reviews present genuine outcome changes".
- Evidence:
  - `openexec status` prints "Progress: %d/%d tasks completed (%.1f%%)"
    (`internal/cli/status.go:173-176`). The counts come from `GET /api/v1/runs`
    (`pkg/api/handlers.go:498-520`).
  - `openexec tui`, the "multi-project execution status" dashboard
    (`internal/cli/tui.go:12-13`), shows `"Progress: " + … + "%"` next to a
    worker count (`internal/tui/app.go:164-166`). The value is computed as
    `(completed * 100) / total` (`internal/tui/file_source.go:259-283`).
  - Native code holds the opposite rule internally: "Do not infer progress
    from activity, tasks, tests, commits, tools, tokens or claims"
    (`pkg/outcome/evaluator.go:252`).
  - The operating model scopes its no-percentages rule to Agent Console only
    (`docs/EXPERIENCE_FIRST_OPERATING_MODEL.md:524-525`).
- Classification: **product** (and **N↔L**). Severity: **medium**. This is
  the clearest native contradiction of a declared non-goal.

**F5.2 — The operating model plans Goal triage and Done authority inside
OpenExec.**
- Clause: K:30-37 and K:67-69 (Console owns Goal/Ready and Goal review); C
  non-goal "Duplicated navigation logic in Agent Console".
- Evidence: `docs/EXPERIENCE_FIRST_OPERATING_MODEL.md:899-963` ("Planned
  OpenExec integration"):
  - E1 "Advisory Goal and experience triage";
  - E2 "Durable gate … Require accepted Intent, Goal … before planning";
  - F3 "Durable completion authority … before accepting Done".
- The document is dated 2026-08-17 (line 3) and was not updated when K
  (2026-09-14) assigned those jobs to Console. No code implements E1-F3 (its
  own status: "not yet enforced by OpenExec").
- Classification: **document**. Severity: **low**.

**F5.3 — ADR-019 proposes a second backlog and agent merges to trunk.**
- Clause: K:27-28 "native tasks remain the durable truth of unfinished
  work"; C authority interpretation (merge is the owner's); C non-goal
  "indiscriminate platform extraction".
- Evidence:
  - ADR-019:24-25 makes "Git-Backed Backlog Queues and Trunk-Based Swarm
    Scaffolding" the default for `init`.
  - At :57-59, "the agent opens a Pull Request or merges directly back to
    `main`".
  - Its status is "Proposed. Implementation: Aspirational" (:5), and no code
    reads `.openexec/backlog/` or claim files (grep).
- Classification: **document**. Severity: **low**.

**Candidate checks answered with no finding:**
- **`goal`, `journey`, `approvals`, `approve` and ADR-012 against "Owner
  approval of navigator-created tasks":**
  - No command makes owner approval a condition of dispatching
    navigator-created work.
  - The scheduler's only hold is the planner-tagged `hitl` mode
    (`pkg/manager/scheduler.go:36-44`).
  - `release approvals` is a read-only list (`internal/cli/release.go:494-557`).
  - `approve --local` covers infra apply effects, which C reserves to the
    owner as consequential effects.
  - The opt-in story/task approval is F3.4. `goal verify` is F4.2. `journey`
    is a hidden deploy check (§2).
  - None performs Console's Goal convergence.
- **Coercive prioritization or inactivity pressure:**
  - A search of non-test `internal/` and `pkg/` Go for `overdue`, `inactiv`
    and `remind` found no owner-facing pressure.
  - Telegram budget alerts report spend percentage (`internal/telegram/budget_formatter.go:107-108`),
    not activity.
- **Cross-project authority leakage:** `skills promote` copies a project
  skill to the user directory only when a human runs it (AGENTS.md, Skill
  Synthesis), which matches L:28 "Learning may transfer". It carries no
  authority.
- **Separate portfolio projects or permanent multi-agent committees:** no
  native code or document found. ADR-009's single isolated reviewer is not a
  committee.

## Findings owned elsewhere

Every finding above is in native source. None proposes an Agent Console
change. The Console-side corrections that the triage contract names as
already owned (tasks 684d and 61e, and the blocked originals) are not
re-proposed here. This pass could not read those tasks' contents from this
checkout, so the only overlap check it could make is by repository. The
earlier resolver defect fixed by Console `ae22eda4` is history recorded in step
1 and is not rebuilt.

## Limitations

- **Only N and K are on `origin/main`.** L, G and C are not: L is an
  uncommitted edit and G is untracked in the owner checkout, and C is a
  console record. Every finding that cites only L, G or C is marked N↔L. If
  the owner never commits L, those findings are drift against an undeclared
  intent.
- **No layer-1 jobs document exists** (D0). The jobs used are the nearest
  declared statements, not an owner-accepted list.
- **Nothing was executed against a running build.** F2.1, F3.2, F3.3 and F4.6
  are read from code and not run. F3.2 does not trace what the pipeline does
  after a gate failure.
- **The Agent Console checkout is stale.** It was read only for the playbook
  rule and dependency order. Its `origin/main` was not available to this pass.
- **Fixes are out of scope.** Engineering defects inside accepted work (for
  example F2.1, F3.3) can proceed under existing authority without an owner
  question. Only new scope or product judgment goes to the owner. This report
  raises no owner decision.

## Downstream

The playbook order (Console `internal/playbook/playbook.go:251-301`) is:
- drift (this report) → interaction (`344c8390…`), which depends on drift;
- → interface (`bb19d2e4…`);
- → accessibility (`869ef988…`);
- → the consolidating report (`8a800117…`).

Each keeps its dependency on the step before it. Their findings are not
anticipated here.
