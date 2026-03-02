---
---

<instructions>
Focused security vulnerability review.
</instructions>

<process>
1. Scan for common vulnerabilities:
   - Injection risks (SQL, command, XSS)
   - Authentication/authorization gaps
   - Sensitive data exposure
   - Insecure configurations
2. Review input validation
   - Are boundaries checked?
   - Is user input sanitized?
3. Check dependency usage
   - Known vulnerable patterns?
4. Review error handling
   - Information leakage?
   - Fail-secure behavior?
5. Document any findings with:
   - Severity assessment
   - Exploitation scenario
   - Recommended fix
</process>
