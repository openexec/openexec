# ADR-008: PII Data Scrubbing & Privacy Shield

*   **ID:** ADR-008
*   **Status:** Approved
*   **Author:** Systems Architect
*   **Date:** 2026-06-06
*   **Decides on:** Data Sanitization before Cloud API Transmission

---

## 1. Context and Problem Statement
When orchestrating AI agents in enterprise or public-sector environments, passing log files, database dumps, or raw source code into external cloud LLMs (OpenAI, Anthropic) introduces massive GDPR and compliance risks. Codebases frequently contain inadvertently committed API keys, developer emails, server IP addresses, or sensitive customer identifiers (e.g., Finnish HETU codes) hidden in test fixtures.

## 2. Decision Drivers
*   OpenExec must be safe to deploy in highly regulated data environments.
*   Data redaction must happen **locally**, before any byte of data touches a network socket.
*   Masking must be deterministic and reversible (or mapping-aware) so the AI's output remains syntactically valid when injected back into the codebase.

## 3. Architectural Decision (The Chosen Path)
We implement a native, mandatory **PII Scrubber Engine** in `pkg/security`.

### How it works:
1.  **The Interceptor:** Before compiling the final JSON payload for the LLM API request, the entire context string (prompts, retrieved pointer records, logs) is passed through the PII Scrubber.
2.  **Regex & Heuristic Masking:** The scrubber identifies sensitive patterns:
    *   Email addresses $\rightarrow$ `<MASKED_EMAIL>`
    *   IP Addresses $\rightarrow$ `<MASKED_IP>`
    *   API Keys/Secrets $\rightarrow$ `<MASKED_CREDENTIAL>`
    *   National IDs (HETU) $\rightarrow$ `<MASKED_ID>`
3.  **Stateful Mapping (Optional):** If the agent needs to manipulate a string containing an IP address, the scrubber maps it to a safe alias (e.g. `IP_ALIAS_1`) during transmission and maps it back to the real value upon return.

### Why this option wins:
A local, deterministic regex/heuristic scrubbing engine guarantees that no sensitive tokens ever leave the execution host. This transforms OpenExec from a standard development tool into an enterprise-ready compliance framework, explicitly protecting organizations from accidental data exfiltration via LLM telemetry.
