# Recapture unit coverage — US-013 / T-US-013-003

The current gate supersedes the historical US-009 coverage measurement.
The declared baseline is `09ac4feb2c7325b42f20a4663b9aa111d43fe386`.
[Study ownership](repair-coverage-scopes.json) and the
[pinned inventory](recapture-coverage-scope.json) declare 14 full function bodies:
the original ten recapture functions, phase inference, repair-task recognition,
retry stop-reason persistence and attempt description. Closures and unchanged
branches count. Production sources, loaders, migrations and schemas are unchanged.

The gate reuses the study inventory implementation already used by US-012.
It includes added/modified helpers in declared sources and refuses unowned
production sources, missing bodies, inventory drift or a weakened threshold.
The shared admitted inventory retains its original default story; both story
selections have negative controls. No new runtime abstraction is introduced.

Expected statement blocks are independently reconstructed by `go tool cover`.
Missing/mismatched instrumentation, partial profiles, stale source hashes,
duplicate/skipped/failed/absent test completions and empty scopes fail.
Package profiles are combined without counting statements twice. The integer
threshold remains `10 * covered > 9 * statements`; exactly 90% fails.

## Current measurement

**473 / 525 statements (90.095238%)**, across all 14 functions, measured on
2026-09-30. Both resolver functions have every statement covered.
No blocks were excluded to reach the threshold. The initial expanded
measurement failed below the threshold; retry persistence refusal coverage
closed the gap.

Run `bash scripts/verify-verification-repair.sh --case recapture-unit-coverage`
or `bash scripts/verification/recapture-unit-coverage.sh --output DIRECTORY`.
The verifier replaces `scope.json`, `coverage.out`, `tests.jsonl` and
`result.json` before starting. Results record exact commands, required tests,
baseline, revision, source hashes, per-function totals and the threshold result.
The legacy aggregate independently rechecks those artifacts against its exact
scenario manifest. Current commands and persisted-state evidence are retained
once in the [story evidence](../verification-evidence-recapture.md).
