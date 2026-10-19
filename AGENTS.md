<!-- owner-model:generated — do not edit. The shared rules come from the owner
     model; put anything specific to this repository in AGENTS.local.md
     and it is preserved across updates. -->

# Working agreements

- NOTES.md is this project's working memory. Capture tasks under `## Now`,
  open questions under `## Questions`. Items under `## For me` are reserved
  for the owner — never execute them.
- State what you verified and how. Unrun code is "unverified", not "works".
- The project's purpose and constraints live in the console's Project
  context — read it before large changes.
- Do not prefix a command with environment variables (`FOO=1 python3 x.py`).
  Permission rules match the start of the command, so the prefix hides the
  real one and an allowed command is refused. Take configuration as a CLI
  flag or read it from a file instead.

## Finishing a request

A request is delivered when the whole thing works end to end, not when each
part exists. This is where AI-assisted work fails: features get built,
individually plausible and individually tested, while the request they came
from is never exercised from one end to the other. Half of a request is not
progress on it.

Before reporting a request done:

- Walk the user's path yourself, in the running system, start to finish — the
  same steps the user described, not a unit test standing in for them.
- Exercise the branches that should fail, not only the happy one. A rule that
  is never refused is a rule that was never applied.
- Check the state actually persisted: reload, re-read the file, re-fetch the
  API. Written is not saved, and saved is not round-tripped.
- Verify the state you built for is reachable. A feature guarding a condition
  the system cannot enter is dead code that reviews as complete.
- If part of it cannot be finished, say which part and why, in the same
  message. Do not hand back a half-built request as if it were whole, and do
  not ask permission for the remaining half instead of doing it.

## Long-running work

A command that may run for more than a few minutes must not be run inline. The
session that started it will end — you will be compacted, the chat will close,
the ssh connection will drop — and the work dies with it, silently, with no
record that it was ever started.

Hand it to the runner instead, then end your turn:

    python3 /mnt/data1/projects/owner-model/job_runner.py start \
        --name "arabia harvest" --project arabia --cwd "$PWD" -- <command>

It returns immediately. The job outlives this session, reports itself into the
owner's attention inbox while it runs, and reports again when it finishes or
fails. A run whose process disappears is reported as lost rather than staying
"running" forever.

Build the flow yourself — the runner has no job definitions and needs none.
Write whatever script the work requires, and give it this exit contract so the
runner can drive it one chunk at a time:

    exit 0  -> finished, nothing left to do
    exit 3  -> did one chunk, call me again
    other   -> failed, stop and report

Chunks matter for anything long: each one is a place to resume from, so an
interrupted job keeps its progress instead of starting over. A command with no
chunking still works — it just runs once.

Say in your reply that the job was started and that its result will arrive in
the inbox. Do not poll it, and do not keep the session alive waiting for it.

# Repository Guidelines

`AGENTS.md` is the shared agent-facing instructions file. The owner-model
scaffolder renders its shared template followed by `AGENTS.local.md` verbatim.
Maintain repository guidance in this generation input and regenerate the
corresponding block in `AGENTS.md`; preserve the shared Working agreements.
Keep `CLAUDE.md` exactly `@AGENTS.md` plus a newline, without separate guidance.

## Autonomy architecture

Before changing planning, execution, continuation, recovery or delivery, read
`docs/OPENEXEC_SIMPLE_LOOP_ARCHITECTURE_CONTRACT.md`. Restate the user outcome,
observed state, owning loop/boundary, existing OpenExec primitive to reuse and
why no new abstraction is necessary. One native implementation loop, one outer
Goal review, deterministic delivery. Report the complexity delta in reviews.

## Project Structure & Module Organization
OpenExec is a Go monorepo with a separate React UI. The CLI entry point is `cmd/openexec/main.go`. Core backend code lives under `internal/` for app-private packages and `pkg/` for reusable/public packages. Frontend code is in `ui/src`, end-to-end tests are in `ui/e2e`, and static assets are in `ui/public`. Operational docs live in `docs/`, while helper scripts and release tooling live in `scripts/` and `bin/`.

