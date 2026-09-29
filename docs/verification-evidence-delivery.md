# Delivery preparation — US-011 / T-US-011-001

Current candidate evidence, 2026-09-29. Source parent is
`47c998821248dfbce16e7385c1ec7d6f75959f31`; this task changes only verifiers and
documentation. Earlier slice reports describe their own checkouts and are not
current execution proof. The supplied Console process observation is revision
`bcd2b229`, started `2026-09-29T17:28:25Z`. It identifies Console only; no OpenExec
product deployment, default-branch merge or external D2 was observed here.

## Verification contract

Run `bash scripts/verify-retained-verification-evidence.sh --case delivery-ready`
for fresh local technical preparation. It composes all eight slice helpers, the
native repair/resume journeys, architecture discovery and verifier negative
controls. Every required scenario must execute, including parent/subtest and
package completion; skipped, missing, failed, duplicate or unexpected scenarios
fail closed. Each invocation creates new output, records commands, preserves
nonzero exits and continues collecting other cases after a failure. The JSON
report records revision, uncommitted paths and externally pending D2. Its local
success does not certify repository gates, publication or owner acceptance.

`--case full` additionally requires `make test`, `make compat-test`,
`make type-check`, `make lint`, `make ui-build`, `make check` and `make pr-gate`.
There is no fallback that silently drops unavailable gates. Use the repository
runner for this potentially long-running mode. Both modes have failure-injection
controls proving every member's nonzero exit prevents success without dropping
remaining evidence. Missing/skipped/duplicate native scenario controls exercise
the actual report validation. Neither mode certifies D2.

## Requirement and journey mapping

| Requirement | Fresh proof | User-visible outcome / refusal |
| --- | --- | --- |
| REQ-001 | retained-result, evidence-boundaries | Real admitted failed process; both engine result branches; exact private argv/cwd and bounded streams persist across SQLite reopen; one repair binds readable evidence. Success, nil/error, cancellation, launch refusal, forged references, unsafe artifact access and public secret leakage are checked. |
| REQ-002 | legacy-recapture, recapture-boundaries | Authoritative command recapture, fresh failure receipt and one repair; successful recheck resumes A. Six terminal leaves persist across restart: exhaustion, remaining-budget exhaustion, already-spent budget, unresolved identity, refusal and cancellation. No repeated dispatch, attempt refund, repair or Settings execution. |
| REQ-002 | recapture-boundaries success/completion-obligation | Real successful check cannot complete A without its supported claim. After reopen Settings has zero attempts. Adding the supported claim permits A completion; reopening the native queue then completes Settings. All 22 persisted checkpoints are required. |
| REQ-001 / REQ-002 | native-journey | Controlled-provider real file checks fail, priority repair writes the missing feature, A resumes and remaining task B finishes. Separate journey closes manager/SQLite before repair, then resumes A from persisted failure. Admitted executor journeys verify trusted repair/resume and reject untrusted artifacts and unresolved legacy identity without host fallback. |
| REQ-002 compatibility | recapture-compatibility | Failure, success and unresolved identity for each of `.openexec`, `.uaos`, `.openexec/tasks.json`; nine leaf journeys and exact package completion. |
| D1 coverage | retention-unit-coverage, recapture-unit-coverage | Each slice independently exceeds 90% over entire changed function bodies; instrumentation and explicit function/test scope are mandatory. |
| D1 mutation | retention-mutations | Unchanged candidate passes. Restoring ExecuteStage discard fails its engine assertion and persisted-evidence journey. Restoring Execute discard fails history plus original and both dedicated persisted-evidence journeys. Compilation failure or unrelated assertion cannot count as mutation detection. |
| D2 | External Console evidence required | Publication, canonical gate, independent review, exact owner merge decision and actual default-branch merge remain outside this stage. Preserve existing T-US-011-002 and its dependency; no new approval task or decision reference. |

These are running backend workflows through public native entry points, real
local child processes and reopened persistent stores. They are not a browser,
live provider or deployed-service demonstration. Injected admission refusals
exercise the native boundary; they do not claim to exercise Console admission.

## Current verification results

