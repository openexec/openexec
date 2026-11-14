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
| `priorResults.compactWith` accepts `search_files` results | Only way the search-heavy transcript fits. Eligible iff: known tool `search_files`, scan complete, no error, unambiguous call/result pairing, already answered by a later assistant turn, at least 256 bytes, valid UTF-8 (recall pages are JSON strings and cannot return other bytes faithfully). Directory-listing rules are unchanged except that they now require a trusted executor explicitly. | Over-omission. One table-driven test per guard; each was verified to fail with that guard removed (`TestCompactionOmitsOnlyCompleteKnownUnambiguousSearches`, `...AmbiguousAssociation`, `TestUntrustedDirectoryListingsAreNeverOmitted`). |
| "Complete" also means: matched in no governing instruction file (`AGENTS.md`, `CLAUDE.md`, ...) and valid UTF-8 | A search result that quotes instructions must stay in the prompt. The scan, not a guess at result text, knows which file it matched in. | Over-retention: a search touching `AGENTS.md` stays verbatim and may then not fit, in which case the run is refused. `TestScanThatTouchedInstructionsOrBinaryBytesIsNeverOmittable` (fails with the `governed` marking removed). |
| A failed or unavailable recall ends the run | An invalid id or offset, or no retained result, used to come back as a tool error and the model carried on. Now the run fails (`OutcomeFailed`) before another dispatch. | A model typo ends the run; the intended fail-closed trade-off. `TestUnsafeRecallFailsTruthfullyWithoutAnotherDispatch` (fails with the guard removed). |
| Position-bound recall id: sha256 of use number of the call ID, result call ID, tool input and output | The old content-only id cannot tell a reused call ID or an identical output at another position. The stub names the call ID and use number. | None added to the index shape (still `map[id]content`). `TestReusedIDsAndIdenticalOutputsKeepSeparatePositions`. |
| `boundedAPIAdapter.covered` and `recalledPage` | Final answers must not treat unread omitted evidence as established. Coverage grows only from pages that start inside what was already read. | Wrong coverage would hide the limitation. `TestRecallCoverageNeedsAContiguousRead`. |
| Limitation note appended to a final text answer, and to a synthesis rebuilt from the handover | If any omitted result was not read to its end, the answer says so rather than silently relying on it. A rebuilt synthesis holds no recalled page, so every omission counts as unread there. | Extra text on runs that compacted but never recalled. Intentional. `TestNativeTaskContinuesAfterBoundedCompactionAndRealRecall`. |

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
  retained transcript, or is refused before dispatch
  (`TestMaxRoundSynthesisIsRefusedNotAnsweredFromStubs`), or, when the handover
  policy rebuilds it, carries the limitation note for every omission
  (`TestOmissionLimitationCountsEverythingWhenRecalledPagesAreOutOfView` covers
  the counting). The rebuilt path is not driven end to end: an active handover
  policy checkpoints before the adapter ever compacts, so the fixture cannot
  reach both.
- Stop: cancellation is checked before every request, including the one that
  would follow a recall, so nothing is dispatched after it. `TestStopHaltsContinuationAndRecall`.
- Context admission, reservations, usage accounting and the Qwen model binding
  are untouched; an oversized request after compaction is refused before
  dispatch (`TestInsufficientContextAfterCompactionFailsBeforeDispatch`).

## Guard-removal controls

`guardsOff` / `guardOn` (`context_recovery.go`) is a test-only switch: nothing
but tests writes it, so every guard is on in production. Each
`TestGuardRemoval*` test in `context_continuation_test.go` switches one guard
off inside the declared suite and asserts the unsafe outcome appears
(`ambiguity`, `scan-complete`, `utf8`, `recall-failure` for unavailable, unknown
id and invalid offset); the guarded tests assert the safe outcome. Switching
every guard off at once makes the guarded tests fail (recorded in the PR).
Insufficient-context refusal is the existing `pkg/agent` admission, covered by
its own tests there; it is not switchable from this package.

## Limits

The Solar request is not replayed. The runs here are synthetic: a scripted
native endpoint, real workspace search, real admission. A run against the real
Qwen route with real search-heavy input remains to be done after deploy.
