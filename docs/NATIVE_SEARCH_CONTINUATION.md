# Native continuation after evidence fills the context

Scope: `pkg/execution`, the existing API tool loop. No new controller, no
fallback model, no grant enlargement. Builds on the paged recall and
duplicate handling of #193.

(The file name is historical: an earlier revision also omitted complete
`search_files` results. The owner decided on 2026-11-04 that search results
carrying file text are never omitted, because file text can quote a governing
instruction in any file and no name or content rule can prove otherwise. That
branch, its scan-completeness mark and its file-name denylist were removed.)

## Outcome

A native task whose directory-listing rounds fill a small context continues.
Older directory listings from a trusted executor (file names alone, no file
text) are replaced in the *request* by a stub, the full listing stays
retrievable through `recall_prior_tool_result`, and the same task then advances
and verifies. Anything else stays verbatim; if it then does not fit, the run
fails before dispatch.

## Delta against the existing loop

| Addition | Why it is necessary | Failure mode it introduces (and the control) |
| --- | --- | --- |
| A failed or unavailable recall ends the run | An invalid id or offset, or no retained result, used to come back as a tool error and the model carried on. Now the run fails (`OutcomeFailed`) before another dispatch. | A model typo ends the run; the intended fail-closed trade-off. `TestUnsafeRecallFailsTruthfullyWithoutAnotherDispatch`. |
| `boundedAPIAdapter.covered` and `recalledPage` | Final answers must not treat unread omitted evidence as established. Coverage grows only from pages that start inside what was already read. | Wrong coverage would hide the limitation. `TestRecallCoverageNeedsAContiguousRead`. |
| Limitation note appended to a final text answer, and to a synthesis rebuilt from the handover | If any omitted result was not read to its end, the answer says so. A rebuilt synthesis holds no recalled page, so every omission counts as unread there. | Extra text on runs that compacted but never recalled. `TestNativeTaskContinuesAfterBoundedCompactionAndRealRecall`. |
| `priorResults.compactWith` requires a trusted executor for directory listings explicitly, an unambiguous call/result pairing, no error, at least 256 bytes and valid UTF-8 (recall pages are JSON strings) | Eligibility is structural only. | One test per guard (`TestCompactionOmitsOnlyKnownUnambiguousDirectoryListings`, `...AmbiguousAssociation`, `TestUntrustedDirectoryListingsAreNeverOmitted`, `TestListingsFromAnExecutorThatCannotVouchAreNeverOmitted`). |

State added: one `map[string]int` of coverage per run. Transitions added: none;
the existing overflow → duplicate removal → compaction → refuse sequence in
`boundedAPIAdapter.complete` is reused.

## What stays fixed

- Search results, file reads (including `AGENTS.md`), unknown tools, failed
  results, and system and owner instructions are never omitted.
- Final synthesis (`tool_choice: none`) is not compacted: it sees the whole
  retained transcript, or is refused before dispatch
  (`TestMaxRoundSynthesisIsRefusedNotAnsweredFromStubs`), or, when the handover
  policy rebuilds it, carries the limitation note for every omission
  (`TestOmissionLimitationCountsEverythingWhenRecalledPagesAreOutOfView`).
- Stop: cancellation is checked before every request, including the one that
  would follow a recall, so nothing is dispatched after it. `TestStopHaltsContinuationAndRecall`.
- Context admission, reservations, usage accounting and the Qwen model binding
  are untouched; an oversized request after compaction is refused before
  dispatch (`TestInsufficientContextAfterCompactionFailsBeforeDispatch`).

## Limits

The Solar request is not replayed. The runs here are synthetic: a scripted
native endpoint, real workspace directory listings, real admission. A
search-heavy task no longer compacts at all; if it overflows it is refused
before dispatch. A run against the real Qwen route remains to be done.
