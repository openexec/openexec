# Native continuation after search evidence fills the context

Scope: `pkg/execution`, the existing API tool loop. No new controller, no
fallback model, no grant enlargement. Builds on the paged recall and
duplicate handling of #193.

## Outcome

A native task whose search rounds fill a small context continues. Complete
search results from earlier rounds are replaced in the *request* by a stub, the
full text stays retrievable through `recall_prior_tool_result`, and the same task
then advances and verifies. Anything that cannot be omitted safely stays
verbatim; if it then does not fit, the run fails before dispatch.

## Delta against the existing loop

| Addition | Why it is necessary | Failure mode it introduces (and the control) |
| --- | --- | --- |
| `ContentBlock.ScanComplete` (`json:"-"`), set by the loop on a tool-result block | Completeness must be the scan's own report, not an inference from tool name or text. Not serialized, so a model or replayed history cannot assert it. | A stale mark would permit omission. It is set only when the result was not handover-compacted (`inRequest == output`) and there was no error. `TestScanProvenanceReachesTranscriptPositionThroughWorkspaceAndComposite`. |
| Optional `scanReportingExecutor` (workspace and composite implement it) | Carries `searchFilesWithProvenance` (from #195) through composite execution to the call/result. Forwarded gateway verbs and any executor without it report "not complete". | An executor that cannot vouch: nothing is omitted and the run is refused as before. `TestSearchResultsWithoutScanProvenanceAreNeverOmitted`. |
| `priorResults.compactWith` accepts `search_files` results | Only way the search-heavy transcript fits. Eligible iff: known tool `search_files`, scan complete, no error, unambiguous call/result pairing, already answered by a later assistant turn, at least 256 bytes. Directory-listing rules are unchanged except that they now require a trusted executor explicitly. | Over-omission. One table-driven test per guard; each was verified to fail with that guard removed (`TestCompactionOmitsOnlyCompleteKnownUnambiguousSearches`, `...AmbiguousAssociation`, `TestUntrustedDirectoryListingsAreNeverOmitted`). |
| Position-bound recall id: sha256 of use number of the call ID, result call ID, tool input and output | The old content-only id cannot tell a reused call ID or an identical output at another position. The stub names the call ID and use number. | None added to the index shape (still `map[id]content`). `TestReusedIDsAndIdenticalOutputsKeepSeparatePositions`. |
| `boundedAPIAdapter.covered` and `recalledPage` | Final answers must not treat unread omitted evidence as established. Coverage grows only from pages that start inside what was already read. | Wrong coverage would hide the limitation. `TestRecallCoverageNeedsAContiguousRead`. |
| Limitation note appended to a final text answer | If any omitted result was not read to its end, the answer says so rather than silently relying on it. | Extra text on runs that compacted but never recalled. Intentional. `TestNativeTaskContinuesAfterBoundedCompactionAndRealRecall`. |

State added: one bool on a transcript block, one `map[string]int` per run.
Transitions added: none; the existing overflow → duplicate removal → compaction
→ refuse sequence in `boundedAPIAdapter.complete` is reused, and compaction is
now also attempted when the transcript holds a scan-complete result, not only
when directory listings are trusted.

## What stays fixed

- Unanswered results (after the last assistant turn), file reads (including
  `AGENTS.md`), unknown tools, failed results, partial or unmarked scans,
  system and owner instructions are never omitted.
- Final synthesis (`tool_choice: none`) is not compacted: it sees the whole
  retained transcript or is refused before dispatch, as before.
- Stop: cancellation is checked before every request, including the one that
  would follow a recall, so nothing is dispatched after it. `TestStopHaltsContinuationAndRecall`.
- Context admission, reservations, usage accounting and the Qwen model binding
  are untouched; an oversized request after compaction is refused before
  dispatch (`TestInsufficientContextAfterCompactionFailsBeforeDispatch`).

## Limits

The Solar request is not replayed. The runs here are synthetic: a scripted
native endpoint, real workspace search, real admission. A run against the real
Qwen route with real search-heavy input remains to be done after deploy.
