# Native capture and replay request shape diagnosis

Signature `sig-5f0828d325c3`, diagnosis task `09678b52c000d34bec5b1772ea6699f6`.
**The original caller-budget ending is NOT REPRODUCED. No product repair or
recovery is claimed.** Current native serialization can send object arguments
to `/api/chat` while its last-request metadata reports only the configured
compatibility base URL. Posting that body to `/chat/completions` can therefore
produce a distinct parser error. The public controls prove this mechanism
locally; they do not establish the historical run's dispatch route or cause.

## Evidence and identity

The implementation inspected here is supplied `origin/main`
`73b86949926dea47e5e4537e0d25387cff5fd867`, initially clean. The approved triage's
`0b4aebd1f8ecfa2450762b5f872e235dc7e5bccf` is an earlier source identity, not
this candidate's base. Production source hashes at this base:

| Source | SHA256 |
| --- | --- |
| `pkg/agent/openai_provider.go` | `abf4449efd35349e8b62d7b83f8c137ad5f2ff9c02932af4de68820b520deda3` |
| `pkg/agent/ollama_budget.go` | `8f53692ee9a8d22c3dea4da93def689746b8516f3694ae80f48eec9a90391697` |
| `pkg/execution/request_capture.go` | `baeab24f669ae69ccfc66bf8c938f59b4c0d6787d42e71f062d67a559ec54b6a` |
| `pkg/execution/request_trace.go` | `0fedc0b24ab1e5a48dcac81c1fb3772a74263311d73d1604faef0eb3d16dc582` |

Current installed executable identity was not attested. The typed host attempt
`openexec version` returned **`Refused: outside this unattended task's authority
— openexec is not in the task's tool chain`**. It was not retried through a
wrapper. The supervisor reports a later installed revision `5fe138d1…` and hash
prefix `cf15fe57`; these are supplied, incomplete identities and do not bind
the first historical process to this source. Checked-in platform binaries are
not evidence of that process's executable either.

The following private facts are **supervisor-supplied structural evidence**,
not newly inspected captures. Private captures and scratch records are outside
the listed filesystem grants; no available typed tool reads those private
files. They were not opened through another route.

| Evidence | What is supplied | Limit |
| --- | --- | --- |
| First capture, associated by supervisor with `c6e40206…` | SHA256 `c2e5f55762d0631a030b2c41a75b92d2df68e7a62c93e31e5788cb766afeb841`; 141 messages; 85 tool argument objects (`dict`), no reported string arguments | Hash is supplied, not independently recomputed; message contents and original wire bytes unavailable |
| First capture envelope | Keys `baseURL,capturedAt,error,outcome,provider,reason,request,sentAt,textLimit,truncated,version`; error `context canceled`; status summary `null`; `hasTrace=false` | Outcome/reason values, actual URL, truncation flag value, text limit value and times are not in this summary. `null` is not HTTP success; the key list has no status field |
| First request | Keys `messages,model,options,stream,tools,truncate` | Matches the current native serializer's field set; does not prove a historical URL, executable, option values or protocol negotiation |
| Original terminal | Supervisor reports the actual Console runs ended `inconclusive:budget_exhausted` after the unchanged two-hour caller limit | Capture alone contains neither the caller's terminal verdict nor its cancellation cause |
| Existing same-endpoint private replay at 23:02Z | One request, timeout 120 seconds, budget 1; immediate HTTP 400: `json cannot unmarshal object into go struct field .messages.tool_calls.function.arguments of type string` | A **private** replay, not the required public synthetic endpoint control; different from both cancellation and caller-budget ending |
| Corroboration, second admission `906087…` | Supervisor reports 23:27:49Z caller-budget ending, 99-message capture, `truncated=true`, `textLimit=2000`, 59 tool messages; private replay again gives object/string HTTP 400 | This capture's truncation values must not be assigned to the first capture |
| Further original witnesses | Supervisor reports the same distinct parser refusal on `85bd…`, `673c…`, `c80…` and `b862…` captures | Different attempts include inspection-only work, authored seeds, and frontend commands. They do not share a proved productive-execution cause |

`hasTrace=false` reports no trace in the supplied evidence; missing records do
not prove tracing was never enabled or that no other record exists. No raw
same-run wire/trace, full capture, exact historical executable or exact replayer
revision was supplied to this pass.

## Serialization and capture boundaries

