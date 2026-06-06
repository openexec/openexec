# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

```bash
# Build
make build                    # Build binary → bin/openexec
go build -o openexec ./cmd/openexec  # Alternative direct build

# Test
make test                     # All tests (Go + UI)
go test ./...                 # Go tests only
go test -v ./internal/loop/... -run TestLoop  # Single test/package
cd ui && npm test             # UI tests (watch mode)
cd ui && npx vitest run --fileParallelism=false  # UI tests (CI mode)

# Compatibility regression tests (CI gate — required before merging)
make compat-test              # Runs: go test ./internal/validation/... -run Compatibility -v

# Lint & Type Check
make lint                     # Go vet + golangci-lint + UI ESLint
make type-check               # Go build check + UI tsc
cd ui && npm run lint         # UI only

# UI Development
cd ui && npm install && npm run dev -- --port 3001  # Dev server with HMR
```

## Architecture Overview

OpenExec is a **single-binary AI orchestration framework** that transforms high-level intent into production code through a deterministic pipeline. It follows the **converged architecture** pattern: deterministic local runtime with small local LLM as gatekeeper and frontier model for hard reasoning.

### Three Execution Modes
| Mode | Description | Side Effects |
|------|-------------|--------------|
| **Chat** | Conversational, no side effects | None |
| **Task** | Scoped action, produces artifacts | Creates files/patches |
| **Run** | Blueprint execution over task | Full automation |

### Core Execution Flow (Blueprint Mode)
```
CLI Command → Manager → Loop → Blueprint Engine → Stages → AI Provider
                ↓                    ↓
         SQLite State        DCP (routing)
```

### Blueprint Stages
Tasks in blueprint mode progress through: **gather_context → implement → lint → test → review**
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

**Re-planning** (heavy phase rerun, e.g. a refactor epic after `maintaining`): just run planning again. The generator numbers from US-001/G-001, so `planner.RemapPlanIDs` (called by `importPlan`) remaps colliding IDs to free ones — same-ID + same-title items are treated as identical and skipped (idempotent re-import), different-title collisions get the next free ID with goal/story/task references following. New stories append to the backlog and phase automatically returns to `planned`/`building`.

**Once-heavy-then-light lifecycle support** (see `docs/LIGHT_MODE.md`): (1) study/terminus stories must write `docs/ARCHITECTURE.md` — the knowledge base is not readable outside OpenExec (planner prompt rules 3/10, `postprocess.go` injected study story); (2) pending hitl tasks block `maintaining` — `backlog_list_stories` reports `hitl_pending` with handoff guidance; (3) `backlog_add_task` files light-mode surgical work into the rolling `US-MAINT` story (`StoryTypeMaintenance`), which is excluded from phase computation, the one-story claim rule, and can never be completed; (4) `openexec skills promote <name>` copies a project skill to `~/.openexec/skills/user/` so lessons compound across projects.

### Review-Stage Convention Push
The blueprint review-stage prompt force-injects active project skills as review criteria (`projectConventionsSection` in `internal/blueprint/executor.go`, size-capped). Push is review-only: a reviewer cannot verify compliance with standards it never loaded, while implement stages keep pull-based skill routing. Unapproved `_candidates` are never injected. Schema↔struct consistency for all MCP tools is enforced by `internal/mcp/schema_audit_test.go` — extend `allToolDefs()` and the struct-pair list when adding tools.

### Skill Synthesis (propose-then-approve)
Agents capture durable project lessons via the `skill_propose` MCP tool (`internal/mcp/skills.go`). Proposals are written to `.openexec/skills/_candidates/<name>/SKILL.md` (`internal/skills/candidates.go`) and are **never** loaded by the registry — `LoadFromDir` skips `_`-prefixed directories — until a human runs `openexec skills approve <name>` (also: `skills proposals`, `skills reject`). Candidate names are a security boundary (kebab-case regex; they become directory components). The asymmetry is deliberate: agents may propose in any mode, only humans activate — an unsupervised agent must never be able to poison future runs with a wrong lesson.

