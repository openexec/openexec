# ADR-014: Budgets, Cost Control & OpenTelemetry Spans

*   **ID:** ADR-014  
*   **Status History:**
    *   2026-06-06: Created by Systems Architect. Status: Approved. Implementation: Shipped (internal/budget & pkg/telemetry).
*   **Decides on:** Financial Guardrails and LLM Observability

---

## 1. Context and Problem Statement
Autonomous AI agent runs can easily consume thousands of API tokens in minutes if they get caught in an infinite retry loop or are tasked with analyzing massive code files. Without strict, real-time financial tracking and execution observability, organizations face massive, un-audited cloud API bills.

## 2. Facing (Decision Drivers)
*   Prevent "token runaway" loops.
*   Provide real-time financial transparency on every agent run.
*   Integrate with industry-standard observability platforms (OpenTelemetry).

## 3. Decided (The Architectural Decision)
We establish a native **Financial Budget Monitor** and **OpenTelemetry Span Tracking** pipeline:
1.  **Real-Time Token Accounting:** Every outbound LLM API call is monitored by `internal/budget/`. It records exact input/output token counts and translates them to dollar values based on active provider pricing tables.
2.  **Hard Cost Limits:** Users configure a hard maximum run cost in `openexec.yaml` (e.g. `max_run_budget_usd: 5.00`). If an active run crosses this budget, the Go pipeline **instantly terminates the LLM session** and marks the run as `paused_budget_exhausted`.
3.  **OTel Gen-AI Spans:** All agent actions, tool calls, and LLM completions are wrapped in standard OpenTelemetry Spans (`pkg/telemetry`). They are exported to Prometheus/Jaeger/OTel collectors, allowing SREs to monitor AI execution traces using their existing enterprise monitoring stack.

## 4. Rejected/Neglected Alternatives
*   *Post-Facto Cost Auditing:* Rejected. Analyzing costs *after* a run completes is too late—the money is already spent. Limits must be enforced programmatically at runtime.

## 5. Consequences & Tradeoffs
*   *Positives:* Guaranteed protection against financial runaway; native integration with corporate APM platforms.
*   *Negatives:* Requires constant updating of Go-based model pricing tables as providers update their API costs.

## 6. Dependencies & References
*   Configured inside `openexec.yaml` (see `docs/CONFIGURATION.md`).
*   Implemented in `internal/budget/` and `pkg/telemetry/`.
