# Verification repair study — US-010 / T-US-010-001

## Scope and provenance

This is completed prerequisite discovery for G-007, not completion of repairs,
D1, review resolution or D2. The selected task's read-only ledger script is
`bash scripts/verify-verification-repair.sh --case study`. The exact accepted
clauses are preserved in [contract snapshot](repair-study-contract.json), with
one owning story per [derived requirement](repair-requirements.json).
[Architecture](../ARCHITECTURE.md) is the current module/persistence map.

[Provenance](repair-study-provenance.json) records the initially clean retained
candidate, its full parent revision/tree, branch, feature, existing PR and review
identity. It also records exact Console source revision, file SHA-256 values,
dependency and separately labelled owner-supplied process observation. A commit
of this study adds only documents/verifiers to that parent. Git history records
its final identity; no self-referential commit hash is embedded in the document.

Read Project context using openexec_get_project(project="openexec"): accepted
Goal/Ready revision 4, interpretation 10, Professional Portfolio Stewardship.
Read AGENTS.md, AGENTS.local.md, docs/AGENTS.md, NOTES.md, PROJECT_INTENT.md and the
Simple Loop contract. Read selected G-007/US-010/012/013/014/015 SQLite rows with
mode=ro. Do not substitute older stories.json or G-006 for these exact clauses.
No owner-only NOTES items or other tasks were executed.

## Source and dependency assessment

The Console main checkout is dirty and older than the observed process; it is
not sufficient deployment evidence. Its linked clean `/mnt/data1/ac-probe`
worktree matches the full revision prefixed by the supplied serving observation.
There, `internal/server/openexec_checks.go:executeOpenExecCheck` resolves
`stage.Name` with `taskCheckCommand`, ignores worker `stage.Commands`, sets cwd
from the admitted candidate, combines stdout/stderr in openExecCheckOutput,
and calls only runtime.VerificationCommandFailure for lint/test errors.
`openexec_check_conventions.go` selects an owner profile, then repository
Makefile/npm/toolchain conventions. Command identity is resolved at execution.

The inspected go.mod and go.sum pin the OpenExec revision listed in provenance;
there is no replace directive. The pin predates this candidate's retention work.
A clean source diff against that Console HEAD verifies the inspected files are
committed source. Neither inspected Console checkout uses the evidence-bearing
API. This is a confirmed integration gap in inspected source, not a claim that
no other checkout anywhere contains work, nor proof about linked binary module
contents. The reported process revision/start is not independently attested here.
No binary build-info, deployment receipt, live incident database, or live
Console execution was supplied/read. The incident row shape comes from the
review; it has not been independently recovered from the production ledger.

The manager retentionFixture already uses only public runtime APIs: it places
references in StageResult.Artifacts and returns VerificationCommandFailure.
Pipeline.runBlueprintMode copies these non-receipt references into the terminal
event alongside the independently trusted receipt. Other boundary fixtures use
internal/gates. Thus the broad claim that external adapters have no attachment
path is incorrect; the missing typed-error attachment helper and actual Console
adoption remain accepted gaps. Fixture source does not establish adoption. Buffer
currently drains into a 4096-byte prefix; Public also bounds to a prefix. Console
retains a suffix before the admitted wrapper truncates it again. Substituting
EvidenceBuffer alone would still lose late diagnostics: US-012 must address
bounded tail retention/redaction and prove a marker beyond 4096 bytes survives.

## F1 — Legacy incident recapture

Disposition: accepted defect. T-US-013-001 now repairs phase inference and
named-stage dispatch with an independent incident journey and both runtime
falsifiers; current proof is in [named recapture](named-recapture.md). Remaining
story-wide coverage and boundary expansion are pending US-013. The root-cause
description below records the prerequisite baseline.

- **Root cause:** `pkg/manager/task_recapture.go:resolveRecaptureCommand` requires
  exactly one gate equal to phase, then a registered original shell command or a
  verify/verification script. Empty phase fails equality; matching lint/test with
  empty script fails resolution. `repairTaskFromRetainedFailure` reaches this
  resolver before any admitted executor can resolve a named check. The terminal
  outcome becomes unresolved/needs_review. The old fixture explicitly asserts
  lint-without-reference refusal, so green legacy tests do not refute the finding.
