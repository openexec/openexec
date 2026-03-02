# OpenExec Engineering Standards (AGENTS.md)

This document provides foundational mandates for AI agents working on OpenExec. These take precedence over general defaults.

## Senior Engineering Mandate: "Observe, then Resolve"

To prevent thrashing and stalling during task execution, agents MUST adhere to these practices:

### 1. Async & UI Testing (React/Vitest)
- **Prefer `findBy*`**: Always use `screen.findByText()` or `screen.findByRole()` for elements that appear after an async action (like a button click).
- **`userEvent` over `fireEvent`**: Use `@testing-library/user-event` for all interactions to ensure proper event bubbling and `act()` wrapping.
- **Acknowledge State Transitions**: If a component has multiple states (e.g., `idle` -> `loading` -> `success`), ensure the test explicitly waits for the transition using `waitFor()`.
- **Avoid `setTimeout`**: Never use manual delays in tests. Use Vitest's `vi.useFakeTimers()` or robust `waitFor` polling.

### 2. Error Diagnostics & Hypothesis
- **Verbosity First**: If a test fails once, do not immediately attempt a "fix." Instead, run the test again with `--verbose` or add `screen.debug()` to see the DOM state.
- **Hypothesis Requirement**: Before modifying any code to fix a bug/test, the agent loop should state a clear hypothesis for the failure (e.g., "Hypothesis: The click is unmounting the component before the state update completes").
- **No Progress = Revert**: If a change does not fix the reported error after one attempt, **REVERT** the file before trying a different strategy. Do not stack unverified changes.

### 3. Environment & Preflights
- **Verify Backend**: For UI tasks that interact with APIs, always verify the API schema (e.g., checking `internal/api/` or `types/`) before implementing the UI.
- **Mock Integrity**: Ensure mocks exactly match the current API response format (check `snake_case` vs `camelCase`).

### 4. Learning Loop (Engram)
- When a complex bug is solved (like the Popover/Vitest timing issue), the agent should summarize the "Lesson Learned" and persist it to `.openexec/engram/learning_log.json`.

---

## Known Project Quirks
- **Vitest & JSDOM**: Be aware that JSDOM does not perfectly simulate all layout-related events (like `onMouseEnter`). If tests fail on interactions, check if the component depends on layout properties.
- **Audit Database**: The real source of truth for task progress is `openexec/.openexec/data/audit.db`.
