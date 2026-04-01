# Orchestration Internals: Pipeline, Loop, DCP, and Blueprint

This document maps the internal orchestration architecture of OpenExec — the Manager, Loop, Blueprint Engine, and DCP (Deterministic Control Plane) — including their interactions, state persistence, text heuristics, diff/patch mechanisms, and stall/retry logic.

## Component Responsibilities

```
┌──────────────────────────────────────────────────────────────┐
│ pkg/manager/Manager                                          │
│  • Lifecycle orchestration (start/stop/pause)                │
│  • Event fan-out to SSE subscribers                          │
│  • Checkpoint persistence (JSONL + SQLite)                   │
│  • Async audit logging                                       │
│  • Watchdog stall detection (5-min threshold)                │
│  • Task scheduling (dependency-aware, parallel)              │
└────────────────────────┬─────────────────────────────────────┘
                         │ spawns
┌────────────────────────▼─────────────────────────────────────┐
│ internal/loop/Loop                                           │
│  • Two execution modes: Blueprint or Standalone              │
│  • Process spawning (Claude, Gemini, etc.)                   │
│  • Stream-JSON event parsing                                 │
│  • Stall detection (30s warn, 60s stall, exponential backoff)│
│  • Thrashing detection via SignalTracker                     │
│  • Deep Trace middleware (ISO 27001 I/O capture)             │
│  • Session recording (evidence capture)                      │
└──────┬───────────────────────────────┬───────────────────────┘
       │ blueprint mode                │ standalone mode
┌──────▼──────────────────────┐  ┌─────▼──────────────────────┐
│ internal/blueprint/Engine   │  │ Direct process execution   │
│  • Stage graph execution    │  │  • Single bounded loop     │
│  • Retry routing per stage  │  │  • Parse stdout events     │
│  • Checkpoint creation      │  │  • Emit to event channel   │
│  • Artifact aggregation     │  └────────────────────────────┘
└──────┬──────────────────────┘
       │ deterministic stages use
┌──────▼──────────────────────┐
│ internal/dcp/Coordinator    │
│  • Intent parsing (BitNet)  │
│  • Tool routing & ranking   │
│  • Mode classification      │
│  • Sensitivity detection    │
│  • PII scrubbing            │
│  • Stateless query-in/out   │
└─────────────────────────────┘
```

## 1. Manager (pkg/manager/)

### Pipeline Lifecycle

The Manager maintains a map of active pipelines keyed by FWU ID. Each pipeline has a `PipelineInfo` snapshot:

| Field | Description |
|-------|-------------|
| FWUID | Pipeline identifier |
| Status | Starting → Running → Complete/Error/Stopped |
| Stage | Current blueprint stage name |
| Agent | Active agent persona |
| Iteration | Current loop iteration |
| ReviewCycles | Number of review iterations |
| CurrentPID | OS process ID of AI subprocess |
| LastActivity | Timestamp for watchdog |

**Self-healing on startup**: Resets ghost "running"/"starting" tasks to "pending" if the manager crashed mid-execution.

### Event Processing (events.go)

`consumeEvents()` reads from the Loop's event channel and:

1. Updates `PipelineInfo` state based on event type
2. Fans out to SSE subscribers (non-blocking, drops on backpressure)
3. Writes checkpoints in two tiers:
   - **JSONL**: `.openexec/checkpoints/<run_id>.jsonl` (human-readable)
   - **SQLite**: `run_checkpoints` table (canonical, for resume)
4. Async audit write via `state.WriteAsync()` with PII scrubbing
5. Records `run_steps` and `artifacts` in parallel

### Watchdog (watchdog.go)

- **Stall threshold**: 5 minutes of no activity
- **Remediation**: Kill process → stop pipeline → auto-restart after 2s
- **High iteration warning**: Flags iterations > 20

### Task Scheduling (scheduler.go)

