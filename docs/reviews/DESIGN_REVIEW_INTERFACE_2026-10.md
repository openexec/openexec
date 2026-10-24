# Design review 4/6 — interface and component conformance

**Date:** 2026-10-24. **Task:** `bb19d2e4a4f248fc9694c0b0423a35b9` (fourth step
of the OpenExec `design-review` run; step 1 is
[DESIGN_REVIEW_INTENT_2026-10.md](DESIGN_REVIEW_INTENT_2026-10.md), step 2 is
[DESIGN_REVIEW_DRIFT_2026-10.md](DESIGN_REVIEW_DRIFT_2026-10.md), step 3 is
[DESIGN_REVIEW_INTERACTION_2026-10.md](DESIGN_REVIEW_INTERACTION_2026-10.md)).

**Reviewed revision:** native `openexec` `origin/main`
`5589b5e146c0e507fde98626b82e166981f0fd2c`. Triage was written against
`c8a3532b7d9df1390f04efe4a324fbc5676b763a` (step 3, #129). Seven native
commits landed between the two (#130, #131, #132, #134, #135, #136, #137), so
this review uses the newer revision. Every `path:line` below is at `5589b5e1`.

This report asks two questions. Does each screen do the job layer 3 gives it?
Do components stay inside their contracts, with no anti-pattern that a
contract declares? It changes no code, intent, grouping or configuration. It
raises no owner decision. It is not evidence of Goal Ready or of served
behaviour.

## Rule being applied

The rule comes from the review step's own contract, Agent Console
`internal/playbook/playbook.go:270-276`, read from the local Console checkout
(step 1 records that checkout as behind its origin): "Every finding names the
screen responsibility or component contract it violates." The step
description is: "Does each screen carry the responsibility layer 3 gave it?
Are components used inside their contracts, and are their anti-patterns
absent?"

## Sources

The labels N, L, G, C, K and KV3 are step 1's (see its "Sources and
provenance" table); step 3 uses the same labels. The labels below them are new
in this report. All are on `origin/main` unless the table says otherwise.

| ID | Source | Last changed |
|---|---|---|
| **N** | `PROJECT_INTENT.md`, owner interview words | `75e3bd87`, 2026-08-22 |
| **L / G / C** | owner checkout `PROJECT_INTENT.md` edit / `GOAL.md` / Console project context | **not on main**; not opened by this pass |
| **K** | `docs/OPENEXEC_SIMPLE_LOOP_ARCHITECTURE_CONTRACT.md` | `593a9e18`, 2026-09-14 |
| **CR** | `ui/src/components/chat/README.md`: chat component tree, responsibilities, WebSocket protocol | `96fa6397`, 2026-03-02 |
| **AR** | `docs/API_REFERENCE.md`: daemon HTTP and WebSocket. It describes itself as "built, partial coverage" and "not re-verified endpoint by endpoint" (AR:3-5) | `5b282f4d`, 2026-08-19 |
| **EP** | `docs/EXECUTION_PROVIDER_CONTRACT.md`: "Normative for callers of" `pkg/execution` (EP:3) | `087e0229`, 2026-09-12 |
| **TA** | `docs/architecture/TOOL-ACTION-CONTRACT-V1.md`: deterministic blueprint actions | `97911238`, 2026-03-15 |
| **LM** | `docs/LIGHT_MODE.md`: MCP backlog tools | `91d11112`, 2026-09-04 |
| **SK** | `docs/SKILLS_SYSTEM.md`: skill candidates | `91d11112`, 2026-09-04 |
| **A11 / A12** | ADR-011 (MCP execution plane) / ADR-012 (approval gate), `docs/architecture/` | `6ecaacd1` / `bbd10949`, 2026-06-15 |
| **AG** | `AGENTS.md`: the architecture and quirk clauses that agents are told to follow | `b21e28cd`, 2026-10-19 |

**Classification**, as in steps 2 and 3:
- **product**: code or a surface departs from the contract.
- **document**: the contract is stale, wrong or missing.
- **N↔L**: the finding rests only on L, G or C.

**Severity:**
- **high**: defeats the surface's primary responsibility.
- **medium**: an owner or agent relies on behaviour that contradicts the
  contract.
- **low**: wording, a stale document, dead code or an opt-in path.

**Check:** each finding says whether it was **read** from code or
**executed**. Where no clause exists, the finding says **no clause** and is
labelled a document gap or a preference.

## 0. What this review checks against

**S0.1: No layer-3 screen document exists.**
- No document on `origin/main` assigns screens their responsibilities,
  hierarchy or states.
- CR is the closest document. It describes components and a protocol, not what
  a screen is responsible for.
- K assigns owner interaction to Agent Console (step 3, W6). It does not
  describe native screens.
- This review does not make responsibilities up, and it does not use Agent
  Console's style or design guides as a stand-in.
- Classification: **document**. Severity: **medium**. Section 1 can only list
  the screens and record what each one depends on. It cannot grade them
  against a declared responsibility.

**S0.2: Step 3 said native OpenExec has no screens. Main ships two more owner
surfaces.**
- Step 3 says "native OpenExec's owner surface is a CLI and daemon API rather
  than screens" (`docs/reviews/DESIGN_REVIEW_INTERACTION_2026-10.md:583-586`).
  Triage carried that statement forward.
- **The web UI.** The daemon serves an embedded React UI at `/`
  (`internal/server/server.go:440-442`). `make build` embeds `ui/dist`
  (`assets_ui_dist.go:12`, `Makefile:30`). The UI has four top-level views:
  Chat, Knowledge Hub, Usage and Replay (`ui/src/App.tsx:92-128`).
- **The terminal UI.** `openexec tui` is a registered command
  (`internal/cli/tui.go:11-14`, `:52-54`) and runs a Bubble Tea dashboard
  (`internal/tui/app.go`).
- Step 2 had already reviewed the TUI (step 2 F5.1,
  `DESIGN_REVIEW_DRIFT_2026-10.md:365-383`). Step 3's statement therefore
  disagreed with step 2 as well as with the code.
- `docs/CONVERSATIONAL_ORCHESTRATION.md:121-126` names both surfaces. It gives
  the web UI as `http://localhost:3000/chat`, but `App.tsx` has no router and
  no `/chat` path. That document calls itself indicative, not verified
  (step 3 I0).
- Classification: **document**, a correction to step 3. Read from code.

**S0.3: Declared anti-patterns exist for some contracts, and for no screen or
UI component.**
- No anti-pattern is declared in CR, in AR or for either screen surface.
- Explicit prohibitions do exist:
  - TA:12 "**What it is NOT:** Arbitrary shell snippets in blueprint files or
    model-defined behaviors."
  - TA:58 "Actions never implicitly operate on "."."
  - TA:67 says the contract "prevents deterministic stages from becoming
    opaque shell commands".
  - EP:21-22 (unenforceable sandbox; writable roots on a read-only request)
    and EP:77 "Neither provider uses a dangerous permission bypass."
  - A12:25 "**Anti-Self-Approval:** The agent is physically blocked from
    calling any approval-bypass tools"; LM:126-127; AG:254.
- Triage expected none, so this corrects triage. Only these declared
  prohibitions are checked below (T4, E4, M1). No anti-pattern list is
  invented for the UI.
- Classification: **document** for the UI gap. Severity: **low**.

## 1. Screens (no layer-3 clause; S0.1)

| Surface | What ships | Depends on | Observation (no clause) |
|---|---|---|---|
| Web: Chat | `ui/src/pages/ChatPage.tsx` mounts `ChatLayout`, `ChatMain`, `SessionSidebar`, `EventPanel`, `CostPanel`, `SessionForkDialog`, `StageTimeline`, `ApprovalsPanel` (`ChatPage.tsx:11-20`) | `/ws` and `/api` sessions, projects, providers, approvals | The WebSocket client and the daemon speak different protocols: §2 U1-U3 |
| Web: Knowledge Hub | `ui/src/pages/KnowledgeHub.tsx` | `GET /api/v1/knowledge/symbols`, `/envs` (`KnowledgeHub.tsx:64-67`) | Those routes exist only when the DCP coordinator is on (`internal/server/server.go:420-423`); otherwise the catch-all returns 404 (`server.go:431-438`) and the page shows "Some knowledge endpoints failed to load" (`KnowledgeHub.tsx:77-79`). The nav button shows either way (`App.tsx:99-104`) |
| Web: Usage | `ui/src/pages/UsageDashboard.tsx` | `/api/usage/*` (`ui/src/hooks/useUsageData.ts:355-450`) | Served when the audit logger and session repository are wired (`pkg/api/server.go:150-151`) |
| Web: Replay | A placeholder that renders "Replay Tooling (v2) - Visualizing audit run steps..." (`ui/src/App.tsx:15-16`) | none | A nav item with no function (`App.tsx:111-116`). **Preference**: no clause says a placeholder may not ship |
| Terminal: `openexec tui` | Project list and log view (`internal/tui/app.go:110-147`) | `.openexec/state.json` and `openexec.db` (`internal/tui/file_source.go:173-237`) | §2 TU1-TU3. Task-count progress is step 2 F5.1 and is not repeated |
| CLI | Cobra commands (`internal/cli`) | daemon API | Steps 2 and 3 cover these |

## 2. Components against their contracts

### Web chat against CR

**U1: The chat UI sends messages and tool approvals the daemon does not
handle. The approval still shows as "approved".**
- Contract: CR:372-378 lists `send_message`, `approve_tool`, `reject_tool`,
  `pause`, `resume` and `stop` as client→server messages. The UI's
  `ChatInput` and `ToolCallApproval` rows describe the same components
  (CR:173-184).
- Evidence:
  - The UI sends the owner's text as WebSocket `send_message`
    (`ui/src/hooks/useChat.ts:445-446`, `ui/src/hooks/useWebSocket.ts:519-531`).
    It sends approvals as `approve_tool` and `reject_tool`
    (`useChat.ts:465`, `:485`; `useWebSocket.ts:535-565`).
  - The daemon's `/ws` handler (`pkg/api/server.go:95`) handles only
    `subscribe`, `unsubscribe`, `subscribe_steps`, `pause`, `resume`, `stop`
    and `ping` (`pkg/api/ws.go:154-240`). Anything else gets a `notice`,
    "unknown request type" (`ws.go:195-201`). No other Go code handles
    `send_message` or `approve_tool`.
  - The UI has no `notice` case and logs it as unknown
    (`useWebSocket.ts:331-332`), so the owner sees nothing.
  - Before anything is sent, the UI adds the user message locally
    (`useChat.ts:429-440`). It also marks the tool call
    `approvalStatus: 'approved'` (`useChat.ts:467-473`). CR:400 allows
    "Optimistic updates for tool approvals", but no `tool_call_update` ever
    confirms or reverses one (see U2).
  - The UI unit tests mock a server that speaks CR's protocol
    (`ui/src/hooks/__tests__/useWebSocket.test.ts:245`, `:316`, `:446`,
    `:486`), so they pass. No Go test drives `/ws`.
- Classification: **product**. The chat client and the daemon disagree; CR
  matches the client. Severity: **high**. The chat screen's main job, sending
  the owner's message, does not reach the daemon, and a rejected or ignored
  approval looks approved. Read from code, not executed.

**U2: The server→client messages the UI expects are never sent.**
- Contract: CR:380-385 lists `message`, `streaming_chunk`,
  `tool_call_update`, `event` and `error`.
- Evidence: the daemon sends `event` (`ws.go:173`), `step` (`ws.go:215`),
  `pong` (`ws.go:239`) and `notice` (`ws.go:198`). The UI's `message`,
  `streaming_chunk`, `tool_call_update` and `error` handlers
  (`useWebSocket.ts:286-314`) never fire against the daemon. `MessageList`
  and `StreamingMessage` (CR:116-119, CR:141-144) are filled only from REST
  history and the local message.
- Classification: **product**. Severity: **medium**. Read from code, not
  executed.

**U3: The UI's event names do not match the daemon's event names.**
- Contract: CR:104-107 says `LoopStatusBadge` shows "Running, Paused,
  Stopped, Complete, Error". CR:109-112 says `ChatActions` shows its buttons
  according to loop state. `ui/src/types/chat.ts:192` says the UI's event
  types "mirrors Go LoopEventType".
- Evidence:
  - No Go `LoopEventType` exists. A search of `*.go` for `LoopEventType` or
    `"loop.start"` returns nothing.
  - The daemon sends `loop.Event` with types such as `iteration_start`,
    `paused`, `complete` and `error` (`internal/loop/event.go:7-52`).
  - The UI changes loop state only for `loop.*` and `iteration.start`
    (`useChat.ts:257-259`, `:311-355`; `types/chat.ts:194-230`).
  - So daemon events never move the badge or the action buttons.
- Classification: **product** and **document** (`types/chat.ts:192`).
  Severity: **medium**. Read from code, not executed.

**U4: CR no longer describes the shipped tree, store, transport or keys.**
- Contract and evidence:
  - CR:314-317 and CR:341 describe a `store/` with a Zustand `chatStore.ts`.
    There is no `ui/src/store/` directory, and `ui/package.json` has no
    `zustand` dependency. State lives in hooks (`ui/src/hooks/useChat.ts`).
  - CR:73, CR:117, CR:214 and CR:397 say the lists are virtualized. No
    component imports a virtualization library, and `virtual` appears nowhere
    in `ui/src` components.
  - CR:365 gives the endpoint as `ws://{host}/api/chat/ws`. The UI connects
    to `/ws` (`ui/src/App.tsx:40`), which is the daemon's route
    (`pkg/api/server.go:95`).
  - CR:189 says "Ctrl+Enter to send". The UI sends on Enter without Shift
    (`ui/src/components/chat/input/InputTextarea.tsx:86-87`;
    `ChatInput.tsx:76`).
  - CR's tree (CR:247-337) leaves out the shipped `blueprint/`, `diff/` and
    `restart/` component folders and all three pages besides chat.
- Classification: **document**. CR was last changed on 2026-03-02;
  `pkg/api/ws.go` changed after that (2026-03-15). Severity: **low**.

**U5: Some exported components call endpoints the daemon does not serve, and
no page mounts them.**
- No clause. CR and AR do not mention these components or endpoints.
- Evidence:
  - The `restart/` and `diff/` backup and rollback components, and their
    hooks, call `/restart/requests…` and `/backups…`
    (`ui/src/hooks/useRestartApproval.ts:171-372`,
    `ui/src/hooks/useBackupHistory.ts:196`, `:241`).
  - No route for either is registered in `pkg/api` or `internal/server`.
  - No page imports these components (`ChatPage.tsx:11-26`); only
    `components/chat/index.ts` exports them.
- Label: **document gap** (dead component surface). Severity: **low**.

### Daemon API against AR

AR says it is partial and not re-verified (AR:3-5), and it excludes the MCP
tools (AR:5-11). The findings below are about what AR does assert.

**A1: AR documents routes the daemon does not serve.**
- Contract:
  - AR:442 `GET /api/v1/blueprints` and AR:471 `GET /api/v1/blueprints/{id}`
  - AR:508 `GET /api/v1/toolsets` and AR:535 `GET /api/v1/toolsets/{name}/tools`
  - AR:563 `POST /api/sessions/{id}/mode`
  - AR:600 and AR:822 say the legacy `/api/fwu/*` routes are "still
    available".
- Evidence: none of these is registered in `pkg/api/server.go:93-159` or
  `internal/server/server.go:415-443`. All of them reach the 404 catch-all
  (`internal/server/server.go:431`). `pkg/api/server.go:97` says "Legacy FWU
  routes REMOVED in Phase Four."
- Classification: **document**. Severity: **low**. Read from code.

**A2: AR's WebSocket section disagrees with `ws.go`.**
- Contract and evidence:
  - AR:764 says "Version fields are intentionally omitted from real-time
    WebSocket events". `ws.go:176-178` and `:218-220` send `prompt_version`,
    `tool_registry_version` and `run_state_machine_version` on every event.
  - AR:739-747 shows a flat event with `"type": "stage_start"` (AR:740). The
    daemon wraps every event as `{"type":"event","payload":…}`
    (`ws.go:173-175`).
  - AR:273 says to "subscribe to the run ID". `subscribe` reads `sessionId`
    (`ws.go:160`); a run ID works only through `subscribe_steps`
    (`ws.go:202-205`).
  - AR's table of contents links to "WebSocket Protocol" (AR:26) and "MCP
    Tools" (AR:27), and neither section exists.
- Classification: **document**. Severity: **low**. Read from code.

**A3: Starting a run from a session defaults to `danger-full-access`. AR
says the default is `workspace-write`.**
- Contract: AR:226 "| `mode` | string | `workspace-write` |".
- Evidence: `POST /api/v1/sessions/{id}/run` sets
  `mode = "danger-full-access"` when no mode is given
  (`pkg/api/handlers.go:861-865`). The handlers at `handlers.go:109`, `:558`
  and `:642` default the same way.
- This review does not settle whether the code or AR is right. A caller that
  trusts AR gets the most permissive mode.
- Not a duplicate: `handleStartRun`'s default (`handlers.go:178`) belongs to
  step 3 I3.2 and its repair. #135 fixed the chat CLI side
  (`internal/cli/chat.go`), not these handlers.
- Classification: **product** or **document**. Severity: **medium**. Read
  from code.

**A4: Much of the registered API is not in AR.**
- Contract: the routes AR documents. AR admits partial coverage (AR:3).
- Evidence: these routes are registered but not documented:
  - Runs: `POST /api/v1/runs`, `runs:execute`, `runs:blueprint`,
    `{id}/start`, `{id}/resume`, `{id}/checkpoints` and `runs/parallel`
    (`pkg/api/server.go:102-112`).
  - Approvals (`pkg/api/server.go:155-158`).
  - Health and readiness (`internal/server/server.go:427-428`).
  - Usage. AR's table of contents links to a usage section (AR:25) that does
    not exist.
- AR's `runs:plan` request shows only `intent_file` and `no_validate`
  (AR:619-622). `PlanRequest` also accepts `request_id`, `intent`,
  `auto_import`, `review` and `compact` (`pkg/manager/planner.go:29-35`).
- Classification: **document**, already admitted. Severity: **low**.

AR conforms on: session create, list and messages, with matching fields
(`pkg/api/sessions.go:15-18`, `:35-65`); projects, directories, providers and
models; the `runs:plan` response fields (`pkg/manager/planner.go:43-46`); and
DCP-only knowledge routes (`internal/server/server.go:420-423`), as AR:779
says.

### `pkg/execution` against EP

**E1: "Canonical working directory" is checked only for being non-empty.**
- Contract: EP:15 "Request ID, canonical working directory, prompt, model,
  sandbox, and writable roots are validated before starting".
- Evidence: `validateProviderRequest` checks that ID, working directory and
  prompt are non-empty (`pkg/execution/agent_cli_provider.go:237-239`). It
  does not make the directory absolute, clean it or resolve symlinks, and it
  does not check the model. Both providers call it
  (`agent_cli_provider.go:121`, `pkg/execution/api_provider.go:195`).
- Classification: **product**. Severity: **low**. Read from code.

**E2: The tool gateway does not validate the working directory.**
- Contract: EP:84-86 "A tool executor must validate the working directory,
  sandbox, and writable roots before the first model request".
- Evidence: `GatewayToolExecutor.ValidateAccess(workingDir, …)` checks the
  sandbox mode and writable roots and never reads `workingDir`
  (`pkg/execution/gateway.go:97-105`).
- Classification: **product**. Severity: **low**. Read from code.

**E3: EP describes protocol version 1; the stdio command speaks versions 1
to 3 and can exit without a result envelope.**
- Contract: EP:54 "exposes protocol version 1", EP:60, and EP:63 "Output
  contains zero or more event envelopes followed by exactly one result".
- Evidence:
  - The code defines versions 3, 2 and 1
    (`internal/cli/execution_stdio.go:30-32`).
  - If building the provider fails, the command returns a bare error with no
    result envelope (`execution_stdio.go:280-283`). An `execute` envelope with
    no request does the same (`execution_stdio.go:317-318`).
- Classification: **document** for the version; **product** for the missing
  result. Severity: **low**. Read from code.

**E4: An exported executor in the package uses a dangerous permission
bypass.**
- Contract: EP:77 "Neither provider uses a dangerous permission bypass." This
  is one of the declared prohibitions in S0.3.
- Evidence: `AgentCLIExecutor` accepts only `danger-full-access`
  (`pkg/execution/cli.go:34-37`). It runs Claude with
  `--dangerously-skip-permissions` (`cli.go:49`).
- Mitigation: it is not a `Provider`, so EP:77 holds for the two providers.
  A non-test search finds no caller outside `cli.go`.
- Classification: **product**, dead exported code. Severity: **low**. Read
  from code.

EP conforms on:
- explicit Claude permission and Codex sandbox
  (`agent_cli_provider.go:205-225`);
- refusal of unenforceable sandboxes and writable roots on read-only requests
  (`agent_cli_provider.go:241-254`);
- cancellation reported as `cancelled`;
- unknown protocol versions rejected (`execution_stdio.go:242-246`);
- workspace-write refused without a tool executor
  (`api_provider.go:215-217`).

The `pkg/execution` tests pass (see "Checks run").

### Blueprint actions against TA

TA has no status line. It is the only committed action contract.

**T1: Only one of TA's seven required v1 actions is registered, and no
built-in blueprint uses it.**
- Contract: TA:45-55 lists `build_context_pack`, `apply_patch`,
  `run_linters`, `run_tests`, `generate_diff_report`,
  `prepare_review_bundle` and `summarize_run`.
- Evidence:
  - The registry holds `run_gates`, `build_context`, `apply_patch`,
    `contracts.GenerateTests` and `checklist.Run`
    (`internal/actions/registry.go:51-58`).
  - Built-in blueprints use `build_context`, `contracts.GenerateTests`,
    `checklist.Run` and `run_gates` (`internal/blueprint/engine.go:127`,
    `:146`, `:193`, `:233`).
  - A blueprint that names a TA action, for example `action: run_tests`,
    fails closed: "declares action %q, which is not registered"
    (`internal/blueprint/executor.go:179-183`).
- Classification: **document**: TA describes a design that was never built.
  Severity: **medium**. An agent authoring a blueprint from TA gets a stage
  that always fails.

**T2: `apply_patch` reports success without applying anything.**
- Contract: TA:50 "`apply_patch` | Apply a structured patch artifact to the
  workspace. | `patch_applied`".
- Evidence: after checking that an `implementation` input is present, the
  action returns `StageStatusCompleted` with "Applied implementation changes
  to workspace" and no artifact (`internal/actions/apply_patch.go:35-39`).
  The comment says "Placeholder".
- No built-in blueprint uses it (T1), so only a custom blueprint can reach
  it.
- Classification: **product**. Severity: **low**. Read from code.

**T3: The request and result envelopes, the policy check and the lifecycle
are not implemented as TA describes.**
- Contract:
  - TA:16 lifecycle: Validate Schema → Check Policy → … → Persist Outputs →
    Emit Events.
  - TA:20-36 envelopes: `action`, `context.toolset`, `context.sandbox`;
    `status: "success"`, an `artifacts[].artifact_id` list and `summary`.
  - TA:40-43 `policy.evaluate` with `allow`, `block` or `require_approval`.
  - TA:64 timeout classes.
- Evidence:
  - `ActionRequest` has `RunID`, `WorkspaceDir`, `Inputs` and `Metadata`
    (`internal/types/types.go:27-32`). `ActionResponse` has `Status`,
    `Output`, `Error` and `Artifacts map[string]string`
    (`types.go:35-40`).
  - Statuses are `pending`, `running`, `completed`, `failed` and `skipped`
    (`types.go:18-24`).
  - The dispatcher calls `action.Execute` directly, with no policy call
    (`internal/blueprint/executor.go:113-129`).
  - The policy package's outcomes are `allow`, `ask` and `deny`
    (`internal/policy/rules.go:17-21`).
  - Timeouts are a raw `time.Duration` (`internal/blueprint/stage.go:37`).
- Classification: **document**. Severity: **low**.

**T4: The standard blueprint's lint and test stages are the shell commands
TA declares an anti-pattern.**
- Contract: TA:12 and TA:67 (S0.3).
- Evidence:
  - `lint` and `test` have no action (`internal/blueprint/engine.go:154-179`;
    "Set from project config", `:159`, `:176`).
  - The pipeline fills them from `execution.lint_commands` and
    `execution.test_commands` (`internal/pipeline/pipeline.go:484-491`).
  - The executor runs them as its "Fallback to shell commands"
    (`internal/blueprint/executor.go:141-178`).
- Mitigation: the commands come from operator-owned project configuration,
  not from the blueprint file or a model, and each run is reported through
  the evidence hooks (`executor.go:153-166`). They are not opaque in the
  sense of being unrecorded.
- Related to #131, which made the lint gate resolvable. #131 did not change
  this route, and no fix is proposed here.
- Classification: **document**. TA forbids what the default route does.
  Severity: **low**.

### MCP tools against LM, SK, A11 and A12

The documented and registered tools conform:
- the eight LM tools (LM:41-48 against `internal/mcp/server.go:310-317`);
- operator-only approval tools (LM:125-127 against `server.go:208`, `:338`;
  `internal/mcp/approvals.go:79`);
- every backlog handler reloads (LM:67-69);
- one story claimed at a time (LM:43);
- `_candidates` are never loaded (SK:8-9 against
  `internal/skills/registry.go:44`).

The `internal/mcp` tests pass (see "Checks run").

**M1: Anti-self-approval does not hold where infra applies run.**
- Contract:
  - A12:25 "The agent is physically blocked from calling any approval-bypass
    tools".
  - LM:126-127 "an agent must never be able to approve its own requests".
  - AG:254 "agent sessions must never self-approve".
- Evidence:
  - Infra tools require `danger-full-access` (`internal/mcp/broker.go:193-197`).
  - In that mode `run_shell_command` is unrestricted (`broker.go:233-236`)
    and runs through `bash -c` (`internal/mcp/server.go:917`), with
    caller-supplied environment variables (`server.go:925-931`).
  - The same agent session can therefore run `openexec approve yes <id>
    --local` (`internal/cli/approve.go:184-186`), or start an MCP server with
    `OPENEXEC_OPERATOR_SESSION=1` (`server.go:208`).
  - The approval MCP tools are gated correctly; the shell route around them
    is not.
- No test covers the shell route. This pass did not try it.
- Classification: **product**. Severity: **medium**. Read from code, not
  executed.

**M2: `US-MAINT` "can never be completed" holds only in the MCP handler.**
- Contract: AG:241 says the maintenance story "can never be completed".
- Evidence:
  - `backlog_complete_story` refuses the maintenance story
    (`internal/mcp/backlog.go:568-570`).
  - `release.Manager.CompleteTask` auto-merges a complete story
    (`internal/release/automerge.go:71-92`) and sets it done
    (`automerge.go:251`), with no maintenance check. This runs only when
    `auto_merge_stories` is set (`internal/cli/config.go:23`;
    `internal/cli/release.go:782`).
- Classification: **product**, opt-in. Severity: **low**. Read from code.

**M3: `backlog_complete_task` does not check the verification its contract
names.**
- Contract: LM:44 "Mark a task done after its verification passes".
- Evidence: the handler sets the task done directly
    (`internal/mcp/backlog.go:532`). It does not run or check
    `verification_script`.
- LM does not say whether the tool or the client verifies.
- Classification: **document**: the wording reads as a guard the tool does
  not apply. Severity: **low**.

A11 states no component prohibitions. A12's tier table names tools the MCP
plane does not ship (`glob`, `grep`, `git_push`). That is a document detail,
not a separate finding.

### Terminal UI (contract: its own help text; no design document)

The TUI's only declared contract is its command description
(`internal/cli/tui.go:13-14`) and its help text (`internal/tui/help.go`).
Neither is a design document, so these findings are labelled
**no design-document clause**.

**TU1: "View project logs" shows no project logs.**
- Clause: `internal/tui/help.go:23` "Enter - View project logs"; the
  command's "worker activity" (`internal/cli/tui.go:14`).
- Evidence:
  - Enter only sets `logViewMode` and ignores the selected project
    (`internal/tui/app.go:196-199`).
  - The log buffer is filled by `ErrorMsg` (`app.go:88-90`) and by
    `ProjectUpdateMsg` (`app.go:234`).
  - No code builds a `ProjectUpdateMsg`, and `App` never calls
    `Source.Subscribe` (`app.go:285-296`). So the view shows only TUI errors.
- Classification: **product**. Severity: **low**. Read from code.

**TU2: "? - Toggle help" does nothing.**
- Clause: `internal/tui/help.go:28`, `:37`.
- Evidence: the `?` key handler is empty, with the comment "Toggle help - not
  implemented yet" (`internal/tui/app.go:205-206`). The dashboard always
  renders the full help (`app.go:124`).
- Classification: **product**. Severity: **low**. Read from code.

**TU3: The TUI opens SQLite without `sqlitecfg.DSN`, against a declared
repository rule.**
- Contract: AG:289 "**SQLite: always open via `sqlitecfg.DSN(path)`** …
  fixed across all open sites".
- Evidence:
  - `sql.Open("sqlite", dbPath+"?mode=ro")`
    (`internal/tui/file_source.go:265`). The sibling site
    `internal/cli/symbol_index.go:79` opens
    `"file:"+path+"?mode=ro"` the same way. Every other non-test open site
    uses `sqlitecfg.DSN`.
  - Without the DSN the connection has no busy timeout. Any error makes
    `readProgressFromSQLite` return false (`file_source.go:266-281`), and the
    TUI falls back to `tasks.json` or shows no progress
    (`file_source.go:201-222`).
- Classification: **product** and **document** (AG's "all open sites").
  Severity: **low**. Read from code, not executed.

## Checks run

All commands ran in this task's worktree at `5589b5e1` with a clean tree.

| Command | Exit | Result |
|---|---|---|
| `go test ./pkg/api/... ./pkg/execution/... ./internal/tui/... ./internal/mcp/... -count=1` | 0 | `ok pkg/api 3.445s`; `ok pkg/execution 2.291s`; `internal/tui [no test files]`; `ok internal/mcp 11.426s` |
| **ui/** `node node_modules/typescript/bin/tsc --noEmit` (the `type-check` script) | 0 | no output |
| **ui/** `node --max-old-space-size=8192 node_modules/vitest/vitest.mjs run --silent --reporter=dot --fileParallelism=false --maxWorkers=1 --minWorkers=1` (the `test:run` script, serial as in `make test`) | 0 | `Test Files 44 passed (44)`, `Tests 661 passed (661)`. The run printed "WebSocket server error: Port is already in use" once and still passed |

What the checks show and do not show:
- These tests prove the components work against their own mocks. They do not
  exercise U1-U3, because the UI tests mock CR's protocol rather than the
  daemon's (U1).
- `internal/tui` has no tests.
- Every finding above was **read from code, not executed**. No daemon, CLI
  binary, browser or model was run.
- Playwright was not run. It is optional in the contract. The hermetic D4/D6
  journeys run in `make test` and CI. The generic suite needs a disposable
  initialized project, and AGENTS.md records it as 71 passed, 2 failed.

**Citations.** Every `path:line` was read with `cat -n` / `sed -n` in this
worktree, whose HEAD is `5589b5e1` and whose tree is clean, so those reads
are the same as `git show 5589b5e1:<path>`. Agent Console
`playbook.go:270-276` was read from the local Console checkout, not from
native main.

## Overlaps with steps 2 and 3, and landed repairs

Main does not record which step-2/3 finding each repair task owns. The repair
tasks named in the contract are 12f61, 081af, 84fb8, 0bc4, 8c058, 4715, ec948
and f519, plus Console 684d and 61e and the blocked originals. This table
therefore matches on finding content and on the native PRs that have landed
since triage.

| Earlier finding / landed PR | Relation here |
|---|---|
| Step 2 F5.1 task-count progress (TUI, `status`) | Same TUI surface (§1); not repeated |
| Step 2 F4.5 TUI counts `completed` | Same function as TU3; TU3 is the connection rule, not the status set |
| Step 3 I3.2, chat escalation; #135 | A3 is a different route (`sessions/{id}/run`). `handleStartRun`'s default stays with I3.2 |
| Step 3 I1.4 unreachable fix stages | T4 is the same stages' shell commands, judged against TA; not a fix proposal |
| Step 3 I6.1 legacy symbol routes | Knowledge Hub (§1) reads the same DCP routes; not repeated |
| Step 3 I1.2 / #134, F2.1 / #130, F3.3 / #132, I6.3 / #136, I6.4 / #137 | Landed since triage; nothing here touches them |
| #131 lint gate resolvable | Related to T4; distinct |
| Step 2 F1.1 `approve` advertises a Task-mode approval path | M1 uses `approve --local` as the bypass route; the defect is the missing session separation, not the advertisement |

No finding here re-proposes a fix owned by those tasks or by the Console tasks
684d and 61e. Every finding is in native source or native documents.

## Limitations

- **No layer-3 screen document exists** (S0.1). The screens in §1 are listed,
  not graded. If the owner writes a layer-3 document, §1 has to be redone
  against it.
- **No declared UI anti-patterns.** Only the prohibitions in TA, EP, A12, LM
  and AG were checked (S0.3).
- **Nothing ran against a live daemon, browser or model.** U1-U3 are the most
  consequential findings and are the most in need of a reproduction. A
  browser or WebSocket check is the natural next evidence.
- **Reviewed at `5589b5e1`, not at triage's `c8a3532b`.** The seven commits
  in between were checked for overlaps.
- **L, G and C were not opened.** No finding rests on them alone, so none is
  N↔L.
- **The Agent Console checkout is stale** (step 1). It was read only for the
  playbook rule.
- **Fixes are out of scope.** The product findings are engineering defects
  inside accepted work and can be filed as native tasks without an owner
  question. Whether A3's code or document is right is an engineering call
  for that task.

## Downstream

The playbook order (Console `internal/playbook/playbook.go:251-301`) is:
- interaction (step 3, on `origin/main`) → interface (this report);
- → accessibility (`869ef988…`), which depends on this report;
- → the consolidating report (`8a800117…`).

Each keeps its dependency on the step before it. Step 5 has two web screens
and a TUI to check. CR's "Accessibility" section (CR:387-393) is the only
declared accessibility clause for them.
