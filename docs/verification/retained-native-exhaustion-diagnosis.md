# Retained native repair exhaustion: diagnosis

This is a diagnosis and synthetic characterization, with no runtime correction.
The original queue remains incomplete. No original store, project document,
allowance, attempt, Stop or completion state was accessed or changed. All
documents and command receipts constructed by the tests are generic, temporary
synthetic data. No model request is reconstructed or replayed.

## Source identity and boundary

Inspected native source revision:
`7c074a9f84da17af0a97199d870c35a2d2eeb463`. The following production blobs are
unchanged by this diagnosis; their identities distinguish source inspection from
an attestation of the binary that produced the reported private incident.

| File | Git blob |
| --- | --- |
| `pkg/manager/task_queue.go` | `87b8efc8b47447757cc872ef3c3376f0fdb4b8f3` |
| `pkg/manager/task_failure.go` | See the named source revision |
| `internal/release/failure_repair.go` | `fb50de6e4e9bd1320fd5065ad34d94dc1fe29398` |
| `pkg/manager/task_recapture.go` | `47737ebef199c92adffe902dbf4d4a0eb2345806` |
| `internal/release/sqlite_store.go` | `a47407f3db8bf31ce3258cc48ab9e96cbae56066` |
| `internal/release/automerge.go` | `d2a1d2a567e1d134cc211d9b07e9d02286c970bf` |
| `internal/pipeline/pipeline.go` | `a6b550444b15c3a5223cd177a4b279de2bb49aa7` |

The exact error is `task repair attempt limit reached`, returned by
`SQLiteStore.CreateFailureRepair` in `internal/release/failure_repair.go:104`.
It counts rows with `metadata.repair_of = rootID` and refuses another repair of
a failed task when that count reaches the **original task's** `max_attempts`.
The original task's `attempt_count` is not subtracted from that repair count.
The final repair's own `attempt_count/max_attempts` is a separate limit.

`executeTaskQueue` (`pkg/manager/task_queue.go:60`) processes a failed task with
`verification_failure_evidence` and no `fixed_by` **before selecting runnable
work**. `repairTaskFromRetainedFailure` (`pkg/manager/task_failure.go:82`) loads
the bound failed deterministic run step, validates the receipt and either
recaptures a diagnostic-free receipt or calls `CreateFailureRepair`. An error
from the latter escapes before `RunnableTasks`. Thus even an independent pending
task can remain undispatched. Dependent work additionally waits for `Delivered`
to follow the closed `fixed_by` chain to a passed leaf. Neither a passing file
check nor an outer Goal review changes that predicate.

## Reproduction and limits

The executable reproducer is
[`pkg/manager/retained_exhaustion_diagnosis_test.go`](../../pkg/manager/retained_exhaustion_diagnosis_test.go).
It belongs to the ordinary Go test suite, run by the declared `go-test` check,
`make test` and CI's `build / Test` step. It requires no private store, network,
model, service or original project files.

`TestRetainedExhaustionDiagnosisQueue/none` creates original A failed at 1/3,
three repairs through the native repair API, final repair failed at 2/2, a
pending dependent, and an independent pending task. Each retained failure has
an actual exit-2 process result persisted with `RecordTaskFailureStep`. The
generic architecture document then passes the exact four-line document check.
`ExecuteTasks(TaskOriented: true)` returns the exact limit error without
dispatching any task. Both managers and the SQLite connection are closed and
reopened; all task fields, historical run records and failure receipts must be
identical. There are zero validation plan revisions in this fixture.

The synthetic failure receipt deliberately contains usable command diagnostics.
The private failure receipt has not been supplied. This reproduces the native
boundary for the recorded state shape, **not the historical endpoint, model
request, command, environment or proof that the deployed source was identical**.
The supplied independent Goal-review event ordering is input evidence, not an
event replay. No request capture exists for that failure.

`TestRetainedExhaustionDiagnosisNearMiss` proves a final repair at 2/2 with only
two repairs against root max=3 still admits one priority repair. A missing
receipt cannot authorize another repair. This distinguishes chain exhaustion
from individual exhaustion. The cancelled and yield queue cases prove those
boundaries take precedence and leave task/run history unchanged.

## Verification obligations and authority inventory

