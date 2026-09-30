# Recapture story evidence — US-013 / T-US-013-003

Verified in the candidate worktree on 2026-09-30. This replaces the historical
US-009 aggregate summary. The requested outcome is bounded native recapture
with durable evidence, refusal and restart behavior. Existing manager queue,
attempt ledger, admitted deterministic executor, receipt storage and coverage
inventory are reused. Production behavior is unchanged; complexity delta is
zero runtime concepts, transitions, retry budgets or owner decisions.

## Criterion-by-criterion evidence

| Criterion | Executed proof | Persisted assertions |
| --- | --- | --- |
| Phase and command resolution | `TestRecaptureUnitPhaseIdentity`, `TestRecaptureUnitResolutionAndEvidence`, four incident journeys | Empty/matching lint/test phases resolve the named check without borrowing the task script; exact historical command takes precedence. Conflicting, ambiguous, missing, malformed and forged identity is refused. |
| Provenance | Resolution table, invalid-receipt tests, public silent/diagnostic evidence journeys | Registered private path, hash, argv and cwd required; foreign, unreadable, unregistered and legacy-directory references refused. Fresh failure binds new evidence; usable evidence produces one repair without recapture. |
| Attempt accounting | `TestRecaptureUnitAttemptRoundTrip`, ownership and persistence refusals, 48 named-check variants | Reopening SQLite preserves claims. Budgets with one, two or three attempts spent permit only two, one or zero further recaptures. A rejected retry write preserves failed status, count, original receipt and absence of stop-reason metadata. |
| Success transition | Legacy success/convergence and named variants | Successful recheck clears obsolete failure binding and returns A to pending. Ordinary execution then completes A and Settings; subsequent reload/resume does not execute again. |
| Reload/resume | Legacy failure/restart, interrupted budget and named variants | Fresh evidence creates one same-story repair; repeated resumes do not duplicate it. Interrupted attempts are never refunded. Named variants reopen before execution and after each of three queue invocations. |
| Refusal boundaries | Boundary helper, queue guards, SQLite write-fault tests, variants | Ambiguity, review/HITL, exhaustion, launch refusal and cancellation remain terminal across restart; Settings stays pending. Pre-claim refusal dispatches nothing; admitted cancellation/refusal retains its debit. |
| Protected formats and complete legacy suite | Four-member `recapture-story` aggregate | Real config/import journeys cover `.openexec`, `.uaos` and `.openexec/tasks.json`; reopened ledger wins over replaced tasks export. Boundary proof requires task and Settings snapshots for terminal/restart/stable and waiting/completed states. |
| Strict unit coverage | [Current full-body measurement](verification/recapture-unit-coverage.md) | Scope, every expected block, source hashes, test completions and threshold are checked; no partial profile or stale success is accepted. |
| Both isolated resolver negative controls | `legacy-incident` | Independently removing phase inference and named fallback makes the lint/empty incident fail specifically at unresolved/needs_review, with zero executions. Each mutation starts from original source in a temporary copy. |

The [scenario manifest](verification/recapture-scenarios.json) pins every
aggregate completion exactly once. Real shell checks traverse native queue,
admitted executor and persisted SQLite receipt paths; ordinary non-check stages
use fixtures. This proves local native behavior, not deployed Console adapter
adoption or external repair delivery.

## Commands and results

- `bash scripts/verify-retained-verification-evidence.sh --case recapture-story`:
  exit 0; all four helpers and all 226 pinned completions passed.
  Final result: `/tmp/openexec-recapture-story-nva6l4rm/result.json`.
- `bash scripts/verify-verification-repair.sh --case recapture-unit-coverage`:
  exit 0; task dispatcher invokes the strict gate; artifacts at
  `/tmp/openexec-recapture-coverage`.
- `bash scripts/verify-verification-repair.sh --case legacy-incident`: exit 0;
  all incident journeys and both expected mutation refusals passed.
  Output: `/tmp/us013-negative.log`.
- `python3 -m unittest discover -s scripts/verification -p 'test_recapture*.py'`:
  exit 0, 33 tests.
- `python3 -m unittest discover -s scripts/verification -p 'test_admitted_unit_coverage.py'`:
  exit 0, 16 tests; shared inventory behavior preserved.
- Host `run_declared_check(lint)`: exit 0, Go vet and UI ESLint.
- Host `run_declared_check(test)`: initial and final exit 0, all Go packages (final manager run: 105.755s)
  and 635 UI tests across 40 files; existing non-failing React warnings.
- Shell syntax and `git diff --check`: exit 0.

The first local gate reproduced a stale ten-function inventory that compared
all intervening stories against the old baseline. Study-owned scope fixes that
cause while retaining unowned-change refusal. The aggregate subsequently
refused newly executed but unlisted subtests; its manifest was updated, without
weakening exact completion checks. A Go module stat-cache write warning in the
sandbox did not prevent builds or tests.

Compatibility evaluation: only tests, verification tooling and evidence changed.
Canonical gates, publication, independent review and exact merge decision remain
owned by Agent Console and the repository runner.