- **Cases:** F1-empty-lint: Phase="", sole lint exit 2, no output/ref/script,
  exact T-US-001-004-13 shape. F1-empty-test: same with test. F1-matching: lint
  and test with matching phases and empty scripts. F1-admitted: stage-name-only
  executor ignores Commands. F1-native: authoritative blueprint definition.
  F1-success: successful recheck resumes original completion obligation.
  F1-restart: remaining/spent budgets survive reopen. F1-boundaries: ambiguous
  multi-check, mismatched nonempty phase, unknown gate, HITL (including legacy
  metadata-free HITL), cancellation, launch refusal, stale ownership, dependencies,
  out-of-scope story, corrupt/unregistered/foreign reference and exhausted budget.
- **Fix:** infer phase only from a sole valid receipt gate when phase is absent;
  retain rejection of a conflicting phase. Reuse the existing RecaptureStage,
  StageExecutor and native queue. With no historical reference, lint/test use an
  admitted named check or an authoritative blueprint definition; never worker
  text or a host fallback under injection. Persist current-check-definition
  provenance and the command actually executed; do not call it historical argv.
  Pipeline.runBlueprintMode also currently requires exactly one Commands entry
  for RecaptureStage; adjust that validation narrowly for admitted named checks
  while retaining native authoritative command validation. Preserve attempts,
  receipt ownership, dependency predicates and Stop/admission.
- **Prove:** seed the exact incident in SQLite with empty script/output/ref,
  reopen, run executeTaskQueue through its public entry. Require exactly one
  lint subprocess exiting 2 with stderr marker; reread new bound run_step and
  registered private evidence. On the next queue iteration require one same-story
  repair whose description contains the marker and usable reference, outcome not
  unresolved, Settings pending/zero attempts. Repeat empty/matching lint/test and
  name-only dispatch variants. Exercise success, all refusal cases and restart;
  reread attempts/dependencies/HITL identity/reason/decision reference unchanged.
- **Falsify:** in separate temporary copies remove only empty-phase inference,
  then restore unconditional empty-command refusal. Each baseline must pass;
  mutated incident must fail the intended assertion with zero fixture calls and
  unresolved/needs_review. Compile failures and unrelated failures do not count.

Qualification: reject the review's literal suggestion that only multi-check,
HITL and budget exhaustion may stop. Cancellation, launch/admission refusal,
unsafe references and stale ownership must not be converted into code-repair
permission. The accepted US-013 clauses explicitly preserve these boundaries.
Do not infer the historical lint cause from current knip or Settings hook state.

## F2 — Public admitted evidence attachment

Disposition: accepted defect. The public attachment and silent persisted tracer
are now implemented by T-US-012-001; current proof is in
[admitted evidence](admitted-evidence.md). Local diagnostics and coverage are
verified; actual Console private capture/attachment remains pending US-012.
Source availability is not write authority. The root-cause description below
records the prerequisite baseline.

- **Root cause:** `pkg/runtime/execution.go:VerificationCommandFailure` delegates
  only to NewCommandFailure. RetainCommandEvidence is public, but attachment to
  the typed failure is internal/gates.CommandFailureWithEvidence. The admitted
  wrapper extracts that private receipt separately from worker artifacts. Console
  has exact argv/cwd in local variables but returns only a textual header/output.
- **Cases:** F2-lint-test: actual Console lint/test failure with exact cwd/argv.
  F2-silent: real silent exit 2 requires structured evidence despite header text.
  F2-tail: >4096-byte stdout/stderr with diagnostic marker near end. F2-results:
  populated-result+error in both engine APIs, nil+error, success, nil/nil semantics.
  F2-boundaries: cancellation, launch/transport refusal, mixed errors, forged
  worker artifacts, storage refusal, token-bearing argv/output and allowlisted
  toolchain context. F2-adoption: actual Console source, dependency and execution.
- **Fix:** export VerificationCommandFailureWithEvidence(ctx,name,err,hash,path)
  via the existing gates mechanism. Reuse runtime capture/read APIs and preserve
  bounded diagnostic tails through public summaries and private capture. Console
  must capture streams, retain exact argv/cwd/exit and attach the reference using
  the public API after dependency update in its authorized checkout. Avoid
  importing internal packages or granting classification authority to artifacts.
- **Prove:** public-runtime-only fixture runs silent lint exit 2; terminal
  EventBlueprintFailed contains a 64-hex ref, manager persistence registers it,
  reopen retains it, diagnosticFreeReceipt is false, ReadCommandEvidence returns
  exact argv/cwd==WorkDir/exit 2. Add marker and oversized separate-stream cases
  through repair creation/reload; verify redaction/truncation and toolchain
  allowlist. Preserve refusal/success/error matrix. Independently run the actual
  Console adapter with its updated dependency; fixture-only success is not
  adoption or delivery-ready. Identify source versus serving binary separately.
