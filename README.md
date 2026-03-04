<p align="center">
  <img src="https://raw.githubusercontent.com/openexec/openexec/main/docs/assets/logo.svg" alt="OpenExec Logo" width="200"/>
</p>

<h1 align="center">OpenExec</h1>

<p align="center">
  <strong>From Intent to Production: Managed Autonomous Development</strong>
</p>

<p align="center">
  <a href="#what-is-openexec">Overview</a> •
  <a href="#conversational-orchestration">Conversational Mode</a> •
  <a href="#how-to-start">Quick Start</a> •
  <a href="#architecture">Architecture</a> •
  <a href="#contributing">Contributing</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"/>
  <img src="https://img.shields.io/badge/status-active-success.svg" alt="Status"/>
  <img src="https://img.shields.io/badge/platform-cross--platform-informational.svg" alt="Platform"/>
</p>

---

## What is OpenExec?

**OpenExec** is a task orchestration framework designed to close the gap between human high-level intent and verified, production-ready code.

Unlike "chat-and-hope" AI tools, OpenExec treats AI agents as managed workers in a structured pipeline. It doesn't just write code; it **plans, reviews, executes, and validates** every change through a recursive autonomous loop.

### Why OpenExec?
*   **Structural Derivation:** Move beyond "generative guessing." OpenExec derives technical tasks from Measurable Goals and Interface Contracts.
*   **Goal-Based Validation:** Every story is linked to a primary project goal. Implementation success is measured by executable verification scripts tied directly to these goals.
*   **Constraint-First:** A guided interview process (Wizard) pins down platform, shape, data source mapping, and contracts before a single line of code is written.
*   **Interface-First Parallelism:** Tasks are automatically scheduled using an enhanced DAG. Dependent stories unlock as soon as their prerequisite's **Interface Contract** is defined, enabling maximum parallel performance.
*   **Headless Execution:** Agents run in a non-interactive daemon mode, managed by a Go-based execution engine.
*   **Senior Architect Reviews:** Built-in multi-iteration self-review cycles ensure implementation readiness.
*   **Autonomous Verification Gates:** The engine automatically executes local verification scripts after every task to ensure the "Definition of Done" is met.

---

## Conversational Orchestration

OpenExec includes a **conversational mode** that transforms project management into an interactive dialogue with AI agents. Instead of batch commands, engineers chat with agents to plan, implement, and verify changes.

### Core Capabilities

| Capability | Description |
| :--- | :--- |
| **Multi-Provider Support** | Chat with Claude, OpenAI, or Gemini through a unified interface |
| **Tool Execution with Approvals** | File operations and shell commands require explicit approval gates |
| **Auto-Context Injection** | Every prompt includes INTENT.md, task state, git status, and recent logs |
| **Session Persistence** | Conversations stored in SQLite for resumption, forking, and audit trails |
| **Real-Time Cost Tracking** | Monitor token usage and estimated costs per session and overall |
| **Quality Gate Integration** | Automatic lint/test/typecheck when agents signal completion |
| **Signal Protocol** | Agents communicate via structured `axon_signal` events |

### Three-Layer Architecture

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

### Guided Intent Wizard

The wizard (`openexec wizard`) provides a conversational interface for project bootstrapping:

```bash
$ openexec wizard

=== OpenExec Guided Intent Interviewer ===

Tell me about your project:
> I want to build a REST API for user management

[Thinking...]

I understand we are building a NEW PROJECT from scratch.

  ✔ Explicit:
    - Project type: REST API
    - Domain: User management

  ⚠ Assumed:
    - Framework: FastAPI (unconfirmed)

? Which framework would you prefer? (FastAPI, Flask, Express):
> FastAPI is fine

# The wizard continues until all constraints are pinned,
# then generates a verified INTENT.md
```

The wizard extracts **explicit facts** from your input, identifies **assumptions** that need confirmation, and generates a structured `INTENT.md` with all constraints validated.

### Agent Loop Lifecycle

