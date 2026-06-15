# ADR-007: Local LLM Tool Selection & Intent Routing

*   **ID:** ADR-007
*   **Status:** Approved
*   **Author:** Systems Architect
*   **Date:** 2026-06-06
*   **Decides on:** The Frontline Intent Classifier (DCP / BitNet)

---

## 1. Context and Problem Statement
Triggering a heavy, expensive cloud model (like GPT-4o or Claude 3.5 Sonnet) merely to determine *what* the user wants or *which* toolset to load is highly inefficient. Simple classification tasks (e.g., "Is this a chat request or a file-editing request?") take too long and cost too much when sent over the network.

## 2. Decision Drivers
*   Minimize API token burn on trivial classification tasks.
*   Reduce input latency for the developer's "inner loop" (chat/query operations).
*   Enforce strict capability boundaries (do not load code-editing tools for a read-only question).

## 3. Architectural Decision (The Chosen Path)
We introduce the **Deterministic Control Plane (DCP) with Local BitNet Routing**.

### The Flow:
1.  **Frontline Classification:** When a user prompt enters OpenExec, it is intercepted by a local, ultra-fast classifier (either a deterministic heuristic engine in `internal/dcp/selector.go` or an optional local 1-bit LLM like a quantized BitNet model).
2.  **Intent Mapping:** The local engine classifies the intent:
    *   *Intent: Read-only Question* $\rightarrow$ Assigns `repo_readonly` toolset.
    *   *Intent: Infrastructure Deployment* $\rightarrow$ Assigns `sre_orchestration` toolset.
3.  **Cloud Handoff:** Only *after* the intent is classified and the safe toolset is compiled does the system invoke the heavy cloud LLM API, ensuring the model only receives the exact tools required to fulfill the goal.

### Why this option wins:
Using a local model or fast heuristic engine as the "router" establishes a cost-free, zero-latency security gate. It physically prevents the heavy implementation agent from ever receiving destructive or unrelated tools, effectively bounding the action space before the cloud prompt is even assembled.
