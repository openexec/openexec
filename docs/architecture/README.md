---
slug: /architecture
title: Architectural Roadmap
sidebar_position: 1
---

# OpenExec Architectural Roadmap

This directory contains the foundational specifications and architectural decisions for OpenExec. These documents serve as the authoritative roadmap for the project's transition from a feature-rich prototype to a disciplined, deterministic AI coding runtime.

## 🏛 Architecture Decision Records (ADR) Registry
OpenExec strictly enforces **ADR-Driven Development** (inspired by the decentralized, verified pattern in Unsorry). Every major architectural modification, design choice, or subsystem integration must be documented as an ADR here before implementation begins.

### Complete ADR Board
*   [**ADR Template**](./ADR_TEMPLATE.md) — Standardized template for authoring new Architecture Decision Records.
*   [**ADR-001: Core Runtime Consolidation**](./ADR-001-CORE-RUNTIME.md)  
    *Status:* Approved | *Implementation:* Shipped  
    *Summary:* Migration from fragmented JSON files and `go-sqlite3` to CGO-free `modernc.org/sqlite` as the single canonical state store.
*   [**ADR-002: Toolset Filtering & Sandboxing**](./ADR-002-TOOLSET-FILTERING.md)  
    *Status:* Approved | *Implementation:* Shipped  
    *Summary:* Capability-based tool grouping and security bounding inside the execution broker.
*   [**ADR-003: Pattern Confidence Gates**](./ADR-003-PATTERN-CONFIDENCE.md)  
    *Status:* Deferred | *Implementation:* Aspirational  
    *Summary:* Proposed metadata-driven evaluation framework for checking code pattern stability.
*   [**ADR-004: Deterministic Orchestration vs. Vibe Coding**](./ADR-004-DETERMINISTIC-ORCHESTRATION.md)  
    *Status:* Approved | *Implementation:* Shipped  
    *Summary:* Replaces open-ended conversational REPLs with a strict 5-stage state-machine pipeline.
*   [**ADR-005: Intent Decomposition & Task Pipeline**](./ADR-005-INTENT-DECOMPOSITION.md)  
    *Status:* Approved | *Implementation:* Shipped  
    *Summary:* Breaks human intent into Blueprints, Stories, and branch-isolated Tasks.
*   [**ADR-006: Multi-Provider Routing & Pointer Records**](./ADR-006-MULTI-PROVIDER-ROUTING.md)  
    *Status:* Approved | *Implementation:* Shipped  
    *Summary:* Agnostic HTTP API adapters (Claude/Gemini/Ollama) and AST-based context extraction.
*   [**ADR-007: Local LLM Intent & Tool Selection**](./ADR-007-LOCAL-LLM-TOOL-SELECTION.md)  
    *Status:* Approved | *Implementation:* Shipped  
    *Summary:* Uses a frontline local/1-bit classifier (DCP) to assign safe toolsets before cloud dispatch.
*   [**ADR-008: PII Data Scrubbing & Privacy Shield**](./ADR-008-PII-DATA-SCRUBBING.md)  
    *Status:* Approved | *Implementation:* Shipped  
    *Summary:* Local sanitization and masking of emails, API keys, HETUs, and IPs before outbound API transmission.
*   [**ADR-009: Adversarial Code Review Agent**](./ADR-009-CODE-REVIEW-AGENT.md)  
    *Status:* Approved | *Implementation:* Shipped  
    *Summary:* Separates implementation and review roles to prevent AI cognitive bias.
*   [**ADR-010: SRE & Infrastructure Safety Gating**](./ADR-010-SRE-INFRASTRUCTURE-SAFETY.md)  
    *Status:* Approved | *Implementation:* Shipped & Partial  
    *Summary:* Restricts the AI action space by omitting destructive commands and parsing Terraform/Ansible plan JSONs deterministically.
*   [**ADR-011: MCP as the Sole Execution Plane**](./ADR-011-MCP-EXECUTION-PLANE.md)  
    *Status:* Approved | *Implementation:* Shipped  
    *Summary:* Outlines the Model Context Protocol (MCP) as the unified client-server and stdio backlog interface.
*   [**ADR-012: Risk-Tiered Approvals & Fail-Closed Human Gates**](./ADR-012-APPROVAL-HUMAN-GATE.md)  
    *Status:* Approved | *Implementation:* Shipped & Partial  
    *Summary:* Implements risk-tiered gates (RequiresApproval=true) for mutating production operations, preventing self-approval.
*   [**ADR-013: Conversational Sessions & Web Console State**](./ADR-013-CONVERSATIONAL-WORKFLOWS.md)  
    *Status:* Approved | *Implementation:* Shipped  
    *Summary:* Maps chat sessions, visual diffs, and rollback clicks in `openexec-web` to isolated Git branches.
