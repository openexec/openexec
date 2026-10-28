# Saved Work integration: finite Qwen query-template diagnosis

Run `c691396fd8ff02f43a71f9bbb58c538d`, diagnosis task
`8d7c348e7db0af94f7944953f7298651`, 2026-10-28.

**The complete original failure is NOT REPRODUCED. Existing retained records
cannot reconstruct a lossless request. This deliverable is documentation only,
not a causal repair or authorization to retry the original Work integration.**
The operator replay failed differently. Later #171 provides separate synthetic
same-endpoint context-boundary evidence and an already-merged transport change;
neither recovers the missing original content.

## Evidence provenance and distinct signatures

The following host-private artifacts were read structurally, without copying
prompt bodies, tool outputs, reasoning or credentials into this repository.
Paths identify evidence for an authorized examiner; they are not public assets.

| Artifact | Private location | SHA-256 |
| --- | --- | --- |
| Last-request capture | `/home/perttu/.agent-console/run-requests/c691396fd8ff02f43a71f9bbb58c538d.json` | `d629ed2fe7183c78574c0a35d2b20aaac804db3ffafb1a3638a00ebc8708d5e9` |
| Run record | `/home/perttu/.agent-console/perttu/scratch/20261028T153229Z/workrun.json` | `5a27f1046e856ca64a3ae2e81f7f4a0061c8d0a77d3458b97100c08e31f00cf0` |
| Retained `/api/runs/c691396fd8ff02f43a71f9bbb58c538d/events` | Same scratch directory, `workevents.json` | `96c990b25cf779cb523bc1a177314dc3f4c5f5d95c8a48d2acf20633d6ba5533` |
| Operator replay summary, containing a private request | Same scratch directory, `work-replay.json` | `6d94dd3eb601456a20ffb1dab333fcdbca9cbe4d64f2fc22e06742033f21a4a9` |

The capture records `provider=qwen-27b`, model `qwen38-27b-mtp2-128k`, base URL
`http://127.0.0.1:11436/v1`, `outcome=failed`, empty reason, `sentAt=15:35:16Z`
and `capturedAt=15:35:18Z`. These client timestamps are not server receipt
times. The error contains HTTP code **500** and **`Jinja Exception: No user
query found in messages.`** The displayed `server_error (Ǵ)` is the historical
status-as-rune representation; the JSON code is 500.

Events 102 (`turn.error`, 15:35:19.775166898Z) and 103 (`turn.failed`,
15:35:23.876181226Z) retain that same error. The run record ends at the latter
timestamp. There are 103 events, including **46 tool starts and 46 completions**:
32 `run_command`, seven `read_file`, six `inspect_repository`, one
`record_route_judgment`. The original description's 49 was an estimate,
superseded by the supervisor's 16:08Z correction and these counted records.
Conflict output is retained at events 37 and 79; no resolved authored candidate
or gate result was produced (supervisor correction). Tool activity and merge
content do not prove useful completion.

Event 100 supplies argv `git show :1:internal/server/queue.go >
/tmp/base-queue.go`; event 101 reports exit 128 and `fatal: ambiguous argument
'>': unknown revision or path not in the working tree.` Git received literal
argv, so no shell redirection occurred. The subsequent template error is a
separate HTTP response. Temporal adjacency establishes no causal connection.

| Signature | Retained observation / source | Discrimination |
| --- | --- | --- |
| Original local query-template failure | Capture and events 102–103 above: HTTP 500, Jinja missing-query message | An endpoint response, not a caller-budget ending or header timeout |
| Operator replay | `work-replay.json`: target `http-500` / `jinja exception: no user query found in messages`; first result `http-400` / `invalid tool call arguments`; `reproduced=false`, `partial=false`, `requests=1`, empty `attempts` and `passes` | Different status and message; no minimization or passing change. `minimalRequest` is merely a report field, not a demonstrated minimum |
| Caller-budget endings | [Native inspection-only diagnosis](NATIVE_INSPECTION_ONLY_BUDGET_DIAGNOSIS.md) and `pkg/execution/caller_deadline_boundary_test.go` | Historical Console terminal `inconclusive:budget_exhausted`, sometimes paired with capture cancellation/termination; absent from this run's terminal signature |
| Hosted LUMI #169 | [Configured LUMI diagnosis](LUMI_HEADER_TIMEOUT_DIAGNOSIS.md) and `pkg/agent/openai_provider.go` transport/status paths | Hosted requests waited for headers under the 120s client timeout; not this loopback endpoint's received HTTP 500 |

