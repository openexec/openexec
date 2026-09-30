# Retained-evidence unit coverage — US-008 / T-US-008-003

The pre-slice baseline is `1eb4cb694ad020a9f49a2171f80691e11ab839b8` (the architecture commit before
T-US-008-001 and T-US-008-002). The production input at verification was
`b97befa60a876e7b2d1f18c1248d1d8d734929d9`; this task changes tests, verification tooling and
documentation only. No product behavior, schema, project loading or migration
path changes here. Existing retained-result, private-access and reload journeys
remain required tests.

Run from the repository root:

```sh
scripts/verification/retention-unit-coverage.sh
python3 -m unittest discover -s scripts/verification -p 'test_*.py'
bash scripts/verify-retained-verification-evidence.sh --case discovery
```

## Executed result

The coverage command passed: **528 / 572 statements = 92.3076923076923%**, strictly
greater than 90%, across the full bodies of 29 added or modified production
functions. All 47 required top-level Go tests passed; no selected test or subtest
was skipped. The verifier/discovery Python suite passed 16 tests, including seven
coverage-gate negative controls. Shell syntax and whitespace checks passed.
The canonical repository gate is reserved for the repository runner and was not
run here. This is local verification evidence, not deployment evidence.

The real-process retained-result journey executes an admitted exit-2 command,
retains bounded evidence, closes/reopens SQLite, resolves artifact references and
creates a persisted repair. Native command fixtures additionally exercise terminal
pipeline evidence through external blueprint loading. Failure branches cover
refusals, cancellation, unknown/malformed blueprints, missing/corrupt/private-mode
artifacts, storage refusal, invalid repair receipts and unavailable provider
configuration. No provider request is made by these fixtures.

## Scope and denominator

[The checked manifest](retention-coverage-scope.json) pins the baseline,
function identities and required tests. The verifier independently compares Go
syntax trees from that baseline with current tracked and untracked product Go
sources under `internal/`, `pkg/` and `cmd/`. New functions and modified whole
function declarations are included; methods include their receiver identity.
Comments and formatting alone do not change the inventory. Removed functions
require explicit scope review rather than silently disappearing. Tooling under
`scripts/` is not product code and has separate verifier tests.

Every function below contributes all Go-instrumented statements in its full body,
including nested closures and unchanged branches. No difficult, unreachable or
error-handling statements are subtracted. This is aggregate statement coverage,
not a claim of complete branch coverage or >90% for each function.

| Production file | Function | Change | Covered / statements |
| --- | --- | --- | --- |
| `internal/blueprint/engine.go` | `(*Engine).Execute` | modified | 38 / 38 |
| `internal/blueprint/engine.go` | `(*Engine).ExecuteStage` | modified | 11 / 11 |
| `internal/blueprint/engine.go` | `failedStageResult` | added | 8 / 8 |
| `internal/blueprint/executor.go` | `(*DefaultExecutor).executeDeterministic` | modified | 48 / 48 |
| `internal/blueprint/executor.go` | `(*DefaultExecutor).runCommandWithCheck` | modified | 26 / 26 |
| `internal/execution/evidence/capture.go` | `(*Buffer).Len` | added | 1 / 1 |
| `internal/execution/evidence/capture.go` | `(*Buffer).String` | added | 1 / 1 |
| `internal/execution/evidence/capture.go` | `(*Buffer).Write` | added | 7 / 7 |
| `internal/execution/evidence/capture.go` | `Toolchain` | added | 5 / 5 |
| `internal/execution/evidence/capture.go` | `bounded` | added | 3 / 3 |
| `internal/execution/evidence/capture.go` | `CommandSecrets` | added | 4 / 4 |
| `internal/execution/evidence/capture.go` | `Public` | added | 5 / 5 |
| `internal/execution/evidence/capture.go` | `PublicStream` | added | 7 / 7 |
| `internal/execution/evidence/capture.go` | `Write` | added | 35 / 42 |
| `internal/execution/evidence/capture.go` | `Read` | added | 30 / 32 |
| `internal/execution/gates/failure.go` | `CommandFailureWithEvidence` | added | 4 / 4 |
| `internal/execution/gates/failure.go` | `NewFailure` | modified | 15 / 15 |
| `internal/execution/gates/failure.go` | `VerificationFailureArtifacts` | modified | 29 / 29 |
| `internal/execution/gates/runner.go` | `(*Runner).RunGate` | modified | 48 / 52 |
| `internal/pipeline/admitted_executor.go` | `(admittedStageExecutor).Execute` | modified | 11 / 11 |
| `internal/pipeline/admitted_executor.go` | `(publicExecutionError).Error` | added | 1 / 1 |
| `internal/pipeline/admitted_executor.go` | `(publicExecutionError).Unwrap` | added | 1 / 1 |
| `internal/pipeline/pipeline.go` | `(*Pipeline).runBlueprintMode` | modified | 124 / 152 |
| `internal/pipeline/pipeline.go` | `(*gateRunnerAction).terminalEvidence` | modified | 10 / 10 |
| `pkg/manager/task_failure.go` | `(*Manager).persistTaskVerificationFailure` | modified | 38 / 40 |
| `pkg/manager/task_failure.go` | `(*Manager).repairTaskFromRetainedFailure` | modified | 15 / 16 |
| `pkg/runtime/evidence.go` | `RetainCommandEvidence` | added | 1 / 1 |
| `pkg/runtime/evidence.go` | `ReadCommandEvidence` | added | 1 / 1 |
| `pkg/runtime/evidence.go` | `PublicVerificationStream` | added | 1 / 1 |

