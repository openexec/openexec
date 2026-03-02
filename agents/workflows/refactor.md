---
params:
  scope: "What is being refactored"
  trigger: "When refactoring is triggered"
  stopping_criteria: "When to stop"
---

<instructions>
Refactor {{scope}} (triggered: {{trigger}}).
</instructions>

<process>
1. Ensure all tests are passing (green state)
2. Identify refactoring opportunities:
   - Duplicated code
   - Long methods
   - Complex conditionals
   - Unclear naming
3. Apply refactoring in small, safe steps
4. Run tests after each change
5. If any test fails, revert immediately
6. Continue until: {{stopping_criteria}}
7. Confirm all tests still pass
</process>
