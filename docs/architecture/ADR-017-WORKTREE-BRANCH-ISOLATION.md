# ADR-017: Worktree and Branch Isolation

*   **ID:** ADR-017  
*   **Status History:**
    *   2026-06-06: Created by Systems Architect. Status: Approved. Implementation: Shipped (internal/worktree) & Partial (Daemon routing).
*   **Decides on:** Task Isolation Boundaries and Workspace Safety

---

## 1. Context and Problem Statement
When OpenExec runs in the background (Daemon Mode) to execute backlog tasks while a human developer is actively coding in the same workspace, a severe conflict arises. If the background agent directly modifies files in the active directory, it will overwrite the developer's uncommitted changes, corrupt local compilers, and make parallel collaboration impossible.

## 2. Facing (Decision Drivers)
*   Absolute safety for the human developer's active workspace.
*   Prevent file conflicts and write collisions between parallel agent workers.
*   Enable clean, sandboxed testing environments locally.

## 3. Decided (The Architectural Decision)
We mandate **Staged Git Worktree and Branch Isolation** using Go's `exec` layer over native Git commands:
1.  **No Direct Workspace Writes:** The background execution daemon is physically blocked from ever modifying files directly inside the developer's active working directory.
2.  **Dynamic Worktree Sandboxing (`internal/worktree/`):** When a Task starts execution, the Go backend runs:
    ```bash
    git worktree add -b task/T-123 .openexec/worktrees/T-123
    ```
    This creates an ephemeral, isolated physical directory copying the Git index.
3.  **Isolated Execution:** The AI agent, compilers, linters, and test suites are executed *strictly* inside this isolated `.openexec/worktrees/T-123` folder. The developer's active directory remains completely untouched.
4.  **Clean Promotion:** Once the tests pass and the review agent signs off, the Go core merges the `task/T-123` branch back to `main` (or opens a PR) and prunes the worktree directory safely.

## 4. Rejected/Neglected Alternatives
*   *Simple Git Stashing:* Rejected. Running `git stash` before execution and `git stash pop` afterward frequently fails due to merge conflicts, causing data loss for the developer.

## 5. Consequences & Tradeoffs
*   *Positives:* Complete isolation; safe, AFK background execution while the developer is actively typing; no local file lock collisions.
*   *Negatives:* Requires local disk space to hold the isolated worktree directory structures.

## 6. Dependencies & References
*   Wired in `internal/worktree/` and the Task runner loop.
