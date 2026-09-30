# Retention discard mutations — US-008 / T-US-008-004

Run `bash scripts/verification/retention-mutations.sh` from any directory.
Optional `--output /path/report.json` replaces the machine-readable report;
any previous report is removed before verification, so failure cannot leave a
stale success. This standalone verifier does not read the unit coverage report
or alter the shared dispatcher (owned by T-US-008-005).

The script snapshots current tracked and non-ignored untracked files into a
temporary candidate, then makes two independent copies of that snapshot.
Only `internal/blueprint/engine.go` is mutated. Exact unique replacement anchors
fail closed on source drift. No mutation touches the candidate worktree, and
all copies are removed on exit. Go test caching is disabled with `-count=1`;
tests have a 60-second timeout and each invocation has a 120-second limit.

The unchanged candidate runs the existing admitted exit-2/reopen/repair journey
and both engine API checks from T-US-008-001. The dedicated mutation journey
reuses its actual command fixture and evidence assertions, placing each engine
API inside the admitted executor seam. It checks persisted failure identity,
output, diagnostics and artifact reference after closing/reopening SQLite,
resolves exact argv/cwd and bounded streams, creates one repair, re-fetches it,
and refuses a missing evidence reference. The Execute adapter forwards the
original typed executor error because Execute itself returns a summary error;
it never reconstructs execution authority from prose or lost artifacts.

## Executed results (2026-09-29)

| Copy | Restored historical behavior | Required observed result |
| --- | --- | --- |
| Unchanged candidate | None | All five leaf tests pass, including both dedicated repair journeys. |
| ExecuteStage | Return `(nil, err)` immediately, discarding the executor result | API assertion `failed result/error pair lost`; dedicated ExecuteStage journey assertion `persisted stage evidence lost`. Other leaf tests pass. |
| Execute | Replace the non-nil result with a new failed result | API assertion `failed history lost`; original repair journey and both dedicated journeys assert `persisted stage evidence lost`. ExecuteStage API check passes. |

The Execute mutation affects both dedicated journeys because the surrounding
pipeline itself uses Execute. The ordinary admitted journey does not use
ExecuteStage; the dedicated adapter makes that boundary reachable without
changing production code. Both mutations compile and run the real fixture;
neither a compiler error nor an unrelated failed assertion counts as proof.

The JSON event validator requires every named leaf and parent/package completion
with exactly the expected pass/fail outcomes and process exit code. Each failed
leaf must emit exactly its designated evidence-retention assertion. It rejects
missing/duplicate results, skipped tests, compiler/tool diagnostics, malformed
output, runtime panics, unexpected passes and unrelated failures.

Verifier controls passed with
`python3 -m unittest discover -s scripts/verification -p test_retention_mutations.py -v`
(three tests, including eleven invalid-proof cases and missing/duplicate mutation
anchors). The existing `evidence-boundaries` verifier passed across manager,
evidence, gates, pipeline and blueprint packages. Shell syntax and `git diff --check` passed. Production compatibility:
no loading, schema, migration, runtime behavior or execution-authority changes.
Complexity delta: one standalone verifier and test-only adapters; no new native
loop, state transition or owner decision. Canonical repository gates remain
with the repository runner; this is verification evidence, not deployment proof.
