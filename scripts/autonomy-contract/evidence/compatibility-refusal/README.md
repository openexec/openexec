# US-008 / T-US-008-004 compatibility and refusal verification

Verified in the candidate worktree on 2026-09-29, based on parent ea868a4d.
This is implementation-stage evidence; publication, review, canonical gating and
owner acceptance remain with Agent Console. No coverage report was consumed.

## Commands and observed results

- `bash scripts/autonomy-contract/verify-runtime-evidence.sh --case compatibility-refusal`
  exited 0: 125 required tests/subtests, no skips. Package inventories: runtime
  56, gates 1, pipeline 11, blueprint 1, manager 49, validation 7.
- `python3 -B -m unittest discover -s scripts/autonomy-contract -p 'test_*.py'`
  exited 0: 62 tests. The verifier's command tests reject missing local,
  refusal and protected-project branches, skipped/incomplete runs, wrong
  package identities, failing processes and timeouts. Behavioral failures
  cannot produce the final PASS marker.
- `git diff --check` exited 0.

## Exercised behavior and storage evidence

The exported local-command boundary executes actual shell processes for exits
0, 1, 125, 126, 127, 128 and 255. Nonzero results must be real `exec.ExitError`
values. Only exits 1 and 125 in that inventory authorize failure evidence;
forged text does not. Local mixed cancellation, timeout, launch and transport
errors are rejected both before classification and when joined to classified
failures, including reversed and wrapped error trees.

The existing `TestPersistedTerminalRefusalsDoNotRepair` runs all 34 named
refusals through the admitted executor and native task queue. It closes the
manager and SQLite connection, opens fresh ones, and queries stored tasks and
run steps. Each refusal leaves one unfinished task at attempt 1, no additional
repair, no attached verification receipt and no deterministic-verification
receipt row. This includes tampered/missing records, worker-forged artifacts,
stale identities/attempts, unsupported exits, non-process outcomes, loader and
context failures, and mixed errors. The verifier requires every named refusal.

The same command reruns the completed success/recovery slices: actual exits
0, 1 and 125, continuous and restarted execution, durable receipt replay,
and A -> repair -> A -> B with repaired files and completed tasks reloaded.
Existing local configured-command and admitted binding/receipt-reset checks
also pass. No production regression required repair.

## Protected project compatibility

Direct compatibility coverage now checks repeated disk loading:

- Existing `.openexec/config.json` projects: two fresh `status --json` CLI
  processes retain the expected project name/path and stopped daemon state.
- Legacy `.uaos/project.json` projects: the same CLI journey succeeds twice;
  repeated `LoadProjectConfig` calls preserve legacy name and resolved path.
- `.openexec/tasks.json` migration fallback: close the initial file source,
  reconstruct it, and verify the original persisted state/task files still
  produce the expected name, running/implement state, two workers and 50%
  completion. Both initial and reloaded branches are mandatory.

These are fixtures executed against the current compiled product, not claims
about a deployed instance. Project loaders, migrations, receipt schemas and
runtime code are unchanged by this task. Complexity delta: no new runtime
concepts, persistent state, transitions, owner decisions or failure modes;
no existing runtime machinery replaced. Only tests, the existing verification
entry point and evidence changed. The repository-wide gate is deferred to the
socket-capable repository runner as instructed; it was not run here.
