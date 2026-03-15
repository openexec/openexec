# OpenExec Architecture Decision Memo (ADR-001)

**Status:** Approved Direction (Implementation in Progress)  
**Date:** March 14, 2026  
**Subject:** Core Runtime Consolidation and Deterministic Orchestration

---

## Purpose
Establish a clear architectural direction for OpenExec so development remains focused on a minimal, deterministic, local-first AI coding runtime rather than drifting into multiple overlapping agent systems.

## 1. Architectural Vision
OpenExec is a **local-first deterministic AI coding runtime** with explicit orchestration and transparent execution. It prioritizes:
*   Local control and privacy-aware context handling.
*   Deterministic orchestration via Blueprints.
*   Explicit runtime state (SQLite).
*   Minimal prompt reliance.
*   Curated tool access (Toolsets).
*   Replayable execution.

OpenExec does not compete on raw agent autonomy, but on **transparency, auditability, and local safety enforcement.** This differentiates it from tools like Claude Code or Cursor, which rely on more opaque, model-driven behavior.

## 2. Core Architectural Model
All system behavior revolves around a single canonical runtime state model:

**Session** → **Task** → **Run** → **Step** → **ToolCall** → **Artifact**

*   **Session:** User interaction context (CLI/IDE).
*   **Task:** Structured user intent extracted from conversation.
*   **Run:** Execution of a task via a blueprint.
*   **Step:** One stage of the blueprint.
*   **ToolCall:** Invocation of system capabilities.
*   **Artifact:** Persistent outputs (patches, logs, diffs, summaries).

## 3. Source of Truth
The **SQLite state store** is the only source of truth. The schema includes sessions, tasks, runs, steps, tool_calls, artifacts, and checkpoints. 

:::important
This replaces legacy JSON artifacts (`stories.json`, `tasks.json`). These files must be removed once migration is complete.
:::

## 4. Execution Model: Blueprints
OpenExec execution is driven by **Blueprints**: deterministic workflows combining deterministic stages (code-based) and agent stages (AI-based).

**Standard Blueprint Example:**
`gather_context` (Det) → `implement` (Agent) → `lint` (Det) → `fix_lint` (Agent) → `test` (Det) → `fix_tests` (Agent) → `review` (Agent).

The **runtime controls transitions**, not the model.

## 5. Responsibilities
*   **Blueprint Engine owns:** Stage ordering, retry limits, failure routing, checkpointing, artifact creation, and tool permissions.
*   **AI Agents are limited to:** Implementation reasoning, diagnosis, summarization, and patch generation. Agents never control the workflow.

## 6. Retry and Loop Policy
All loops must have explicit caps (e.g., `implement_retries = 2`, `test_fix_retries = 2`). These caps prevent runaway agent loops and unexpected token spend.

## 7. Tool Architecture (Toolsets)
Tools are never exposed individually by default. Instead, the runtime assigns **Toolsets** (e.g., `coding_backend`, `repo_readonly`, `docs_lookup`). Each toolset contains curated tools, improving reliability and reducing tool confusion for the model.

## 8. Local LLM Gatekeeper
OpenExec includes a small local model used only for routing and filtering:
*   Intent classification (Task vs. Chat).
*   Toolset and Knowledge source selection.
*   Context ranking and Sensitivity detection (Redaction).
*   **The local model does not orchestrate workflows.**

## 9. Context Assembly
Context is assembled locally before model calls. Process: 
**Deterministic Retrieval** → **Local LLM Ranking** → **Sanitized Context Pack** → **Frontier Model**.
This reduces token usage and prevents data leakage.

## 10. Safety and Policy Layer
The runtime enforces path access, tool permissions, and sandbox modes. These rules live in **runtime code/config**, not in agent prompts.

## 11. Conversation Model
Conversation is not a separate system. Chat interactions exist within the Session model.
**Transitions:** `chat` → `task` → `run`. 
Chat only becomes execution when a Task artifact is created.

## 12. Observability and Replay
Every run must be replayable. The system records prompts, tool calls, artifacts, and stage transitions. A run timeline must allow debugging of *why* a step occurred and *which* policy was triggered.

## 13. Decision Summary

| Action | Items |
| :--- | :--- |
| **Keep** | Local-first runtime, Blueprint orchestration, Unified SQLite, Toolsets, Local Router. |
| **Remove** | Legacy 5-phase pipeline, `stories.json`, `tasks.json`, prompt-driven transitions. |
| **Migrate** | CLI entrypoints to Run model, legacy context builders to unified system. |
| **Defer** | Multi-agent swarms, distributed workers, large MCP tool ecosystems. |

## 14. Competitive Positioning
OpenExec differentiates by being suitable for **regulated environments, air-gapped systems, and institutional governance.** It provides reproducible AI-assisted development that typical "opaque" coding agents cannot match.

---
**Final Direction:** OpenExec is a deterministic local AI coding runtime with explicit orchestration, not a generalized autonomous agent swarm. Engineering discipline must focus on removing legacy coexistence quickly once the new runtime is stable.
