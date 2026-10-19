# OpenExec execution and recovery architecture

## Evidence scope

Updated for US-010 / T-US-010-001 from implementation baseline
`722a7a7f` on 2026-09-29. This is a checkout map, not deployment evidence.
The supplied Project context describes the accepted goal as restoring trusted
persisted deterministic-runner verification failure recovery while rejecting
unauthorized evidence. AGENTS.md, AGENTS.local.md, docs/AGENTS.md and NOTES.md
were read. No separate live Console Project-context endpoint was available;
the supplied briefing and local plan are context, not proof of implementation.

## Restored resources

The earlier discovery baseline lacked `pkg/runtime/execution.go`,
`internal/execution/gates/failure.go` and `pkg/manager/task_failure.go`.
All three exist in the US-010 candidate. The current map below supersedes
those absence claims. Exact final check status belongs in
`docs/runtime-verification-handoff.md`; source existence is not a passing check.

## Module and API map

Source links use Go declaration names after `#`; the verifier resolves these
against declarations, including receiver methods, rather than Markdown anchors.

| Module | Current API / source | Responsibility |
|---|---|---|
| Trusted terminal boundary | [VerificationTerminalFailure](../pkg/runtime/execution.go#VerificationTerminalFailure), [NewTerminalFailure](../internal/execution/gates/terminal.go#NewTerminalFailure) | Caller-owned authenticated loader; immutable terminal and task/stage/attempt/source matching. |
| Failure conversion | [VerificationFailureArtifacts](../internal/execution/gates/failure.go#VerificationFailureArtifacts) | Typed failed checks only; refuses mixed errors. |
| Native repair | [persistTaskVerificationFailure](../pkg/manager/task_failure.go#persistTaskVerificationFailure), [repairTaskFromRetainedFailure](../pkg/manager/task_failure.go#repairTaskFromRetainedFailure) | Durable failed-step receipt and existing same-story repair queue. |
| Public execution | [Request](../pkg/execution/execution.go#Request), [Result](../pkg/execution/execution.go#Result), [Provider](../pkg/execution/execution.go#Provider) | Provider requests, terminal outcome/reason and streaming execution; runtime classification is exposed separately above. |
| CLI transport | [serveExecutionProtocol](../internal/cli/execution_stdio.go#serveExecutionProtocol) | Versioned stdio describe/probe/execute; accepts protocol 1/2/3, refuses replay in version 1. |
| Provider construction | [newConfiguredAPIProvider](../internal/cli/execution_stdio.go#newConfiguredAPIProvider) | Project-selected API configuration and gateway/workspace sandbox restrictions. |
| Stage execution | [executeDeterministic](../internal/blueprint/executor.go#executeDeterministic) | Registered actions or local shell commands produce stage status, output, error and artifacts. |
| Gate execution | [RunGate](../internal/execution/gates/runner.go#RunGate) | Local command execution with timeout, exit code, warning mode and fix hints. |
| Blueprint control | [Execute](../internal/blueprint/engine.go#Execute) | Stage transitions, checkpoints and bounded failure-handler retries. |
| Task scheduling | [ExecuteTasks](../pkg/manager/scheduler.go#ExecuteTasks), [filterAutoDispatchable](../pkg/manager/scheduler.go#filterAutoDispatchable) | Pending dependency graph dispatch; holds HITL tasks and their dependents; cascades failed dependencies. |
| Persistence producer | [writeRunStepAsync](../pkg/manager/events.go#writeRunStepAsync) | Asynchronous event-to-step/artifact/checkpoint writes; errors are logged. |
| State storage | [WriteRunStepWithArtifacts](../pkg/db/state/store.go#WriteRunStepWithArtifacts), [GetRunStep](../pkg/db/state/store.go#GetRunStep) | Store and retrieve structured execution state. |
| Resume | [GetLatestCheckpoint](../pkg/db/state/store.go#GetLatestCheckpoint), [WithResumeCheckpoint](../pkg/manager/manager.go#WithResumeCheckpoint) | Read a run checkpoint and supply it, with applied tool calls, to pipeline configuration. |
| Validation | [LinkValidationEvidence](../pkg/db/state/verification.go#LinkValidationEvidence), [ReadCompletionReport](../pkg/db/state/verification.go#ReadCompletionReport) | Bind validation evidence and reload frozen completion reports. |

## Producer, persistence and reload

The local deterministic executor first tries a registered action, then configured
shell commands. An unknown action without commands, or an empty deterministic
stage, fails. Command failure becomes stage error text; the callback also receives
the original command error. The separate gate runner extracts `exec.ExitError`
exit codes, uses -1 for other command errors, and allows configured warning mode.
These are observed local producers, not authenticated persisted-runner imports.

Manager events can write run steps, artifacts and checkpoints asynchronously.
The inspected step writer selects start/complete events and maps completion events
to `completed`; it does not establish the requested trusted failure producer.
[writeCheckpointJSONL](../pkg/manager/checkpoints.go#writeCheckpointJSONL) and
[writeCheckpointSQLite](../pkg/manager/checkpoints.go#writeCheckpointSQLite)
record run, phase/stage, iteration and artifacts; their write errors are ignored.
A write attempt alone therefore cannot prove durable evidence.

The Store exposes checkpoint and run-step reads, while
[FinalizeEvidenceCoverage](../pkg/db/state/verification.go#FinalizeEvidenceCoverage)
transactionally freezes the first completion report for an accepted validation
revision. Reloading that report is a supported path distinct from importing a
terminal runner failure into native repair. The admitted recovery fixtures now close and reopen SQLite and the manager,
reloading failed-step receipts, completed repair/original/remaining tasks and
replaying evidence without another dispatch. The admitting caller persists and
reloads terminal evidence before classification; its loader remains the
producer authentication and integrity boundary.

## Binding validation

`LinkValidationEvidence` requires an accepted plan and accepted item, matching
worktree-state and patch hashes, a run step belonging to the supplied run,
matching evidence/step status, and an existing artifact when its hash is supplied.
[EvidenceStatusMatchesRunStep](../pkg/db/state/verification.go#EvidenceStatusMatchesRunStep)
maps passed/completed, failed/failed, inconclusive/inconclusive and
not_run or unavailable/unavailable. Coverage also checks the current repository
state; finalization refuses moved state and accepted items without evidence.

These checks bind repository state, plan/item and run/step. They are not proof
of trusted producer identity or of task/stage/attempt/source binding for imported
runner completions. `NewTerminalFailure` checks terminal identity, exact expected binding, terminal
outcome and exit range. The admitted pipeline independently matches the typed
receipt against its native executing task and stage attempts. A source label
alone cannot authenticate a worker-writable record.
Public `Result` has executor/model/sandbox/session/time/outcome/reason fields;
it is not a native repair authorization receipt.

## Native recovery and receipts

Blueprint `Execute` follows `OnSuccess` or `OnFailure`, counts stage retries,
and enforces a total retry limit. [Resume](../internal/blueprint/engine.go#Resume)
resumes a paused run. The restored native task queue additionally consumes a
validated failure receipt, creates and runs one same-story repair, resumes the
original task and continues remaining tasks. The US-008 running matrix covers
A -> repair -> A -> B, receipt-boundary restart and duplicate replay. Refusal
branches reopen storage and require zero unauthorized receipts or repair tasks.
See the candidate-bound US-008 evidence and final US-010 handoff for check status.

## Related tests and verification

Existing source coverage includes
[TestDeterministicStageWithUnregisteredActionFails](../internal/blueprint/executor_failclosed_test.go#TestDeterministicStageWithUnregisteredActionFails),
[TestDeterministicStageWithCommandsStillPasses](../internal/blueprint/executor_failclosed_test.go#TestDeterministicStageWithCommandsStillPasses),
[TestRunner_RunGate](../internal/execution/gates/runner_test.go#TestRunner_RunGate),
[TestAuditIncludesArtifactsAndCheckpointWritten](../pkg/manager/events_artifacts_test.go#TestAuditIncludesArtifactsAndCheckpointWritten), and
[TestValidationEvidenceAndCompletionRefuseInconsistentIrreversibleState](../pkg/db/state/verification_test.go#TestValidationEvidenceAndCompletionRefuseInconsistentIrreversibleState).
Their existence alone is not a passing test result.

Run this task's document/source verification with:

```sh
bash -euo pipefail scripts/autonomy-contract/verify-runtime-evidence.sh --case architecture-contracts
python3 scripts/autonomy-contract/test_architecture_contracts.py
```

The first checks required sections, resolvable Go source declarations and restored
resource references. The second exercises the actual command in isolated
fixtures, including missing documents, broken references and unsupported cases.
Current executed results are recorded once in Discovery verification evidence
below. Existing Go tests listed above were inspected, not run for this
documentation/tooling change.
Documentation checks alone do not prove the engine recovery lifecycle. This documentation and
verifier change does not modify loading, migration, `.openexec`/`.uaos` handling
or runtime behavior; compatibility-sensitive product support is unchanged.

## Accepted obligation traceability

Mapped for US-007 / T-US-007-002 against source baseline
`f598478da3cce87e641659c95d14218b275664e7`. The supplied accepted Goal and
owner request, plus local plan
`.openexec/artifacts/plans/6f65394f312469446aa0ac378593b1293961d0f92c8e5388ca39e3928ff58bb0.json`,
provide the obligation context. That local planning artifact is not a delivery
receipt or an independently retrieved accepted-contract record. D1 and D2 below
use the supplied plan's meanings; no REQ identifiers or new grants are introduced.
Rows are evidence obligations, not assertions of implemented behavior.

| Obligation | Responsible party | Required evidence | Current finding |
|---|---|---|---|
| acceptedContract.goal | OpenExec engine | Trusted persisted deterministic-runner terminal evidence crosses the exported verification boundary into existing native repair; unauthorized failure evidence is rejected. | Unverified: final US-010 rerun pending; prior US-008 recovery evidence exists. |
| D1 engine reproduction and recovery | OpenExec engine | Behavioral pre-fix reproduction and post-fix exported runtime API plus admitted executor regression; actual exit 1, persistence reload, validated task/stage/attempt/source bindings, native repair execution, original-task resumption and queue continuation with durable receipts. | Unverified: final candidate rerun pending; US-008 exercised the admitted lifecycle and reopened receipts. |
| D1 engine refusal and compatibility | OpenExec engine | Exit 0 without repair; qualifying exits 1 through 125; replay/restart without duplicate repairs; refusal of untrusted, missing, stale, tampered or worker-forged evidence, invalid bindings/exits, cancellation, timeout, launch/transport and mixed errors; reopened state contains no unauthorized repair or receipt. Preserve local exec.ExitError and legacy .openexec/.uaos/tasks.json behavior. | Unverified: final candidate rerun pending; US-008 refusal and protected-project checks are recorded separately. |
| D1 engine validation | OpenExec engine | Strictly greater than 90% unit statement coverage over all added and complete modified production functions; external consumer fixture; make test, make compat-test and make type-check; candidate-bound commands, results and reopened persistence. | Unverified: final coverage, external consumer and required make checks await runner evidence; see the US-010 handoff. |
| D1 downstream producer/projection | Agent Console | Structured runner producer/projection evidence preserves actual terminal exit and task/stage/attempt/source bindings through trusted persistence and reload into the consumable engine revision; worker artifacts alone cannot establish provenance. | Pending: Console producer/projection source and authentic run receipts were not inspected. |
| D1 downstream production wiring and policy matching | Agent Console | Exact engine revision consumed by production wiring; launch request, effective execution policy and completion evidence prove policy matching through the real integration. | Pending: no current module pin, deployed engine identity or production policy result was independently verified. |
| D1 downstream verification | Agent Console | Authentic passing results from the waiting Console's unchanged launch-and-policy verifier, native-stage and real-HTTP completion/policy contracts, coverage and repository gates, bound to the integrated candidate. | Pending: engine tests and reported Console process identity cannot satisfy downstream verification. |
| D2 default-branch merge | Agent Console delivery, with owner decision | Published pull request, canonical gate, independent review, exact candidate-bound owner merge decision and authentic default-branch merge evidence identifying the merged fix. | Pending: local commits and owner acceptance do not prove merge or deployment. |

## Promotion controls and retained owner boundary

The current owner request assigns delivery after queue preparation to Agent
Console: candidate commit, pull-request publication, canonical gate, independent
review and presentation of the exact merge decision to the owner. Task stages
prepare evidence and commit authorized workspace changes; they do not publish,
raise the owner's decision, merge or deploy. The full repository gate belongs in
the socket-capable repository runner. These are supplied operating controls;
this study does not claim to have verified Console's enforcement implementation.

The local scheduler's `filterAutoDispatchable` (linked in the API map) excludes
HITL tasks and their transitive dependents from automatic dispatch. This observed
hold is not itself authentication of a decision or a promotion authorization.

The retained final boundary from the supplied plan is:

- Task: `T-US-010-002`
- Mode: `hitl`
- Depends on: `T-US-010-001`
- Decision reason: The owner must make the exact merge decision after Agent Console attaches the published pull request and required review evidence; accepted repair scope does not supply that candidate-specific decision.
- Decision reference: absent; no authentic candidate-bound owner decision was supplied.

Preserve that single boundary, identity, reason and dependency. AFK preparation
must not depend on a future owner decision. Console presents the candidate and PR;
the final task consumes only an authentic authorized decision matching both.
Missing, unauthorized or candidate/PR-mismatched records must be refused by the
later owner-acceptance verifier; synthetic fixtures never constitute acceptance.
No grant or decision reference is invented here. Acceptance does not establish
that downstream D1 passed or that D2 merged.

## Traceability verification

```sh
bash -euo pipefail scripts/autonomy-contract/verify-runtime-evidence.sh --case architecture-traceability
python3 scripts/autonomy-contract/test_architecture_traceability.py
```

The traceability case validates the document/source contract, unique obligation
rows with responsible parties and evidence/status fields, retained boundary and
historical-evidence qualification. Its isolated command tests mutate persisted
fixtures to exercise missing obligations, authority drift and failed checks.
Current results are recorded in Discovery verification evidence below. The command
re-reads saved documentation and sources; fixtures exercise the real shell entry
point. This verifies documentation consistency, not engine behavior, Console
policy enforcement, owner acceptance or delivery.

## Discovery verification evidence

Historical US-007 command tests passed at
`5762115710d445717edd5670e4882bf085d9bac1` against its then-current absence
observations. They did not validate the restored implementation. US-010 updates
the checker to require restored declarations and reject missing resources.
Final verification status, reproduction commands and blockers are maintained
once in `docs/runtime-verification-handoff.md`.

Complexity delta: no runtime concepts, persistent product state, transitions,
owner decisions or replacement machinery. Added only verification tooling and
an external consumer fixture. Project loading and migration code are unchanged;
existing compatibility/refusal checks are reused. No delivery actions occur.

## Historical claims and external evidence

The previous March architecture overview described broad orchestration intent;
its assertions of shipped functionality are not retained as current proof.
The supplied Console revision `e6745def` and start time `2026-09-29T16:21:34Z`
are historical, unverified observations identifying a reported Console process only.
Checkout notes, earlier summaries and module-pin claims remain historical unless
independently verified against current repository/deployment evidence. They establish neither this engine's deployed revision nor merge,
downstream policy matching, or native-stage/HTTP verification. Those resources
were not inspected. Publication, independent review, canonical gates and the exact
owner merge decision remain Console-owned delivery activities outside this stage.

## Retained diagnostic evidence and recapture architecture

The following upstream PR #66 map is retained alongside the structured-terminal
contract above. Its revision-bound evidence remains historical; syncing these
implementations requires fresh combined validation.

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

Read root/docs instructions, NOTES and project intent (`PROJECT_INTENT.md`).
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

## Planner verification

### 10. Planner prompt rules & planning gate (`internal/planner/`, `internal/cli/release.go`)

**Purpose:** Turn an intent document into goals/stories/tasks, and keep unverifiable plans out of the backlog.

**Module map:**
| File | Role |
|------|------|
| `internal/planner/prompt.go` | Prompt constants: `StoryGenerationPrompt` (full plan, rules 1–12), `CompactStoryGenerationPrompt` (one story, rules 1–6), `StoryReviewPrompt`, `StoryFixPrompt`, `WizardSystemPrompt` |
| `internal/planner/planner.go` | `Planner.GeneratePlan` / `GenerateCompactPlan` render the prompts and parse the JSON response |
| `internal/planner/lint.go` | `LintVerificationScript`, `LintPlanVerification` — deterministic false-green detector (never fails the CLI import; `ReviewPlan` refuses on it); `StaleBaseRefIssue` — shell-aware bare `main`/`master` revision detector for one script |
| `internal/planner/stale_base.go` | `PlanStaleBaseRefIssues` (per-plan issue map keyed by owner), `StaleBaseRefOwners` (sorted owners), `PlanStaleBaseRefError` (the same map as one refusal error) — the stale-base rule shared by every import call site |
| `internal/planner/lint_test.go` | `TestLintVerificationScript` (false-green vs sound samples), `TestStaleBaseRefIssue` (stale vs sound scripts, each for `main` and `master`), `TestShellCommands` (quotes, comments, separators, substitutions), `TestRevisionEndpoints` (`..`/`...` split, `^`/`~N`/`@{…}`/`:path` stripping), `TestLintPlanVerification` (keyed by story/task id) |
| `internal/planner/stale_base_test.go`, `review_test.go` | `TestPlanStaleBaseRefIssues` (story and task owners); `TestReviewPlanRefusesStaleBaseRefDespiteApproval`; `TestReviewPlanUsesExistingDiscipline` (false-green lint forces `Approved=false`) |
| `internal/planner/planner_test.go` | `TestStoryPrompt_RequiresOriginDefaultRef` (rule 8 as rendered by `GeneratePlan`) and `TestCompactPromptRequiresRemoteBaseRef` (compact rule 4 as rendered by `GenerateCompactPlan`) pin the `origin/<default>` clause |
| `pkg/manager/planner_stale_base_test.go` | `TestManagerPlan_RejectsStaleBaseRef` — `Manager.Plan` import routes at story and task level |
| `internal/cli/stale_base_binary_e2e_test.go` | `//go:build e2e`: runs the built `bin/openexec story import --dry-run` in a temp `.openexec` project (goal-less object and legacy array refused naming `story US-001 task T-US-001-001` and `origin/main`; the `origin/main` variant previews the import). Run `go build -o bin/openexec ./cmd/openexec`, then `go test -tags e2e ./internal/cli/ -run TestStoryImportBinaryStaleBaseGate` |
| `pkg/runtime/runtime.go` | Public wrappers `LintPlanVerification`, `RemapPlanIDs` for embedders |
| `pkg/manager/planner.go` | `Manager.Plan`: intent validation → `GeneratePlan`/`GenerateCompactPlan` → optional `ReviewPlan` → plan artifact → `importBoundPlan` (`preparePlanIDs` → `RemapPlanIDs`); a non-empty `RequestID` diverts to `replayReviewedPlan` |
| `pkg/manager/planner_replay.go` | `replayReviewedPlan`: durable reviewed planning (generate/compact → review → refine loop → `rel.ImportReviewedPlan`) with retained receipts in `run_steps` |
| `internal/cli/release.go` | `storyImportCmd` (`openexec story import [file]`) — hosts the PLANNING GATE |
| `internal/cli/release_test.go` | `TestReleaseCmd` drives `release create/show/finish`, `story create/list`, `task create/approve`, `goal verify`; plus `statusIcon`, config loading and `getReleaseManager`; `TestImport_PlanningGate_RejectsStaleBaseRef` drives `story import --dry-run` through the stale-base check for each file shape (with goals, goal-less object, legacy bare array): story-level reject, task-level reject naming the task id, `origin/main` pass |

**Rule rendering.** The prompts are static Go raw strings filled with `fmt.Sprintf`: `GeneratePlan` renders `StoryGenerationPrompt` with two `%s` slots (optional PRD block, then the intent text); `GenerateCompactPlan` renders `CompactStoryGenerationPrompt` with one `%s` (intent). No project configuration reaches the prompt — rules are identical for every repository.

**Rule 8 (VERIFIABILITY)** in `StoryGenerationPrompt` requires every story to carry an executable `verification_script` that checks its linked goal and exits non-zero on failure, and forbids false-green shapes: `|| <fallback>` after a test/assertion, `A && B || C`, `2>/dev/null` on the checked command, `grep -q` piped into another command, and non-specific assertions. Its last bullet is the **`origin/<default>` clause**: a script that compares against the default branch MUST use the remote ref `origin/<default>` (e.g. `git diff --name-only origin/main...HEAD`), NEVER a bare local branch name such as `main` — task worktrees are synced to `origin/<default>` and do not advance the local branch, so a stale local `main` fails correct work or passes wrong work. **Compact prompt clause:** `CompactStoryGenerationPrompt` rule 4 (the story and every task MUST have a concrete `verification_script` that fails when the change is broken) ends with the same sentence verbatim, so compact planning receives the instruction too (pinned as rendered by `TestCompactPromptRequiresRemoteBaseRef`, next to `TestStoryPrompt_RequiresOriginDefaultRef` for rule 8). The prompts are instructions a model may ignore; the deterministic stale-base check (`PlanStaleBaseRefIssues`, below) is what refuses a plan, and it runs at the call sites listed under **Stale-base call sites** regardless of which prompt produced the plan.

**`LintVerificationScript(script string) []string`** matches four regexes (`falseGreenPatterns`): a test/grep command followed by `||`; `2>/dev/null`; `grep -q… |`; and `&& … ||`. Empty scripts return nil; an empty result means "no known anti-pattern", not "sound". **`LintPlanVerification(plan) map[string][]string`** runs it over every story and task script and returns issues keyed by story/task id. It never fails the CLI import, but it is not purely advisory: `Planner.ReviewPlan` (`internal/planner/review.go`) calls it after parsing the reviewer's verdict and forces `Approved=false` (appending `; verification lint refused: …` to the assessment) when it returns any issue, so on the reviewed `Manager.Plan` routes a false-green script blocks import (`TestReviewPlanUsesExistingDiscipline`). It is also exported through `pkg/runtime` for embedders. `LintPlanVerification` does **not** call `StaleBaseRefIssue`.

**PLANNING GATE (hard fail)** lives in `storyImportCmd.RunE`, after schema-version checks (accepted: `1.0`, `1.1`, `legacy` bare array; missing version only warns) and before the dry-run print or any DB write. Checks 1–2 (goal coverage) run only when the file has `goals`; for each goal they return an error when:
1. `PLANNING GATE FAILED: Primary goal <id> (<title>) has no supporting stories` — no story has that `goal_id`;
2. `PLANNING GATE FAILED: Primary goal <id> (<title>) has no stories with a verification_script` — none of those stories has a non-empty script.

3. **Stale-base check (hard fail).** Outside the goals branch, so it applies to every file shape (goal-less objects and legacy bare arrays included), the gate runs `planner.PlanStaleBaseRefIssues` (`StaleBaseRefIssue` per script) over every story's `verification_script` and every task-object's `verification_script` (task entries that are bare id strings are skipped). The first owner in sorted order returns `PLANNING GATE FAILED: story <story-id>: <issue>` or `PLANNING GATE FAILED: story <story-id> task <task-id>: <issue>`, where `<issue>` is ``verification script diffs against the bare local `main` ref; use `origin/main` — candidate worktrees are synced to origin/<default>, where the local `main` is stale or missing`` (with `master` substituted when that is the ref found).

**Shell-aware detector.** `StaleBaseRefIssue(script string) string` (`internal/planner/lint.go`) is a hard gate, so it looks only at executable git revision arguments rather than regex-matching the whole script. It tokenizes the script as shell — single/double quotes, backslash continuations, `#` comments, and `;`/`&&`/`||`/`|`/newline/subshell/command-substitution boundaries — and, per command, skips prefixes (`!`, `if`, `time`, `NAME=value` …) to find `git`, skips git global options with values (`-C`, `-c`, `--git-dir`, …), and continues only for `diff|log|show|merge-base|rev-list|rev-parse|cherry|range-diff`. Positionals before `--` are revisions; subcommand options with a separate value (`-n`, `--grep`, `--since`, …) are skipped. Each revision is split on `..`/`...`, and each endpoint loses a leading `^` and any `@{…}`, `^`/`~N` or `:path` suffix; an endpoint that is exactly `main` or `master` is refused. So `main...HEAD`, `'main' HEAD`, `"main..HEAD"`, `^main`, `main~1`, `main:README.md`, `master@{u}...HEAD`, and a bare ref mixed with a remote one (`git diff origin/main...HEAD && git diff main...HEAD`) are refused; `origin/main`, `upstream/main` and `feature-main` pass, as do main/master in comments, in data for other commands (`grep -Fq 'main...HEAD' docs/…`, `echo git diff main...HEAD`), as option values (`--grep main`), in non-revision git subcommands (`git checkout main`) and as pathspecs after `--` (`git diff --exit-code origin/main...HEAD -- main`). It returns `""` when the script is acceptable. Every example in this paragraph is a case in `TestStaleBaseRefIssue` (run for both `main` and `master`); tokenizing is pinned by `TestShellCommands` and endpoint stripping by `TestRevisionEndpoints`. Apart from this check the CLI gate never inspects script content — it does not call the false-green linter. `openexec plan` (`internal/cli/plan.go`) prints its own `PLANNING GATE FAILED:` banner, but that one reports intent-validation issues from `Manager.Plan`, which does not run the goal-coverage gate; its stale-base check fails as an import error (routes 2–6 below).

**Stale-base call sites.** `PlanStaleBaseRefIssues(plan)` (`internal/planner/stale_base.go`) applies `StaleBaseRefIssue` to every story and task `verification_script` of a `ProjectPlan` and returns a map from owner (`story US-001`, `story US-001 task T-US-001-002`) to diagnostic; it never reads goals, so goal-less and legacy plans are checked like full ones. `PlanStaleBaseRefError` wraps the same map as one `stale base ref refused: <owner>: <issue>; …` error in `StaleBaseRefOwners` (sorted) order. It is called at exactly these sites:
- **CLI import** — `storyImportCmd.RunE` (`internal/cli/release.go`) calls `PlanStaleBaseRefIssues(generatedStoriesPlan(stories))` outside the goal-coverage branch and reports the first owner as `PLANNING GATE FAILED` (routes 1a, 1b).
- **`ReviewPlan`** (`internal/planner/review.go`) calls `PlanStaleBaseRefError` after parsing the reviewer's verdict and forces `Approved=false`, appending the error to the assessment so refinement receives the owner and `origin/` fix (routes 3–4).
- **`importBoundPlan`** (`pkg/manager/planner.go`) calls `PlanStaleBaseRefError` first, before `preparePlanIDs` and any `rel.Create*` write (routes 2–3).
- **`replayReviewedPlan`** (`pkg/manager/planner_replay.go`) calls it at two steps: the **refinement step**, on the `RefinePlan` result when `AutoImport` is set, before `preparePlanIDs`/`ValidatePlanIdentities` (route 5); and the **retained/import step**, on the approved `result.Plan` immediately before `rel.ImportReviewedPlan`, which covers fresh, compact and retained-receipt replays alike and refuses without rewriting the receipt or plan artifact (routes 4, 6).

**Plan import routes.** Every code path that persists planner-generated goal/story/task rows, and which of the call sites above guards it. `TestManagerPlan_RejectsStaleBaseRef` (`pkg/manager/planner_stale_base_test.go`) covers routes 2–6 at story and task level, each for full and compact generation: `{full,compact}/native` (route 2), `/reviewed-direct` (route 3: `Review` + `AutoImport`, no `RequestID`), `/reviewed` (route 4, via `replayRequest()`), `/refined` (route 5) and `/retained` (route 6). `TestImport_PlanningGate_RejectsStaleBaseRef` and the e2e binary test cover 1a/1b.

| # | Route | Entry points | Row writer | Script checks before write | `StaleBaseRefIssue` |
|---|-------|--------------|------------|----------------------------|---------------------|
| 1a | CLI story import, file has non-empty `goals` | `openexec story import [file]` (`storyImportCmd.RunE`, `internal/cli/release.go`) | `mgr.CreateGoal` / `mgr.CreateStory` / task creation in the same `RunE` | Goal coverage + stale-base check (story and task-object scripts) | **Yes** — `PLANNING GATE FAILED` |
| 1b | CLI story import, no `goals` (object without goals, or legacy bare array → `schema_version` `legacy`) | same command | same writer | Stale-base check only (goal coverage sits inside `if len(sf.Goals) > 0`; the stale-base check runs after it) | **Yes** — `PLANNING GATE FAILED` |
| 2 | Non-reviewed auto-import | `openexec plan` (`AutoImport: true`, no review); `POST` plan handler (`pkg/api/handlers.go` `handlePlan`) with `auto_import` and no `request_id`/`review` | `Manager.importBoundPlan(plan, false)` (`importPlan` is a thin wrapper with no callers) → `rel.CreateGoal`/`CreateStory`/`CreateTask` | `plan.Validate()` (structure), then the stale-base check first in `importBoundPlan`, before `preparePlanIDs` | **Yes** — import error |
| 3 | Reviewed `Manager.Plan` without a `RequestID` (full or compact) | `Manager.Plan` with `Review` + `AutoImport` | `importBoundPlan(plan, true)` (refuses if `preparePlanIDs` changed IDs after review) | `ReviewPlan` → `LintPlanVerification` (false-green), the stale-base check and `LintHumanBoundaries` force `Approved=false`; `importBoundPlan` re-checks stale base | **Yes** — review refusal, then import error |
| 4 | Reviewed replay (`RequestID` set) — fresh generate or compact (`planner_replay.go`, `retained.Result == nil` block) | `Manager.Plan` → `replayReviewedPlan` (the route the PR #63 review reports Agent Console's task loop uses, with `Review` + `AutoImport`) | `rel.ImportReviewedPlan` (`internal/release/reviewed_plan_import.go`, atomic insert of `reviewedPlanRows`) | `ReviewPlan` refusal as in route 3 (its assessment names the owner and the `origin/` fix, which feeds refinement); stale-base check again before `ImportReviewedPlan` | **Yes** — review refusal, then import error |
| 5 | Reviewed replay — refined plan (`RefinePlan` after a rejected review, bounded by `ReviewLimit`/`MaxReviewCycles`) | same | `rel.ValidatePlanIdentities` (identities only) at refinement, then `rel.ImportReviewedPlan` once a later review approves | With `AutoImport`, the stale-base check runs on the refined plan before `preparePlanIDs`/`ValidatePlanIdentities` and fails the request before re-review | **Yes** — error after refinement |
| 6 | Reviewed replay — retained result (receipt already holds an approved plan+review; replay re-verifies artifact/review hashes and imports) | same, repeated call with the same `RequestID` | `rel.ImportReviewedPlan` | The stale-base check runs before `ImportReviewedPlan` even on an approval recorded before the rule existed; it refuses without rewriting the receipt or plan artifact | **Yes** — import error |

Not plan imports (single rows, no planner output): `openexec story create` (`release.go` `CreateStory` at the story-create command) and `backlog_add_task` (`internal/mcp/backlog.go`, rolling `US-MAINT` story). `openexec doctor` calls `Manager.Plan` with `AutoImport: false` and persists nothing.

Consequence: on every route above (1a–6) a plan whose story- or task-level script uses a bare `main...HEAD` is refused before any row is written, and the diagnostic names the owner (`story US-1` or `story US-1 task T-2`) and the `origin/main` fix. Routes 2–6: every `TestManagerPlan_RejectsStaleBaseRef` subtest (asserts the owner and `origin/` in the error, and zero imported tasks after reopening the DB). Routes 1a/1b: `TestImport_PlanningGate_RejectsStaleBaseRef` and `TestStoryImportBinaryStaleBaseGate`.

**Default branch threading.** None: no project setting names the default branch to the planner. The only related setting is `base_branch` (`project.ProjectConfig.BaseBranch`, `release.Config.BaseBranch`, default `"main"`), consumed by the release manager and `safe_commit`; it is never passed to `planner.GeneratePlan`/`GenerateCompactPlan`. Rule 8 therefore speaks of `origin/<default>` generically, and the gate recognises only `main`/`master` as bare default-branch names.

### Review 474755f1 finding map

Review `474755f1d4af4c2b4e1faa5240c74345` (PR #63) worked by code inspection only: the reviewer ran no commands. Its evidence line ranges (`lint.go:26-60`, `release.go:918-959`, `planner_replay.go:240-275`, `prompt.go:162-168`, `ARCHITECTURE.md:350`) come from the tree before US-012/US-013. Commits `e63086c9` (tokenized detector), `62a728d5` (CLI gate outside the goals branch), `c09f3b5f`/`1ac5b539` (`ReviewPlan` and every `Manager.Plan` import path) and `a029f1d4` (compact rule 4) changed those regions afterwards. The table below maps every listed case to the code and test at HEAD; every test named here passed in the US-018 validation run below.

**HIGH — native planning and replay bypass the gate**

| Case | HEAD code | Covered by |
|------|-----------|------------|
| Compact native planning gets no remote-ref instruction | `CompactStoryGenerationPrompt` rule 4 carries the `origin/<default>` clause verbatim | `TestCompactPromptRequiresRemoteBaseRef` (rendered by `GenerateCompactPlan`) |
| Compact native planning gets no deterministic rejection | `importBoundPlan` → `PlanStaleBaseRefError` before `preparePlanIDs` | `TestManagerPlan_RejectsStaleBaseRef/compact/native/{story,task}` |
| Full native planning imports a response that ignores the instruction | same `importBoundPlan` check | `…/full/native/{story,task}` |
| Reviewer-approved plan with `main...HEAD` through `replayReviewedPlan` (Agent Console's `Review`+`AutoImport`+`RequestID` route), full and compact | `ReviewPlan` forces `Approved=false`; `PlanStaleBaseRefError` runs again before `rel.ImportReviewedPlan` | `…/{full,compact}/reviewed/{story,task}`, `TestReviewPlanRefusesStaleBaseRefDespiteApproval` |
| Reviewed `Manager.Plan` without a `RequestID` (route 3), full and compact | `ReviewPlan` refusal; `importBoundPlan` re-checks | `…/{full,compact}/reviewed-direct/{story,task}` |
| Refined plan | `PlanStaleBaseRefError(refined)` before `preparePlanIDs`/`ValidatePlanIdentities` and before re-review | `…/{full,compact}/refined/{story,task}` (asserts no re-review) |
| Retained-plan replay, refused without rewriting accepted artifacts | `PlanStaleBaseRefError(result.Plan)` before `ImportReviewedPlan`; the receipt is not rewritten | `…/{full,compact}/retained/{story,task}` (compares receipt metadata and plan artifact bytes before and after) |
| Manual import of a goal-less object or legacy bare array, story and task level | `storyImportCmd.RunE` runs `PlanStaleBaseRefIssues` outside `if len(sf.Goals) > 0` | `TestImport_PlanningGate_RejectsStaleBaseRef` (`goal-less object:`/`legacy array:` subtests), `TestStoryImportBinaryStaleBaseGate` |
| Rule 8 in full generation | `StoryGenerationPrompt` rule 8 | `TestStoryPrompt_RequiresOriginDefaultRef` |
| Owner and `origin/` fix in the diagnostic; nothing imported after reopening the DB; `origin/main` persists | — | `requireStaleRefusal` + `importedTaskCount` (reopens `release.Manager` on the same DB) in every `TestManagerPlan_RejectsStaleBaseRef` subtest; each subtest also runs `origin/main` and requires persisted tasks |
| Docs claim "every imported plan" | Coverage is now stated per route in **Plan import routes** | — |

**MEDIUM — the gate confuses script text and pathspecs with Git refs**

| Reviewer script (each for `main` and `master`) | Expected | `TestStaleBaseRefIssue` template | CLI import (`TestImport_PlanningGate_RejectsStaleBaseRef`) |
|------|----------|----------------------------------|------------|
| `git diff --exit-code origin/main...HEAD -- main` | accept | sound `git diff --exit-code origin/REF...HEAD -- REF` | `reviewer_scripts/{main,master}/{story,task}/sound/0` |
| `grep -Fq 'main...HEAD' docs/ARCHITECTURE.md` | accept | sound `grep -Fq 'REF...HEAD' docs/ARCHITECTURE.md` | `reviewer_scripts/…/sound/1` |
| Comment-only mention followed by a valid assertion | accept | sound `"# never use REF...HEAD\ngit diff --exit-code origin/REF...HEAD -- internal/"` | `reviewer_scripts/…/sound/2` (`"# never REF...HEAD\ngit diff --exit-code origin/REF...HEAD"`) |
| `git diff 'main' HEAD` | reject | stale `git diff 'REF' HEAD` | `reviewer_scripts/…/stale/0` |
| Quoted two-dot/three-dot ranges | reject | stale `git diff "REF...HEAD"`, `git diff 'REF..HEAD'` | `reviewer_scripts/…/stale/1` (`"REF..HEAD"`), `…/stale/2` (`'REF...HEAD'`) |
| Mixed remote/bare comparison | reject | stale `git diff origin/REF...HEAD && git diff REF...HEAD` | `reviewer_scripts/…/stale/3` |
| Unquoted bare range, story and task owner | reject | stale `git diff --name-only REF...HEAD …`, `git log REF..HEAD` | with goals: story `main...HEAD`, task `main..HEAD`; goal-less object and legacy array: story `main...HEAD`, task `master..HEAD` |

The `reviewer_scripts` group runs each template for `main` and `master`, owned by the story or by task `T-US-001-002`, through `storyImportCmd.RunE` (28 leaves). Stale leaves require the owner (`story US-001` or `T-US-001-002`) and `origin/<ref>` in the error; sound leaves require `✓ Planning Gate passed.`.

**Remaining gaps at HEAD**

None. Both findings' cases are exercised at their gate, and the HIGH and MEDIUM falsify controls are recorded below.

**Validation run (US-018, 2026-09-29, candidate worktree at `8ed32d3b`)**

| Command | Result |
|---------|--------|
| `go vet ./...` | clean, no output |
| `go test ./... -count=1` | `ok` for every package with tests (`internal/planner`, `internal/cli`, `pkg/manager` included); no `FAIL` |
| `make compat-test` | `TestCompatibility_ExistingProjects_StatusCLI` (current `.openexec` and legacy `.uaos`), `TestCompatibility_LegacyProjectConfigFallback`, `TestCompatibility_LegacyTasksJSONFallback` — PASS |
| `go build -o bin/openexec ./cmd/openexec`, then `go test -tags e2e -count=1 ./internal/cli/ -run TestStoryImportBinaryStaleBaseGate -v` | PASS: goal-less object and legacy array refused with `PLANNING GATE FAILED: story US-001 task T-US-001-001`, naming the `origin/main` fix; the `origin/main` variant previews 1 story |
| `go test -count=1 ./internal/planner/ ./internal/cli/ ./pkg/manager/ -run 'StaleBase\|OriginDefaultRef\|RemoteBaseRef\|PlanningGate' -v` | PASS: `TestStaleBaseRefIssue`, `TestStoryPrompt_RequiresOriginDefaultRef`, `TestCompactPromptRequiresRemoteBaseRef`, `TestReviewPlanRefusesStaleBaseRefDespiteApproval`, `TestPlanStaleBaseRefIssues` (2 subtests), `TestImport_PlanningGate_RejectsStaleBaseRef` (9 subtests + 28 `reviewer_scripts` leaves), `TestManagerPlan_RejectsStaleBaseRef` (20 subtests) |

**Dispositions for `resolve_feature_review`** (review `474755f1d4af4c2b4e1faa5240c74345`)

- **HIGH — native planning and replay bypass the gate: accepted, repaired.** The shared validator `PlanStaleBaseRefIssues`/`PlanStaleBaseRefError` (`internal/planner/stale_base.go`) now runs in `ReviewPlan`, `importBoundPlan`, and both `replayReviewedPlan` steps (refined plan; approved/retained plan before `ImportReviewedPlan`, receipt not rewritten), and in the CLI import outside the goals branch. Compact rule 4 carries the `origin/<default>` clause. Evidence: `TestManagerPlan_RejectsStaleBaseRef` (full/compact × native/reviewed/reviewed-direct/refined/retained × story/task, DB reopened, `origin/main` persists), `TestImport_PlanningGate_RejectsStaleBaseRef` goal-less/legacy subtests, `TestStoryImportBinaryStaleBaseGate`, `TestCompactPromptRequiresRemoteBaseRef`; negative controls (a)–(c) above fail as required. Docs: the "every imported plan" claim is replaced by the per-route table. Commits `c09f3b5f`, `1ac5b539`, `62a728d5`, `a029f1d4`, `ae986da4`.
- **MEDIUM — gate confuses script text and pathspecs with Git refs: accepted, repaired.** `StaleBaseRefIssue` now tokenizes shell (quotes, comments, separators, substitutions) and inspects only revision arguments of revision-taking git subcommands before `--`. Evidence: `TestStaleBaseRefIssue`, `TestShellCommands`, `TestRevisionEndpoints`, and the 28 `reviewer_scripts` leaves of `TestImport_PlanningGate_RejectsStaleBaseRef` (reviewer's exact scripts, `main`/`master`, story/task); negative controls (a) regexes restored and (b) `return ""` fail 16/28 leaves each. Commits `e63086c9`, `8ed32d3b`.

### Review 474755f1 — HIGH negative controls

Run 2026-09-29 in the candidate worktree (US-016). Each mutation was applied on its own, the matching tests were run with `-v`, and the production file was restored with `git checkout -- <file>` before the next one. None of these mutations is committed. Baseline and final rerun: `go test ./internal/planner/ ./internal/cli/ ./pkg/manager/` → `ok` for all three packages, and every stale-base test passes (including the 20 `TestManagerPlan_RejectsStaleBaseRef` subtests).

**(a) Remove the shared validation** — delete the `PlanStaleBaseRefError` calls in `importBoundPlan` (`pkg/manager/planner.go`) and at both steps of `replayReviewedPlan` (`pkg/manager/planner_replay.go`). `go test ./pkg/manager/ -run TestManagerPlan_RejectsStaleBaseRef -v`:
```
--- FAIL: TestManagerPlan_RejectsStaleBaseRef
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/full/native/story
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/full/refined/story
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/full/retained/story
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/compact/native/story
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/compact/refined/story
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/compact/retained/story
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/full/native/task
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/full/refined/task
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/full/retained/task
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/compact/native/task
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/compact/refined/task
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/compact/retained/task
planner_stale_base_test.go:98: stale base ref plan was accepted: &{... Valid:true Issues:[] ...}
planner_stale_base_test.go:181: stale refined plan was not refused: &{... Valid:false Issues:[Required verification represented; stale base ref refused: story US-1: ...] ...}
planner_stale_base_test.go:229: retained stale plan was not refused: &{... Valid:true Issues:[] ...}
```
The `{full,compact}/reviewed/*` and `/reviewed-direct/*` subtests still pass under (a) because `ReviewPlan` refuses the approved stale plan on its own (`TestReviewPlanRefusesStaleBaseRefDespiteApproval`); the import-side check is the second layer there. On the refined route the stale plan now reaches re-review (refused there, but with no error before re-review, which the subtest forbids).

**(b) Restore the goals-only CLI condition** — move the `PlanStaleBaseRefIssues` block in `storyImportCmd.RunE` (`internal/cli/release.go`) inside `if len(sf.Goals) > 0`. `go test ./internal/cli/ -run TestImport_PlanningGate -v`:
```
--- FAIL: TestImport_PlanningGate_RejectsStaleBaseRef
    --- FAIL: TestImport_PlanningGate_RejectsStaleBaseRef/goal-less_object:_story-level_bare_main_is_rejected
    --- FAIL: TestImport_PlanningGate_RejectsStaleBaseRef/goal-less_object:_task-level_bare_master_is_rejected
    --- FAIL: TestImport_PlanningGate_RejectsStaleBaseRef/legacy_array:_story-level_bare_main_is_rejected
    --- FAIL: TestImport_PlanningGate_RejectsStaleBaseRef/legacy_array:_task-level_bare_master_is_rejected
release_test.go:266: expected the planning gate to reject a bare main ref; output: Note: This performs a one-time import. ...
release_test.go:278: expected the planning gate to reject a task-level bare master ref; output: Note: This performs a one-time import. ...
```
The with-goals subtests (`bare_main_is_rejected`, `task-level_bare_main_is_rejected_with_the_task_ID`) still pass, as they should: the check still runs when goals are present.

**(c) Remove the compact clause** — delete the `origin/<default>` sentence from `CompactStoryGenerationPrompt` rule 4 (`internal/planner/prompt.go`). `go test ./internal/planner/ -run 'OriginDefaultRef|RemoteBaseRef' -v`:
```
--- FAIL: TestCompactPromptRequiresRemoteBaseRef
planner_test.go:174: rule 4 of the rendered compact prompt is missing "origin/<default>"
planner_test.go:174: rule 4 of the rendered compact prompt is missing "git diff --name-only origin/main...HEAD"
planner_test.go:174: rule 4 of the rendered compact prompt is missing "NEVER a bare local branch name such as 'main'"
```
`TestStoryPrompt_RequiresOriginDefaultRef` (full-prompt rule 8) still passes.

No CLI route gap was open for the HIGH finding (goal-less and legacy shapes were already covered), so `TestImport_PlanningGate_StaleBaseRoutes` was not added.

### Review 474755f1 — MEDIUM negative controls

Run 2026-09-29 in the candidate worktree (US-017). Each mutation of `StaleBaseRefIssue` (`internal/planner/lint.go`) was applied on its own and `go test ./internal/cli/ -count=1 -run 'TestImport_PlanningGate_RejectsStaleBaseRef/reviewer_scripts' -v` was run. `git checkout -- internal/planner/lint.go` then restored the file, and `git diff --exit-code HEAD -- internal/planner/lint.go` was clean before the next step. Neither mutation is committed. Baseline and final rerun: all 28 `reviewer_scripts` leaves pass, and `go test ./internal/planner/ ./internal/cli/` → `ok`.

**(a) Restore the whole-script regexes** (the pre-`e63086c9` `staleBaseRefPatterns` loop). 16 of 28 leaves fail: every sound leaf, plus `stale/0`. The quoted ranges and the mixed comparison still match the range regex, so they stay rejected:
```
--- FAIL: TestImport_PlanningGate_RejectsStaleBaseRef/reviewer_scripts/{main,master}/{story,task}/sound/{0,1,2}
--- FAIL: TestImport_PlanningGate_RejectsStaleBaseRef/reviewer_scripts/{main,master}/{story,task}/stale/0
release_test.go:369: sound story script "git diff --exit-code origin/main...HEAD -- main" was refused: PLANNING GATE FAILED: story US-001: verification script diffs against the bare local `main` ref; ...
release_test.go:369: sound task script "grep -Fq 'master...HEAD' docs/ARCHITECTURE.md" was refused: PLANNING GATE FAILED: story US-001 task T-US-001-002: ...
release_test.go:369: sound story script "# never main...HEAD\ngit diff --exit-code origin/main...HEAD" was refused: ...
release_test.go:381: stale task script "git diff 'master' HEAD" was accepted; output: Note: This performs a one-time import. ...
```

**(b) `return ""`** (the detector always accepts). 16 of 28 leaves fail: every stale leaf. Every sound leaf still passes:
```
--- FAIL: TestImport_PlanningGate_RejectsStaleBaseRef/reviewer_scripts/{main,master}/{story,task}/stale/{0,1,2,3}
release_test.go:381: stale story script "git diff \"main..HEAD\"" was accepted; ...
release_test.go:381: stale task script "git diff 'master...HEAD'" was accepted; ...
release_test.go:381: stale story script "git diff origin/main...HEAD && git diff main...HEAD" was accepted; ...
```

---

## origin/main sync map

Observed 2026-09-30 for US-019 / T-US-019-001 at candidate HEAD `92e3d48a`.
`git merge-base HEAD origin/main` returned fork point `45f1c1fe`;
`git rev-parse origin/main` resolved to `2fc03214`.
`git merge-tree --write-tree --name-only HEAD origin/main` exited 1 with
one content conflict: `docs/ARCHITECTURE.md`. This was a merge probe only;
that historical probe preceded the US-019 NOTES addition. The actual US-020
merge also conflicted in NOTES.md; both note sets were retained. No code conflicted.

The US-020 resolution uses the upstream rewrite as its base and preserves the
candidate additions under these headings (within the Planner verification section):

| Side | Keep-list |
| --- | --- |
| origin/main (264-line rewrite) | `## Context and scope`; `## Module map and APIs`; `## Data flow and observed gaps`; `## Accepted requirement mapping`; `## Boundaries and conventions`; `## Evidence ownership`; `## Discovery verification` |
| Candidate additions since the fork | ``### 10. Planner prompt rules & planning gate (`internal/planner/`, `internal/cli/release.go`)``; `### Review 474755f1 finding map`; `### Review 474755f1 — HIGH negative controls`; `### Review 474755f1 — MEDIUM negative controls` |

Inspection confirms full prompt rule 8 and compact rule 4 require
`origin/<default>`. `internal/planner/stale_base.go` checks story and task
scripts, sorts owners and builds refusal errors. The CLI import gate calls it
outside goal coverage; manager imports check it before persistence in
`importBoundPlan`, refined auto-import and retained/approved replay.

US-019 discovery checks passed (all `-count=1`): `go test ./internal/planner/ -run
OriginDefaultRef`, `go test ./internal/planner/ -run RemoteBaseRef`, and
`go test ./internal/planner/ ./internal/cli/ ./pkg/manager/ -run StaleBase`.
These exercise remote-ref acceptance and bare-ref refusal, including manager
reload assertions for persisted/absent tasks and retained-artifact preservation.
Host `run_declared_check` named `lint` exited 0 (Go vet and UI ESLint).
After `go build -o bin/openexec ./cmd/openexec`, `go test -tags e2e -count=1
./internal/cli/ -run StaleBase -v` passed: the actual CLI refused bare refs in
goal-less objects and legacy arrays, and previewed the remote-ref import.
The task's verification script also passed, re-reading the saved sync map.
No production code or tests changed; project-format compatibility is unchanged.

### US-020 merged-tree verification

Merged `origin/main` at `2fc032141ccc1b34b71110247cc781b82468941a`
with `--no-ff`, retaining the candidate history. Only ARCHITECTURE.md and
NOTES.md conflicted; no code conflicted. The upstream architecture is preserved
verbatim as the base, followed by the complete candidate planner section,
finding map, HIGH/MEDIUM controls and stale-base coverage notes. The US-019
sync map remains above, with its historical probe distinguished from this merge.
Both sets of working notes are retained. No production code or tests were
changed by the resolution; no new abstraction or runtime behavior was added.

Fresh checks on 2026-09-30:

- `go build ./...` and `go build -o bin/openexec ./cmd/openexec`: exit 0.
- D1: `go test ./internal/planner/ -run OriginDefaultRef -count=1`,
  `go test ./internal/planner/ -run RemoteBaseRef -count=1`, and
  `go test ./internal/planner/ ./internal/cli/ ./pkg/manager/ -run StaleBase -count=1`:
  exit 0, including refusal/acceptance and persisted-state reload checks.
- `go test -tags e2e -count=1 ./internal/cli/ -run StaleBase -v`: exit 0;
  built CLI refuses bare refs in goal-less objects/legacy arrays and accepts
  the remote-ref import preview.
- Host declared `lint`: exit 0 (`go vet ./...` and UI ESLint).
- Host declared `test`: exit 0 (all Go packages; 40 UI files, 635 tests).
  UI output includes non-failing WebSocket port and React warnings.
- The task verification script, read from the task ledger and run after the
  merge commit, exits 0: pinned ancestry, marker scan, build, D1, compatibility
  and required architecture headings. An additional marker scan uses separate
  `-e` patterns in an `if ...; then exit 1; fi` guard, without alternation.
- `make compat-test`: exit 0; current `.openexec`, legacy `.uaos`, legacy
  config and `.openexec/tasks.json` fallback checks pass.
- Discovery dispatcher: exit 0, 27 declarations; all nine discovery tests pass.
- Saved-file comparison confirms the complete upstream base and candidate
  planner/review section, with exactly one sync map. Conflict-marker scan and
  `git diff --check` pass.

Local Go commands use a writable `/tmp` build cache after the default cache
was refused by the sandbox. Go emitted a read-only module-stat-cache warning,
but the subsequent builds and checks exited 0. Canonical delivery remains
Console-owned; these results do not claim deployment or PR acceptance.

### Validation on origin/main 2fc03214

US-021 / T-US-021-001, verified 2026-09-30 on merged HEAD
`67e5e16b3ec5d96167016c16f3fdb652a192f552`.
`git merge-base --is-ancestor 2fc03214 HEAD` exited 0. The outcome is
fresh D1 evidence on the merged candidate; the existing planner validator and
CLI import gate own refusal, and Console owns subsequent delivery. No new
abstraction, runtime behavior, schema, or test expectation is introduced.
Only this documentation change is persisted.

Commands below ran in this candidate worktree. Local Go commands used
`export GOCACHE=/tmp/openexec-go-cache` for writable build output.

| Check | Observed result |
| --- | --- |
| `go test -count=1 ./internal/planner/ ./internal/cli/ ./pkg/manager/` | Exit 0; all three complete package suites passed (0.013s, 1.540s, 117.910s). |
| `make compat-test` | Exit 0; current `.openexec`, legacy `.uaos`, legacy config and `.openexec/tasks.json` fallback passed. |
| `make build` | Exit 0; UI TypeScript/Vite build and Go `ui_dist` binary build passed, embedding commit `67e5e16b`. |
| `go test -tags e2e -count=1 ./internal/cli/ -run TestStoryImportBinaryStaleBaseGate -v` | Exit 0 against that built binary. Goal-less object and legacy array with `main...HEAD` both failed with `PLANNING GATE FAILED: story US-001 task T-US-001-001` and the `origin/main` remedy. The remote-ref variant printed `Would import 0 goals and 1 stories`. |
| Host `run_declared_check` named `test` (default, then `args=["GOCACHE=/tmp/openexec-go-cache"]`) | Both exited 0; all Go packages passed (mostly cached), plus 40 UI files / 635 tests. Non-failing WebSocket port and React warnings appeared. |
| Restored D1: `go test -count=1 ./internal/planner/ ./internal/cli/ ./pkg/manager/ -run 'StaleBase\|OriginDefaultRef\|RemoteBaseRef\|PlanningGate'` | Exit 0 for all three packages after both negative controls were restored. Manager cases reopen the DB to check refused tasks are absent and remote-ref tasks persist. |

The task's exact `verification_script`, extracted from its plan artifact and
run with `bash /tmp/us021-verify.sh`, exited 0: clean production-file diff,
`go test ./internal/planner/... ./internal/cli/... ./pkg/manager/... -count=1`,
`make compat-test`, and the saved validation-heading assertion all passed.
A final built-binary rerun also exited 0 before restoring `bin/openexec`.
`git diff --check` passed, and rereading the document confirmed one validation
section. No tests were added, removed, or weakened.

**Temporary negative controls.** Each mutation was applied separately to the
working tree; tests were unchanged. Removing only the full prompt rule-8
remote-ref bullet in `internal/planner/prompt.go`, then running
`go test -count=1 ./internal/planner/ -run TestStoryPrompt_RequiresOriginDefaultRef -v`,
exited 1: the rendered rule lacked `origin/<default>`,
`git diff --name-only origin/main...HEAD`, and the prohibition on bare `main`.
The compact rule-4 clause remained intact. `git restore -- internal/planner/prompt.go`
restored the original immediately afterward.

Replacing the CLI gate initializer
`planner.PlanStaleBaseRefIssues(generatedStoriesPlan(stories))` with
`map[string]string(nil)` in `internal/cli/release.go`, then running
`go test -count=1 ./internal/cli/ -run TestImport_PlanningGate_RejectsStaleBaseRef -v`,
exited 1: story/task bare refs were accepted in full, goal-less and legacy plans,
and stale reviewer-script cases were accepted. This bypass leaves the goal
coverage branch intact, isolating the stale-base gate.
`git restore -- internal/cli/release.go` restored the original immediately afterward.
`git diff --exit-code -- internal/planner/prompt.go internal/cli/release.go`
then exited 0 with no output. The restored D1 rerun above passed.

**Stale-base module map.** The canonical file/API entries are in the Planner
verification module map above: `internal/planner/lint.go` owns the shell-aware
`StaleBaseRefIssue`; `internal/planner/stale_base.go` owns
`PlanStaleBaseRefIssues`, sorted `StaleBaseRefOwners`, and `PlanStaleBaseRefError`.
The CLI entry is `storyImportCmd` in `internal/cli/release.go`, outside goal
coverage. Other consumers are `ReviewPlan` in `internal/planner/review.go`,
`importBoundPlan` in `pkg/manager/planner.go`, and refined/retained replay in
`pkg/manager/planner_replay.go`. Their route and persistence assertions remain
in the existing test entries and route table; no duplicate validator is added.

Compatibility evaluation: no production or test files change, so existing
project-format support is preserved; compatibility tests exercise it directly.
The built binary is verification output and is restored before committing.
Canonical gate, publication, independent review and owner acceptance remain
Console-owned; these checks make no deployment claim.
