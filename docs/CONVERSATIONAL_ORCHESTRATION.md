# Conversational Orchestration Guide

OpenExec's conversational orchestration transforms project management from command-line batch operations into an interactive, multi-turn dialogue with AI agents. This guide covers the architecture, usage, and configuration of the conversational mode.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Getting Started](#getting-started)
- [The Agent Loop](#the-agent-loop)
- [Context Injection](#context-injection)
- [Tool Execution](#tool-execution)
- [Signal Protocol](#signal-protocol)
- [Session Management](#session-management)
- [The Guided Intent Wizard](#the-guided-intent-wizard)
- [Web UI Reference](#web-ui-reference)
- [Configuration](#configuration)
- [Troubleshooting](#troubleshooting)

---

## Overview

Conversational orchestration enables engineers to:

- **Chat with AI agents** (Claude, OpenAI, Gemini) to manage software projects
- **Execute tools** (read/write files, run commands, apply patches) with explicit approvals
- **Monitor progress** in real-time with cost and token tracking
- **Maintain persistent sessions** that can be resumed or forked
- **Bootstrap projects** through guided interviews that generate structured intents

The system follows three core principles:

1. **Provider Agnosticism**: All LLM providers (Anthropic, OpenAI, Google) are accessed through a unified interface
2. **Tool-First Execution**: All agent capabilities route through MCP tools with approval gates
3. **Signal-Driven Communication**: Agents communicate state changes via the `axon_signal` protocol

---

## Architecture

### Three-Layer Model

```
┌─────────────────────────────────────────────────────────┐
│                    User Interface                        │
│  (Web UI / CLI / TUI)                                   │
├─────────────────────────────────────────────────────────┤
│                    Agent Loop                            │
│  - Conversation turn lifecycle                          │
│  - Context injection & summarization                    │
│  - Token/cost tracking                                  │
│  - Completion detection                                 │
├─────────────────────────────────────────────────────────┤
│                    Execution Layer                       │
│  - Provider adapters (Claude, OpenAI, Gemini)          │
│  - Tool executor with approval gates                    │
│  - Session persistence (SQLite)                         │
└─────────────────────────────────────────────────────────┘
```

### Key Components

| Component | Location | Purpose |
|-----------|----------|---------|
| Agent Loop | `internal/loop/agent_loop.go` | Orchestrates conversation turns and tool execution |
| Provider Interface | `internal/agent/provider.go` | Unified interface for all LLM providers |
| Executor | `internal/loop/executor.go` | Handles tool execution with approvals |
| Context Builder | `internal/context/builder.go` | Auto-gathers project context for prompts |
| Session Storage | `internal/db/session/` | Persists conversations in SQLite |
| MCP Server | `internal/mcp/server.go` | Exposes tools via JSON-RPC |

---

## Getting Started

### Prerequisites

1. OpenExec CLI installed and in PATH
2. At least one LLM provider configured (API key for Claude, OpenAI, or Gemini)
3. A project initialized with `openexec init`

### Quick Start

```bash
# Initialize a new project
openexec init

# Start the guided intent wizard (conversational project setup)
openexec wizard

# Or start the execution daemon with conversational UI
openexec start --daemon

# Launch the terminal UI dashboard
openexec tui

# Or access the web UI (if enabled)
# Default: http://localhost:3000/chat
```

### First Conversation

1. Create a new session (via UI or wizard)
2. Select your provider (Claude, OpenAI, Gemini) and model
3. Type your request in natural language
4. Review and approve any tool calls
5. The agent iterates until signaling `phase-complete`

---

## The Agent Loop

The agent loop (`internal/loop/agent_loop.go`) is the core orchestrator that manages iterative LLM interactions.

### Iteration Lifecycle

```
┌──────────────────────────────────────────────────────────┐
│ 1. Build Context                                         │
│    - Gather project context (INTENT.md, tasks, logs)    │
│    - Apply token budget constraints                      │
│    - Inject as system/user preamble                     │
├──────────────────────────────────────────────────────────┤
│ 2. Build Request                                         │
│    - Select model from session config                   │
│    - Include conversation history                        │
│    - Register available MCP tools                        │
├──────────────────────────────────────────────────────────┤
│ 3. LLM Request (with retry logic)                        │
│    - Stream response from provider                       │
│    - Track token usage and cost                          │
│    - Handle transient errors with backoff               │
├──────────────────────────────────────────────────────────┤
│ 4. Process Response                                      │
│    - Parse tool calls from response                      │
│    - Execute tools with approval gates                   │
│    - Check for completion signals                        │
│    - Update state and token counts                       │
├──────────────────────────────────────────────────────────┤
│ 5. Check Completion                                      │
│    - phase-complete signal received?                    │
│    - Max iterations reached?                             │
│    - Budget exceeded?                                    │
│    - If not complete, loop to step 1                    │
└──────────────────────────────────────────────────────────┘
```

### Configuration Options

```yaml
# openexec.yaml - Agent loop settings
execution:
  timeout: 600                    # Task timeout in seconds
  auto_fix: true                  # Auto-fix when quality gates fail
  max_fix_iterations: 2           # Max fix attempts

agents:
  default: claude
  claude:
    model: sonnet                 # or opus
    timeout: 600
```

### Session State

The loop maintains state including:

- `Iteration`: Current iteration count
- `TotalTokens`: Cumulative token usage
- `TotalCostUSD`: Cumulative cost
- `Messages`: Full conversation history
- `LastSignal`: Most recent axon_signal

Sessions can be paused, resumed, or forked for experimentation.

---

## Context Injection

The context builder (`internal/context/`) automatically gathers and injects project-aware context into every LLM turn.

### What Gets Injected

| Source | Description | Default Budget |
|--------|-------------|----------------|
| `INTENT.md` | Project vision and requirements | 500 tokens |
| `tasks.json` | Current task state summary | 300 tokens |
| Git status | Modified files, branch info | 200 tokens |
| Recent logs | Last 50 lines of execution.log | 500 tokens |
| Directory tree | Project structure (respecting .gitignore) | 500 tokens |
| Package info | package.json, go.mod, requirements.txt | 200 tokens |

### Token Budgeting

Context injection respects a configurable token budget:

```yaml
context:
  enabled: true
  max_tokens: 4000               # Total context budget
  min_relevance_score: 0.1       # Minimum relevance for inclusion
```

When context exceeds the budget:
1. Items are prioritized by relevance and type
2. Lower-priority items are truncated or excluded
3. The system ensures critical context (task, intent) is always included

### Gatherers

Specialized gatherers collect different context types:

- `gatherer_git.go`: Git status, diff, log
- `gatherer_directory.go`: File tree structure
- `gatherer_instructions.go`: INTENT.md, CLAUDE.md
- `gatherer_environment.go`: Environment variables
- `gatherer_package.go`: Package manager files

---

## Tool Execution

All agent actions execute through MCP tools (`internal/loop/executor.go`) with approval workflows.

### Available Tools

| Tool | Description | Risk Level |
|------|-------------|------------|
| `read_file` | Read file contents | Low |
| `write_file` | Write/create files | High |
| `list_directory` | List directory contents | Low |
| `run_shell_command` | Execute shell commands | High |
| `git_apply_patch` | Apply git patches | High |
| `axon_signal` | Signal orchestrator | Low |

### Approval Workflow

1. Agent requests tool execution
2. Executor checks approval policy
3. If approval required:
   - UI displays approval request
   - User can approve or reject with reason
   - Timeout defaults to 5 minutes
4. On approval, tool executes in project workspace
5. Result (success or error) sent back to agent

### Path Validation

All file operations are validated against `WORKSPACES_ROOT`:
- Paths must be within the project workspace
- Symlink traversal outside workspace is blocked
- Sensitive paths (`.env`, credentials) trigger warnings

### Configuration

```yaml
safety:
  enabled: true
  file_locking: true             # Lock files during edits
  allow_parallel: false          # Single agent per project
```

---

## Signal Protocol

The Axon Signal protocol enables agents to communicate structured events to the orchestrator.

### Signal Types

| Signal | Purpose | Effect |
|--------|---------|--------|
| `phase-complete` | Task finished | Triggers quality gates, may complete loop |
| `blocked` | Waiting for human input | Pauses loop, notifies user |
| `progress` | Incremental work done | Resets thrash detection counter |
| `decision-point` | Needs human decision | Pauses for user input |
| `planning-mismatch` | Assumptions violated | May trigger replanning |
| `scope-discovery` | Found new requirements | Logs for review |
| `route` | Hand off to another agent | Routes to specified target |

### Signal Usage

Agents signal via the `axon_signal` tool:

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

### Quality Gates on Completion

When `phase-complete` is signaled:

1. Run configured quality gates (lint, typecheck, test)
2. If all pass: loop completes successfully
3. If any fail: generate fix prompt and continue loop
4. Retry up to `max_fix_iterations` times

```yaml
quality:
  gates:
    - lint
    - typecheck
    - test
```

---

## Session Management

Sessions are persisted in SQLite (`conversations.db`) for resumption and audit.

### Database Schema

**sessions**
- `id`: UUID primary key
- `project_path`: Workspace path
- `provider`, `model`: LLM configuration
- `title`, `status`: Session metadata
- `parent_session_id`: For forking
- `created_at`, `updated_at`: Timestamps

**messages**
- `session_id`: Foreign key to session
- `role`: user/assistant/system
- `content`: Message text
- `tokens_input`, `tokens_output`, `cost_usd`: Usage tracking

**tool_calls**
- `session_id`, `message_id`: References
- `tool_name`, `tool_input`, `tool_output`: Tool data
- `approval_status`, `approved_by`, `approved_at`: Approval tracking
- `started_at`, `completed_at`, `error`: Execution tracking

**session_summaries**
- `session_id`: Reference
- `summary_text`: Compressed context
- `messages_summarized`, `tokens_saved`: Metrics

### Session Operations

**Create**: New session with provider/model selection
```
POST /api/chat/sessions
{
  "project_path": "/path/to/project",
  "provider": "claude",
  "model": "sonnet"
}
```

**Resume**: Continue from where you left off
```
The loop restores: iteration, tokens, cost, messages, lastSignal
```

**Fork**: Branch session for experimentation
```
POST /api/chat/sessions/:id/fork
{
  "new_model": "opus"  // optionally change model
}
```

---

## The Guided Intent Wizard

The wizard (`openexec wizard`) provides a conversational interface for project bootstrapping.

### Purpose

Instead of manually writing INTENT.md, the wizard:
1. Conducts a multi-turn dialogue
2. Extracts structured facts from free-form input
3. Identifies assumptions that need confirmation
4. Validates against a schema
5. Generates a verified INTENT.md

### Flow

```
User Dump → Classification → Constraint Pinning → Validation → INTENT.md
```

### Classification Fork

The wizard first determines if this is:

- **Greenfield**: Building from scratch
  - Focus: Architecture selection, scaffolding, core features
- **Refactor**: Modifying existing code
  - Focus: Legacy mapping, API contracts, parity testing

### Example Session

```
$ openexec wizard

=== OpenExec Guided Intent Interviewer ===
   Project: my-project
   Model:   sonnet

Tell me about your project (free-form dump, or type 'exit' to quit):

> I want to build a desktop tool for tracking flight prices

[Thinking...]

I understand we are building a NEW PROJECT from scratch.

  ✔ Explicit:
    - Project type: Desktop application
    - Domain: Flight price tracking

  ⚠ Assumed:
    - Platform: macOS (unconfirmed)

? Which platforms should this support? (macOS, Windows, Linux):

> Just macOS for now

  ✔ Explicit:
    - Platform: macOS

? How will we fetch flight data? Is there a specific API?

> ...
```

### State Persistence

Wizard state is saved to `.openexec/wizard_state.json`:
- Resume interrupted sessions
- Track explicit vs. assumed facts
- Show completeness percentage

### Commands

| Command | Description |
|---------|-------------|
| `/status` | Show current completeness |
| `/assumptions` | List all assumptions |
| `/render` | Preview INTENT.md |
| `/done` | Force completion |
| `exit` | Save and quit |

---

## Web UI Reference

The React-based web UI provides a rich interface for conversational orchestration.

### Component Structure

```
ChatLayout (main grid)
├── SessionSidebar (left)
│   ├── SessionList
│   ├── NewSessionButton
│   └── SessionFilters
│
├── ChatMain (center)
│   ├── ChatHeader
│   │   ├── SessionTitle
│   │   ├── ModelIndicator
│   │   ├── LoopStatusBadge
│   │   └── ChatActions
│   │
│   ├── MessageList
│   │   ├── UserMessage
│   │   ├── AssistantMessage
│   │   │   ├── MessageContent
│   │   │   └── ToolCallList
│   │   └── StreamingMessage
│   │
│   ├── ChatInput
│   └── LoopProgressBar
│
├── EventPanel (right, collapsible)
│   └── EventList
│
└── CostPanel (right, collapsible)
    ├── CostSummary
    ├── TokenUsage
    └── BudgetProgress
```

### WebSocket Protocol

**Client → Server:**
| Message | Payload |
|---------|---------|
| `send_message` | `{ content, sessionId }` |
| `approve_tool` | `{ toolCallId, sessionId }` |
| `reject_tool` | `{ toolCallId, reason }` |
| `pause` | `{ sessionId }` |
| `resume` | `{ sessionId }` |
| `stop` | `{ sessionId }` |

**Server → Client:**
| Message | Payload |
|---------|---------|
| `message` | `{ role, content, tokens, cost }` |
| `streaming_chunk` | `{ delta, iteration }` |
| `tool_call_update` | `{ status, toolCallId }` |
| `event` | `{ type, iteration, message }` |
| `signal` | `{ signalType, target, metadata }` |
| `loop_state` | `{ iteration, cost, tokens, status }` |
| `error` | `{ message, code }` |

### State Management

The UI uses Zustand with these slices:
- **Connection**: WebSocket state
- **Sessions**: List and current session
- **Messages**: History and streaming
- **ToolCalls**: Pending approvals
- **Loop**: Iteration state
- **Events**: Real-time events
- **Cost**: Token/USD tracking

---

## Configuration

### Full Configuration Example

```yaml
# openexec.yaml

# Agent settings
agents:
  default: claude
  claude:
    model: sonnet
    timeout: 600
    skip_permissions: false
  openai:
    model: gpt-4
    timeout: 600
  gemini:
    model: 1.5-pro
    timeout: 600

# Execution settings
execution:
  timeout: 600
  auto_fix: true
  max_fix_iterations: 2

# Context injection
context:
  enabled: true
  max_tokens: 4000

# Quality gates
quality:
  gates:
    - lint
    - typecheck
    - test

# Safety rules
safety:
  enabled: true
  file_locking: true
  allow_parallel: false
```

### Environment Variables

```bash
# Provider API keys
export OPENEXEC_CLAUDE_API_KEY=sk-ant-...
export OPENEXEC_OPENAI_API_KEY=sk-...
export OPENEXEC_GEMINI_API_KEY=AIza...

# Agent selection
export OPENEXEC_DEFAULT_AGENT=claude
export OPENEXEC_AGENT_CLAUDE_MODEL=opus

# Execution overrides
export OPENEXEC_EXECUTION_TIMEOUT=1800
export OPENEXEC_CONTEXT_MAX_TOKENS=8000
```

---

## Troubleshooting

### Common Issues

**Session won't start**
- Check provider API key is set
- Verify project is initialized (`openexec init`)
- Check logs in `.openexec/logs/`

**Tools not executing**
- Ensure safety settings allow the operation
- Check file paths are within workspace
- Verify approval wasn't rejected

**Context too large**
- Reduce `context.max_tokens` setting
- Add files to `.gitignore` to exclude from tree
- Check for large log files

**Agent stuck in loop**
- Check for thrashing (no progress signals)
- Review quality gate failures
- Manually signal `phase-complete` if appropriate

**High costs**
- Set budget limits in config
- Use lighter models (sonnet vs opus)
- Enable context summarization

### Debug Commands

```bash
# Check session state
openexec status --verbose

# View execution logs
tail -f .openexec/logs/execution.log

# List active sessions
openexec session list

# Export session for debugging
openexec session export SESSION_ID --format json
```

### Getting Help

- **Documentation**: This file and `docs/CONFIGURATION.md`
- **Logs**: Check `.openexec/logs/` for detailed execution logs
- **GitHub Issues**: Report bugs and feature requests
