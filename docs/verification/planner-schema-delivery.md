# Planner schema delivery evidence — US-008 / T-US-008-004

Verified 2026-10-01 in candidate
`a2c7daf0a875c10027e2d680305633d0`, based on
`f2576a52d21dd19ad0396c8975630cf9f249a506` plus this task's verifier and
documentation changes. This is candidate evidence, not deployment evidence.

Outcome: prove that bounded scalar-schema correction is necessary, then exercise
rejected review → malformed refinement → correction → independent approval →
atomic import → database reopen/replay. Native refinement owns correction;
existing reviewed-plan receipts own reservations and import. This task reuses
those implementations and their runtime journeys. Complexity delta: one isolated
verification helper and two dispatcher modes; no production abstractions,
persistent state, transitions or owner decisions added.

## Commands and observed results

All commands below completed; none required a multi-minute background job.
The full repository test and lint ran through Console's declared host checks.
No `make check` or `make pr-gate` was attempted in the sandbox.

| Exact invocation | Exit | Observed output |
| --- | --- | --- |
| `bash scripts/verify-planner-schema-recovery.sh --case control` | 0 | Expected disabled exit 1; restored exit 0; source unchanged |
| `bash scripts/verify-planner-schema-recovery.sh --case all > /tmp/planner-schema-all.log 2>&1` | 0 | Control, discovery, recovery and strict replay passed |
| `make compat-test type-check GOCACHE=/tmp/openexec-compact-go-cache > /tmp/planner-schema-compat-types.log 2>&1` | 0 | Compatibility PASS; Go build and `tsc --noEmit` passed |
| `run_declared_check(check="test")` (declared `make test`, repeated after the full verifier) | 0 | `test exited 0.`; all Go packages passed; `Test Files 40 passed (40)`, `Tests 635 passed (635)` |
| `run_declared_check(check="lint")` | 0 | `lint exited 0.`; `go vet ./...` and `eslint src --max-warnings 0` passed |
| `bash -n scripts/verify-planner-schema-recovery.sh` | 0 | No output |
| `git diff --check` | 0 | No output |

Recovery runs fresh instrumented tests with `-count=1`, plus three Python
verifier controls. Replay runs fresh tests with `-count=1`, plus five Python
negative controls, and prints:

```text
pkg/runtime: 8 required tests passed; no skips
pkg/manager: 13 required tests passed; no skips
Strict schema replay verification passed
```

Full-suite Go results largely used Go's test cache; the changed-path control,
coverage and replay evidence is uncached. Raw replaceable local control receipts
and coverage live under `.openexec/planner-schema-checks/`; the two command logs
above are temporary. This document is the durable delivery receipt; generated
logs and caches are not committed.

## Repair-disabled control

`planner_schema_control.py` replaces the unique
`if errors.As(err, &decode) {` condition with
`if false && errors.As(err, &decode) {` in a temporary Go overlay. The decoder,
manager, review/import logic and tests are unchanged. Production source is never
mutated; discarding the overlay restores the repair even if verification fails.
The helper requires the approved subtest's failure event, exit 1 and both the
specific assertion and decoder diagnostic, so compilation or infrastructure
failure cannot pass as a regression control.

Actual disabled command (temporary directory is regenerated on each run):

```sh
go test ./pkg/manager -count=1 -timeout=60s -run '^TestReviewedPlanSchemaCorrection$/^approved$' -json -overlay /tmp/planner-schema-control-5793o77h/overlay.json
```

Observed exit **1**, failing
`TestReviewedPlanSchemaCorrection/approved` at `correction failed:` with:

```text
json: cannot unmarshal array into Go struct field Story.stories.requirement_id of type string
```

The same command without `-overlay …` exited **0**, including the approved
subtest pass and no skips/failures. Source equality was checked against the
original bytes; `internal/planner/review.go` SHA-256 was
`dbda84b6d430af06588ed6f47680c1e15a30051962f5f77a32455681343b2894`.
The complete `--case all` then repeated the control and restored verification.

## Runtime, persistence and refusal coverage

These are executions of the exported runtime and actual Manager.Plan with local
deterministic completion adapters and real SQLite/artifact writes. The approved
journey independently rejects the first plan, emits the concrete array failure,
reserves correction before dispatch, retains the full response and exact decoder
diagnostic, corrects the scalar mapping, independently approves and imports.
After closing/reopening SQLite, the journey replays with a larger configured
review limit and checks retained reservations, diagnostics and the original
limit. It reads two persisted tasks, exactly one import receipt and two review
history records. Task `T-1` remains `in_progress` with attempt count 2; scalar
`REQ-001`, retained goals and the HITL boundary survive reopening. Replay neither
dispatches again nor resets progress nor duplicates import.

Explicit refusals cover malformed correction, interruption/exhaustion, remaining
budget after restart, removed human boundary, invalid execution mode, missing or
malformed approval and explicit rejection. Reopened databases contain zero tasks
and imports in those cases. Legacy HITL without a decision reason remains HITL;
no reason or approval is invented. Atomic import rollback, conflicting request
identity and missing adapters also pass. The strict replay manifest rejects
missing, duplicate, unexpected, failed, skipped or wrong-package test evidence.

## Coverage and compatibility

Fresh full-body changed-function coverage is **222/239 statements (92.887029%)**,
strictly greater than 90%. The frozen machine-readable
[scope](planner-schema-coverage-scope.json) covers parseResponse, typed error
Error/Unwrap, RefinePlan, RefinePlanWithSchemaCorrection and replayReviewedPlan.
All statements in those bodies count, including 17 uncovered defensive/error
statements. This is changed-function coverage, not repository-wide coverage.
The removed coercing decoder and non-executable prompt constant are explicitly
accounted for by the [coverage gate](planner-schema-unit-coverage.md).

Compatibility command output includes passes for
`TestCompatibility_ExistingProjects_StatusCLI` (current `.openexec` and legacy
`.uaos`), `TestCompatibility_LegacyProjectConfigFallback` and
`TestCompatibility_LegacyTasksJSONFallback`. The latter actually loads
`.openexec/tasks.json` when SQLite is unavailable. This task changes no loader,
migration, schema or runtime behavior. Earlier schema repair intentionally
replaced array concatenation with strict scalar decoding and interrupted-budget
refunds with durable consumption; those existing accepted-behavior test changes
are documented in [repair evidence](planner-schema-correction.md). This stage
changes no Go tests and weakens no assertions.

## Limits and Console handoff

The host checks now execute successfully; historical dispatch refusals in earlier
stage receipts are not current blockers. The test run emits existing React
act/style warnings and a Vitest WebSocket port-in-use message despite exit 0 and
all UI tests passing. Local coverage emits a read-only Go module stat-cache
warning; type-check emits an experimental Node proxy warning. No check failed
except the deliberate repair-disabled control.

No live model, deployed OpenExec service, browser journey or paired Console
adapter was exercised. These deterministic public API journeys prove candidate
behavior, not model reliability or deployment. The supplied Console revision
is not an OpenExec deployment attestation. Console owns canonical gates,
independent review, publication and the owner's merge decision. No publication,
merge, deployment or retry of the parent Goal was performed.
