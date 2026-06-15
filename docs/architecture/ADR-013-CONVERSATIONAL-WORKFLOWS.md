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
We implement a stateful **Database-Backed Conversational Session & Forking Architecture**:
1.  **Conversational Session:** Every interaction is bound to a unique `session_id` persisted in the SQLite `openexec.db` database.
2.  **DB-Backed Forking:** When a developer forks a conversation in the Web Console, the Go backend copies the active session message records and run checkpoints into a new database row via `SessionRepo.ForkSession`, referencing the parent session. This occurs entirely in memory/SQLite with zero file-system or Git branch overhead.
3.  **Visual Diff & Local Git Control:** The Web Console displays real-time, interactive file diffs using Monaco Editor. If a code path fails, the developer can manually use Git to discard local workspace modifications or run standard git checkouts, while the conversational database safely maintains both separate message trees.

## 4. Rejected/Neglected Alternatives
*   *Git Branch-Backed Forking:* Rejected. Spawning physical Git branches for every conversational fork is heavy, slow, and pollutes the developer's local repository with dozens of transient branches that require constant garbage collection. Keeping the forks inside SQLite is instantaneous and zero-overhead.

## 5. Consequences & Tradeoffs
*   *Positives:* Instantaneous, zero-overhead conversation forking; safe exploration of alternative prompts; minimal disk and Git footprint.
*   *Negatives:* The local file system still has only one active worktree—if a developer wants to compile and run both forks simultaneously, they must manually clone or stash changes to switch workspace states.

## 6. Dependencies & References
*   Exposed via REST API `/api/v1/sessions` (documented in `docs/API_REFERENCE.md`).
*   Implemented inside `internal/conversation/` and the `openexec-web` React interface.
