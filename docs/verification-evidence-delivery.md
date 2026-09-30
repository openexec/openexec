# Exact-candidate delivery evidence — US-015 / T-US-015-001

This replaces the earlier US-011 delivery summary. The authoritative local run
and retained evidence inventory are in [delivery result](verification/delivery-result.json).
The executed candidate is identified by its parent revision, worktree file digest
and verifier/source hashes; the subsequent commit adds this evidence documentation.
No report here proves a merge, publication, deployment or actual Console adoption.
D2 is explicitly incomplete.

## Verification contract

`python3 scripts/verification/delivery.py --case delivery-ready --output <empty-directory>`
executes all 16 required members: four retention proofs, four recapture proofs,
admitted-all, named-recapture, recapture-variants, private-storage,
storage-unit-coverage, native-journey, discovery and verifier-controls. Every
member must succeed; remaining cases still execute after failures. Helpers reject
missing/skipped/duplicate scenarios, missing instrumentation, weak coverage and
mutation failures at unexpected assertions. Output directories cannot reuse old
success. Start/end file hashes detect candidate changes during a run.

All four full-body coverage slices require strictly greater than 90%, independently.
The old retention baseline now accounts for the newer independently measured
admitted/storage scopes. Only the shared non-deployed test fixture is excluded.
Unknown production functions and missing original functions still fail closed.
The study checksum now includes the three helper bodies added by T-US-013-003;
the previous checksum incorrectly rejected that expanded denominator.

`--adapter-evidence <file>` forwards an external report to the existing strict
adoption validator. Wrong candidate, missing provenance, missing lint/test ×
silent/diagnostic × nil/non-nil cases, missing reload assertions, failed commands,
unresolved work and coverage at or below 90% are rejected. Omission records
**deferred after merge**, never adopted. The evidence API exists only in this
candidate until merge; Console must update its dependency and adapter afterward.

`--case goal-complete --merge-evidence <file>` reruns the same local checks and
requires the Console receipt schema enforced by `delivery.check_merge`: exact
candidate revision and file digest; Console, merge and default-branch revisions;
repository/default branch/PR; owner decision, independent review, canonical gate
and merge references; true exact-candidate approval/review/default-branch and gate
assertions; no unresolved findings. Missing or stale evidence refuses completion.
This validates an externally authenticated Console receipt, not its authenticity
or remote Git state independently. Console must obtain and authenticate those
references. Local fixtures must never be submitted as real external evidence.
No validator performs merge or publication. `full` additionally runs the canonical
command map, with no fallback for unavailable gates; that gate is Console-owned.

## Requirement traceability and finding dispositions

| Requirement / finding | Evidence and disposition for Console |
| --- | --- |
| REQ-001 / F2 public admitted evidence | OpenExec fix verified locally: admitted-all plus retained-result and native-journey preserve non-nil failed results, exact argv/cwd, bounded diagnostic tails and private repair references. Eight real admitted command variants reopen SQLite, reload failure and repair, and check redaction. Actual Console adapter adoption is deferred until this API merges; update dependency, call RetainCommandEvidence and VerificationCommandFailureWithEvidence, then supply the strict external report. |
| REQ-002 / F1 legacy incident recapture | Fixed locally: named-recapture reproduces sole lint/test exit-2 receipts with empty or matching phase and no historical command. Forty-eight recapture-variants cover success, existing evidence, historical identity, fresh/remaining/spent budget, ambiguity, mismatch, HITL, review, cancellation and launch refusal. Removing inference or named fallback fails at unresolved/needs_review with zero executions. |
| REQ-003 / F3 private evidence in source commits | Fixed locally: private-storage proves ignored state-tree storage, source-context exclusion, staging exclusion, path/symlink refusal and readable registered evidence after reopen. The isolated old-directory mutation must fail both source and staging assertions. Storage coverage includes full file bodies. Unregistered legacy artifacts remain refused; compatibility does not authorize unsafe paths. |
| REQ-004 / F4 D2 delivery | Local D1 reproductions, positive fixes and isolated regression controls are prepared. D2 remains incomplete: no candidate-matched external merge receipt exists. Console owns canonical gate, existing-PR delivery, independent review, finding disposition and exact owner merge decision after the queue finishes. No replacement PR or delivery task is created. |

The requirement register preserves the original accepted wording as history.
For this stage, the owner's explicit instruction defers actual adapter adoption
until after merge; it does not relabel that adoption as verified.

## Persisted journeys and controls

The aggregate walks real local admitted commands through StageExecutor, engine,
pipeline, SQLite failure recording, native repair creation and queue continuation.
It closes and reopens the database before asserting task status, attempt counts,
original/repair dependency, readable evidence and diagnostic tail. Successful
recapture alone cannot complete A without a supported completion claim; Settings
remains unattempted after reopen until that claim permits continuation. Terminal
refusal, cancellation, ambiguous identity and spent/interrupted budgets persist
without refund, duplicate dispatch or unauthorized repair.

Protected `.openexec`, `.uaos` and `.openexec/tasks.json` formats exercise failure,
success and unresolved journeys. Restoring either engine result-discard branch
fails the required engine and persisted-evidence assertions. Named resolver and
old-directory mutations use isolated replacements, leave candidate sources intact,
and require expected assertion failures rather than arbitrary process failure.
Verifier controls also remove members, drop/duplicate/skip scenarios, damage
coverage profiles, weaken thresholds and remove receipt fields.

## Source versus runtime integration

Read the supplied Console revision directly with `git show` from the Console
repository, without trusting its working tree. Source revision
`4c819427643655fc85d830b3a79f337488bd3fc0` uses
VerificationCommandFailureWithOutput in executeOpenExecCheck and pins OpenExec
`v0.13.2-0.20260930061734-4290c0d23b00`. It does not call the evidence-bearing API.
Source hashes are recorded in the result inventory. The owner-supplied process
observation started at `2026-09-30T08:52:05Z`; it is not binary-module attestation,
a runtime adapter journey or proof of an OpenExec deployment. No such claims are
made from checkout notes or historical reports.

## Resources, compatibility and complexity

Declared host lint/test results are retained with exit codes and output. The
host generic command runner refused this stage and directed sandbox execution;
its message explicitly says an OpenExec stage has no job_runner.py lane. The
aggregate therefore ran in the stage's sandbox with retained completed results.
No asynchronous start or partial output is called a pass. The Go module stat
cache emitted a read-only warning; the writable `/tmp` build cache allowed checks
to complete. UI tests emitted existing React warnings but passed. Canonical
`make check`/`make pr-gate` remain for the later socket-capable repository runner.
No deployment/host-port test is substituted with source inspection.

Only verification and documentation change. Runtime, loaders, migration schemas,
receipt classification, admission and retry behavior are unchanged; protected
format journeys and `make compat-test` corroborate compatibility. Project context
was read before this work (Goal/Ready 4, interpretation 10); this task follows the
specific accepted verification-repair Goal and effect restrictions.
Complexity delta: zero new runtime concepts, persistent state, transitions,
owner decisions or replacement machinery. Existing deterministic verification
helpers are composed; Console's delivery boundary remains intact.
