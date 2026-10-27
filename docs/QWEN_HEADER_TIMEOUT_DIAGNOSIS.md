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