- **Falsify:** replace new wrapper with NewCommandFailure; silent fixture must
  fail missing-reference/diagnostic-free assertions. Independently restore each
  engine discard branch; engine and persisted repair assertions must fail. A
  prefix-only buffer/public summarizer must fail the late-marker assertion.
  Baseline pass, isolated mutation and exact assertion failure are mandatory.

Qualification: reject the absolute no-public-path claim: StageResult.Artifacts
already transports references independently of error attachment, as described
above. The required new wrapper must still prove typed receipt attachment; its
silent falsifier must not also seed the same reference in result.Artifacts, or
the fallback path would mask removal of the wrapper. A Console header alone makes
today's diagnosticFreeReceipt false,
so the silent test must assert a registered reference explicitly and use no
artificial header in the attachment falsifier. An absent reference is not
repaired merely by keeping text. A public wrapper alone cannot complete US-012.

## F3 — Private evidence enters source commits

Disposition: accepted, reproduced and repaired by T-US-014-001. Current storage policy,
staging reproduction and verification are recorded in
[private storage](private-storage.md). Unix permission bits do not prevent Git publication.

- **Root cause:** originally, evidence.Write created .openexec-verification at the repository
  root. Only this repository ignores it. ensureGitignore manages .openexec/data,
  logs and cache; old initialized targets and Console-style .openexec ignores
  do not cover the sibling directory. safe_commit stages `git add .`, so raw
  argv/stdout/stderr become source candidates despite public redaction.
- **Cases:** F3-fresh: initialized temporary repo with committed initialization
  files. F3-existing: old managed block ignoring .openexec/data, no reinit.
  F3-console: repository ignoring .openexec/. F3-gates and F3-deterministic:
  both real failed command paths; token=abc stderr and credential-bearing argv.
  F3-staging: status/clean gates and git add . exclude evidence. F3-legacy:
  registered old references, unregistered/foreign paths, malformed hashes,
  symlink parents/files, public permissions, corrupt payload and missing file.
- **Fix:** new evidence belongs under .openexec/data/verification, 0700/0600.
  Update secure nested creation, reader and resolution expectations together;
  changing a constant alone is insufficient because current Mkdir is one level.
  Define bounded legacy handling: only registered validated owner-private files;
  migrate to protected state and update references safely, or explicitly refuse
  with preserved ledger evidence. Never broaden arbitrary path reads or leave
  migrated raw source files stageable. No mandatory target reinitialization.
- **Prove:** each repo variant has a clean committed baseline; ensureGitignore
  is exercised in CLI package tests for fresh/existing initialization. Run real
  gate and deterministic exit-2 processes, reload evidence with raw token stderr,
  assert empty git status --porcelain --untracked-files=all. Execute git add .
  and inspect the index to prove no payload/credential argv is staged. Reopen
  database and resolve both new refs and policy-supported old refs. Assert all
  permission/path refusals and that public events, repair summaries and receipt
  fingerprints exclude fixture secrets. Use isolated repos, not this worktree.
- **Falsify:** restore old unignored directory in a temporary source copy; the
  corresponding clean-worktree/staging assertions must fail naming its hash file.
  Restoring unsafe old-path reads must fail permission/path/secret controls.
  A status check before committing init files is an invalid positive fixture.

## F4 — D2 delivery

Disposition: accepted as an unfulfilled external evidence requirement; no claim
of default-branch merge or non-merge has been independently verified here.

- **Root cause:** candidate commits and old successful local reports do not
  attest exact-candidate merge. This stage has neither merge evidence nor effects.
- **Cases:** missing, malformed, foreign-candidate, wrong-repository/default-branch
  evidence; valid Console merge evidence; study/D1-only success.
- **Fix:** US-015 validates Console-provided merge identity and required gate/review
  provenance. Console owns publication, review resolution and merge execution.
- **Prove:** goal-complete rejects absent/stale/mismatched merge evidence and only
  accepts an externally verified exact-candidate default-branch merge. Preserve
  existing PR and retained owner acceptance task/dependencies/decision reason.
- **Falsify:** remove candidate matching or treat local study success as D2;
  missing/foreign-evidence controls must fail the goal-complete verifier.