The supervisor's corrected read-only input establishes the following facts:
the original and final repair have no validation plan revisions at all; both
have the same task script below; the original script passed on the retained
candidate; both tasks have no direct dependencies. It does **not** establish
the gate configuration or the contents of a bound failure receipt. The older
inaccessible-check observation is historical evidence of failure; the fresh
row projection did not reverify those terms in task metadata.

```sh
set -euo pipefail
test -f docs/ARCHITECTURE.md
grep -q '## Module Map' docs/ARCHITECTURE.md
test $(wc -l < docs/ARCHITECTURE.md) -gt 20
```

| Obligation or evidence | Source and disposition |
| --- | --- |
| Document exists, Module Map heading exists, document exceeds 20 lines | Exact task script supplied by supervisor; passing artifact evidence. Does not prove native completion or other checks. |
| Accepted required/blocking validation items | None in the supplied original/final-repair snapshot. `CanCompleteTask` preserves permissive legacy behavior when there is no accepted revision. |
| Blueprint lint and test stages | `internal/blueprint/engine.go:115` defines the standard task path; `internal/pipeline/pipeline.go:483` loads `execution.lint_commands` and `execution.test_commands` from project config. They are separate from `Task.VerificationScript`. Missing commands fail the deterministic stage. Original values and execution adapter are unknown. |
| Additional configured quality gates/checklist | Standalone manager setup in `pkg/manager/manager.go:460`, config in `internal/project/project.go`, `internal/execution/gates/config.go`, and stage opt-in in `internal/pipeline/pipeline.go:578`. StageExecutor uses the injected execution boundary instead. Original configuration/adapter/receipt is unknown. |
| Story verification/acceptance and task description obligations | Story and description content was not included in the bounded snapshot. `verifyStories` in `internal/cli/release.go:1646` executes story scripts when requested; it does not reconcile a native task. |
| Historical repository-wide inaccessible check | Its failure stays failed. It is required for this native task only if the configured/admitted check or accepted task contract made it so. A repository Makefile, task-specific script pass, or error summary alone establishes no such authority. |
| Stop, execution/effect authority, resource accounting | Not granted by a Goal review, artifact, completion API or this diagnosis. Ordinary execution retains native cancellation/admission/accounting; original scope remains stopped wherever its existing authority requires it. |

`TestRetainedExhaustionDiagnosisConfiguredGateAuthority` drives the real project
config loader, standard blueprint, deterministic executor, failure persistence
and queue repair boundary. A passing task document check cannot bypass a
configured failing test command. Conversely, a failing task script does not
replace a passing configured blueprint test command. Only unrelated
implementation/action stages are stubbed; this is evidence about gate routing,
not a completed implementation or an independent product review.

The **remaining original gate-authority question is unknown**, not waived.
The exact missing read-only projection for a supervisor is:

- `tasks.id, story_id, description, verification_script, needs_review, metadata`
  for the original and final repair, projecting the values of
  `verification_failure_evidence`, `failure_evidence`, `diagnosis` and
  their referenced record identities rather than merely their presence.
- `run_steps.id, run_id, phase, agent, iteration, status, metadata` for those
  referenced failures. Within metadata, the relevant fields are
  `verification_failure_receipt` (gate, command, output, exit_code),
  `verification_failure_digest`, `stage_output`, `stage_error`,
  `stage_diagnostics`, and registered artifact identities. Only bounded,
  redacted projections are needed, not raw private captures or documents.
- `stories.verification_script, acceptance_criteria, contract` for the parent
  story, plus the accepted task description/plan obligation projection.
- The incident candidate's effective `execution.lint_commands`,
  `execution.test_commands`, `quality_gates`, and relevant `quality.gates` /
  `quality.custom` name, command and mode configuration; also whether an
  admitted StageExecutor supplied different named checks. Project config uses
  `.openexec/config.json`, with `.uaos/project.json` fallback. Gate config uses
  `openexec.yaml`, with `.openexec/openexec.yaml` fallback. Historical effective
  values are needed; today's file alone cannot attest an old invocation.
- The exact receipt reference behind the historical inaccessible-check
  observation, and native running build/source identity for the two incidents.
  The observation's evaluation timestamp alone does not identify a `run_steps`
  row. No receipt ID or artifact hash for that observation was provided.

