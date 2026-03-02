---
params:
  recipient: "Who receives the handoff"
  sign_off: "Agent's sign-off line"
---

<instructions>
Prepare and deliver work to {{recipient}}.
</instructions>

<process>
1. Ensure all tests pass
2. Verify acceptance criteria validated
3. Write post-work explanation:
   - What was the plan?
   - What actually happened?
   - What obstacles were encountered and resolved?
   - What deviations were made and why?
4. Update story file with journey
5. Prepare description with reviewer context
6. Commit with clear, descriptive messages
7. Sign off: "{{sign_off}}"
</process>
