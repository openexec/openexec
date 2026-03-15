# OpenExec Architecture Overview

This document explains how OpenExec turns an idea into executable work. OpenExec follows the **converged architecture** pattern used by modern AI coding tools: deterministic local runtime with small local LLM as gatekeeper and frontier model for hard reasoning.

## Converged Architecture: The 7-Layer Model

```
┌─────────────────────────────────────────────────────────────────────┐
│ 1. INTERACTION LAYER                                                │
│    CLI, Web UI, Slack/Ticket triggers                               │
└─────────────────────────────────────────────────────────────────────┘
                                    │
┌─────────────────────────────────────────────────────────────────────┐
│ 2. SESSION/RUNTIME LAYER                                            │
│    Session state, approvals, mode (chat/task/run), event stream     │
└─────────────────────────────────────────────────────────────────────┘
                                    │
┌─────────────────────────────────────────────────────────────────────┐
│ 3. CONTEXT ASSEMBLY LAYER                                           │
│    Files, diffs, rules, docs, tickets, repo metadata                │
│    Two-stage: deterministic gather → local LLM ranking              │
└─────────────────────────────────────────────────────────────────────┘
                                    │
┌─────────────────────────────────────────────────────────────────────┐
│ 4. TOOL LAYER (Toolsets, not flat tools)                            │
│    repo_readonly, coding_backend, coding_frontend, debug_ci, etc.   │
└─────────────────────────────────────────────────────────────────────┘
                                    │
┌─────────────────────────────────────────────────────────────────────┐
│ 5. POLICY/SANDBOX LAYER                                             │
│    Permissions, approvals, resource limits, audit trail             │
└─────────────────────────────────────────────────────────────────────┘
                                    │
┌─────────────────────────────────────────────────────────────────────┐
│ 6. ORCHESTRATION LAYER (Blueprints)                                 │
│    gather_context → implement → lint → test → review                │
└─────────────────────────────────────────────────────────────────────┘
                                    │
┌─────────────────────────────────────────────────────────────────────┐
│ 7. MODEL LAYER (Plural)                                             │
│    Local LLM: routing, classification, redaction                    │
│    Frontier LLM: implementation, reasoning, synthesis               │
└─────────────────────────────────────────────────────────────────────┘
```

## Execution Model: The Blueprint Engine

OpenExec uses a flexible **Blueprint Engine** for task execution. A blueprint defines a graph of stages that can be deterministic (code-based) or agentic (AI-based).

### Standard Task Blueprint
The default blueprint for task execution follows this flow:
`gather_context` → `implement` → `lint` → `test` → `review`

| Stage | Type | Toolset | Description |
|-------|------|---------|-------------|
| `gather_context` | Deterministic | repo_readonly | Gather relevant files and project map |
| `implement` | Agentic | coding_backend | Frontier model makes code changes |
| `lint` | Deterministic | coding_backend | Run configured linters (e.g., go vet, ruff) |
| `fix_lint` | Agentic | coding_backend | AI fixes linting errors if they occur |
| `test` | Deterministic | coding_backend | Run project test suite |
| `fix_tests` | Agentic | coding_backend | AI fixes failing tests |
| `review` | Agentic | repo_readonly | Final verification and summary |

---

## Three Execution Modes

OpenExec supports three operational modes that control permissions and workflow:

| Mode | Description | Side Effects | Approval |
|------|-------------|--------------|----------|
| **Chat** | Conversational exploration | None (read-only) | Not required |
| **Task** | Scoped action producing artifacts | Creates files/patches | Required per action |
| **Run** | Blueprint execution | Full automation | Pre-approved blueprint |

Mode transitions follow strict rules:
- Chat can escalate to Task or Run (requires user approval)
- Task can return to Chat or escalate to Run
- Run returns to Chat/Task on checkpoint, completion, or failure

## State Management

**SQLite is the canonical state store** for all task and run state.