`ExecuteTasks()` provides dependency-aware parallel execution:
- Topological sort with task-level and story-level dependencies
- Configurable worker pool parallelism
- Poll-based completion (2s intervals)

---

## 2. Loop (internal/loop/)

### Execution Modes

#### Blueprint Mode (`runBlueprint`)

```
emit EventBlueprintStart
  → blueprintEngine.StartRun(runID, input)
  → for each stage:
      emit EventStageStart
      → blueprintEngine.ExecuteStage(stage, input)
      → on success:
          emit EventStageComplete
          create checkpoint if stage.CreateCheckpoint
          transition to stage.OnSuccess
      → on failure:
          if retries < stage.MaxRetries:
              emit EventStageRetry
              transition to stage.OnFailure
          else:
              emit EventBlueprintFailed, return error
  → when CurrentStage == "complete":
      emit EventBlueprintComplete
      emit EventComplete with aggregated artifacts
```

#### Standalone Mode (`runStandalone`)

```
StartProcess() with optional Deep Trace middleware
  → goroutine: capture stderr to LogDir
  → goroutine: parser.Parse(stdout)
  → select: wait for context cancel or parse completion
  → return exit status
```

### Process Spawning (process.go)

`StartProcess()` resolves the runner (Claude, Gemini, etc.) via `runner.Resolve()` and builds command args.

**Claude-specific args** (`buildClaudeArgs`):
- `--output-format stream-json` — event stream format
- `--max-turns 50` — conversation limit
- `--disallowedTools EnterPlanMode,ExitPlanMode,AskUserQuestion` — non-interactive
- Conditional: `--dangerously-skip-permissions` for workspace/danger modes
- Optional: `--mcp-config <path>` for MCP server configuration

**Autonomy preamble** prepended to prompt:
- Disables planning phase
- Instructs use of `git_apply_patch` MCP tool for diffs
- Enables `openexec_signal` for blocking/decision signaling

**Environment variables propagated**:
- `OPENEXEC_MODE` — execution mode
- `WORKSPACE_ROOT` — working directory

### Event Stream Parsing (parser.go)

Reads line-delimited JSON (up to 1MB/line) from subprocess stdout.

**Routing by envelope type**:

| Type | Handler | Action |
|------|---------|--------|
| `system` | Skipped | — |
| `assistant` | `parseAssistant()` | Text + tool use events |
| `tool_result` | `parseToolResult()` | Tool output + artifact extraction |
| `result` | End signal | — |

**parseAssistant()** processing:
1. Emit `EventProgress` heartbeat
2. For text content:
   - Emit `EventAssistantText`
   - Check `DetectLegacyStepResult()` — `STEP_RESULT:` prefix with JSON
   - Check `DetectCompletionSignal()` — heuristic completion patterns
   - Check planning mismatch pattern
3. For tool_use content:
   - Check if `openexec_signal` or `axon_signal` (suffix match)
   - If signal: extract and emit `EventSignalReceived`
   - Else: emit `EventToolStart`

**parseToolResult()** processing:
1. Emit `EventProgress` heartbeat
2. Scan for `ARTIFACT:patch <hash> <path>` markers
3. Emit `EventToolResult` with text and artifacts

### Signal Tracker (progress.go)

Tracks agent signals for thrashing detection:

- `RecordSignal(type, iteration)` — records phase-complete, route, progress signals
- `CheckThrashing(iteration)` — returns true if `iteration - lastProgressIter >= threshold`
- `PhaseComplete()` — checks if phase-complete signal was received

---

## 3. Blueprint Engine (internal/blueprint/)

### Default Blueprint: `standard_task`

```
gather_context ──→ implement ──→ lint ──→ test ──→ review ──→ complete
(deterministic)    (agentic)    (det.)   (det.)   (agentic)
                     ↑  ↓        ↑ ↓      ↑ ↓
                     └──┘      fix_lint  fix_tests
                   (self-retry) (agentic) (agentic)
```

