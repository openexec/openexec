# OpenExec Architecture

Current validation: US-015 / T-US-015-001, 2026-09-30, accepted G-007.
Final evidence and finding dispositions: [delivery evidence](verification-evidence-delivery.md).
Prerequisite study: US-010 / T-US-010-001.
Exact candidate/Console identities, evidence limits, all review checklists and
required implementation proofs live in [repair study](verification/repair-study.md).
The [requirement register](verification/repair-requirements.json) preserves exact
accepted clauses, backed by the [read-only contract snapshot](verification/repair-study-contract.json).
Earlier US-007/008/009/011 reports describe G-006 and their own fixture scopes;
they do not establish the incident or actual Console integration as repaired.

## Context and scope

Outcome: preserve failed verification evidence so native repair can reproduce
the actual check, and recover legacy diagnostic-free failures with bounded
recapture. Current code retains failed results, private bounded diagnostics and
SQLite receipts. The public attachment API and empty-phase named-check recapture
are implemented; current proof is in [admitted evidence](verification/admitted-evidence.md)
and [named recapture](verification/named-recapture.md). Private storage and the
strict 16-member aggregate are locally verified. Actual Console adoption is
deferred until this API merges; D2 remains incomplete pending a candidate-matched
Console merge receipt. Source inspection is distinct from runtime attestation.

The existing native task loop owns implementation, verification, retries and
repair. Console owns admission/effects, outer Goal review and delivery. Reuse
StageResult, run_steps/artifacts, counted task attempts and CreateFailureRepair;
no new scheduler, recovery queue, state machine or authority concept is needed.
Follow the [Simple Loop contract](OPENEXEC_SIMPLE_LOOP_ARCHITECTURE_CONTRACT.md).
Complexity delta: concepts added/removed 0; new persistent state, transitions,
owner decisions and runtime failure modes 0; existing machinery replaced none.
This validation stage changes verification and documentation only; production behavior is unchanged.

Read root/docs instructions, NOTES and [project intent](../PROJECT_INTENT.md).
Fresh Console Project context reports accepted Goal/Ready revision 4 and
interpretation 10 (Professional Portfolio Stewardship). The selected G-007 and
US-010 ledger contract narrow this work to the study. The candidate ledger was
read via SQLite mode=ro; no tasks, decisions or runtime state were changed.

## Module map and APIs

The table is also the discovery verifier's source-reference manifest. Each row
names an existing file and a declaration manually inspected for this task.

| Source | Declaration | Responsibility |
| --- | --- | --- |
| `cmd/openexec/main.go` | `main` | CLI entry; commands live in internal/cli. |
| `pkg/manager/scheduler.go` | `ExecuteTasks` | RunOptions selects the task-oriented route. |
| `pkg/manager/manager.go` | `Config` | StageExecutor injection into pipeline configuration. |
| `pkg/runtime/execution.go` | `VerificationCommandFailure` | Public admitted interface and typed command classification. |
| `pkg/runtime/evidence.go` | `RetainCommandEvidence` | Public private-capture API; typed evidence attachment still missing. |
| `internal/execution/evidence/capture.go` | `Buffer` | Bounded prefix/tail capture; US-012 exercises persisted diagnostic tails. |
| `internal/execution/evidence/capture.go` | `Read` | Content-addressed private reader; US-014 owns safe storage and old-reference policy. |
| `pkg/manager/task_recapture.go` | `resolveRecaptureCommand` | Registered shell/verify-script resolution; single-gate phase inference and current named-check fallback. |
| `internal/cli/init.go` | `ensureGitignore` | Target managed ignore block; currently excludes state but not sibling evidence directory. |
| `internal/blueprint/stage.go` | `StageResult` | Output, Error, Diagnostics, Artifacts, Attempt and timing. |
| `internal/blueprint/stage.go` | `StageExecutor` | Execute(context.Context, *Stage, *StageInput) (*StageResult, error). |
| `internal/blueprint/engine.go` | `ExecuteStage` | Single-stage call; retains non-nil result on error. |
| `internal/blueprint/engine.go` | `Execute` | Full blueprint; retains non-nil result on error. |
| `internal/pipeline/admitted_executor.go` | `Execute` | Passes result/error and captures trusted error receipt separately. |
| `internal/pipeline/pipeline.go` | `terminalEvidence` | Terminal deterministic failure receipt, distinct from worker artifacts. |
| `internal/execution/gates/failure.go` | `CheckFailure` | Classification consists of gate and exit_code only. |
| `internal/execution/gates/failure.go` | `NewCommandFailure` | Only direct normal failed command exits qualify. |
| `pkg/manager/events.go` | `consumeEvents` | Persist terminal evidence before publishing terminal state. |
| `pkg/manager/events.go` | `writeRunStepAsync` | Activity telemetry; not the synchronous repair handoff. |
| `pkg/manager/task_failure.go` | `persistTaskVerificationFailure` | Persist and re-read evidence bound to the task attempt. |
| `pkg/db/state/task_failure.go` | `RecordTaskFailureStep` | Atomic run_steps insertion and task failure/evidence binding. |
| `pkg/manager/task_failure.go` | `repairTaskFromRetainedFailure` | Re-fetch and validate retained evidence before repair generation. |
| `internal/release/failure_repair.go` | `CreateFailureRepair` | Idempotent same-story prerequisite and original-task continuation. |
| `internal/release/failure_repair.go` | `RunnableTasks` | Dependency, story scope, HITL and attempt eligibility. |
| `pkg/manager/task_queue.go` | `executeTaskQueue` | Sequential queue; durable attempt charged before dispatch. |
| `pkg/manager/task_execution_lock.go` | `reconcileInterruptedTasks` | Fresh-queue reconciliation under exclusive workspace ownership. |
| `pkg/agent/openai_provider.go` | `OpenAIProvider` | API provider implementation; execution is not CLI-only. |

