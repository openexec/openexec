# OpenExec API Reference

This document provides a comprehensive API reference for OpenExec's integrated orchestration platform. It covers all HTTP endpoints, WebSocket protocols, MCP tools, and data types used for multi-project management.

## Table of Contents

- [Overview](#overview)
- [HTTP REST API](#http-rest-api)
  - [Task Execution API](#task-execution-api)
  - [Session API](#session-api)
  - [Project API](#project-api)
  - [Model & Provider API](#model--provider-api)
  - [Files API](#files-api)
  - [Usage & Cost API](#usage--cost-api)
- [WebSocket Protocol](#websocket-protocol)
- [MCP Tools](#mcp-tools)
- [Event Types](#event-types)
- [Signal Protocol](#signal-protocol)
- [Data Types (DTOs)](#data-types)
- [Error Codes](#error-codes)

---

## Overview

OpenExec exposes a unified API served by the single-binary OpenExec server (`openexec start`, port 8080 by default).

| API Type | Protocol | Base Path | Purpose |
|----------|----------|-----------|---------|
| Runs API (v1) | HTTP/JSON | `/api/v1/runs` | Planning and execution (recommended) |
| Blueprint API | HTTP/JSON | `/api/v1/blueprints` | Blueprint management and execution |
| Toolset API | HTTP/JSON | `/api/v1/toolsets` | Toolset listing and configuration |
| FWU API | HTTP/JSON | `/api/fwu` | Task-based execution control (**deprecated**) |
| Session API| HTTP/JSON | `/api/sessions`| Chat history and state management |
| Project API| HTTP/JSON | `/api/projects`| Multi-project discovery and init |
| Model API  | HTTP/JSON | `/api` | Provider and model metadata |
| WebSocket | WS/JSON | `/ws` | Real-time conversation streaming |

### Execution Architecture

OpenExec uses a strict separation of concerns:

- **MCP is the sole execution plane.** All file operations, command execution, and tool invocations flow through the MCP (Model Context Protocol) server with mandatory workspace scoping, permission gating, and audit logging.

- **The daemon owns all orchestration.** The CLI is a thin client that triggers server-side execution via the API. No local orchestration occurs in CLI commands.

- **Deterministic state machine.** Runs progress through blueprint stages (gather_context -> implement -> lint -> test -> review) governed by a versioned state machine. See [STATE_MACHINE.md](./STATE_MACHINE.md) for details.

- **DCP is suggest-only.** The Deterministic Control Plane provides intent routing suggestions but does not execute tools directly. All execution requests are forwarded to MCP.

---

## HTTP REST API

### Session API

Base path: `/api/sessions`

#### Create Session
`POST /api/sessions`

Creates a new conversation session bound to a project workspace.

**Request Body:**
```json
{
  "projectPath": "/Users/perttu/projects/my-app",
  "provider": "anthropic",
  "model": "claude-3-5-sonnet-20241022",
  "title": "Optional title"
}
```

**Response (201 Created):**
Returns a [SessionDTO](#session-dto).

---

#### List Sessions
`GET /api/sessions`

Lists all sessions, optionally filtered by project.

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `project_path` | string | Filter sessions by absolute project path |

**Response:**
`Array<SessionDTO>`

---

#### Get Session Messages
`GET /api/sessions/{id}/messages`

Retrieves paginated conversation history.

**Query Parameters:**
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit`   | int  | 50      | Max messages |
| `offset`  | int  | 0       | Offset for pagination |

**Response:**
```json
{
  "messages": [
    {
      "id": "msg_123",
      "sessionId": "sess_abc",
      "role": "assistant",
      "content": "Message text",
      "tokensInput": 100,
      "tokensOutput": 50,
      "costUsd": 0.001,
      "createdAt": "2024-03-01T12:00:00Z"
    }
  ],
  "pagination": {
    "offset": 0,
    "limit": 50,
    "hasMore": false,
    "totalCount": 1
  }
}
```

---

#### Start Run from Session
`POST /api/v1/sessions/{id}/run`

Converts an existing chat session into a deterministic, blueprint-driven run. This endpoint allows users to escalate a conversation into an automated execution flow with streaming progress and resumability.

**Request Body:**
```json
{
  "blueprint_id": "standard_task",
  "mode": "workspace-write",
  "task_description": "Implement user authentication",
  "use_summary": false,
  "messages": 5
}
```

**Request Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `blueprint_id` | string | `standard_task` | Blueprint to execute (`standard_task`, `quick_fix`) |
| `mode` | string | `workspace-write` | Execution mode (`read-only`, `workspace-write`, `danger-full-access`) |
| `task_description` | string | (derived) | Explicit task description; if omitted, derived from session messages |
| `use_summary` | bool | `false` | Use session summary instead of full messages |
| `messages` | int | `0` | Number of recent messages to include for context (0 = last user message only) |

**Task Derivation:**

When `task_description` is not provided, the endpoint derives the task from the session's conversation:
- If `messages=0` or `messages=1`: Uses the content of the last user message
- If `messages>1`: Includes recent conversation context with the last user message

**Response (201 Created):**
```json
{
  "run_id": "BP-standard-20240315-120000",
  "blueprint_id": "standard_task",
  "session_id": "sess_abc123",
  "status": "starting"
}
```

**Errors:**

| Status | Condition |
|--------|-----------|
| `400 Bad Request` | Invalid `blueprint_id`, `mode`, or missing session ID |
| `400 Bad Request` | Could not derive task description (no user messages in session) |
| `404 Not Found` | Session does not exist |
| `503 Service Unavailable` | Manager not available |

**Example Usage:**

```bash
# Start a run with explicit task description
curl -X POST "http://localhost:8080/api/v1/sessions/sess_abc123/run" \
  -H "Content-Type: application/json" \
  -d '{"task_description": "Add user authentication", "blueprint_id": "standard_task"}'

# Start a run with derived task from session
curl -X POST "http://localhost:8080/api/v1/sessions/sess_abc123/run" \
  -H "Content-Type: application/json" \
  -d '{"blueprint_id": "quick_fix"}'
```

**Monitoring Progress:**

After starting a run, monitor progress via:
- **WebSocket**: Connect to `/ws` and subscribe to the run ID to receive real-time events
- **Timeline API**: `GET /api/v1/runs/{run_id}/timeline` for stage history and checkpoints
- **Run Status**: `GET /api/v1/runs/{run_id}` for current status and output

---

### Project API

Base path: `/api/projects`

#### List Projects
`GET /api/projects`

Scans the `projects-dir` for directories containing an `openexec.yaml`.

**Response:**
```json
[
  {
    "name": "my-app",
    "path": "/absolute/path/to/my-app",
    "type": "fullstack-webapp"
  }
]
```

---

#### Initialize Project
`POST /api/projects/init`

Creates a new OpenExec workspace in a target folder.

**Request Body:**
```json
{
  "name": "my-new-app",
  "path": "../my-new-app"
}
```

**Response (201 Created):**
Returns the created [ProjectConfig](#projectconfig-dto).

---

#### Run Wizard
`POST /api/projects/wizard`

Interacts with the Guided Intent Interviewer.

**Request Body:**
```json
{
  "projectPath": "/path/to/project",
  "message": "User input text",
  "state": "serialized-json-state",
  "model": "sonnet",
  "render": false
}
```

**Response:**
- If `render: false`: Returns `WizardResponse` (JSON).
- If `render: true`: Returns generated `INTENT.md` (Markdown).

---

### Model & Provider API

#### List Providers
`GET /api/providers`

Returns the availability status of AI providers based on environment variables.

**Response:**
```json
[
  {
    "name": "anthropic",
    "available": true
  },
  {
    "name": "openai",
    "available": false,
    "reason": "OPENAI_API_KEY is required"
  }
]
```

---

#### List Models
`GET /api/models`

Returns all models known to the OpenExec Model Catalog.

**Response:**
```json
[
  {
    "id": "claude-4-6-sonnet-20260215",
    "name": "Claude 4.6 Sonnet",
    "provider": "anthropic",
    "capabilities": {
      "streaming": true,
      "tool_use": true,
      "max_context_tokens": 400000
    },
    "price_per_m_input_tokens": 3.0,
    "price_per_m_output_tokens": 15.0
  }
]
```

---

### Files API

#### List Directories
`GET /api/directories`

Browses the local filesystem for directory selection in the UI.

**Query Parameters:**
| Parameter | Description |
|-----------|-------------|
| `path` | Path to browse (defaults to projects root) |

**Response:**
```json
[
  { "name": "..", "path": "/parent" },
  { "name": "my-folder", "path": "/parent/my-folder" }
]
```

---

## Data Types (DTOs)

### Session DTO
Mapped to UI `Session` interface. Uses **camelCase** tags.

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | UUID |
| `projectPath` | string | Absolute path |
| `provider` | string | anthropic, openai, gemini |
| `model` | string | model id |
| `title` | string | display name |
| `status` | string | active, archived, deleted |
| `createdAt` | string | RFC3339 |
| `updatedAt` | string | RFC3339 |

### Project Discovery DTO
| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Name from openexec.yaml |
| `path` | string | Absolute path |
| `type` | string | fullstack-webapp, etc. |

---

### Blueprint API

Base path: `/api/v1/blueprints`

#### List Blueprints
`GET /api/v1/blueprints`

Returns all available blueprints.

**Response:**
```json
[
  {
    "id": "standard_task",
    "name": "Standard Task",
    "description": "Default blueprint for implementing tasks with lint/test validation",
    "version": "1.0",
    "initial_stage": "gather_context",
    "stages": ["gather_context", "implement", "lint", "test", "review"]
  },
  {
    "id": "quick_fix",
    "name": "Quick Fix",
    "description": "Simplified blueprint for small, targeted fixes",
    "version": "1.0",
    "initial_stage": "implement",
    "stages": ["implement", "verify"]
  }
]
```

---

#### Get Blueprint
`GET /api/v1/blueprints/{id}`

Returns a specific blueprint with full stage definitions.

**Response:**
```json
{
  "id": "standard_task",
  "name": "Standard Task",
  "stages": {
    "gather_context": {
      "name": "gather_context",
      "type": "deterministic",
      "toolset": "repo_readonly",
      "on_success": "implement",
      "create_checkpoint": true
    },
    "implement": {
      "name": "implement",
      "type": "agentic",
      "toolset": "coding_backend",
      "max_retries": 3,
      "timeout": "10m",
      "on_success": "lint",
      "on_failure": "implement"
    }
  }
}
```

---

### Toolset API

Base path: `/api/v1/toolsets`

#### List Toolsets
`GET /api/v1/toolsets`

Returns all available toolsets.

**Response:**
```json
[
  {
    "name": "repo_readonly",
    "description": "Read-only repository operations",
    "tools": ["read_file", "glob", "grep", "git_status"],
    "risk_level": "low",
    "phases": ["gather_context", "review"]
  },
  {
    "name": "coding_backend",
    "description": "Backend implementation tools",
    "tools": ["read_file", "write_file", "git_apply_patch", "run_shell_command"],
    "risk_level": "medium",
    "phases": ["implement", "fix_lint", "fix_tests"]
  }
]
```

---

#### Get Toolset Tools
`GET /api/v1/toolsets/{name}/tools`

Returns tools available in a specific toolset.

**Response:**
```json
{
  "name": "coding_backend",
  "tools": [
    {
      "name": "read_file",
      "description": "Read file contents"
    },
    {
      "name": "write_file",
      "description": "Write file contents"
    }
  ]
}
```

---

### Mode Transitions

Sessions support mode transitions via the Session API:

#### Transition Mode
`POST /api/sessions/{id}/mode`

Transitions a session to a new execution mode.

**Request Body:**
```json
{
  "mode": "task",
  "reason": "user_approved"
}
```

**Valid Transitions:**
| From | To | Condition |
|------|-----|-----------|
| chat | task | `user_approved` |
| task | run | `inputs_ready` |
| run | chat | `checkpoint` |

**Response (200 OK):**
```json
{
  "previous_mode": "chat",
  "current_mode": "task",
  "transitioned_at": "2024-03-15T12:00:00Z"
}
```

---

### FWU API (Deprecated)

Base path: `/api/fwu`

> **Deprecation Notice:** The FWU API (`/api/fwu/*`) is deprecated and will be removed in a future release. New consumers should use the [Runs API (v1)](#runs-api-v1) instead, which provides improved determinism, artifact persistence, and version tracking.

**Endpoints (legacy):**
- `POST /api/fwu/{id}/start` - Start a task (use `POST /api/v1/runs/{id}/start`)
- `GET /api/fwu/{id}/status` - Get task status (use `GET /api/v1/runs/{id}`)
- `GET /api/fwu` - List tasks (use `GET /api/v1/runs`)
- `POST /api/fwu/{id}/pause` - Pause a task
- `POST /api/fwu/{id}/stop` - Stop a task

---

### Runs API (v1)

Base path: `/api/v1/runs`

#### Create Plan
`POST /api/v1/runs:plan`

Generates a project plan from an intent file and persists it as an artifact.

**Request Body:**
```json
{
  "intent_file": "INTENT.md",
  "no_validate": false
}
```

**Input Validation:**
- `intent_file` must be a valid path within the workspace root (no path traversal)
- Sensitive files (`.env`, `.ssh/*`, `*.pem`, `*.key`, etc.) are blocked by the denylist
- Returns `400 Bad Request` with a clear error message for invalid paths

**Response (200 OK):**
```json
{
  "plan": { /* ProjectPlan object */ },
  "valid": true,
  "plan_id": "PLAN-20240315-120000",
  "artifact_hash": "abc123def456...",
  "artifact_path": ".openexec/artifacts/plans/abc123def456....json",
  "prompt_version": "1.0.0"
}
```

**Response Fields:**

| Field | Description |
|-------|-------------|
| `plan_id` | Timestamp-based identifier (e.g., `PLAN-20240315-120000`) |
| `artifact_hash` | SHA-256 hash of the serialized JSON bytes (content-addressed) |
| `artifact_path` | Path to the persisted artifact file |
| `prompt_version` | Version of prompt templates used for generation |

**Artifact Hashing:**

The `artifact_hash` is computed as `SHA-256(canonicalized_json)` where `canonicalized_json` is a deterministic JSON representation with:

- **Sorted keys**: All object keys are sorted alphabetically (recursive for nested objects)
- **Consistent formatting**: Indented JSON with standard spacing
- **Normalized values**: Arrays preserve order; primitives use standard JSON encoding

This hash is:

- **Content-addressed**: Identical plans produce identical hashes regardless of original key order
- **Stable across restarts**: The hash depends only on plan content, not timestamps or runtime state
- **Used as filename**: Artifacts are stored at `.openexec/artifacts/plans/<hash>.json`

This enables deduplication and allows clients to verify artifact integrity by recomputing the hash.

**Example Canonicalization:**

```json
// Input (any key order):
{"z": 1, "a": {"y": 2, "x": 1}}

// Canonicalized output (keys sorted):
{"a": {"x": 1, "y": 2}, "z": 1}
```

---

### Tool Call Idempotency

Tool calls support **skip-on-resume** semantics via idempotency keys. When a run is resumed, previously completed tool calls are skipped by checking their idempotency key against the database.

**Key Generation:**

```
idempotency_key = SHA-256(version + ":" + run_id + ":" + tool_name + ":" + canonicalized_args + ":" + tool_registry_version)
```

| Component | Description |
|-----------|-------------|
| `version` | Idempotency key algorithm version (currently `"1"`) |
| `run_id` | Per-run scope ensures same operation in different runs produces different keys |
| `tool_name` | MCP tool name (e.g., `write_file`, `git_apply_patch`) |
| `canonicalized_args` | JSON with sorted keys (same canonicalization as artifacts) |
| `tool_registry_version` | Invalidates keys when tool definitions change |

**Idempotent Tools:**

Only deterministic tools participate in skip-on-resume:

| Tool | Idempotent | Reason |
|------|------------|--------|
| `write_file` | Yes | Same content produces same result |
| `git_apply_patch` | Yes | Same patch produces same result |
| `run_shell_command` | No | Side effects may vary; partial execution risk |
| `read_file` | No | Read-only, no state change to skip |

**Database Constraint:**

Tool calls with idempotency keys are stored with a unique constraint:

```sql
CREATE UNIQUE INDEX idx_tool_calls_idempotency_unique
  ON tool_calls(idempotency_key)
  WHERE idempotency_key IS NOT NULL;
```

This prevents duplicate writes and enables efficient lookup during resume.

---

### Blueprint Event Types

When running in blueprint mode, the following events are emitted:

| Event Type | Description | Fields |
|------------|-------------|--------|
| `blueprint_start` | Blueprint execution started | `blueprint_id` |
| `blueprint_complete` | Blueprint completed successfully | `blueprint_id`, `iteration` |
| `blueprint_failed` | Blueprint failed | `blueprint_id`, `stage_name`, `error` |
| `stage_start` | Stage execution started | `blueprint_id`, `stage_name`, `stage_type`, `attempt` |
| `stage_complete` | Stage completed successfully | `blueprint_id`, `stage_name`, `artifacts` |
| `stage_failed` | Stage failed | `blueprint_id`, `stage_name`, `error` |
| `stage_retry` | Stage retrying | `blueprint_id`, `stage_name`, `attempt` |
| `checkpoint_created` | Checkpoint created | `blueprint_id`, `stage_name` |

**Example WebSocket Event:**
```json
{
  "type": "stage_start",
  "blueprint_id": "standard_task",
  "stage_name": "implement",
  "stage_type": "agentic",
  "attempt": 1,
  "iteration": 2,
  "text": "Starting stage \"implement\" (attempt 1)"
}
```

---

### Run-Step Event Metadata

Events emitted during run execution include version metadata for debugging and reproducibility:

| Field | Description |
|-------|-------------|
| `prompt_version` | Version of prompt templates used |
| `tool_registry_version` | Version of MCP tool definitions |
| `run_state_machine_version` | Version of run state machine logic |

These fields appear in **audit entries** and can be used to correlate behavior changes with version updates.

**WebSocket Events:** Version fields are intentionally omitted from real-time WebSocket events to minimize bandwidth. The WS protocol streams `loop.Event` objects containing progress data (phase, iteration, text, tool calls). For version information during a run, query `GET /api/v1/runs/{id}/steps` which returns audit entries with full metadata.

---

### Feature Flags

#### DCP (Deterministic Control Plane)

Set `OPENEXEC_ENABLE_DCP=true` to enable the optional DCP layer for intent routing.

When disabled (default):
- MCP is the sole execution plane
- All tool calls go through `broker.Authorize`

When enabled:
- DCP routes are registered (`POST /api/v1/dcp/query`, `GET /api/v1/knowledge/*`)
- BitNet-based intent classification available
- **DCP is suggest-only by default** - it returns `IntentSuggestion` objects, not execution results
- MCP remains the sole execution plane; DCP suggests which tool to use

**DCP Response Format (Suggest-Only Mode):**

```json
{
  "result": {
    "tool_name": "deploy",
    "args": {"env": "production"},
    "confidence": 0.87,
    "description": "Deploy to specified environment",
    "is_fallback": false
  }
}
```

**Security Note:** DCP no longer executes tools directly. All execution flows through MCP's authorization and audit system. The `Coordinator.AllowExecution` flag exists for testing but should remain `false` in production.

#### Unified Database Reads

Unified database reads are **enabled by default**. Set `OPENEXEC_USE_UNIFIED_READS=0` to disable.

With unified reads enabled (default):
- `GET /api/v1/runs` queries unified DB with filtering support
- `GET /api/v1/runs/{id}` falls back to DB if not found in memory
- `GET /api/v1/runs/{id}/steps` reads from `run_steps` table instead of audit logger
- Supports historical run data persisted across restarts

When disabled:
- Handlers read from in-memory Manager state (active runs only)
- Historical runs not accessible after restart

**Bake-in Phase**: Enable this flag in staging/development to verify data consistency before production rollout.

#### Legacy FWU Flows

Set `OPENEXEC_ENABLE_LEGACY_FWU=true` to enable deprecated legacy FWU CLI flows.

When disabled (default):
- CLI uses `/api/v1/runs/*` endpoints exclusively
- Legacy `/api/fwu/*` endpoints still available but CLI won't use them

When enabled:
- CLI may use legacy FWU patterns (temporary compatibility)
- Deprecation warnings are suppressed

**Note**: This flag exists for migration only and will be removed in a future release.

---

### Feature Flag Summary

| Flag | Default | Purpose |
|------|---------|---------|
| `OPENEXEC_ENABLE_DCP` | `false` | Enable Deterministic Control Plane (BitNet routing) |
| `OPENEXEC_USE_UNIFIED_READS` | `true` | Read run state from unified DB (set to `0` to disable) |
| `OPENEXEC_ENABLE_LEGACY_FWU` | `false` | Enable deprecated FWU CLI flows |

**Future Flags (Reserved):**
| Flag | Purpose |
|------|---------|
| `OPENEXEC_ENABLE_APPROVAL` | Enable approval manager workflow |
| `OPENEXEC_ENABLE_TELEGRAM` | Enable Telegram bot integration |
| `OPENEXEC_ENABLE_TWILIO` | Enable Twilio SMS/voice integration |
| `OPENEXEC_ENABLE_SUMMARIZATION` | Enable session summarization |
| `OPENEXEC_ENABLE_EVIDENCE_UPLOAD` | Enable S3 evidence upload |

These subsystems are not currently initialized in the core server. They exist as isolated modules for future integration.

---

## Error Codes

The API uses standard HTTP status codes:

- **200 OK**: Success
- **201 Created**: Resource created (Session, Project)
- **400 Bad Request**: Invalid parameters or body
- **404 Not Found**: Resource (Session, Project) doesn't exist
- **500 Internal Error**: Logic failure (see `error` field in JSON response)

**Error Response Format:**
```json
{
  "error": "Reason for failure"
}
```
