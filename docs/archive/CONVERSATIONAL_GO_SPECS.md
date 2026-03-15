# OpenExec Conversational Console Specification (Go-Native)

## Overview
The Conversational Console is a stateful, multi-model chat interface integrated directly into the `openexec` binary. it allows users to manage multiple projects, chat with AI agents (Claude, Codex, Gemini), and execute verified operations via MCP tools.

## Core Pillars

### 1. Multi-Project Management
- **WORKSPACES_ROOT**: The root directory containing multiple project folders (e.g., `magpie/`, `axon/`).
- **Isolation**: Each session is bound to a `project_path`. Context injection and tool execution are strictly scoped to this path.

### 2. Provider Facade (`internal/agent`)
A unified Go interface for all LLM providers:
```go
type Provider interface {
    GenerateStream(ctx context.Context, req Request) (<-chan ResponseChunk, error)
    GetName() string
    GetModels() []string
}
```
Implemented for:
- **Anthropic**: Claude 3.5 Sonnet/Opus
- **OpenAI**: GPT-5 / Codex
- **Google**: Gemini 1.5 Pro/Flash

### 3. Auto-Context Injection
Before every turn, the server gathers:
- `INTENT.md`
- `tasks.json` (Summary of status)
- Last 50 lines of `.openexec/logs/execution.log`
- Current directory tree (respecting `.gitignore`)

### 4. MCP Tool Bridge
Standardized tools routed through the Go backend:
- `read_file`, `write_file`, `list_directory`
- `run_shell_command`
- `axon_signal` (querying the execution engine)
- **Approval Gate**: UI-blocking modal for all mutation tools.

## Data Schema (SQLite - `conversations.db`)
- `sessions`: id, project_path, provider, model, title, created_at
- `messages`: session_id, role, content, tool_calls, tokens, created_at

## Implementation Phases

### Phase 1: Engine Foundation
- `internal/agent`: Interface and initial Anthropic/Gemini drivers.
- `internal/db`: SQLite schema for session persistence.
- `internal/server`: API routes for session management and streaming chat.

### Phase 2: Tooling & UI
- `internal/mcp`: Tool implementation and approval state machine.
- `ui/`: React/Next.js dashboard (embedded into Go binary).
- `cmd/openexec console`: The entrypoint command.

### Phase 3: Advanced Features
- Session forking across models.
- "Meta-mode": Ability for the console to edit and recompile `openexec` source code.
