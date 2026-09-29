# Legacy recovery unit coverage — US-009 / T-US-009-003

This slice verifies the existing native recovery loop, using the receipt and
restart fixture from T-US-009-001. It adds no production behavior, persistent
concepts, schedulers, transitions or owner decisions. Compatibility evaluation:
production sources, project loading, migrations and schemas are unchanged.
Existing native queue/restart/repair regressions execute alongside the new tests.
No recapture boundary verifier, boundary test, or compatibility result is consumed.

## Scope against the pre-slice baseline

The baseline is `c00aa9e11535f3498c1acbd750c2ab125d6f49e6`, immediately before
T-US-009-001. The pinned machine inventory and required test identities live in
[recapture-coverage-scope.json](recapture-coverage-scope.json).

All ten added or modified production functions are measured over their **entire
bodies**, including closures and unchanged branches:

| File | Function | Change | Responsibility |
| --- | --- | --- | --- |
| internal/pipeline/pipeline.go | (*Pipeline).runBlueprintMode | modified | Restrict recapture to a validated deterministic command, retaining admission and evidence handling; ordinary pipeline paths remain in scope. |
| internal/release/failure_repair.go | RunnableTasks | modified | Delegate ordinary selection to the shared predicate. |
| internal/release/failure_repair.go | runnableTasks | added | Apply scope, dependency, priority and attempt rules with a failed-task substitution for recapture selection. |
| internal/release/failure_repair.go | (*Manager).RecaptureEligible | added | Check eligibility without writing task status. |
| pkg/manager/task_execution_lock.go | (*Manager).reconcileInterruptedTasks | modified | Restore interrupted recapture to failed without refunding attempts. |
| pkg/manager/task_failure.go | (*Manager).repairTaskFromRetainedFailure | modified | Validate retained receipts and route insufficient diagnostics to recapture. |
| pkg/manager/task_queue.go | (*Manager).executeTaskQueue | modified | Process recapture in the existing queue and wait for dependencies. |
| pkg/manager/task_recapture.go | (*Manager).diagnosticFreeReceipt | added | Distinguish classification from usable public or registered private evidence. |
| pkg/manager/task_recapture.go | (*Manager).resolveRecaptureCommand | added | Resolve authoritative script/private command identity, refusing ambiguous or unavailable references. |
| pkg/manager/task_recapture.go | (*Manager).recaptureTaskFailure | added | Claim persisted attempts, run the check and persist success, failure or terminal disposition. |

The verifier inventories parsed Go function bodies against the baseline; a
manifest omission, extra identity, removed function, or empty scope fails.
It derives expected instrumentation independently with `go tool cover`.
Missing or mismatched blocks fail, even when a partial profile would exceed
the threshold. Duplicate package instrumentation is combined without counting
statements twice. The integer comparison is `10 * covered > 9 * statements`;
exactly 90% fails. Required tests and pinned recovery subtests must pass; any
skip or failure in the run fails the gate.

## Executed verification

On 2026-09-29:

- `scripts/verification/recapture-unit-coverage.sh` passed: **427 / 471
  statements (90.658174%)** across the full scope.
- The dedicated unit tests exercised authoritative and unresolved identity,
  public/private evidence sufficiency, invalid receipts, dependency selection,
  ownership guards, failed/ignored SQLite writes, persisted attempt claims,
  interrupted restart with one remaining attempt, fresh failure binding,
  successful receipt removal, and terminal restart refusal.
- Implementation journeys exercised real shell commands through persisted
  evidence, database reopen, same-story repair and queue completion; Settings
  remained blocked while prerequisites were incomplete.
- `python3 -m unittest discover -s scripts/verification -p 'test_recapture_unit_coverage.py'`
  passed all seven verifier controls: threshold weighting, missing/wrong
  instrumentation, partial bodies, empty scope, omitted functions, duplicate
  package blocks and skipped/failed/absent tests.
- Shell syntax and `git diff --check` passed.

## Aggregation interface

Run `scripts/verification/recapture-unit-coverage.sh --output DIRECTORY`.
It replaces its four artifacts before verification, so stale success cannot
survive failure: `scope.json`, `coverage.out`, `tests.jsonl`, and `result.json`.
The result includes the baseline, revision, production source hashes,
per-function statement totals, aggregate result, exact command and required
test identities. A threshold failure records `passed: false` and exits nonzero;
earlier failures leave no success result. The executed artifacts are at
`/tmp/openexec-recapture-coverage`.

Full canonical gates and delivery remain with the socket-capable repository
runner and Agent Console; neither was performed in this stage.
