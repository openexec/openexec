# Orchestration Architecture Map

> Generated: 2026-03-31 | Task: T-US-000-001 (Chassis: Map Orchestration Architecture)

This document maps the three orchestration planes (Manager/Pipeline, Loop, DCP), their
interactions, state management, text heuristics, diff/patch mechanisms, and recovery logic.

---

## 1. Orchestration Planes Overview

```
                    ┌──────────────────────────────────────────────┐
                    │              Manager (pkg/manager/)           │
                    │  Multi-pipeline orchestrator + watchdog       │
                    │  Topological task scheduling                  │
                    │  SSE fan-out, async state writes              │
                    └──────────┬───────────────────────────────────┘
                               │ Start/Stop/Pause per FWUID
                               ▼
                    ┌──────────────────────────────────────────────┐
                    │              Loop (internal/loop/)            │
                    │  Blueprint or standalone execution mode       │
                    │  Process spawning, event parsing              │
                    │  Stall detection, quality gates               │
                    └──────────┬───────────────────────────────────┘
                               │ ExecuteStage / StartRun
                               ▼
                    ┌──────────────────────────────────────────────┐
                    │         Blueprint (internal/blueprint/)       │
                    │  Stage graph: gather_context → implement →    │
                    │  lint → test → review                        │
                    │  Checkpoints, retries, agentic subloops      │
                    └──────────────────────────────────────────────┘

          ┌──────────────────────────────────────────────────────────┐
          │                DCP (internal/dcp/)                       │
          │  Stateless tool-routing layer (suggest-only)             │
          │  BitNet local LLM for intent → tool mapping             │
          │  Mode/toolset/sensitivity classification                 │
          │  PII sanitization pipeline                               │
          │  DOES NOT import loop or pipeline (enforced by tests)    │
          └──────────────────────────────────────────────────────────┘
```

### 1.1 Manager (pkg/manager/)

**Responsibility:** Multi-pipeline lifecycle orchestrator.

| Concern | Implementation |
|---------|---------------|
| Pipeline map | `pipelines map[string]*entry` guarded by `sync.RWMutex` |
| Lifecycle | `Start` / `Stop` / `Pause` / `Status` / `List` per FWUID |
| States | `starting → running → paused / complete / error / stopped` |
| Task scheduling | Topological sort with dependency-aware parallel worker pool (`scheduler.go`) |
| Event consumption | Per-pipeline goroutine consuming `loop.Event` channel, fan-out to SSE subscribers via 64-slot buffered channels; slow subscribers drop events |
| Watchdog | 30s polling; stall if no activity for 5 min → kill PID, stop pipeline, auto-restart with 2s delay (`watchdog.go`) |
| Ghost cleanup | On startup, resets `running`/`starting` tasks to `pending` for crash recovery |
| Async writes | `state.WriteAsync()` for run_steps, artifacts, checkpoints — non-blocking |

**Key files:** `manager.go` (530 LOC), `events.go` (343), `scheduler.go` (217), `watchdog.go` (112), `planner.go` (310), `checkpoints.go` (88)

### 1.2 Loop (internal/loop/)

**Responsibility:** Core iteration engine — spawns AI subprocess, parses events, orchestrates blueprint stages.

**Two execution modes:**

| Mode | Entry | Description |
|------|-------|-------------|
| Blueprint | `runBlueprint()` | Stage-driven: creates `blueprint.Run`, iterates stages via `engine.ExecuteStage()`, handles retries/checkpoints |
| Standalone | `runStandalone()` | Single Claude Code process, pipes stdout to `Parser`, bounded by context timeout |

**Process spawning** (`process.go`):
- Resolves executor (Claude/Gemini) via `runner.Resolve()`
- Flags: `--output-format stream-json`, `--verbose`, `--max-turns 50`
- Disables interactive tools: `EnterPlanMode`, `ExitPlanMode`, `AskUserQuestion`
- Injects autonomy preamble into prompt
- Adds `--mcp-config` for openexec-signal server

**Flow control:** `Stop()`, `Pause()`, `Resume()` via atomics. `GetHealth()` returns `LoopHealth` snapshot.

