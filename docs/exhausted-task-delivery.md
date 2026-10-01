# Exhausted-task delivery preparation

US-009 / T-US-009-001 preparation; T-US-009-002 delivery handoff.
**Preparation complete; delivery and D2 pending.**
The authoritative machine-readable companion is
[exhausted-task-delivery.json](exhausted-task-delivery.json). This is the
exhausted-task correction story, not earlier identity/retention stories that
reuse task numbers.

## Candidate identity and reproducibility

Candidate branch: `outcome/ad5a70cd0924dae18de345dbdf74e623`.
Preparation base revision: `dbfec11b6934dec8b1fa72c50cc6f871f44043a6`.
Implementation comparison base: `ca2bdf8c254cfa95d7140064b177f54e7529a763`.
These are fixed ancestry anchors, not a claim that HEAD remains at the base.
The preparation-stage commit preserved evidence only. This handoff stage leaves
its changes uncommitted; Console owns candidate commits after queue completion.

The companion's `source_files` and `source_sha256` bind 1,336 repository files,
including implementation, tests, fixtures, verifier and relevant documentation.
The digest is SHA256 of UTF-8 compact JSON (`sort_keys=True`, separators `,` and
`:`) containing sorted path/mode/content-SHA256 rows. Git's cached plus unignored
untracked union is read from the working filesystem, so staged, unstaged and new
source are included. Deleted files disappear; executable modes and symlink target
text are hashed. The verifier recomputes the complete set, not just listed files.

Excluded to avoid self-reference: this Markdown, the companion and its results
directory. Working memory (`NOTES.md`), `bin/`, `ui/dist/`, `ui/coverage/`,
`.gocache/`, Python caches and Git-ignored disposable outputs are excluded.
No relevant source changes were present at stage entry; all new verifier source
and test fixtures are included before checks run. HEAD is intentionally outside
the content hash, allowing the evidence-preservation commit without a hash cycle.
Branch identity and both base ancestors are independently checked.

## Changed behavior and native journey

A trusted explicit correction decision authorizes one pass of required checks
against the exact retained candidate, task, receipt, accepted plan and graph.
The existing native queue consumes that allowance before deterministic execution;
it preserves exhausted attempts, original receipts and completed prerequisites.
Fresh evidence and the existing validation/effect controls determine completion.
Success releases dependents through the same queue. Continuing failure records
fresh evidence and a terminal disposition while independent work drains.

The implementation boundary is OpenExec's existing queue, task metadata, SQLite
ledger and deterministic executor. This stage adds only a read-only preparation
verifier and evidence. Complexity delta: no new runtime concept, transition,
owner decision, execution loop or Console controller. Project context and the
Simple Loop contract were read; the broader portfolio Goal does not expand this
selected task. Detailed interfaces and prior intentional test changes remain in
[the implementation record](verification/exhausted-task-reconciliation.md).
No existing behavioral tests were changed in this stage.

## Required checks and outcomes

The preparation-stage checks below each have exit 0, a content-bound command/result entry in the companion and
stored output. The first two outputs are the actual host tool responses.

| Command | Evidence / outcome |
| --- | --- |
| `run_declared_check lint` → `make lint` | [lint.json](verification/exhausted-task-delivery-results/lint.json): Go vet and UI ESLint passed |
| `run_declared_check test` → `make test` | [test.json](verification/exhausted-task-delivery-results/test.json): Go suite and 635 UI tests, 40 files passed |
| `scripts/verify-exhausted-task-reconciliation.sh` | [acceptance.txt](verification/exhausted-task-delivery-results/acceptance.txt): 84 required tests/subcases, no skips; coverage and removal proof passed |
| `scripts/verify-exhausted-task-reconciliation.sh exhaustion-controls` | [reload.txt](verification/exhausted-task-delivery-results/reload.txt): persisted JSON reread, native failure/exhaustion controls passed |
| `make compat-test` | [compatibility.txt](verification/exhausted-task-delivery-results/compatibility.txt): current/legacy loading and fallbacks passed |
| `make type-check` | [types.txt](verification/exhausted-task-delivery-results/types.txt): Go build and UI tsc passed |

The preparation verifier and its controls are reproduced with:

```sh
scripts/verify-exhausted-task-delivery-evidence.sh --phase preparation
scripts/verify-exhausted-task-delivery-evidence.sh --self-test
```

Both passed after writing and rereading the artifacts. Three Python test methods
include eleven persisted CLI refusal fixtures, plus uncommitted-byte, executable
mode, untracked-file, deletion and nonlocal-reference controls. Fixtures reject
stale branch/base/digest, absent evidence/checks, failed/stale checks, omitted
coverage and unsupported D2, delivery or Goal completion claims. Only preparation
is accepted as a phase; this tool cannot certify external delivery.

## Affected functions and removal-sensitive proof

