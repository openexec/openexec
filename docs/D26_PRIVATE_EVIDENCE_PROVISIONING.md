# D26 private evidence: provisioning and recovery status

`TestD26PrivateOccurrenceEvidence` (`pkg/execution/d26_evidence_test.go`) reads a
private capture that is never committed:

- path: `/home/perttu/.agent-console/run-requests/d26bd137975612543c90e5b983f60db8.json`
- required SHA-256: `0cbc9da8b5a6ed7d03db2b4a0a8ac16818f1c648f3db776090c83651bba20708`
  (recorded in `docs/QWEN_DISPATCHED_CONTEXT_OVERFLOW_DIAGNOSIS.md`)

What the test guards: it hash-checks `d26-events.json` in the scratch directory, and
fails closed (`t.Fatal`) when the capture is missing or malformed or its content
(message/tool counts, truncation, `textLimit`) differs. **The test does not hash the
capture itself**; the capture's authenticity is guaranteed by the restore step below,
which is therefore mandatory. Do not edit, skip or weaken the test, and do not
synthesize the capture.

## Repeatable restore step (operator, outside the repository)

Given an authentic copy `SRC` from a recorded source (Agent Console run-request
archive or backup):

    scripts/restore-d26-capture.sh SRC
    go test ./pkg/execution -run TestD26 -v   # PrivateOccurrenceEvidence must RUN, not skip

The script computes SHA-256 of `SRC`, refuses (exit 1, nothing written) unless it equals
the required hash, then installs it at the capture path with mode 0600. Negative
control is committed: `TestD26RestoreScriptFailsClosed`
(`pkg/execution/d26_restore_script_test.go`) runs the real script on scratch wrong,
empty, missing and directory inputs and requires exit non-zero, no output file, a
`refused` message and no content in the output. (No positive case can exist without
the authentic bytes.) The full search record and refusal are in
`docs/D26_CAPTURE_PROVENANCE_SEARCH.md`.

## Recovery status: NOT RECOVERED

No authentic copy has been found. Host-side SHA-256 searches by the supervisor
(task addenda, 2026-11-15) found zero matches and zero read errors in:

- `~/.agent-console/run-requests` (212 files), `~/.agent-console/backups` (58),
  `/mnt/data1/backups` (94), files up to 50 MB;
- `~/.agent-console/perttu/scratch`: 151,399 regular non-symlink files up to 50 MB
  plus all 55 larger files (3,905,690,883 bytes). Two occurrence-named files there
  (`…153647Z`, `…160203Z`) hash differently and must not be provisioned.

This unattended pass could not itself search: its tools refuse paths outside the
checkout and inline scripts. Not searched: archive members, other hosts (including CI
runners), other locations. Recovery is not proved impossible, and the cause of absence
(pruned vs never retained) is unknown. If an authentic copy surfaces, apply the restore
step. If none exists, changing the requirement is an owner decision (open card
`d26-capture-unrecoverable`, default: keep requirement, gate stays blocked). No test or
evidence change is made here; f764's Stop, accounting and PR 233 identity are untouched.