These are evidence dependencies, not owner strategy choices. The diagnosis
does not open the private store to obtain them or publish their contents.

## Existing primitives exercised

`TestRetainedExhaustionDiagnosisAcceptedCompletion` exercises all three release
completion routes: `CompleteTask`, `SetTaskStatus(done)` and `UpdateTask(done)`.
It constructs only graph identity with fixture SQL, then uses
`CreateValidationPlanRevision`, `AcceptValidationPlanRevision`, actual check
processes, structured run steps, `LinkValidationEvidence`, and
`FinalizeEvidenceCoverage`. It never inserts completion claims by hand.
Both required and blocking proof are covered. Missing, failed, wrong-state and
stale proof cannot complete the task. Supported proof produces a persisted
report and a passed leaf; `RunnableTasks` then releases the existing dependent.
Earlier failed tasks, attempts, limits, links, run records and receipts remain
unchanged after reopening. These APIs supply validation bookkeeping, not proof
that original authority has been granted.

`TestRetainedExhaustionDiagnosisLegacyCompletionLimitation` shows all three
routes accept a failed, exhausted legacy leaf with a failing script and
unfulfilled `hitl`/`needs_review`. They neither execute the script nor take
Stop/execution-authority inputs. Their availability is consequently **not a
lawful legacy reconciliation primitive**. These characterization tests document
present limitations; they are not a proposal to preserve a bypass in a future
correction.

`TestRetainedExhaustionDiagnosisRecaptureIsNotReconciliation` exercises native
diagnostic-free recapture through the public queue. It refuses exhausted
attempts before dispatch, refuses injected admission failure, and retains
cancellation. Terminal `needs_review` and the consumed attempts persist;
dependent work stays pending. Recapture spends existing attempts and does not
prove an entire task complete even when its recheck passes. It cannot lawfully
retry the supplied spent leaf. The usable-diagnostic reproduction follows the
other branch and reaches the repair-chain limit instead.

Sibling route inspection: the CLI `task complete` calls `CompleteTask`; MCP
`backlog_complete_task`, sequential queue completion and the older batch
scheduler call `SetTaskStatus`. These share the tested guard, but no CLI/MCP
transport-level authority enforcement is claimed by the release API tests.
Graph HTTP completion calls `FinalizeEvidenceCoverage` and reports coverage;
it does not transition an exhausted native task or resume its queue. Story
verification is reporting, not task completion. Repair creation for provider
stops and review findings also uses `CreateFailureRepair`; a finding on already
done work has different accounting by design and is not a substitute for the
recorded failed leaf. No guard, filter, matcher or runtime rewrite is added.

## Materially changed next step for the same queue

Do not run another read-only Goal review, reset attempts, retry the leaf,
manufacture a done-task finding, call permissive completion, or add replacement
documentation work. First resolve the bounded effective-gate/receipt evidence
above. If a required check genuinely remains failed, delivered artifact evidence
does not reconcile it: it needs genuine passing validation under existing
authority. No numerical-limit correction is justified by this reproduction.

If the retained work is fully validated and authorized, the precise current
capability gap is **native reconciliation of an exhausted legacy failed repair
leaf from current, complete task-and-config proof at the existing queue
boundary, with Stop/admission and unchanged historical accounting**. The tested
validation/reporting and completion primitives are reusable parts, but their
composition does not currently supply those guarantees for a no-plan legacy
leaf. A separate accepted correction would need this behavioral regression:

1. The same failed chain and pending task IDs, a current retained candidate,
   complete authoritative task/config validation, and sufficient existing
   verification authority allow a native validation disposition to make that
   chain delivered and select its existing successor.
2. Missing/failed/stale/wrong-candidate proof, unresolved gate identity, explicit
   Stop, absent admission, or exhausted verification authority refuse the
   disposition and successor dispatch. A passing document check alone refuses.
3. Failure receipts, spent task/repair counters, maximums, closed chain links
   and effect boundaries remain intact; any validation work is accounted for
   without resetting allowances or retrying implementation. No new backlog,
   controller or grant is introduced.

That is a technical correction target, not a claim to have found the private
failure's causal repair or permission to implement it in this task. Whether a
source correction is needed for the historical failing gate remains unknown
until its actual receipt/configuration is supplied. Autonomous safe legacy
reconciliation itself is not demonstrated by any current entry point.

