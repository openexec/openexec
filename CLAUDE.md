# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

```bash
# Build
make build                    # Build binary → bin/openexec
go build -o openexec ./cmd/openexec  # Alternative direct build

# Test
make test                     # All tests (Go + UI)
go test ./...                 # Go tests only
go test -v ./internal/loop/... -run TestLoop  # Single test/package
cd ui && npm test             # UI tests (watch mode)
cd ui && npx vitest run --fileParallelism=false  # UI tests (CI mode)

# Lint & Type Check
make lint                     # Go vet + golangci-lint + UI ESLint
make type-check               # Go build check + UI tsc
cd ui && npm run lint         # UI only

# UI Development
cd ui && npm install && npm run dev -- --port 3001  # Dev server with HMR
```

## Architecture Overview

OpenExec is a **single-binary AI orchestration framework** that transforms high-level intent into production code through a deterministic pipeline.

### Core Execution Flow
```
CLI Command → Manager → Pipeline → Loop → AI Provider (Claude/OpenAI/Gemini)
                ↓
         SQLite State Store
```

### Pipeline Phases (5-phase state machine)
Each task progresses through: **TD → IM → RV → RF → FL**
- **TD** (clario): Technical Design - research and strategy
- **IM** (spark): Implementation - code changes
- **RV** (blade): Review - quality assurance with routing back to IM if needed
- **RF** (hon): Refinement - post-review optimization
- **FL** (clario): Finalize - verification and state sync

### Key Packages

| Package | Purpose |
|---------|---------|
| `cmd/openexec/` | Entry point, calls `cli.Execute()` |
| `internal/cli/` | Cobra commands (init, plan, start, run, chat, doctor) |
| `internal/loop/` | Core iteration engine - spawns AI, parses events |
| `internal/pipeline/` | Phase orchestration and state machine |
| `internal/mcp/` | Model Context Protocol server (JSON-RPC stdio) |
| `internal/prompt/` | Prompt assembly from personas/workflows/manifests |
| `internal/release/` | SQLite-backed task/story state management |
| `pkg/agent/` | AI provider adapters (anthropic, openai, gemini) |
| `pkg/manager/` | Multi-pipeline orchestrator |
| `pkg/api/` | HTTP handlers and WebSocket |
| `ui/` | React 18 + TypeScript + Vite (embedded in binary) |

### Agent Definitions
Agent personas, workflows, and manifests live in `agents/`:
- `agents/personas/` - Role definitions (YAML)
- `agents/workflows/` - Prompt templates
- `agents/manifests/` - Agent metadata linking persona to workflow

### State & Persistence
- **SQLite**: Canonical state store at `.openexec/data/audit.db`
- **Tract**: Separate JSON-RPC microservice for story/task storage
- **Config**: `.openexec/config.json` for project settings

---

## Engineering Mandates

### Observe, then Resolve
To prevent thrashing during task execution:

**Error Diagnostics**
- If a test fails, rerun with `--verbose` or `screen.debug()` before attempting fixes
- State a clear hypothesis before modifying code
- If a change doesn't fix the error after one attempt, **REVERT** before trying a different strategy

**UI Testing (React/Vitest)**
- Use `findBy*` for elements appearing after async actions
- Use `userEvent` over `fireEvent` for proper event bubbling
- Wait for state transitions with `waitFor()`, never `setTimeout`
- Verify API schemas in `internal/api/` or `types/` before implementing UI
- Ensure mocks match current API response format (snake_case vs camelCase)

### Known Quirks
- **JSDOM limitations**: Doesn't fully simulate layout events (onMouseEnter). Check if failing tests depend on layout properties
- **Audit DB**: Source of truth for task progress is `.openexec/data/audit.db`

### Learning Loop
When solving complex bugs, persist lessons to `.openexec/engram/learning_log.json`