| Current source and real path | Observed behavior and executable control |
| --- | --- |
| [Compatible Complete and Stream](../pkg/agent/openai_provider.go), `buildOpenAIRequest`, `convertToOpenAIMessage`, `openAIFunctionCall` | `Arguments` is a string made from `ToolInput`; both methods marshal, remember the bytes and POST to configured base plus `/chat/completions`. `TestCaptureFidelitySerializerDispatchAndReplay/compatible` and `/negotiated-stream` check received bytes and argument types |
| [Native completion](../pkg/agent/ollama_budget.go), `EnableLocalOllamaBounds`, `completeLocalOllama` | Negotiation pins loopback HTTP `/v1` and version `0.32.13`. Native tool input is decoded into a map, marshalled as an object, remembered and POSTed to `/api/chat`. Ordinary negotiated text `Complete` and explicit `CompleteBounded` share this serializer. `/native` and `/bounded` test both |
| [LastRequest](../pkg/agent/openai_provider.go), `LastRequest`, `remember` | Remembers actual serialized bytes and timestamp; returns **configured BaseURL**, not the dispatched request URL. The tests compare received bytes byte for byte with `LastRequest`, while asserting actual path separately |
| [Capture](../pkg/execution/request_capture.go), `captureRequest`, `sanitizeCapture` | Parses remembered JSON into a tree. Strings stay strings, objects stay objects; no argument-string decoding occurs. It redacts every string, bounds strings under `messages`, omits media and rewrites JSON. The tests write and re-read the 0600 capture and compare tree types |
| [Trace](../pkg/execution/request_trace.go), `record`, `cleanMap`, `write` | `wire` is a sanitized JSON tree derived from remembered bytes, not raw HTTP bytes or a server receipt. Flags report redaction, image omission, incomplete entries and dropped count. The public dispatch control persists and re-reads the wire tree and clean flags; existing trace controls exercise changed/dropped records |

For this current implementation, an object argument in a capture is not a
string that capture sanitization decoded. Native serialization provides a
source-supported origin for that shape. Historical provenance is still a
hypothesis because the first capture lacks an attested target route and source
identity. No provider/model defect follows from the shape alone.

Capture starts at 2000 runes per message string and can reduce that bound to
fit its file target. It keeps head/tail text with a cut marker. Redaction can
change content even when `truncated=false`; capture has no separate redaction
flag. `TestCaptureFidelityTruncationPreservesTypesButLosesContent` demonstrates
native object leaves shortened in place and compatible argument strings
shortened without changing their JSON type. Its compatible example remains
valid inner JSON despite lost content. The near miss matters: valid inner JSON
does **not** prove fidelity. `TestCaptureFidelityTruncationCanBreakCompatibleArgumentJSON`
uses a numeric array: valid on-wire JSON becomes an invalid inner argument
string after capture truncation, while the outer capture still decodes.

Even a clean trace's JSON tree is not a byte witness: property order, whitespace
and numeric lexical representation need not survive parsing and serialization.
The public compatible control demonstrates different raw-body and replay-body
hashes despite equal JSON trees. Fidelity flags cannot attest request headers,
the actual target URL, historical server processing or missing earlier rounds.

## Three different endings

