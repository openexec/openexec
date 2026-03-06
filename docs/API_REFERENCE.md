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
| FWU API | HTTP/JSON | `/api/fwu` | Task-based execution control |
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
