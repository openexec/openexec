# ADR-009: The Code Review Agent

*   **ID:** ADR-009
*   **Status:** Approved
*   **Author:** Systems Architect
*   **Date:** 2026-06-06
*   **Decides on:** The Verification Pipeline and Adversarial Assessment

---

## 1. Context and Problem Statement
An AI agent acting as a solitary developer suffers from cognitive blindness. If it makes a logical error while generating code, it is unlikely to spot its own mistake when asked to review it immediately afterward. Relying solely on compilers or linters checks for syntax, but misses architectural drift, security flaws, or deviations from the original user intent.

## 2. Decision Drivers
*   Code must be reviewed for structural soundness, not just compilation success.
*   The review process must be separated from the implementation process to break cognitive bias.
*   The pipeline must simulate real-world PR review dynamics.

## 3. Architectural Decision (The Chosen Path)
We introduce an isolated **Reviewer Agent** into the `review` stage of the OpenExec blueprint pipeline.

### The Mechanism:
1.  **Role Separation:** The `Worker` agent executes the `implement` stage. Once the code compiles and passes local tests, the state machine transitions to `review`.
2.  **Clean Context:** The orchestrator boots a *fresh* session for the `Reviewer` agent. The Reviewer does not receive the Worker's scratchpad or reasoning history. It receives only:
    *   The original Blueprint/Story acceptance criteria.
    *   The active Git Diff (`git diff main..task_branch`).
    *   The project's coding standards (e.g., from `intent-compiler/packs`).
3.  **Adversarial Grading:** The Reviewer agent acts as an adversarial gatekeeper. It must explicitly sign off on the diff. If it detects a violation, it generates structured feedback and issues a "Reject."
4.  **Feedback Loop:** A rejection pushes the pipeline back to the `Worker` agent, appending the Reviewer's feedback to the Worker's context for a remediation pass.

### Why this option wins:
Deploying a secondary, contextually isolated LLM instance acts as a semantic quality gate. Just as human teams rely on peer reviews to catch logical gaps that linters miss, this dual-agent setup provides a robust, self-healing architecture that drastically improves the reliability of the merged code.