*   [**ADR-014: Budgets, Cost Control & OpenTelemetry Spans**](./ADR-014-BUDGET-OTEL-TELEMETRY.md)  
    *Status:* Approved | *Implementation:* Shipped  
    *Summary:* Prevents financial runaway via hard cost limits and exports execution traces using OTel Gen-AI Spans.
*   [**ADR-015: Skills & Reusable Agent Libraries**](./ADR-015-SKILLS-TRUST-BOUNDARIES.md)  
    *Status:* Approved | *Implementation:* Shipped  
    *Summary:* Specifies the three-tier instruction precedence cascade (Project > Folder > Global) for `SKILL.md` collections.
*   [**ADR-016: Operational Memory, Predictive Loading & Caching**](./ADR-016-OPERATIONAL-MEMORY-CACHING.md)  
    *Status:* Approved | *Implementation:* Shipped & Partial  
    *Summary:* Establishes symbol-record caching in SQLite and background pre-loading of files to minimize latency.
*   [**ADR-017: Worktree and Branch Isolation**](./ADR-017-WORKTREE-BRANCH-ISOLATION.md)  
    *Status:* Approved | *Implementation:* Shipped & Partial  
    *Summary:* Guarantees developer safety by spawning parallel background agents strictly inside isolated `git worktree` sandboxes.
*   [**ADR-018: Unified State Store & Schema Boundaries**](./ADR-018-UNIFIED-STATE-SCHEMA.md)  
    *Status:* Approved | *Implementation:* Shipped  
    *Summary:* Establishes `.openexec/openexec.db` as the transactional, single source of truth for runs, tasks, and audit logs.

---

## 1. Vision & Strategy
High-level conceptual models and the strategic differentiation of OpenExec.

*   [**The One Diagram**](./THE-ONE-DIAGRAM.md) — The fundamental mental model: Runtime governs LLM, not the other way around.
*   [**The 80% Trap**](./THE-80-PERCENT-TRAP.md) — Instruction set for bridging the gap from intent to execution.
*   [**Strategic Advantages**](./STRATEGIC-ADVANTAGES.md) — Why OpenExec's deterministic approach is superior for regulated and enterprise environments.

## 2. Core Architecture v1.0
The technical blueprint for the unified Go runtime.

*   [**Runtime Architecture v1.0**](./RUNTIME-ARCHITECTURE-V1.md) — Detailed specification of the 7-layer converged architecture.
*   [**Runtime Package Layout**](./RUNTIME-PACKAGE-LAYOUT.md) — Modular Go package structure (Kernel + Subsystems).
*   [**Runtime Loop Reference**](./RUNTIME-LOOP-REFERENCE.md) — Pseudo-code implementation of the tiny core orchestration kernel.

## 3. Technical Specifications
Detailed contracts for the runtime subsystems.

*   [**Blueprint DSL v1**](./BLUEPRINT-DSL-V1.md) — YAML specification for defining deterministic and agentic workflows.
*   [**Tool Action Contract v1**](./TOOL-ACTION-CONTRACT-V1.md) — The typed interface for deterministic runtime actions (apply_patch, run_tests, etc.).
*   [**Policy & Sandbox Contract v1**](../future/POLICY-SANDBOX-CONTRACT-V1.md) — Capability-based access control and runtime safety enforcement.
*   [**Run Timeline & Replay System**](../future/TIMELINE-REPLAY-SYSTEM.md) — Event-driven observability and deterministic run reconstruction.
*   [**Operational Memory Layer**](../future/OPERATIONAL-MEMORY-LAYER.md) — Pointer-record architecture for deterministic system knowledge.

## 4. Roadmap & Execution
The tactical plan for simplifying and stabilizing the codebase.

*   [**V1 Simplification Pass**](./SIMPLIFICATION-PASS.md) — The "7 Deletions" required to eliminate architecture bloat.
*   [**V1 Cut List & Migration Board**](./V1-CUT-LIST-MIGRATION-BOARD.md) — Status tracking for component removal, migration, and stabilization.

## 5. Research & Future
*   [**Self-Healing & Self-Upgrade**](./SELF-HEALING-UPGRADE.md) — Future architecture for autonomous runtime diagnosis and repair.
*   [**Runtime Evolution Interface (REI)**](./RUNTIME-EVOLUTION-INTERFACE.md) — Boundary between the active runtime and evolution workflows (copied from docs).
*   [**Elixir/BEAM Orchestrator**](./ELIXIR_BEAM_ORCHESTRATOR.md) — Research on high-scale, supervisor-tree concurrency using the BEAM virtual machine.

---
*Last Updated: 2026-06-06 — Registry Expanded to 18 Canonical ADRs*
