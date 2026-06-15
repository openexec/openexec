# ADR-004: Deterministic Orchestration vs. Vibe Coding

*   **ID:** ADR-004
*   **Status:** Approved
*   **Author:** Systems Architect
*   **Date:** 2026-06-06
*   **Decides on:** Core Orchestration Philosophy

---

## 1. Context and Problem Statement
A prevailing trend in AI engineering is "vibe coding"—giving a massive, unstructured prompt to a frontier model (like Claude 3.5 Sonnet or GPT-4o) and letting it autonomously iterate in a REPL loop until the code "looks right." 

**The empirical failure of vibe coding:**
1.  **State Drift & Regression:** Without bounded tasks, an AI often breaks functioning subsystems while trying to fix an unrelated bug.
2.  **Unpredictable Execution:** If the API connection drops or the prompt gets too long, the context is lost, and the state is unrecoverable.
3.  **Auditing Nightmare:** In regulated environments (finance, public sector), it is unacceptable to have untraceable AI decisions mutating production codebases.

## 2. Decision Drivers
*   Execution must be fully auditable and replayable.
*   System states must transition predictably (no hidden "black-box" loops).
*   AI must act within explicitly defined constraints, not guess the architecture.

## 3. Considered Options

### Option A: Open-Ended Agentic Loop (Vibe Coding)
*   **Pros:** Easy to implement, feels highly flexible, matches default chat interfaces.
*   **Cons:** Fails spectacularly on large codebases due to context window fatigue and unconstrained side-effects.

### Option B: Deterministic State Machine (OpenExec Method)
*   **Pros:** AI is treated as a computational worker, not a project manager. Execution is locked into a strict DAG (Directed Acyclic Graph) of pipelines.
*   **Cons:** Higher initial setup cost for defining pipeline stages.

## 4. Architectural Decision (The Chosen Path)
**We choose Option B: The Deterministic State Machine.**

OpenExec enforces that the **Runtime governs the LLM, not the other way around.** 
We implement a strict multi-stage blueprint for every action:
1.  `gather_context`
2.  `implement`
3.  `lint`
4.  `test`
5.  `review`

If an agent fails a quality gate, it does not invent a new strategy; it follows the retry backoff limits defined in the orchestrator. If it hits the `MaxIterations` limit, it deterministically emits a `ThrashingDetected` event and halts.

### Why this option wins:
It produces predictable, verifiable outcomes. By isolating the AI's reasoning into strictly bounded stages, we ensure that the generated code is structurally aligned with the project's actual requirements, not the AI's hallucinated assumptions.
