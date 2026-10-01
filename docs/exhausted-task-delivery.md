# Exhausted-task correction verification

US-011 / T-US-011-004. This record replaces the earlier US-009 preparation
summary. The candidate is verified locally; publication, canonical gates,
independent review, the owner's merge decision and D2 remain Console-owned.
No merge, deployment or live incident correction is claimed.

## Candidate and scope

Branch: `outcome/ad5a70cd0924dae18de345dbdf74e623`.
Stage-entry revision: `45d0ac5e01fa9361f532995e933a6bc546412239`.
Production comparison base: `ca2bdf8c254cfa95d7140064b177f54e7529a763`.
The retained preparation ancestor is
`dbfec11b6934dec8b1fa72c50cc6f871f44043a6`.
The ordinary Git commit for this task preserves the candidate; its subject names
US-011 and T-US-011-004. It does not supply external delivery authority.

[The machine record](exhausted-task-delivery.json) binds source paths, modes and
SHA256 values, including staged, unstaged and untracked source. The record, this
Markdown, NOTES.md and the evidence directory are excluded to avoid self-reference;
ignored caches/build output are excluded. HEAD is outside the content digest so
committing the verified bytes does not invalidate the evidence. The verifier
checks branch, ancestor identities, source content and every referenced artifact.

Read the live Project context and Simple Loop architecture contract. The selected
outcome is verification of bounded correction on the same retained candidate.
The observed gaps were an old integration-only measurement, future test names,
planned skips and missing complete orchestration. This change extends the existing
repository harness, AST scope resolver, native queue/SQLite fixtures and shipped
CLI walkthrough. No production implementation, runtime record, loop, authority
object or owner decision is added. Complexity delta: test fixtures and one
orchestration entry point; no product architecture change.

## Reproduction and exact results

Run `bash scripts/verify-exhausted-task-reconciliation.sh all`. It executes
self-tests, dedicated unit coverage, every named native case, persisted reloads,
race checks, frozen discovery, the public executable and compiled removal controls.
Go JSON must contain exact run/pass events and package success; missing, failed
or skipped names refuse acceptance. Required child names are pinned independently
of parent tests. Disjoint verification commands may run concurrently.

Final candidate verification, 2026-10-01:

| Check | Exact result / retained evidence |
| --- | --- |
| `run_declared_check lint` | Exit 0; Go vet and UI ESLint — [host response](verification/exhausted-task-delivery-results/lint.json) |
| `run_declared_check test` | Exit 0; Go suite and 635 UI tests in 40 files — [host response](verification/exhausted-task-delivery-results/test.json) |
| `scripts/verify-exhausted-task-reconciliation.sh all` | Exit 0; 147 required native names, all 35 mapped cases, no skips; discovery, race checks and all removal controls passed — [all log](verification/exhausted-task-delivery-results/acceptance.txt) |
| Dedicated atomic unit coverage | **1,442 / 1,596 statements (90.3509%) across 55 whole functions** — [unit result](verification/exhausted-task-delivery-results/unit-result.json), [atomic profile](verification/exhausted-task-delivery-results/unit-coverage.out) |
| `scripts/verify-exhausted-task-reconciliation.sh exhaustion-controls` | Exit 0; 16 required names, fresh failure and task snapshots reread — [reload log](verification/exhausted-task-delivery-results/reload.txt) |
| `make compat-test` | Exit 0; current `.openexec`, legacy `.uaos`, config fallback and `.openexec/tasks.json` — [output](verification/exhausted-task-delivery-results/compatibility.txt) |
| `make type-check` | Exit 0; Go build and UI tsc — [output](verification/exhausted-task-delivery-results/types.txt) |

[All-mode provenance](verification/exhausted-task-delivery-results/all-result.json)
records the stage-entry revision, branch, source hashes and log digests. It rejects
source changes during verification. The delivery package was written, reread and
recomputed against current source. All 34 verifier self-tests passed (exit 0),
including persisted delivery-record refusal fixtures —
[output](verification/exhausted-task-delivery-results/self-tests-full.txt).
Canonical gates remain unverified as specified below.

## Unit denominator and threshold

The unit invocation is a separate `go test -json -count=1 -covermode=atomic`
process with `-coverpkg` covering manager, release, state and CLI. Its explicit
[test manifest](../scripts/verification/exhausted-task-unit-tests.json) selects
direct units and isolated package/component tests with fixture providers and
executors. The dedicated correction tests invoke reconciliation with an executor
that runs no shell/provider, and invoke Cobra directly. Native shell journeys and
`TestTaskCorrectCLI` do not contribute to this profile. Those run separately.
Three pre-existing empty legacy placeholders (`TestStartDuplicate`, `TestPause`,
`TestStop`) are outside this manifest; modern queue/writer, Stop, cancellation,
pause and Wait tests execute instead. No skip is accepted within the selected run.

