# OpenExec Run Timeline & Replay System

Every run in OpenExec produces a complete deterministic timeline of decisions, tool calls, actions, model responses, artifacts, and policy checks. It combines Git history, CI logs, and LLM traces into one structured event stream.

## 1. The Timeline Concept
Every run produces a chronological event log:
**Run** → **Step** → **Events** (ToolCall, PolicyCheck, ModelInvocation, ArtifactCreated).

### Event Types
*   **StepStarted:** Marks the beginning of a blueprint stage.
*   **ModelInvocation:** Records model used, tokens, and duration.
*   **ToolCall:** Records tool name, inputs, and outputs.
*   **PolicyCheck:** Records capability evaluated and the decision made.
*   **ArtifactCreated:** Pointers to patches, logs, or context packs.

## 2. Storage & Layout
Timelines are stored in two formats:
1.  **SQLite:** For complex queries across multiple runs.
2.  **JSONL:** An append-only log (`timeline.jsonl`) inside the run directory for trivial replay.

**Run Directory Layout:**
`.openexec/runs/run_<id>/`
*   `timeline.jsonl`
*   `artifacts/` (patches, test results, etc.)
*   `context_pack.json`
*   `policy_snapshot.json`

## 3. Run Replay Modes
Replay allows reproducing runs without new model calls or testing runtime changes:
*   **Exact Replay:** Uses stored model responses from artifacts. No new model calls.
*   **Model Re-run:** Re-runs only the LLM steps (useful for model comparison or prompt tuning).
*   **Step Replay:** Replays a single specific step for targeted debugging.

## 4. Observability Benefits
*   **Time Travel Debugging:** Inspect the exact context pack and prompt for any historical step.
*   **Performance Metrics:** Calculate tokens per run, tool latency, and step success rates.
*   **Safety Auditing:** Generate reports on repo writes, network attempts, and policy violations.
*   **Run Diffing:** Compare two runs to see how model or tool changes affected the output.

## 5. Strategic Advantage
Most AI agents are opaque ("it tried and failed"). OpenExec provides **Full Execution Transparency**. By making every run inspectable, replayable, and auditable, OpenExec acts as **"GitHub Actions for AI Development."**

---
**Summary:** The Timeline & Replay system ensures that OpenExec remains a controlled engineering tool rather than a "magic" black box.