| Store | Location | Purpose |
|-------|----------|---------|
| SQLite | `.openexec/openexec.db` | Task state, run history, audit trail |
| Artifacts | `.openexec/artifacts/` | Patches, summaries, context bundles |
| Config | `.openexec/config.json` | Project settings |

## Toolsets

Toolsets group related tools by function and risk level:

| Toolset | Tools | Risk Level | Phases |
|---------|-------|------------|--------|
| `repo_readonly` | read_file, glob, grep, git_status | Low | gather_context, review |
| `coding_backend` | read_file, write_file, git_apply_patch, run_shell_command | Medium | implement, lint, test |
| `coding_frontend` | read_file, write_file, git_apply_patch, npm_run | Medium | implement, lint, test |
| `debug_ci` | read_file, run_shell_command, git_log, ci_status | Medium | fix_ci |
| `docs_research` | read_file, glob, web_fetch | Low | gather_context |
| `release_ops` | git_tag, git_push, changelog_update | High | finalize |

## Local LLM Role: Gatekeeper, Not Boss

**Local LLM SHOULD do:**
- Intent classification (chat vs task vs run)
- Toolset selection
- Knowledge-source selection
- Context pack ranking
- Redaction/risk scoring
- Summarization of local text

**Local LLM should NOT do:**
- Core coding generation
- Long bugfix loops
- Final code synthesis
- Workflow control logic
- Permission decisions alone

## Execution Flow

1. **CLI/API**: User triggers execution via CLI (`openexec blueprint "task"`) or API
2. **Mode Selection**: Session starts in chat mode, escalates to task/run with approval
3. **Blueprint Engine**: Executes stages (gather_context → implement → lint → test → review)
4. **Tool Routing**: DCP routes tool calls to appropriate toolset based on mode
5. **Approval Gates**: Task mode pauses for approval on write operations
6. **State Persistence**: All progress persisted to SQLite audit database

## Runner Resolution

The server resolves the runner at startup from `execution.executor_model` (or override via `runner_command`/`runner_args`) and passes it to loops.

`GET /api/health` returns: `{ runner: { command, args, model } }` for verification.

## Recovery

- **Auto-heal**: When execution detects code is already implemented, task is marked complete
- **Checkpoint Resume**: Blueprint runs can resume from checkpoints after failure
- **Idempotent Tool Calls**: Write operations track idempotency keys to prevent duplicates on resume

## Mermaid Diagram

```mermaid
flowchart LR
  subgraph U[User]
    A[Task Description]
  end

  subgraph M[Mode System]
    M1[Chat Mode]
    M2[Task Mode]
    M3[Run Mode]
  end

  subgraph B[Blueprint Engine]
    B1[gather_context]
    B2[implement]
    B3[lint]
    B4[test]
    B5[review]
  end

  subgraph T[Toolsets]
    T1[repo_readonly]
    T2[coding_backend]
  end

  subgraph S[State]
    S1[(SQLite DB)]
    S2[Artifacts]
  end

  subgraph O[Observability]
    O1[/api/health]
    O2[/api/v1/runs]
  end

  %% Flow
  A --> M1
  M1 -- approval --> M2
  M2 -- inputs ready --> M3
  M3 --> B1
  B1 --> B2 --> B3 --> B4 --> B5
  B1 --> T1
  B2 --> T2
  B3 --> T2
  B4 --> T2
  B5 --> T1
  B5 --> S1
  B5 --> S2
  M3 -. status .-> O1
  M3 -. progress .-> O2
```

---

## Migration Notes

Historical documentation about legacy systems has been archived:
- **[LEGACY_5PHASE_PIPELINE.md](./archive/LEGACY_5PHASE_PIPELINE.md)**: The original 5-phase pipeline (TD/IM/RV/RF/FL)
- **[LEGACY_JSON_STORAGE.md](./archive/LEGACY_JSON_STORAGE.md)**: JSON-based state management (stories.json, tasks.json)

These systems have been superseded by the Blueprint Engine and SQLite state management described in this document.
