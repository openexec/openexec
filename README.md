<p align="center">
  <img src="https://raw.githubusercontent.com/openexec/.github/main/profile/logo.svg" alt="OpenExec Logo" width="160"/>
</p>

<h1 align="center">OpenExec</h1>

<p align="center">
  <strong>Executable engineering judgment for AI coding tools</strong>
</p>

<p align="center">
  <img src="https://img.shields.io/github/v/release/openexec/openexec?style=flat-square&color=orange" alt="Version"/>
  <img src="https://img.shields.io/github/actions/workflow/status/openexec/openexec/go.yml?style=flat-square" alt="Build Status"/>
  <img src="https://img.shields.io/badge/go-%2300ADD8.svg?style=flat-square&logo=go&logoColor=white" alt="Go"/>
  <img src="https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square" alt="License"/>
</p>

---

## What Is OpenExec?

Can expert engineering judgment become executable?

Most engineering expertise is tacit. It lives in senior engineers' heads: where to look first, which files not to touch, which tests actually matter, when a shortcut is acceptable, and when a change must stop for human review.

AI coding tools can generate code, but they do not automatically inherit that judgment. Left alone, an agent can read too much, touch the wrong surface, retry unpredictably, skip implicit checks, or leave behind a confident summary instead of evidence.

OpenExec is a deterministic runtime for AI coding tools. It wraps Claude Code, Codex, Gemini CLI, or any OpenAI-compatible API in executable structure: blueprints, scoped tools, quality gates, checkpoints, memory, policy, approvals, and audit trails.

The model still reasons. OpenExec governs the run.

## The Thesis

OpenExec treats AI work as an engineering process, not a chat session.

Documentation can explain expert judgment. Checklists can remind people to apply it. OpenExec tries to encode more of that judgment as reusable runtime behavior:

- **Blueprints** capture repeatable workflows.
- **Tool policies** limit what an agent can see or change.
- **Quality gates** make tests, linting, formatting, and review explicit.
- **Checkpoints** preserve intermediate state for recovery and inspection.
- **Memory and skills** carry lessons from previous work into future runs.
- **Audit trails** record what happened, not just what the agent said happened.

That is the product boundary: OpenExec is not the agent. It is the control plane around the agent.

## Current Scope

OpenExec is intentionally factual about what exists today and what remains architectural direction.

**Shipped or implemented in the repository:**
- Single-binary Go runtime with CLI, TUI, embedded web UI, and MCP server surfaces.
- Blueprint execution for staged work such as gather context, implement, lint, test, and review.
- Provider adapters for local AI CLIs and OpenAI-compatible HTTP APIs.
- Context assembly, pruning, skills loading, memory, caching, checkpointing, and predictive loading packages.
- SQLite-backed state, audit, approval, budget, telemetry, and PII scrubbing components.
- SRE-oriented safety primitives including allowlisted infra tools, Terraform plan inspection, and approval gates.

**Still maturing:**
- End-to-end replay and durable recovery UX.
- Production polish for the web console and operator workflows.
- Organization-wide policy distribution.
- Large-scale distributed agent and backlog coordination.

The direction is ambitious, but the core idea is narrow: make AI execution inspectable, bounded, repeatable, and easier to improve.

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
- **SRE Orchestration**: Deny-by-default infra tools (Ansible, Salt, SSH, Terraform) compiled from an operator-owned allowlist; TOCTOU-safe saved-plan terraform applies with deterministic destructive-change detection; persistent human approval gate with cross-process sign-off. See [docs/SECURITY_MODEL.md](docs/SECURITY_MODEL.md) for how hallucinations, prompt injection, and log poisoning are contained — and [docs/SRE_ORCHESTRATION_ROADMAP.md](docs/SRE_ORCHESTRATION_ROADMAP.md) for the engineering spec
- **Web UI**: React/Vite dashboard (embedded in binary)
- **Terminal UI**: Bubble Tea TUI

**The Reusability Ecosystem (Three-Tier AI Libraries):**
- **Tier 1 (Architecture)**: Standardize layouts, data schemas, and API contracts using [blueprints](https://github.com/openexec/blueprints).
- **Tier 2 (Functional)**: Ingest regulatory, compliance, and functional standards dynamically using [intent-compiler packs](https://github.com/openexec/intent-compiler).
- **Tier 3 (Code/Implementation)**: Reuse components and utility snippets instantly via [Skills](docs/REUSABILITY_LIBRARIES.md) (`SKILL.md` format).

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