Other subsystems: internal/loop manages native execution and subprocesses;
pkg/db/state stores runtime records; internal/release owns tasks/dependencies;
ui/src is the React UI and ui/e2e its browser tests. See
[light mode](LIGHT_MODE.md), [skills](SKILLS_SYSTEM.md),
[security](SECURITY_MODEL.md) and [symbol tools](SYMBOL_TOOLS_REVIEW.md)
for their separate contracts. These are not new evidence/recovery owners.

## Data flow and observed gaps

1. Manager.ExecuteTasks selects explicitly scoped runnable tasks, acquires the
   workspace lock and persists an incremented task attempt before dispatch.
2. Manager Config.StageExecutor reaches pipeline.Config.StageExecutor and the
   admitted wrapper. The public runtime aliases expose the blueprint types.
   With injection enabled, native executor/host-gate fallback is bypassed.
3. The executor returns a StageResult and possibly an error. Both engine APIs
   retain non-nil results on error and mark them failed, preserving supplied
   output, artifacts, diagnostics and timing. Execute synthesizes missing
   results; ExecuteStage preserves its nil-result error return. ExecuteStage
   records and returns non-nil failed results alongside the error.
4. Engine.Execute adds its result to Run.Results and PreviousStages before the
   completion callback. Pipeline terminal failure events carry the last stage's
   identity, output, diagnostics and artifact references. Receipt keys are
   stripped from those artifacts; only the private trusted receipt authorizes
   repair, and cancellation suppresses that receipt.
5. consumeEvents synchronously invokes persistTaskVerificationFailure before
   exposing terminal status. RecordTaskFailureStep commits run_steps metadata
   and tasks.metadata.verification_failure_evidence atomically for the matching
   in-progress attempt. Artifact references are saved synchronously first;
   the manager reads the step back. Async stage telemetry
   excludes stage-failed events and cannot substitute for this handoff.
6. repairTaskFromRetainedFailure validates task ownership, failed status, agent
   and receipt, then includes retained evidence and artifact references in the
   diagnosis. CreateFailureRepair
   preserves story/candidate identity and makes repair a prerequisite of the
   original task. RunnableTasks selects repair before ordinary pending work.
7. T-US-008-001's retained-result verifier runs an admitted exit-2 fixture through
   both engine APIs and pipeline persistence, then closes/reopens the database
   before repair creation. It checks exact argv/cwd, bounded stdout/stderr,
   a diagnostic marker and a usable artifact reference. T-US-008-002 adds
   production bounded capture and private artifact access through
   internal/execution/evidence, exposed to admitted adapters by pkg/runtime.
   Native configured checks and gates use the same helper. The evidence-boundaries
   verifier covers secret-bearing oversized streams, fixed toolchain version
   keys, owner-only file/directory modes, tamper/path/symlink refusal, and
   failure/success/refusal/cancellation/nil-result outcomes after database reopen.
   Public summaries are redacted; exact argv/cwd and bounded raw streams remain
   privately resolvable. Independent coverage and mutations remain assigned to
   T-US-008-003 and T-US-008-004.
8. T-US-009-001 implements diagnostic-free legacy receipt recognition,
   authoritative command resolution and bounded single-stage recapture through
   the native queue/pipeline. Existing task attempts persist the bound; unusable
   receipts cannot repeatedly create repairs. Resolution limits, terminal/reload
   evidence and reusable fixtures are in [legacy recapture](verification/legacy-recapture.md).
   Independent boundary, coverage and compatibility proof remain assigned below.

The [inspection evidence](VERIFICATION_FAILURE_INSPECTION.md) contains the
preceding task's detailed source trace and controlled native journey results.
Those results are historical evidence. Current study findings and proof gaps are
linked above; no fresh product-repair proof is claimed by this stage.

