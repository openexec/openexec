# Native triage sequence: fixture, private trace and the Console boundary

Signature `sig-ad3b25c2a04f` (qwen-27b / openexec-api, `inconclusive:max_turns`).
Status: **diagnosis and instrumentation only. Not reproduced at the endpoint, no
repair claimed, no original recovery claimed.** A passing fixture or a written
trace is not evidence that the signature is fixed or understood.

## What the retained capture could not hold

`OPENEXEC_REQUEST_CAPTURE` (`pkg/execution/request_capture.go`) keeps the **last
request** only, each message cut to about 2000 characters, with no response, no
event stream and no caller outcome. The private capture of native run 9fc124a8
is exactly that: one request, truncated history, `tools: []`. It cannot
reconstruct the sequence, and the truncated history is **not** reconstructable
from anything retained. It is not to be filled in by guess.

## What now exists

**Fixture (`pkg/execution/request_trace_test.go`).** A local scripted HTTP server
drives the real `APIProvider` and the real OpenAI-compatible adapter through the
observed shape: 16 rounds of two tool calls (32 calls), then the tool-less
synthesis request answered with 8971 characters. It is a **fixture**. It shows
what the loop sends, receives and records. It proves nothing about how the
configured endpoint or model behaves, and it is not a same-endpoint
reproduction. Same-endpoint verdict: **unknown, not tested**.

What the fixture does establish about the loop (code-level, deterministic):

- `max_turns` is the outcome of *every* end of the tool loop, including a
  synthesis that returned a complete answer. A complete answer followed by
  `execution.inconclusive` is the code's normal ending, not a second fault.
- The synthesis request keeps the tool definitions (`ToolChoice: "none"`). The
  empty `tools` seen in the 9fc capture is therefore **not explained** by this
  loop; its cause is unknown.

**Opt-in trace (`pkg/execution/request_trace.go`, `OPENEXEC_REQUEST_TRACE`).**
Off by default. For a run that ends failed or inconclusive it writes, once, a
0600 file in a 0700 directory with: every model request (roles, system, tools,
tool_choice, tool results), the adapter's wire body when the adapter remembers
it (the OpenAI-compatible one does), the response or error of each, every event
in order, and the caller outcome. Nothing is truncated. Authentication headers
are never part of it, the configured endpoint is recorded with the capture's
redaction. Write failures are ignored, so the run is unchanged. Tool grants,
sandbox and roots are not touched.

Fidelity, stated rather than assumed:

- Secret-shaped strings are replaced (the capture's `redactSecrets`). That is a
  privacy transformation, not wire fidelity. The file records
  `redactionChangedContent`; when true the trace is **not** byte-identical to
  what was sent and any resulting difference in replay is unknown.
- Images are omitted (`imagesOmitted`).
- Past 256 MiB later entries are dropped and the file says `incomplete` with
  `droppedEntries`.
- Provider-specific response metadata round-trips as generic JSON.
- Only the tool-using path is traced. The tool-less single-request path is not.
- The consumer in the tests (`traceReplayAdapter`) reads the trace and re-runs
  the loop offline, refusing a request that differs from the recorded one. It is
  **not** `scripts/api-replay.py`, which replays one captured request; that
  script was not run on the new format and compatibility is not claimed.

## The OpenExec to Console boundary

Proven in this repository (tests above and `terminal_draft_boundary_test.go`):
OpenExec emits the synthesized text as an `assistant.delta`, then
`execution.inconclusive` with reason `max_turns`; `Result.FinalText` holds the
text; no `execution.completed` is emitted.

**Not verified here.** The Console is a separate repository and its source could
not be searched from this worktree. The earlier diagnosis
(`QWEN_MAX_TURNS_AFTER_DRAFT_DIAGNOSIS.md`) reports that the Console maps
`execution.inconclusive` to a non-terminal `turn.error` and persists a triage
contract only for a run whose status is `completed`. That is consistent with the
observed events 68 and 69 and the empty `triageContract`, but it was not
re-traced for this task, the full terminal-run-to-task persistence path and all
writers of the contract were not enumerated, and "empty by design" versus "lost"
is **unknown**. No path was found or added that turns assistant text from an
inconclusive run into a contract, and none must: an unvalidated draft is never
auto-accepted.

## Next original-recovery test

At the next materially changed original run, with the new build installed, set
`OPENEXEC_REQUEST_TRACE` once to a private path. Then:

1. Compare the recorded synthesis response (last entry) with the Console's
   8971-character-class draft. Equal means the text crossed OpenExec intact;
   different localizes the loss before the Console.
2. Compare the trace's final events with the Console `turn.error` text.
3. In the Console, check whether the contract was ever offered to the task
   update path (a Console-side test feeding `execution.inconclusive` with final
   text through the provider and server loop is the open follow-up there).
4. Only then run `scripts/api-replay.py` against a request extracted from the
   trace's `wire` body, and only the single request it supports.

## Complexity delta

One env-gated writer (`request_trace.go`, reusing `writePrivate`,
`sanitizeCapture` and `redactSecrets`), three small hook points in
`APIProvider` (a nil-checked per-run field, an event-sink wrapper, one call
around the existing `Complete`), and one test file. No new loop, state plane,
limit, model, tool or default.
