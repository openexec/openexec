# OpenExec Runtime Package Layout (Go)

This document defines the modular package structure for the OpenExec Go runtime. It follows a "Kernel + Subsystems" pattern to ensure isolation, testability, and deterministic control.

## 1. Design Goals
*   **Isolate deterministic control logic** from agentic reasoning.
*   **Decouple LLM providers** from the execution engine.
*   **Enforce strict dependency flow** to avoid circular imports.
*   **Keep the core runtime small** and delegate to specialized subsystems.

## 2. Repository Layout

```text
openexec/
├── cmd/openexec/          # CLI Entry point
├── internal/
│   ├── runtime/           # Core orchestrator (Sessions, Tasks, Runs)
│   ├── blueprint/         # DSL interpreter and execution loop
│   ├── actions/           # Deterministic Tool Action implementations
│   ├── tools/             # Tool harness and curated toolsets
│   ├── policy/            # Safety & Sandbox engine
│   ├── context/           # Local context pack assembly
│   ├── router/            # Local LLM routing layer
│   ├── model/             # LLM provider adapters (OpenAI, Anthropic, etc.)
│   ├── workspace/         # Git worktree and filesystem isolation
│   ├── state/             # SQLite persistence layer
│   ├── artifacts/         # Persistent output management
│   └── events/            # Observability and audit event system
├── pkg/
│   ├── api/               # Public API definitions
│   └── schema/            # Shared data structures
├── blueprints/            # YAML Blueprint DSL files
└── migrations/            # SQLite database migrations
```

## 3. Subsystem Responsibilities

| Package | Responsibility | Key Interface |
| :--- | :--- | :--- |
| `runtime` | Lifecycle coordination | `Runtime.RunTask(taskID) (RunID)` |
| `blueprint` | Workflow execution | `Engine.ExecuteRun(runID)` |
| `actions` | Deterministic tasks | `Action.Execute(ctx, req) (Result)` |
| `policy` | Safety enforcement | `Engine.Evaluate(req) (Decision)` |
| `tools` | Capability harness | `Tool.Execute(ctx, req) (Result)` |
| `state` | SQLite storage | `RunStore.CreateRun(taskID)` |
| `router` | Intent classification | `Router.RouteRequest(input) (Plan)` |
| `model` | LLM normalization | `Model.Generate(ctx, req) (Response)` |

## 4. Dependency Flow
To maintain a clean architecture, dependencies must flow downward:
`cmd` → `runtime` → `blueprint` → `actions` → `tools` → `policy` → `workspace` → `state`.

**Parallel Subsystems:** `context`, `router`, `model`, `cache`, `events`, and `artifacts` should not depend on the `runtime` package.

## 5. Implementation Rules
*   **Subsystem Mapping:** Every feature must map to exactly one subsystem (e.g., safety belongs in `policy`).
*   **Small Kernal:** The `cmd/openexec` and `internal/runtime` packages should remain minimal.
*   **Componentized AI:** The LLM is treated as a component inside the runtime, not the driver of the runtime.

---
**Summary:** This layout produces a modular, maintainable runtime where the LLM is a strictly controlled resource within a deterministic Go shell.
