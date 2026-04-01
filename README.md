<p align="center">
  <img src="https://raw.githubusercontent.com/openexec/.github/main/profile/logo.svg" alt="OpenExec Logo" width="160"/>
</p>

<h1 align="center">OpenExec</h1>

<p align="center">
  <strong>AI orchestration framework: deterministic pipelines around AI CLI tools</strong>
</p>

<p align="center">
  <img src="https://img.shields.io/github/v/release/openexec/openexec?style=flat-square&color=orange" alt="Version"/>
  <img src="https://img.shields.io/github/actions/workflow/status/openexec/openexec/go.yml?style=flat-square" alt="Build Status"/>
  <img src="https://img.shields.io/badge/go-%2300ADD8.svg?style=flat-square&logo=go&logoColor=white" alt="Go"/>
  <img src="https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square" alt="License"/>
</p>

---

## What Is OpenExec?

OpenExec is a **single-binary orchestration layer** that wraps existing AI CLI tools (Claude Code, Codex, Gemini CLI) with deterministic infrastructure: structured pipelines, quality gates, checkpointing, and memory. It does not implement its own LLM clients -- it spawns subprocesses for the CLIs you already use.

## How It Works

```
openexec init          # Configure project (model, settings)
openexec run           # Execute tasks via blueprint pipeline

Execution Flow:
  CLI -> Manager -> Pipeline -> Blueprint Engine -> AI CLI (claude/codex/gemini)
                                    |
                      gather_context -> implement -> lint -> test -> review
```

### Three Execution Modes

| Mode | Description | Side Effects |
|------|-------------|--------------|
| **Chat** | Conversational, no side effects | None |
| **Task** | Scoped action, produces artifacts | Creates files/patches |
| **Run** | Blueprint execution over task | Full automation |

### Supported AI CLIs

| CLI | Provider | Installation |
|-----|----------|--------------|
| `claude` | Anthropic | `npm install -g @anthropic-ai/claude-code` |
| `codex` | OpenAI | `npm install -g @openai/codex` |
| `gemini` | Google | (Google's CLI tool) |

OpenExec resolves model names to CLI commands automatically. Claude models spawn `claude`, OpenAI models spawn `codex`, Gemini models spawn `gemini`.

---

## Features

**Core (always on):**
- **Blueprint Execution**: 5-stage pipeline (gather_context -> implement -> lint -> test -> review)
- **Multi-Model Support**: Claude, Codex, Gemini via their CLI tools
- **Deterministic Routing**: Keyword-based task classification (mode, toolset, repo zones, sensitivity)
- **Backward Compatibility**: Legacy `.uaos/` project format still supported

**Opt-in (via `.openexec/config.json`):**
- **BitNet Routing**: Local 1-bit LLM for enhanced intent classification, auto-downloads model
- **Quality Gates V2**: Auto-detects project type (Go/Python/TS/Rust), runs lint/test/format gates
- **Checkpointing**: Deterministic checkpoints after each stage for crash recovery
- **Memory System**: Extracts learning patterns from completed stages, injects context in future runs
- **Predictive Loading**: Pre-fetches likely-needed files based on task description
- **Caching**: Knowledge cache and tool result cache to avoid redundant work
- **Multi-Agent Parallel**: Split large tasks across parallel workers (when `worker_count > 1`)

**Infrastructure:**
- **MCP Server**: JSON-RPC tool server with read_file, write_file, git_apply_patch, run_shell_command
- **Web UI**: React/Vite dashboard (embedded in binary)
- **Terminal UI**: Bubble Tea TUI

### Opt-in Configuration

```json
{
  "execution": {
    "quality_gates_v2": true,
    "cache_enabled": true,
    "predictive_load": true,
    "memory_enabled": true,
    "checkpoint_enabled": true,
    "bitnet_routing": true,
    "worker_count": 4
  }
}
```

---

## Quick Start

### Prerequisites

Install at least one AI CLI:

```bash
# Install Claude Code (recommended)
npm install -g @anthropic-ai/claude-code

# Or install Codex
npm install -g @openai/codex

# Or install Gemini CLI
# (follow Google's installation instructions)
```

### Installation

Download the latest binary for your platform, or use the automated script:

```bash
curl -sSfL https://openexec.io/install.sh | sh
```

### Usage

```bash
openexec init          # Set up project and AI models
openexec wizard        # Define goal, generates INTENT.md
openexec run           # Execute blueprint pipeline
openexec chat          # Conversational mode
openexec doctor        # Verify CLI tools and configuration
```

---

## Project Structure

```
openexec/
├── cmd/openexec/          # CLI entry point
├── internal/
│   ├── blueprint/         # Stage-based execution engine
│   ├── cache/             # Multi-level caching
│   ├── checkpoint/        # Crash recovery
│   ├── cli/               # Cobra commands
│   ├── context/           # Two-stage context assembly
│   ├── dcp/               # Deterministic Control Plane (tool routing)
│   ├── harness/           # Integrated orchestration
│   ├── loop/              # CLI process management
│   ├── mcp/               # Model Context Protocol server
│   ├── memory/            # Pattern learning
│   ├── parallel/          # Multi-agent coordination
│   ├── predictive/        # File pre-loading
│   ├── quality/           # Lint/test gates
│   ├── router/            # BitNet + keyword routing
│   ├── runner/            # Model -> CLI resolution
│   ├── toolset/           # Toolset definitions and registry
│   ├── tui/               # Terminal UI (Bubble Tea)
│   └── validation/        # E2E and compatibility tests
├── pkg/
│   ├── agent/             # AI provider adapters
│   ├── manager/           # Multi-pipeline orchestrator
│   └── api/               # HTTP handlers and WebSocket
├── ui/                    # Web UI (React/Vite)
├── agents/                # Personas, workflows, manifests
└── docs/                  # Documentation
```

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

---

<p align="center">
  Single-binary AI orchestration. Go + React.
</p>