### Infra Tools (SRE command registry — Phase 1)
`internal/infra` + `internal/mcp/infra.go` implement the deny-by-default SRE command registry (`docs/SRE_ORCHESTRATION_ROADMAP.md`). The operator-owned allowlist `.openexec/infra.yaml` (never the init-generated `openexec.yaml`; unknown keys fail the load) compiles into parameter-bounded MCP tools: `ansible_run_playbook`, `salt_apply_state` (apply-class), `ssh_run_query`, `terraform_plan` (read-class; `terraform_apply` deliberately absent until the Phase-2 saved-plan pipeline). Execution is `exec.CommandContext` argv arrays — no shell, validate-and-reject (never sanitize), playbooks resolved as basenames against the configured dir. All infra tools require danger-full-access mode and are exempt from coding-toolset filtering (separate capability plane); apply-class invocations request approval in-handler (dry-runs don't) and **fail closed when no approval gate is wired** — which is currently everywhere, so apply-class is inert until the Phase-3 sign-off channel. Infra executions are never idempotency-marked (run_shell_command precedent).

### Smart-Zone Budget (context size flagging)
Agentic runners report the **peak single-call context size** (input + cache tokens of one call — never a cumulative sum across turns, which would overcount) as the `peak_context_tokens` artifact: the CLI parser extracts it from assistant-message usage, the API runner tracks it across turns. `blueprint.DefaultExecutor` flags stages whose peak exceeds `SmartZoneTokens` (0 → `DefaultSmartZoneTokens` 100k, −1 disables) with a `smart_zone_exceeded` artifact, "task is too big, split it" diagnostics, and a `[SmartZone]` log line. It is a flag, not a failure — the stage still completes.

### State & Persistence
- **SQLite**: Canonical state store at `.openexec/data/audit.db`
- **Tract**: Separate JSON-RPC microservice for story/task storage
- **Config**: `.openexec/config.json` for project settings

### Backward Compatibility: `.uaos/` → `.openexec/`
Project loading (`internal/project/`) uses dual-path reading: first tries `.openexec/config.json`, then falls back to `.uaos/project.json` (legacy format). The legacy `tasks.json` fallback for progress calculation is also preserved. This backward compatibility is enforced by `make compat-test` in CI.

---

## Engineering Mandates

### Anti-Regression Policy
Do not merge changes that modify compatibility-sensitive behavior (project loading, migration, self-healing, legacy workspace handling) without proving support was preserved. Either:
1. Add/update automated regression coverage that directly exercises the changed path, OR
2. Include a short compatibility evaluation explaining why existing-project support cannot have dropped.

Protected behaviors: existing `.openexec` projects, legacy `.uaos` projects, and migration fallbacks like `.openexec/tasks.json`.

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
- Verify API schemas in `internal/api/` or `types/` before implementing UI
- Ensure mocks match current API response format (snake_case vs camelCase)

### Commit Messages
Follow Conventional Commit prefixes: `fix:`, `feat:`, `docs:`, `release:`, etc. Keep subjects imperative and under one line.

### Known Quirks
- **SQLite: always open via `sqlitecfg.DSN(path)`** (`pkg/db/sqlitecfg`). The driver registered as `"sqlite"` is modernc.org/sqlite, which **silently ignores** mattn-style DSN params (`?_journal_mode=WAL`) — connections opened that way ran with journal_mode=delete, foreign_keys=0, busy_timeout=0 (verified empirically; fixed across all open sites). modernc needs `_pragma=journal_mode(WAL)` syntax, which it applies per pooled connection. `TestDSNAppliesPragmas` guards the runtime state.
- **JSDOM limitations**: Doesn't fully simulate layout events (onMouseEnter). Check if failing tests depend on layout properties
- **Audit DB**: Source of truth for task progress is `.openexec/data/audit.db`
- **Go version**: Requires Go 1.25+
- **CI matrix**: Tests run on both Ubuntu 22.04 and macOS

### Learning Loop
When solving complex bugs, persist lessons to `.openexec/engram/learning_log.json`