**Key files:** `loop.go` (358), `parser.go` (237), `stall.go` (364), `process.go` (249), `config.go` (242), `middleware.go` (349), `capture.go` (155), `event.go` (133), `gates_integration.go` (111), `progress.go` (64), `mcpconfig.go` (59)

### 1.3 Blueprint Engine (internal/blueprint/)

**Responsibility:** Stage-based execution with checkpoints and retry logic.

**Default blueprint** (`standard_task`):
```
gather_context (deterministic, checkpoint)
    → implement (agentic, max 3 retries, OnFailure=implement)
    → lint (deterministic, OnFailure=fix_lint)
    → fix_lint (agentic, max 2 retries)
    → test (deterministic, OnFailure=fix_tests)
    → fix_tests (agentic, max 2 retries)
    → review (agentic, checkpoint)
```

**Stage types:**
- **Deterministic**: Go-native `ActionRegistry` (build_context, run_gates, apply_patch) or shell command fallback
- **Agentic**: `AgenticRunner` → `LoopAgenticRunner` creates bounded subloop (max 10 iterations)

**Key types:** `Blueprint`, `Stage`, `Run`, `StageResult`, `StageInput`, `Engine`, `DefaultExecutor`, `Registry`

**Key files:** `engine.go` (411), `stage.go` (215), `executor.go` (368), `registry.go` (122)

### 1.4 DCP — Deterministic Control Plane (internal/dcp/)

**Responsibility:** Stateless tool-routing layer. Suggest-only by default — MCP handles execution.

**Architecture invariants** (enforced by `architecture_test.go`):
- MUST NOT import `internal/pipeline` or `internal/loop`
- `ProcessQuery` is pure function (no internal state accumulation)

**Routing pipeline:**
```
Query → PII sanitization → BitNet router → IntentSuggestion
                                              ↓
                                    Route() builds RoutingPlan:
                                      mode, toolset, repoZones,
                                      knowledgeSources, sensitivity,
                                      needsFrontier, confidence
```

**Classification heuristics** (keyword-based):

| Classifier | Method | Output |
|-----------|--------|--------|
| Mode | `classifyMode()` | chat / task / run |
| Toolset | `selectToolset()` via `toolset.Selector` | repo_readonly, coding_backend, etc. |
| Repo zones | `identifyRepoZones()` | internal/api, pkg/db, etc. |
| Knowledge sources | `rankKnowledgeSources()` | code_symbols, git_history, etc. |
| Sensitivity | `detectSensitivity()` | low / medium / high |
| Frontier need | `needsFrontierModel()` | bool |

**Fallback triggers:** router error → low confidence (<0.2) → missing tool → all fall to `general_chat`

**Tool scoring** (`selector.go`): Multi-factor relevance scoring — exact name match (+0.5), word overlap with description (+0.0-0.3), category match (+0.15), action verb detection (+0.1).

**Key files:** `coordinator.go` (662), `selector.go` (235)

---

## 2. SQLite Schema & JSON Artifact Relationships

### 2.1 Database Location

Canonical store: `.openexec/openexec.db` (WAL mode, foreign keys enabled)

Legacy: `.openexec/data/audit.db` (referenced in older docs)

### 2.2 Schema Tables

