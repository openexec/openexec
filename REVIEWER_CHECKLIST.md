# OpenExec Reviewer Checklist

This checklist guides reviewers to ensure the OpenExec ecosystem remains robust, efficient, and ISO‑compliant.

Applies to:
- `openexec` (CLI/Execution)
- `openexec-planner` (Planning/Scheduling)

---

## 🛡️ OpenExec Reviewer Checklist

### 1. Concurrency & Deadlock Prevention (CLI/Execution)
- Recursive Mutexes: Ensure no function called while holding a `sync.Mutex` attempts to acquire that same mutex (directly or via a callback like `checkReady`).
- Channel Lifecycle: Verify that task channels (e.g., `readyTasks`) are closed exactly once, and only when all pending and running tasks are fully accounted for (`doneCount == totalCount`).
- Race Conditions: Check that shared counters (`doneCount`, `totalCount`) and maps (`nodes`) are only accessed/mutated within a `mu.Lock()` block.

### 2. Adaptive DAG Integrity (CLI/Execution)
- Dynamic Task Discovery: When `tasks.json` is re‑scanned, verify that the system correctly identifies new tasks and injects them into the `nodes` map without duplicating existing ones.
- Dependency Persistence: Ensure that tasks correctly preserve their `depends_on` relationships even after multiple re‑scans and across different `openexec` run sessions.
- Status Mapping: Verify that the engine correctly maps both `"completed"` and `"done"` strings from the JSON to the internal `StatusCompleted` state to avoid redundant execution.

### 3. Senior Implementation Heuristics (Orchestration)
- Technical Strategies: Confirm that every implementation task in the generated `stories.json` contains a `technical_strategy` field. It should not just say "Implement X," but should provide architectural hints (e.g., "Use Pydantic for validation," "Handle None values").
- Task Granularity: Check that implementation tasks are small and atomic, followed immediately by a corresponding `Test:` task.
- Skeleton Seeding: Verify that for UI‑centric projects, the first story includes a mandatory task to auto‑import a "Starter Skeleton" or "Hello World" workflow.

### 4. ISO Compliance & Traceability (Orchestration/Audit)
- Goal Linking: Ensure every story is explicitly linked to a `goal_id` from the `INTENT.md`.
- Audit Logging: Verify that all "Verification Evidence" (the output of `verification_script`) is correctly captured and stored in the `audit.db`.
- Validation Checkpoints: Confirm that each story concludes with a `Validation` task that ensures the integrated feature set satisfies the story's acceptance criteria.

