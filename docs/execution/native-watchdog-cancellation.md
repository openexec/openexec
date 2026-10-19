# Native watchdog cancellation evidence

The manager's existing watchdog measures time since real pipeline-event ingestion.
Blueprint start/stage events establish activity even when the admitted provider
never produces output. Timer heartbeats do not establish activity. Paused or
operator-attention execution and deterministic stages (including silent checks)
are excluded from silence detection; their own command deadlines and explicit
Stop remain effective. The watchdog rechecks eligibility under the activity lock
before cancelling the current attempt.

Cancellation uses the native attempt context, including context-bound commands.
It does not kill an unvalidated PID or launch a detached background restart.
Existing native task retention and Start/Resume own further execution within the
caller's authority. `Manager.Wait` joins execution and event persistence before
work-directory cleanup or evidence inspection.

Consumers of `manager.Status` and `manager.Subscribe` receive the additive JSON
field `cancellation_cause`. Terminal native events carry the same cause. Values:

| Value | Evidence |
| --- | --- |
| `watchdog_stall` | Existing watchdog cancelled an eligible silent attempt |
| `owner_stop` | Caller invoked `Manager.Stop` through its authenticated boundary |
| `timeout` | Attempt context deadline expired |
| `allowance_exceeded` | Caller cancelled with `manager.ErrAllowanceExceeded` using `context.WithCancelCause` |
| `unknown` | Plain context cancellation or an unrecognized cause |
| absent | No attempt cancellation observed |

`Stop` does not authenticate a caller itself; consumers must retain their existing
authentication/Stop controls. No error-message substring establishes cause
identity. A string saying `watchdog_stall` remains unknown. First cancellation
wins, and late buffered events cannot overwrite its stopped status. Stage-local
verification failures retain their existing failure semantics; `timeout` here
specifically denotes an attempt context deadline, not inference from stage text.

For durable receipt reconstruction after `Wait`, the existing SQLite `runs`
record contains `status=stopped` and `error_message` equal to the cause value.
Completed stage artifacts and checkpoint references remain available. No database
schema or new execution journal is required. Status snapshots remain in memory;
the persisted run record is the restart evidence. Audit metadata also includes
`cancellation_cause` when auditing is configured.

`pkg/manager/watchdog_native_test.go` exercises disposable native standard-task
execution with short watchdog thresholds, a real silent subprocess, terminal
subscription, retained useful work/checkpoint evidence and a reopened SQLite
store. It checks owner/unknown/plain/allowance/deadline cancellation, active work,
paused work and a silent deterministic check. Its removal controls rerun the
native silent probe with activity ingestion removed and watchdog cause relabelled
as owner Stop; both must fail. All run under the declared `test` gate.

Console task 222c2f84cfe48fd6e6bab27e819b3ef6 (Needs you fix 3) still owes its
dependency-pin update and integrated browser/server cancellation proof after this
native repair lands. This repair does not establish the original Goal as Ready.