The approved record says the operator used `--timeout 45 --budget 8` with
the same endpoint/model. The summary itself records only one request, rounded
`seconds=0.0`, and no executable or effective-timeout attestation. It is not an
eight-request search. No replay or provider call was made by this diagnosis.

## Exact fidelity boundary

`pkg/agent/openai_provider.go` marshals and remembers the serialized request
before dispatch. `pkg/execution/request_capture.go` then parses that body and
writes a sanitized JSON tree, privately (0600), for a failed/inconclusive run.
It preserves array order, roles, field presence, tool definitions and parameter
shape; it does not preserve original wire bytes. Every string can be redacted.
Under `messages`, strings including content, reasoning and tool arguments are
bounded; images/audio are omitted. The initial limit is 2,000 runes per string,
keeping 1,500 head and 500 tail runes plus an omission marker; the writer can
shrink it to meet its 512 KiB file bound. Redaction is not reversible and the
capture has no separate redaction-change flag.

This capture explicitly has **`truncated=true`, `textLimit=2000`**:

| Retained field | Observed fidelity |
| --- | --- |
| Request keys | `messages`, `model`, `tool_choice`, `tools`; `tool_choice=auto` |
| Message order / roles | 64 messages: one system, one user, 16 assistant, 46 tool. The sole user role is present; this does not prove what the server retained after its own processing |
| Content | 19 fields have cut markers: system, user, and 17 tool results. Markers total 299,465 omitted characters. This is not a measurement of original wire bytes or tokens |
| Tool calls/results | 46 call IDs correspond to 46 result IDs. Order and linkage survive; complete content does not |
| Tool arguments | Exactly one historical argument string fails JSON decoding: zero-based message 33, `record_route_judgment`, marker `[662 characters cut]`. Other argument strings decode; that alone does not attest their original bytes |
| Assistant content presence | Nine assistant messages omit `content`; capture sanitization preserves this field absence. Failure-time serialization allowed it; current #170 does not |
| Tool definitions | 22 definitions retained; no inference about earlier requests or effective permissions follows |

The malformed historical argument is **consistent with** the replay's HTTP
400. No controlled replay replacing only that argument was performed, so this
is not a proved explanation even of the 400, much less the original 500.
The original adapter serialized tool input as a JSON argument string; capture
truncation can break that inner JSON without breaking the outer capture JSON.
It does not show that the original on-wire argument was invalid.

The Console event stream is independently lossy. In the authorized sibling
`agent-console/internal/providers/openexec.go`, `consoleEvents` translates
native starts/completions through `redactInput` / `redactOutput` in
`internal/providers/redact.go`. Input strings retain at most 500 bytes;
file-content fields are dropped and secret-shaped values scrubbed. Outputs
retain 2,000-byte tails, while file-reading results are not stored. Here the
judgment at event 56 has a rationale retaining 500 characters plus a truncation
marker. Eleven completed outputs carry truncation markers and seven are
`[content not stored]`. Thus the event cannot restore the missing argument,
large diff output, or read-file results. Re-reading current files, rerunning
Git, or guessing omitted rationale would create different history.

No matching `c691396fd8ff02f43a71f9bbb58c538d.trace.json` exists beside the
capture. `pkg/execution/request_trace.go` can retain per-round request/wire
trees, but still redacts secrets and omits images, and exposes
`redactionChangedContent`, `imagesOmitted`, `incomplete`, and `droppedEntries`.
It is not automatically byte-identical even when present. It cannot recover
this missing past record; tracing was not enabled here.

The historical Console replay script snapshot at
`/tmp/claude-1000/perttu-work-integration-153229/scripts/api-replay.py` uses
Python JSON reserialization and `urllib` against `<baseURL>/chat/completions`.
Its `replay` stops before minimization when the first signature differs.
That explains the summary's structure, not the original endpoint failure.
The summary does not attest that script snapshot's exact revision. The script
is absent from this OpenExec checkout; no replacement replay mechanism is added.

**Conclusion: the available capture plus events cannot yield a lossless
same-endpoint replay.** An initial request/envelope alone would also lack
the omitted subsequent model responses and tool results. No full original
request, full-wire replay, minimal causal change or fail-before/pass-after
original repair is established.

## Identity and existing mechanisms, including later #171

