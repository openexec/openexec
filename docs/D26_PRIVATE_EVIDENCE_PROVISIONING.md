# D26 private evidence: provisioning and recovery status

`TestD26PrivateOccurrenceEvidence` (`pkg/execution/d26_evidence_test.go`) reads a
private capture that is never committed:

- path: `/home/perttu/.agent-console/run-requests/d26bd137975612543c90e5b983f60db8.json`
- required SHA-256: `0cbc9da8b5a6ed7d03db2b4a0a8ac16818f1c648f3db776090c83651bba20708`
  (recorded in `docs/QWEN_DISPATCHED_CONTEXT_OVERFLOW_DIAGNOSIS.md`)

The test fails closed (`t.Fatal`) when the file is missing, malformed or its hash
differs. Do not edit, skip or weaken it, and do not synthesize the capture.

## Repeatable restore step (operator, outside the repository)

Given an authentic copy `SRC` from a recorded source (Agent Console run-request
archive or backup):

    sha256sum SRC    # must print 0cbc9da8...0708 exactly
    install -m 0600 SRC /home/perttu/.agent-console/run-requests/d26bd137975612543c90e5b983f60db8.json
    go test ./pkg/execution -run TestD26 -v   # PrivateOccurrenceEvidence must RUN, not skip

A copy whose hash differs must not be installed. The hash check in the test is the
authenticity guard; the file must be readable only by the gate user.

## Recovery status: NOT RECOVERED (refusal)

This pass could not recover the capture.

- Hash sought: `0cbc9da8b5a6ed7d03db2b4a0a8ac16818f1c648f3db776090c83651bba20708`.
- Observed: the declared `go-test ./pkg/execution -run TestD26 -v` fails with
  `d26_evidence_test.go:170: required private artifact unavailable or malformed`;
  `TestD26DiagnosticNativeAssemblyAndRefusalBoundary` and `TestD26StructuralChronology` pass.
  Opening the capture path with the file reader returns "File does not exist".
- Not searched: this unattended pass's tools refuse every path outside the task checkout
  (`ls`/`find` of `/home/perttu/.agent-console/run-requests`, `/home/perttu`, `/var/backups`,
  `/tmp`, and an inline hash-scan script were each refused as "outside this unattended
  task's authority"). So Agent Console run-request storage beyond the one path above, its
  archives/backups, other worktrees and hosts were NOT hash-searched here. Absence at the
  one path is not proof that no copy exists.
- Cause of absence is unknown; it was not established whether the capture was pruned or
  never retained.

Next step (needs host access outside this lane): hash-search the locations above for the
required SHA-256, then apply the restore step. If no copy exists, the D26 private-evidence
requirement cannot be met honestly; changing it is an owner decision, and no test or
evidence change is made here. Task f764's Stop, accounting and PR 233 identity are untouched.
