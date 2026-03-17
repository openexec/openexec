<p align="center">
  <img src="https://raw.githubusercontent.com/openexec/.github/main/profile/logo.svg" alt="OpenExec Logo" width="160"/>
</p>

<h1 align="center">OpenExec</h1>

<p align="center">
  <strong>The Deterministic AI Operating System: From Intent to Production</strong><br>
  <em>Treating AI systems as operational software, not just prototypes.</em>
</p>

<p align="center">
  <img src="https://img.shields.io/github/v/release/openexec/openexec?style=flat-square&color=orange" alt="Version"/>
  <img src="https://img.shields.io/github/actions/workflow/status/openexec/openexec/go.yml?style=flat-square" alt="Build Status"/>
  <img src="https://img.shields.io/badge/go-%2300ADD8.svg?style=flat-square&logo=go&logoColor=white" alt="Go"/>
  <img src="https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square" alt="License"/>
  <img src="https://img.shields.io/badge/GDPR-Compliant-green.svg?style=flat-square" alt="GDPR"/>
</p>

<p align="center">
  <a href="https://openexec.io">Website</a> •
  <a href="docs/GET_STARTED.md">Get Started</a> •
  <a href="#architecture">Architecture</a> •
  <a href="#blueprints">Blueprints</a> •
  <a href="https://github.com/openexec/openexec/issues">Report Bug</a>
</p>

---

## What is OpenExec?

**OpenExec** is a single-binary task orchestration framework designed to treat AI agents as managed workers in a structured, production-grade pipeline. Built by a platform engineer with 20+ years of high-scale experience, it bridges the machine speed of AI with the institutional trust required for real-world business flows.

Unlike experimental "chat-and-hope" AI tools, OpenExec treats AI orchestration as **operational software**: observable, deterministic where required, and fully auditable. It plans, reviews, executes, and validates every change through a recursive autonomous loop.

## ⚡ Core Capabilities

| 🛡️ **Safety Gates** | 🧠 **Local Context** | 🔐 **PII Shield** |
| :--- | :--- | :--- |
| YAML-based guardrails block unsafe code before it hits your disk. | Local indexing ensures LLMs only see what they need—reducing cost and risk. | Automatic local scrubbing of emails, IP addresses, and sensitive metadata. |

## Core Pillars: Turning Intent into Reliable Execution

OpenExec embeds governance and observability directly into the orchestration architecture.

1.  **Operational AI:** AI systems are treated as software dependencies with strict SLAs, not black-box experiments.
2.  **Safety by Design (Rule-Based Logic):** Translate organizational policies into local YAML guardrails. Rules act as physical gates—if an action breaks policy, the system blocks it locally before it happens.
3.  **Production-Grade Observability:** Built-in instrumentation for every decision and tool call. Records not just *what* changed, but *why*, providing a complete reasoning chain for accountability.
4.  **Institutional Memory (Owned Logic):** You own the library of logic the AI builds. Organizational patterns stay local, enabling you to swap AI providers without losing operational intelligence.
5.  **Information Limiting (Privacy-First):** Precise context assembly ensures cloud models only receive the specific context required for the task. Sensitive metadata stays behind your firewall.
6.  **Digital Flight Recorder:** Every autonomous loop is recorded in a tamper-proof SQLite vault, ensuring full auditability for compliance and debugging.

---

## Architecture

OpenExec is a **Self-Contained Monolith** designed for high-integrity autonomous operations. It follows a converged architecture pattern: **deterministic local runtime** providing safety and grounding, with **frontier models** providing high-level reasoning.

### The 7-Layer Operational Model

```mermaid
graph TD
    subgraph Layer 1: Interaction
        UI[Web Dashboard]
        CLI[Unified CLI]
    end

    subgraph Layer 2: Runtime
        Session[Session Manager]
        Mode[Mode Controller: Chat/Task/Run]
    end

    subgraph Layer 3: Context
        Assembly[Context Assembly]
        Index[Knowledge Indexer]
    end

    subgraph Layer 4: Tooling
        DCP[Deterministic Control Plane]
        Toolsets[Curated Toolsets]
    end

    subgraph Layer 5: Governance
        Policy[YAML Guardrails]
        PII[PII/GDPR Shield]
    end

    subgraph Layer 6: Orchestration
        Engine[Blueprint Engine]
        Flow[gather → implement → lint → test → review]
    end

    subgraph Layer 7: Intelligence
        Local[Local Routing Model]
        Frontier[Frontier Reasoning Model]
    end

    UI & CLI --> Session
    Session --> Mode
    Mode --> Assembly
    Assembly --> DCP
    DCP --> Policy
    Policy --> Engine
    Engine --> Local & Frontier

    style Layer 6: Orchestration fill:#8957e5,color:#fff
    style Layer 5: Governance fill:#238636,color:#fff
    style Layer 4: Tooling fill:#1f6feb,color:#fff
```

### Blueprint Execution Flow

Every task is executed through a hardened, stage-based pipeline that ensures verification happens at every step.

```mermaid
stateDiagram-v2
    [*] --> gather_context: Start Run
    gather_context --> implement: Context Ready
    
    state Implementation_Loop {
        implement --> lint: Change Applied
        lint --> fix_lint: Lint Failure
        fix_lint --> lint: Fix Applied
    }
    
    state Validation_Loop {
        lint --> test: Lint Passed
        test --> fix_tests: Test Failure
        fix_tests --> test: Fix Applied
    }
    
    test --> review: All Tests Passed
    review --> [*]: Task Complete
    
    note right of gather_context: Deterministic (repo_readonly)
    note right of implement: Agentic (coding_backend)
    note right of lint: Deterministic (Quality Gate)
```

### Operational Primitives

| Component | Implementation | Role |
| :--- | :--- | :--- |
| **DCP** | `internal/dcp` | Deterministic routing and tool execution control. |
| **Toolsets** | `internal/toolset` | Role-based grouping of capabilities (e.g., `coding_backend`, `debug_ci`). |
| **Blueprints** | `internal/blueprint` | Stage-based graph execution with built-in retries and checkpoints. |
| **Persistence** | `pkg/db/state` | Canonical state store using SQLite for immutable task traces. |

---

## Quick Start

For a detailed walkthrough, see the **[Getting Started Guide](docs/GET_STARTED.md)**.

### 1. Installation
Download the latest binary for your platform, or use the automated script:

```bash
# Default (installs to /usr/local/bin or ~/.local/bin)
curl -sSfL https://openexec.io/install.sh | sh
```

### 2. The Execution Flow
Follow these steps to transform an idea into a verified project:

1.  **Initialize (`git init && openexec init`)**: Set up the project and select your preferred AI models.
2.  **Guided Interview (`openexec wizard`)**: Chat with the AI Architect to generate a verified `INTENT.md`.
3.  **Plan (`openexec plan INTENT.md`)**: Decompose intent into a structured set of technical stories and tasks.
4.  **Start Server (`openexec start --ui`)**: Launch the visual dashboard and background daemon.
5.  **Run (`openexec run`)**: Execute tasks through the specialized **Autonomous Pipeline**.

---

## Contributing

We welcome engineers and AI enthusiasts to help evolve the orchestration plane.
Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

---

<p align="center">
  Built with AI, for production-grade AI orchestration.
</p>