| Stage | Type | Toolset | MaxRetries | Checkpoint |
|-------|------|---------|------------|------------|
| gather_context | Deterministic | repo_readonly | — | Yes |
| implement | Agentic | coding_backend | 3 | No |
| lint | Deterministic | coding_backend | — | No |
| fix_lint | Agentic | coding_backend | 2 | No |
| test | Deterministic | coding_backend | — | No |
| fix_tests | Agentic | coding_backend | 2 | No |
| review | Agentic | repo_readonly | — | Yes |

### Quick Fix Blueprint: `quick_fix`

```
implement ──→ verify ──→ complete
(agentic)    (deterministic)
```

### Stage Execution (executor.go)

**Deterministic stages**:
1. Try Action Registry first (Go-native: `build_context`, `run_gates`)
2. Fall back to shell commands (`sh -c`)
3. Auto-pass if no commands configured
4. Run quality gates after success

**Agentic stages**:
1. Get `AgenticRunner` (SimpleAgenticRunner or LoopAgenticRunner)
2. Apply stage timeout (default 5min via EngineConfig)
3. `RunAgentic()` → output + artifacts
4. Run quality gates after success

**LoopAgenticRunner** bridges blueprint to loop:
- `LoopFactory` creates bounded subloop per agentic stage
- Builds rich prompt from stage metadata, task description, briefing, previous results
- Calls `loop.Run(ctx)` and `loop.GetResult()`

### Retry Logic

Two-tier retry enforcement:

1. **Per-stage**: `Stage.MaxRetries` — how many times a specific stage can retry
2. **Global**: `EngineConfig.MaxTotalRetries` (default 10) — total retries across all stages

```
Stage fails
  → if OnFailure != "" AND GetRetries(stage) < MaxRetries:
      increment stage retries
      check totalRetries <= MaxTotalRetries
      transition to OnFailure stage
  → else: fail the entire run
```

### Checkpoint System

- Stages with `CreateCheckpoint: true` trigger checkpoint creation
- Checkpoints recorded in `Run.Checkpoints` slice
- `EngineConfig.OnCheckpoint` callback invoked
- Used for resume support via `ResumeFromCheckpoint` config

### Stage Input Progression

`StageInput` carries forward through all stages:
- `PreviousStages` accumulates results
- `ContextPack` remains available throughout
- `Briefing` persists from initial input
- Each stage can access all prior outputs and artifacts

---

## 4. DCP — Deterministic Control Plane (internal/dcp/)

### Architectural Isolation

DCP is **intentionally isolated** from orchestration. Enforced by `architecture_test.go`:
- **No imports** from `internal/pipeline` or `internal/loop`
- **Stateless**: same input → same output, no internal state accumulation

**Allowed dependencies**: `internal/knowledge`, `internal/router`, `internal/tools`, `internal/logging`, `internal/mode`, `internal/toolset`, `pkg/util`

### ProcessQuery Flow

```
Query string
  → Router.ParseIntent() (BitNet)
  → Confidence check (threshold: 0.2)
  → Argument sanitization (PII scrub + infra masking)
  → Tool verification (registered?)
  → if AllowExecution: execute tool directly
    else: return IntentSuggestion (default)
  → on any failure: fallback to general_chat
```

### Route Method (Full Routing Plan)

`Route()` produces a `RoutingPlan` with:

| Field | Description |
|-------|-------------|
| Mode | Chat / Task / Run |
| Toolset | e.g., repo_readonly, coding_backend |
| RepoZones | Relevant code areas |
| KnowledgeSources | Ranked by relevance |
| Sensitivity | Low / Medium / High |
| NeedsFrontier | Whether frontier model is required |
| Confidence | Overall routing confidence (0.0–1.0) |

**Mode classification** (keyword-based):
- **Chat**: "what is", "explain", "why", "describe"
- **Task**: "add", "remove", "create", "update", "fix"
- **Run**: "implement", "refactor", "migrate", "deploy"

