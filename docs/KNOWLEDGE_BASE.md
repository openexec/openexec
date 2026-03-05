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

The DCP includes a **Local 1-bit LLM (BitNet b1.58 2B)** wrapper managed by the **Local Inference Manager**.

### Self-Contained Inference
The system no longer requires manual installation of external AI tools. The Inference Manager:
1.  **Auto-Locates Engine:** Searches for `bitnet-cli` in local `./bin` folders and user paths.
2.  **Environment Awareness:** Verifies model availability before attempting surgical tool selection.
3.  **Local Latency:** Your query is parsed **locally**, selecting the correct **Surgical Tool** (e.g., `read_symbol`) in milliseconds.
3.  The tool fetches the **Deterministic Record** from SQLite.
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

## Tool Creation Standard

To add a new tool to the DCP, implement the `tools.Tool` interface in `internal/tools/`. Every tool must:
1.  Define a JSON-RPC compatible `InputSchema`.
2.  Query the `knowledge.Store` for deterministic records.
3.  Register itself with the `Coordinator` in `server.go`.
