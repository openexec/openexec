# OpenExec Knowledge Base: The Deterministic Control Plane (DCP)

> **Note:** This documents the DCP subsystem which provides optional enhanced routing. Deterministic routing (always-on) handles most use cases without DCP. DCP is opt-in via the `OPENEXEC_ENABLE_DCP=true` environment variable or `EnableDCP` server config. When disabled (the default), deterministic routing handles all classification.

This document explains the architecture of OpenExec's **Deterministic Knowledge Base**, a purpose-driven relational system that replaces traditional Vector Databases for codebase management.

## Why a Deterministic Knowledge Base?

Standard AI agents often rely on "Semantic Search" (VectorDBs) to find information. This is probabilistic—the agent gets what is "similar," not necessarily what is "exact."

OpenExec's **DCP** uses structured SQLite tables to store surgical **Pointer Records**. This ensures:
1.  **Zero Hallucination:** The agent sees exactly where a function starts and ends on disk.
2.  **Low Latency:** Local lookups happen in <1ms, avoiding expensive cloud round-trips.
3.  **Privacy:** Your codebase structure and environment topologies never leave your machine.

---

## Specialized Knowledge Tables

Unlike `CLAUDE.md` which saturates a single file with context, the DCP separates knowledge by purpose:

### 1. Symbols (`symbols`)
The "OpenCode" map of your project.
- **Fields:** Name, Kind (func/struct), File Path, Line Range, Purpose, Signature.
- **Usage:** Used by the `read_symbol` tool to inject surgical snippets into the LLM context.

### 2. Environments (`environments`)
Hard facts about where and how your code runs.
- **Fields:** Env Name, Runtime Type (k8s/docker), Auth Steps, Topology (IPs/Services).
- **Usage:** Used by the `deploy` tool to execute precise ops commands without guessing IPs.

### 3. API Contracts (`api_docs`)
The source of truth for your interfaces.
- **Fields:** Path, Method, Request/Response Schemas, Description.
- **Usage:** Automatically updated by the Indexer to keep documentation in sync with code handlers.

---

## Knowledge Hub UI

The **Knowledge Hub** is the visual dashboard for your project's deterministic brain. It is accessible via the OpenExec Web UI and provides real-time visibility into the DCP records.

### Visual Audit
- **Symbols Dashboard:** Search and view all indexed function pointers and their recorded purposes.
- **Topology View:** Verify the IP addresses and service mappings for your environments before a deployment.
- **Policy Registry:** Audit the active safety gates that are currently protecting your codebase.

---

## The BitNet Intent Router

The DCP includes an optional **Local 1-bit LLM (BitNet b1.58 2B)** wrapper managed natively within the binary.

### Self-Contained Inference
OpenExec includes an internal **Inference Manager** that manages the local 1-bit model.
1.  **Direct Execution:** The system parses intents locally in milliseconds.
2.  **Tool Selection:** BitNet surgically selects the correct tool (e.g., `read_symbol`) based on your natural language.
3.  **Context Efficiency:** The expensive cloud LLM only receives the exact deterministic records fetched by the local tools.
4.  The expensive primary LLM (Claude/GPT) receives only the **exact context** it needs.

### Model Auto-Download
The BitNet model **auto-downloads on first use** to `~/.openexec/models/`. There is no need to manually install or download the model. Any GGUF model can be used, but the routing prompt is tuned for the default model. If the model is unavailable, the system falls back to deterministic routing.

This "Surgical Context" approach reduces token usage by up to 90% compared to full-file reading.

---

## Autonomous Compliance Shield

The **Compliance Shield** is a hard gate that prevents unverified code from entering your repository.

### The `safe_commit` Tool
Instead of using raw `git commit`, the system uses the `safe_commit` tool. It automatically:
1.  **Detects Environment**: Identifies Go or Python source code.
2.  **Runs Mandatory Gates**: Executes static analysis (`go vet`, `ruff`, `mypy`).
3.  **Blocks on Failure**: If any gate fails, the commit is aborted, and the AI is provided with the exact error to fix.

This ensures that every single commit in your project history is verified and compliant with organizational standards.
