# ADR-011: MCP as the Sole Execution Plane

*   **ID:** ADR-011  
*   **Status History:**
    *   2026-06-06: Created by Systems Architect. Status: Approved. Implementation: Shipped (Server + mcp-serve cli).
*   **Decides on:** The Client-Server Execution Interface

---

## 1. Context and Problem Statement
Early prototypes of OpenExec interlined prompt assembly, file editing, and command execution in a single, local monolithic loop. This created severe tight-coupling, making it impossible to delegate execution to external engines or allow lightweight developer interaction without booting a heavy, long-running Go server.

## 2. Facing (Decision Drivers)
*   Decouple orchestration logic from actual file/tool execution.
*   Support industry-standard agent protocols (Model Context Protocol).
*   Support two-speed loops (fast local developer CLI vs. heavy background pipeline).

## 3. Decided (The Architectural Decision)
We establish the **Model Context Protocol (MCP)** as the *sole, unified execution interface* for OpenExec.
*   The Go core runs a background daemon hosting an MCP JSON-RPC server (`internal/mcp/server.go`).
*   All operations—reading files, writing files, applying git patches, executing whitelisted shell commands, and signalling status—are modeled as strictly typed MCP tools.
*   **Two-Speed Seam:** We expose `openexec mcp-serve` over standard I/O (stdio). External lightweight CLI clients (like Claude Code) can plug directly into this stdio stream, utilizing OpenExec's entire SRE backlog and operational memories without booting background network servers.

## 4. Rejected/Neglected Alternatives
*   *Custom JSON-RPC REST API:* Rejected. Custom REST APIs require clients to implement bespoke transport adapters, creating high adoption friction compared to standard MCP.

## 5. Consequences & Tradeoffs
*   *Positives:* Native, out-of-the-box compatibility with Claude Code, Cursor, and standard MCP clients; perfect process-isolation between client and server.
*   *Negatives:* Minor serialization overhead due to JSON-RPC over stdio/HTTP.

## 6. Dependencies & References
*   Referenced in `docs/LIGHT_MODE.md`.
*   Implemented in `internal/mcp/` and `internal/cli/mcp_serve.go`.
