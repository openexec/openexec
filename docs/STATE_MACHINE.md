# OpenExec Execution State Machine

This document describes the deterministic state machines that govern execution in OpenExec. OpenExec uses a **Blueprint Engine** as its primary orchestration model, while maintaining a legacy **5-Phase Pipeline** for backward compatibility.

## 1. The Blueprint Engine (Primary)

The Blueprint Engine executes a graph of **Stages**. Each stage is either **Deterministic** (runs local commands) or **Agentic** (requires AI reasoning).

### Standard Task Blueprint
The standard flow for implementing a task follows this sequence:

```
gather_context → implement → lint (→ fix_lint) → test (→ fix_tests) → review
```

| Stage | Type | Description |
|-------|------|-------------|
| `gather_context` | Deterministic | Assembles files, symbols, and project metadata. |
| `implement` | Agentic | Frontier model generates code changes/patches. |
| `lint` | Deterministic | Executes project-specific linters (e.g., `go vet`, `ruff`). |
| `fix_lint` | Agentic | Triggered on lint failure; AI attempts to fix errors. |
| `test` | Deterministic | Executes project test suite (e.g., `go test`, `pytest`). |
| `fix_tests` | Agentic | Triggered on test failure; AI attempts to fix regressions. |
| `review` | Agentic | Final quality check and implementation summary. |

### Blueprint Features
- **Conditional Routing:** Stages can route to different next stages based on success or failure.
- **Automatic Retries:** Agentic stages support bounded retries for self-correction.
- **Checkpoints:** State is persisted at key stages to allow resuming after a pause or crash.

---

## 2. Legacy 5-Phase Pipeline (Deprecated)

The original pipeline uses a fixed sequence of phases, each assigned to a specific AI persona.

| Phase | Code | Purpose | Agent |
|-------|------|---------|-------|
| Technical Design | `TD` | Analyze task, produce structured plan | clario |
| Implement | `IM` | Execute the plan, make code changes | spark |
| Review | `RV` | Quality review, routing decision | blade |
| Refactor | `RF` | Address review feedback | hon |
| Feedback Loop | `FL` | Final validation and summary | clario |

### Legacy Phase Flow
`TD` → `IM` → `RV` (→ `RF` → `FL` or back to `IM`) → `Done`

---

## 3. Run Statuses

Regardless of the engine used, every **Run** progresses through these top-level statuses:

| Status | Description |
|--------|-------------|
| `pending` | Created but not yet started. |
| `running` | Actively executing stages/phases. |
| `paused` | Temporarily suspended (waiting for human approval or retry backoff). |
| `complete` | Successfully finished all requirements. |
| `failed` | Encountered an unrecoverable error or exhausted retries. |
| `stopped` | Manually terminated by the operator. |

---

## 4. Observability & Events

The state machine emits versioned events to the **Audit Vault** (`.openexec/data/audit.db`):

| Event | Description |
|-------|-------------|
| `phase_start` / `stage_start` | Execution unit initiated. |
| `iteration_start` | Individual loop iteration begins. |
| `tool_use` | Agent invoked a specific tool (e.g., `write_file`). |
| `route_decision` | Transition choice made by the engine or agent. |
| `operator_attention` | Human intervention required (e.g., max retries reached). |

---

## 5. Determinism & Replay

OpenExec ensures reliability through:
1. **Idempotency Keys:** Prevent duplicate tool executions during a resume.
2. **Artifact Hashing:** All patches and logs are content-addressed.
3. **Event Sourcing:** The full execution history is stored, allowing `openexec replay <run-id>`.

---
*Key Implementation: `internal/blueprint/`, `internal/pipeline/`, `pkg/db/state/`*