**Sensitivity detection** (keyword-based):
- **High**: password, secret, key, token, credential, api_key, private, ssh, auth_token
- **Medium**: email, user, customer, account, config, environment
- **Low**: everything else

**Frontier model decision**: Required if confidence < 0.5

### Tool Scoring (selector.go)

Multi-factor weighted scoring:

| Factor | Weight | Signal |
|--------|--------|--------|
| Exact name match | 0.50 | Highest |
| Description overlap | 0.30 | Word intersection |
| Category match | 0.15 | Category patterns |
| Action verb detection | 0.10 | read/write/edit/delete/search/run |

Phase-specific tool suggestions via `SuggestForPhase()`:
- **intake**: read, search, glob, grep
- **planning**: read, search, knowledge
- **execute**: write, edit, bash, run
- **review**: read, diff, test
- **finalize**: git, commit, format

### Sanitization Pipeline

All tool arguments pass through:
```
SanitizeInput()        → remove non-printable chars
  → ScrubPII()         → GDPR: remove emails, SSNs, etc.
    → MaskInfrastructure() → hide IPs, domains, ports
```

Applied recursively to all string/map/array values.

---

## 5. SQLite Schema & JSON Artifact Relationships

### State Store (pkg/db/state/)

```
sessions ──< messages ──< tool_calls
    │
    ├──< run_specs (immutable execution specs)
    │
    └──< runs ──< run_steps
              │
              ├──< run_checkpoints
              │      artifacts: JSON {hash → path}
              │      message_history: JSON []
              │      tool_call_log: JSON []
              │
              └──< artifacts (content-addressed)
                     hash (PK), type, path, size, metadata
```

**Key tables**:

| Table | Purpose | Key Fields |
|-------|---------|------------|
| `sessions` | Conversation context | id, project_path, provider, model, status, parent_session_id |
| `messages` | Message history | session_id, role, content, tokens_*, cost_usd |
| `tool_calls` | Tool invocations | message_id, tool_name, tool_input/output, idempotency_key |
| `run_specs` | Immutable exec specs | intent, context_hash, prompt_hash, model, mode |
| `runs` | Execution instances | session_id, task_id, spec_id, worktree_path, status |
| `run_steps` | Iteration records | run_id, trace_id, phase, agent, iteration, inputs_hash, outputs_hash, cache_key |
| `run_checkpoints` | Resume snapshots | run_id, phase, iteration, artifacts, message_history, tool_call_log |
| `artifacts` | Content-addressed store | hash (PK), type (patch/context_bundle/test_log/summary), path, size |

### Release Store (internal/release/)

```
goals ──< stories ──< tasks
                        │
                        └── checkpoints (run state snapshots)
```

| Table | Purpose | Key Fields |
|-------|---------|------------|
| `releases` | Release metadata | name, version, status, git_*, approval_* |
| `goals` | High-level objectives | title, description, success_criteria |
| `stories` | User stories | goal_id, role/want/benefit, acceptance_criteria (JSON), depends_on (JSON), git_* |
| `tasks` | Work units | story_id, assigned_agent, attempt_count, max_attempts, git_commits (JSON), pr_* |
| `checkpoints` | Run snapshots | run_id, stage, message_history, tool_call_log, artifacts, context_hash |

### Audit Trail (pkg/audit/)

| Table | Purpose | Key Fields |
|-------|---------|------------|
| `audit_entries` | Immutable event log | event_type, severity, session_id, run_id, actor_*, provider, model, tokens_*, cost_usd, metadata (JSON) |
| `audit_logs` | Legacy simple log | timestamp, event_type, iteration, data |

**30+ event types** covering: session lifecycle, messages, tool calls, LLM requests/responses, usage/budget, context, security, system, run lifecycle.

### JSON Artifact Locations