## Accepted requirement mapping

G-007 supersedes the old normalized G-006 mapping. Each exact clause has one
owner in the linked requirement register; D1 and D2 belong to REQ-004.

| Label | Accepted clauses and proof | Owner |
| --- | --- | --- |
| REQ-001 | Exact US-012 contract: preserve failed results, admitted command identity/cwd and usable repair reference; actual Console adoption and tails required. | US-012 |
| REQ-002 | Exact US-013 contract: bounded native recapture of diagnostic-free legacy receipts; no new loop/state machine or owner decision. | US-013 |
| REQ-003 | Exact US-014 contract: classification/provenance separate from private diagnostics; no indiscriminate environment or secret logging. | US-014 |
| REQ-004 | Exact US-015 contract: D1 reproduces and fixes failure; D2 verifies merge to default branch. D1 local proofs and finding dispositions are in the delivery evidence; D2 remains incomplete. | US-015 |

## Boundaries and conventions

- Classification is not diagnosis: CheckFailure and its digest prove a recorded
  normal exit, not its cause or provenance. Only the trusted configured command
  boundary may originate a receipt; worker prose/artifacts cannot authorize repair.
  Missing knip or present-day Settings hook drift cannot establish the historical
  unknown lint exit-2 cause.
- NewCommandFailure accepts direct exec.ExitError codes 1–125 with live context;
  launch/transport/refusal, cancellation, unknown errors and mixed error trees
  do not become verification repair authority. Preserve nil-result errors and
  success semantics. Nil/nil is not a documented successful adapter contract;
  resolve it explicitly before changing behavior.
- Private diagnostic evidence is separate from classification fingerprints.
  Exact private command identity must remain resolvable without exposing secret
  values publicly. Bound stdout/stderr; redact command values and diagnostics;
  explicitly allowlist toolchain metadata, never dump the environment.
  Capture retains 4096 bytes per stream (first/last 2048 on overflow) with truncation flags; toolchain input
  accepts only go_version, node_version, npm_version, python_version and
  rustc_version, each capped at 128 bytes. Admitted adapters use EvidenceBuffer,
  RetainCommandEvidence and PublicVerificationStream; they supply known secret
  values from admission/configuration because arbitrary prose secrets cannot be
  inferred reliably. Common credential assignments are additionally redacted.
  Truncated public streams omit cut lines at both sides of the omitted middle
  and the incomplete final line, reserving space for the truncation marker. Exact private payloads
  live in .openexec/data/verification (0700), covered by initialization’s data ignore,
  with content-addressed files
  (0600); ReadCommandEvidence requires matching hashes and rejects public modes,
  traversal and symlinks at every nested component. Recapture requires a registered
  reference to that exact location. Root-level legacy files are explicitly refused
  without deleting their files or ledger references; see
  [private storage policy](verification/private-storage.md).
  No public evidence-content endpoint is introduced.
- Engine stage retry counters/MaxTotalRetries differ from persisted task attempts.
  Fresh-queue reconciliation already reopens eligible receipt-free failures once
  under the lock; it is broader than diagnostic-free recapture. Do not reset
  attempts, create another retry budget or bypass Stop/pause, HITL, admission,
  candidate branch, spent attempts, dependencies or selected story scope.
- Keep Settings waiting until its existing dependency completion conditions hold.
  Preserve `.openexec`, legacy `.uaos` and `.openexec/tasks.json` fallbacks.
- Go uses gofmt, lowercase packages and colocated *_test.go files; UI uses
  TypeScript/React, PascalCase components and camelCase helpers. Tests use Go
  testing, Vitest and Playwright. Stable symbol references are preferable to
  stale line numbers. SQLite is durable task truth; generated reports are evidence.
- Console owns publication, canonical gate, independent review, owner presentation
  and merge execution after the queue. Preserve T-US-011-002 and its dependency
  on T-US-011-001 verbatim. No new delivery/approval task or decision_ref.
  Current Console source/dependency and the separately supplied process observation
  are recorded once in the study provenance; deployment and D2 remain unverified.

## Evidence ownership

The historical G-006 slice implementations and evidence below exist in the candidate.
Current G-007 repair ownership is US-012/013/014; US-015 owns final aggregation.
US-010 owns the study dispatcher, documentation and study verifier tests.
Only serial implementation/aggregation owners edit the shared dispatcher
`scripts/verify-retained-verification-evidence.sh`. Independent verifiers run
standalone helpers, consume completed production/shared fixtures directly and
never require sibling reports. Dedicated test names and helper functions must
also be unique within a Go package. Additional package-local unit files use the
same reserved basename; do not edit another owner's tests to share helpers.

