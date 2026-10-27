# Qwen triage runs ending `max_turns` after a complete draft: diagnosis

Signature `sig-ad3b25c2a04f`. Status: **original terminal cause UNKNOWN; not
reproduced at the endpoint; no repair claimed.** What is explained is the
*composition* that turns a complete draft into a non-saved result. Why the model
needed all 16 rounds is not. No raw request or event content is held here, and
nothing changes a limit, model, tool, deadline or the Console.

## Observed (from the task statement; private run histories are not read)

Configured `qwen-27b` / `openexec-api`, three triage runs ended `max_turns`:

| Run | Ended / window | Observed | Task `triageContract` |
| --- | --- | --- | --- |
| `e169e4ea…` | 2026-10-27T02:16:34Z | no candidate | empty |
| `575f503d…` | 03:03:44Z | complete 6835-char plan emitted | empty |
| `b93662ba…` | 03:09:08Z–03:22:33Z | 36 `tool.started` events, 16 rounds, complete 5699-char plan (final `assistant.delta`) at seq 75, `turn.error` 76, `turn.inconclusive` 77 | empty |

Replay of the captured first request (`scripts/api-replay.py`, same endpoint,
model, 120 s timeout, budget 8) PASSED for `b936` and `575f`: **not reproduced**.
One request cannot reproduce a 16-round loop.

## Actual source and limits (OpenExec 0286f602, Console 1d8aa1d8; the task says the run's source was refreshed to `5dbbee26`, which is not verified here)

- `APIProvider` counts model **rounds**, default `MaxSteps` 16
  (`pkg/execution/api_provider.go`, `NewAPIProvider`; `internal/cli/execution_stdio.go`
  leaves it unset). 36 calls in 16 rounds is exactly the limit; the number of
  calls is irrelevant to it.
- A round with **no tool call and visible text** ends `succeeded`/`completed`.
  After round 16 the loop sends one tool-less synthesis request
  (`ToolChoice: "none"`); whatever it returns, even a complete answer, the
  outcome is `inconclusive`/`max_turns` and the terminal event carries **no text**
  (`api_provider.go`, final `sink(Event{Type: EventInconclusive, Reason: ReasonMaxTurns})`).
  The draft is streamed as an `assistant.delta` before it and is in `Result.FinalText`.
- Console (`internal/providers/openexec.go` `consoleEvents`) forwards only
  deltas and tool events; `execution.inconclusive` becomes a non-terminal
  `turn.error`, and the store then ends the run `turn.inconclusive`
  (`internal/store/store.go` terminal switch).
  `finishTaskRunLocked` writes `TriageContract` **only when the run status is
  `completed`**; otherwise the task returns to `needs_triage`.

## Explanation of the observed sequence (consistent with, not proof of, the cause)

For `b936` (corrected counts: 36 `tool.started`, 5699-char final delta): 36
calls give 72 forwarded tool events (started+completed), but the draft is at seq
75, so **two forwarded events before it are unexplained** (usage or other events
are candidates; that is a guess and the history was not re-read). Then `turn.error`
76 (the inconclusive event) and `turn.inconclusive` 77. A final delta followed by a
textless terminal fits a draft from the **synthesis request after round 16**, but
the event arithmetic does not close exactly, so this is consistent with, not proof
of, that origin. The persisted result is empty because a *complete draft arrived
under a non-completed terminal*, and the Console only persists completed runs. Whether the model would have drafted
earlier with fewer reads, and why it spent 16 rounds (the note observes an
unauthorized absolute-path first call, then many directory reads), is unknown.
The earlier no-candidate run `e169` has no recorded draft; it is not explained
by this composition alone.

## Authored reproduction and smallest changed input

`pkg/execution/terminal_draft_boundary_test.go` (authored responses, real
`APIProvider`; **a mock boundary, not SAME-endpoint proof**):

- `TestCompleteDraftAfterRoundLimitStillEndsMaxTurns`: 36 calls in 16 rounds,
  then a complete draft: `inconclusive/max_turns`, draft in `FinalText`, delta
  then textless inconclusive terminal, no `completed` event.
- `TestSameDraftWithinRoundLimitSucceeds`: same 36 calls in 15 rounds, draft
  given as a normal round: `succeeded`. The smallest ordering change is the
  draft landing in round ≤16 instead of the post-limit synthesis.
- `TestFailedSynthesisIsMaxTurnsWithoutDraft` (near miss): synthesis returning a
  serialized tool call twice is also `max_turns` but has no draft.

Together with `caller_deadline_boundary_test.go` this separates rounds, caller
deadline and draft. No limit was changed, tool restriction relaxed or terminal
forced to success; the loop's truthfulness is unchanged.

## Not shown / next-occurrence capture fields

Same-endpoint causal reproduction is still required before any repair. Needed
from the next occurrence (bounded, redacted): per-round tool-call counts and
timestamps; whether the first draft text arrived in a round with tool calls
available or in the `ToolChoice: none` request; the exact forwarded event list
with types for sequences ~70-77 (the two events between the 72 tool events and the draft at seq 75 are a missing capture field); the last request capture
(`request_capture`, native 151) with message count and size; and the Console
run `status`/`error` plus task status after the run. A repair candidate to
evaluate *later*, not claimed here: whether a complete synthesis answer should
be a completed result for the Console. That is a delivery-semantics decision
touching terminal truthfulness and is out of scope.

## Complexity delta

Zero production change: one test file and this note.
