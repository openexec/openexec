# Operator correction interface — US-011 / T-US-011-003

The shipped CLI now exposes the existing manager correction API:

```sh
export OPENEXEC_OPERATOR_SESSION=1
openexec task correct A --dir /candidate/root \
  --authorize-correction --decision-ref owner:correction-123
# Optionally require a specific current accepted plan:
openexec task correct A --dir /candidate/root \
  --authorize-correction --decision-ref owner:correction-123 --plan PLAN_ID
```

Run this in a trusted local operator process. The launcher supplies the existing
`OPENEXEC_OPERATOR_SESSION` boundary (also used by operator MCP approval tools),
captured when the command is constructed. Agent processes must not receive or
control that launcher authority. The reference and explicit flag cannot promote
an agent session. As with the existing split, this is not authentication against
an actor with arbitrary host execution or database-write access. No correction
MCP tool or untrusted worker-output handler is added.

The operator explicitly authorizes correction and supplies its audit reference;
the reference is not looked up in a Console decision registry. References must be
1–256 ASCII reference characters, starting with an alphanumeric character and
otherwise using letters, digits, `. _ : / @ + -`. Whitespace and empty references
are rejected. Console transport must authenticate its caller, validate the actual
accepted decision and effect scope, and invoke this trusted boundary or the
manager API with exact native bindings. A caller-supplied reference alone is not
Console acceptance. That integration remains a Console-side follow-up.

The command reads `.openexec/openexec.db`, validates the retained deterministic
failure receipt, derives the Git candidate path/branch/digest, selects the current
accepted task plan if present and computes the scan-manifest state binding.
`--plan` is an optional assertion, never a way to select an older plan or skip an
applicable plan. The manager validates candidate/plan freshness and atomically
persists authorization for the exhausted failed task. Replays are refused.
Execution remains in `Manager.ExecuteTasks` with the existing story scope and
executor effects; this command runs no repair or verification stage itself.

## Verification

`python3 scripts/verify-task-correct.py` builds the public executable and runs it
against disposable Git/SQLite projects under this repository's `.openexec`.
The retained [transcript](task-correct-transcript.txt) includes public invocations,
exit codes and reopened records. Scratch projects and compiled overlays are
removed after verification. No live incident was authorized.

Planned and no-plan fixtures retain failed task A at 3/3, a completed repair,
a timeout receipt, dependent Settings and independent work. They exercise denied
agent access, absent explicit authority, missing/invalid references, incorrect
plan, stale graph, changed branch, malformed receipt and repeated authorization.
Before authorization the scoped native queue completes independent work and
returns `TaskQueueBoundary` while A remains failed. After authorization, actual
shell verification runs through the configured native executor; A and Settings
complete. Reopened SQLite asserts the exact decision, receipt, candidate identity,
plan/state binding, unchanged 3/3 attempts and completed repair history. Ordinary
implementation stages use the existing fixture's deterministic success adapter;
no provider or external effects are needed.

Two compiled CLI source overlays falsify the happy journeys: skipping the manager
authorization leaves A failed with `TaskQueueBoundary`; changing the decision
reference fails exact persisted-binding assertions. The unmodified source is
rebuilt and rerun afterward. The mutation selector excludes the denial subtest
so each control reaches its intended persistence assertion; positive runs include
all denials. A separate CLI test proves changing the environment after command
construction cannot promote an agent command.

No existing tests were weakened or replaced. Compatibility evaluation: no schema,
loader, migration, fallback, queue or correction lifecycle change; this is a new
opt-in CLI caller using the existing SQLite and manager APIs. Complexity delta:
one command, no new persistent record or scheduler. The host lint/test results
and final verification outcome are recorded below; canonical gates, independent
review, publication and merge remain Console-owned.

Final results (2026-10-01): task verifier exited 0, including both expected
mutation failures and the restored-source CLI/manager rerun. Declared host
`test` exited 0 (Go suite and all 635 UI tests); final declared host `lint`
exited 0 (Go vet and UI ESLint). `git diff --check` passed. The initial local
Go-cache write refusal was resolved with a writable temporary build cache;
no verification remained blocked. The canonical gate was not run in this stage.