The denominator is the union of the frozen inventory, every function in correction
production files, and every changed production function since the comparison
base, including shared scheduler/start/events/store functions and CLI registration.
Whole function statement counts come from Go source instrumentation. Omitted
profile blocks receive zero hits; neither missing functions nor partial profiles
reduce the denominator. The original profile remains atomic; an evaluator copy
normalizes only the mode header because statement positions are identical.
Integer arithmetic requires `covered * 10 > statements * 9`. Self-tests reject
89%, exactly 90%, missing/empty profiles and omitted blocks; they accept 91%.
They also refuse absent/skipped cases, malformed evidence and scope narrowing.

## Final finding dispositions

All 35 discovery cases are mapped in the existing
[case matrix](../scripts/verification/exhausted-task-review-cases.json).
The individual native events, command transcript, reread snapshots and removal
logs are retained in the evidence directory. Qualifications below state the
accepted scope rather than claiming an external Console implementation.

| Finding | Final disposition and evidence |
| --- | --- |
| F1 — stale pre-admission authority blocks independent work | **Accepted.** Edited bytes, new commits, untracked output, actual branch changes, task branch, state hash, current/newer plans, graph, binding, malformed records, receipt and restart reach consumed `pre_admission_refused` without dispatch. Independent work drains; 3/3 history and retained receipts survive reopen. Fresh decisions bind changed candidate bytes/HEAD instead of resetting them. Same-queue independent edits, operational store failures, replacement history, concurrent admission/refusal and concurrent grants are covered. Overlays removing refusal persistence, snapshot/SQL ownership, spent/fresh-evidence guards or operational error propagation fail named assertions. |
| F2 — no public trusted authorization caller | **Accepted for OpenExec.** The built shipped executable runs planned and no-plan Git/SQLite scratch journeys: denied agent/flag/reference/plan/receipt/branch/graph inputs; explicit authorization; scoped queue; database reopen; exact decision and candidate binding; unchanged original receipt and attempts. Removing manager authorization, altering the decision or bypassing operator/reference guards fails the public walkthrough. Console-side transport absence is not established and is not part of this disposition. |
| F3 — omitted task scripts / applicable native obligations | **Accepted with a concrete scope qualification.** Planned, planner-imported and legacy no-plan cases run actual successful, failing, quoted and absent-file scripts; required plan failure/unsupported argv are exercised only where a plan exists. All supported command forms (sh, /bin/sh, named lint/test) execute and retain item evidence. Script execution, finalization and unsupported-item removal controls fail. **Rejected subproposal:** inventing a blanket refusal for tasks with no declared obligations contradicts the existing native completion contract retained by T-US-011-002. The explicit no-plan/optional-plan tests require zero invented checks while retaining authority, completion guards, attempts and receipts; an overlay introducing such a refusal fails. |
| F4 — exhaustion boundary hides original evidence | **Accepted.** Exhausted failed work projects `attempt_limit` and original evidence; ordinary below-limit failures retain `failed`. JSON/reopen tests retain structured evidence and reason while `Error()` omits private reason. Boundary-kind and evidence-removal overlays fail the intended assertions. |

The original diagnostic-bearing regression still completes A then Settings after
explicit authorization. Removing queue reconciliation reproduces exactly
`task repair attempt limit reached`. Stop/cancellation overlays prove the controls
can actually prevent completion. The denied executor test attempts a file write:
the configured executor refuses it and the file is absent; removing the executor
binding creates the file and fails the assertion. This proves preservation of the
configured executor boundary, not an OS sandbox or arbitrary host-code adversary.

## Test changes and compatibility

Added direct correction/CLI units, explicit changed-candidate refusal and fresh
binding cases, supported-argv execution, persisted independent-change assertions,
and the denied-effect side-effect assertion. The script matrix now generates
plan-only cases only for planned fixtures instead of manufacturing four skips.
The records verifier now permits implemented-but-unverified case definitions and
literal hyphens in actual subtest names; it still refuses unsupported completion
claims and nonliteral selectors. No production behavior or assertion was weakened.

Protected `.openexec`, legacy `.uaos` and `.openexec/tasks.json` fallback checks
execute through `make compat-test`. No project loader, migration, schema or legacy
selection behavior changed. Full host tests include Go and UI; type-check includes
Go compilation and UI `tsc --noEmit`.

## Limitations and delivery boundary

These are candidate tests with synthetic retained incidents, local Git/SQLite,
fixture providers and real deterministic shell checks. They are not the owner's
live database or a Console browser journey. The frozen discovery archive has its
own pinned baseline and is regression evidence, not this candidate's unit profile.
Logs are local evidence, not signed attestations. Existing React act warnings and
Go's read-only module-stat-cache warning are recorded with the actual exits.

The supplied Console serving observation is revision `69d80355`, started
`2026-10-01T16:34:34Z`; it was not independently checked here and is not OpenExec
deployment evidence. Canonical `make check`/`make pr-gate`, independent review,
publication, owner acceptance and default-branch merge are explicitly **unverified**
in this stage. Missing merge evidence proves neither delivery nor non-delivery.
Console must bind these verified bytes to the published candidate/PR head, obtain
its canonical gate and review, request the exact owner merge decision, then record
and re-fetch the actual OpenExec merged revision and default branch (including
content equivalence for squash/rebase). D2 remains pending until that evidence exists.
