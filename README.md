<p align="center">
  <img src="https://raw.githubusercontent.com/openexec/openexec/main/docs/assets/logo.svg" alt="OpenExec Logo" width="200"/>
</p>

<h1 align="center">OpenExec</h1>

<p align="center">
  <strong>AI-Powered Task Orchestration for Software Development</strong>
</p>

<p align="center">
  <a href="#features">Features</a> •
  <a href="#quick-start">Quick Start</a> •
  <a href="#supported-agents">Agents</a> •
  <a href="#documentation">Docs</a> •
  <a href="#contributing">Contributing</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"/>
  <img src="https://img.shields.io/badge/go-1.23+-00ADD8.svg" alt="Go"/>
  <img src="https://img.shields.io/badge/node-20+-339933.svg" alt="Node"/>
</p>

---

**OpenExec** transforms how you build software with AI. Define your intent, and let AI agents write, test, and ship production-ready code — with built-in quality gates, human approvals, and full audit trails.

```bash
# Initialize a new project
openexec onboard

# Run a task
openexec run T-001

# Or let the daemon handle everything
openexec daemon start
```

## Features

- **Multi-Agent Support** — Use Claude, Codex, Gemini, or local models via Ollama
- **Quality Gates** — Automated linting, type checking, testing, and security scans
- **Human-in-the-Loop** — Telegram/WhatsApp approvals for critical operations
- **Auto-Fix** — Automatically generates fix tasks when quality gates fail
- **Full Audit Trail** — Every action logged for compliance and debugging
- **Multi-Project** — Orchestrate multiple repositories from a single daemon
- **Language Agnostic** — Works with Python, Go, TypeScript, Rust, and more

## Supported AI Agents

| Agent | CLI | Provider | Models | Best For |
|-------|-----|----------|--------|----------|
| **Claude Code** | `claude` | Anthropic | sonnet, opus | Complex reasoning, large codebases |
| **Codex** | `codex` | OpenAI | gpt-4.1, gpt-5 | Code completion, refactoring |
| **Gemini** | `gemini` | Google | 3.1-pro-preview, 3.1-flash-preview | Multi-modal, fast iteration |
| **OpenCode** | `opencode` | Ollama | Any local model | Privacy, offline, cost-free |

## Quick Start

### Installation

```bash
# Clone the CLI repository
git clone https://github.com/openexec/openexec-cli.git
cd openexec-cli

# Build and install the CLI
go build -o bin/openexec .

# (Optional) Add to your PATH
# sudo mv bin/openexec /usr/local/bin/
```

### Install an AI Agent

```bash
# Pick one (or more)
npm install -g @anthropic-ai/claude-code   # Claude Code
npm install -g @openai/codex               # Codex
npm install -g @google/gemini-cli          # Gemini
go install github.com/opencode-ai/opencode@latest  # OpenCode (local)
```

### Initialize Your Project

```bash
cd your-project

# Interactive setup (recommended)
openexec onboard

# Or quick setup with defaults
openexec onboard --quickstart
```

### Run Tasks

```bash
# Run a specific task
openexec run T-001

# Run all pending tasks
openexec daemon start

# Check status
openexec status
```

## How It Works

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   INTENT    │────▶│   PLANNING   │────▶│    TASKS    │
│  (PRD/Spec) │     │ (Goal Tree)  │     │  (stories)  │
└─────────────┘     └──────────────┘     └──────┬──────┘
                                                │
                    ┌───────────────────────────┘
                    ▼
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│  EXECUTION  │────▶│   QUALITY    │────▶│   REVIEW    │
│ (AI Agent)  │     │   GATES      │     │  (optional) │
└─────────────┘     └──────────────┘     └──────┬──────┘
                                                │
                    ┌───────────────────────────┘
                    ▼
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   HITL      │────▶│    MERGE     │────▶│    DONE     │
│ (approval)  │     │  (optional)  │     │             │
└─────────────┘     └──────────────┘     └─────────────┘
```

## Configuration

Create `openexec.yaml` in your project root:

```yaml
project:
  name: my-project

agents:
  default: claude
  claude:
    model: sonnet
    timeout: 600
  codex:
    model: gpt-5
    timeout: 600