## Coverage contract

[Declared scopes](repair-coverage-scopes.json) cover entire production function
bodies, including shared functions in each relevant story. US-012 covers result
preservation, attachment, buffering, classification and persisted repair; US-013
covers resolution, attempts, queue ownership, dependencies and compatibility;
US-014 covers write/read, nested path validation, private modes, staging and
legacy refs. Public attachment is now implemented and traced by T-US-012-001; the
OpenExec full-body coverage obligation is implemented by T-US-012-004 in
[admitted unit coverage](admitted-unit-coverage.md). US-012 separately requires actual
Console adapter execution and coverage evidence in that repository. Existing manifests are starting inventories,
not exemptions for new/changed functions.

Each implementation story must exceed 90% statement coverage, not equal it.
Reject missing/empty profile, scoped source/function/block, stale source hash,
skipped required case, absent added function or denominator drift hiding an
uncovered branch. Recompute instrumentation and denominator against the pinned
baseline and entire current function bodies. Aggregate US-015 composes the three
independent proofs without waiving missing Console integration. No measured
coverage or runtime success is asserted by this study.

## Resources, authority and delivery

Concrete integration blocker: Console files are readable but outside supplied
writable roots; its source/dependency update and adapter journey need an
authorized Console workspace. A recorded blocker does not satisfy US-012.
No running binary/module attestation or current remote merge/gate artifact is
available in this stage. A second review or replacement PR is not requested.
Available connected tools were inspected; local stage work does not invoke
publication, review-resolution, merge or Navigator operations.

US-015 must run required OpenExec gate, make test, make compat-test,
make type-check, go vet ./... and Settings baseline reconciliation. The candidate
has no scripts/verify-settings-baseline.sh and no make check/pr-gate target;
record this command-resource gap and resolve its mapping in the canonical runner,
not silently waive it. Hook drift is separate from the unproven historic exit 2.
Long checks belong to the repository job runner with actual completion evidence.
Socket-capable canonical gates run later in Console's runner, not this sandbox.

safe_commit usage: no safe_commit tool/daemon exists in this stage. Per the
explicit owner override, stage only changed study paths and make an ordinary
local commit with US-010 and T-US-010-001 in the subject after verification.
Do not call the production safe_commit tool or git add . in this candidate.
Console retains canonical candidate publication, review resolution and merge.
Do not invent a new owner task or decision reference; preserve the retained HITL.

Compatibility evaluation: only documentation and Python/shell verifiers change;
no loaders, schemas, receipts or runtime behavior are modified. Existing
.openexec, .uaos and tasks.json support cannot regress from this stage. Future
US-013/014 repairs must execute those protected-format journeys and old-reference
policy cases. Complexity delta: runtime concepts/state/transitions/owner
questions/failure modes added 0, removed 0; no production machinery replaced.

## Study verification

The strict study case checks saved files, exact clauses and single ownership,
all four finding checklists, coverage inventories, module declarations, links,
provenance and pending boundaries. It fails on malformed/unknown cases. Negative
controls remove a source/declaration, alter a clause or owner, duplicate a
requirement, drop a case/checklist section, corrupt provenance, omit a coverage
scope/function, lower its threshold and break a documentation link. These are
study-verifier falsifiers, not executed product-repair falsifiers.

Fresh verification, 2026-09-29:

| Command | Observed result |
| --- | --- |
| `bash scripts/verify-verification-repair.sh --case study` | Exit 0; 70 source declaration checks, exact clauses, all checklists/scopes/provenance and persisted docs/NOTES validated. |
| `python3 -m unittest discover -s scripts/verification -p 'test_*.py' -q` | Exit 0; 74 tests, including 20 new study tests and nine discovery tests. |
| `bash -n scripts/verify-verification-repair.sh` | Exit 0. |
| `git diff --check` | Exit 0. |

The command was also executed from /tmp against a saved isolated fixture, then
executed again after deleting a required source: pass became exit 1 with the
intended missing-source error. Unknown, pending delivery-ready and malformed
arguments returned exit 2. Every other negative control rereads modified files;
malformed/missing/duplicate-key evidence fails closed. Console source SHA-256
values were checked against the recorded commit blobs, not just worktree names.
No historical fixture pass substitutes for this study path. Product runtime
journeys, production mutations and measured coverage were not run in this stage;
the checklists reserve them explicitly for their implementation owners.
