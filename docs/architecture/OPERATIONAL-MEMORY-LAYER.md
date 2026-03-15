# OpenExec Operational Memory Layer (Pointer Record Architecture)

The Operational Memory Layer stores structured, persistent knowledge about systems and operations. Unlike probabilistic RAG retrieval, this layer contains **typed pointer records** that reference systems, services, environments, and procedures.

## 1. Why Pointer Records?
Most AI systems rely on prompts and probabilistic document retrieval, which fails in production operations due to instruction drift, token limits, and loss of history. Pointer records solve this by storing explicit references to operational truth.

**Example:** Instead of guessing how to deploy a service, the runtime retrieves an exact `ServiceRecord` containing the repo path, owners, and approved remediation commands.

## 2. Core Record Types
The memory layer uses schema-backed records:
*   **ServiceRecord:** Metadata about a specific service (repo, deployment method, owners).
*   **RunbookRecord:** Failure modes and approved remediation actions.
*   **EnvironmentRecord:** Cluster info, policy levels, and allowed actions.
*   **ProcedureRecord:** Step-by-step playbooks (e.g., "how to restart service").
*   **MaintenanceRecord:** Historical log of past issues and applied patches.

## 3. The Pointer Graph
Records form a deterministic knowledge graph designed for operational systems:
`ServiceRecord` → `RunbookRecord` → `ProcedureRecord` → `ActionContracts`.

## 4. Retrieval & Hydration
Operational memory is retrieved **before** model reasoning:
1.  Identify target service/system.
2.  Fetch relevant records (Service, Runbook, Environment).
3.  Assemble a **Structured Context Pack** (metadata, constraints, recent history).
4.  LLM reasons over the hydrated context.

## 5. Safety & Redaction
Pointer records can include **Redaction Policies**. The context builder applies these scrubbing rules (e.g., masking emails or PII) locally before sending any data to a cloud model, significantly reducing leakage risk.

## 6. Strategic Advantage
Most AI coding tools optimize for writing new code. OpenExec optimizes for **maintaining real systems**. By combining operational memory with deterministic orchestration, OpenExec provides a rare combination of safety and precision for production maintenance.

---
**Summary:** OpenExec is a deterministic coding and maintenance runtime that combines blueprint orchestration with an operational memory layer built on pointer records.