The engine functions preserve result/error combinations and executor identity,
timing, attempt, diagnostics and references. The deterministic executor functions
cover action responses, command execution, redacted callbacks and capture failure.
Capture functions cover bounded draining, metadata allowlisting, redaction and
private serialization/readback. Gate functions propagate trusted references while
excluding diagnostics and artifact identities from the classification fingerprint.
Pipeline functions sanitize admitted results, preserve error identity, retain
terminal evidence and refuse worker-supplied authority. Manager functions persist
references before publishing repair eligibility and construct the repair from the
persisted step, including output, diagnostics, errors and references. Runtime
functions exercise the exported adapter API round trip.

## Enforcement and aggregation

The gate runs uncached (`-count=1`) selected tests using Go coverage instrumentation
across all six packages. It independently invokes `go tool cover` for each scoped
file to enumerate the exact expected block coordinates and statement counts.
Every expected block in each full function must exist in the test profile with a
matching statement count. Repeated blocks from combined package profiles merge
execution counts but contribute statements only once. The gate uses integer
arithmetic (`10 * covered > 9 * statements`), so exactly 90% fails.

It refuses an empty scope, a manifest that omits a changed function, a stale
baseline, missing/mismatched instrumentation, an empty function denominator,
missing dedicated unit files, omitted dedicated tests, missing package/test pass
events, and any skipped or failed selected test/subtest. Dedicated tests are found
with the same Go parser; the manifest also requires the shared boundary and
retained-result journeys. Negative controls exercise empty/partial instrumentation,
wrong counts, omitted scope, absent/skipped/failed tests, strict threshold handling
and merging package profiles.

Each invocation replaces its output evidence, first removing stale success files.
Default output is `/tmp/openexec-retention-coverage`; `--output PATH` selects a
runner-owned artifact directory. Collect:

- `scope.json`: independently derived function identities and full-body positions.
- `coverage.out`: raw Go statement instrumentation and execution counts.
- `tests.jsonl`: exact Go test execution events, including subtests.
- `result.json`: baseline, source revision, production source SHA-256 hashes,
  exact test command, required tests, per-function counts, aggregate and pass flag.

`result.json` is absent if scope, compilation, test execution or instrumentation
validation fails; below-threshold coverage emits `passed: false` and exits nonzero.
Only a zero exit status and a fresh `passed: true` result authorize aggregation.
Artifacts are emitted outside source control; this document records the executed
summary. The shared dispatcher and sibling mutation files are unchanged.
