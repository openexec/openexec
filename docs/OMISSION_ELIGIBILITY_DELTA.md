# Omission eligibility: preservation classes, delta and one open conflict

Applies to `priorResults.compactWith` (`pkg/execution/context_recovery.go`) on
the PR204 tree. This pass adds tests and this record only; no product code,
model, grant, allowance, Stop, accounting or refusal behaviour changed.

Architecture complexity delta: **zero** new concepts, persistent state,
transitions, owner decisions or runtime failure modes; no existing machinery
replaced. Added: `pkg/execution/context_preservation_test.go` (198 test lines,
8 tests) and this document. Product code added: 0 lines.

## Guard inventory (what makes a result omittable)

A tool result is omitted only if all of these hold, each a structural fact:

| Guard (in `compactWith`) | Keeps verbatim | Test |
| --- | --- | --- |
| block is a `tool_result` paired to exactly one `tool_use` (`!ok`, `ambiguous`) | system/owner text, ambiguous pairs | `TestSystemAndOwnerInstructionsAreNeverStubbed`, `TestAmbiguousCallIDs…` |
| `b.ToolError == ""` | failed-tool results | `TestFailedToolResultsAreNeverStubbed` |
| tool name is exactly `list_directory` or `search_files` | file reads (incl. `AGENTS.md`), unknown tools | `TestFileReadsIncludingAgentsMdAreNeverStubbed`, `TestUnknownToolResultsAreNeverStubbed`, `TestQuotedInstructionInsideNonEligibleResultsIsPreserved` |
| `b.ScanComplete` (set only by the executing harness) | partial or unmarked scans | `TestPartialOrUnmarkedScansAreNeverStubbed` |
| `trustDirectories` (executor vouches for listings) | listings from untrusted executors | `TestPartialOrUnmarkedScansAreNeverStubbed` |

## Guard-removal receipts (each mutation compiled; `go test ./pkg/execution -run <the eight tests>`)

| Mutation | Result |
| --- | --- |
| delete `b.ToolError != ""` | RED: `TestFailedToolResultsAreNeverStubbed` only; restored → green |
| delete `b.ScanComplete &&` | RED: `TestPartialOrUnmarkedScansAreNeverStubbed`; restored → green |
| `call.name != "list_directory" && b.ScanComplete` (any tool eligible) | RED: file-read, unknown-tool and quoted-instruction tests; restored → green |
| delete `&& trustDirectories` | RED: `TestPartialOrUnmarkedScansAreNeverStubbed`; restored → green |

System/owner instructions have no removable guard of their own: compaction
only ever rewrites `ToolOutput` of paired tool-result blocks, so a text or
system block cannot be stubbed. That test is a construction check with a
positive control (it ran compaction and the companion search was stubbed);
a mutation removing the type/pairing guard did not make it fail for the right
reason, so it is **not** claimed mutation-proven.

## Open conflict: search results quoting an instruction outside the allowlist

`workspace_search.go` marks a scan non-complete when it matched in a file named
in `governingInstructionFiles` (`AGENTS.md`, `CLAUDE.md`, …). That is a name
denylist: an instruction quoted in any other file (a doc, `NOTES.md`, a code
comment) still yields `ScanComplete=true` and the result is omitted (with
byte-exact recall). The review asked for this to be repaired without any name
or content heuristic. The only structural repair is that a `search_files`
result carrying matched file text is never omitted, which removes search
omission altogether and makes the retained native vertical proof
(`TestNativeTaskContinuesAfterBoundedCompactionAndRealRecall`: multi-round
search, compaction, real paged recall) vacuous. Keeping both is not possible
without a classifier, which the task forbids guessing, so this pass keeps the
retained behaviour and does not claim the quoted-in-search case is fixed.
It is an owner decision (below).
