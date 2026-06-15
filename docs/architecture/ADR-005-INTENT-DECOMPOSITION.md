# ADR-005: Intent Decomposition & The Execution Loop

*   **ID:** ADR-005
*   **Status:** Approved
*   **Author:** Systems Architect
*   **Date:** 2026-06-06
*   **Decides on:** Task Breakdown, Branching, and the Coding Loop

---

## 1. Context and Problem Statement
When a user asks to "Build a complete billing dashboard," handing the entire intent to an LLM results in scattered, incomplete code. The LLM attempts to write frontend UI, backend routes, and database migrations simultaneously, overwhelming its output token limit and introducing massive syntax errors.

## 2. Decision Drivers
*   AI agents write best when focused on highly isolated, single-file or single-component tasks.
*   Work must be reviewable by humans before merging.
*   Failure on one component must not block or corrupt parallel work on another.

## 3. Architectural Decision (The Chosen Path)
We mandate a strict **Intent Decomposition Pipeline** utilizing Git Branching and scoped state machines.

### The Decomposition Hierarchy
1.  **Intent:** The human's raw input (e.g., "Build a billing dashboard").
2.  **Blueprint:** The orchestrator defines constraints, architecture, and dependencies.
3.  **Stories:** User-visible outcomes (e.g., "User can view invoice history").
4.  **Tasks:** Executable code units (e.g., "Implement `InvoiceTable.tsx`", "Add `GET /invoices` route").

### The Coding and Review Loop
When a Task is popped from the `.openexec.db` backlog:
1.  **Branch Isolation:** The execution daemon runs `git checkout -b task/T-123` (optionally inside an isolated `git worktree`). The AI never operates directly on `main`.
2.  **Execution Loop:** The AI iterates through its assigned toolset, generating code.
3.  **Local Verification:** The daemon runs `npm run test` or `go test` locally against the branch.
4.  **Review Stage:** If the tests pass, an isolated **Code Review Agent** (see ADR-009) inspects the diff.
5.  **Merge / Abort:** If the reviewer signs off, the human operator approves the PR/Branch merge. If it fails, it decomposes or aborts.

### Why this option wins:
By forcing the orchestrator to break large intents into microscopic, branch-isolated Tasks, we guarantee that the LLM is only ever solving a bounded problem with a clear success criteria. If the agent hallucinates, it only corrupts a disposable feature branch, leaving the main repository entirely safe.
