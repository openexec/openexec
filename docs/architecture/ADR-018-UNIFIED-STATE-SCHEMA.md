# ADR-018: Unified State Store & Schema Boundaries

*   **ID:** ADR-018  
*   **Status History:**
    *   2026-06-06: Created by Systems Architect. Status: Approved. Implementation: Shipped (state.db / openexec.db).
*   **Decides on:** The Canonical State Database and Schema Boundaries

---

## 1. Context and Problem Statement
Early versions of OpenExec relied on a mixture of JSON files (`stories.json`, `tasks.json`) and in-memory structures to track task progress and session states. This created "archeology" where states drifted, data was lost on crashes, and concurrent background-daemon writes would easily corrupt the files.

## 2. Facing (Decision Drivers)
*   Establish a single, canonical, transactional source of truth for all runtime states.
*   Ensure complete recoverability and resumability of runs after system crashes.
*   Prevent shadow state systems (no side-car JSON file writes).

## 3. Decided (The Architectural Decision)
We establish the **Unified SQLite Schema** as the *only* canonical runtime state database:
1.  **Database Boundary:** The SQLite database (`.openexec/openexec.db`) is the absolute source of truth. All modules (Planner, Scheduler, Pipelines, and API) must interact *exclusively* via this canonical database.
2.  **Schema Tables:** The database enforces strict schemas for all active entities:
    *   `runs` / `run_checkpoints` — For execution state and message history replay.
    *   `stories` — For user outcomes.
    *   `tasks` — For executable code units.
    *   `audit_entries` — For cryptographically hashed execution trails.
3.  **No Side-car JSON:** JSON files are completely deprecated for state tracking. The system performs a one-time bootstrap migration from old JSON files to SQLite on first boot, and never writes to JSON state files again.

## 4. Rejected/Neglected Alternatives
*   *Postgres/MySQL:* Rejected. Introducing server-based databases violates the zero-dependency, single-binary distribution mandate. SQLite provides transactional, serverless SQL natively.

## 5. Consequences & Tradeoffs
*   *Positives:* Transactional ACID safety; resilient crash-recovery; SQL querying capabilities over all historical agent decisions.
*   *Negatives:* Requires executing migrations and schema updates programmatically in Go on database boot.

## 6. Dependencies & References
*   Decided inside `docs/architecture/THE-80-PERCENT-TRAP.md`.
*   Implemented in `pkg/db/state/` and `internal/release/`.
