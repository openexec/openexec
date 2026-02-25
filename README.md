<p align="center">
  <img src="https://raw.githubusercontent.com/openexec/openexec/main/docs/assets/logo.svg" alt="OpenExec Logo" width="200"/>
</p>

<h1 align="center">OpenExec</h1>

<p align="center">
  <strong>From Intent to Production: Managed Autonomous Development</strong>
</p>

<p align="center">
  <a href="#what-is-openexec">Overview</a> •
  <a href="#how-to-start">Quick Start</a> •
  <a href="#architecture">Architecture</a> •
  <a href="#the-workflow">Workflow</a> •
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
*   **Structured Planning:** High-level goals are decomposed into hierarchical Goal Trees and User Stories.
*   **Constraint-First:** A guided interview process (Wizard) pins down platform, shape, and contracts before a single line of code is written.
*   **Dependency-Aware Parallel Execution:** Tasks are automatically scheduled using a DAG (Directed Acyclic Graph) to run independent work in parallel while respecting prerequisites.
*   **Headless Execution:** Agents run in a non-interactive daemon mode, managed by a Go-based execution engine.
*   **Senior Architect Reviews:** Built-in multi-iteration self-review cycles ensure implementation readiness.
*   **Quality Gates:** Automated pre-flight checks and post-task validation (lint, test, build) act as permanent guardrails.

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

---

## GitFlow Integration & Traceability

OpenExec enforces a strict GitFlow architecture to ensure every code change is traceable back to its original requirement.

1.  **Release Mapping:** An `INTENT.md` represents a high-level release (e.g., `v1.0.0`). OpenExec creates a **Release Branch** (`release/1.0.0`) from your base branch (`main` or `develop`).
2.  **Story Branches:** Each user story (e.g., `US-001`) is isolated in its own **Feature Branch** (`feature/US-001`), branched from the active Release Branch.
3.  **Task Commits:** Every technical task (e.g., `T-001`) results in a dedicated **Commit**. If multiple iterations (fixes) are required to pass quality gates, each fix is its own commit, providing a full audit trail of the agent's reasoning.
4.  **Auto-Merge Cascade:** When all tasks for a story are complete and approved, OpenExec automatically merges the Feature Branch into the Release Branch.
5.  **Release Finalization:** Once all stories are merged, the Release Branch is merged back into the base branch (`main`), tagged (e.g., `v1.0.0`), and ready for deployment.

---

## Architecture

OpenExec is now consolidated into two primary repositories for simplified management and atomic deployment:

| Module | Repository | Role | Language |
| :--- | :--- | :--- | :--- |
| **OpenExec Core** | [`openexec`](../openexec) | The "Body" & "Interface" - contains CLI, Execution Engine, Interface Gateway (Telegram/WhatsApp), and MCP Signal Server. | Go |
| **Orchestrator** | [`openexec-orchestration`](../openexec-orchestration) | The "Brain" - handles planning, dependency modeling, and the Wizard. | Python |
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
