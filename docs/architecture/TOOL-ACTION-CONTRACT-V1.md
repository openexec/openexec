# OpenExec Tool Action Contract v1

This document defines the runtime contract for deterministic blueprint actions. It ensures that when a blueprint specifies an action (e.g., `action: run_tests`), the execution is explicit, typed, policy-checked, and replayable.

## 1. Design Goals
Tool actions must be:
*   **Deterministic in interface:** Inputs and outputs follow strict schemas.
*   **Runtime-owned:** The Go runtime handles execution, not the LLM.
*   **Policy-checked:** Permissions are evaluated before execution.
*   **Replayable:** Recorded inputs and outputs allow for exact reconstruction.

**What it is NOT:** Arbitrary shell snippets in blueprint files or model-defined behaviors.

## 2. Core Execution Model
Every deterministic action follows this lifecycle:
`Resolve Inputs` → `Validate Schema` → `Check Policy` → `Execute` → `Persist Outputs` → `Emit Events` → `Return Typed Result`

## 3. Standard Action Envelope

### Request
```json
{
  "action": "run_tests",
  "run_id": "run_123",
  "inputs": { "test_scope": "changed" },
  "context": { "toolset": "coding_backend", "sandbox": "workspace_write" }
}
```

### Success Result
```json
{
  "status": "success",
  "artifacts": [{ "name": "test_report", "artifact_id": "art_001" }],
  "summary": "12 tests executed, 0 failed"
}
```

## 4. Policy Check Contract
Before any action runs, the policy engine evaluates the request:
`policy.evaluate(action, inputs, workspace, sandbox, toolset)`

Possible outcomes: `allow`, `block`, or `require_approval`.

## 5. Required Actions for v1

| Action | Purpose | Outputs |
| :--- | :--- | :--- |
| `build_context_pack` | Assemble local context for the next agent step. | `context_pack` |
| `apply_patch` | Apply a structured patch artifact to the workspace. | `patch_applied` |
| `run_linters` | Execute the configured lint pipeline. | `lint_report` |
| `run_tests` | Execute the project test suite. | `test_report` |
| `generate_diff_report` | Create a human-readable diff of changes. | `diff_report` |
| `prepare_review_bundle`| Bundle artifacts for the review stage. | `review_bundle` |
| `summarize_run` | Produce a deterministic summary of the run. | `summary` |

## 6. Workspace & Artifacts
*   **Workspace:** Every action runs against an explicit workspace (id, path, branch). Actions never implicitly operate on ".".
*   **Artifacts:** Named outputs passed between stages, mapped to SQLite records and optional disk blobs.

## 7. Idempotency & Timeouts
*   **Idempotent:** `build_context_pack`, `run_linters`, `run_tests`.
*   **Non-idempotent:** `commit_changes`, `push_branch`.
*   **Timeouts:** Actions must declare a timeout class (`fast`, `medium`, `long`).

---
**Summary:** This contract prevents deterministic stages from becoming opaque shell commands, ensuring the blueprint remains enforceable and the runtime stays inspectable.
