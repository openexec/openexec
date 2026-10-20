# Native watchdog cancellation evidence

The manager's existing watchdog measures time since real pipeline-event ingestion.
Standalone native stage-start events enable silence monitoring. Admitted
executors opt in per stage using the exported `runtime.StageInput` methods:

- Call `input.MonitorActivity()` before starting a provider only after connecting
  its actual progress callbacks to `input.ReportActivity()`.
- Report observed output/progress, never timer heartbeats or synthetic output.
- Before a blocking check inside an agentic stage, call
  `endCheck := input.BeginCheck()` and `defer endCheck()`. Scopes nest, completion
  is idempotent, and silence detection resumes with a fresh activity window when
  the last check finishes.
- Callbacks are scoped to one executor call; late reports cannot refresh or
  exempt a successor stage. Each stage resets its monitoring and wait state.

Uninstrumented admitted adapters (including Console's current adapter) are
excluded from silence detection: their silence is not evidence of a stall.
They retain explicit Stop, context deadlines and allowance cancellation. Console
must wire provider progress and check scopes before opting in; upgrading the
dependency alone does not prove integrated watchdog cancellation.

Paused/operator-attention execution and deterministic stages are also excluded.
The default executor brackets quality gates even when they run inside an
agentic stage. The standalone API runner also brackets `run_shell_command`
with these same check events: shell output is buffered until exit, so lack of
output cannot establish a stall while that bounded tool is running. The tool's
own timeout and the attempt context remain in force. This applies to all shell
commands without guessing whether command text names a check. Provider inference
becomes eligible again after the tool returns; no timer activity is fabricated.
The standalone CLI parser brackets identified `tool_use` calls with the same
check events, including built-in Bash and MCP calls. It matches results by
`tool_use_id` in Claude's `user.message.content` envelope (and the supported
top-level result format). Parallel calls keep separate scopes; duplicate in-flight starts
or results and unrelated results cannot close another call's scope. Error
results also end the matching scope. EOF or a terminal result closes remaining
scopes; cancellation joins the parser before closing its event stream. This is
a wait for an observed tool call, not a blanket agentic-stage exemption. The
CLI/tool retains its own timeout and effect authorization responsibilities.
Check scopes do not exempt Stop or deadlines. The watchdog
rechecks eligibility under the activity lock before cancelling the attempt.

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
store. It checks owner/unknown/plain/allowance/deadline cancellation, real
admitted subprocess output, uninstrumented adapters, paused work, a silent
deterministic subprocess and a silent gate executed by DefaultExecutor inside an
agentic stage. Stop and deadlines interrupt check scopes; silent inference
is cancelled after its check finishes. Executable removal
controls reject missing activity ingestion, relabelled cancellation, disconnected
admitted progress, a missing agentic quality-gate scope, a missing API shell
scope and a missing CLI tool scope. `TestNativeWatchdogAPIShell` runs a disposable
HTTP provider through the standalone native pipeline, a real buffered shell subprocess and the manager's
watchdog. It covers healthy completion beyond the silence threshold, owner Stop,
attempt deadline, tool timeout and silent inference after a completed tool;
completed files/checkpoints and separately reopened SQLite receipts are checked.
`TestNativeWatchdogCLITool` runs a disposable stream-json CLI process around an
actual OpenExec MCP `tools/call` and buffered shell subprocess. It proves healthy
completion across the threshold, tool timeout feedback, native owner Stop and
attempt deadline receipts during waits, and resumed watchdog cancellation after
tool return. Files, checkpoints and reopened SQLite receipts are checked here
too. This proves native attempt cancellation; it does not prove an external
provider CLI's descendant process cleanup. `TestParserToolWaitScopes` covers
parallel, duplicate, unrelated and error-result correlation; stream-end tests
check scopes are balanced. All run under the declared `test` gate. The pipeline
suite also verifies nested/idempotent scopes
and rejection of late callbacks.

Console task 222c2f84cfe48fd6e6bab27e819b3ef6 (Needs you fix 3) still owes its
dependency-pin update and integrated browser/server cancellation proof after this
native repair lands. This repair does not establish the original Goal as Ready.
