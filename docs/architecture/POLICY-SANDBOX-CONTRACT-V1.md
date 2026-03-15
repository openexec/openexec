# OpenExec Policy & Sandbox Contract v1

This document defines the Policy & Sandbox layer which enforces runtime safety rules for all tool actions in OpenExec.

## 1. Purpose
The Policy & Sandbox layer ensures that tool actions operate within defined boundaries, protecting the local system and preventing data leakage.
*   Restrict file system and network access.
*   Enforce sandbox modes and approval checkpoints.
*   Operate independently of model prompts (Runtime-owned safety).

## 2. Core Design Principles
*   **Deny by default:** If a capability is not explicitly allowed, it is denied.
*   **Capability-based:** Actions are evaluated by capability (e.g., `repo_read`) rather than raw tool names.
*   **Sandbox-first:** Every run operates in a specific isolation mode.
*   **Deterministic evaluation:** Policy checks do not depend on LLM reasoning.

## 3. Sandbox Modes

| Mode | Purpose | Key Permissions |
| :--- | :--- | :--- |
| `read_only` | safe analysis | Read-only access to files and git history. No writes/commits. |
| `workspace_write` | default coding | Modify workspace files, run linters and tests. No commits/push. |
| `repo_write` | repo management | Create branches, commit changes (usually requires approval). |
| `danger_full_access`| dev-level access | Arbitrary shell and network access. Requires explicit approval. |

## 4. Capability Model
Each tool action declares required capabilities. Example mapping:
*   `repo_read`: read repository files.
*   `repo_write`: modify workspace files.
*   `git_commit`: create local commits.
*   `network_access`: external HTTP calls.
*   `shell_exec`: run arbitrary shell commands.

## 5. Path Access Policy
File system access is scoped to:
*   `repo_root`: The absolute boundary for the project.
*   `workspace_path`: The active working area.
*   Reads outside `repo_root` are blocked; writes outside `workspace_path` are blocked or require escalation.

## 6. Network Policy
Network access is denied by default. It is allowed only if the sandbox permits it and the host is in the `network_allowlist` (e.g., `github.com`, `api.openai.com`).

## 7. Approval Levels
*   **none:** Automatic execution.
*   **risky:** Asks user for confirmation (e.g., `git_commit`).
*   **critical:** Always asks user (e.g., `shell_exec`).

## 8. Event Logging & Snapshot
Every policy decision is logged as an immutable event (`policy_check`, `policy_block`). When a run begins, a **Policy Snapshot** is stored to ensure the same rules apply throughout the run lifecycle for reproducibility.

---
**Summary:** The Policy & Sandbox layer forms the safety gate of OpenExec, ensuring that runtime rules always override agent intent.
