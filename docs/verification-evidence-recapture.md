# Recapture verification — US-009 / T-US-009-005

Verified 2026-09-29 in the candidate worktree. This stage composes the existing
native recapture helpers; it changes no production execution, loading, migration
or schema behavior. Complexity delta: no production concepts, state transitions,
retry budgets or owner decisions. The existing dispatcher, Go journeys, JSONL
proof and coverage inventory are reused.

## Criteria and proof

The explicit [scenario manifest](verification/recapture-scenarios.json) pins
137 completions (including suite/subtest parents), partitioned by helper.
The aggregate requires every completion exactly once, the expected package
completions, and zero skipped or failed tests. Missing members and duplicate
manifest entries fail. Coverage includes 21 existing regression subtests beyond
the independent helper's required identities, so these cannot silently vanish.

| Criterion | Case and evidence | Assertions |
| --- | --- | --- |
| REQ-002: resolve and recapture legacy failures within the native budget | `legacy-recapture`; 11 completions; [implementation report](verification/legacy-recapture.md) | Real command through admitted executor, persisted receipt, database reopen, fresh repair reference; successful recheck resumes original task and Settings. Authoritative resolution refuses unsupported identity. |
| REQ-002: durable termination and dependencies | `recapture-boundaries`; 9 completions; [boundary report](verification/recapture-boundaries.md) | Six terminal leaves each require terminal/restart/stable snapshots. Failure requires repair/restart snapshots. Success requires waiting/completed snapshots. Both task and Settings records must be present; aggregate revalidates raw checkpoints and their report copies. |
| D1: comprehensive unit coverage strictly above 90% | `recapture-unit-coverage`; 104 completions; [coverage scope and report](verification/recapture-unit-coverage.md) | All ten changed production functions, entire bodies against baseline c00aa9e1; independently reconstructed instrumentation, exact function scope, source hashes and measured report totals. Empty/partial scope, missing blocks and exactly 90% fail. |
| REQ-002: protected formats | `recapture-compatibility`; 13 completions; [compatibility report](verification/recapture-compatibility.md) | Nine leaf journeys: failure, success and unresolved identity for `.openexec`, legacy `.uaos` configuration, and `.openexec/tasks.json` import. Exact parents/leaves and package completion required. |

Terminal journeys cover exhaustion, restart with remaining/spent budget,
unresolved command, launch refusal and cancellation. Reopened ledgers retain
attempt counts; restarting cannot refund attempts, dispatch again, create repair
or run Settings. Failed recapture binds readable private command evidence to one
repair. Successful recapture removes obsolete failure evidence but cannot bypass
the completion obligation: Settings has no attempt until supported completion of
its prerequisite. These are executed native queue/database journeys, with real
local subprocess checks and fixture executors for other stages; they do not
claim deployed service behavior.

Compatibility journeys enter through real config/import paths, reopen SQLite
before execution and reread it afterward. Separate tasks JSON is replaced with
an empty export before reopen to prove durable ledger state wins over stale
exports. Failure/refusal leaves Settings pending; success completes the queue.
Production format support is unchanged by this verifier-only composition.

## Fresh commands and results

All commands below ran from the workspace; helpers use a writable temporary Go
cache and fresh evidence directories. Raw logs are temporary artifacts; the
committed manifest and runnable helpers reproduce the checks.

| Command | Exit | Result / evidence directory |
| --- | --- | --- |
| `bash scripts/verify-retained-verification-evidence.sh --case recapture-story` | 0 | All four helpers exit 0; `/tmp/openexec-recapture-story-xdqdnwq4/result.json` |
| `bash scripts/verify-retained-verification-evidence.sh --case recapture-boundaries` | 0 | `/tmp/openexec-recapture-story-c0q5nn4q/result.json` |
| `bash scripts/verify-retained-verification-evidence.sh --case recapture-unit-coverage` | 0 | `/tmp/openexec-recapture-story-_qeezhw8/result.json` |
| `bash scripts/verify-retained-verification-evidence.sh --case recapture-compatibility` | 0 | `/tmp/openexec-recapture-story-dx05o95f/result.json` |
| `python3 -m unittest discover -s scripts/verification -p 'test_*.py' -q` | 0 | 48 verifier tests passed, including six new aggregate controls |
| `bash -n scripts/verify-retained-verification-evidence.sh` | 0 | Shell syntax passed |
| `git diff --check` | 0 | Whitespace check passed |

Full-body coverage: **427 / 471 statements (90.658174%)**. The aggregate rereads
`scope.json`, `coverage.out`, `tests.jsonl` and `result.json`, recomputes measured
coverage against source instrumentation and compares the source hashes. A
helper's exit-zero or claimed percentage alone cannot establish success.

Negative controls reject missing aggregate members/scenarios, duplicate manifest
entries, skipped/failed protected formats, absent reports, missing persisted
snapshots and incomplete function scope. An injected helper exit 7 stays failed
in the reloaded aggregate result while the remaining cases still execute.
The first real aggregate run also refused 21 unlisted coverage subtests despite
all helpers exiting zero; the pinned manifest was corrected before the passing
run above. Its failed proof remains at
`/tmp/openexec-recapture-story-knu9x3se/result.json`.

Each invocation allocates a fresh directory. Every helper's command, exit status
and combined log is retained; the aggregate writes `passed: false` on helper or
proof failure and continues collecting the other cases. Successful reports
include revalidated scenarios and the underlying reports. No stale success
artifact is reused. The legacy helper adds only an optional fresh output path
so the aggregate can consume its existing proof directly.

## Blockers and delivery limits

No stage-local blocker remains. Canonical full repository gates (`make check`,
`make pr-gate`), independent review, publication and delivery belong to the
socket-capable repository runner and Agent Console; they were not run here.
Earlier sibling attempts and their environment limitations remain documented
in their own reports and are not substituted for this fresh aggregate evidence.