Complexity delta: **0**. Runtime concepts added/removed: 0; persistent runtime
state added: 0; transitions added: 0; new owner decisions: 0; existing machinery
replaced: none. Only tests and this diagnosis are added. No loading, migration,
legacy format, permission, provider or production behavior changes.

After a separately accepted causal correction and checked installation, recovery
still requires genuine original validation, authoring the next existing useful
documentation task, and independent assessment of all original Ready conditions.
This synthetic diagnosis delivers none of those post-install observations.

## Executed verification receipts

Full narrow reproduction invocation:

```text
run_declared_check(check="go-test", args=["./pkg/manager", "-run", "TestRetainedExhaustionDiagnosisQueue/none", "-count=1", "-v"])
```

Full returned result (the temporary path contains only synthetic data):

```text
go-test exited 0.
=== RUN   TestRetainedExhaustionDiagnosisQueue
=== RUN   TestRetainedExhaustionDiagnosisQueue/none
2026/11/01 09:07:05 [StateStore] Initialized: path=/tmp/TestRetainedExhaustionDiagnosisQueuenone240628203/002/state.db, schema=v1, async_queue_size=1000
[MIGRATION] Bootstrapping from JSON files into SQLite (one-time only)
[MIGRATION] Bootstrap complete. SQLite is now the canonical source.
2026/11/01 09:07:05 [Watchdog] Started monitoring with stall threshold 5m0s
    retained_exhaustion_diagnosis_test.go:143: historical gate argv=["sh" "-c" "printf 'synthetic configured gate failed\\n'; exit 2"] output="synthetic configured gate failed\n" result=exit status 2
    retained_exhaustion_diagnosis_test.go:143: historical gate argv=["sh" "-c" "printf 'synthetic configured gate failed\\n'; exit 2"] output="synthetic configured gate failed\n" result=exit status 2
    retained_exhaustion_diagnosis_test.go:143: historical gate argv=["sh" "-c" "printf 'synthetic configured gate failed\\n'; exit 2"] output="synthetic configured gate failed\n" result=exit status 2
    retained_exhaustion_diagnosis_test.go:143: historical gate argv=["sh" "-c" "printf 'synthetic configured gate failed\\n'; exit 2"] output="synthetic configured gate failed\n" result=exit status 2
    retained_exhaustion_diagnosis_test.go:143: artifact argv=["bash" "-c" "set -euo pipefail\ntest -f docs/ARCHITECTURE.md\ngrep -q '## Module Map' docs/ARCHITECTURE.md\ntest $(wc -l < docs/ARCHITECTURE.md) -gt 20"] output="" result=<nil>
    retained_exhaustion_diagnosis_test.go:160: ExecuteTasks stop=none result=task repair attempt limit reached
2026/11/01 09:07:05 [StateStore] Initialized: path=/tmp/TestRetainedExhaustionDiagnosisQueuenone240628203/002/state.db, schema=v1, async_queue_size=1000
2026/11/01 09:07:05 [Watchdog] Started monitoring with stall threshold 5m0s
--- PASS: TestRetainedExhaustionDiagnosisQueue (0.21s)
    --- PASS: TestRetainedExhaustionDiagnosisQueue/none (0.21s)
PASS
ok  	github.com/openexec/openexec/pkg/manager	0.220s
```

All diagnostic scenarios, after the final test changes:

```text
run_declared_check(check="go-test", args=["./pkg/manager", "-run", "TestRetainedExhaustionDiagnosis", "-count=1"])
go-test exited 0.
ok  	github.com/openexec/openexec/pkg/manager	8.844s
```

Affected-package checks `go vet -buildvcs=false ./pkg/manager` and
`go build -buildvcs=false ./pkg/manager` both exited 0 with no output.
The CLI build `go build -buildvcs=false -o /tmp/openexec-retained-diagnosis ./cmd/openexec`
also exited 0 with no output; it is a build check, not an installed correction.
These are targeted checks, not a full declared gate. Full repository gates,
independent exact-head review, publication, installation and original-queue
recovery have no execution result in this diagnosis. The unattended delivery
lane owns review and hosted checks; their future execution is not evidence of
a pass now.
