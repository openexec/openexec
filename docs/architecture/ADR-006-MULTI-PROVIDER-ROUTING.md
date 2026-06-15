# ADR-006: Multi-Provider API Support & Pointer Records

*   **ID:** ADR-006
*   **Status:** Approved
*   **Author:** Systems Architect
*   **Date:** 2026-06-06
*   **Decides on:** Model Routing and Knowledge Extraction

---

## 1. Context and Problem Statement
Relying on a single AI provider (e.g., Anthropic Claude Code CLI) locks the platform into their pricing model, rate limits, and output formats. Furthermore, passing an entire project codebase into the context window for every request costs dollars per query and degrades the LLM's accuracy ("Needle-in-a-haystack" degradation).

## 2. Decision Drivers
*   The orchestrator must support heterogeneous environments (OpenAI, Claude, Gemini, DeepSeek, local Ollama).
*   Context windows must remain lean to preserve reasoning quality and reduce costs.

## 3. Architectural Decision (The Chosen Path)
We implement two fundamental architecture pillars: **Agnostic HTTP API Adapters** and **Pointer Records**.

### Part A: Multi-Provider API Compatibility
OpenExec drops hardcoded CLI dependencies in favor of a unified HTTP/JSON adapter layer (`pkg/agent`).
*   It supports standard OpenAI-compatible endpoints directly.
*   Users can route requests to Claude, Codex, Gemini, or self-hosted models (like Qwen or LLaMA) simply by changing the `api_base_url` and `api_provider` in `openexec.yaml`.

### Part B: Pointer Records (Deterministic Knowledge Base)
Instead of relying on fuzzy RAG (Vector Embeddings) or full-file dumping, we utilize a **Symbol-Weighted Pointer Record** system backed by SQLite.
*   **Indexing:** OpenExec parses local files into an Abstract Syntax Tree (AST), recording the exact line numbers and file paths of every function, class, and interface into SQLite.
*   **Surgical Context:** When the AI needs to modify `BillingController`, OpenExec uses BM25/Symbol queries to find exactly where `BillingController` is defined, and only extracts and injects those specific lines of code into the prompt.

### Why this option wins:
Decoupling the execution pipeline from the underlying LLM provider ensures future-proof survivability. Coupling this with Pointer Records guarantees that no matter which API provider is used, the model receives highly compressed, surgically accurate code context, yielding 70-95% token savings.
