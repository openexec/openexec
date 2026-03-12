# Visualization Guide

This guide gives you a ready‑to‑draw blueprint for diagrams. Use these shapes and edges to explain OpenExec’s flow without hand‑holding.

## Swimlanes
- Roles: User, Wizard, Planner, Orchestrator (Server/Manager), Runner (claude/codex/gemini), CI/CD
- Artifacts: INTENT.md, goals[], stories.json, tasks.json, .openexec/stories/*.md, .openexec/fwu/*.md
- Controls: Doctor/Preflight, Planning Gate, Import/Reconcile, Auto‑Heal, Health API

## Node Sequence
1) Idea (User)
2) Wizard (Q&A) → INTENT.md (PRD)
3) Planner → stories.json (stories, depends_on, tasks[] with verification_script)
4) Planning Gate (schema + goal coverage) → pass/fail with hints
5) Story Import & Reconciliation
   - Create/Update stories & tasks
   - Enforce story barriers and intra‑story sequence
   - Sync "done" from story markdown
6) Doctor/Preflight
   - Resolve runner (model→CLI)
   - PATH preflight, auth hints
   - /api/health exposes runner {command,args,model}
7) Execution Pipeline (TD → IM → RV → RF → FL)
   - Runner receives stdin prompt (claude/codex/gemini)
   - Gates via openexec.yaml & verification_script
8) Auto‑Heal
   - If already implemented → complete & persist (Manager or tasks.json fallback)
   - Scope conflict → print exact file paths to repair

## Edges (Data)
- Wizard → INTENT.md
- Planner → stories.json (+ goals[] linkage via story.goal_id)
- Import → .openexec/stories/*.md & Manager DB
- Orchestrator → .openexec/fwu/T-*.md during execution
- Health → GET /api/health returns runner metadata

## Mermaid (starter)
```mermaid
flowchart LR
  A[Idea/Problem] --> B[Wizard Q&A]
(INTENT.md)
  B --> C[Planner]
(stories.json)
  C --> D[Planning Gate]
(schema + goals)
  D -- pass --> E[Import & Reconcile]
(stories/tasks)
  E --> F[Doctor/Preflight]
(runner resolve)
  F --> G[Execution TD→IM→RV→RF→FL]
(runner CLI)
  G --> H[Auto‑Heal]
(persist repair)
```

## Legend
- Blue: Roles, Orange: Controls, Green: Artifacts, Purple: Execution phases
- Thick arrows: control flow; thin arrows: data artifacts

## Tips
- Put barriers on the diagram: `story.depends_on → ALL tasks of prerequisite stories`.
- Show intra‑story sequencing as a linked list.
- Annotate runner mapping (model→CLI) next to Preflight; add /api/health box.
