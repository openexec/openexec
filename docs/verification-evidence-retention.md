# Retention verification — US-008 / T-US-008-005

Verified in the candidate workspace on 2026-09-29. This composes the completed
[coverage report](verification/retention-unit-coverage.md) and
[mutation report](verification/retention-mutations.md) by executing their helpers
again against the current candidate. Historical reports alone never authorize a
pass. No production sources or sibling Go fixtures changed in this task.

## Executed commands and results

From the repository root:

```sh
bash scripts/verify-retained-verification-evidence.sh --case retention-story
python3 -m unittest discover -s scripts/verification -p 'test_*.py' -v
bash scripts/verify-retained-verification-evidence.sh --case discovery
bash -n scripts/verify-retained-verification-evidence.sh scripts/verification/retention-unit-coverage.sh scripts/verification/retention-mutations.sh
git diff --check
```

The aggregate passed all four members. The Python verifier suite passed 24 tests;
discovery passed 22 source declarations plus mapping, ownership and links; shell
syntax and whitespace checks passed. The discovery test formerly treating
`retention-story` as pending now checks only unknown/malformed requests.

Fresh machine evidence from the aggregate is at
`/tmp/openexec-retention-story-jvc4y4e4/result.json`. Its sibling JSONL logs record
executed Go scenarios; `coverage/` contains `result.json`, `scope.json`,
`coverage.out`, and `tests.jsonl`; `mutations.json` records candidate and both
mutants. Temporary evidence is local and is not committed. Every invocation
prints a new evidence directory, and the aggregate JSON records exact subprocess
commands, coverage source revision/hashes, and mutation assertion identities.

## Criterion mapping

Each command below uses the common prefix
`bash scripts/verify-retained-verification-evidence.sh --case`.
The [required scenario manifest](verification/retention-scenarios.json) names every
expected test and subtest; package completion is also mandatory. Counts include
parents: retained-result has 4 test completions, evidence-boundaries 12, unit
coverage 74 (47 top-level tests), and mutation proof 8 completions per copy
(including the package completion represented by the empty string).

| Criterion | Case argument / required evidence | Observed result |
| --- | --- | --- |
| REQ-001: preserve non-nil failed results in both engine APIs | `retained-result`; `TestRetainedResultEngineBranches/ExecuteStage` and `/Execute` | Both pass with original result identity, output, diagnostics and artifact references. |
| REQ-001: actual admitted failed check, usable repair after reload | `retained-result`; `TestRetainedResultAdmittedFailureReloadAndRepair` | Real exit-2 process; exact argv/cwd, bounded stdout/stderr and marker survive SQLite close/reopen; failed step identity and artifact lookup pass; exactly one persisted repair is re-fetched; missing evidence refuses repair. |
| REQ-001: bounds, private evidence and redacted public diagnostics | `evidence-boundaries`; capture, gate, native command, admitted result, trusted references, private access and reload scenarios | All pass. Reloaded event history excludes secret sentinels; private artifact retains exact command evidence; repair descriptions remain redacted. Private access rejects invalid paths/modes and symlinks. |
| REQ-001: classification stays independent of diagnostics; no new authority | `evidence-boundaries`; `TestRetentionBoundariesReload/failure` and trusted-reference checks; coverage reference/fingerprint tests | Reloaded receipt is exactly gate=test, exit_code=2; diagnostic marker and redaction retained separately; forged references do not supply authority. |
| REQ-001: nil-result, success, refusal and cancellation semantics | `evidence-boundaries`; reload `/nil-error`, `/success`, `/refusal`, `/cancel`; unit engine result/error matrix | Non-verification outcomes have no failure binding after reopen; repair is refused and task count stays one; unexecuted commands create no evidence; success remains completed with exit 0. |
| D1: strictly greater than 90% aggregate full-body statement coverage | `retention-unit-coverage`; scope derived from baseline `1eb4cb69`, checked against coverage scope manifest | 528 / 572 statements = 92.3076923076923%, across 29 added/modified production function bodies. No omitted instrumentation or skipped selected tests. Per-function counts and uncovered statements remain in the independent coverage report and fresh JSON. |
| D1: independent sensitivity to both historical discard branches | `retention-mutations`; unchanged candidate and independent ExecuteStage/Execute copies | Candidate passes. ExecuteStage mutant fails at `failed result/error pair lost` and dedicated `persisted stage evidence lost`. Execute mutant fails at `failed history lost` plus original and both dedicated persisted-evidence assertions. Compilation errors, panics and unrelated assertions do not count. |
| D1: complete fail-closed composition | `retention-story`; all four members above | Fresh helpers complete with no missing/skipped scenarios. Aggregate success is written only after every member validates. Python controls prove each subprocess failure propagates, empty output is rejected, and missing mutation branches/assertions cannot pass. |

The dedicated mutation journeys put each engine API inside the admitted executor
seam, run the real process, close/reopen SQLite, resolve persisted references,
create and re-fetch the repair, and refuse missing evidence. Execute also wraps
the outer pipeline, explaining why its mutation breaks both dedicated journeys.
This verifies a reachable running backend journey without a daemon or provider.

## Enforcement and limits

The dispatcher exposes all four member cases and the aggregate. The aggregate
uses the same implementation as each member case, always runs the independent
helpers, and preserves nonzero subprocess failures. Fresh unique output
directories prevent stale successful reports from satisfying a later run.
The committed scenario manifest is checked against complete pass events,
including subtests; missing, unexpected, duplicate, failed or skipped test
completions are rejected. Adding/removing scenarios requires explicit manifest
review. The independent coverage manifest continues to pin function and test
scope; the mutation validator continues to require exact assertion diagnostics.
The composition additionally checks report presence, coverage threshold, expected
mutation outcomes and explicit assertion maps.

No blockers remain for this implementation stage. Canonical repository gates
(`make check`, `make pr-gate`), independent review, publication and owner acceptance
belong to the repository runner/Agent Console and were not run here. This is not
deployment evidence. Compatibility evaluation: only verification scripts,
manifests and documentation changed; project loading, legacy `.uaos` support,
migration fallbacks, schema and execution authority are unchanged. Complexity
delta: one verifier composition module and explicit scenario manifest, with no
new production loops, transitions or owner decisions.
