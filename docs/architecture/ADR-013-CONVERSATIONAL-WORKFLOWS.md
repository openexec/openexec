# ADR-013: Conversational Sessions & Web Console State

*   **ID:** ADR-013  
*   **Status History:**
    *   2026-06-06: Created by Systems Architect. Status: Approved. Implementation: Shipped (internal/conversation & openexec-web).
*   **Decides on:** Web Console State, Forking, and Rollbacks

---

## 1. Context and Problem Statement
When developers interact with AI agents via chat, they frequently want to explore multiple speculative paths (e.g. "What if we refactored it using Library A vs Library B?"). If the conversation is strictly linear, exploring a dead-end path corrupts the active directory and force-commits broken code, making rollbacks difficult and frustrating.

## 2. Decision Drivers
*   Support non-linear exploration of requirements and code designs.
*   Provide complete, visual file diffs and state rollbacks in the Web Console.
*   Track agent performance, token expenditures, and decisions visually.

## 3. Decided (The Architectural Decision)
We implement a stateful **Conversational Session & Forking Architecture** mapped directly to the local Git workspace:
1.  **Conversational Session:** Every interaction is bound to a unique `session_id` persisted in SQLite.
2.  **Branch-Isolated Forking:** When a user clicks "Fork Conversation" in the Web Console (`openexec-web`), the Go backend dynamically runs `git checkout -b fork/<session_id>` behind the scenes, creating a physical isolation boundary.
3.  **Visual Diff & Rollback:** The Web Console displays real-time, interactive file diffs using Monaco Editor. If a path fails, the developer can click "Rollback State," and OpenExec runs `git reset --hard` and deletes the active session checkpoints, instantly returning the workspace and DB state to the chosen historical node.

## 4. Rejected/Neglected Alternatives
*   *In-Memory State Copying:* Rejected. Trying to duplicate directory states in-memory or copy files to temp folders is slow, heavy, and drifts from Git's native, highly optimized tree-tracking.

## 5. Consequences & Tradeoffs
*   *Positives:* Zero-risk requirement exploration; immediate state recovery; beautiful visual audit trails of all file mutations.
*   *Negatives:* Generates a large collection of local, transient git branches that must be garbage-collected periodically.

## 6. Dependencies & References
*   Exposed via REST API `/api/v1/sessions` (documented in `docs/API_REFERENCE.md`).
*   Implemented inside `internal/conversation/` and the `openexec-web` React interface.
