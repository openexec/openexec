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

## Run 7a722: structural diagnosis (revision `b5894311`)

Status: **the native loop's behaviour for this sequence is explained; the cause
of the model's behaviour is not reproduced at the configured endpoint; the
caller-visible half is unknown.** Source: the operator's metadata-only receipt
of the private trace (18 entries, 72 events, final text 6021 characters,
`droppedEntries` 0, `incomplete` false, `imagesOmitted` false,
`redactionChangedContent` true). The private trace itself was not opened by this
task, and no content, endpoint, token or tool argument is recorded here.

Structural facts (from the receipt):

- Entries 1-16: tools sent (2 definitions), `tool_choice` `auto`; every response
  was `tool_use` with 2,1,1,1,2,2,1,2,1,1,2,2,1,1,1,2 calls = 23 calls, matching
  23 `tool.proposed`/`started`/`completed` events and no response text.
- Entry 17: the synthesis request. Tools still sent (2), `tool_choice` `none`.
  The response was **another `tool_use`** (1 block, stop reason `tool_use`, 0 text
  characters). That call was not executed (23 tool events, not 24).
- Entry 18: the final-synthesis recovery request: no tools on the request or the
  wire, `tool_choice` `none`. The response was one text block of 6021 characters,
  stop reason `end_turn`, containing all seven contract section names.
- Events end `tool.completed, assistant.delta, execution.inconclusive`
  (`max_turns`); no `execution.completed`.

Diagnosis. This is the code's documented path, not a second fault:
`completeFinalSynthesis` (`pkg/execution/api_provider.go`) sends the synthesis
request with tools kept for strict endpoints; when the endpoint ignores
`tool_choice: none` and returns a tool call, it makes one tool-less retry with
`finalSynthesisRecoveryInstruction`; that retry produced the draft; the loop then
ends `inconclusive:max_turns` because every end of the tool loop does. So the
16-round budget was spent on research (the supervisor's reading of the private
trace is that the model searched for a split-task symbol with only `read_file`
and `list_directory`; that is a contributing capability gap tracked by PR #159,
not established here as a sufficient cause), the model ignored `tool_choice:
none` once, and the recovery returned a complete draft. The empty `tools` of the
9fc capture is explained for 7a722 by entry 18 being the recovery request.

Authored-surrogate reproduction (`pkg/execution/sequence_7a722_test.go`): a local
server scripts exactly this shape through the real `APIProvider` and OpenAI
adapter. The run ends `inconclusive:max_turns` with the 6021-character text, 18
recorded entries with the structure above, 23 tool events and the terminal event
order; the written trace replays offline through `traceReplayAdapter` with the
same 18 requests, outcome, final text and event types. Near miss: with a
recovery that is also a tool call, no assistant text is emitted. This is a
fixture; it is not the configured endpoint.

Observed directly by the trace: OpenExec's requests, the endpoint's responses
and OpenExec's events. **Not observable from OpenExec:** the caller-visible
result and whether the Console persisted or dropped the draft (Console 7154).
**Limits:** `redactionChangedContent` true means the trace is not byte-identical
to the wire, so an offline replay of the private trace may differ from the real
request sequence; this task did not replay the private trace.

Not reproduced: why the endpoint returned a tool call under `tool_choice: none`
at entry 17, and why the operator's last-request replay against the same endpoint timed out (120.1 s).
Missing observations and the exact commands (supervisor-run, protected,
structural output only). `traceReplayAdapter` is a **recorded-response replay**:
it serves the trace's recorded responses back and never contacts an endpoint.

1. Offline, runnable now against the private trace (no endpoint, no content
   printed; run from the repository root of the reviewed revision):

   `go test ./pkg/execution/ -run TestTraceStructureReader -v -count=1 -args -trace-structure=<path to the private 7a722 trace.json>`

   (`pkg/execution/trace_structure_test.go`.) It prints one line per entry
   (tool counts on the request and the wire, `tool_choice`, roles, response block
   types, text length, stop reason), the event counts and terminal order, the
   outcome fields, and a replay verdict: whether the loop, rebuilt from entry 1,
   re-derives all 18 recorded requests, or the first request that differs and
   the names of the differing top-level fields only. A divergence there is the
   redaction-fidelity limit or a loop change since the run, and is itself the
   observation. It answers "does OpenExec re-derive this sequence" and nothing
   about the endpoint.
2. Endpoint side (why entry 17 returned a tool call under `tool_choice: none`,
   and why the operator's last-request replay timed out at 120.1 s): no command
   exists. `scripts/api-replay.py` sends one captured request; the recorded
   requests of the trace could be sent one at a time to the original endpoint,
   but a model-driven sequence cannot be re-driven from recorded data, so a
   whole-sequence same-endpoint reproduction is not available and is not
   claimed. The timeout and the entry-17 behaviour are different observations.

No repair is claimed or authorized by this finding; a later repair must name a
reproduced cause and a failing-then-passing test.

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