The failure-time installed `6921931b` lineage and prior #150/#153 installation
are supervisor-reported provenance, not executable identity attested by the
capture. Initial triage inspected `42eb7cfa` after #170. This candidate starts
at supplied `origin/main` **`5fe138d17a9bd9890bbf2cf3a8628922ef76012f`**. The
17:16Z supervisor addendum reports that revision installed by the fresh
17:18Z check. Private `20261028T170418Z/native-installed-ancestry.json` agrees
with local history: #169 `60086317`, #170 `42eb7cfa`, #171 `5fe138d1` follow
`6921931b`. None of these later facts attests the failed process's executable.

`pkg/execution/api_provider.go` replays prior user/assistant text in order,
appends the current user prompt, and appends each assistant response and each
tool result. `completeChecked` checks plain-query presence and matched,
answered tool calls before transport. It cannot prove that an endpoint will
retain the query. The compatibility serializer preserves tool-call IDs,
names and argument strings; results retain their tool role and call ID,
errors become content, and empty output gets `(no output)`.

#170 changed compatibility `openAIMessage.Content` from `omitempty` to a
required string, including tool-only assistant turns. Its
`TestAPIProviderRequiredContentAfterToolCall` exercises a different endpoint
requirement. This does not prove the nine absent fields caused this incident.

Prior #153 (`6d99bb02`, task evidence **cbfb04**) bounded native workspace
file pages to 8,192 bytes. `TestLargeFileReadsKeepUserTurnWithinFrontTruncatingEndpoint`
drives the real executor/serializer against a simulated front-truncating
endpoint, including refused oversized page input. It proves that simulated
mechanism, not this run, whose Console history also contains large Git output.
It does not bound every Console `run_command` result.

The private #171 receipt `20261028T170418Z/native171.json` names
candidate head `f07cd3e8a61ac312d9a242ad3cff853831ebe2b0`, merge
`5fe138d17a9bd9890bbf2cf3a8628922ef76012f`, and reports a synthetic valid-user
plus tool-history comparison at the same endpoint: compatibility HTTP 500,
native `truncate:false` HTTP 400 refusing 612,369 tokens against unchanged
131,072 context. Its body and existing `NOTES.md` report 130,275 tokens in
the original run's preceding request. These are prior reported measurements,
not new measurements or a full-original replay by this pass. The receipt is a
PR-body snapshot, not raw request/response or exact-head CI/review evidence.

The actual #171 source routes ordinary negotiated local text `Complete`
requests through existing `completeLocalOllama` with zero admission caps.
It sends native `/api/chat`, `truncate:false`, preserves defaults/explicit
options and usage, and returns typed context overflow for the recognized
native HTTP 400 context refusal. Negotiation pins loopback `/v1` and native
version `0.32.13`; media, forced choice and unnegotiated calls retain the
compatibility route. `Stream` remains separate. Tests in
`pkg/agent/ollama_unbounded_test.go` exercise these real client routes against
HTTP fixtures, including fits/overflow, fallback near misses, stop reasons,
private-error exclusion, transport errors and caller cancellation.

This distinguishes independent synthetic context-boundary evidence from both
the historical malformed capture replay and useful original Work recovery.
No generic runtime patch, cap/model/deadline change, tracing, nested provider,
credential access or equivalent original-work retry is part of this diagnosis.

## Finite handoff and reproduction conditions

The native implementation loop owns request/history tooling; deterministic
delivery owns Console integration. Reuse existing serialization, capture/replay
and tests. Complexity delta **0**: no controller, persistent state, transition,
authority, owner decision or implementation loop is added.

A future causal claim needs privately retained complete original message
content, reasoning where serialized, tool arguments/results and definitions,
order/IDs, request parameters, exact serialization/transport revision and
effective endpoint/model/config provenance. Establish whether redaction or
omission changed it. Keep credentials private and use the existing authorized
configured route; do not invent missing bytes from current files. Record
actual request size, route, model identity, timing and response signature.
Same endpoint means the original configured endpoint/model, not a substitute
fixture or a changed native route passed off as the original compatibility
request. A changed backend/template revision also limits equivalence.

Only after that equivalent failure reproduces should minimization and a
single causal change be tested failing-before/passing-after. A different
error, a later pass or unavailable full history is nonreproduction and a
stopping point. A future repair must preserve history/privacy/refusal/terminal
coverage, receive independent exact-head review, and pass the configured
Linux/macOS hosted checks. These conditions authorize no new provider calls
or tracing in this task.

