# ADR-015: Skills & Reusable Agent Libraries

*   **ID:** ADR-015  
*   **Status History:**
    *   2026-06-06: Created by Systems Architect. Status: Approved. Implementation: Shipped (internal/skills & Claude-Code compatibility).
*   **Decides on:** AI Reusability and Instruction Precedence

---

## 1. Context and Problem Statement
When tasked with writing code (like React components or database connections), agents frequently build things from scratch, introducing custom styling, redundant dependencies, or violating established codebase conventions. We need a way to teach agents "how we write code here" without bloating every single system prompt with thousands of lines of instruction text.

## 2. Facing (Decision Drivers)
*   Ensure agents reuse existing workspace components, helpers, and patterns.
*   Enable Claude Code compatibility (uses `.md` skills standard).
*   Provide clean resolution of conflicting instructions across multiple folders.

## 3. Decided (The Architectural Decision)
We establish the **Three-Tier Skills Registry** (`internal/skills/`):
1.  **Skills Format:** Skills are stored as `SKILL.md` markdown files carrying YAML frontmatter (defining name, description, tags, and file-matching patterns) paired with a markdown body (containing reusable code snippets, APIs, or architectural constraints).
2.  **Automated Context Injection:** When an active task is dispatched, the orchestrator scans the files being modified, checks the skills registry, and automatically injects matching skills into the prompt.
3.  **Precedence Hierarchy:** To resolve instruction conflicts, the compiler enforces a strict, scoped inheritance cascade:
    *   `Tier 1: Project-Specific` (`.openexec/skills/`) — Supersedes all.
    *   `Tier 2: Sub-Directory` (`src/components/SKILL.md`) — Supersedes global.
    *   `Tier 3: User-Local` (`~/.claude/skills/`) — Default personal patterns.
    *   `Tier 4: Global Built-in` — Baseline fallback rules.

## 4. Rejected/Neglected Alternatives
*   *Monolithic System Prompts:* Rejected. Putting all rules into a single system prompt consumes massive input tokens on every query and dilutes the model's focus on the active task.

## 5. Consequences & Tradeoffs
*   *Positives:* Zero-overhead instruction scaling; dynamic component reuse; complete compatibility with Claude Code's skill specs.
*   *Negatives:* Drastically increases prompt compilation complexity since the system must parse and resolve Markdown frontmatters at runtime.

## 6. Dependencies & References
*   Detailed in `docs/REUSABILITY_LIBRARIES.md` and `docs/SKILLS_SYSTEM.md`.
*   Implemented inside `internal/skills/`.
