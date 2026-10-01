# Bounded exhausted-task reconciliation

US-008 / T-US-008-001 implements the native slice identified by the
[discovery record](../exhausted-task-discovery.md). That record describes the
pre-change refusal, not current reconciliation behavior. Live Project context
(accepted portfolio Goal revision 4) and the Simple Loop contract were read;
this change remains confined to the selected exhausted-task correction.

The native task queue owns admission and completion. The existing task metadata,
workspace writer lock, deterministic pipeline, accepted validation plans,
validation evidence links and completion claims provide the required machinery.
A completed repair or a failed receipt alone never grants another attempt.

## Native interface and persistence

A trusted caller supplies `release.TaskCorrection` to
`Manager.AuthorizeTaskCorrection`. Its explicit decision reference binds the
retained task and receipt, canonical candidate root, branch, exact Git candidate
digest, accepted validation plan and graph state. `CorrectionCandidate` computes
the candidate identity; `knowledge.BuildScanManifest` supplies the existing
validation state hash. The caller authenticates the decision; this API does not
add Console transport, infer permission from a receipt, or grant new effects.

The task's reserved `task_correction` metadata record is written once. Native
admission atomically consumes one whole pass of required checks before launching
any check. It does not increment, reset or replace ordinary attempt history.
Ordinary task creation cannot inject this authority, and ordinary updates retain
the durable record even when given a stale or nil metadata snapshot. Replaying
an authorization cannot renew it. Interrupted admitted work becomes needs_review;
it never silently reruns the consumed pass.

`ExecuteTasks` retains its explicit story scope, writer exclusion, dependency and
story selection, review/HITL boundaries, configured executor, timeout,
cancellation and Stop. It executes every accepted required/blocking item through
existing deterministic recapture stages with retries disabled. Supported items
use an accepted `sh -c` command or the existing named lint/test check adapter.
An absent plan, empty check set, unsupported check, stale graph, different
candidate or mismatched receipt refuses admission.

Each successful check produces a fresh native step and validation evidence link.
Completion uses the existing accepted-plan guard inside the disposition
transaction and additionally requires fresh evidence for this exact correction
decision, candidate and every required item. Historical supported claims alone
cannot complete it. Candidate identity is checked after each check and before
completion. Stop is serialized with the final disposition. Failure, interruption
or candidate drift retains unfinished work and the consumed allowance. Success
completes the retained task, enabling its dependent in the original native queue.

Candidate identity covers HEAD, branch, canonical root, tracked and unignored
untracked file bytes and modes, deletions and internal symlink targets. Escaping
symlinks, detached HEAD, subdirectories and unsupported Git inputs refuse.
Ignored runtime output is not candidate source; verification output must stay
outside the Git candidate. This does not attest arbitrary ignored dependencies
or a deployed product.

## Executed evidence

The following commands passed on the implementation candidate:

- `scripts/verify-exhausted-task-reconciliation.sh admission-completion`:
  three required named tests executed and passed, with reloaded SQLite evidence.
- `scripts/verify-exhausted-task-reconciliation.sh native-queue`:
  three required named tests executed and passed, with no skips.
- `python3 -m unittest discover -s scripts/verification -p
  test_exhausted_task_reconciliation.py`: three verifier controls passed.
  Missing runs, zero tests, missing package passes, skips, failures, duplicate
  runs and truncated persisted JSON refuse. Fragmented Go JSON output is
  reassembled before persistence evidence is parsed and printed.
- Existing discovery verifier controls: five passed, including source inventory
  and frozen-baseline provenance checks.
- Host `run_declared_check(check="lint")`: exit 0, Go vet and UI ESLint.
- Host `run_declared_check(check="test")`: exit 0, Go suite and UI Vitest
  (40 files, 635 tests). Both host checks were rerun after the production
  changes; subsequent test-only strengthening passed both task modes again.
- `make compat-test`: current `.openexec`, legacy `.uaos`, configuration fallback
  and legacy tasks JSON fallback journeys passed.
- `make type-check`: Go build and UI TypeScript checks passed.

The five distinct Go tests are `TestCorrectionAdmissionPersistence`,
`TestCorrectionCompletionObligations`, `TestCorrectionNativeQueueSuccess`,
`TestCorrectionNativeQueueRefusals` and `TestCorrectionNativeStopAndFailure`.
They use actual manager pipelines, real shell subprocesses, native repair
creation, accepted validation plans and actual database close/reopen. Inspected
reloaded snapshots retain the original task at 3/3, its branch and commit list,
the same completed repair identity and unchanged historical failure receipt.
Successful correction persists consumed/completed and the dependent completes;
unsuccessful correction keeps the dependent pending with no attempt spent.
Concurrent admission admits only once; restart after admission, after failure
and after completion cannot mint verification. Tests also cover absent authority,
candidate/task mismatch, review, dependencies, stale plans, cancellation, Stop,
real check failure, candidate mutation and stale task metadata writes.

No existing test assertions were weakened or replaced. New tests assert the
accepted correction behavior, while the frozen discovery continues to assert
the historical refusal. During verification, long Go JSON evidence was initially
printed incompletely; the verifier now joins output fragments and rejects a
truncated snapshot, with a regression control for both cases.

## Compatibility and complexity

There is no schema migration, new task type, executor, scheduler, controller,
owner decision or delivery route. One reserved task metadata record carries the
explicit correction and its consumption. The added bounded transition is failed
to in_progress to done/needs_review without refunding exhausted attempts.
Existing task selection and validation are reused; their ordinary interfaces
and legacy no-accepted-plan behavior remain intact. No current/legacy discovery
or JSON fallback path changed. The compatibility journeys above additionally
exercise those protected paths.

The complexity increase is the minimum durable binding and consumption needed
to make the observed exhausted-candidate refusal safely reconcilable across
restart. New refusal conditions are missing/moved authority, candidate,
validation or consumed allowance. No existing execution machinery was replaced.
This is native implementation evidence, not Console deployment, publication,
Goal acceptance or default-branch delivery evidence.
