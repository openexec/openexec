# The 80% Trap: Bridging the Gap from Intent to Execution

This document defines the core philosophy for avoiding "Intent Drift" and the "80% Completion Plateau" in AI-assisted engineering. It serves as a mandatory instruction set for both human architects and AI executors working on OpenExec.

## 1. The Symptom: The 80% Plateau
In many greenfield AI projects, the system reaches a state where it is "technically working" but feels incomplete. Common issues include:
*   **Isolated Modules:** Components exist but are not deeply interconnected.
*   **Hidden Technical Debt:** AI takes shortcuts (e.g., raw shell commands) instead of building native logic.
*   **Feature Gaps:** The app does what the prompt said, but not what the user *expected* (missing the "obvious" connective tissue).
*   **Intent Drift:** The implementation gradually moves away from the high-level architecture as the AI focuses on individual task success.

## 2. The Root Cause: Narrative vs. Contract
The gap exists because we often rely on **narrative understanding** (PRDs and ADRs) rather than **machine-enforceable contracts**. 

When an AI "understands" a plan, it still searches for the path of least resistance. Without hard boundaries, it will build isolated silos that meet the immediate requirement but fail to integrate into the larger system.

## 3. The Solution: Boundary-First Development
To reach 100% and ensure modules are interconnected, we must move from "writing logic" to "defining boundaries."

### Phase 1: Define the Interface (The "Handshake")
Before any logic is implemented, the **Go Interface** must be written. 
*   **Rule:** You cannot implement a subsystem until its `interface.go` exists in an `internal/` package.
*   **Why:** This forces the AI to "see" how this module will link to others before it writes a single line of execution code.

### Phase 2: Define the Blueprint (The "Workflow")
The implementation must be driven by a **Blueprint DSL**, not a free-form loop.
*   **Rule:** Transitions between stages must be controlled by the runtime, not by agent prompts.
*   **Why:** This prevents the AI from "wandering" off the architectural path.

### Phase 3: State-First Persistence (The "Shared Brain")
Modules remain isolated when they don't share a common source of truth.
*   **Rule:** No side-car JSON files or hidden in-memory states. All modules must interact via the **Unified SQLite Schema**.
*   **Why:** SQLite is the "connective tissue" that ensures Module A knows exactly what Module B did.

## 4. Instruction Set for AI Tasks
When starting a new feature or refactor, follow these steps strictly:

1.  **Scrutinize the Interface:** Look at the existing interfaces. If you are adding logic that doesn't fit a contract, **propose an interface change first**.
2.  **Ban Shortcuts:** Do not use `shell_command` or `sh -c` for logic that should be Go-native. If a tool is missing, build a **Typed Action Contract**.
3.  **Check the Linkage:** Ask: "How does this module inform the next stage?" If the answer is "through a prompt," it is a failure. The answer must be "through a persisted artifact in SQLite."
4.  **Verify via Replay:** Use the **Run Replay** system to ensure the new implementation matches the architectural expectation, not just the "happy path."

## 5. The Goal: Self-Hosting
Our objective is for OpenExec to use this approach to build itself. By defining boundaries that are impossible to cross, we ensure that the AI remains a **component within a deterministic system** rather than an uncontrolled agent.

---
*If it isn't in an interface, it isn't in the project. If it isn't in SQLite, it didn't happen.*
