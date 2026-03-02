# OpenExec API Reference

This document provides a comprehensive API reference for OpenExec's Conversational Orchestration system. It covers all HTTP endpoints, WebSocket protocols, MCP tools, and event types.

## Table of Contents

- [Overview](#overview)
- [HTTP REST API](#http-rest-api)
  - [Task Execution API](#task-execution-api)
  - [Session API](#session-api)
  - [Usage & Cost API](#usage--cost-api)
- [WebSocket Protocol](#websocket-protocol)
  - [Connection](#connection)
  - [Client Messages](#client-messages)
  - [Server Messages](#server-messages)
- [MCP Tools](#mcp-tools)
  - [axon_signal](#axon_signal)
  - [read_file](#read_file)
  - [write_file](#write_file)
  - [run_shell_command](#run_shell_command)
  - [git_apply_patch](#git_apply_patch)
  - [Session Fork Tools](#session-fork-tools)
- [Event Types](#event-types)
- [Signal Protocol](#signal-protocol)
- [Data Types](#data-types)
- [Error Codes](#error-codes)

---

## Overview

OpenExec exposes multiple API surfaces:

| API Type | Protocol | Purpose |
|----------|----------|---------|
| REST API | HTTP/JSON | Task management, session control, usage metrics |
| WebSocket | WS/JSON | Real-time conversation streaming, events |
| MCP Server | JSON-RPC 2.0 | Tool execution for AI agents |

### Base URLs

| Environment | REST API | WebSocket |
|-------------|----------|-----------|
| Local Dev | `http://localhost:8080/api` | `ws://localhost:8080/ws` |
| Production | Configured via `daemon.health_port` | Same host |

### Authentication

API keys are passed via environment variables to the OpenExec process. No authentication headers are required for local API calls.

---

## HTTP REST API

### Task Execution API

Base path: `/api/fwu`

#### Start Task Execution

```http
POST /api/fwu/{id}/start
```

Starts execution of a task with the given ID.

**Parameters:**

| Name | In | Type | Required | Description |
|------|-----|------|----------|-------------|
| `id` | path | string | Yes | Task identifier (e.g., "T-001") |

**Request Body:**

```json
{
  "provider": "claude",
  "model": "sonnet",
  "system_prompt": "Optional system prompt override",
  "max_iterations": 50,
  "budget_usd": 10.0
}
```

**Response:**

```json
{
  "task_id": "T-001",
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "running",
  "started_at": "2024-01-15T10:30:00Z"
}
```

---

#### Get Task Status

```http
GET /api/fwu/{id}/status
```

Returns the current status and state of a task.

**Response:**

```json
{
  "task_id": "T-001",
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "running",
  "iteration": 5,
  "total_tokens": 15420,
  "total_cost_usd": 0.0234,
  "last_signal": {
    "type": "progress",
    "reason": "Implemented authentication module"
  },
  "started_at": "2024-01-15T10:30:00Z",
  "last_updated_at": "2024-01-15T10:35:22Z"
}
```

**Status Values:**

| Status | Description |
|--------|-------------|
| `pending` | Task queued, not yet started |
| `running` | Task actively executing |
| `paused` | Task paused by user/system |
| `completed` | Task completed successfully |
| `failed` | Task failed with error |
| `stopped` | Task stopped by user |

---

#### List Active Tasks

```http
GET /api/fwus
```

Returns all currently active tasks.

**Response:**

```json
{
  "tasks": [
    {
      "task_id": "T-001",
      "session_id": "550e8400-e29b-41d4-a716-446655440000",
      "status": "running",
      "iteration": 5,
      "total_cost_usd": 0.0234
    }
  ],
  "total": 1
}
```

---

#### Pause Task

```http
POST /api/fwu/{id}/pause
```

Pauses a running task after the current iteration completes.

**Response:**

```json
{
  "task_id": "T-001",
  "status": "paused",
  "iteration": 5
}
```

---

#### Stop Task

```http
POST /api/fwu/{id}/stop
```

Immediately stops a running task.

**Response:**

```json
{
  "task_id": "T-001",
  "status": "stopped",
  "stopped_at": "2024-01-15T10:40:00Z"
}
```

---

#### Stream Task Events

```http
GET /api/fwu/{id}/events
```

Server-Sent Events (SSE) endpoint for real-time task events.

**Headers:**

```
Accept: text/event-stream
```

**Event Stream Format:**

```
event: loop.start
data: {"session_id":"550e8400...","iteration":1,"message":"Loop execution begins"}

event: iteration.start
data: {"session_id":"550e8400...","iteration":1,"message":"iteration 1"}

event: llm.request_start
data: {"session_id":"550e8400...","iteration":1}

event: tool.call_requested
data: {"session_id":"550e8400...","tool_call":{"id":"tc_123","name":"read_file","status":"pending"}}
```

---

### Session API

Base path: `/api/chat/sessions`

#### Create Session

```http
POST /api/chat/sessions
```

Creates a new conversation session.

**Request Body:**

```json
{
  "project_path": "/path/to/project",
  "provider": "claude",
  "model": "sonnet",
  "title": "Implement user authentication"
}
```

**Response:**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "project_path": "/path/to/project",
  "provider": "claude",
  "model": "sonnet",
  "title": "Implement user authentication",
  "status": "active",
  "created_at": "2024-01-15T10:30:00Z"
}
```

---

#### Get Session

```http
GET /api/chat/sessions/{id}
```

Retrieves session details including conversation history.

**Response:**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "project_path": "/path/to/project",
  "provider": "claude",
  "model": "sonnet",
  "title": "Implement user authentication",
  "status": "active",
  "message_count": 12,
  "total_tokens": 15420,
  "total_cost_usd": 0.0234,
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:45:00Z"
}
```

---

#### Fork Session

```http
POST /api/chat/sessions/{id}/fork
```

Creates a fork of an existing session for experimentation.

**Request Body:**

```json
{
  "fork_point_message_id": "msg_123",
  "title": "Alternative approach",
  "provider": "openai",
  "model": "gpt-4"
}
```

**Response:**

```json
{
  "forked_session_id": "660e8400-e29b-41d4-a716-446655440001",
  "parent_session_id": "550e8400-e29b-41d4-a716-446655440000",
  "fork_point_message_id": "msg_123",
  "fork_depth": 1,
  "title": "Alternative approach"
}
```

---

### Usage & Cost API

Base path: `/api/usage`

#### Get Usage Summary

```http
GET /api/usage/summary
```

Returns overall platform usage statistics.

**Query Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `since` | string (RFC3339) | Filter: events after this time |
| `until` | string (RFC3339) | Filter: events before this time |
| `session_id` | string | Filter: specific session |

**Response:**

```json
{
  "total_tokens_input": 125000,
  "total_tokens_output": 45000,
  "total_tokens": 170000,
  "total_cost_usd": 2.45,
  "total_requests": 150,
  "successful_requests": 148,
  "failed_requests": 2,
  "average_duration_ms": 3500.5,
  "by_provider": {
    "claude": {
      "provider": "claude",
      "total_tokens_input": 100000,
      "total_tokens_output": 35000,
      "total_cost_usd": 1.95,
      "total_requests": 120
    },
    "openai": {
      "provider": "openai",
      "total_tokens_input": 25000,
      "total_tokens_output": 10000,
      "total_cost_usd": 0.50,
      "total_requests": 30
    }
  },
  "period": {
    "since": "2024-01-01T00:00:00Z",
    "until": "2024-01-15T23:59:59Z"
  }
}
```

---

#### Get Provider Usage

```http
GET /api/usage/providers
```

Returns usage breakdown by provider.

**Response:**

```json
{
  "providers": [
    {
      "provider": "claude",
      "model": "sonnet",
      "session_count": 45,
      "message_count": 890,
      "total_tokens_input": 100000,
      "total_tokens_output": 35000,
      "total_cost_usd": 1.95
    }
  ],
  "total": {
    "session_count": 50,
    "message_count": 1000,
    "total_tokens_input": 125000,
    "total_tokens_output": 45000,
    "total_tokens": 170000,
    "total_cost_usd": 2.45
  }
}
```

---

#### Get Session Usage

```http
GET /api/usage/sessions/{sessionID}
```

Returns usage statistics for a specific session.

**Response:**

```json
{
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "message_count": 24,
  "tool_call_count": 15,
  "total_tokens_input": 8500,
  "total_tokens_output": 3200,
  "total_tokens": 11700,
  "total_cost_usd": 0.0234,
  "summary_count": 1,
  "tokens_saved": 4000
}
```

---

#### Get Tool Call Statistics

```http
GET /api/usage/tools
```

Returns aggregated tool call statistics.

**Response:**

```json
{
  "total_requested": 500,
  "total_approved": 480,
  "total_rejected": 15,
  "total_auto_approved": 350,
  "total_completed": 475,
  "total_failed": 5,
  "by_tool": {
    "read_file": 200,
    "write_file": 150,
    "run_shell_command": 100,
    "axon_signal": 50
  }
}
```

---

#### Get Audit Logs

```http
GET /api/usage/audit-logs
```

Returns paginated audit log entries.

**Query Parameters:**

| Name | Type | Default | Description |
|------|------|---------|-------------|
| `limit` | integer | 100 | Max entries to return (max: 1000) |
| `offset` | integer | 0 | Entries to skip |
| `since` | string | - | RFC3339 timestamp filter |
| `until` | string | - | RFC3339 timestamp filter |
| `event_type` | string[] | - | Filter by event types |

**Response:**

```json
{
  "entries": [
    {
      "id": "audit_123",
      "timestamp": "2024-01-15T10:30:00Z",
      "event_type": "llm_request",
      "severity": "info",
      "session_id": "550e8400...",
      "provider": "claude",
      "model": "sonnet",
      "tokens_input": 1500,
      "tokens_output": 500,
      "cost_usd": 0.003,
      "duration_ms": 2500,
      "success": true
    }
  ],
  "total_count": 1500,
  "has_more": true,
  "limit": 100,
  "offset": 0
}
```

---

#### Get Cost by Model

```http
GET /api/usage/cost-by-model
```

Returns cost breakdown by model.

**Response:**

```json
{
  "models": [
    {
      "provider": "claude",
      "model": "sonnet",
      "session_count": 30,
      "message_count": 600,
      "total_tokens_input": 80000,
      "total_tokens_output": 28000,
      "total_tokens": 108000,
      "total_cost_usd": 1.55,
      "percentage_of_total": 63.27
    },
    {
      "provider": "claude",
      "model": "opus",
      "session_count": 5,
      "message_count": 50,
      "total_tokens_input": 10000,
      "total_tokens_output": 4000,
      "total_tokens": 14000,
      "total_cost_usd": 0.90,
      "percentage_of_total": 36.73
    }
  ],
  "total_cost_usd": 2.45
}
```

---

## WebSocket Protocol

### Connection

Connect to the WebSocket endpoint to receive real-time events and send commands.

```javascript
const ws = new WebSocket('ws://localhost:8080/ws?session_id=550e8400...');
```

**Query Parameters:**

| Name | Required | Description |
|------|----------|-------------|
| `session_id` | Yes | Session ID to connect to |

---

### Client Messages

Messages sent from client to server.

#### send_message

Send a user message to the conversation.

```json
{
  "type": "send_message",
  "content": "Please implement the authentication module",
  "session_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

---

#### approve_tool

Approve a pending tool call.

```json
{
  "type": "approve_tool",
  "tool_call_id": "tc_123",
  "session_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

---

#### reject_tool

Reject a pending tool call.

```json
{
  "type": "reject_tool",
  "tool_call_id": "tc_123",
  "reason": "This would delete important files",
  "session_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

---

#### pause

Pause the agent loop.

```json
{
  "type": "pause",
  "session_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

---

#### resume

Resume a paused loop.

```json
{
  "type": "resume",
  "session_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

---

#### stop

Stop the agent loop immediately.

```json
{
  "type": "stop",
  "session_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

---

### Server Messages

Messages sent from server to client.

#### message

A complete message (user or assistant).

```json
{
  "type": "message",
  "role": "assistant",
  "content": "I'll implement the authentication module...",
  "tokens": {
    "input": 150,
    "output": 500
  },
  "cost": 0.001
}
```

---

#### streaming_chunk

A streaming chunk during response generation.

```json
{
  "type": "streaming_chunk",
  "delta": "I'll start by",
  "iteration": 1,
  "accumulated_text": "I'll start by"
}
```

---

#### tool_call_update

Update on a tool call status.

```json
{
  "type": "tool_call_update",
  "tool_call_id": "tc_123",
  "status": "approved",
  "tool_name": "write_file",
  "input": {"path": "/src/auth.go", "content": "..."},
  "output": "Successfully wrote 256 bytes",
  "started_at": "2024-01-15T10:30:00Z",
  "completed_at": "2024-01-15T10:30:01Z"
}
```

**Tool Call Status Values:**

| Status | Description |
|--------|-------------|
| `pending` | Awaiting approval |
| `approved` | Approved, not yet started |
| `rejected` | Rejected by user/policy |
| `running` | Currently executing |
| `completed` | Completed successfully |
| `failed` | Execution failed |
| `timeout` | Execution timed out |
| `cancelled` | Cancelled by user/system |
| `auto_approved` | Auto-approved by policy |

---

#### event

A loop event notification.

```json
{
  "type": "event",
  "event_type": "iteration.start",
  "iteration": 1,
  "message": "iteration 1",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

#### signal

An orchestration signal from the agent.

```json
{
  "type": "signal",
  "signal_type": "phase-complete",
  "target": null,
  "reason": "Implemented authentication module",
  "metadata": {
    "files_changed": ["src/auth.go", "src/auth_test.go"],
    "tests_passed": true
  }
}
```

---

#### loop_state

Current loop state update.

```json
{
  "type": "loop_state",
  "iteration": 5,
  "status": "running",
  "total_tokens": 15420,
  "total_cost_usd": 0.0234,
  "iterations_since_progress": 0
}
```

---

#### error

An error notification.

```json
{
  "type": "error",
  "message": "Rate limit exceeded",
  "code": "rate_limit",
  "recoverable": true
}
```

---

## MCP Tools

MCP (Model Context Protocol) tools are exposed via JSON-RPC 2.0 over stdio. The protocol version is `2024-11-05`.

### Protocol Format

**Request:**

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "tool_name",
    "arguments": { ... }
  }
}
```

**Response:**

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Tool output here"
      }
    ]
  }
}
```

**Error Response:**

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Error message"
      }
    ],
    "isError": true
  }
}
```

---

### axon_signal

Send a structured signal to the Axon orchestrator.

**Arguments:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `type` | string | Yes | Signal type (see [Signal Types](#signal-types)) |
| `reason` | string | No | Human-readable reason |
| `target` | string | No | Target agent for `route` signals |
| `metadata` | object | No | Additional structured data |

**Example:**

```json
{
  "type": "phase-complete",
  "reason": "Implemented user authentication module",
  "metadata": {
    "files_changed": ["src/auth.go", "src/auth_test.go"],
    "tests_passed": true
  }
}
```

**Signal Types:**

| Type | Purpose | Effect |
|------|---------|--------|
| `phase-complete` | Task finished | Triggers quality gates, may complete loop |
| `blocked` | Waiting for input | Pauses loop, notifies user |
| `progress` | Incremental work done | Resets thrash detection counter |
| `decision-point` | Needs human decision | Pauses for user input |
| `planning-mismatch` | Assumptions violated | May trigger replanning |
| `scope-discovery` | Found new requirements | Logs for review |
| `route` | Hand off to agent | Routes to `target` |

---

### read_file

Read the contents of a file.

**Arguments:**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `path` | string | Yes | - | File path (absolute or relative) |
| `encoding` | string | No | `utf-8` | Encoding: `utf-8`, `utf-16`, `ascii`, `binary` |
| `offset` | integer | No | 0 | Byte offset to start reading |
| `length` | integer | No | - | Max bytes to read (reads entire file if omitted) |

**Example:**

```json
{
  "path": "/src/auth.go",
  "encoding": "utf-8"
}
```

**Response:**

```json
{
  "content": [
    {
      "type": "text",
      "text": "package auth\n\nimport (...)"
    }
  ]
}
```

---

### write_file

Write content to a file.

**Arguments:**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `path` | string | Yes | - | File path |
| `content` | string | Yes | - | Content to write (base64 for binary) |
| `encoding` | string | No | `utf-8` | Encoding: `utf-8`, `utf-16`, `ascii`, `binary` |
| `mode` | string | No | `overwrite` | Write mode: `overwrite`, `append` |
| `create_directories` | boolean | No | `false` | Create parent directories if missing |

**Example:**

```json
{
  "path": "/src/auth.go",
  "content": "package auth\n\n// ... implementation",
  "mode": "overwrite"
}
```

**Response:**

```json
{
  "content": [
    {
      "type": "text",
      "text": "Successfully wrote 256 bytes to /src/auth.go"
    }
  ]
}
```

---

### run_shell_command

Execute a shell command.

**Arguments:**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `command` | string | Yes | - | Shell command to execute |
| `args` | string[] | No | - | Arguments (safer, bypasses shell) |
| `working_directory` | string | No | cwd | Working directory |
| `timeout_ms` | integer | No | 30000 | Timeout in milliseconds (100-600000) |
| `env` | object | No | - | Additional environment variables |
| `stdin` | string | No | - | Input to pass to stdin |

**Example:**

```json
{
  "command": "go test -v ./...",
  "working_directory": "/path/to/project",
  "timeout_ms": 60000
}
```

**Response:**

```json
{
  "content": [
    {
      "type": "text",
      "text": "=== RUN TestAuth\n--- PASS: TestAuth (0.01s)\nPASS"
    }
  ],
  "stdout": "=== RUN TestAuth\n--- PASS: TestAuth (0.01s)\nPASS",
  "stderr": "",
  "exit_code": 0
}
```

---

### git_apply_patch

Apply a unified diff/patch to files.

**Arguments:**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `patch` | string | Yes | - | Patch in unified diff format |
| `working_directory` | string | No | cwd | Git repository root |
| `check_only` | boolean | No | `false` | Only verify, don't apply |
| `reverse` | boolean | No | `false` | Unapply the patch |
| `three_way` | boolean | No | `false` | Use 3-way merge for conflicts |
| `ignore_whitespace` | boolean | No | `false` | Ignore whitespace differences |
| `context_lines` | integer | No | 3 | Context lines (0-10) |

**Example:**

```json
{
  "patch": "--- a/src/auth.go\n+++ b/src/auth.go\n@@ -1,3 +1,4 @@\n package auth\n \n+// Package auth provides authentication.\n import (",
  "working_directory": "/path/to/project"
}
```

**Response:**

```json
{
  "content": [
    {
      "type": "text",
      "text": "Patch applied successfully (1 file(s), +1/-0 lines)"
    }
  ],
  "stats": {
    "files_changed": 1,
    "additions": 1,
    "deletions": 0,
    "hunks": 1
  },
  "affected_files": ["src/auth.go"]
}
```

---

### Session Fork Tools

These tools are available when a fork manager is configured.

#### fork_session

Create a fork of an existing session.

**Arguments:**

| Name | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `parent_session_id` | string | Yes | - | Session to fork from |
| `fork_point_message_id` | string | Yes | - | Message ID to fork at |
| `title` | string | No | auto | Title for forked session |
| `provider` | string | No | inherit | LLM provider override |
| `model` | string | No | inherit | Model override |
| `copy_messages` | boolean | No | `false` | Copy messages vs reference |
| `copy_tool_calls` | boolean | No | `false` | Also copy tool calls |
| `copy_summaries` | boolean | No | `false` | Also copy summaries |

**Response:**

```json
{
  "forked_session_id": "660e8400-e29b-41d4-a716-446655440001",
  "parent_session_id": "550e8400-e29b-41d4-a716-446655440000",
  "fork_point_message_id": "msg_123",
  "title": "Fork of Session 1",
  "provider": "claude",
  "model": "sonnet",
  "messages_copied": 10,
  "fork_depth": 1,
  "ancestor_chain": ["550e8400-e29b-41d4-a716-446655440000"]
}
```

---

#### get_fork_info

Get fork relationship information for a session.

**Arguments:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `session_id` | string | Yes | Session ID to query |

**Response:**

```json
{
  "session_id": "660e8400...",
  "parent_session_id": "550e8400...",
  "root_session_id": "550e8400...",
  "fork_point_message_id": "msg_123",
  "fork_depth": 1,
  "child_count": 2,
  "total_descendants": 5,
  "ancestor_chain": ["550e8400..."]
}
```

---

#### list_session_forks

List all direct forks of a session.

**Arguments:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `session_id` | string | Yes | Parent session ID |

**Response:**

```json
{
  "parent_session_id": "550e8400...",
  "fork_count": 2,
  "forks": [
    {
      "id": "660e8400...",
      "title": "Alternative approach",
      "provider": "claude",
      "model": "opus",
      "status": "active",
      "created_at": "2024-01-15T10:30:00Z",
      "fork_point": "msg_123"
    }
  ]
}
```

---

## Event Types

Events are emitted during agent loop execution. Each event has a type, kind (category), and associated data.

### Event Categories

| Kind | Description |
|------|-------------|
| `lifecycle` | Loop lifecycle events |
| `iteration` | Iteration-level events |
| `llm` | LLM interaction events |
| `tool` | Tool execution events |
| `context` | Context management events |
| `message` | Message events |
| `gate` | Quality gate events |
| `signal` | Orchestration signals |
| `cost` | Cost tracking events |
| `session` | Session events |
| `thrashing` | Thrashing detection events |

### All Event Types

#### Lifecycle Events

| Type | Description |
|------|-------------|
| `loop.start` | Loop execution begins |
| `loop.pause` | Loop paused |
| `loop.resume` | Loop resumed |
| `loop.stop` | Loop stopped by user |
| `loop.complete` | Loop completed successfully |
| `loop.error` | Loop terminated with error |
| `loop.timeout` | Loop timed out |
| `loop.max_reached` | Max iterations reached |

#### Iteration Events

| Type | Description |
|------|-------------|
| `iteration.start` | Iteration begins |
| `iteration.complete` | Iteration completes |
| `iteration.retry` | Iteration will be retried |
| `iteration.skip` | Iteration skipped |

#### LLM Events

| Type | Description |
|------|-------------|
| `llm.request_start` | LLM request initiated |
| `llm.request_end` | LLM request completed |
| `llm.stream_start` | Streaming begins |
| `llm.stream_chunk` | Streaming chunk received |
| `llm.stream_end` | Streaming ends |
| `llm.error` | LLM request error |
| `llm.rate_limit` | Rate limit hit |
| `llm.context_window` | Context window exceeded |

#### Tool Events

| Type | Description |
|------|-------------|
| `tool.call_requested` | Tool call requested by LLM |
| `tool.call_queued` | Tool call queued for approval |
| `tool.call_approved` | Tool call approved |
| `tool.call_rejected` | Tool call rejected |
| `tool.call_start` | Tool execution begins |
| `tool.call_progress` | Tool execution progress |
| `tool.call_complete` | Tool execution completes |
| `tool.call_error` | Tool execution error |
| `tool.call_timeout` | Tool execution timed out |
| `tool.call_cancelled` | Tool execution cancelled |
| `tool.result_sent` | Tool result sent to LLM |
| `tool.auto_approved` | Tool auto-approved by policy |

#### Context Events

| Type | Description |
|------|-------------|
| `context.injected` | Auto-context injected |
| `context.truncated` | Context was truncated |
| `context.summarized` | Context was summarized |
| `context.refreshed` | Context was refreshed |

#### Message Events

| Type | Description |
|------|-------------|
| `message.user` | User message received |
| `message.assistant` | Assistant response |
| `message.system` | System message injected |

#### Quality Gate Events

| Type | Description |
|------|-------------|
| `gate.check_start` | Quality gate check begins |
| `gate.check_pass` | Quality gate passed |
| `gate.check_fail` | Quality gate failed |
| `gate.fix_start` | Auto-fix attempt begins |
| `gate.fix_success` | Auto-fix succeeded |
| `gate.fix_fail` | Auto-fix failed |

#### Signal Events

| Type | Description |
|------|-------------|
| `signal.received` | Signal received from agent |
| `signal.sent` | Signal sent to orchestrator |
| `signal.phase_complete` | Phase completion signal |

#### Cost Events

| Type | Description |
|------|-------------|
| `cost.updated` | Cost tracking updated |
| `cost.budget_warn` | Budget warning (80% threshold) |
| `cost.budget_exceeded` | Budget exceeded |

#### Session Events

| Type | Description |
|------|-------------|
| `session.created` | New session created |
| `session.restored` | Session restored from storage |
| `session.persisted` | Session state persisted |
| `session.forked` | Session forked |

#### Thrashing Events

| Type | Description |
|------|-------------|
| `thrashing.detected` | Loop thrashing detected |
| `thrashing.resolved` | Thrashing resolved |

---

### Event Structure

```json
{
  "id": "evt_123",
  "type": "tool.call_complete",
  "kind": "tool",
  "timestamp": "2024-01-15T10:30:00Z",
  "session_id": "550e8400...",
  "iteration": 5,
  "message": "Tool execution completed",
  "tool_call": {
    "id": "tc_123",
    "name": "write_file",
    "status": "completed",
    "duration_ms": 150
  }
}
```

---

## Signal Protocol

The Axon Signal protocol enables agents to communicate structured events to the orchestrator.

### Signal Structure

```json
{
  "type": "phase-complete",
  "reason": "Human-readable explanation",
  "target": "agent_name",
  "metadata": {
    "key": "value"
  }
}
```

### Signal Types Reference

| Signal | Purpose | Required Fields | Optional Fields |
|--------|---------|-----------------|-----------------|
| `phase-complete` | Task finished | `type` | `reason`, `metadata` |
| `blocked` | Waiting for input | `type` | `reason` |
| `progress` | Work completed | `type` | `reason`, `metadata` |
| `decision-point` | Needs decision | `type` | `reason`, `metadata` |
| `planning-mismatch` | Assumptions wrong | `type` | `reason` |
| `scope-discovery` | New requirements | `type` | `reason`, `metadata` |
| `route` | Hand off | `type`, `target` | `reason`, `metadata` |

### Signal Effects

| Signal | Loop Effect | Quality Gates |
|--------|------------|---------------|
| `phase-complete` | May complete | Triggered |
| `blocked` | Pauses | No |
| `progress` | Resets thrash counter | No |
| `decision-point` | Pauses | No |
| `planning-mismatch` | Continues | No |
| `scope-discovery` | Continues | No |
| `route` | May complete | No |

---

## Data Types

### Session

```typescript
interface Session {
  id: string;                    // UUID
  project_path: string;          // Workspace path
  provider: string;              // claude, openai, gemini
  model: string;                 // Model identifier
  title: string;                 // Session title
  status: SessionStatus;         // active, paused, archived, deleted
  parent_session_id?: string;    // For forks
  fork_point_message_id?: string;
  created_at: string;            // RFC3339 timestamp
  updated_at: string;
}

type SessionStatus = 'active' | 'paused' | 'archived' | 'deleted';
```

### Message

```typescript
interface Message {
  id: string;
  session_id: string;
  role: 'user' | 'assistant' | 'system';
  content: ContentBlock[];
  tokens_input?: number;
  tokens_output?: number;
  cost_usd?: number;
  created_at: string;
}

interface ContentBlock {
  type: 'text' | 'tool_use' | 'tool_result';
  text?: string;
  tool_use_id?: string;
  tool_name?: string;
  tool_input?: any;
  tool_result?: string;
  is_error?: boolean;
}
```

### ToolCall

```typescript
interface ToolCall {
  id: string;
  session_id: string;
  message_id: string;
  tool_name: string;
  tool_input: any;
  tool_output?: string;
  status: ToolCallStatus;
  approval_status: ApprovalStatus;
  approved_by?: string;
  approved_at?: string;
  rejected_by?: string;
  rejection_reason?: string;
  started_at?: string;
  completed_at?: string;
  duration_ms?: number;
  error?: string;
}

type ToolCallStatus = 'pending' | 'approved' | 'rejected' | 'running' |
                       'completed' | 'failed' | 'timeout' | 'cancelled' |
                       'auto_approved';

type ApprovalStatus = 'pending' | 'approved' | 'rejected' | 'auto_approved';
```

### AgentLoopState

```typescript
interface AgentLoopState {
  iteration: number;
  total_tokens: number;
  total_cost_usd: number;
  messages: Message[];
  last_signal?: Signal;
  iterations_since_progress: number;
  started_at: string;
  last_iteration_at: string;
}
```

### Usage

```typescript
interface Usage {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  cache_read_tokens?: number;
  cache_write_tokens?: number;
}
```

---

## Error Codes

### HTTP Status Codes

| Code | Meaning |
|------|---------|
| 200 | Success |
| 400 | Bad Request - Invalid parameters |
| 404 | Not Found - Resource doesn't exist |
| 409 | Conflict - Resource state conflict |
| 429 | Rate Limited |
| 500 | Internal Server Error |

### JSON-RPC Error Codes

| Code | Meaning |
|------|---------|
| -32700 | Parse error |
| -32600 | Invalid request |
| -32601 | Method not found |
| -32602 | Invalid params |
| -32603 | Internal error |

### Agent Loop Errors

| Error | Description |
|-------|-------------|
| `ErrLoopStopped` | Agent loop stopped by user |
| `ErrLoopPaused` | Agent loop paused |
| `ErrMaxIterations` | Max iterations reached |
| `ErrMaxTokens` | Max tokens exceeded |
| `ErrBudgetExceeded` | Budget limit exceeded |
| `ErrContextOverflow` | Context window overflow |
| `ErrProviderError` | Provider returned error |
| `ErrToolExecutionFail` | Tool execution failed |
| `ErrNoProvider` | No provider available |

### Provider Error Codes

| Code | Retryable | Description |
|------|-----------|-------------|
| `rate_limit` | Yes | Rate limit exceeded |
| `server_error` | Yes | Provider server error |
| `timeout` | Yes | Request timeout |
| `invalid_request` | No | Invalid request format |
| `authentication` | No | Authentication failed |
| `insufficient_quota` | No | Quota exceeded |

---

## Retry Configuration

The agent loop supports automatic retries for transient failures.

### Default Configuration

```go
RetryConfig{
    MaxRetries: 3,
    Backoff: []time.Duration{
        1 * time.Second,
        5 * time.Second,
        15 * time.Second,
    },
    RetryableErrors: []string{
        "rate_limit",
        "server_error",
        "timeout",
    },
}
```

### Configuration via YAML

```yaml
retry:
  max_attempts: 3
  base_delay: 1.0
  multiplier: 2.0
  max_delay: 60.0
```

---

## Rate Limits

Rate limits vary by provider:

| Provider | Requests/min | Tokens/min |
|----------|--------------|------------|
| Claude | 60 | 100,000 |
| OpenAI | 60 | 90,000 |
| Gemini | 60 | 120,000 |

The agent loop automatically handles rate limit errors with exponential backoff.

---

## See Also

- [Conversational Orchestration Guide](CONVERSATIONAL_ORCHESTRATION.md) - Usage guide and architecture
- [Configuration Guide](CONFIGURATION.md) - Full configuration reference
