---
slug: /architecture
title: Architectural Roadmap
sidebar_position: 1
---

# OpenExec Architectural Roadmap

This directory contains the foundational specifications and architectural decisions for OpenExec. These documents serve as the authoritative roadmap for the project's transition from a feature-rich prototype to a disciplined, deterministic AI coding runtime.

## 🏛 Architecture Decision Records (ADR) Registry
OpenExec strictly enforces **ADR-Driven Development**. Every major architectural modification, design choice, or subsystem integration must be documented as an ADR here before implementation begins. 

*   [**ADR Template**](./ADR_TEMPLATE.md) — Standardized template for authoring new Architecture Decision Records.
*   [**ADR-001: Core Runtime Consolidation**](./ADR-001-CORE-RUNTIME.md) — The official mandate for core runtime consolidation and SQLite migration.
*   [**ADR-002: Toolset Filtering & Sandboxing**](./ADR-002-TOOLSET-FILTERING.md) — Strict security bounding and capabilities-based access control.
*   [**ADR-003: Pattern Confidence Gates**](./ADR-003-PATTERN-CONFIDENCE.md) — Deterministic evaluation criteria for semantic matches.
*   [**ADR-004: Deterministic Orchestration vs. Vibe Coding**](./ADR-004-DETERMINISTIC-ORCHESTRATION.md) — Why state machines and strict gates replace open-ended prompting.
*   [**ADR-005: Intent Decomposition & Task Pipeline**](./ADR-005-INTENT-DECOMPOSITION.md) — Breaking intents into stories, tasks, branch isolation, and the coding loop.
*   [**ADR-006: Multi-Provider Routing & Pointer Records**](./ADR-006-MULTI-PROVIDER-ROUTING.md) — Agnostic HTTP API adapters (Claude/Codex/Gemini/OpenAI) and AST-based context extraction.
*   [**ADR-007: Local LLM Intent & Tool Selection**](./ADR-007-LOCAL-LLM-TOOL-SELECTION.md) — Using a frontline local/1-bit classifier (DCP) to assign safe toolsets.
*   [**ADR-008: PII Data Scrubbing & Privacy Shield**](./ADR-008-PII-DATA-SCRUBBING.md) — Local sanitization of sensitive information before outbound API transmission.
*   [**ADR-009: Adversarial Code Review Agent**](./ADR-009-CODE-REVIEW-AGENT.md) — Separating implementation and review roles to prevent AI cognitive bias.

## 1. Vision & Strategy
High-level conceptual models and the strategic differentiation of OpenExec.

*   [**The One Diagram**](./THE-ONE-DIAGRAM.md) — The fundamental mental model: Runtime governs LLM, not the other way around.
*   [**The 80% Trap**](./THE-80-PERCENT-TRAP.md) — Instruction set for bridging the gap from intent to execution.
*   [**Strategic Advantages**](./STRATEGIC-ADVANTAGES.md) — Why OpenExec's deterministic approach is superior for regulated and enterprise environments.

*   [**Architecture Decision Memo (ADR-001)**](/architecture/ADR-001-CORE-RUNTIME) — The official mandate for core runtime consolidation and SQLite migration.

## 2. Core Architecture v1.0
The technical blueprint for the unified Go runtime.

*   [**Runtime Architecture v1.0**](/architecture/RUNTIME-ARCHITECTURE-V1) — Detailed specification of the 7-layer converged architecture.
*   [**Runtime Package Layout**](/architecture/RUNTIME-PACKAGE-LAYOUT) — Modular Go package structure (Kernel + Subsystems).
*   [**Runtime Loop Reference**](/architecture/RUNTIME-LOOP-REFERENCE) — Pseudo-code implementation of the tiny core orchestration kernel.

## 3. Technical Specifications
Detailed contracts for the runtime subsystems.

*   [**Blueprint DSL v1**](/architecture/BLUEPRINT-DSL-V1) — YAML specification for defining deterministic and agentic workflows.
*   [**Tool Action Contract v1**](/architecture/TOOL-ACTION-CONTRACT-V1) — The typed interface for deterministic runtime actions (apply_patch, run_tests, etc.).
*   [**Policy & Sandbox Contract v1**](/architecture/POLICY-SANDBOX-CONTRACT-V1) — Capability-based access control and runtime safety enforcement.
*   [**Run Timeline & Replay System**](/architecture/TIMELINE-REPLAY-SYSTEM) — Event-driven observability and deterministic run reconstruction.
*   [**Operational Memory Layer**](/architecture/OPERATIONAL-MEMORY-LAYER) — Pointer-record architecture for deterministic system knowledge.

## 4. Roadmap & Execution
The tactical plan for simplifying and stabilizing the codebase.

*   [**V1 Simplification Pass**](/architecture/SIMPLIFICATION-PASS) — The "7 Deletions" required to eliminate architecture bloat.
*   [**V1 Cut List & Migration Board**](/architecture/V1-CUT-LIST-MIGRATION-BOARD) — Status tracking for component removal, migration, and stabilization.

## 5. Research & Future
*   [**Self-Healing & Self-Upgrade**](./SELF-HEALING-UPGRADE.md) — Future architecture for autonomous runtime diagnosis and repair.
*   [**Runtime Evolution Interface (REI)**](./RUNTIME-EVOLUTION-INTERFACE.md) — Boundary between the active runtime and evolution workflows.
*   [**Elixir/BEAM Orchestrator**](./ELIXIR_BEAM_ORCHESTRATOR.md) — Research on high-scale orchestration using the BEAM virtual machine.


---
*Last Updated: March 14, 2026*
