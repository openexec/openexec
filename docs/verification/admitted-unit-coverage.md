# Admitted-path unit coverage — US-012 / T-US-012-004

Run independently of Console, providers and host ports:

```sh
bash scripts/verify-verification-repair.sh --case admitted-unit-coverage
# Optional artifact directory:
python3 scripts/verification/admitted_unit_coverage.py --output /tmp/admitted-coverage
```

The gate measures complete current function bodies from the US-012 study scope,
including its planned public attachment API. The local inventory must match the
study. AST comparison against its pinned baseline adds every changed or added
function in those sources (including storage helpers and WithOutput wrappers).
Changed production files outside the three study stories are refused; the
explicit shared admitted test fixture is test support. Other stories' bodies
are not substituted for this story's denominator. Console's external scope is
not measured and cannot be satisfied by this local result.

Fresh Go instrumentation determines every block and statement count independently
of the supplied profile. Missing files, functions, profiles or blocks, changed
statement counts, stale source hashes, missing/failed/skipped required tests and
coverage at or below 90% fail. Each invocation deletes previous output first.
The output directory contains scope.json, coverage.out, tests.jsonl and
result.json (with measured totals, source hashes, revision, exact command and
external_scope_measured=false). Failed measurement cannot retain a stale success.

The required manifest combines existing result/error matrices for both engine
APIs, public attachment and classification cases, private stream round trips,
redaction, storage refusal, command tails and persisted repair journeys. New
unit cases cover partition-independent buffer boundaries and empty writes,
invalid receipt content despite a correct digest, and clearing prior attached
repair authority on the next nil-result success or transport refusal. Nil/nil
pair preservation is tested at the admitted adapter boundary. No assertion
claims an arbitrary nil/nil executor is valid for the full blueprint loop.
Real public silent/diagnostic and Console-shaped WithOutput fixtures execute
commands, close/reopen SQLite, create repair tasks and re-read persisted context.
The selected tests need no running Console or external model.

Verified on 2026-09-30:

- Standalone task gate: exit 0; 598 / 659 statements, **90.74355%**, across
  39 whole production functions and 68 required top-level tests.
- Python verifier suite: 107 passed, including 16 admitted profile/scope controls.
  The initial combined suite exposed leaked shared baseline configuration;
  the gate now restores shared configuration after each call.
- Declared host `test`: exit 0, Go suite and 635 UI tests in 40 files.
- Declared host `lint`: exit 0, Go vet and UI ESLint.
- Shell syntax and `git diff --check`: passed.

Compatibility evaluation: only tests, verification scripts and documentation
change. No project loader, schema, receipt format or runtime behavior changes;
existing .openexec, .uaos and tasks.json support is preserved. Complexity delta:
no runtime concepts, transitions, execution loops or owner decisions added.
Canonical gates, actual Console coverage, review and publication remain with
their existing owners; this local gate makes no deployment or delivery claim.