Final `delivery-ready` exited 0. Fresh evidence is
`/tmp/openexec-delivery-3u0a2ed4/result.json`; raw logs and per-member reports are
in its sibling directories. These temporary artifacts are local, not publication
artifacts. All eleven members passed. Required completions: retention result 4,
evidence boundaries 12, retention coverage 74, mutation 8 per copy, legacy
recapture 11, recapture boundaries 9, recapture coverage 104, protected formats
13, and native journey 6. Counts include parent tests. Discovery checked 22
source declarations; all 54 Python verifier controls passed.

| Executed command | Exact result |
| --- | --- |
| `bash scripts/verify-retained-verification-evidence.sh --case delivery-ready` | Exit 0, all eleven members passed; no skipped required scenario. |
| Retention full-body coverage helper, invoked by aggregate | 530/582 statements = 91.06529209621993%, 29 current function bodies. |
| Recapture full-body coverage helper, invoked by aggregate | 427/471 statements = 90.65817409766454%, ten current function bodies. |
| Retention mutation helper, invoked by aggregate | Candidate passed; both independent restored discard branches rejected at required engine and persisted-evidence assertions. |
| `timeout 120 make compat-test` | Exit 0; protected compatibility suite passed. |
| `timeout 120 make type-check` | Exit 0; Go build and UI TypeScript passed. |
| `timeout 90 make lint` | Exit 0; Go vet and UI ESLint passed. Optional golangci-lint was absent and did not execute. |
| `timeout 90 make ui-lint` | Exit 0; current UI lint baseline is clean. |
| `timeout 90 make ui-build` | Exit 0; TypeScript and Vite build passed. |
| `timeout 90 make ui-test` | Exit 0; 40 files, 635 tests passed. |
| `timeout 120 make test` | Sandbox interrupted execution: network access to `api.anthropic.com` blocked by allowlist. No completed exit status was returned; no timeout or pass is inferred. UI checks were run separately above. |
| `timeout 90 make check` | Exit 2: `No rule to make target 'check'.` |
| `timeout 90 make pr-gate` | Exit 2: `No rule to make target 'pr-gate'.` |
| `python3 -m unittest discover -s scripts/verification -p 'test_*.py' -q` | Exit 0; 54 tests passed after final script edits. |
| `bash -n scripts/verify-retained-verification-evidence.sh` | Exit 0. |
| `git diff --check` | Exit 0. |

Baseline logs and exact command JSON are in `/tmp/openexec-delivery-baseline/`.
The sandbox aborted `make test` before that command could write its exit JSON;
its partial log is not a successful full-suite result. `--case full` was tested
with injected pass/failure results, but not run end to end here: canonical gates
are runner-owned, the targets are absent locally, and unrestricted repository
tests require capabilities unavailable here. These failures were not waived by
the verifier. Console must resolve canonical command mapping and run full gates.
Current lint success does not explain the historical unknown lint exit-2 cause;
no `knip` entry exists in the current UI package or lockfile.


## Coverage consolidation and compatibility evaluation

The first composed run correctly refused retention's old scope because the
later recapture slice added functions relative to retention's baseline. The fix
accounts for the exact union of both pinned function manifests, rejecting any
unowned or missing function, then measures all 29 retention function bodies in
the current candidate. Shared functions remain counted in each slice. Recapture
independently measures its ten full bodies. No production function is excluded
from the union, no threshold is lowered, and no historical coverage is reused.
A negative control removes a function and adds an unowned function; both fail.

Only Python/shell verification and documentation changed. No project loading,
migration, schema, receipt classification, retry or admission behavior changed.
Protected-format journey reruns and compatibility tests cover existing `.openexec`,
legacy `.uaos` and tasks.json fallback support. Complexity delta: no production
concepts, persistent state, transitions, owner decisions or replacement machinery;
one deterministic verifier composition reuses existing helpers.

## Delivery boundary and blockers

Canonical full repository success remains unverified here; exact current
baseline failures belong in the results above. Resolve command mapping and rerun
in Console's repository runner before declaring full D1 repository readiness.
D2 is externally pending, regardless of local checks or this candidate commit.
No publication, review request, owner presentation, merge or deployment was
attempted. The retained owner boundary remains with Console after the task queue.
