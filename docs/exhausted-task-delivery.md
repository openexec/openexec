# Exhausted-task correction verification

US-012 / T-US-012-001 and T-US-012-002. This record finalizes the retained US-011 verification
receipts for independently runnable preparation. The candidate is verified locally; publication, canonical gates,
independent review, the owner's merge decision and D2 remain Console-owned.
No merge, deployment or live incident correction is claimed.

## Candidate and scope

Branch: `outcome/ad5a70cd0924dae18de345dbdf74e623`.
Preparation entry and retained tested candidate: `cc79237b82fcccd4716396984bc2e799d6d35a28`.
The implementation verification ran on working bytes starting from
`45d0ac5e01fa9361f532995e933a6bc546412239`; the retained commit preserves those bytes.
Production comparison base: `ca2bdf8c254cfa95d7140064b177f54e7529a763`.
The retained preparation ancestor is
`dbfec11b6934dec8b1fa72c50cc6f871f44043a6`.
The preparation commit names US-012 and T-US-012-001; the handoff commit names
US-012 and T-US-012-002. These preserve the candidate without supplying external
delivery authority.

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

Retained implementation verification, 2026-10-01 (actual receipts from T-US-011-004):

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
recomputed against current source. The retained implementation run passed all 34 verifier self-tests (exit 0),
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

## Retained implementation test changes and compatibility

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

The supplied Console serving observation is revision `0f5ce914`, started
`2026-10-01T17:12:15Z`; it was not independently checked here and is not OpenExec
deployment evidence. Canonical `make check`/`make pr-gate`, independent review,
publication, owner acceptance and default-branch merge are explicitly **unverified**
in this stage. Missing merge evidence proves neither delivery nor non-delivery.
Console must bind these verified bytes to the published candidate/PR head, obtain
its canonical gate and review, request the exact owner merge decision, then record
and re-fetch the actual OpenExec merged revision and default branch (including
content equivalence for squash/rebase). D2 remains pending until that evidence exists.

## Final preparation and provenance

Run `bash scripts/verify-exhausted-task-delivery-evidence.sh --phase preparation`
from this repository. D1 is verified native; D2 remains pending. The schema-2
record retains the original check digests and tested source digest rather than
attributing old checks to later edits. The current source digest additionally
covers the preparation validator, its records-command adapter, tests and refusal fixtures. Only those
four named source paths may differ from the retained tested candidate; any
implementation, test-harness or scope drift requires fresh implementation evidence.
Git objects and every validation input are repository-local. No external checkout,
owner database, temporary scratch directory or network request is required to
validate the saved package. Temporary paths in transcripts identify the runs;
they are not validation inputs.

The structured `case_outcomes` enumerates all 35 exact native cases; the existing
case matrix supplies each setup, assertion and falsifier definition. Structured
`falsifier_outcomes` binds 23 actual removal outcomes to their retained logs:
12 review controls, six correction controls, four public-command controls and
the original exact-exhaustion control. The validator rereads event run/pass
pairs, source hashes, coverage totals, all-mode log hashes, four finding
dispositions, scratch transcript and saved SQLite reload payloads. Original
receipt hashes are also checked against the retained commit, preventing a later
edit from silently replacing supplied evidence.

The [scratch transcript](verification/task-correct-transcript.txt) records the
built executable's planned and no-plan journeys: operator/flag/reference and
binding denials, explicit authorization, scoped queue, reopened correction,
original receipt retention, four compiled falsifiers and a restored-source rerun.
It is preserved byte-for-byte, including actual argv and exits. This stage walks
the preparation and refusal CLI paths against those persisted receipts; it does
not claim to rerun the prior product journeys.

The preparation self-test includes 24 persisted refusal fixtures and native handoff boundary controls.
Preparation succeeds with no coordinator export; `--phase delivery` exits 1 with
`actual coordinator merge evidence required`. A Console revision or checkout
note cannot substitute for that export. `--merge-evidence` accepts only a local
file and validates coordinator identity, exact clean candidate, required gate,
review and owner-decision references, observed default branch and local merge
objects, with ancestry or identical-tree evidence for squash/rebase. These are
trusted coordinator exports, not cryptographically authenticated attestations.
No supplied merge receipt was found or invented; the verifier does not publish,
request approval, merge or deploy.

Compatibility evaluation for this preparation stage: only documentation,
evidence validation and its tests change. Product Go/UI source, task storage,
project loading, `.uaos` migration and `.openexec/tasks.json` fallback are unchanged
from the tested candidate. Complexity delta: no runtime concept, state, transition,
loop or owner decision; existing repository validators and receipts are reused.
Fresh host lint and test both exited 0 (Go vet/tests, UI ESLint and 635 UI tests);
their scope is unchanged Go/UI source, while the final Python validator is tested
by the preparation self-test. These receipts are retained separately as `preparation-lint.json` and `preparation-test.json` in the evidence directory.

