# OpenExec Architecture Overview

This document explains how OpenExec turns an idea into executable work by moving through Wizard → PRD → Stories → Tasks → Execution, with built‑in validation and recovery. Use it as a primer and as a source for visualizations.

## Big Picture
- Wizard (Conversational): guides a short Q&A to draft a PRD (INTENT.md).
- Planner: decomposes the PRD into goals, stories, and tasks (stories.json).
- Planning Gate: validates structure (schema version, goal coverage, verification scripts).
- Import & Reconciliation: materializes stories and tasks, fixes linkages, and enforces ordering.
- Execution Pipeline: runs TD → IM → RV → RF → FL with the resolved CLI runner (claude/codex/gemini).
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
