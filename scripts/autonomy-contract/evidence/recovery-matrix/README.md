# US-008 / T-US-008-002 verification

Verified in the candidate worktree on 2026-09-29. This is implementation-stage
behavioral evidence, not deployment or delivery evidence.

## Commands and observed results

- `bash scripts/autonomy-contract/verify-runtime-evidence.sh --case recovery-matrix`
  exited 0: 106 named required tests/subtests passed across runtime (44), gates
  (1), pipeline (11), blueprint (1), and manager (49); no skips.
- `bash scripts/autonomy-contract/verify-runtime-evidence.sh --case runtime-boundary`
  exited 0 with the extended runtime, gates, pipeline and blueprint inventories.
- `python3 -B -m unittest discover -s scripts/autonomy-contract -p 'test_*.py'`
  exited 0: 54 tests, including eight runtime-verifier tests.
- `git diff --check` exited 0.

## Behavioral path

`TestPersistedRecoveryMatrix` runs actual admitted shell checks for exits 0, 1
and 125, authenticates persisted terminal bytes through the caller-owned loader,
and invokes the exported runtime boundary through the native task queue.
Each exit has continuous and restart branches. Failure restart closes both the
manager and SQLite connection after the receipt, before repair insertion;
success restart follows the queue's durable completion step.

The success branches reopen two completed tasks with no repair or receipt.
The failure branches execute A -> repair -> A -> B, reopen three completed tasks
and exactly one receipt, and compare retained exit, terminal ID and task/stage
attempt bindings. They read the repaired and remaining-work files from disk.
Re-delivering the retained failure twice after reopening preserves the existing
repair; replaying the queue dispatches no work, adds no receipt, and leaves no
active failure metadata on completed tasks.

`TestPersistedTerminalRefusalsDoNotRepair` exercises 34 named refusals: missing or
tampered storage, worker source/artifacts, stale task/stage/attempt bindings,
missing exit, blank or wrong identity, zero/negative attempts, unavailable loader,
loader-returned typed failure, out-of-range exits, cancelled/timeout/launch/
transport outcomes, cancelled/expired contexts, and mixed errors. Each reopens
the manager and SQLite store and asserts one unfinished task at attempt 1,
no repair task, no attached receipt and no deterministic-verification run step.
Out-of-range and non-process terminal outcomes are persisted fixture values;
qualifying recovery exits come from actual subprocesses.

`TestTerminalBoundary` additionally refuses cancellation during loading and an
unknown terminal outcome. Invalid matching bindings test validation independently
of equality checks. `TestAdmittedTerminalBindingAndReceiptReset` covers success,
exit 125, mixed timeout and forged artifacts on a successful worker result,
alongside existing mutation, cancellation and receipt-reset cases.

## Verifier and scope

The recovery-matrix case requires the explicitly named six success/failure/restart
branches and all refusal branches, plus existing native recovery and admitted
boundary tests. It requires run and terminal test events, the expected package
identity, exactly one matching terminal package verdict and the expected process
exit. Empty runs, missing branches, skips, malformed output, package failures,
nonzero process exits and subprocess timeouts fail closed. A failure cannot emit
the final PASS marker. The shell entry point is invoked with bash, matching its
non-executable repository file mode.

Runtime behavior and persistence schemas did not need changes. Complexity delta:
no concepts, persistent state, runtime transitions, owner decisions or runtime
failure modes added; no machinery replaced. This extends tests and the existing
verifier. Existing local-command compatibility checks remain required. Project
loading and migration code are unchanged. The repository-wide gate is reserved
for the socket-capable repository runner as instructed; it was not run here.
