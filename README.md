<p align="center">
  <img src="https://raw.githubusercontent.com/openexec/openexec/main/docs/assets/logo.svg" alt="OpenExec Logo" width="200"/>
</p>

<h1 align="center">OpenExec</h1>

<p align="center">
  <strong>The Deterministic AI Operating System: From Intent to Production</strong>
</p>

<p align="center">
  <a href="#what-is-openexec">Overview</a> •
  <a href="#conversational-orchestration">Conversational Mode</a> •
  <a href="#how-to-start">Quick Start</a> •
  <a href="docs/GET_STARTED.md">Getting Started Guide</a> •
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

**OpenExec** is a single-binary task orchestration framework designed to close the gap between human high-level intent and verified, production-ready code.

Unlike "chat-and-hope" AI tools, OpenExec treats AI agents as managed workers in a structured pipeline. It doesn't just write code; it **plans, reviews, executes, and validates** every change through a recursive autonomous loop.

## Core Pillars: Turning Policy into Reality

OpenExec bridges the gap between machine speed and institutional trust by embedding governance directly into the architecture.

1.  **Safety by Design (Rule-Based Logic):** Translate laws and regulations into local YAML guardrails. Rules act as physical gates—if an action breaks your policy, the system blocks it locally before it happens.
2.  **Institutional Memory (Owned Logic):** You own the "Library" of logic the AI builds. Your organizational patterns stay local, ensuring you can swap AI providers without losing your intelligence.
3.  **Information Limiting (Privacy-First):** Control exactly what external APIs see. Sensitive metadata—like API keys, server IPs, and full network maps—stay local. Cloud models only receive the specific context required for the task.
4.  **GDPR Compliance (PII Shield):** Detect and scrub Personally Identifiable Information (PII) locally. Automatically masks emails, Finnish personal identity codes (HETU), IP addresses, and API keys before they reach any external cloud model.
5.  **Digital Flight Recorder:** Records not just *what* changed, but *why*. Captures the complete reasoning chain in a tamper-proof vault for public sector accountability.
6.  **Multi-Platform Resilience:** Native support for macOS, Linux, and Windows. Includes **Automatic Port Probing** to resolve conflicts and handles OS security (like Gatekeeper) out-of-the-box.

**Governance isn't a speed limit; it's the brakes that allow you to move at machine speed safely.**


---

## Local Knowledge Map

OpenExec maintains local project context to improve precision and reduce prompt bloat. Indexers and gatherers assemble deterministic context packs for execution steps.

- Precision: focus execution on the smallest necessary code slices.
- Privacy: keep source local; only minimal context is sent to providers when required.

Context indexing is optional and used internally by the orchestrator; no special CLI is required to get started.

---

## Quick Start

For a detailed walkthrough, see the **[Getting Started Guide](docs/GET_STARTED.md)**.

### 1. Installation
Download the latest binary for your platform, or use the automated script:

```bash
# Default (installs to /usr/local/bin or ~/.local/bin)
curl -sSfL https://openexec.io/install.sh | sh

# Non-sudo / Custom path
curl -sSfL https://openexec.io/install.sh | INSTALL_DIR=$HOME/bin sh
```

The script automatically falls back to `~/.local/bin` if it doesn't have permission to write to system directories.

Alternatively, build from source:
```bash
go build -o openexec ./cmd/openexec
```

### 2. Updating
To update to the latest version at any time:
```bash
openexec update
```

## The Execution Flow
Follow these steps to transform an idea into a verified project:

1.  **Initialize (`git init && openexec init`)**
    Set up Git if necessary, then run the OpenExec initialization to select your preferred AI models.
2.  **Guided Interview (`openexec wizard`)**
    Chat with the AI Architect to define your project shape, platform, and contracts. It generates a verified `INTENT.md`.
3.  **Plan (`openexec plan INTENT.md`)**
    OpenExec decomposes your intent into a structured set of technical stories and tasks by chatting with the AI agent.
4.  **Start Server (`openexec start --ui`)**
    Launch the integrated server and open the web dashboard. Use `--daemon` for background mode with **Automated PID Tracking**.
5.  **Run (`openexec run`)**
    The agents begin implementing your tasks through a specialized **Autonomous Pipeline**.

### Task Lifecycle Phases
Every task in OpenExec automatically progresses through five distinct phases, each handled by a specialized agent persona:

- **TD (Technical Design / clario):** Research, codebase mapping, and strategy formulation.
- **IM (Implementation / spark):** Actual code modification and task execution.
- **RV (Review / blade):** Independent quality assurance and architectural validation.
- **RF (Refinement / hon):** Post-review adjustments and optimization.
- **FL (Finalize / clario):** Verification of all goals and state synchronization.

---

## Architecture

OpenExec is a **Self-Contained Monolith** designed for atomic deployment and maximum reliability.

```mermaid
graph TD
    User([CLI / UI]) --> Orchestrator[Deterministic Orchestrator]

    subgraph "Execution Layer"
        Orchestrator --> Loop[Phase State Machine]
        Loop --> Tools[Tool Harness (read/write/patch/run/git)]
        Tools --> Gates[Quality Gates]
        Gates --> Commit[Safe Commit]
    end

    subgraph "Persistence"
        Orchestrator --> DB[(SQLite: sessions, runs, events)]
    end

    style User fill:#238636,color:#fff
    style Orchestrator fill:#1f6feb,color:#fff
    style DB fill:#161b22,color:#c9d1d9
```

| Component | Role | Implementation |
| :--- | :--- | :--- |
| **CLI** | Unified Interface | Go (Cobra) |
| **Planner** | Story & Goal Generation | Chat with AI Agent |
| **Wizard** | Requirement Gathering | Chat with AI Agent |
| **Orchestrator** | Durable Task Execution | Go + SQLite |
| **Dashboard** | Visual Hub | React (Embedded in binary) |

---

## Local UI Development

The CLI and orchestration engine are delivered as a single binary with the UI embedded. For development of the React dashboard:

```bash
cd ui
npm install
npm run dev -- --port 3001
```

Open the dashboard at http://localhost:3001. The dev server proxies requests to the backend started via `openexec start`.


---

## Contributing

We welcome engineers, architects, and AI enthusiasts to help evolve the orchestration plane.
Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

---

<p align="center">
  Built with AI, for AI-assisted development.
</p>



## Architecture & Visualization

- Read the high‑level architecture at `docs/ARCHITECTURE.md`.
- Use `docs/VISUALIZATION_GUIDE.md` to generate diagrams (swimlanes, nodes/edges, Mermaid starter, runner/health callouts).

Key references:
- Artifacts: INTENT.md (PRD), goals[], .openexec/stories.json, .openexec/tasks.json, .openexec/stories/*.md, .openexec/fwu/*.md
- Ordering: story.depends_on injects ALL tasks from prerequisite stories; tasks in each story run in listed order
- Runner: server resolves model→CLI once at startup; `GET /api/health` returns `{ runner: { command, args, model } }`
- Runs: create via `POST /api/v1/runs` then `POST /api/v1/runs/{id}/start`; or start a task via `POST /api/fwu/{task_id}/start`. Poll status with `GET /api/fwu/{task_id}/status`.
