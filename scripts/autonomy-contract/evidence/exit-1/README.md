# US-008 / T-US-008-001 evidence

The user outcome is persisted deterministic verification failure entering the
existing native repair queue, followed by original-task resumption and next-task
execution. Inspection of this candidate found the exported runtime execution
seam, admitted pipeline adapter, failure-artifact converter and native repair
queue present. Prior missing-resource notes describe an earlier checkout.
The native task loop owns repair; Console still owns authorization and delivery.

## Reproduction

`baseline-revision.txt` identifies the unmodified product tested before this fix.
`baseline_test.go.fixture` uses only that revision's exported runtime classifier,
admitted executor and ordinary manager queue. It runs `sh -c 'exit 1'`, persists
and reloads the actual completion, invokes the old classifier with the reloaded
error, and reopens SQLite. A remains failed at attempt 1, only A/B exist, and no
repair ran. The test fails its behavioral assertion, not compilation or API
availability. `baseline.jsonl` preserves that run. `baseline-manifest.json`
protects the recorded fixture/log/revision from accidental edits.

The behavioral verifier extracts that revision into a temporary directory and
reruns the fixture before checking the candidate's successful regression.
It requires the test's failure event and specific observed-state diagnostics;
build failures, skipped tests and empty runs cannot stand in for reproduction.

## Boundary and trust

`runtime.VerificationTerminalFailure` consumes an ID through an explicitly
trusted loader owned by the admitting caller. The loader must authenticate the
producer, verify immutable stored bytes and terminal status, and refuse unknown
records. The source string or receipt digest alone never supplies authority.
This is the same trusted-executor responsibility as the existing local-command
classifier; arbitrary worker JSON/artifacts cannot opt into it. The tests use
an authenticated fixture store outside the worker directory, reopen its bytes,
and reject tampering. Integration with a particular external Console terminal
store remains that caller's responsibility; this stage does not deploy one.

The classifier requires matching task, stage, task attempt, stage attempt,
source, completion ID, normal process outcome and an actual exit code. Normal
exits 1–125 classify, exit 0 succeeds without repair, and other exits or failure
outcomes refuse classification. Loader errors cannot launder typed failures.
The pipeline snapshots the executing identity before dispatch and independently
matches returned evidence. Existing artifact conversion continues rejecting
mixed errors and worker-supplied artifacts. Receipts retain the completion ID,
exit and complete binding using the existing run_steps payload.

## Native recovery

`TestPersistedExitOneRecovery` executes real commands against a temporary project:
A fails `test -f feature.txt`; the native repair task writes that file; A resumes;
B verifies the file and writes remaining.txt. Both continuous execution and
restart after receipt persistence before repair creation are required subtests.
Reopened storage must contain exactly three completed tasks, A at attempt 2,
one repair with retained failure evidence and matching terminal identities.
Reexecuting the completed queue must dispatch nothing.

The lifecycle refusal inventory reopens storage after tampering, missing records,
stale task/stage/attempt bindings, worker source/artifacts, mixed errors,
cancellation/timeout/launch/transport outcomes and exit 126. Each requires no
repair and no deterministic verification receipt. Focused boundary tests also
cover exit 0, exit 125, malformed exits, missing exits, cancellation context,
loader errors, wrapping and local exec.ExitError compatibility.

## Verification

Run each case with `bash scripts/autonomy-contract/verify-runtime-evidence.sh
--case CASE`: behavioral-reproduction, runtime-boundary, native-recovery-tracer,
and exit-1-slice. `runtime_evidence.py` declares the required test inventories,
including lifecycle branches, and rejects missing, skipped, failing and
zero-test runs. `test_runtime_evidence.py` tests those refusals. The recorded
candidate output is in `candidate-verification.txt`.

Verification also passed `python3 -B -m unittest discover -s
scripts/autonomy-contract -p test_runtime_evidence.py` (three verifier tests),
all 49 autonomy-contract Python tests, and
`go test ./internal/validation/... -run Compatibility -count=1 -timeout=90s`.
Targeted `go vet` also passed for runtime, gates, blueprint, pipeline and manager.
A broader package test attempt was interrupted by the sandbox when an existing
test contacted api.anthropic.com; that run is incomplete, not a passing gate.

No database schema, legacy project loading, migration or task JSON fallback
changed. Existing local command evidence remains supported and tested. The
canonical repository gate and deployment remain outside this implementation
stage; test receipts are not deployment evidence.

Complexity delta: no scheduler, task type, persistent table, controller, owner
decision or recovery transition added or replaced. Added one structured terminal
loader seam and optional binding fields in the existing receipt. Task and stage
attempts propagate through existing stage input. Invalid or unavailable trusted
loads now fail closed; existing native repair/resume transitions are reused.
