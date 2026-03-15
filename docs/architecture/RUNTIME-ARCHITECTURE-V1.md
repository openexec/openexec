# OpenExec Runtime Architecture v1.0

## 1. System Goal
OpenExec is a local-first AI coding runtime that executes structured development workflows through deterministic orchestration and controlled LLM assistance.

The runtime:
*   Runs locally.
*   Keeps state explicit.
*   Minimizes prompt size.
*   Controls tool access through curated toolsets.
*   Ensures reproducibility and replayability.
*   Reduces data leakage through local context assembly and routing.

## 2. High-Level Architecture
OpenExec follows a converged architecture pattern: a **deterministic local runtime** managing a **small local LLM** for routing/filtering and a **frontier model** for complex reasoning and implementation.

## 3. Core Components

### 3.1 Interaction Layer
*   **Entrypoints:** CLI, IDE plugin, API (future).
*   **Responsibilities:** Start sessions, send messages, approve actions, inspect runs.
*   **Note:** The interaction layer does not contain orchestration logic.

### 3.2 Runtime Core
A single daemon or embedded service responsible for:
*   State lifecycle.
*   Blueprint execution.
*   Tool routing.
*   Model calls.
*   Policy enforcement.

## 4. Canonical Runtime State Model
All runtime activity is stored in **SQLite**.

**Hierarchy:**
`Session` → `Task` → `Run` → `Step` → `ToolCall` → `Artifact`

### 4.1 Sessions
Represents a user interaction context.
*   **Fields:** `session_id`, `created_at`, `user_input_log`, `mode` (chat | task).

### 4.2 Tasks
Structured intent extracted from conversation (e.g., "Fix OAuth token refresh bug").
*   **Fields:** `task_id`, `session_id`, `description`, `toolset`, `repo_scope`, `status`.

### 4.3 Runs
Execution instance of a task.
*   **Fields:** `run_id`, `task_id`, `blueprint_name`, `status`, `created_at`.

### 4.4 Steps
Each stage of blueprint execution.
*   **Fields:** `step_id`, `run_id`, `stage`, `status`, `attempt_count`.

### 4.5 Tool Calls
Recorded tool invocation.
*   **Fields:** `tool_call_id`, `step_id`, `tool_name`, `inputs`, `outputs`, `status`.

### 4.6 Artifacts
Persistent outputs (patches, summaries, diffs, logs, context packs).
*   **Fields:** `artifact_id`, `run_id`, `type`, `location`, `metadata`.

## 5. Blueprint Engine
Blueprints define deterministic workflow graphs. Each stage is either **Deterministic** (code-based) or **Agentic** (AI-based).

**Standard Blueprint Example:**
`gather_context` → `implement_changes` → `run_linters` → `fix_lints` → `run_tests` → `fix_tests` → `review_patch`.

**Pseudo-logic:**
```python
for stage in blueprint:
    execute(stage)
    if success:
        continue
    if failure and retries_remaining:
        retry
    else:
        halt
```

## 6. Retry Policy
Retries are defined per stage and enforced by the runtime (e.g., `implement_retries = 2`, `test_fix_retries = 2`).

## 7. Tool System
Tools are grouped into curated **Toolsets**. Agents receive only the tools required for the task.
*   **Example:** `coding_backend` includes `read_file`, `apply_patch`, `grep`, `run_tests`, and `git_commit`.

## 8. Tool Harness
Provides controlled execution for filesystem access, git operations, shell commands, test execution, linters, formatters, and documentation retrieval.

## 9. Policy Engine
Enforces safety constraints (allowed directories, network access, toolset restrictions, approval checkpoints) before tool execution.

## 10. Context Builder
Constructs minimal context packs.
**Process:** `Deterministic Retrieval` → `Local LLM Ranking` → `Context Compression` → `Sanitized Context Pack`.

## 11. Local Router Model
A small local LLM performing intent classification, task vs chat detection, toolset selection, and sensitivity scoring.

## 12. Model Adapter Layer
Normalizes calls to providers (OpenAI, Anthropic, Gemini, Local) including prompt formatting and structured output normalization.

## 13. Cache Manager
Reduces token usage via model response caches, embedding caches, repo scan caches, and context pack caches.

## 14. Sandbox Model
Execution occurs in isolated environments such as **Git worktrees per run** or temporary workspace directories.

## 15. Observability
Complete event history (step start, tool call, model I/O, policy decisions) allowing full run reconstruction.

## 16. Storage
SQLite is the single source of truth. JSON files (`stories.json`, `tasks.json`) are deprecated.

## 17. Conversation Model
Chat is not a separate system. Flow: `Chat` → `Task` → `Run`. A task is created when a user request becomes actionable.

## 18. Security and Privacy
Leak prevention via deterministic retrieval, local model filtering, and context sanitization.

## 19. Development Principles
*   Single source of truth (SQLite).
*   Deterministic orchestration.
*   Minimal prompt logic.
*   Toolsets over raw tools.
*   Local context assembly first.
*   Bounded agent loops.
*   Replayable runs.

## 20. Future Extensions
Remote execution workers, Slack/ticket integrations, organization-level policy servers, and multi-agent delegation.

---
**Summary:** OpenExec v1.0 priorities control, transparency, and reproducibility over pure automation.
