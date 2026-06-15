# ADR-XXX: Title of the Architectural Decision

*   **ID:** ADR-XXX  
*   **Status:** [Proposed | Draft | Approved | Deprecated]  
*   **Author:** [Your Name / Agent Name]  
*   **Date:** 2026-06-06  
*   **Decides on:** [Subsystem / File Path / Protocol / Toolset / Flow]

---

## 1. Context and Problem Statement

*Describe the current codebase reality, limitations, or requirements driving this decision. What is the business or technical problem? If this decision is prompted by a failure mode or bug (such as a database concurrency lock or an API-routing bottleneck), document the empirical evidence here.*

## 2. Decision Drivers

*What are the forces or constraints we must satisfy?*
*   e.g. Maintain a zero-dependency compiled Go binary footprint.
*   e.g. Avoid external cloud provider API locks.
*   e.g. Enforce absolute, deterministic sandboxing on infrastructure actions.
*   e.g. Keep local CLI startup latency under 100ms.

## 3. Considered Options

*Present at least two alternative architectural options that were evaluated, and briefly explain their pros and cons. Never pick a single path silently without presenting tradeoffs.*

### Option A: [Name of Option]
*   **Pros:** ...
*   **Cons:** ...

### Option B: [Name of Option]
*   **Pros:** ...
*   **Cons:** ...

## 4. Architectural Decision (The Chosen Path)

*State the chosen option clearly. Provide high-level pseudocode, directory layouts, and concrete system-design parameters.*

### Why this option wins:
*Detail the technical rationale for why this option is superior to the alternative considered options.*

## 5. Implementation Roadmap

*Break the implementation of this decision down into clear, sequential, testable milestones:*
1.  **Phase 1 (Go Core):** [Steps to implement core types, files, and interfaces...]
2.  **Phase 2 (Wired/Integrated):** [Steps to connect to the active server/CLI...]
3.  **Phase 3 (Gates & Tests):** [Steps to write unit tests, integration tests, or quality gates...]

## 6. Consequences & Tradeoffs

*What happens now that we have made this decision?*
*   **Positive Consequences:** What becomes easier or cheaper?
*   **Negative Consequences:** What becomes harder? What are the new trade-offs or technical debts we are introducing?
*   **Compatibility Impact:** Does this introduce any breaking changes to existing database schemas, configs, or CLI APIs?
