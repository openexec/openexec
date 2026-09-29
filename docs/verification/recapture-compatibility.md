# Protected-format recapture compatibility

US-009 / T-US-009-004, verified 2026-09-29 in the candidate worktree.

## Scope and source-backed evaluation

`pkg/manager/recapture_compatibility_test.go` owns nine journeys: each of
`openexec`, `uaos`, and `tasks_json` exercises failed recapture with fresh
repair evidence, successful recapture with queue completion, and unresolved
command refusal. Dedicated JSON fixtures live under
`pkg/manager/testdata/recapture-compatibility/` and are copied into a fresh
`testing.T.TempDir` for every journey.

- `internal/project/project.go:LoadProjectConfig` reads `.openexec/config.json`
  and falls back to `.uaos/project.json`. Both configurations are loaded before
  the queue starts and again after recapture/reopen; name and project directory
  are checked. Legacy support here means legacy **configuration**, not an
  invented `.uaos` task ledger.
- `internal/release/manager.go:Load` and `bootstrapFromJSONUnlocked` import
  embedded story tasks or separate `.openexec/tasks.json` only when SQLite has
  no stories. Fixtures enter through these real import paths, preserving the
  verification script and attempt budget. Before execution they close/reopen
  the database, exercising existing SQLite-backed projects. The separate-task
  fixture replaces its JSON with an empty task list after execution and then
  reopens again: SQLite status and attempt count must survive stale exports.
- `pkg/manager/task_recapture.go` supplies the runnable mechanism directly.
  Journeys call `Manager.ExecuteTasks`, reusing only T-US-009-001's shared
  receipt, executor and restart helpers in `recapture_fixture_test.go`.
  Real `sh -c` commands run through `blueprint.DefaultExecutor`. Failure checks
  reread the retained command evidence (exact command, cwd, exit 2 and stderr),
  fresh failure binding, and exactly one evidence-linked repair. The fixture
  stops at ordinary repair execution; it does not claim repair completion.
  Success executes the original task and dependent Settings through the queue,
  using controlled successful results for non-verification stages. Unresolved
  identity dispatches no command, consumes no attempt and stays terminal after
  restart. Settings stays pending on failure/refusal.
- `internal/tui/file_source.go` retains its tasks JSON progress fallback when
  SQLite is unavailable. The existing `make compat-test` exercised this path,
  both current and legacy CLI status, and legacy config loading.

Only dedicated tests, fixtures, the verifier and documentation changed. No
loader, migration, schema, executor or queue behavior changed. The exercised
paths preserve protected-format support; this is not a claim that every
historical project shape is supported. Complexity delta: no production concepts,
state, transitions, owner decisions or replacement machinery added.

## Fresh verification

| Check | Result | Raw local evidence |
| --- | --- | --- |
| `bash scripts/verification/recapture-compatibility.sh` | PASS: nine leaf journeys, three format parents and suite parent; no skips | `/tmp/openexec-recapture-compatibility-od3i14d0/result.json`, `tests.jsonl`, `stderr.log` |
| `python3 -m unittest discover -s scripts/verification -p test_recapture_compatibility.py` | PASS: four verifier controls, including missing leaves/parents/package, skipped/failed/duplicate/unexpected events and malformed/empty input | terminal execution |
| `bash -n scripts/verification/recapture-compatibility.sh` and `git diff --check` | PASS | terminal execution |
| `make compat-test` | PASS, exit 0 | `/tmp/recapture-compat-test-iu0cfl8r/output.log`, `result.json` |
| `make type-check` | PASS, exit 0; Go build and UI `tsc --noEmit` | `/tmp/recapture-type-check-qcj3f_bu/output.log`, `result.json` |
| `make test` | BLOCKED: sandbox rejected provider network access; no completed exit status, UI tests not reached | partial `/tmp/recapture-test-sve_39ko/output.log`; tool failure quoted below |

Exact `make test` tool failure: `Unified exec process failed: Network access to
"api.anthropic.com" was blocked: domain is not on the allowlist for the current
sandbox mode.` The partial log reached `go test ./...` and reported the
`internal/agent` package passing; it does not identify the test making the
blocked request. The sandbox interrupted the process before the wrapper could
write `result.json`. Neither an exit code nor full-suite success is inferred.

Make commands ran with `GOCACHE=/tmp/openexec-retention-go-cache` and separate
fresh `TMPDIR` directories, using a 110-second process-group deadline per target.
This bounds local checks; it does not turn incomplete checks into passes.
Canonical full gates remain assigned to the socket-capable repository runner.
No publication, merge, deployment or independent delivery review was attempted.

## Verifier contract

`scripts/verification/recapture-compatibility.sh [--output NEW_DIRECTORY]`
executes fresh Go tests with `-count=1 -timeout=60s -json`, records stdout and
stderr, and emits `result.json`. It refuses an existing output directory and
uses an isolated child TMPDIR. `passed: true` requires exit zero plus exact
required scenario and package completion, with no skips, failures, duplicates
or unexpected scenarios. A failed run records `passed: false` and exits nonzero.
The test manifest is exposed as `REQUIRED` in `recapture_compatibility.py` for
T-US-009-005 aggregation. No unit coverage report or boundary verification
output is consumed. Raw temporary logs are not committed artifacts; this note
is the durable evidence record and the script reproduces the proof.
