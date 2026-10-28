# Qwen header timeout: diagnosis (original run e35efd1e, task 18da)

Status: **not reproduced, cause unknown, no fix claimed.** This note holds only
what the repository and the task statement establish. It contains no captured
request or response content.

## Failure as reported (verbatim)

Original third implementation `e35efd1ee1f008766a01057cf48f8dd5`, configured
`qwen-27b` / `openexec-api`, ran 23:59:53Z–01:25:26Z and failed `server_error`:

    HTTP error: Post local11436/v1/chat/completions: context deadline exceeded (Client.Timeout exceeded while awaiting headers)

Last durable tool read (`internal/domain/blocked.go`) completed 01:10:25Z.
No candidate, changed gate or implementation result exists.

## Established facts

| Area | Fact | Source |
| --- | --- | --- |
| Transport deadline | `http.Client.Timeout` covers the whole request, and with no response headers it fires as "awaiting headers". | `pkg/agent/openai_provider.go` (`NewOpenAIProvider`) |
| Transport deadline | The stdio execution path gives loopback endpoints (`localhost`, loopback IPs) a 15 min request budget, not the 120 s default. | `internal/cli/execution_stdio.go` `loopbackRequestBudget`, pinned by `TestLoopbackEndpointGetsUnattendedRequestBudget` |
| Transport deadline | The pipeline path (`internal/pipeline/pipeline.go`, `createAPIAgenticLoop`) does not set `Timeout` and so uses the 120 s default. Which path the failing run used is **unknown**. | code read |
| Timing | Last durable read 01:10:25Z to run end 01:25:26Z is 15 min 01 s. This matches the 15 min loopback budget, not the 120 s default. This is consistent with one request that waited the full budget for headers, but it is an inference, not a measurement. | task timestamps |
| Request construction | `Complete` marshals once and records the body (`remember`); the error previously carried neither elapsed wait nor effective timeout nor body size. | code read |
| Retry | The error is `server_error`, `Retryable: true`; the provider itself does not retry. Whether the caller retried is **unknown**. | `handleHTTPError` |
| Reproduction | The 01:30Z replay of the captured request against the same endpoint and model passed at `--timeout 60 --budget 1`. | task statement (supervisor) |

## Unknown (evidence absent here)

- Endpoint queue depth / inference state at 01:10–01:25Z.
- Size of the final request and the number of turns (private capture is
  supervisor-held and not in this checkout; `scripts/api-replay.py` and
  `run-requests/` are not in this repository).
- Whether the run was on the stdio (15 min) or pipeline (120 s) path, and
  whether any retry occurred inside the 15 min.
- Time-to-first-header of the failing request.

## Outcome: not reproduced

The single-request replay passing does not show recovery of the full turn, does
not show that any different timeout fixes it, and is not a root cause. A
15 min wait with a request that later answers within 60 s points away from "the
budget was too small" and towards a stalled or queued endpoint, but that is a
hypothesis. Nothing here raises a deadline, context or resource, substitutes a
model or provider, or touches the query-template / malformed-tool work (a
different failure).

## Change made (observability only)

`OpenAIProvider.handleHTTPError` now keeps the original error text and appends
`(waited <elapsed> of <effective timeout> request timeout; request body <n>
bytes)`. `Code` (`server_error`), `Retryable` (true) and the default timeout
are unchanged. Tests: `pkg/agent/openai_header_timeout_test.go`.

## Continuation basis for original task 18da

Materially different from the failed one, within existing authority, and not
claimed to fix the timeout: resume 18da as a fresh native run from the last
durable read (`internal/domain/blocked.go`), with the split/remainder
correction scoped to that file and its direct callers so the conversation
history and final request are far smaller than the third attempt's, on the
build that carries the new wait/budget/size fields. If it times out again, the
error will state the wait against the budget and body size, which separates a
stalled endpoint (wait equals budget, small body) from an oversize request.
Supervisor-side, the same-endpoint replay should be run with the captured body
size recorded against queue state at the failing time.

## Reopened on run 1685d4be (task bd2b0715, installed `f3310a1a`)

Same diagnosis, new occurrence. Every row is tagged **established**,
**inferred** or **unknown**, with its source. Cause remains **unknown**: there
is no reproducer. This section holds metadata only and no capture content.

### New facts (verbatim)