| Task | Owned test files / fixture namespace | Owned verifier helper or dispatcher cases | Evidence |
| --- | --- | --- | --- |
| T-US-007-002 | `scripts/verification/test_discovery.py` | `scripts/verification/discovery.py`; discovery | This document; NOTES.md |
| T-US-008-001 | `pkg/manager/retention_journey_test.go`; `pkg/manager/retention_fixture_test.go` | retained-result | Integrated admitted exit-2 fixture and reload assertions |
| T-US-008-002 | `pkg/manager/retention_boundaries_test.go`; extends retention_fixture_test.go serially | evidence-boundaries | Shared completed privacy/boundary inputs for both independent consumers |
| T-US-008-003 | `internal/blueprint/retention_unit_test.go`; `pkg/manager/retention_unit_test.go`; package-local retention_unit_test.go | `scripts/verification/retention-unit-coverage.sh` | `docs/verification/retention-unit-coverage.md`; baseline and all changed functions |
| T-US-008-004 | `pkg/manager/retention_mutation_test.go` | `scripts/verification/retention-mutations.sh` | `docs/verification/retention-mutations.md`; isolated mutations, expected assertion failures only |
| T-US-008-005 | No sibling test edits | retention-unit-coverage, retention-mutations, retention-story | docs/verification-evidence-retention.md; manifest and aggregate |
| T-US-009-001 | `pkg/manager/recapture_journey_test.go`; `pkg/manager/recapture_fixture_test.go` | legacy-recapture | Shared restart, resolution, receipt and terminal-state fixtures |
| T-US-009-002 | `pkg/manager/recapture_boundaries_test.go` | `scripts/verification/recapture-boundaries.sh` | `docs/verification/recapture-boundaries.md`; persisted termination/restart and Settings waiting |
| T-US-009-003 | `pkg/manager/recapture_unit_test.go`; package-local recapture_unit_test.go | `scripts/verification/recapture-unit-coverage.sh` | `docs/verification/recapture-unit-coverage.md`; full-function scope/results |
| T-US-009-004 | `internal/validation/recapture_compatibility_test.go`; `pkg/manager/recapture_compatibility_test.go` | `scripts/verification/recapture-compatibility.sh` | `docs/verification/recapture-compatibility.md`; isolated protected-format fixtures/results |
| T-US-009-005 | No sibling test edits | recapture-boundaries, recapture-unit-coverage, recapture-compatibility, recapture-story | docs/verification-evidence-recapture.md; manifest and aggregate |
| T-US-011-001 | Earlier composed native journey | Historical full, delivery-ready | Superseded by US-015 aggregate below |
| T-US-015-001 | Strict aggregate and external receipt controls | full, delivery-ready, goal-complete | docs/verification-evidence-delivery.md; 16 members, four strict coverage slices, deferred adoption and incomplete external D2 |

Retention unit coverage and mutations independently consume T-US-008-002.
Recapture boundaries, unit coverage and compatibility independently consume
T-US-009-001. Each helper exits nonzero on failure, absent instrumentation or
missing/skipped required scenarios; no echo-based soft success. Mutation proof
rejects compilation/unrelated failures. Aggregators compose their own slice's
helpers only after completion and preserve every subprocess failure. Per-slice
reports must map all criteria to exact commands, statuses, assertions, coverage,
reload/restart evidence and blockers. Aggregation cannot defer essential proof.

## Discovery verification

Run from any directory:

```sh
bash scripts/verify-retained-verification-evidence.sh --case discovery
python3 -m unittest discover -s scripts/verification -p 'test_discovery.py' -v
```

Discovery checks required sections, normalized mapping, exclusive verifier/test
ownership, source declarations and local documentation links. Unknown or malformed
cases fail closed. Discovery does not execute retention/recapture verification
or certify D1/D2. Historically inspected both engine discard branches, admitted
wrapper, receipt classifier, pipeline callbacks/terminal handoff, manager writes,
atomic state transaction, repair generation and queue reconciliation at baseline.
Remaining module declarations were checked against their source definitions.
The discovery run passed with 22 declarations; all nine verifier tests passed,
including missing source/declaration, omitted source, incorrect/duplicate mapping,
shared helper, broken link and unknown/pending/malformed dispatcher refusals.
Bash syntax validation and git diff --check passed. Tests re-read isolated files
from disk, and discovery re-read the saved candidate documents. The first run
caught an incorrect StageExecutor source path; the manifest now points to its
actual declaration in stage.go.

Repository commands include make build, make lint, make test, make compat-test
and make type-check. Recovery/legacy product changes require targeted regression
and compatibility coverage plus required repository checks. Canonical gates run
later in Console's socket-capable repository runner. This checkout has no
make check or make pr-gate target; delivery must record the actual command mapping.
This documentation/script-only stage changes no project loader, schema or runtime
behavior, so protected-format support is unchanged. Full repository gates and
live Console-to-OpenExec delivery are not claimed here.