review:
  review_agent: codex    # Different agent for code review
  require_review: true

quality:
  gates:
    - lint
    - typecheck
    - test

execution:
  auto_fix: true         # Auto-generate fix tasks on failure
  max_fix_iterations: 3
```

Or configure interactively:

```bash
openexec config set agents.default codex
openexec config set review.review_agent claude
openexec config show
```

## Quality Gates by Language

<details>
<summary><strong>Python</strong></summary>

```yaml
quality:
  gates: [lint, typecheck, test]
  custom:
    - name: lint
      command: "ruff check src/"
    - name: typecheck
      command: "mypy src/"
    - name: test
      command: "pytest --cov=src"
```
</details>

<details>
<summary><strong>Go</strong></summary>

```yaml
quality:
  gates: [go_fmt, go_lint, go_test, go_sec]
  custom:
    - name: go_fmt
      command: "go fmt ./... && git diff --exit-code -- '*.go'"
    - name: go_lint
      command: "golangci-lint run ./..."
    - name: go_test
      command: "go test -v -race ./..."
    - name: go_sec
      command: "gosec ./..."
```
</details>

<details>
<summary><strong>TypeScript / JavaScript</strong></summary>

```yaml
quality:
  gates: [lint, typecheck, test, audit]
  custom:
    - name: lint
      command: "npm run lint"
    - name: typecheck
      command: "npm run type-check"
    - name: test
      command: "npm test"
    - name: audit
      command: "npm audit --omit=dev"
```
</details>

<details>
<summary><strong>Rust</strong></summary>

```yaml
quality:
  gates: [fmt, clippy, test]
  custom:
    - name: fmt
      command: "cargo fmt --check"
    - name: clippy
      command: "cargo clippy -- -D warnings"
    - name: test
      command: "cargo test"
```
</details>

## Project Structure

```
openexec/
├── initial/                 # Core Python CLI (pip install)
├── openexec-cli/            # Go CLI with TUI dashboard
├── openexec-execution/      # Go execution engine
├── openexec-interface/      # Telegram/WhatsApp HITL
├── openexec-orchestration/  # Planning & Goal Tree
└── openexec-web/            # Next.js dashboard
```

## Environment Variables

```bash
# Agent selection
export OPENEXEC_DEFAULT_AGENT=claude
export OPENEXEC_AGENT_CLAUDE_MODEL=opus
export OPENEXEC_REVIEW_AGENT=codex

# Execution
export OPENEXEC_EXECUTION_TIMEOUT=900
export OPENEXEC_EXECUTION_AUTO_FIX=true

# Daemon
export OPENEXEC_DAEMON_MAX_PARALLEL=4
```

See [Configuration Guide](docs/CONFIGURATION.md) for all options.

## Human-in-the-Loop (HITL)

Enable approval workflows via Telegram or WhatsApp:

```yaml
# .env
TELEGRAM_BOT_TOKEN=your-bot-token
TELEGRAM_WEBHOOK_SECRET=your-secret
```

Operators can approve, reject, or pause tasks directly from their phone.

## Multi-Project Orchestration

Manage multiple projects from a single daemon:

```yaml
project:
  name: my-orchestrator
  type: meta

daemon:
  multi_project: true
  projects_path: "/path/to/projects"
  project_filter:
    - frontend
    - backend
    - shared-lib
  max_parallel: 2
```

## CLI Reference

| Command | Description |
|---------|-------------|
| `openexec onboard` | Interactive project setup |
| `openexec run <task>` | Execute a specific task |
| `openexec daemon start` | Start background task processor |
| `openexec status` | Show task status |
| `openexec gates` | Run quality gates |
| `openexec config show` | Display configuration |
| `openexec agents --all` | List available agents |

## Documentation

- [Configuration Guide](docs/CONFIGURATION.md) — All settings and options
- [INTENT.md](INTENT.md) — Project vision and goals

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

```bash
# Clone the repo
git clone https://github.com/openexec/openexec-cli.git
cd openexec-cli

# Build with development flags
go build -v .

# Run tests
go test ./...
```

## License

MIT License — see [LICENSE](LICENSE) for details.

---

<p align="center">
  Built with AI, for AI-assisted development
</p>