| Table | Purpose | Key Columns |
|-------|---------|-------------|
| `runs` | Execution instances | id, session_id, task_id, spec_id, project_path, mode, status, worktree_path, metadata(JSON) |
| `run_steps` | Individual iterations | run_id(FK), trace_id, phase, agent, iteration, inputs_hash, outputs_hash, cache_key |
| `artifacts` | Content-addressed storage | hash(PK), type(patch/context_bundle/test_log/summary), path, size, metadata(JSON) |
| `run_checkpoints` | Resume support | run_id(FK), phase, iteration, timestamp, artifacts(JSON map), message_history(JSON), tool_call_log(JSON) |
| `goals` | Release goals | title, description, success_criteria, verification_method |
| `stories` | User stories | epic_id, goal_id, acceptance_criteria(JSON), depends_on(JSON), story_type, priority, git_branch, approval_* fields |
| `tasks` | Atomic work units | story_id(FK), depends_on(JSON), attempt_count, max_attempts, git_commits(JSON), approval_* fields, needs_review |
| `sessions` | Conversation sessions | project_path, provider, model, parent_session_id |
| `messages` | Conversation messages | session_id(FK), role, content, tokens_*, cost_usd |
| `tool_calls` | Tool invocations | message_id(FK), tool_name, tool_input, tool_output, idempotency_key, approval_status |
| `run_specs` | Run specifications | session_id, intent, context_hash, prompt_hash, model, mode |
| `audit_entries` | Audit log | event_type, severity, actor_id, metadata(JSON), tokens, cost |
| `symbols` | Code knowledge | name, kind, file_path, start_line, end_line, purpose |

### 2.3 Artifact Flow

```
MCP tool writes patch → .openexec/artifacts/patches/<hash>.diff
                              ↓
                    artifacts table: hash(PK), type='patch', path
                              ↓
              run_checkpoints.artifacts: {"hash": "path", ...}
                              ↓
              loop Event.Artifacts: map[string]string (same format)
```

Plans: `.openexec/artifacts/plans/<hash>.json`

### 2.4 Checkpoint Dual-Write

Checkpoints written to both:
1. **JSONL**: `.openexec/checkpoints/<run_id>.jsonl` (human-readable, append-only)
2. **SQLite**: `run_checkpoints` table (queryable, with message_history and tool_call_log)

### 2.5 JSON → SQLite Migration

Release manager (`internal/release/manager.go`) implements one-time bootstrap:
- On first `Load()`, checks if SQLite is empty
- If empty, imports `stories.json`, `tasks.json`, `release.json`
- After migration, SQLite is canonical — JSON files never touched again

---

## 3. Text Heuristic Parsing Locations

All text heuristic parsing that extracts structured data from unstructured AI output:

### 3.1 Loop Parser (`internal/loop/parser.go`)

| Location | Pattern | Purpose |
|----------|---------|---------|
| Line ~115 `parseAssistant` | `schema.DetectLegacyStepResult(text)` — searches for `STEP_RESULT: {JSON}` | Extract typed completion result (status, reason, confidence, artifacts) |
| Line ~131 `parseAssistant` | `schema.DetectCompletionSignal(text)` — matches phrases: "already completed", "already done", "successfully implemented", etc. | Legacy completion detection (deprecated) |
| Line ~139 `parseAssistant` | `strings.Contains(lower, "planning mismatch") && strings.Contains(lower, "analysis reveals")` | Detect scope reconciliation |
| Line ~147 `parseAssistant` | Tool name suffix matching: `openexec_signal`, `axon_signal` | Route signal tool calls to `emitSignal()` |
| Lines 181-190 `parseToolResult` | `ARTIFACT:patch <hash> <path>` marker in tool result content | Extract patch hash and path into `event.Artifacts` |

### 3.2 DCP Coordinator (`internal/dcp/coordinator.go`)

| Location | Pattern | Purpose |
|----------|---------|---------|
| `classifyMode()` ~L419 | Keyword sets: "implement"/"refactor" → run; "what is"/"explain" → chat; "add"/"fix" → task | Mode classification |
| `identifyRepoZones()` ~L490 | Keyword map: "api" → "internal/api"; path prefix extraction for "internal/", "pkg/", "cmd/" | Repo zone identification |
| `detectSensitivity()` ~L590 | Keyword sets: "password"/"secret"/"token" → high; "email"/"config" → medium | Sensitivity classification |
| `rankKnowledgeSources()` ~L546 | Keyword priority groups: "readme" → local_docs; "function"/"class" → code_symbols; etc. | Knowledge source ranking |
| `needsFrontierModel()` ~L620 | Mode + toolset + confidence rules | Frontier model decision |
| `selectToolset()` ~L470 | Via `toolset.Selector.SelectForTask()` with keyword matching | Toolset selection |

### 3.3 DCP Selector (`internal/dcp/selector.go`)