## Build, Test, and Development Commands

All commands below start at the repository root unless marked **ui/**. For a
**ui/** command, change to `ui/` first; do not nest repeated `cd ui` calls.
Use Go 1.25+, Node.js 22 with npm, Python 3, Git (including repository history
for regression negative controls), and a POSIX shell/Make. Download Go modules
with `go mod download`; install the locked UI dependencies with **ui/** `npm ci`.
In Console worktrees, use the provided exact-lockfile `ui/node_modules` link;
never install into or modify that shared directory.

| Command | Purpose / additional prerequisites |
| --- | --- |
| `make build` | Build UI, then `bin/openexec` with `ui_dist` and version metadata. |
| `go build -buildvcs=false -o openexec ./cmd/openexec` | Direct backend binary; uses tracked fallback UI, not the built UI. The explicit flag also supports linked worktrees. |
| `make test` | Python defect negative controls, D6 Chromium journey, Go tests, D2 wiring, serial Vitest and D4 Chromium journey; not just unit tests. Install Chromium first (below). Needs free loopback ports 18080/13001; D6 uses an ephemeral port. |
| `go test ./...` | Go tests only; provider-dependent tests may need network/credentials or skip without them. |
| `go test -v ./internal/loop/... -run TestLoop` | Focused loop tests. |
| `make compat-test` | Existing-project regression gate: `go test ./internal/validation/... -run Compatibility -v`. |
| `make lint` | Go vet, optional installed golangci-lint, UI ESLint. |
| `make type-check` | Go build and UI TypeScript checks. |
| `make build-all` | Docker release build/extraction to `dist/`: Linux and macOS amd64/arm64, Windows amd64. Requires Docker daemon, base-image network access and build disk space. Known baseline failure: `Dockerfile.build` references `golang:1.25-bullseye`, which the registry returned as not found during instructions verification; do not claim release builds pass. |
| **ui/** `npm run build` | TypeScript compile and Vite production assets. |
| **ui/** `npm run lint` | ESLint; warnings fail. |
| **ui/** `npm test` | Vitest watch mode; stop with Ctrl-C after startup/tests. |
| **ui/** `npx vitest run --fileParallelism=false --maxWorkers=1 --minWorkers=1` | Bounded serial UI suite, also used by `make test`. |
| **ui/** `npm run test:coverage` | Vitest run with V8 coverage. |
| **ui/** `npm run dev -- --port 3001` | Vite HMR server; stop with Ctrl-C after use. Needs free port 3001. |
| **ui/** `npx playwright install chromium` | Browser installation for Make/browser tests; Linux system libraries must also be installed by the host administrator. |
| **ui/** `npm run test:e2e` | Generic Playwright suite; see the disposable project setup below. |

Generic E2E (`ui/playwright.config.ts`) starts the built backend on port 8080
and Vite on 3001. It expects `../initial` relative to the repository root as
backend data. In a fresh disposable checkout, create a disposable sibling
`initial` directory. Build with `make build`, then initialize the disposable
checkout itself with `./bin/openexec init --yes`: `start` loads project config
from its working directory even when `--data-dir` is supplied
(`internal/cli/start.go`). Never initialize an owner checkout or reuse owner
project data for this suite. Ensure both ports are free: the config reuses
existing servers, so occupied ports can silently select the wrong instance.
The hermetic D4/D6 browser journeys in `make test` use their own fixtures and
need no sibling project or live provider. Do not assume generic E2E passes
merely because those journeys pass. Fresh disposable-project verification of
the generic suite returned 71 passed and 2 failed (D4 under the generic config
and project initialization); report these as failures, not a passing E2E gate.

CI (`.github/workflows/ci.yml`) uses Go 1.25 and Node 22 on `ubuntu-latest`
and `macos-latest`; it builds embedded UI, runs Go/UI/compatibility tests and
runs advisory golangci-lint. Local Make tests additionally run defect gates.
A command's presence here does not assert a green baseline: report its actual
output and exit status, including environmental failures and skips.

## Coding Style & Naming Conventions
Format Go code with `gofmt` and keep packages lowercase and focused. Preserve the existing layout: exported identifiers use `CamelCase`, internal helpers use `camelCase`, and tests stay next to implementation files as `*_test.go`. In the UI, use TypeScript with React function components, `PascalCase` page/component names, and `camelCase` hooks/utilities such as `useSession.ts` and `formatters.ts`. Lint UI changes with the UI lint command above.

## Testing Guidelines
Backend tests use Go’s standard `testing` package and run with `go test ./...`. Frontend unit tests use Vitest; end-to-end coverage uses Playwright. Name UI tests `*.spec.ts` or `*.test.ts(x)` and keep them close to the feature or under `ui/e2e` for browser flows. Run targeted checks before opening a PR, then finish with `make test`, `make compat-test`, and `make type-check` when the change touches project loading, migration, self-healing, or legacy behavior.

## Anti-Regression Policy
Do not merge a change that modifies compatibility-sensitive behavior without proving support was preserved. For each such change, do one of these: add or update automated coverage that directly exercises the changed path, or record a short compatibility evaluation explaining why existing project support cannot have dropped. Treat existing `.openexec` projects, legacy `.uaos` projects, and migration fallbacks such as `.openexec/tasks.json` as protected behavior until explicitly deprecated.

## Commit & Pull Request Guidelines
Recent history follows short Conventional Commit prefixes such as `fix:`, `docs:`, and `release:`. Keep subjects imperative and under one line, for example `fix: restore quality gate wiring`. PRs should describe scope, note any config or schema changes, link related issues, and include screenshots or recordings for visible UI changes. Call out any skipped tests or follow-up work explicitly.

## Security & Configuration Tips
Do not commit secrets, local caches, or generated artifacts. The repo already contains transient directories like `ui/dist`, `ui/coverage`, and `.gocache`; treat them as disposable output, not source.

## Architecture Overview

CLI forms in this overview describe runtime interfaces, not build/check setup
commands. Use them only in an initialized project with the required provider,
operator configuration and effect authority; the guidance grants no new effects.

OpenExec is a **single-binary AI orchestration framework** that transforms high-level intent into production code through a deterministic pipeline. The native loop and delivery boundaries are defined in `docs/OPENEXEC_SIMPLE_LOOP_ARCHITECTURE_CONTRACT.md`; do not infer provider selection or authority from this overview.

### Three Execution Modes
| Mode | Description | Side Effects |
|------|-------------|--------------|
| **Chat** | Conversational, workspace read-only | Orchestrator bookkeeping may still change (see Light Mode) |
| **Task** | Scoped action, produces artifacts | Creates files/patches |
| **Run** | Blueprint execution over task | Full automation |

### Core Execution Flow (Blueprint Mode)
```
CLI Command → Manager → Loop → Blueprint Engine → Stages → AI Provider
                ↓                    ↓
         SQLite State        DCP (routing)
```

### Blueprint Stages
The core blueprint flow is shown below; configured blueprints can add validation and repair stages (see `internal/blueprint/`). Tasks progress through: **gather_context → implement → lint → test → review**
- **gather_context** (deterministic): Gather relevant files and context
- **implement** (agentic): Implement the requested changes
- **lint** (deterministic): Run linting checks
- **test** (deterministic): Run tests
- **review** (agentic): Review changes and generate summary

### Key Packages

| Package | Purpose |
|---------|---------|
| `cmd/openexec/` | Entry point, calls `cli.Execute()` |
| `internal/cli/` | Cobra commands (init, plan, start, run, chat, doctor) |
| `internal/loop/` | Core iteration engine - spawns AI, parses events, blueprint execution |
| `internal/blueprint/` | Stage-based execution engine with checkpoints and retries |
| `internal/dcp/` | Deterministic Control Plane - thin tool-routing layer |
| `internal/project/` | Project config loading with legacy `.uaos/` fallback |
| `internal/validation/` | E2E and compatibility regression tests (CI gate) |
| `internal/toolset/` | Toolset definitions and registry |
| `internal/context/` | Two-stage context assembly (deterministic + LLM ranking) |
| `internal/mcp/` | Model Context Protocol server (JSON-RPC stdio) |
| `internal/planner/` | Intent → goals/stories/tasks generation (prompts enforce vertical slices + afk/hitl tagging) |
| `internal/prompt/` | Prompt assembly from personas/workflows/manifests |
| `internal/release/` | SQLite-backed task/story state management |
| `pkg/agent/` | AI provider adapters (anthropic, openai, gemini) |
| `pkg/manager/` | Multi-pipeline orchestrator |
| `pkg/api/` | HTTP handlers and WebSocket |
| `ui/` | React 18 + TypeScript + Vite (embedded in binary) |

### Toolsets
Tools are grouped into toolsets by function and risk level:

| Toolset | Risk | Description |
|---------|------|-------------|
| `repo_readonly` | Low | Read operations only |
| `coding_backend` | Medium | Backend implementation |
| `coding_frontend` | Medium | Frontend implementation |
| `debug_ci` | Medium | CI/CD debugging |
| `docs_research` | Low | Documentation and research |
| `release_ops` | High | Release operations |

### Agent Definitions
Agent personas, workflows, and manifests live in `agents/`:
- `agents/personas/` - Role definitions (YAML)
- `agents/workflows/` - Prompt templates
- `agents/manifests/` - Agent metadata linking persona to workflow

### Task Execution Modes (afk/hitl)
Planner-generated tasks carry `"mode": "afk" | "hitl"` (story-generation prompt rule 12), stored in `release.Task.Metadata["mode"]` and read via `Task.ExecutionMode()` (defaults to afk). The batch scheduler (`pkg/manager/scheduler.go`, `ExecuteTasks`) never auto-dispatches hitl tasks and transitively holds back their dependents (task- and story-level) so the dependency resolver cannot deadlock. Single-task runs via `Manager.Start` are not gated — humans run hitl tasks individually. Surgical-scope chassis merging (`internal/planner/postprocess.go`) inherits hitl if any merged task was hitl.

### Vertical Slices (task decomposition)
The story-generation prompt (`internal/planner/prompt.go`, rules 4–5) decomposes complex work into **vertical slices**: each task crosses every layer it needs (schema → service → UI) and ends in something runnable; layer-by-layer (horizontal) plans and Diagnose/Implement/Verify phase tasks are rejected by the story review prompt. The first slice of a story must be a tracer bullet — the thinnest end-to-end path.

### Light Mode (story backlog over MCP)
`openexec mcp-serve` exposes the story backlog to external MCP clients (Claude Code) without booting the daemon: `backlog_list_stories`, `backlog_get_story`, `backlog_claim_story` (one story in progress at a time), `backlog_complete_task`, `backlog_complete_story`, `memory_read`. Implementation in `internal/mcp/backlog.go`. Two invariants: (1) every handler calls `Load()` — mcp-serve is long-lived and other processes write the same `.openexec/openexec.db`, so cached state must never be served; (2) backlog writes are allowed in **all** permission modes including read-only chat — they mutate orchestrator bookkeeping, not workspace files (documented exception, see `docs/LIGHT_MODE.md`).

### Project Phases (two-speed routing signal)
`release.ComputePhase(stories)` / `Manager.Phase()` derive a phase from backlog state: `new` (no plan) → `planned` (plan exists, nothing done) → `building` (initial build underway) → `maintaining` (all stories done — heavy lifting complete). `backlog_list_stories` returns `phase` (for clients/UI), and `openexec chat` prints a light-mode hint in `maintaining` phase before booting the engine. Phase is guidance for lane selection, never a gate.

**Re-planning** (heavy phase rerun, e.g. a refactor epic after `maintaining`): just run planning again. The generator numbers from US-001/G-001, so `planner.RemapPlanIDs` (called by `importPlan`) remaps colliding IDs to free ones — same-ID items are reused only when full canonical content matches (including parents, ordered task membership, acceptance criteria, dependencies, strategy, priority and mode). Nil, JSON null (text or BLOB), SQL NULL and empty lists are equivalent; nonempty list values and order remain significant. Changed content gets fresh IDs with goal/story/task references following, even when titles match. Exact replay preserves retained rows, completed metadata, commits and reviewed import receipts without duplicating work. New stories append to the backlog and phase automatically returns to `planned`/`building`.

**Once-heavy-then-light lifecycle support** (see `docs/LIGHT_MODE.md`): (1) study/terminus stories must write `docs/ARCHITECTURE.md` — the knowledge base is not readable outside OpenExec (planner prompt rules 3/10, `postprocess.go` injected study story); (2) pending hitl tasks block `maintaining` — `backlog_list_stories` reports `hitl_pending` with handoff guidance; (3) `backlog_add_task` files light-mode surgical work into the rolling `US-MAINT` story (`StoryTypeMaintenance`), which is excluded from phase computation, the one-story claim rule, and can never be completed; (4) `openexec skills promote <name>` copies a project skill to `~/.openexec/skills/user/` so lessons compound across projects.

### Review-Stage Convention Push
The blueprint review-stage prompt force-injects active project skills as review criteria (`projectConventionsSection` in `internal/blueprint/executor.go`, size-capped). Push is review-only: a reviewer cannot verify compliance with standards it never loaded, while implement stages keep pull-based skill routing. Unapproved `_candidates` are never injected. Schema↔struct consistency for all MCP tools is enforced by `internal/mcp/schema_audit_test.go` — extend `allToolDefs()` and the struct-pair list when adding tools.

### Skill Synthesis (propose-then-approve)
Agents capture durable project lessons via the `skill_propose` MCP tool (`internal/mcp/skills.go`). Proposals are written to `.openexec/skills/_candidates/<name>/SKILL.md` (`internal/skills/candidates.go`) and are **never** loaded by the registry — `LoadFromDir` skips `_`-prefixed directories — until a human runs `openexec skills approve <name>` (also: `skills proposals`, `skills reject`). Candidate names are a security boundary (kebab-case regex; they become directory components). The asymmetry is deliberate: agents may propose in any mode, only humans activate — an unsupervised agent must never be able to poison future runs with a wrong lesson.

### Infra Tools (SRE command registry — Phases 1–4 shipped)
`internal/infra` + `internal/mcp/infra.go` implement the deny-by-default SRE command registry (`docs/SRE_ORCHESTRATION_ROADMAP.md`). The operator-owned allowlist `.openexec/infra.yaml` (never the init-generated `openexec.yaml`; unknown keys fail the load) compiles into parameter-bounded MCP tools: `ansible_run_playbook`, `salt_apply_state` (apply-class), `ssh_run_query`, `terraform_plan`, `terraform_apply`. Execution is `exec.CommandContext` argv arrays — no shell, validate-and-reject (never sanitize), playbooks resolved as basenames against the configured dir. All infra tools require danger-full-access mode and are exempt from coding-toolset filtering (separate capability plane); infra executions are never idempotency-marked (run_shell_command precedent).

**Saved-plan terraform (P2):** `terraform_plan save=true` writes the fixed `openexec-<env>.tfplan`; `terraform_apply` inspects it via `show -json`, detects destructive actions deterministically (`internal/infra/tfplan.go` — replace is the `["delete","create"]` pair, never a literal "replace"), surfaces findings to the approver, and applies exactly that plan (TOCTOU-safe). Unverifiable plans are refused.

**Approval & tiering (P3):** apply-class invocations request approval in-handler (dry-runs `check`/`test` don't) through `internal/approval.PersistentGate` (`.openexec/approvals.db`, cross-process polling, default wait 5m / `OPENEXEC_APPROVAL_WAIT` — the MCP *client's* tool-call timeout also bounds the wait; align both for longer reviews), failing closed without a gate. `risk_profile: low` environments run applies autonomously; destructive terraform changes always need a human. Sign-off: `openexec approve list|yes|no <id> --local`, or `approval_list`/`approval_decide` MCP tools in an `OPENEXEC_OPERATOR_SESSION=1` server only (never reuse backlog tools for approval — their read-only-mode exception would become an authorization bypass; agent sessions must never self-approve).

**Intent routing (P4, suggest-only):** `internal/tools.NewInfraSuggestTools` registers routing targets in the DCP coordinator (behind `OPENEXEC_ENABLE_DCP`); their `Execute` always refuses — router output is an untrusted proposal, execution happens only via the MCP plane.

### Smart-Zone Budget (context size flagging)
Agentic runners report the **peak single-call context size** (input + cache tokens of one call — never a cumulative sum across turns, which would overcount) as the `peak_context_tokens` artifact: the CLI parser extracts it from assistant-message usage, the API runner tracks it across turns. `blueprint.DefaultExecutor` flags stages whose peak exceeds `SmartZoneTokens` (0 → `DefaultSmartZoneTokens` 100k, −1 disables) with a `smart_zone_exceeded` artifact, "task is too big, split it" diagnostics, and a `[SmartZone]` log line. It is a flag, not a failure — the stage still completes.

### State & Persistence
- **SQLite**: Canonical goal/story/task state is `.openexec/openexec.db` (`internal/release/manager.go`). Audit logger storage is configured by the caller (`pkg/audit/logger.go`); the server shares its state DB (`internal/server/server.go`). Do not use the historical `.openexec/data/audit.db` path as the canonical task ledger.
- **Tract**: Legacy terminology, not a required storage microservice; current release state uses SQLite.
- **Config**: `.openexec/config.json` for project settings

### Backward Compatibility: `.uaos/` → `.openexec/`
Project loading (`internal/project/`) uses dual-path reading: first tries `.openexec/config.json`, then falls back to `.uaos/project.json` (legacy format). The legacy `tasks.json` fallback for progress calculation is also preserved. This backward compatibility is enforced by `make compat-test` in CI.

## Engineering Mandates

Compatibility and commit requirements are in Anti-Regression Policy and Commit & Pull Request Guidelines above.

### Observe, then Resolve
To prevent thrashing during task execution:

**Error Diagnostics**
- If a test fails, rerun with `--verbose` or `screen.debug()` before attempting fixes
- State a clear hypothesis before modifying code
- If a change doesn't fix the error after one attempt, **REVERT** before trying a different strategy

**UI Testing (React/Vitest)**
- Use `findBy*` for elements appearing after async actions
- Use `userEvent` over `fireEvent` for proper event bubbling
- Wait for state transitions with `waitFor()`, never `setTimeout`
- Verify API schemas in `pkg/api/` and `ui/src/types/` before implementing UI
- Ensure mocks match current API response format (snake_case vs camelCase)

### Known Quirks
- **SQLite: always open via `sqlitecfg.DSN(path)`** (`pkg/db/sqlitecfg`). The driver registered as `"sqlite"` is modernc.org/sqlite, which **silently ignores** mattn-style DSN params (`?_journal_mode=WAL`) — connections opened that way ran with journal_mode=delete, foreign_keys=0, busy_timeout=0 (verified empirically; fixed across all open sites). modernc needs `_pragma=journal_mode(WAL)` syntax, which it applies per pooled connection. `TestDSNAppliesPragmas` guards the runtime state.
- **JSDOM limitations**: Doesn't fully simulate layout events (onMouseEnter). Check if failing tests depend on layout properties
- **Task progress**: Read `.openexec/openexec.db`; the historical `.openexec/data/audit.db` path is not the task ledger.
- **Go version**: Requires Go 1.25+
- **CI matrix**: See the build section; runner labels are `ubuntu-latest` and `macos-latest`, not a pinned Ubuntu 22.04.

### Learning Loop
When solving complex bugs, persist lessons to `.openexec/engram/learning_log.json` (a project-local notes file, not an automatically loaded skill registry). Use the propose-then-approve skill path above for reusable agent instructions; never activate a candidate yourself.
