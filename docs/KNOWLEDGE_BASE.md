# OpenExec Knowledge Base: The Deterministic Control Plane (DCP)

This document explains the architecture and usage of OpenExec's **Deterministic Knowledge Base**, a purpose-driven relational system that replaces traditional Vector Databases for codebase management.

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

## CLI Management

Use the `openexec knowledge` command suite to manage your project's brain.

### Indexing your Code
To automatically populate symbol and API pointers:
```bash
openexec knowledge index .
```
This scans your `.go` files (and other supported languages) and records every function's purpose and location.

### Inspecting Records
See what the system currently "knows":
```bash
# Show all code functions and their purposes
openexec knowledge show symbols

# Show environment topologies (IPs, clusters)
openexec knowledge show envs

# Show detected API endpoints
openexec knowledge show api
```

### Multi-Project Management
List all projects on your system that have a DCP Knowledge Base:
```bash
openexec knowledge ls
```

---

## Knowledge Hub UI

The **Knowledge Hub** is the visual dashboard for your project's deterministic brain. It is accessible via the OpenExec Web UI and provides real-time visibility into the DCP records.

### Visual Audit
- **Symbols Dashboard:** Search and view all indexed function pointers and their recorded purposes.
- **Topology View:** Verify the IP addresses and service mappings for your environments before a deployment.
- **Policy Registry:** Audit the active safety gates that are currently protecting your codebase.

---

## The BitNet Intent Router

The DCP includes a **Local 1-bit LLM (BitNet b1.58 2B)** wrapper managed natively within the binary.

### Self-Contained Inference
OpenExec includes an internal **Inference Manager** that manages the local 1-bit model.
1.  **Direct Execution:** The system parses intents locally in milliseconds.
2.  **Tool Selection:** BitNet surgically selects the correct tool (e.g., `read_symbol`) based on your natural language.
3.  **Context Efficiency:** The expensive cloud LLM only receives the exact deterministic records fetched by the local tools.
4.  The expensive primary LLM (Claude/GPT) receives only the **exact context** it needs.

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

---

## GDPR Compliance & PII Shield

OpenExec provides a native way to address **GDPR** and **PII (Personally Identifiable Information)** concerns before any data reaches a cloud API.

### 1. Privacy-First Filtering
The **Local Tool Search (RAG for Tools)** already acts as a metadata shield. By selecting only the necessary tool definitions locally, we prevent the exposure of your full system architecture to external providers.

### 2. Creating Custom Privacy Tools
Users can create their own "Privacy Shields" by implementing the `tools.Tool` interface. 
- **The `pii_scrubber` Tool:** A local tool that scans user queries or project files for patterns (emails, phone numbers, IDs) and replaces them with placeholders *before* the data is sent to the cloud LLM.
- **The `gdpr_gate`:** A policy gate that blocks any commit or deployment that contains identifiable information in unencrypted fields.

### 3. User-Defined Rulesets
OpenExec allows you to build your own rulesets directly in the **Local Knowledge Map**.
*   **YAML Guardrails:** Define PII patterns in your `openexec.yaml`.
*   **Deterministic Blocking:** If the local BitNet router detects a high probability of sensitive data in a query, it can trigger a warning or automatically scrub the information.

**By handling privacy locally, OpenExec ensures that "Need to Know" is the default state for all external communication.**
