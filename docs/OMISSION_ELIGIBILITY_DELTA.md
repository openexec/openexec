# Omission eligibility: preservation classes, guard receipts and delta

Applies to `priorResults.compactWith` (`pkg/execution/context_recovery.go`),
built on the PR204 tree. Eligibility for omission is **positive and
structural**; no file name, tool-output text or completeness mark can make a
result eligible. Owner decision of 2026-11-04 (answer to
`search-omission-vs-quoted-instruction`): search results carrying file text are
never omitted. The model, grant, allowance, Stop, accounting and refusal
behaviour are unchanged.

## Architecture complexity delta

**Architecture n: zero.** New concepts: 0. New persistent state, transitions,
owner decisions or runtime failure modes: 0. Concepts removed: 3 (the
`ContentBlock.ScanComplete` mark, `scanReportingExecutor`, and the
`governingInstructionFiles` name denylist, plus the position-bound search recall
id that existed only for search). Concepts removed are not counted as new.

Lines against PR204 head `a149fceb` (`git diff --numstat a149fceb 5b7b1bcd`,
the repair commit; later main merges are excluded):

| File | Added | Removed | Net |
| --- | --- | --- | --- |
| `pkg/execution/context_recovery.go` | 4 | 15 | -11 |
| `pkg/execution/api_provider.go` | 4 | 13 | -9 |
| `pkg/execution/workspace_search.go` | 1 | 20 | -19 |
| `pkg/execution/composite_tools.go` | 0 | 17 | -17 |
| `pkg/execution/workspace_tools.go` | 0 | 20 | -20 |
| `pkg/agent/provider.go` | 0 | 5 | -5 |
| **Product code total** | **9** | **90** | **-81** |
| `pkg/execution/context_preservation_test.go` (new, 9 tests) | 224 | 0 | +224 |
| `pkg/execution/context_continuation_test.go` (rebuilt on listings) | 71 | 164 | -93 |
| **Test total** | **295** | **164** | **+131** |
| Docs (`NATIVE_SEARCH_CONTINUATION.md` +25/-27, this file new) | | | |

`workspace_search.go`, `composite_tools.go`, `workspace_tools.go` and
`agent/provider.go` carry only the PR204 scan additions removed.

## Guard inventory (a result is omitted only if every one holds)

| Guard in `compactWith` | Keeps verbatim | Test |
| --- | --- | --- |
| block is a `tool_result` paired to exactly one `tool_use` (`eligible` map; `!ok`) | system and owner text, anything not a paired result | `TestSystemAndOwnerInstructionsAreNeverStubbed` |
| no ambiguous call ID (`ambiguous`) | ambiguous pairs | `TestCompactionDoesNotOmitAmbiguousAssociation` |
| `b.ToolError == ""` | failed-tool results | `TestFailedToolResultsAreNeverStubbed` |
| tool name is exactly `list_directory` | search results, file reads incl. `AGENTS.md`, unknown tools, quoted instructions | `TestFileReadsIncludingAgentsMdAreNeverStubbed`, `TestUnknownToolResultsAreNeverStubbed`, `TestSearchResultsCarryingFileTextAreNeverStubbed`, `TestQuotedInstructionInsideNonEligibleResultsIsPreserved` |
| `trustDirectories` (executor vouches for listings) | listings from untrusted executors; unmarked/partial output | `TestPartialOrUnmarkedScansAreNeverStubbed`, `TestUntrustedDirectoryListingsAreNeverOmitted` |
| at least 256 bytes and valid UTF-8 | small and non-round-trippable output | `TestCompactionOmitsOnlyKnownUnambiguousDirectoryListings` |
| only the older listings (the newest is kept) | the newest listing | `TestReusedIDsAndIdenticalOutputsRecallTheirOwnContent` |

`assertPreserved` appends a newer trusted listing to every request under test, so
a result a broken guard wrongly treats as eligible would be stubbed instead of
passing because nothing newer existed.

## Guard-removal receipts

Each mutation compiled (`go test ./pkg/execution/`), went red, and was restored
to green (`ok pkg/execution`).

| Mutation (guard removed) | Red tests |
| --- | --- |
| `call.name == "list_directory"` replaced by always-true (any tool eligible) | file reads/`AGENTS.md`, unknown tools, search-with-file-text, quoted-instruction, partial/unmarked, `TestCompactionOmitsOnlyKnownUnambiguousDirectoryListings`, plus the older recovery/discipline tests |
| `b.ToolError != ""` deleted | `TestFailedToolResultsAreNeverStubbed`, `TestCompactionOmitsOnlyKnownUnambiguousDirectoryListings` |
| `&& trustDirectories` deleted | `TestUntrustedDirectoryListingsAreNeverOmitted`, `TestListingsFromAnExecutorThatCannotVouchAreNeverOmitted`, `TestPartialOrUnmarkedScansAreNeverStubbed` |
| tool-result type and pairing guard replaced by always-eligible (so system/owner text blocks are rewritten) | `TestSystemAndOwnerInstructionsAreNeverStubbed` and 16 others |
| `!ambiguous[...]` replaced by always-true | `TestCompactionDoesNotOmitAmbiguousAssociation` |

An earlier attempt that did not compile (an unused variable) was discarded, not counted.

## Verification

`go test ./pkg/execution/ ./pkg/agent/` ok; `go vet ./...` printed nothing;
`make compat-test` passed. The real native vertical test
`TestNativeTaskContinuesAfterBoundedCompactionAndRealRecall` (multi-round listings,
bounded compaction, real multi-page position-checked `recall_prior_tool_result`,
answer written and read back) passes; Stop, insufficient-context, unsafe-recall and
ambiguous-ID tests pass on the same path.

## What is not retained from PR204

Search-result omission and its byte-exact paged recall are gone by owner decision;
the vertical proof now runs on directory listings. The paged recall, reused
call IDs, Stop and refusal proofs are retained. A search-heavy task that
overflows is refused before dispatch rather than compacted.
