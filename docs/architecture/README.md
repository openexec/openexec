# OpenExec Architectural Roadmap

This directory contains the foundational specifications and architectural decisions for OpenExec. These documents serve as the authoritative roadmap for the project's transition from a feature-rich prototype to a disciplined, deterministic AI coding runtime.

## 1. Vision & Strategy
High-level conceptual models and the strategic differentiation of OpenExec.

*   [**The One Diagram**](./THE-ONE-DIAGRAM.md) — The fundamental mental model: Runtime governs LLM, not the other way around.
*   [**Strategic Advantages**](./STRATEGIC-ADVANTAGES.md) — Why OpenExec's deterministic approach is superior for regulated and enterprise environments.
*   [**Architecture Decision Memo (ADR-001)**](./ADR-001-CORE-RUNTIME.md) — The official mandate for core runtime consolidation and SQLite migration.

## 2. Core Architecture v1.0
The technical blueprint for the unified Go runtime.

*   [**Runtime Architecture v1.0**](./RUNTIME-ARCHITECTURE-V1.md) — Detailed specification of the 7-layer converged architecture.
*   [**Runtime Package Layout**](./RUNTIME-PACKAGE-LAYOUT.md) — Modular Go package structure (Kernel + Subsystems).
*   [**Runtime Loop Reference**](./RUNTIME-LOOP-REFERENCE.md) — Pseudo-code implementation of the tiny core orchestration kernel.

## 3. Technical Specifications
Detailed contracts for the runtime subsystems.

*   [**Blueprint DSL v1**](./BLUEPRINT-DSL-V1.md) — YAML specification for defining deterministic and agentic workflows.
*   [**Tool Action Contract v1**](./TOOL-ACTION-CONTRACT-V1.md) — The typed interface for deterministic runtime actions (apply_patch, run_tests, etc.).
*   [**Policy & Sandbox Contract v1**](./POLICY-SANDBOX-CONTRACT-V1.md) — Capability-based access control and runtime safety enforcement.
*   [**Run Timeline & Replay System**](./TIMELINE-REPLAY-SYSTEM.md) — Event-driven observability and deterministic run reconstruction.
*   [**Operational Memory Layer**](./OPERATIONAL-MEMORY-LAYER.md) — Pointer-record architecture for deterministic system knowledge.

## 4. Roadmap & Execution
The tactical plan for simplifying and stabilizing the codebase.

*   [**V1 Simplification Pass**](./SIMPLIFICATION-PASS.md) — The "7 Deletions" required to eliminate architecture bloat.
*   [**V1 Cut List & Migration Board**](./V1-CUT-LIST-MIGRATION-BOARD.md) — Status tracking for component removal, migration, and stabilization.

## 5. Research & Future
*   [**Elixir/BEAM Orchestrator**](./ELIXIR_BEAM_ORCHESTRATOR.md) — Research on high-scale orchestration using the BEAM virtual machine.

---
*Last Updated: March 14, 2026*
