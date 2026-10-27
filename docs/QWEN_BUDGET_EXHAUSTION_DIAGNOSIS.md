# Qwen implementation runs ending `inconclusive:budget_exhausted`: diagnosis

Signature `sig-5f0828d325c3`, original Console task `e5debaf1f4239643668399f8052b1849`.

Status: **cause of the missing terminal completion is unknown; no fix claimed.**
The two-hour ending is explained. Why the model never reached a final answer in
that time is not. This note holds no captured request or response content, and
nothing in it raises a limit or changes a model, provider, tool or deadline.

## Observed (from the task statement; run histories are supervisor-held)

Configured `qwen-27b` / `openexec-api`, two distinct implementation runs:

| Run | Start | End | Tool calls | Result |
| --- | --- | --- | --- | --- |
| `00d80764238d20480e3b8715de93ed18` | 2026-10-26T21:38:10Z | 23:38:10Z | 123 | `inconclusive:budget_exhausted` |
| `04f5951f75cd84a546e4b98ed71e9a6f` | 2026-10-26T23:44:39Z | 2026-10-27T01:44:40Z | 107 | `inconclusive:budget_exhausted` |

Supervisor reading of the second history: three intended documentation edits
were present and `git diff --check` passed, but the model kept re-confirming
references it had already inspected and emitted no completed response and no
candidate commit. The "119 files" figure is cumulative tool accounting, not the
three-file diff.

## Which boundary ended the runs

Three separate things are easy to conflate:

1. **The two-hour ending (explained, with a caveat).** Both runs lasted 2 h to
   within one second. In the Agent Console checkout at `/mnt/data1/projects/agent-console`
   (`internal/server/server.go`, `executeRun` caller) every run context gets
   `context.WithTimeoutCause(parent, 2*time.Hour, errNavigationBudgetExhausted)`,
   replaced by the persisted NavigationBudget's earlier deadline when one is
   bound. When the context ends with that cause the run is recorded as
   `inconclusive: budget_exhausted`. That is the only producer of the string
   found: this repository only lists `budget_exhausted` as an accepted wire
   reason (`internal/cli/execution_stdio.go` `validInconclusiveReason`) and
   `pkg/execution/execution.go` declares the constant; **nothing in OpenExec
   emits it.** So the label comes from the caller's wall-clock cancellation,
   not from an OpenExec tool loop outcome. Which of the two deadlines (fixed
   2 h fallback or a bound NavigationBudget) fired is not shown by anything read
   here; the exact 2 h length fits the fallback. The boundary "may be working as
   configured".
2. **OpenExec's own limit did not visibly end them.** `APIProvider.execute`
   (`pkg/execution/api_provider.go`) counts model *rounds* (`MaxSteps`, default
   16, unset by `internal/cli/execution_stdio.go`), not tool calls; reaching it
   gives one tool-less synthesis request and `inconclusive` / `max_turns`, which
   would have read `max_turns`, not `budget_exhausted`. A round may hold many
   calls, so 107 and 123 calls are compatible with fewer than 16 rounds. If that
   path ran, 16 rounds in 2 h means roughly 7.5 min per round, which would point
   at slow requests; this is an inference, not a measurement, and it is not
   known that the runs used this path.
3. **What the loop does when the caller's deadline fires.** It stops. During a
   model request the call returns the context error and the run ends `failed`
   with a failed event; between rounds it ends `cancelled`. In neither case is a
   synthesis request sent, so a deadline gives the model no chance to turn
   already-made edits into a final answer, while the round limit does. Pinned by
   `pkg/execution/caller_deadline_boundary_test.go`. These are the boundary's
   behaviour on authored responses; they do not reproduce the original run.

## Model/tool behaviour vs candidate/delivery state

- Model/tool behaviour: repeated re-confirmation of inspected references over
  100+ calls. **Cause unknown.** Candidates (not tested): a loop that never
  decides its work is done, tool results that do not read as "done", or slow
  requests that left few rounds. None is shown.
- Candidate state: edits existed in the worktree but were never committed or
  reported; the deadline ended the run first. The existing task `e5de` holds a
  saved "finish retained draft" basis and is queued; it is not touched here.
- Delivery state: nothing delivered from either run.

## Reproduction and its limits

Not reproduced. The original request and event history were not captured for
either run (`pkg/execution/request_capture.go`, #151, was not yet installed), so
`scripts/api-replay.py` has nothing to replay, and one endpoint call would not
reproduce a 100-call outcome. No run against the configured endpoint was made.
The new tests are deterministic boundary fixtures only:

- `TestCallerDeadlineDuringModelRequestEndsWithoutSynthesis`
- `TestCallerDeadlineDuringToolEndsCancelledWithoutSynthesis`
- `TestRoundLimitCountsRoundsNotToolCallsAndIsNotBudgetExhausted` (near miss:
  112 calls in 16 rounds ends `max_turns`, never `budget_exhausted`)

## Evidence needed on the next occurrence

1. The `request_capture` file of the last request (installed in native 155), with
   its message count and body size.
2. Per-round timestamps from the run history: rounds completed in the two hours
   and time spent in each model request versus each tool.
3. Whether the run used the stdio or pipeline path, and the NavigationBudget
   bound at run start, to tell the fixed 2 h fallback from a persisted deadline.
4. The model's last few assistant turns (bounded, redacted): repeating the same
   reads versus slow single rounds distinguishes a decision loop from latency.

## Complexity delta

Zero production change: one test file and this note.
