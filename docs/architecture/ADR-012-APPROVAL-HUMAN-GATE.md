# ADR-012: Risk-Tiered Approvals & Fail-Closed Human Gates

*   **ID:** ADR-012  
*   **Status History:**
    *   2026-06-06: Created by Systems Architect. Status: Approved. Implementation: Shipped (internal/approval memory gates) & Partial (SQLite persistence).
*   **Decides on:** The Human-in-the-Loop (HITL) Trust Model

---

## 1. Context and Problem Statement
Operating autonomous agents on production systems requires a robust trust boundary. If an agent executes high-risk actions (like pushing code to Git or running Terraform apply on production) without human verification, a single logic error can cause massive downtime. Conversely, forcing human approvals on trivial tasks (like reading logs) destroys development velocity.

## 2. Facing (Decision Drivers)
*   Enforce absolute safety for mutating, high-risk infrastructure actions.
*   Enable frictionless, zero-gate execution for low-risk, read-only diagnostic tasks.
*   Ensure approval states are persistent and survive daemon restarts.

## 3. Decided (The Architectural Decision)
We implement a **Risk-Tiered Approval Gate** in `internal/approval/`:
1.  **Low-Risk (Read-Only):** Exposes tools like `read_file`, `glob`, or `grep`. Runs fully autonomously (`RequiresApproval = false`).
2.  **Medium-Risk (Workspace Writes):** Exposes `write_file`, `git_apply_patch`. Gated by local approval prompts.
3.  **High-Risk (External Mutations):** Exposes `git_push`, `terraform_apply`. **Marked as HITL (Human-in-the-loop) by definition.** They block execution and halt-closed, awaiting explicit operator signature.
4.  **Isolated Approvals Database:** Pending approvals are written to a dedicated, isolated database at `.openexec/approvals.db` to ensure separation from the active workspace progress and story schemas.
5.  **Bounded Wait & Fail-Closed Timeout:** When an approval is requested, the system halts and polls `.openexec/approvals.db` for an operator's decision. It enforces a **bounded wait timeout (default: 5 minutes)**. If the timeout is reached without explicit human signature, the gate **fails closed**, aborting the active task and rolling back uncommitted changes.
6.  **Anti-Self-Approval:** The agent is physically blocked from calling any approval-bypass tools; only the human operator's console or a signed stdio MCP command can authorize a pending gate.

## 4. Rejected/Neglected Alternatives
*   *Time-out Auto-Approve:* Rejected. Having a pending deployment auto-approve if the human doesn't respond in 5 minutes violates the fail-closed core security mandate.
*   *Unified State Store for Approvals:* Rejected. Keeping approvals in `openexec.db` complicates access permissions since external stdio tools (`mcp-serve`) need lightweight, un-locked, transaction-free access to write operator decisions. Isolating approvals to `approvals.db` minimizes locking contention.

## 5. Consequences & Tradeoffs
*   *Positives:* Total environment-aware safety; immutable audit logs of who approved what.
*   *Negatives:* Introducing manual gates halts fully autonomous "fire-and-forget" continuous pipelines on production branches.

## 6. Dependencies & References
*   Wired inside `internal/approval/` and `internal/mcp/broker.go`.
