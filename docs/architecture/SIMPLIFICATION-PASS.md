# OpenExec V1 Simplification Pass

To ensure OpenExec remains a minimal, deterministic, and maintainable AI coding runtime, the following "7 Deletions" must be executed during the v1 stabilization phase.

## 1. Delete Dual Runtime Behavior
**Problem:** Separate engines for chat, execution, and planning create duplicate state, prompts, and routing logic.
**Solution:** Move to one canonical flow: **Session → Task → Run**.
*   Chat is strictly the entrypoint.
*   Execution is strictly a Run.
*   No parallel logic trees or "brain" duplication.

## 2. Delete Legacy JSON State as Live Truth
**Problem:** Relying on `stories.json` or `tasks.json` alongside SQLite creates "archaeology" instead of a single runtime truth.
**Solution:** SQLite is the **only** authoritative state.
*   JSON/YAML exist only as input definitions, exports, or debug artifacts.
*   If recovery depends on a JSON file, that code must be removed.

## 3. Delete Prompt-Driven Workflow Control
**Problem:** If prompts decide the next stage or whether to test, the prompt is secretly the workflow engine.
**Solution:** Transitions, retries, and routing belong in the **Blueprint Engine**.
*   LLM responsibility is limited to: Implementation, Diagnosis, Summarization, and Review Reasoning.

## 4. Delete Flat Tool Universes
**Problem:** Exposing all tools by default causes model confusion and increases leakage risk.
**Solution:** Enforce **Curated Toolsets** (e.g., `repo_readonly`, `coding_backend`).
*   Each run receives exactly one default toolset with tightly controlled access.

## 5. Delete Arbitrary Shell as a First-Class Primitive
**Problem:** `shell("do whatever")` destroys determinism, replayability, and policy enforcement.
**Solution:** Prioritize **Typed Deterministic Actions** (e.g., `apply_patch`, `run_tests`).
*   Arbitrary shell exists only as a high-risk, approval-required capability.

## 6. Delete Premature Multi-Agent/Distributed Ideas
**Problem:** Swarms, remote workers, and complex delegation layers are second-system traps for v1.
**Solution:** Stick to **One Runtime, One Engine, One Active Run**.
*   Focus on runtime discipline over "agentness."

## 7. Delete Overlapping Orchestration Models
**Problem:** Coexisting concepts (legacy phases vs. new blueprints) make the system impossible to reason about.
**Solution:** The **Blueprint Engine** is the only execution model.
*   All other flows (e.g., analytical tasks) must become artifacts or helper subsystems for the blueprint.

---

## Descoping Priorities (Postpone)
*   **A. Big UI Ambitions:** Focus on CLI and terminal timeline first; dashboard is secondary.
*   **B. Broad MCP Ambitions:** Stick to a local tool registry before building a "Toolshed" platform.
*   **C. Hyper-generic Abstraction:** Normalize I/O and streaming; don't try to hide every model difference.

## Hard V1 Rule
If a feature does not clearly improve **determinism, local control, state clarity, tool safety, or replayability**, it must be postponed.

---
**Summary:** OpenExec wins by being a small core with typed actions and explicit workflows, not by having more "magic" than its competitors.
