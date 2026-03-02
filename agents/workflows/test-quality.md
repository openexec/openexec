---
---

<instructions>
Review test suite effectiveness — this determines trust level for entire review.
</instructions>

<process>
1. Review test suite structure
   - Are acceptance tests BDD-style (Given-When-Then)?
   - Are unit tests focused on behavior, not implementation?
2. Assess coverage
   - Are critical paths covered?
   - Are edge cases addressed?
3. Evaluate test quality
   - Do tests fail for the right reasons?
   - Are assertions meaningful?
   - Could tests pass with broken code?
4. Check mutation testing results
   - Were survivors addressed or documented?
5. Rate trust level: High / Medium / Low
6. Document findings with explanations
</process>
