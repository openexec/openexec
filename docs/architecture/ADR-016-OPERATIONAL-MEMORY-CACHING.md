# ADR-016: Operational Memory, Predictive Loading & Caching

*   **ID:** ADR-016  
*   **Status History:**
    *   2026-06-06: Created by Systems Architect. Status: Approved. Implementation: Shipped (Pointer Records) & Partial (Predictive Loader).
*   **Decides on:** Context Assembly, Caching, and Pre-loading

---

## 1. Context and Problem Statement
Retrieving codebase context during multi-stage runs is the most expensive phase of AI execution. If an agent has to wait for disk scans, AST parsing, and symbol-weight calculations on every single step, the local CLI inner loop feels slow, heavy, and unresponsive.

## 2. Facing (Decision Drivers)
*   Minimize context assembly latency (keep under 50ms).
*   Reduce token consumption by caching parsed structures.
*   Enable background "predictive pre-loading" of context while the user is typing.

## 3. Decided (The Architectural Decision)
We implement a stateful, SQLite-backed **Operational Memory & Predictive Loading** architecture:
1.  **AST Pointer Record Cache:** When OpenExec is initialized, it parses the codebase and stores symbol pointer records (file paths, line numbers, function signatures) in the canonical `openexec.db`. On subsequent runs, it only rescans files modified in the Git index, reducing index latency to <5ms.
2.  **Predictive Loading Store (`internal/predictive/`):** Predictive metadata, file access historical patterns, and predicted files are stored in a dedicated database at `.openexec/predictive.db`.
3.  **Callable `PredictAndLoad` Framework:** The system exposes a synchronous `PredictAndLoad` function which takes the active task parameters, predicts symbol dependencies, pre-loads the predicted files into an **in-memory file cache**, and serves them instantly to the context assembler, keeping context load times under 50ms.
4.  **Keystroke-Watching Daemon & Cloud Prompt Caching (Aspirational / Future):** We plan to build a background watcher daemon that monitors active keystrokes in the Web Console to pre-emptively load symbols before the user hits "Run", and integrate cloud-provider-level prompt caching (e.g. Anthropic Prompt Caching) once direct API integrations are completed in production loops.

## 4. Rejected/Neglected Alternatives
*   *On-Demand Scanning:* Rejected. Rescanning the filesystem and rebuilding AST trees on every single agent step introduces massive I/O bottlenecks on large codebases.
*   *Unified Database for Predictions:* Rejected. Separating predictions to `.openexec/predictive.db` prevents high-frequency read/write access logs of file-access statistics from conflicting with transaction locks on the core progress schemas in `openexec.db`.

## 5. Consequences & Tradeoffs
*   *Positives:* Ultra-responsive, sub-50ms context assembly; dramatic cloud API cost savings due to prompt cache hits.
*   *Negatives:* Consumes local disk space for the SQLite symbol index; requires background file-system watchdogs.

## 6. Dependencies & References
*   Detailed in `docs/CONTEXT_PRUNING.md` and `docs/KNOWLEDGE_BASE.md`.
*   Implemented in `internal/context/` and `internal/predictive/`.
