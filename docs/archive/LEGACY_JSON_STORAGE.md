# Legacy JSON-Based Storage (Archived)

> **ARCHIVED**: March 2026
>
> This document describes the legacy JSON-based state management system which has been superseded by SQLite in OpenExec v1. It is preserved for historical reference and migration purposes.

## Overview

The original OpenExec system used JSON files for state persistence:

| File | Purpose |
|------|---------|
| `.openexec/stories.json` | Stories with `depends_on` and tasks (T-... IDs), plus `verification_script` |
| `.openexec/tasks.json` | Materialized task list (imported or healed) |
| `.openexec/stories/*.md` | Story files with status headers |
| `.openexec/fwu/T-*.md` | Task context (FWU) files used during execution |

## stories.json Structure

```json
{
  "stories": [
    {
      "id": "US-001",
      "title": "User Authentication",
      "status": "pending",
      "goal_id": "G-001",
      "depends_on": ["US-000"],
      "tasks": [
        {
          "id": "T-001",
          "title": "Implement login endpoint",
          "verification_script": "go test ./auth/..."
        }
      ]
    }
  ],
  "goals": [
    {
      "id": "G-001",
      "description": "Enable user login"
    }
  ]
}
```

## tasks.json Structure

```json
{
  "tasks": [
    {
      "id": "T-001",
      "story_id": "US-001",
      "title": "Implement login endpoint",
      "status": "pending",
      "phase": "TD"
    }
  ]
}
```

## Key Artifacts

- **INTENT.md (PRD)**: The product brief created by Wizard or by hand
- **goals[]**: Optional goals block tied to stories via `story.goal_id`

## Ordering and Barriers

- **Cross-story barriers**: `story.depends_on` injects ALL tasks from prerequisite stories as dependencies
- **Intra-story sequence**: Tasks are executed in listed order (each depends on the previous task)
- **Cycle guard**: Scheduler fails fast when a dependency cycle is detected

## Recovery Mechanisms

- **Auto-heal**: When Manager update fails, completion is upserted into `tasks.json` to persist state
- **Planning mismatch**: If the code is already implemented, the task is completed and persisted; true scope conflicts provide exact file paths to repair

## Issues with JSON Storage

1. **Dual-write problem**: Both in-memory state and JSON files needed synchronization
2. **Race conditions**: Multiple processes could corrupt state
3. **No transactional integrity**: Partial writes could leave inconsistent state
4. **Limited queryability**: Complex queries required loading entire files

---

## Migration to SQLite

SQLite is now the canonical state store. Benefits:

1. **Single source of truth**: `.openexec/openexec.db` (or `.openexec/data/audit.db`)
2. **ACID transactions**: Atomic, consistent, isolated, durable operations
3. **Rich queries**: SQL support for complex filtering and aggregation
4. **Event sourcing**: Full execution history with `openexec replay <run-id>`

### Migration Notes

- JSON files may still exist for import compatibility
- Use `openexec migrate` (if available) to convert legacy state
- After migration, JSON files are not the source of truth

See [ARCHITECTURE.md](../ARCHITECTURE.md) for current architecture documentation.
