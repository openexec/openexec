# US-008 / T-US-008-003 implementation-unit coverage

Verified in this candidate worktree on 2026-09-29. This stage adds unit tests and
verification tooling; product implementation is unchanged. This is not delivery,
deployment, compatibility or canonical repository-gate evidence.

## Scope and method

The implementation baseline is the parent of the first behavioral slice,
identified by the full commit in the JSON reports. The AST inventory compares
that baseline with the current working tree, including untracked production Go
files, and selects every added or modified function declaration under `internal/`
and `pkg/`. It measures the **complete body**, including nested closures, using
Go cover-profile statement weights (`NumStmt`), not changed lines or a package
percentage. Repeated instrumentation blocks from the selected unit runs are
merged once; a block is covered if any selected unit test executes it.

`final.json` is the authoritative included-file/function inventory, covered and
total statement counts, remaining uncovered blocks and exact Go command argument
lists. All eleven included functions exceed 90%, as does their combined total.
The verifier enforces both using integer comparisons, so exactly 90% fails.
There are no excluded executable statements within those function bodies.

Excluded from the denominator: unchanged functions; test and fixture code;
verification scripts and their AST helper (tooling, not the consumed product
implementation); comments and type/config-field declarations with no executable
statements. In particular, the stage input/config additions are declarations;
their executing assignments in engine, pipeline and manager remain included.
No integration test, subprocess recovery test, compatibility test or package-wide
coverage average contributes to the result.

## Unit boundary

The explicit unit inventory is in `implementation_coverage.py` and each report's
commands. Existing runtime terminal-loader and admitted-executor tests use
in-memory doubles. Engine tests use a mock executor. Added receipt tests reject
malformed JSON, empty/invalid checks and malformed typed evidence.

New pipeline tests inspect inputs and events with an in-memory executor,
exercise invalid blueprint loading, configured commands without executing them,
checkpoint events, skills, timeout/attempt propagation, artifacts, failure and
review selection. A native configuration case uses a no-op gate runner, an empty
quality manager and local context/symbol fixtures, then refuses a missing API key
before provider dispatch. Context utilities may read local files and Git state;
no verification command or model executes. The asynchronous quality callback is
awaited. Manager tests use temporary SQLite fixtures, cancelled contexts and an
absent blueprint to test start configuration and admission without a task queue,
runner or recovery journey. Duplicate starts and queue-owned starts are refused.

## Reproduction and results

Run from the repository root; scripts set a writable temporary Go cache.

```sh
python3 -B scripts/autonomy-contract/implementation_coverage.py --baseline-units
bash scripts/autonomy-contract/verify-runtime-evidence.sh --case implementation-unit-coverage
python3 -B -m unittest discover -s scripts/autonomy-contract -p 'test_*.py'
bash scripts/autonomy-contract/verify-runtime-evidence.sh --case recovery-matrix
git diff --check
```

The first command intentionally exits 1: `baseline.json` measures the selected
pre-task unit inventory against the completed slices, without the added unit
tests. This is a unit-coverage baseline, not the earlier behavioral-reproduction
baseline. It still enforces the threshold. Empty baseline-only package runs
provide zero-count instrumentation; they are forbidden in the final unit run.
The second command exits 0 and emits the report retained as `final.json`.
Coverage profiles are temporary; rerunning produces them from source.

The Python suite passes 60 tests. Verifier negative controls reject missing or
malformed profiles, missing function instrumentation, skipped/incomplete/wrong
package verdicts, empty final inventories, exactly-at-threshold coverage and a
low-coverage function hidden by a high aggregate. AST tests include receiver
identity, signatures and nested closures; measurement tests ensure unrelated
functions and files cannot increase the numerator or denominator.

The affected behavioral recovery matrix was rerun separately: 106 required
named tests/subtests pass with no skips, including success, restart and refusal
branches. Its coverage is never loaded by this verifier. `git diff --check`
passes. Full repository gates remain assigned to the socket-capable runner.
No production repair or new runtime abstraction was needed; complexity delta is
limited to test fixtures and one AST-based coverage command.
