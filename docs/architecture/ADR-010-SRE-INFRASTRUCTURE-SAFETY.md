# ADR-010: SRE & Infrastructure Safety Gating

*   **ID:** ADR-010  
*   **Status History:**
    *   2026-06-06: Created by Systems Architect. Status: Approved. Implementation: Shipped (Commit 4c39c08) & Partial (Plan-apply roadmap).
*   **Decides on:** Infrastructure & Deployment Safety Controls

---

## 1. Context and Problem Statement
When deploying infrastructure changes via automation (Terraform, Ansible, Salt, SSH), letting an AI agent execute arbitrary commands can lead to catastrophic failures—such as dropping production databases or purging cloud VPCs. Standard terminal executors are highly vulnerable to command injection and model drift.

## 2. Facing (Decision Drivers)
*   Must enforce absolute non-destructive execution pathways in production.
*   Prevent Time-of-Check to Time-of-Use (TOCTOU) race conditions in plans.
*   Strict validation of all dynamic parameters at the Go runtime level.

## 3. Decided (The Architectural Decision)
We mandate a multi-tiered safety boundary for SRE tasks:
1.  **Capability Deprivation:** Destructive CLI commands (like `terraform destroy`) are completely omitted from compiled tools.
2.  **Strict Argument Arrays:** Execute commands via programmatic slice arrays (`exec.CommandContext`), completely bypassing shell expansions (`sh -c`).
3.  **Saved-Plan Workflow:** For Terraform/Ansible, the planning stage outputs a binary state plan (`terraform plan -out=deploy.tfplan`). The apply stage *only* accepts this binary file path, preventing drift.
4.  **Deterministic Plan Parsing:** We parse `terraform show -json` in Go. If a `delete` or `replace` action is detected on critical tiers, the pipeline immediately halts and triggers a `decision-point` signal.

## 4. Rejected/Neglected Alternatives
*   *Reactive Regex Fencing:* Rejected. Agents easily bypass regex filters by writing wrapper scripts (e.g. Python files using `os.remove`) and executing those scripts.
*   *LLM-Based Plan Inspection:* Neglected. Relying on an LLM to check if a plan is safe is non-deterministic, slow, and expensive.

## 5. Consequences & Tradeoffs
*   *Positives:* Zero command injection vulnerabilities; absolute protection against accidental infrastructure deletions.
*   *Negatives:* Restricts the agent to a smaller, allowlisted set of pre-approved playbooks and state files.

## 6. Dependencies & References
*   Matches roadmap specifications in `docs/SRE_ORCHESTRATION_ROADMAP.md`.
*   Depends on `internal/toolset` and `pkg/db/sqlitecfg` (for WAL concurrency).
