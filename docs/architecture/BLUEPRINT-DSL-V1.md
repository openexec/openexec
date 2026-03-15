# OpenExec Blueprint DSL v1

## 1. Design Goals
The DSL provides a structured way to define:
*   Ordered stages.
*   Deterministic vs. Agent stages.
*   Retry caps and success/failure routing.
*   Allowed toolsets and approval checkpoints.
*   Artifact outputs and optional conditions.

**Core Rule:** Blueprint defines workflow shape. Runtime owns execution semantics.

## 2. Format
OpenExec uses **YAML** for v1 because it is human-readable, easy to diff, and maps directly to Go structs.

## 3. Example Structure
```yaml
version: 1
name: backend_bugfix
description: Fix a backend issue with deterministic validation stages

defaults:
  toolset: coding_backend
  sandbox: workspace_write
  max_step_retries: 2
  approval_mode: on_risky_actions

stages:
  - id: gather_context
    type: deterministic
    action: build_context_pack
    outputs:
      - context_pack

  - id: implement
    type: agent
    goal: Implement requested changes using the context pack
    inputs: [context_pack]
    outputs: [patch]
    on_success: run_linters
    on_failure: fail

  - id: run_linters
    type: deterministic
    action: run_linters
    inputs: [patch]
    outputs: [lint_report]
    on_success: run_tests
    on_failure: fix_lints

  - id: fix_lints
    type: agent
    goal: Fix lint errors without changing unrelated behavior
    inputs: [patch, lint_report]
    outputs: [patch]
    retry_limit: 2
    on_success: run_linters
    on_failure: fail
```

## 4. Core Concepts

### 4.1 Stage Types
*   **deterministic:** Executed directly by the runtime (e.g., `build_context`, `run_tests`, `apply_patch`). Maps to a runtime-owned action handler.
*   **agent:** Requires AI reasoning (e.g., `implement`, `review_patch`, `diagnose_failure`).

### 4.2 Artifact Model
Artifacts (e.g., `patch`, `lint_report`) are named outputs passed between stages. Each artifact maps to a real object in SQLite and an optional file blob on disk.

### 4.3 Routing & Terminal States
Stages route to the next `id` based on `on_success` or `on_failure`. Terminal states include:
*   `done`: Execution finished successfully.
*   `fail`: Execution halted due to error.
*   `paused`: Waiting for human approval.

## 5. Blueprint Execution Rules
1.  **Validation:** Schema check, unique IDs, verified routes, and required fields (action for det, goal for agent).
2.  **Retries:** If a stage fails, the runtime increments the attempt count. If `attempts < retry_limit`, it retries; otherwise, it follows `on_failure`.
3.  **Approvals:** If `approval_required: true`, the runtime pauses until the operator approves or rejects the result.

## 6. What the DSL is NOT
To maintain simplicity, the DSL should not contain:
*   Arbitrary shell scripting or expressions.
*   Nested loops or function definitions.
*   Model prompt templates or giant policy blocks.

## 7. Recommended File Layout
Blueprints are stored in `.openexec/blueprints/`:
*   `scoped_code_change.yaml`
*   `debug_ci_failure.yaml`
*   `backend_bugfix.yaml`

---
**Summary:** This DSL is intentionally small to provide explicit workflow definitions and deterministic routing without the complexity of a general-purpose programming language.