## Post-queue Agent Console handoff (T-US-012-002)

Preserve feature `ad5a70cd0924dae18de345dbdf74e623`, the candidate branch above,
[pull request #80](https://github.com/openexec/openexec/pull/80), and review
`afa2fdef5783532f6034a12a002ab508`. These are retained identities from the accepted
plan, not a fresh assertion of the remote PR or review state. The retained tested
revision above anchors implementation evidence; subsequent preparation and
handoff commits must be bound to the exact published PR head by Console.

Agent Console owns candidate commits, canonical gate, publication, independent
review and review resolution, and presentation of the **exact owner merge
decision**, subject to actual effect authority. The owner makes that decision.
Console must preserve the existing decision boundary and candidate identity;
new changes invalidate an earlier exact-head approval. Local task commits only
persist authorized work. This handoff neither approves a merge nor creates a
native delivery, acceptance, publication, review-request or duplicate HITL task.

The persisted accepted-plan snapshot in the evidence directory is checked for
internal-only dependencies, AFK tasks and preparation-only verification. The
selected task command now routes `--record delivery` to the finalized package
validator and enforces every evidence obligation, including Console ownership
and stage separation. All its preparation flags strengthen the same validation;
none bypass a check. Preparation completes without an export and prints **D2
pending**. Queue completion is not Goal completion.

After queue completion, Console supplies a repository-local trusted export and
runs this separate command (replace the path only with the actual export):

```sh
python3 scripts/verify-exhausted-task-records.py --record delivery --require-provenance --require-coordinator-merge-evidence --merge-evidence docs/verification/exhausted-task-delivery-results/coordinator-merge.json
```

No such receipt is currently supplied. Without `--merge-evidence`, the same
command must refuse with `actual coordinator merge evidence required`; it is
never a native task dependency or verification gate.

The export contract is the existing `check_merge` plus `validate_merge`:
`issuer=agent-console`, retained feature, `repository=openexec`, retained PR URL,
`status=merged`, exact clean `candidate_revision` and `candidate_files_sha256`,
actual `console_revision`, `merge_revision`, `default_branch_revision`, actual
`default_branch`, fresh timezone-bearing `observed_at` (within 24 hours),
`owner_decision_ref`, `independent_review_ref`, `canonical_gate_ref`, and
`merge_ref`. It requires true `candidate_in_default_branch`,
`exact_candidate_reviewed`, `exact_candidate_approved`, `canonical_gates_passed`,
and empty `unresolved_findings`. These are trusted Console claims, not self-issued
proof or an authorization mechanism.

Console must observe the repository and default branch, fetch the corresponding
Git objects, and retain the merged PR observation. Validation checks the origin
URL, origin/HEAD, exact observed branch tip, and merge membership in that tip.
`integration=ancestry` requires the verified candidate to be an ancestor of the
merge; `integration=tree` requires identical trees for squash/rebase. A PR URL,
Console serving revision, local test pass or prose claim cannot substitute for
that chain. No branch name, merge SHA, decision reference or approval is invented.

This stage reuses the existing evidence validators and accepted native plan.
Live Project context and the Simple Loop contract were read. Complexity delta:
no runtime concepts, state, transitions, effects or owner decisions added.
Compatibility: no product code, loading, migration or legacy fallback changed.

Fresh handoff verification: the exact selected task script exited 0 and reported
D2 pending ([receipt](verification/exhausted-task-delivery-results/handoff-task.json));
the separate merge check exited 1 for absent coordinator evidence
([refusal](verification/exhausted-task-delivery-results/handoff-merge-refusal.json)).
The preparation self-test exited 0: eight methods, including 24 persisted evidence
refusals, five native-boundary mutations, missing receipt fields, wrong repository,
wrong default branch, wrong retained PR, absent candidate ancestry and dirty
candidate refusals ([log](verification/exhausted-task-delivery-results/handoff-tests.txt)).
These tests add coverage for the accepted boundary; no existing assertions changed.
Host `run_declared_check lint` exited 0 (Go vet and UI ESLint;
[receipt](verification/exhausted-task-delivery-results/handoff-lint.json)).
`git diff --check` passed. The read-only Go module-stat-cache warning did not
change these exits. Full canonical gates and actual delivery remain post-queue
Console work. Persisted JSON and Git state were reread after the handoff commit.