The companion contains every measured function from the
[48-function inventory](../scripts/verification/exhausted-task-inventory.json),
with exact covered/total statement counts. Total: **1,293 / 1,433 (90.2303%)**.
The threshold applies to the aggregate whole-function denominator, not to each
function individually. The native proof reconciles changed production bodies
against the implementation base and rejects missing inventory or profile blocks.
The detailed prior-stage explanation remains in
[the reconciliation verification record](exhausted-task-reconciliation-evidence.md).

The fresh acceptance log contains a Go overlay removing the actual reconciliation
and exhaustion-disposition queue integration. `TestCorrectionDiagnosticQueueSuccess`
then exits 1 with the exact original `task repair attempt limit reached` refusal.
The verifier accepts that expected failure only after matching the test and source
location; compile errors, skips and unrelated errors do not count. The candidate
source remains unchanged and overlay cleanup is verified by its SHA256.

## Refusals and persisted reload assertions

Real native tests exercise absent/consumed authority; stale candidate, receipt,
plan and graph; unmet dependencies; failed/unsupported validation; denied effects;
agent-only claims; Stop; cancellation; interrupted admission and candidate drift.
Re-entry cannot renew consumption. Independent work can drain without releasing
blocked dependents or creating an unauthorized repair.

The native tests close and reopen SQLite, then reread task, original receipt,
consumed correction, branch/commits, completed prerequisite and dependent. Attempts
remain 3/3. Successful completion and reopen are exercised by both native success
tests in acceptance. The separately stored reload output preserves structured
failure/exhaustion payloads: original `legacy` receipt, fresh failed receipt,
continuing-failure reason/disposition and no-authority `attempt_limit` survive.
The preparation verifier parses these persisted payloads again. CLI fixtures are
written, closed/read by a new verifier process and refused for their intended
reason; the valid package is reread from disk after the handoff edits.

## Remaining limitations

These are candidate results, not a live owner-database or Console UI journey.
Native fixtures run the real queue and SQLite with synthetic retained receipts;
independent discovery's actual timeout reproduction is prior-stage evidence.
The prior embedded build and discovery outcomes remain labeled dependency
results, not fresh checks here. Host Go tests can use Go cache; focused native
acceptance uses `count=1`. Existing React act warnings and a read-only module
stat-cache warning did not fail checks. Stored logs are hashed local evidence,
not externally signed attestations. Source changes require fresh evidence.

No production/loading behavior changes in this stage; compatibility tests still
passed. The repository's full canonical gate runs later in the socket-capable
repository runner, together with independent review.

## Historical Console evidence and Console-repository follow-ups

The supplied serving-process observation names Console revision `f7bf25d5`,
started `2026-10-01T14:36:52Z`. It is not independently reverified here and is not
OpenExec deployment evidence. Earlier Console tests, Settings commits and hooks
drift are historical claims, not current completion proof.

Console owns candidate commits, publication to the durable feature PR, the
canonical gate, independent review and asking the owner for the exact merge
decision after the queue finishes. The owner supplies that decision; neither
preparation readiness nor task completion supplies approval. No PR, review
verdict, acceptance, merge or deployment is claimed by this package.
Console-repository authority transport, Settings presentation and running-revision
verification need separate scoped evidence; they are not implemented or certified
by this OpenExec task.

## Pending D2 merge evidence

D2 stays pending until Console supplies actual OpenExec default-branch merge
evidence. Missing evidence is not a claim that a merge has or has not occurred.
The handoff requires a traceable chain:

- Identify the OpenExec repository and its actual target default branch from
  current repository-host evidence; do not assume a branch name.
- Bind the verified source manifest/digest and preparation base to Console's
  candidate commit(s) and published PR head. If relevant source changes, refresh
  verification; a matching branch name or preparation base alone is insufficient.
- Supply canonical gate results and independent review references for that exact
  candidate revision, plus the owner's actual exact merge decision and its scope
  (repository, PR, candidate revision and target branch). These references are
  currently absent, not placeholders for an invented approval.
- Supply the repository-host merged PR record, merged revision and merge time,
  naming the target default branch. Re-fetch that branch and prove the merged
  revision is in its history and contains the verified fix. For squash or rebase,
  record the source-to-merged-revision mapping and content comparison; candidate
  ancestry alone cannot establish equivalence.

Record these facts and durable evidence references together when Console obtains
then verifies them. A green preparation verifier only establishes candidate
readiness; a published PR, passing gate, favorable review or owner decision alone
is not a merge. Console's serving revision cannot establish OpenExec delivery.
This stage performs no acceptance, publication, merge, deployment or lifecycle
mutation. No runtime concepts, transitions or owner decisions were added.

Handoff verification: persisted package reread, preparation verifier and all
three boundary test methods (including eleven persisted refusal fixtures) passed.
Fresh host `run_declared_check(check="lint")` exited 0 (Go vet and UI ESLint).
Native checks in the table remain preparation-stage evidence, not fresh runs
claimed by this documentation-only handoff. No tests or production files changed.