```
┌──────────────────────────────────────────────────────────┐
│ 1. Build Context                                         │
│    - Gather project context (INTENT.md, tasks, logs)    │
│    - Apply token budget constraints                      │
├──────────────────────────────────────────────────────────┤
│ 2. LLM Request                                           │
│    - Stream response from provider                       │
│    - Track token usage and cost                          │
├──────────────────────────────────────────────────────────┤
│ 3. Process Response                                      │
│    - Execute tool calls with approval gates              │
│    - Check for completion signals                        │
├──────────────────────────────────────────────────────────┤
│ 4. Check Completion                                      │
│    - phase-complete signal → run quality gates          │
│    - Gates pass → loop completes                        │
│    - Gates fail → auto-fix and retry                    │
└──────────────────────────────────────────────────────────┘
```

### Signal Protocol

Agents communicate state via the `axon_signal` tool:

| Signal | Purpose |
| :--- | :--- |
| `phase-complete` | Task finished; triggers quality gates |
| `blocked` | Waiting for human input; pauses loop |
| `progress` | Incremental work done; resets thrash detection |
| `decision-point` | Needs human decision before continuing |
| `route` | Hand off to another specialized agent |

### Quick Start

```bash
# Initialize project and configure providers
openexec init

# Start guided intent wizard
openexec wizard

# Generate execution plan from INTENT.md
openexec plan INTENT.md

# Launch autonomous execution daemon
openexec start --daemon

# Monitor via terminal UI
openexec tui
```

**Full Documentation:** [docs/CONVERSATIONAL_ORCHESTRATION.md](docs/CONVERSATIONAL_ORCHESTRATION.md)

---

## How It Works

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   INTENT    │────▶│   PLANNING   │────▶│    TASKS    │
│  (PRD/Spec) │     │ (Goal Tree)  │     │  (stories)  │
└─────────────┘     └──────────────┘     └──────┬──────┘
       ▲                   │                    │
       │            ┌──────▼──────┐             │
       └────────────┤    GOALS    │◀────────────┘
                    └──────┬──────┘
                           │
                    ┌──────▼───────┐     ┌─────────────┐
                    │  EXECUTION   │────▶│   QUALITY    │
                    │ (AI Agent)   │     │   GATES      │
                    └──────┬───────┘     └──────┬──────┘
                           │                    │
                    ┌──────▼───────┐     ┌──────▼──────┐
                    │   HITL       │────▶│    VERIFY    │
                    │ (approval)   │     │ (Goal-Based) │
                    └──────────────┘     └──────────────┘
```

---

## GitFlow Integration & Traceability

OpenExec enforces a strict GitFlow architecture to ensure every code change is traceable back to its original requirement.

1.  **Release Mapping:** An `INTENT.md` represents a high-level release (e.g., `v1.0.0`). OpenExec creates a **Release Branch** (`release/1.0.0`) from your base branch (`main` or `develop`).
2.  **Story Branches:** Each user story (e.g., `US-001`) is isolated in its own **Feature Branch** (`feature/US-001`), branched from the active Release Branch.
3.  **Task Commits:** Every technical task (e.g., `T-001`) results in a dedicated **Commit**. If multiple iterations (fixes) are required to pass quality gates, each fix is its own commit, providing a full audit trail of the agent's reasoning.
4.  **Auto-Merge Cascade:** When all tasks for a story are complete and approved, OpenExec automatically merges the Feature Branch into the Release Branch.
5.  **Release Finalization:** Once all stories are merged, the Release Branch is merged back into the base branch (`main`), tagged (e.g., `v1.0.0`), and ready for deployment.

---

## Manual Testing & Integrated UI

The unified OpenExec environment allows you to manage multiple projects and AI models through a single web interface.

### 1. Start the Unified Stack
From the `openexec` root directory, launch the integrated backend and UI:

```bash
# Start backend (Port 8080)
./bin/axon serve --tract-store ../initial --audit-db .openexec/data/audit.db --projects-dir .. --port 8080

# Start UI (Port 3001) in another terminal
cd ui && npm run dev -- --port 3001
```

### 2. Testing Workflows

#### A. Project Discovery
- Open `http://localhost:3001`.
- The **Project Workspace** dropdown in the left sidebar should automatically list all directories in your workspace containing an `openexec.yaml` file.
- Switching projects will filter the session list to only show conversations for that workspace.

#### B. Initializing a New Project
- Click the **"Init"** button next to the project selector.
- Use the **Directory Picker** to navigate your local filesystem and select a target folder.
- Enter a project name and click **Initialize**.
- The backend will create the `.openexec` structure and `openexec.yaml`, and the UI will automatically select the new project.