| Location | Pattern | Purpose |
|----------|---------|---------|
| `scoreToolForIntent()` ~L100 | Exact name match (+0.5), word overlap (+0-0.3), category match (+0.15), verb detection (+0.1) | Tool relevance scoring |

### 3.4 BitNet Router (`internal/router/bitnet.go`)

| Location | Pattern | Purpose |
|----------|---------|---------|
| ~L63-104 | JSON parse of router output; "could not determine intent" phrase detection; confidence < 0.2 threshold | Intent extraction from local LLM |

---

## 4. Diff/Patch Mechanisms

### 4.1 Unified Diff Parser (`internal/mcp/patch.go`)

Full unified diff parser (~550 LOC):

```go
Patch { Files []PatchFile { Hunks []PatchHunk { Lines []PatchLine } } }
```

**Parsing capabilities:**
- Git diff headers: `diff --git a/file b/file`
- File headers: `--- a/file` / `+++ b/file`
- Hunk headers: `@@ -start,count +start,count @@`
- Line types: context (` `), add (`+`), remove (`-`), special (`\`)
- Binary files, new file mode, deleted file mode, renames, `/dev/null`

**Validation** (`ValidatePatch()` → `PatchValidationResult`):
- Old/new line count matches header declaration
- Positive line numbers
- Both `---` and `+++` headers present
- Warnings for context-only hunks
- Stats: FilesChanged, Additions, Deletions, Hunks

### 4.2 Minimal Patcher (`internal/patch/patcher.go`)

```go
ApplyUnifiedDiff(root string, r io.Reader, dryRun bool) error
```

**Strategy:** Full-file replacement (not precise hunk application).
- Keeps lines prefixed with `+` or ` ` (space context)
- Drops lines prefixed with `-`
- Skips `@@` hunk markers
- Path validation: rejects escapes outside workspace root

### 4.3 MCP `git_apply_patch` Tool

Exposed via MCP server for AI agents to apply patches:
- Agent writes unified diff
- MCP validates and applies via patcher
- Artifact stored at `.openexec/artifacts/patches/<hash>.diff`
- Hash recorded in `artifacts` table and checkpoint

### 4.4 Artifact Storage

```
.openexec/artifacts/
├── patches/    <hash>.diff    → artifacts table (type='patch')
└── plans/      <hash>.json    → artifacts table (type='context_bundle')
```

Content-addressed: SHA256 hash is primary key, enables deduplication.

---

## 5. Stall Detection & Retry Logic

### 5.1 Loop-Level Stall Detector (`internal/loop/stall.go`)

**State machine:**
```
Normal ──[30s idle]──► Warning ──[60s idle]──► Stalled ──[attempt<max]──► Recovering ──[activity]──► Normal
                                                  │
                                                  └─[attempt>=max]──► FatalStall
```

**Configuration** (`StallConfig`):
| Parameter | Default | Purpose |
|-----------|---------|---------|
| `NoOutputTimeout` | 60s | Hard stall threshold |
| `WarningThreshold` | 30s | Warning before stall |
| `MaxStallAttempts` | 3 | Recovery attempts before fatal |
| `BaseBackoff` | 1s | Initial backoff |
| `MaxBackoff` | 30s | Backoff cap |
| `BackoffMultiplier` | 2.0 | Exponential factor |
| `ProviderTimeout` | 5m | Hard provider call timeout |
| `IdleActivityThreshold` | 5s | Activity freshness requirement |

**Backoff formula:** `backoff = BaseBackoff * (BackoffMultiplier ^ (stallCount - 1))`, capped at `MaxBackoff`

**ProviderStallWatcher:** Background goroutine on 1s ticks → emits `EventProgress` with STALL_WARNING / STALL_DETECTED / STALL_RECOVERED / FATAL_STALL.

### 5.2 Manager-Level Watchdog (`pkg/manager/watchdog.go`)

- Polls every 30s
- Stall if `time.Since(LastActivity) > 5 min`
- Recovery: kill PID → stop pipeline → auto-restart with 2s delay
- Ghost cleanup on startup: reset `running`/`starting` tasks to `pending`

### 5.3 Blueprint Stage Retries (`internal/blueprint/`)

| Stage | MaxRetries | OnFailure |
|-------|-----------|-----------|
| gather_context | 0 | (none) |
| implement | 3 | implement (self-retry) |
| lint | 0 | fix_lint |
| fix_lint | 2 | fix_lint (self-retry) |
| test | 0 | fix_tests |
| fix_tests | 2 | fix_tests (self-retry) |
| review | 0 | (none) |

**Total retry limit:** `EngineConfig.MaxTotalRetries` (default 10) prevents infinite retry loops.

**Retry flow:**
1. Stage executes → fails
2. Check `stage.OnFailure` and retry count
3. If retries < MaxRetries: increment counter, route to OnFailure stage
4. If retries >= MaxRetries: `run.Fail()`, return error

### 5.4 Thrashing Detection (`internal/loop/progress.go`)

**SignalTracker** accumulates agent signals:
- Tracks `phase-complete`, `route`, `progress` signals
- `CheckThrashing(iteration)` returns true if idle > `thrashThreshold` iterations without progress signals
- Loop emits `EventThrashingDetected` and can terminate

### 5.5 Quality Gate Retries (`internal/loop/gates_integration.go`)

- `MaxGateRetries` (default 3) from loop config
- On gate failure: `buildGateFixPrompt()` creates corrective prompt with attempt counter
- Instructs executor to fix code, not just re-signal completion

---

## 6. Interaction Map

```
┌─────────────┐     starts/stops     ┌──────────────┐
│   Manager    │────────────────────►│     Loop      │
│              │◄────────────────────│              │
│  pipelines   │   Event channel     │  blueprint    │
│  watchdog    │                     │  or standalone│
│  scheduler   │                     │              │
└──────┬──────┘                     └──────┬──────┘
       │                                    │
       │ async writes                       │ ExecuteStage
       ▼                                    ▼
┌──────────────┐                   ┌──────────────┐
│   SQLite     │                   │  Blueprint   │
│  (state DB)  │                   │   Engine     │
│              │                   │              │
│ runs         │◄──────────────────│ Run state    │
│ run_steps    │   checkpoint      │ StageResult  │
│ artifacts    │   writes          │ Checkpoints  │
│ checkpoints  │                   └──────┬──────┘
└──────────────┘                          │
                                          │ deterministic: ActionRegistry
                                          │ agentic: LoopAgenticRunner
                                          ▼
                                 ┌──────────────────┐
                                 │  AI Subprocess   │
                                 │  (Claude/Gemini) │
                                 │                  │
                                 │  MCP server ◄────┼── DCP routing
                                 │  openexec-signal │   (HTTP, suggest-only)
                                 └──────────────────┘
```

**Key interaction contracts:**
1. Manager → Loop: `Start()` creates Loop with Config, receives `<-chan Event`
2. Loop → Blueprint: `engine.StartRun()`, `engine.ExecuteStage()` per stage
3. Blueprint → AI: Agentic stages spawn bounded subloops via `LoopAgenticRunner`
4. AI → Loop: Stream-JSON stdout parsed by `Parser` into typed `Event`s
5. Loop → Manager: Events consumed, written async to SQLite
6. DCP ↔ Loop: **No direct coupling** — DCP is HTTP-only, accessed by AI via MCP tools

---

## 7. Key Architectural Properties

| Property | Implementation |
|----------|---------------|
| Single source of truth | SQLite (JSON bootstrap is one-time migration) |
| Content addressing | SHA256 hash as artifact PK |
| Dual checkpoint write | JSONL (human-readable) + SQLite (queryable) |
| Stateless routing | DCP has no state between queries (enforced by tests) |
| Non-blocking fan-out | SSE subscribers with bounded buffers, drop tracking |
| Crash recovery | Watchdog ghost cleanup + checkpoint-based resume |
| Audit trail | Deep-Trace middleware wraps all subprocess I/O with SHA256 hashes |
| Deterministic replay | `CacheKey` = SHA256(fwuID|model|maxiters|stableHash) |
