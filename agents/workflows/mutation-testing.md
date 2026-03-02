---
params:
  intent: "Why this agent runs mutation testing"
  stopping_criteria: "When to stop iterating"
---

<instructions>
{{intent}}
</instructions>

<process>
1. Run mutation testing tool
2. Review surviving mutants
3. For each survivor:
   - Understand what mutation wasn't caught
   - Assess if it represents a meaningful behavior gap
   - Add test if gap is significant
4. Re-run mutation testing
5. Continue until: {{stopping_criteria}}
6. Document any accepted survivors with reasoning
</process>