1. **Console caller ending:** the supplied original terminal is
   `inconclusive:budget_exhausted`. In the authorized Console source
   `1d8aa1d8f120c8347d0431210c92475085182a2a`,
   [server.go](https://github.com/openexec/agent-console/blob/1d8aa1d8f120c8347d0431210c92475085182a2a/internal/server/server.go#L1826)
   uses a two-hour fallback context with a budget cause (or an earlier persisted
   budget context). Its [terminal classification](https://github.com/openexec/agent-console/blob/1d8aa1d8f120c8347d0431210c92475085182a2a/internal/server/server.go#L2366)
   maps that cause to the budget verdict.
2. **Native cancellation:** `completeLocalOllama` returns `ctx.Err()` when the
   caller cancels an in-flight HTTP request. `APIProvider.Execute` captures its
   own result/error, not Console's later classification. The new
   `TestCaptureFidelityNativeCancellationIsNotCallerVerdict` drives real native
   HTTP through `APIProvider.Execute`, cancels after dispatch with a distinct
   synthetic cause, and verifies persisted `failed`, empty reason and
   `context canceled`, while the caller still holds the cause. This proves how
   the labels can differ; it does not recover the historical cancellation cause.
3. **Replay parser refusal:** posting a retained object-argument body to a
   string-argument parser returns HTTP 400 before inference. The public local
   control posts the persisted native tree unchanged to its compatibility
   route and asserts that distinct error; the original unmodified native bytes
   pass at the synthetic native route. A genuine compatible serializer output
   also passes at compatibility. No stringification is used to make it pass.

The local server is an explicit schema model using Go's JSON decoder, not the
historical configured endpoint or its parser implementation. A parser-only
control is **not** same-endpoint proof. The captured error is not a recorded
endpoint cancellation response either: capture stores the native execution
error. HTTP 400 cannot reproduce the preceding two-hour caller budget.

The separately supplied 60-second native gateway result-loss witnesses concern
[gateway.go](../pkg/execution/gateway.go)'s tool HTTP client. Model header
timeouts (including 900 seconds), tool result transport, and caller cancellation
are different boundaries. Later frontend artifacts/terminal receipts cannot be
used to infer this historical wire route or a common cause. They are preserved
as separate supervisor evidence, not investigated by this finite task.

## Public controls and unavailable endpoint evidence

[capture_fidelity_test.go](../pkg/execution/capture_fidelity_test.go) and its
[native](../pkg/execution/testdata/capture-fidelity-native.json) and
[compatible](../pkg/execution/testdata/capture-fidelity-compatible.json) captures
contain only invented public text, one public tool and a reserved invalid
hostname. Each fixture's entire request tree is checked against actual
production serializer dispatch in the Go test suite. These are reduced
structural controls, **not** historical reconstructions or demonstrated minima.
Their compatible tool linkage, native `tool_name`, route-specific fields and
argument types come from those serializers, not an object-to-string rewrite.

The executable local control is `go test ./pkg/execution -run
TestCaptureFidelity -count=1 -v`. It is part of the Go suite already declared
by CI. It records only public request hashes, route and argument type, writes
and re-reads temporary captures/traces, exercises 400 and passing near misses,
and makes no live provider call. The fixtures deliberately have no genuine
endpoint, credential or model; they must not be mistaken for runnable historical
requests at their placeholder destination.

**The public same-configured-endpoint control remains UNPROVEN.** The fresh
private replay is not credited for it. The first capture's authorized summary
does not include the configured URL/model, actual route or credential reference.
Those cannot be taken from another task's endpoint or guessed. The task also
reserves later host measurement to supervisor tooling, bounded to at most two
synthetic requests, each budget 1 / timeout 120 seconds. No such request was
made here; the allowance is unconsumed by this pass.

The supervisor says Console's actual replayer exists at `scripts/api-replay.py`.
The granted Console checkout at `1d8aa1d8…` returned the exact filesystem error
`sed: can't read /mnt/data1/projects/agent-console/scripts/api-replay.py: No such file or directory`.
Typed `inspect_repository` for that tracked path returned **`files of
agent-console: nothing to report.`** This is a checkout discrepancy, not proof
that the supervisor's host script does not exist. No private scratch copy was
opened and no foreign checkout was changed. The [completed native172 account](QWEN_QUERY_TEMPLATE_DIAGNOSIS.md)
documents an older replayer that posts to `<baseURL>/chat/completions`; that
historical account is the only available source for the replayer behavior, not
an attestation of the current script. Exact current replay construction and
revision remain unverified.

A wire-faithful **historical** control cannot be reconstructed: no same-run
raw request/target/trace is available, the first summary supplies shape only,
and the corroborating captures explicitly lose text. The source-proven public
native control establishes what current code emits, not what that process sent.
Recovering a complete final body would still not recover the earlier model
responses, tool results, per-round timings and caller cause required to replay
the original loop. No omitted content is filled in from current files.

## Smallest correction proposal and test plan

The verified defect in diagnostic provenance belongs to native
`OpenAIProvider.remember` / `LastRequest` and the existing capture/trace boundary:
the actual serialized body and configured base URL can describe different
protocol routes. The smallest source-supported follow-up is to retain the
actual dispatched target with the existing remembered body, expose it
separately from `baseURL` in existing diagnostics, and preserve the configured
base field for old consumers. This is a proposal to correct diagnostic
provenance, **not** a proposed serializer or original-budget repair. No
production correction is implemented here.

Fail-before/pass-after/removal plan for that separately scoped correction:

- Extend the existing public dispatch control to require captured/trace target
  metadata equal the URL the test server received. Current native code cannot
  satisfy that assertion: `LastRequest` returns the configured `/v1` base for
  the `/api/chat` dispatch. Cover compatible Complete, negotiated Stream,
  ordinary/bounded native Complete and forced/media/unnegotiated fallbacks.
- After the correction, require target metadata for every remembered dispatch,
  with body and route kept together. Preserve string/object argument types,
  privacy transformations, old capture handling and current serializer output.
  Test stale/no-send and refused requests so metadata cannot attest an unsent
  request merely because a body was remembered before `Do`.
- Remove the target propagation in a negative control; the native route assertion
  must fail again. This would prove the provenance fix only.
- In **agent-console**, inspect and pin the actual replayer revision before any
  correction. If it blindly appends `/chat/completions` as the historical script
  did, use attested dispatch protocol/target or refuse an ambiguous capture
  before a request is spent. Add a failing native-shaped-capture replay test,
  passing correct-route and ordinary-compatible controls, and a removal test
  restoring the wrong-route refusal. Do not infer the route solely from object
  arguments or stringify them. Foreign source implementation is separate work.

No passing causal change, minimum, original-budget reproduction, effective
permission failure or guessed provider/model cause is established. No admitted
original is restarted or requeued. Original source publication, metrics,
Delivery, private listener, runner outcomes and their original acceptance,
Stop and accounting remain owed.

## Prior credit and architecture

Completed **df875/native164** already provided opt-in full trace and an offline
reader/replay witness. [Its sequence diagnosis](NATIVE_TRIAGE_SEQUENCE_BOUNDARY.md)
records the private offline replay stopping at **request 14 of 18**, with
`messages` divergence; an operational reader pass is not an 18-of-18 replay.
That investigation and completed native172's different query-template/header
diagnosis are not reopened. Their artifacts and limitations remain credited.

The user outcome is a finite account of capture/forwarded-wire/replay fidelity.
Current state is two native serializers, lossy diagnostic trees and unbound
historical routing. The owning boundary is native transport/capture plus
Console's existing replay consumer, not another implementation or Goal loop.
Reuse existing serializers, private capture/trace, offline replay and
deterministic delivery. Complexity delta: **zero production concepts, persistent
state, transitions, owner decisions or controllers added; nothing replaced.**

Sibling-path audit covered compatible Complete/Stream; negotiated ordinary and
bounded native calls; forced-choice, media and unnegotiated fallbacks; CLI
negotiation in `internal/cli/execution_stdio.go`; run-local bounded-adapter
unwrapping in `lastRequestOf`; last-request capture versus full trace/offline
replay; and Console's separate terminal classification. Existing fallback tests
exercise media/unnegotiated routes. Native non-thinking completion shares the
same serializer with an optional `think` field; it adds no argument conversion.
Other providers do not implement this remembered compatible request boundary
and are outside this observed incident. No guard, matcher, rewrite or production
behavior was added. Legacy project formats, task state and migration are
untouched, so their compatibility cannot have changed through this patch.

## Verification and delivery boundary

Focused host verification returned:

| Selection | Actual result |
| --- | --- |
| Declared `go-test`, `./pkg/execution -run TestCaptureFidelity -count=1 -v` | `go-test exited 0`; `PASS`; final package result `0.020s` |
| Direct focused `pkg/execution`: new fidelity tests, capture tests, trace controls, fixture offline replay and divergence, structure reporting, caller-deadline and round-limit tests | `Succeeded in 7.052s`; `PASS`; package `6.635s` |
| Direct focused `pkg/agent`: `TestOllama…`, `TestUnboundedLocalOllamaComplete…`, `TestLocalOllamaCompletePreservesCallerCancellation` | `Succeeded in 1.441s`; `PASS`; package `0.520s` |
| `go vet ./pkg/execution` | Exit 0, no diagnostics |

The initial sandbox run was **not passed**: `httptest: failed to listen on a
port: listen tcp6 [::1]:0: socket: operation not permitted`. Host tests above
resolved that environment restriction. The declared-check tool refused combined
`-run` regexes containing `|` (`has characters a declared check does not take`);
the direct host test tool accepted the focused argv. These are finite local
fixture checks, not live inference.

CI already declares `go test -v ./...` on Linux/macOS, so an examiner can execute
these tests through the declared `go-test` check without a standalone script.
The full local gate was not run, as instructed. Independent diagnosis review
inspected the code, provenance, test validity and privacy; it reported no
blocking code/privacy findings and required correcting the foreign-checkout
sentence above and preserving the endpoint-control gap. It did not rerun tests
or contact an endpoint. The separate lane examiner and exact-head hosted
CI/publication are still pending; this note does not assert their future
results. No deployment is required for this
diagnosis. Any later authorized synthetic endpoint measurement remains a
separate supervisor obligation, and merging a diagnosis proves no original
product recovery.
