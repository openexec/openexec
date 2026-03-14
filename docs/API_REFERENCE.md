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
| FWU API | HTTP/JSON | `/api/fwu` | Task-based execution control (**deprecated**) |
| Session API| HTTP/JSON | `/api/sessions`| Chat history and state management |
| Project API| HTTP/JSON | `/api/projects`| Multi-project discovery and init |
| Model API  | HTTP/JSON | `/api` | Provider and model metadata |
| WebSocket | WS/JSON | `/ws` | Real-time conversation streaming |

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

Set `OPENEXEC_USE_UNIFIED_READS=true` to enable reading run state from the unified database.

When disabled (default):
- Handlers read from in-memory Manager state (active runs only)
- Historical runs not accessible after restart

When enabled:
- `GET /api/v1/runs` queries unified DB with filtering support
- `GET /api/v1/runs/{id}` falls back to DB if not found in memory
- `GET /api/v1/runs/{id}/steps` reads from `run_steps` table instead of audit logger
- Supports historical run data persisted across restarts

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
| `OPENEXEC_USE_UNIFIED_READS` | `false` | Enable reading run state from unified DB |
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
