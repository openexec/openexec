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

OpenExec has evolved from a fixed 5-phase pipeline to a flexible **Blueprint Engine**. A blueprint defines a graph of stages that can be deterministic (code-based) or agentic (AI-based).

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

### Legacy 5-Phase Pipeline (Deprecated)
The original state machine phases are still supported for backward compatibility:
- **TD (Technical Design)**, **IM (Implement)**, **RV (Review)**, **RF (Refactor)**, **FL (Finalize)**.

---

## Three Execution Modes

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

## Big Picture (Legacy View)

- Wizard (Conversational): guides a short Q&A to draft a PRD (INTENT.md).
- Planner: decomposes the PRD into goals, stories, and tasks (stories.json).
- Planning Gate: validates structure (schema version, goal coverage, verification scripts).
- Import & Reconciliation: materializes stories and tasks, fixes linkages, and enforces ordering.
- Execution: Blueprint engine runs stages with toolset-based permissions.
- Built-in State (Tract): Context briefings are generated directly from the release manager state without external service dependencies.
- Auto‑Healing: repairs known mismatches (e.g., already implemented) and persists state.
- Observability: /api/health exposes the resolved runner; logs and per‑loop stderr tails aid debugging.

## Key Artifacts
- INTENT.md (PRD): the product brief created by Wizard or by hand.
- goals[]: optional goals block tied to stories via story.goal_id.
- .openexec/stories.json: stories with depends_on and tasks (T‑… IDs), plus verification_script.
- .openexec/tasks.json: materialized task list (imported or healed).
- .openexec/stories/US-*.md: story files (Status: pending/done).
- .openexec/fwu/T-*.md: task context (FWU) files used during execution.

## Ordering & Barriers
- Cross‑story barriers: story.depends_on injects ALL tasks from prerequisite stories as dependencies.
- Intra‑story sequence: tasks are executed in listed order (each depends on the previous task).
- Cycle guard: scheduler fails fast when a dependency cycle is detected.

## Runner Resolution
- The server resolves the runner once at startup from execution.executor_model (or overrides runner_command/runner_args) and passes it to loops.
- /api/health returns: `{ runner: { command, args, model } }` for quick verification.

## Recovery
- Auto‑heal: when Manager update fails, completion is upserted into tasks.json to persist state.
- Planning mismatch: if the code is already implemented, the task is completed and persisted; true scope conflicts provide exact file paths to repair.

## Mermaid Diagram (Starter)
```mermaid
flowchart LR
  %% Roles / Stages as subgraphs
  subgraph U[User]
    A[Idea / Problem]
  end

  subgraph W[Wizard]
    B[Conversational Q&A → PRD]
  end

  subgraph P[Planner]
    C[Decompose → Stories & Tasks]
  end

  subgraph G[Planning Gate]
    D[Schema & Goal Coverage]
  end

  subgraph I[Import & Reconcile]
    E[Create/Update Stories & Tasks\nBarriers + Sequencing\nSync Status]
  end

  subgraph DCT[Doctor / Preflight]
    F[Resolve Runner (model→CLI)\nPATH/Auth Checks]
  end

  subgraph X[Execution Pipeline]
    G1[TD]
    G2[IM]
    G3[RV]
    G4[RF]
    G5[FL]
  end

  subgraph H[Auto‑Heal]
    H1[Complete & Persist\n(or Print Repair Hints)]
  end

  subgraph O[Observability]
    O1[/GET /api/health → runner {command,args,model}/]
  end

  %% Artifacts
  P1((INTENT.md))
  P2[(.openexec/stories.json)]
  P3[(.openexec/tasks.json)]
  SMD[(.openexec/stories/US-*.md)]
  FWU[(.openexec/fwu/T-*.md)]

  %% Flow
  A --> B --> P1
  P1 --> C --> P2
  P2 --> D
  D -- pass --> E
  E --> SMD
  E --> P3
  E --> F
  F --> G1
  G1 --> G2 --> G3 --> G4 --> G5
  G1 --> FWU
  G5 --> H1
  H1 -- persist --> P3
  F -. exposes .-> O1
```
