---
---

<instructions>
Execute full code review workflow with trust-first priority.
</instructions>

<process>
1. Read implementation from Spark
2. Read post-implementation explanation from story file
3. Execute trust-first review sequence:
   a. Test Quality — How much can we trust the test suite?
   b. Design Alignment — Is direction correct per Clario's design?
   c. Correctness — Does it work as intended?
   d. Security — Any vulnerabilities?
   e. Readability — Can others understand?
4. For each issue found:
   - Write explanatory feedback ("this breaks because X")
   - Ask curious questions when approach is unclear
   - Provide concrete alternatives
5. Self-check: Grade all comments for constructiveness
6. Compile review (story file comments + code annotations)
7. Make routing decision:
   - Issues found → Return to Spark
   - Design concerns → Consult Clario first
   - Approved → Handoff to Hon
8. Sign off: "The cuts are marked. Ready for refinement."
</process>