#### C. Running the Guided Wizard
- Select a project from the dropdown.
- Click the **"Wizard"** button.
- A chat interface will appear. Type "start" to begin the guided intent interview.
- Follow the prompts to define your project. Once complete, click **"Generate INTENT.md"** to persist the requirements.

#### D. Multi-Model Chat Sessions
- Click the **"New"** session button in the sidebar.
- Select your preferred **Provider** (Anthropic, OpenAI, Gemini) and **Model** (e.g., Claude 3.5 Sonnet, GPT-4o).
- Create the session and start chatting. The orchestrator will use the specific model selected for that conversation turn.

### 3. Automated Integration Tests
Verify the full UI-Backend handshake using Playwright:

```bash
cd ui
npm run test:e2e:list
```

---

## Architecture

OpenExec is now consolidated into two primary repositories for simplified management and atomic deployment:

| Module | Repository | Role | Language |
| :--- | :--- | :--- | :--- |
| **OpenExec Core** | [`openexec`](../openexec) | The "Body" & "Interface" - contains CLI, Execution Engine, Interface Gateway (Telegram/WhatsApp), and MCP Signal Server. | Go |
| **Orchestrator** | [`openexec-planner`](../openexec-planner) | The "Brain" - handles planning, dependency modeling, and the Wizard. | Python |
| **Dashboard** | [`openexec-dashboard`](../openexec-dashboard) | The "Observability" - browser-based UI for monitoring multi-project activity. | TypeScript/Next.js |

### Key Components (Consolidated):
*   **CLI (`openexec`):** Unified interface for project management, dashboards, and execution control.
*   **Execution Engine (`openexec start`):** Subcommand that launches the autonomous task daemon.
*   **Interface Gateway (`openexec-interface`):** Subcommand that handles human-in-the-loop approvals via Telegram/WhatsApp.
*   **MCP Server (`openexec mcp-serve`):** Built-in tool server that allows agents to communicate status directly to the core.

---

## How to Start

### 1. Installation
The quickest way to get started is using the unified install script in the core repository:

```bash
git clone https://github.com/openexec/openexec.git
cd openexec
./scripts/install.sh
```

### 2. The Execution Flow
Follow these steps to transform an idea into a verified project:

1.  **Initialize (`openexec init`)**
    Set up your project configuration and select your preferred AI models (Claude, Codex, Gemini).
2.  **Guided Interview (`openexec wizard`)**
    Chat with the AI Architect to define your project shape, platform, and integration contracts. It generates a verified `INTENT.md`.
3.  **Plan (`openexec plan INTENT.md`)**
    OpenExec decomposes your intent into a structured set of technical stories and tasks.
4.  **Import (`openexec story import`)**
    Synchronize the AI-generated plan into the local SQLite tracking system.
5.  **Start Daemon (`openexec start --daemon`)**
    Launch the background engine that manages the autonomous agents.
6.  **Run (`openexec run`)**
    The agents begin implementing your tasks (concurrently by default), signaling completion via `axon_signal`.
7.  **Monitor (`openexec status` or `openexec tui`)**
    Watch the real-time progress and logs through the terminal dashboard.
8.  **Verify Goals (`openexec goal verify --execute`)**
    Run the high-level verification scripts to prove that the project's primary goals have been met.

---

## The Managed Loop

OpenExec operates on a **recursive autonomous loop**:

1.  **Context Construction:** The engine builds a rich prompt containing the task, relevant files, and system constraints.
2.  **Autonomous Action:** The agent (e.g., Claude Code) implements changes locally.
3.  **Verification:** The agent runs local tests or uses quality gates to verify the fix.
4.  **Signaling:** When complete, the agent uses the **Axon tool** to signal `phase-complete`.
5.  **Review:** An independent reviewer agent validates the work against the original acceptance criteria.

---

## Multi-Agent Support

| Agent | Best For |
| :--- | :--- |
| **OpenCode** | The Unified Operator. Can act as a local replacement or manage other agents. |
| **Claude Code** | Complex reasoning, large refactors, and architectural changes. |
| **Codex** | High-speed code completion and standard REST API implementation. |
| **Gemini** | Fast iteration and large-context codebase analysis. |

---

## Contributing

We welcome engineers, architects, and AI enthusiasts to help evolve the orchestration plane.
Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

---

<p align="center">
  Built with AI, for AI-assisted development.
</p>
