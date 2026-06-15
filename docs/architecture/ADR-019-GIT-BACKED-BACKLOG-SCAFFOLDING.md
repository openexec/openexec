# ADR-019: Git-Backed Backlog Queues & Trunk-Based Swarm Scaffolding

*   **ID:** ADR-019  
*   **Status History:**
    *   2026-06-06: Created by Systems Architect. Status: Proposed. Implementation: Aspirational (Planned).
*   **Decides on:** Project Scaffolding, Git-Backed State, and Agile Swarm Workflows

---

## 1. Context and Problem Statement
Currently, OpenExec operates on a local-first model where task states, stories, and run histories are stored inside a local, transaction-locked SQLite database (`.openexec/openexec.db`). While this is perfect for single-machine, local-only CLI execution, it creates a severe bottleneck when scaling to distributed building systems:
1.  **Central Daemon Requirement:** Multi-machine collaboration (e.g. parallel agents or remote SRE operators working on the same backlog) requires a shared running backend server to coordinate locks.
2.  **No Native Git Visibility:** Task progress, claims, and assignments are invisible inside standard Git histories and pull request reviews.
3.  **Ad-Hoc Branching:** Short-lived feature branches are created ad-hoc, but there is no structured pattern that binds the lifecycle of a task directly to trunk-based Git flows.

We need a decentralized, serverless mechanism to scaffold new projects so they support highly collaborative, asynchronous, and agile multi-agent building systems.

## 2. Facing (Decision Drivers)
*   Enable distributed multi-agent coordination with **zero central server infrastructure**.
*   Adopt **Trunk-Based Development** (short-lived feature branches, fast validation gates, immediate merging to main).
*   Expose task progress, claims, and requirements directly in Git history.
*   Retain backward compatibility with local SQLite audits.

## 3. Decided (The Architectural Decision)
We decide to introduce **Git-Backed Backlog Queues and Trunk-Based Swarm Scaffolding** as the default structure for new projects initialized via OpenExec (e.g. `openexec init`).

### A. The Scaffolding Structure (`openexec init`)
When a user initializes OpenExec inside a new repository, the system scaffolds a file-backed, Git-tracked workspace:

```
my-project/
├── .openexec/
│   ├── backlog/            # File-backed task queue
│   │   ├── US-001.md       # User Story: YAML frontmatter + markdown requirements
│   │   └── T-101.md        # Task: YAML frontmatter + verification script
│   ├── claims/             # Active agent/developer lease locks
│   │   └── T-101.claim     # Claim: YAML (agent_id, timestamp, ttl)
│   ├── skills/             # Project-specific reusable code/instruction libraries
│   └── openexec.yaml       # Project configuration (allowlists, gates, SRE configs)
└── docs/
    └── architecture/       # ADR-driven architecture registry
        └── README.md
```

### B. The Trunk-Based Coordination Lifecycle (The Unsorry Loop)
We completely eliminate the database lock bottleneck for distributed tasks. The Git repository's remote server (e.g. GitHub/GitLab) acts as the state synchronization bus:

1.  **Backlog Registration:** New features are written as markdown files directly to `.openexec/backlog/` on the `main` branch (trunk).
2.  **Advisory Claiming:** When an agent (or human developer) wants to work on `T-101`, they create a claim file `.openexec/claims/T-101.claim` on their local machine.
3.  **Short-Lived Branching:** The agent runs:
    ```bash
    git checkout -b task/T-101
    git add .openexec/claims/T-101.claim
    git commit -m "claim: lock T-101 for agent-abc"
    ```
4.  **Local Implementation & Gates:** The agent implements code on the branch, running local validation gates (lint, test).
5.  **Merge & Progress Sync:** Once gates pass, the agent opens a Pull Request or merges directly back to `main`. 
    *   During the merge, the claim file is deleted, and the task status inside `.openexec/backlog/T-101.md` is updated to `completed`.
    *   First push to merge into `main` (trunk) wins the claim locks. Conflict resolutions are handled natively by Git merge logic.

---

## 4. Rejected/Neglected Alternatives
*   *Central REST API Coordinator:* Rejected. Operating a centralized API coordinator adds hosting costs, maintenance tax, and single-point-of-failure vulnerabilities. Leveraging the Git repository itself as the state store is zero-cost, serverless, and highly secure.

---

## 5. Consequences & Tradeoffs
*   *Positives:* Zero central server dependency; total progress visibility inside pull requests; perfect trunk-based agile rhythm; native history of who solved which task and how.
*   *Negatives:* Increases Git commit noise. If many parallel agents are committing claims, Git merge conflicts can arise in `.openexec/backlog/` (mitigated by automated merge resolvers or isolated task file naming).

---

## 6. Dependencies & References
*   Inspired by `unsorry`'s serverless coordination model.
*   Expands the `internal/worktree` isolation model defined in `ADR-017`.
*   Integrates with the `mcp-serve` backlog tools defined in `ADR-011` and `ADR-010`.
