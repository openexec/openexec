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

## The BitNet Intent Router

The DCP includes a **Local 1-bit LLM (BitNet b1.58 2B)** wrapper. When you interact with OpenExec via the UI or CLI chat:
1.  Your query is parsed **locally** by BitNet.
2.  BitNet selects the correct **Surgical Tool** (e.g., `read_symbol`).
3.  The tool fetches the **Deterministic Record** from SQLite.
4.  The expensive primary LLM (Claude/GPT) receives only the **exact context** it needs.

This "Surgical Context" approach reduces token usage by up to 90% compared to full-file reading.

---

## Tool Creation Standard

To add a new tool to the DCP, implement the `tools.Tool` interface in `internal/tools/`. Every tool must:
1.  Define a JSON-RPC compatible `InputSchema`.
2.  Query the `knowledge.Store` for deterministic records.
3.  Register itself with the `Coordinator` in `server.go`.