Return this finite result to original **`7739868857240931a8c4356ec6416a58`**
and prior **cbfb04 / #153** evidence. The same Work PR
[agent-console #1401](https://github.com/openexec/agent-console/pull/1401)
retains the original integration, gates, delivery, full saved-content,
300 ms, Retry, browser and reload obligations under the same Stop and
accounting. No reset, acceptance or unchanged-original retry follows from
this diagnosis. The supervisor's separate bounded integration from its own
scratch checkout proceeds independently; this document claims no result for
that work.

After deploy: **no deployment is required for this diagnosis**. Any later
live recurrence measurement or Work browser validation is separate authorized
work, not a premerge acceptance requirement of this metadata-only task.

## Verification boundary

Only this diagnosis document is added. Existing code/tests provide mechanism
evidence; no speculative regression is introduced without causal reproduction.
The following existing tests were run directly with `go test`, `-run` selecting
the named tests, `-count=1 -v`. Both commands exited **0**, printing **`PASS`**:

| Package / selected tests | Observed boundary |
| --- | --- |
| `pkg/execution`: `TestReplayReachesBothExecutePaths`, `TestReplayPreservesOrder`, `TestReplayValidationRefusesWhatMustNotBeReplayed` | Real API history in tool and streaming paths, ordered replay, invalid-role/content refusals |
| `pkg/execution`: `TestAPIProviderCapturesTheFailedRequest`, `TestAPIProviderCapturesTheLastRequestAtTheCap`, `TestRequestTraceControls`, `TestRequestTraceSizeBoundIsReportedNotSilent`, `TestRequestTraceKeepsTheRefusedRequest` | Private bounded capture, secret exclusion, absent/success/refused-request controls, trace fidelity/size/refusal behavior |
| `pkg/execution`: `TestMultiToolReplayKeepsUserQueryAndOrderingOnTheWire`, `TestTransportGuardRefusesBeforeAnyRequestWhenNoPlainQuery`, `TestValidateTransportConversation`, `TestEndpointIgnoringDisabledToolsEndsInconclusiveWithoutExtraTools`, `TestNormalToolRunStillSucceeds` | Real wire order/linkage, queryless refusal and near misses, terminal refusal/success branches |
| `pkg/execution`: `TestLargeFileReadsKeepUserTurnWithinFrontTruncatingEndpoint`, `TestReadFilePagesBoundHistoryAndRoundTripSource`, `TestAPIProviderRequiredContentAfterToolCall` | #153 simulated front truncation and actual paged-file round trip; #170 required content, empty/prose assistant cases |
| `pkg/execution`: `TestCallerDeadlineDuringToolEndsCancelledWithoutSynthesis`, `TestRoundLimitCountsRoundsNotToolCallsAndIsNotBudgetExhausted` | Caller cancellation and tool-round terminal distinctions |
| `pkg/agent`: `TestUnboundedLocalOllamaCompletePreservesContextAndDefaults`, `TestUnboundedLocalOllamaCompletePreservesExplicitOptionsAndFallbacks`, `TestUnboundedLocalOllamaCompleteStopReasons`, `TestUnboundedLocalOllamaCompleteErrorContract`, `TestLocalOllamaCompletePreservesCallerCancellation` | #171 real client against fixtures: native fits/overflow, explicit options and compatibility fallback near misses, stop/error/privacy/cancellation contracts |

Terminal package output: `ok github.com/openexec/openexec/pkg/execution 5.307s`
and `ok github.com/openexec/openexec/pkg/agent 0.530s`. These are fixture
mechanism checks, not original-incident reproduction or live inference.
They are part of the Go suite declared in CI; no standalone verifier is needed.
No full local gate or live provider request was run. Independent exact-candidate
review and hosted CI are required before promotion; their results are not
asserted here.

Sibling-path audit covered tool/stream history initialization, compatibility
Complete/Stream serialization, negotiated bounded/ordinary native completion,
media/forced-choice/unnegotiated fallback, local workspace file paging versus
Console command output, and bounded capture versus opt-in trace/event storage.
No guard, matcher, filter or rewrite is added on any path. Legacy project
loading, migration and task persistence are untouched.

`.github/workflows/ci.yml` declares `build (ubuntu-latest, 1.25)` and
`build (macos-latest, 1.25)`, with Node 22 UI build/tests, embedded Go build,
Go suite, workspace-search guard removals and compatibility regression. Lint
is advisory (`continue-on-error: true`). A source declaration or a prior PR's
green result is not a passing receipt for this candidate. The publication
lane owns these exact-head checks and independent review.
