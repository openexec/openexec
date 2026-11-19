# Omission eligibility: preservation tests, guard receipts and delta

Applies to `priorResults.compactWith` (`pkg/execution/context_recovery.go`) as
it stands on main `f085f0cd` (#214). This change tracks main's structural
eligibility rule and adds tests only; **no product code differs from main**
(`git diff f085f0cd -- pkg` shows only the new test file).

The rule (unchanged): a result is omitted only by structure, never by file
name, tool-output text or completeness text:

- a `list_directory` result from a trusted executor, older than the newest
  listing; or
- a `search_files` result whose own scan set `ScanComplete` and which a later
  assistant turn has answered (the stub says it was NOT inspected; the content
  is recallable byte for byte).

Both need an unambiguous call/result pair, no tool error, at least 256 bytes
and valid UTF-8. System and owner instructions, file reads (including
`AGENTS.md`), unknown tools, failed results, and partial, unmarked or
unanswered scans stay verbatim. The model, grant, allowance, Stop, accounting
and refusal behaviour are unchanged. The owner's 2026-11-04 answer to
`search-omission-vs-quoted-instruction` ("never omit search results with file
text") applied to the original combined scope; the 2026-11-14 renewal limited
this task to the never-stubbed classes and left search omission to follow-up
82b543a0, which landed it on main. This change does not reverse it.

## Architecture complexity delta

**Architecture n: zero.** New concepts: 0. New persistent state, transitions,
owner decisions or runtime failure modes: 0. Product code lines: +0/-0.

Lines against main `f085f0cd` (`git diff --numstat f085f0cd`):

| File | Added | Removed |
| --- | --- | --- |
| `pkg/execution/context_preservation_test.go` (new, 8 tests) | 261 | 0 |
| `docs/OMISSION_ELIGIBILITY_DELTA.md` (this file, new) | see file | 0 |
| Product code | 0 | 0 |

## Guard inventory (a result is omitted only if every one holds)

| Guard in `compactWith` | Keeps verbatim | Test |
| --- | --- | --- |
| block is a `tool_result` paired to exactly one `tool_use` (`eligible`; `!ok`) and at least 256 bytes | system and owner text, anything not a paired result | `TestSystemAndOwnerInstructionsAreNeverStubbed` |
| no ambiguous call ID (`ambiguous`) | ambiguous pairs | `TestCompactionDoesNotOmitAmbiguousAssociation` (existing) |
| `b.ToolError == ""` | failed-tool results | `TestFailedToolResultsAreNeverStubbed` |
| tool name is exactly `list_directory` or `search_files` | file reads incl. `AGENTS.md`, unknown tools, quoted instructions | `TestFileReadsIncludingAgentsMdAreNeverStubbed`, `TestUnknownToolResultsAreNeverStubbed`, `TestQuotedInstructionInsideNonEligibleResultsIsPreserved` |
| `trustDirectories` for listings | listings from untrusted executors | `TestPartialOrUnmarkedScansAreNeverStubbed` |
| search: `b.ScanComplete` | partial and unmarked scans | `TestPartialOrUnmarkedScansAreNeverStubbed`, `TestUnmarkedUnansweredOrFailedSearchResultsAreNeverStubbed` |
| search: `i < lastAssistant` (answered by a later turn) | unanswered scans | `TestUnmarkedUnansweredOrFailedSearchResultsAreNeverStubbed` |
| only older listings (newest kept) | the newest listing | existing recovery tests |

`assertPreserved` appends a newer trusted listing to every request under test,
so a result a broken guard wrongly treats as eligible is stubbed instead of
passing because nothing newer existed. Two positive controls
(`TestPositiveControlOlderDirectoryListingIsStubbed`, and the marked answered
search inside `TestUnmarkedUnansweredOrFailedSearchResultsAreNeverStubbed`)
show compaction runs.

## Guard-removal receipts

Each mutation compiled, was run with
`go test ./pkg/execution/ -run 'Preserved|NeverStubbed|Quoted|Positive|Partial'`
(the system/owner one with `-run TestSystemAndOwner`), failed, and was restored
(`git diff f085f0cd -- pkg/execution/context_recovery.go` is empty).

| Mutation (guard removed) | Red tests |
| --- | --- |
| `b.ScanComplete &&` deleted from the search case | `TestPartialOrUnmarkedScansAreNeverStubbed`, `TestUnmarkedUnansweredOrFailedSearchResultsAreNeverStubbed` |
| `i < lastAssistant` deleted (with `_ = lastAssistant` so it compiles; a first attempt without it did not compile and was discarded) | `TestUnmarkedUnansweredOrFailedSearchResultsAreNeverStubbed` |
| `b.ToolError != ""` deleted | `TestFailedToolResultsAreNeverStubbed`, `TestUnmarkedUnansweredOrFailedSearchResultsAreNeverStubbed` |
| `call.name == "list_directory" &&` deleted (any tool eligible) | file reads/`AGENTS.md`, unknown tools, partial/unmarked, quoted-instruction, search tests, and main's `TestOmissionEligibilityIgnoresQuotedInstructionsAndFilenames` |
| `&& trustDirectories` deleted | `TestPartialOrUnmarkedScansAreNeverStubbed` |
| type, pairing and 256-byte guards replaced by an always-eligible fallback (text blocks are rewritten) | `TestSystemAndOwnerInstructionsAreNeverStubbed` |

## Verification

`go test -count=1 ./pkg/execution/` passes except
`TestD26PrivateOccurrenceEvidence` ("required private artifact unavailable or
malformed"), the private-artifact test already on the owner's Needs-you card,
which does not touch this change. The existing #193 tests, main's #214 tests
and the native continuation/recall test are unchanged and pass. Bare
`go vet ./...` and `make compat-test` were not re-run in this pass (they are
not declared checks here); the earlier receipt covers an older head and is not
claimed for this one.