| Fact | Tag | Source |
| --- | --- | --- |
| Run `1685d4be96c6d36a48b9ad0b389eec8a` started 11:18:19Z on installed `f3310a1a` and ended 11:46:21Z after 77 inspection tools, with no product source authoring. | established | supervisor evidence, 2026-10-28 12:03Z |
| Last request body was **289,936 bytes**. | established | error text written by `handleHTTPError` (`request body 289936 bytes`) |
| It waited **15m0.006s of the 15 min** request timeout for headers, then `context deadline exceeded (Client.Timeout exceeded while awaiting headers)`. | established | same error text; budget from `loopbackRequestBudget` |
| Last tool completed **11:31:17Z**. | established | supervisor evidence |
| Private capture metadata: sentAt 11:31:16Z, capturedAt 11:46:16Z, outcome failed, textLimit 2000, truncated true. Its error equals the Console terminal error. | established | supervisor-read host metadata, not in this checkout |
| The stdio loopback path (15 min budget) is the likely caller. Duration alone does not uniquely identify the caller. | inferred | `internal/cli/execution_stdio.go`; the pipeline path would use 120 s |
| Native 166 line-coordinate reads were adopted, but useful authoring was not reached. | established | supervisor evidence; not a cause of the timeout |

### What the new facts close from the #155 Unknown list

- **Final request size:** closed for this run, 289,936 bytes (the #155 run's
  size is still unknown).
- **Full-budget wait:** closed. The wait is measured, 15m0.006s of 15 min, not
  inferred from timestamps. One request waited the whole client budget without
  receiving headers.
- **Stdio vs pipeline path:** narrowed to inferred stdio, not proven.

### What stays open

- **Unknown:** endpoint queue depth and inference state at 11:31–11:46Z.
- **Unknown:** time-to-first-header and whether the endpoint ever began
  generating (server-side first-byte time).
- **Unknown:** retry count inside the 15 min. The provider does not retry;
  callers are not shown to.
- **Unknown:** why replays differ from the failure (below).

### Capture vs terminal signature

- A capture has both `sentAt` and `capturedAt`. For this run they are
  11:31:16Z and 11:46:16Z, 15 min apart: the capture was written when the
  request failed, not at send time. (established: host metadata)
- Here the capture error and the Console terminal error are **identical**: the
  same header timeout, 15m0.006s of 15 min, 289,936 bytes. There is one
  failure, seen twice. (established)
- This differs from the historical budget captures, where the capture recorded
  a cancellation and the caller's terminal recorded the caller budget. No
  capture-versus-terminal difference exists for run 1685. (established)

### What the short replays do and do not mean

The 28 October 11:50Z same-endpoint replay at 45 s / budget 1 failed with a
**different, generic 45 s timeout**. It is not a reproduction: it shows the
endpoint did not return headers within 45 s for that replay, not that the 15
min runtime failure recurred, what the minimum is, or the cause. The earlier
b56/523 replays likewise ended in 30 s timeouts, not their captures'
cancellation signatures; the 01:30Z replay of run e35efd1e passed. Mixed
pass/timeout across short replays fits a variable endpoint but is not evidence
of one (inferred only). Replays used a truncated capture (textLimit 2000), so
none is identical to the original full request. (established)

### Fixture decision

The existing httptest fixture (`TestOpenAIHeaderTimeoutReportsWaitAndBudget`)
already witnesses the header-wait path at a small scale. One new case adds
something it did not: `TestOpenAIHeaderTimeoutLargeBodyReportsReceivedSize`
sends a body near 290 KB that the server fully reads before stalling, and
asserts the reported `request body N bytes` equals the bytes received, with
code `server_error` and `Retryable: true` unchanged. It shows the
instrumentation stays correct at this size. It does not witness endpoint
queueing or inference, and does not reproduce the failure.

### Exact missing observation

The single observation that would separate a stalled endpoint from a slow
inference or oversize prompt is the **endpoint-side time-to-first-byte (and
queue state) for the request that begins at the `sentAt` of the next
occurrence**, matched against the error text's body size. It is host-held and
outside this repository. A retained full trace is the other candidate; full
tracing was deliberately retired, so none is expected.

### Evidence route for the supervisor (next occurrence)

1. Read the terminal error text: `waited X of Y request timeout; request body N
   bytes`. X≈Y with no headers means a full-budget wait.
2. Read the capture metadata (`sentAt`, `capturedAt`, outcome) with the
   existing request-capture writer (`pkg/execution/request_trace.go`,
   `OPENEXEC_REQUEST_TRACE`), metadata only.
3. From the endpoint's own logs, read queue depth, prompt-processing start and
   first-byte time for the request starting at `sentAt`. Compare with N.
4. Record whether a replay of the same capture at the same time of day
   returns headers, with the timeout used. A pass or short timeout is not a
   reproduction.

No deadline, cap, provider, model, Stop, accounting or context change is made
or implied. A diagnosis alone does not self-heal 18da or bd2b0715.
