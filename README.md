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

OpenExec is a **single-binary orchestration layer** that wraps AI CLI tools (Claude Code, Codex, Gemini CLI) or connects directly to any OpenAI-compatible API (Kimi, Mistral, Ollama, etc.) with deterministic infrastructure: structured pipelines, quality gates, checkpointing, and memory.

## How It Works

```
openexec init          # Configure project (model, settings)
openexec run           # Execute tasks via blueprint pipeline

Execution Flow:
  CLI -> Manager -> Pipeline -> Blueprint Engine -> AI CLI or API Provider
                                    |
                      gather_context -> implement -> lint -> test -> review
```

### Three Execution Modes

| Mode | Description | Side Effects |
|------|-------------|--------------|
| **Chat** | Conversational, no side effects | None |
| **Task** | Scoped action, produces artifacts | Creates files/patches |
| **Run** | Blueprint execution over task | Full automation |

### Supported Execution Modes

**CLI Tools** (spawn local subprocess):

| CLI | Provider | Installation |
|-----|----------|--------------|
| `claude` | Anthropic | `npm install -g @anthropic-ai/claude-code` |
| `codex` | OpenAI | `npm install -g @openai/codex` |
| `gemini` | Google | `npm install -g @google/gemini-cli` |

**API Providers** (OpenAI-compatible HTTP API):

Any OpenAI-format API works: Kimi K2.5, Mistral, Ollama, DeepSeek, Together AI, etc. Configure in `openexec init` or set directly in config:

```json
{
  "execution": {
    "api_provider": "openai_compat",
    "api_base_url": "https://api.moonshot.cn/v1",
    "api_key": "$KIMI_API_KEY",
    "api_model": "moonshot-v1-128k"
  }
}
```

API mode enables true multi-agent parallel execution with the coordinator pattern.

---

## Features

**Core (always on):**
- **Blueprint Execution**: 5-stage pipeline (gather_context -> implement -> lint -> test -> review)
- **Multi-Model Support**: Claude, Codex, Gemini via CLI; any OpenAI-compatible API via shim
- **Deterministic Routing**: Task classification (mode, toolset, repo zones, sensitivity)
- **Skills System**: Load SKILL.md knowledge packages, auto-selected per task, Claude Code compatible
- **Context Pruning**: Intelligent file selection to reduce token usage

**Opt-in (via `.openexec/config.json`):**
- **API Provider**: Use any OpenAI-format API (Kimi, Mistral, Ollama) instead of CLI tools
- **Coordinator Multi-Agent**: Frontier model decomposes tasks, workers execute in parallel, coordinator merges
- **BitNet Routing**: Local 1-bit LLM for enhanced intent classification (auto-downloads model)
- **Quality Gates V2**: Auto-detects project type (Go/Python/TS/Rust), runs lint/test/format gates
- **Checkpointing**: Deterministic checkpoints after each stage for crash recovery
- **Memory System**: Extracts learning patterns from completed stages, injects in future runs
- **Predictive Loading**: Pre-fetches likely-needed files based on task description
- **Caching**: Knowledge cache and tool result cache to avoid redundant work

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
    "worker_count": 4,
    "api_provider": "openai_compat",
    "api_base_url": "https://api.openai.com/v1",
    "api_key": "$OPENAI_API_KEY",
    "api_model": "gpt-4o",
    "coordinator_model": "gpt-4o",
    "worker_model": "gpt-4o-mini"
  }
}
```

---

## Quick Start

### Prerequisites

Install at least one AI CLI **or** configure an API provider:

```bash
# Option A: Install a CLI tool
npm install -g @anthropic-ai/claude-code   # Claude Code
npm install -g @openai/codex               # Codex (OpenAI)
npm install -g @google/gemini-cli          # Gemini CLI

# Option B: Use any OpenAI-compatible API (no CLI needed)
# Configure during 'openexec init' — works with Kimi, Mistral, Ollama, etc.
```

### Installation

Download the latest binary for your platform, or use the automated script:

```bash
curl -sSfL https://openexec.io/install.sh | sh
```

### Usage

```bash
openexec init          # Set up project, AI models, and features
openexec wizard        # Define goal, generates INTENT.md
openexec run           # Execute blueprint pipeline
openexec chat          # Conversational mode
openexec doctor        # Verify CLI tools and configuration
openexec skills list   # List loaded skills
openexec knowledge index  # Index project symbols
```

---

## Project Structure

```
openexec/
├── cmd/openexec/          # CLI entry point
├── internal/
│   ├── agent/             # Coordinator + worker multi-agent execution
│   ├── blueprint/         # Stage-based execution engine
│   ├── cache/             # Knowledge + tool result caching
│   ├── checkpoint/        # Crash recovery checkpoints
│   ├── cli/               # Cobra commands (init, run, chat, skills, knowledge)
│   ├── context/           # Two-stage context assembly + pruning
│   ├── loop/              # CLI subprocess + API runner execution
│   ├── mcp/               # Model Context Protocol server (JSON-RPC)
│   ├── memory/            # Pattern learning across sessions
│   ├── parallel/          # Parallel blueprint engine
│   ├── predictive/        # File pre-loading
│   ├── quality/           # Lint/test/format gates
│   ├── router/            # Deterministic + BitNet routing
│   ├── skills/            # SKILL.md loading, selection, Claude import
│   └── toolset/           # Toolset definitions and registry
├── pkg/
│   ├── agent/             # AI provider adapters (OpenAI, Anthropic, Gemini)
│   ├── manager/           # Multi-pipeline orchestrator + scheduler
│   └── api/               # HTTP handlers and WebSocket
├── ui/                    # Web UI (React/Vite, embedded in binary)
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