| Artifact | Path | Purpose |
|----------|------|---------|
| Plans | `.openexec/artifacts/plans/<hash>.json` | Execution plans from planner LLM |
| Checkpoints | `.openexec/checkpoints/<run_id>.jsonl` | Human-readable event log |
| Config | `.openexec/config.json` | Project configuration |
| Legacy config | `.uaos/project.json` | Backward-compatible project config |
| Legacy tasks | `.openexec/tasks.json` | Fallback progress calculation |
| Audit DB | `.openexec/data/audit.db` | Canonical state store |

### Idempotency & Replay

- **idempotency_key**: `sha256(tool + args + version)` on `tool_calls` table (unique index)
- **cache_key**: Stable semantic hash (`FWUID|model|iterations|stablePromptHash`)
- **prompt_hash**: SHA-256 of full composed prompt
- `CheckIdempotencyKey()` / `GetToolCallByIdempotencyKey()` enable skipping re-execution on resume

---

## 6. Text Heuristic Parsing Locations

All text heuristic parsing lives in **internal/loop/** (parser.go, schema.go):

### Legacy Step Result Detection
**File**: `internal/loop/schema.go:164-181` — `DetectLegacyStepResult()`
- Searches for `"STEP_RESULT:"` substring in assistant text
- Splits on prefix, extracts JSON payload up to newline
- Parses into `StepResult` struct and validates

### Completion Signal Detection
**File**: `internal/loop/schema.go:186-205` — `DetectCompletionSignal()`

Case-insensitive substring matching:

| Pattern | Mapped Reason |
|---------|---------------|
| "already completed" | "Agent verified implementation already exists" |
| "already done" | "Agent verified task already done" |
| "implementation is complete" | "Agent confirmed implementation complete" |
| "criteria appear to be met" | "Agent verified acceptance criteria met" |
| "task accomplished" | "Agent confirmed task accomplished" |
| "successfully implemented" | "Agent confirmed successful implementation" |

### Artifact Marker Detection
**File**: `internal/loop/parser.go:181-191`
- Line-by-line scan for `"ARTIFACT:patch "` prefix in tool results
- Format: `ARTIFACT:patch <hash> <path>`
- Extracts `patch_hash` and `patch_path` into artifacts map

### Planning Mismatch Detection
**File**: `internal/loop/parser.go:139-145`
- Checks if text contains both `"planning mismatch"` AND `"analysis reveals"` (case-insensitive)
- Emits progress signal about scope reconciliation

### Signal Name Detection
**File**: `internal/loop/parser.go` — `isOpenExecSignal()`
- Suffix match: tool name ends with `"openexec_signal"` or `"axon_signal"`

---

## 7. Diff/Patch Mechanisms

### Patch Parsing (internal/mcp/patch.go)

`ParsePatch()` — regex-based unified diff parser:

```go
type Patch struct {
    Files      []PatchFile   // Per-file changes
    RawContent string        // Original patch string
}

type PatchFile struct {
    OldName, NewName string
    Hunks            []PatchHunk
    IsBinary, IsNew, IsDeleted, IsRenamed bool
    GitHeaders       []string
}

type PatchHunk struct {
    OldStart, OldCount int
    NewStart, NewCount int
    Header             string   // Full @@ line
    Lines              []PatchLine
}

type PatchLine struct {
    Type    LineType  // LineContext(' '), LineAdd('+'), LineRemove('-')
    Content string   // Line text without prefix
}
```

Validation returns `PatchValidationResult` with stats: FilesChanged, Additions, Deletions, Hunks.

### Patch Application (internal/patch/patcher.go)

`ApplyUnifiedDiff()` — minimal full-file replacement applicator:
- **Security**: Rejects paths escaping workspace root
- **Mode**: Ignores hunk headers (@@), processes line prefixes only
- **dryRun**: Validation without writing
- **Directory creation**: `MkdirAll` for new file paths

### MCP Tool: git_apply_patch

**File**: `internal/mcp/tools.go` — `GitApplyPatchToolDef()`

Exposed as MCP tool with parameters:
- `patch_content` (string) — unified diff content
- `working_directory` (string)
- `check_only` (bool) — dry run
- `context_lines` (int)
- `three_way_merge` (bool)
- `ignore_whitespace` (bool)
- `reverse` (bool)

---

## 8. Stall Detection & Retry Logic

### Loop-Level Stall Detection (internal/loop/stall.go)

**StallDetector** state machine:

```
Normal ──(30s idle)──→ Warning ──(60s idle)──→ Stalled ──→ Recovering
  ↑                                               │            │
  └───────────────(activity recorded)──────────────┴────────────┘
```

**Configuration defaults**:

| Setting | Default | Purpose |
|---------|---------|---------|
| NoOutputTimeout | 60s | Stall threshold |
| WarningThreshold | 30s | Warning threshold |
| MaxStallAttempts | 3 | Recovery attempts before fatal |
| BaseBackoff | 1s | Initial backoff |
| MaxBackoff | 30s | Backoff cap |
| BackoffMultiplier | 2.0 | Exponential factor |
| ProviderTimeout | 5min | Hard timeout |
| IdleActivityThreshold | 5s | Activity granularity |

**Backoff formula**: `BaseBackoff × BackoffMultiplier^(stallCount-1)`, capped at MaxBackoff.

**ProviderStallWatcher**: Background goroutine checking every 1s:
1. On stall: sleep for computed backoff, then retry
2. On fatal stall (> MaxStallAttempts): cancel context

### Manager-Level Watchdog (pkg/manager/watchdog.go)

- **Threshold**: 5 minutes no activity
- **Action**: Kill process → stop pipeline → auto-restart after 2s
- Complements loop-level detection with coarser-grained process supervision

### Blueprint Retry Logic (internal/blueprint/engine.go)

Two-tier system:
1. **Per-stage**: `Stage.MaxRetries` controls individual stage retries
2. **Global**: `EngineConfig.MaxTotalRetries` (default 10) caps total retries across all stages

Retry routing is graph-based: `OnFailure` can point to self (self-retry) or a different stage (fallback). The default blueprint uses:
- `implement` → self-retry (max 3)
- `lint` → `fix_lint` → back to `lint` (max 2)
- `test` → `fix_tests` → back to `test` (max 2)

### Loop Config Retry

- `MaxRetries` and `RetryBackoff` on Loop Config for standalone mode
- Separate from blueprint retry logic

---

## Component Interaction Summary

```
User CLI/API
     │
     ▼
  Manager.Start()
     │
     ├─ Creates Loop with Config
     │   (prompt, mode, model, stall config, blueprint config)
     │
     ├─ Loop.Run()
     │   ├─ Blueprint mode:
     │   │   ├─ Engine.StartRun()
     │   │   ├─ Engine.ExecuteStage() for each stage
     │   │   │   ├─ Deterministic: Action Registry or shell commands
     │   │   │   └─ Agentic: LoopAgenticRunner → bounded subloop
     │   │   │       └─ StartProcess() → AI subprocess
     │   │   │           └─ Parser reads stream-JSON events
     │   │   │               ├─ Text → heuristic checks → events
     │   │   │               ├─ Tool use → DCP routing or signal
     │   │   │               └─ Tool result → artifact extraction
     │   │   ├─ Retry/checkpoint on stage transitions
     │   │   └─ Aggregate artifacts on completion
     │   │
     │   └─ Standalone mode:
     │       └─ Single process → parse → events
     │
     ├─ Manager.consumeEvents()
     │   ├─ Update PipelineInfo state
     │   ├─ Fan-out to SSE subscribers
     │   ├─ Write checkpoints (JSONL + SQLite)
     │   ├─ Record run_steps + artifacts (async)
     │   └─ Audit log (async, PII-scrubbed)
     │
     └─ StallDetector + Watchdog monitor liveness
```
