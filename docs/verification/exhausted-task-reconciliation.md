# Bounded exhausted-task reconciliation

US-008 / T-US-008-001 and T-US-008-002 implement the native slices identified by the
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

## Exhaustion and continuing failure

Exhausted tasks without correction authority retain failed status and the
original `verification_failure_evidence`, with an `exhaustion` disposition in
existing metadata. They do not enter repair creation. Interrupted legacy
recaptures retain their existing exhausted/needs_review disposition. Independent
runnable tasks drain using the existing story/dependency/priority policy; the
queue then returns its typed retained-work boundary. Dependents stay pending.

An admitted correction's fresh failure is atomically persisted in run_steps and
`task_correction.fresh_evidence_id`, without replacing its original receipt
binding. The consumed record becomes continuing_failure/needs_review with the
actual failure reason. This also drains independent work without a new repair.
Denied execution, Stop, cancellation, drift and unmet completion obligations
persist a terminal correction disposition and end that invocation. Re-entry
cannot renew consumption. A crash between fresh failure persistence and final
disposition becomes interrupted/needs_review on restart.

Stop remains effective even if an executor returns success afterward: late
pipeline events cannot overwrite stopped status. Both task disposition and run
status are reread after database reopen. Completion still checks cancellation,
candidate identity and accepted validation obligations, including fresh proof
for every required item. Agent prose, artifact claims and historical supported
completion claims do not discharge those obligations.

## Executed evidence

Fresh verification for T-US-008-002 uses the executable default acceptance suite:
`scripts/verify-exhausted-task-reconciliation.sh`. It runs verifier self-tests and
both native slices, requiring all 31 named tests/subcases to run and pass with no
skips. `exhaustion-controls` is also a working standalone mode requiring 16
named tests/subcases. Admission/completion and native-queue modes remain usable.
The verifier rejects missing subcases, missing package passes, duplicate tests,
failures, skips, unstructured evidence and truncated persisted JSON. Five Python
control tests exercise these refusals and fragmented-output reconstruction.

The new native queue journeys cover continuing and unchanged check failure,
exhaustion without authority, blocked dependents, independent draining, denied
effects, agent-only claims, cancellation after admission and at completion,
Stop at completion and unmet validation introduced during verification. Each
journey closes and reopens SQLite, rereads the retained task, receipt, consumed
allowance, completed prerequisite and dependent, then re-enters the queue and
tries to renew authority. Continuing failures also reread the actual command
evidence file, checking argv, cwd and exit code (including unchanged exit 2).
No extra repair may appear. Ordinary task attempt history remains 3/3.

`TestCorrectionNativeQueueRefusals` additionally covers interruption after a
fresh failure was persisted but before disposition. The release persistence
test now emits structured reloaded state and checks failure cannot re-admit.
`TestRecaptureBoundariesTerminal/restart_spent` deliberately now expects the
native drained-queue boundary instead of an immediate recapture error; its
exhausted state, history, blocked-dependent and no-dispatch assertions remain.
The existing named recapture variants retain their original assertions.

The new Stop-at-completion test first failed with `control bypassed`, exposing
a queued completion overwriting Stop. It passes after the status fix. Related
recapture tests exposed a lost legacy exhausted disposition; that production
compatibility gap was fixed, rather than weakening the state assertions.
The initial host test check exited 2; final check results are recorded below.

- Default acceptance suite: exit 0, all required cases pass; reloaded evidence
  and dispositions parsed again by the verifier.
- Standalone exhaustion-controls: exit 0; 16 required tests/subcases, no skips.
- Related legacy named-recapture and terminal tests: exit 0 (17.389 seconds).
- `make compat-test`: exit 0; current `.openexec`, legacy `.uaos`, configuration
  and tasks JSON fallbacks passed.
- `make type-check`: exit 0; Go build and UI TypeScript passed.
- Host `run_declared_check(check="test")`: exit 0; Go suite and UI Vitest
  passed (40 files, 635 UI tests). The final acceptance rerun additionally
  checks structured release failure evidence and persisted stopped run status.
- Host `run_declared_check(check="lint")`: exit 0; Go vet and UI ESLint passed.
- `git diff --check`: exit 0.

## Compatibility and complexity

There is no schema migration, new task type, executor, scheduler, controller,
owner decision or delivery route. Existing task metadata carries the correction, consumption, failure evidence
and exhaustion disposition; no table or schema was added. The added bounded transition is failed
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
