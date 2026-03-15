# Legacy 5-Phase Pipeline (Archived)

> **ARCHIVED**: March 2026
>
> This document describes the original 5-phase pipeline which has been superseded by the Blueprint Engine in OpenExec v1. It is preserved for historical reference.

## Overview

The original state machine progressed through five phases:

| Phase | Agent | Purpose |
|-------|-------|---------|
| **TD** | clario | Technical Design - research and strategy |
| **IM** | spark | Implementation - code changes |
| **RV** | blade | Review - quality assurance with routing back to IM if needed |
| **RF** | hon | Refinement - post-review optimization |
| **FL** | clario | Finalize - verification and state sync |

## State Management (Legacy)

State was managed via JSON files:

- `.openexec/stories.json`: Stories with `depends_on` and tasks (T-... IDs), plus `verification_script`
- `.openexec/tasks.json`: Materialized task list (imported or healed)
- `.openexec/stories/US-*.md`: Story files with status headers
- `.openexec/fwu/T-*.md`: Task context (FWU) files used during execution

## Key Artifacts (Legacy)

- `INTENT.md` (PRD): The product brief created by Wizard or by hand
- `goals[]`: Optional goals block tied to stories via `story.goal_id`

## Ordering & Barriers (Legacy)

- **Cross-story barriers**: `story.depends_on` injects ALL tasks from prerequisite stories as dependencies
- **Intra-story sequence**: Tasks are executed in listed order (each depends on the previous task)
- **Cycle guard**: Scheduler fails fast when a dependency cycle is detected

## Recovery (Legacy)

- **Auto-heal**: When Manager update fails, completion is upserted into `tasks.json` to persist state
- **Planning mismatch**: If the code is already implemented, the task is completed and persisted; true scope conflicts provide exact file paths to repair

---

## Migration to Blueprint Engine

The Blueprint Engine replaces the 5-phase pipeline with:

1. **Stage-based execution**: `gather_context -> implement -> lint -> test -> review`
2. **Typed modes**: `chat`, `task`, `run` with explicit permissions
3. **SQLite state**: Canonical state in `.openexec/openexec.db`
4. **Toolsets**: Permission-scoped tool groups instead of phase-based tool access

See [ARCHITECTURE.md](../ARCHITECTURE.md) for current architecture documentation.
