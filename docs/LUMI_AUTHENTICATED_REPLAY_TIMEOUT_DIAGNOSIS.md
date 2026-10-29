# Hosted Qwen and authenticated replay timeouts, 29 October 2026

Status: **report-only diagnosis; cause unknown; `reproduced=false`; no
cause-proven repair, minimum failing request or passing single-change control.**
Signal: `sig-8a9440cc6148`. The new observation is that the authenticated
truncated replay also times out. This does not explain why either client
timed out, establish identical transport phases, or deliver the original
conversation and queued useful work.

## Reconstruction from retained evidence

The [public metadata extract](evidence/lumi-header-timeouts-2026-10-29.json)
contains allowlisted fields read from actual private captures, original event
files and, for the first two runs, persisted run records. It includes private
artifact paths relative to `/home/perttu/.agent-console` and file SHA256s.
These identify the inspected files, **not original request wire bytes**.
No message bodies, tokens, credentials or private captures are published.

This inspection's HEAD and supplied `origin/main` both equal
`7f1692da65e15b6db5f23377fb43340db9fc894d`. The triage's `c6b5101b` was its
earlier inspection baseline, not this candidate's base or an attestation of
the executable serving any of these requests.

All times below are UTC on **29 October 2026**. All captures identify
`lumi-qwen38-27b`, `Qwen/Qwen3.8-27B`, and initial POST
`https://aitta-api.csc.fi/openai/v1/chat/completions`, with dispatch provenance
`source-known-transport-attempt`. The original serialized sizes and elapsed
waits below are from each actual `turn.error`, checked equal to its capture
error. For the first two they also equal the persisted run error.

| Run | Capture sentAt | Capture / turn.error | turn.failed | Original serialized bytes | Native wait / limit |
| --- | --- | --- | --- | ---: | --- |
| `377289c1b0a23df3a151553fc69fe67b` | 11:03:28 | 11:05:28 / 11:05:28.967690600 | 11:05:31.356130533 | 39,599 | 120.029s / 120s |
| `f9a93297a10b6fab17c9bb0c6600a912` | 11:10:13 | 11:12:13 / 11:12:13.653632777 | 11:12:16.170488865 | 39,547 | 120.030s / 120s |
| `9cb9c09f82ec37d3976358fc1972349c` (addendum) | 11:33:21 | 11:35:21 / 11:35:21.307417135 | 11:35:22.652690603 | 39,571 | 120.028s / 120s |
| `3c538a9fc9716d51bff31689b9b7250f` (addendum) | 11:36:16 | 11:38:16 / 11:38:16.653203125 | 11:38:18.351937602 | 39,485 | 120.030s / 120s |

Both primary persisted runs have mode `workspace-write`, provenance `owner`,
and session `51b131f680521e2db45cf40a7e2ab811`. They started at
11:03:24.467997878 and 11:10:06.289124892 respectively. Each of the four
original event streams consists of `outcome.source.observe`, `turn.started`,
`turn.error`, `turn.failed`. There is no recorded assistant output or tool
dispatch. The contextual `outcome.source.observe` event is not a model tool
execution. The two addendum files named `run-*.json` contain **event lists**;
no separate persisted run record was inspected for those two. Their reported
session relationship remains supervisor-supplied context.

The addendum's aggregate count 20 and lastSeen 11:36:13.114398374 are
supervisor-reported, not a count recomputed here. That timestamp equals the
newer run's `outcome.source.observe`, rather than its 11:38:16 `turn.error`.
Do not assign the aggregate timestamp to its HTTP failure. These additional
observations neither establish a shared root cause nor authorize another
unchanged probe.

### Original request, captured content and replay are different witnesses

Every capture has `truncated=true`, `textLimit=2000`. The primary captured
message roles are system/user/assistant/user, with character counts
`[2026,25,5,81]` and `[2026,25,5,29]`, and 19 tool definitions each. Counts
describe retained strings; the system's cut marker accounts for part of
its 2,026 characters. None is a faithful archive of the original 39,599- or
39,547-byte body. No original wire body hash or original complete body is
available in these artifacts. Definitions of tools do not prove execution.

`pkg/agent/openai_provider.go`, `Complete` and `Stream`, validate and serialize
the compatibility request, build POST `<configured base>/chat/completions`,
set the configured Authorization header, remember body/initial dispatch, and
call `http.Client.Do`. The diagnostic size is `len(body)` at that call.
The captured target agrees with this route; the earlier local native
`/api/chat` versus compatibility-route discrepancy is not demonstrated here.
Remembering bytes and dispatch proves a source-side attempt, not completed
upload, a redirected destination, endpoint receipt or inference start.

`pkg/execution/request_capture.go` sanitizes a copy of the remembered JSON,
redacts secrets, omits images and bounds message/argument/reasoning strings.
It atomically writes privately (0600); old adapters can lack dispatch.
`sentAt` precedes `Do` and is persisted to whole seconds. Neither it nor
`capturedAt` is a server timestamp. Opt-in request trace has its own fidelity
flags and per-entry dispatch; it also is not a raw wire archive and cannot
recover these past requests. See [the dispatch contract](REQUEST_DISPATCH_PROVENANCE.md).

### The authenticated replay's literal result

The supplied safe summary at
`perttu/scratch/20261029T111647Z/replay-new-summary.json` records:

| Field | Observed value |
| --- | --- |
| `reproduced` / `partial` | **false / false** |
| `requests` / `seconds` | **1 / 120.2** |
| `first.outcome` | **fail** |
| Target class | `timeout` |
| Target normalized message | `server_error: http error: post "<url> context deadline exceeded (client.timeout exceeded while awaiting headers) (waited` |
| Replay class | `timeout` |
| Replay normalized message | `http error: request timed out after #.#s` |

The supervisor reports existing configured authentication, same endpoint/model,
timeout 120 seconds and budget 1. Its shape agrees with the truncated `f9a`
capture: the four character counts above, original model and 19 tools.
The private result was inspected only for metadata: `attempts=[]`, `passes=[]`,
one request and no reproduction. Its `minimalRequest` field name does not
establish a minimum. No reduction or passing single change exists.

The existing pinned source comparator requires **both class and exact normalized
message**. `TestLUMIReplaySignatureControl` executes that source excerpt offline
through the declared Go suite. It demonstrates the mismatch, identical-message
controls, existing time/URL normalization, different-message timeout near misses,
HTTP/authentication errors and inconclusive/outcome separation. Broadening the
comparator would manufacture a different claim, not establish fidelity or cause.

The retained Console replayer source snapshot's SHA256 is
`544bad733f239a063d7f1871e2175fd495ead1d194d3e19d141fdc6dda5b9195`.
Its `Endpoint.send` reserializes a JSON tree using Python `json.dumps`, then
uses `urllib.request.urlopen` and reads a bounded response body in the same
try block. Its generic timeout handler covers both that opening operation and
the body read. Therefore its message does not isolate connect, TLS, upload,
response-header or response-body phase. The same configured numeric timeout
also does not attest identical deadline semantics between Python and Go.
The result does not attest the executed replayer revision, replay wire hash/size,
phase timings, HTTP receipt, request ID or effective launch config. The snapshot
and existing pinned comparator explain the result's comparison semantics;
they do not attest the historical replay process. No credential was read or
hosted request executed by this diagnosis pass.

## What the observations discriminate

The Go error is literally `context deadline exceeded (Client.Timeout exceeded
while awaiting headers)`. Its client deadline includes work before response
headers. There are no per-phase observations of DNS, connection, TLS, write
completion or first byte. The suffix reports elapsed `Do` time, configured
client timeout and serialized size; it does not distinguish those causes.

| Hypothesis | Evidence now | Missing discriminator / conclusion |
| --- | --- | --- |
| Request construction or capture/replay change | Current compatibility builder and recorded initial target agree. Capture intentionally loses content; Python reserializes it. Existing real serializer tests cover both routes and fidelity loss. | Full original body, effective historical builder/config and replay wire hash/size. No newly demonstrated local defect or causal single-change control. |
| Client/network transport stall | Both clients time out; only Go reports awaiting headers. | Correlated connect/TLS/write/first-byte timings and receipt records. Cannot equate Python body/connect/TLS/header timeout with Go's observation. |
| Missing/invalid credential | Go reached `Do`, with no HTTP authentication response recorded. Supervisor reports the replay was authenticated. | Historical credential-reference/injection provenance and gateway authentication result, without values. Current empty-reference refusal does not prove original credentials were accepted. |
| Endpoint delay, queue, prefill or backend fault | Native client waited its whole 120-second limit; this later lossy replay failed rather than the earlier passing control. | Gateway receipt, auth, model/backend routing, queue/start/prefill and first-header timeline, including cancellation/late completion. No provider-side causal evidence is retained here. |

Thus the replay **now times out for an undetermined reason**. The earlier
passing truncated control and this later timeout used different retained
requests and times; they are not a one-variable experiment. Neither a small
user input, body size nor elapsed time selects a cause. The absence of an HTTP
error response does not establish authentication success, and metadata catalog
readiness is not inference availability.

Historical executable path/digest/revision, launch config directory/revision,
credential-reference provenance and phase/server records remain unavailable.
Current checked-out source is not historical executable attestation. The primary
event streams have no `base.refreshed` event to use even as a repository refresh
record, and such a record would still not attest a running binary.

## One bounded supervisor evidence input

No further unchanged replay is requested: the supplied single-request allowance
is spent and these captures are lossy. The following is **one read-only supervisor
input for retained-record correlation**, not an invented executable/tool or a
new measurement grant. It fixes the configured route identity and historical
120-second limit, requests no credential value or body, and has request budget
zero. Endpoint/launch records not available to this worktree must be reported
as unavailable. It can finish with missing records; do not substitute a fixture.

```json
{
  "operation": "correlate-retained-records-only",
  "signal": "sig-8a9440cc6148",
  "run_ids": ["377289c1b0a23df3a151553fc69fe67b", "f9a93297a10b6fab17c9bb0c6600a912"],
  "native_route": "existing Console-configured OpenExec execution-stdio --provider api --api-provider lumi-qwen38-27b",
  "model": "Qwen/Qwen3.8-27B",
  "initial_method": "POST",
  "initial_url": "https://aitta-api.csc.fi/openai/v1/chat/completions",
  "historical_request_timeout_seconds": 120,
  "additional_http_request_budget": 0,
  "native_utc_window": ["2026-10-29T11:03:22Z", "2026-10-29T11:12:17Z"],
  "replay_summary": "/home/perttu/.agent-console/perttu/scratch/20261029T111647Z/replay-new-summary.json",
  "request": [
    "Retained launch executable digest/revision and config directory/revision, plus configured credential resolution/injection provenance only; no values",
    "Whether a private complete original wire witness exists; report its fidelity, byte count and hash only, or unavailable",
    "Replay execution timestamp, replayer revision, actual wire bytes/hash and phase timings where retained, or unavailable",
    "Correlated gateway request ID/receipt/auth decision, backend/model route, queue/prefill/first-header/cancellation times where retained, or unavailable"
  ],
  "output": "Allowlisted shape/hash/timing/status and provenance metadata only; no bodies or credentials",
  "stop": "Return retained evidence or explicit unavailable fields; no HTTP probes, retries, minimization, new tracing or configuration changes"
}
```

For any later separately authorized measurement, reuse the existing configured
credential resolver and named provider, model, endpoint and 120-second native
client limit, with an explicit finite request budget. `newConfiguredAPIProvider`
loads `Request.ConfigDirectory()` (or envelope directory), resolves the named
entry and existing credential reference, and refuses empty resolution. Console's
`internal/providers/openexec.go` injects the one resolved credential into the
child environment. A shell without this injection is not that configured route.
No task code should read credentials or create a service/key-file workaround.

First establish a faithful private witness and equivalent native signature.
Only then minimize one part at a time and identify the smallest passing change
before proposing a correction at that same boundary. If fidelity or reproduction
fails, stop with that result. No cause-proven correction is supported now.

## Verification, sibling paths and delivery boundary

The directly executed focused check returned **exit 0**:

```text
go test ./pkg/agent ./pkg/execution -run 'Test(OpenAIHeaderTimeout|OpenAIHTTPStatusErrorHasNoTransportSuffix|LUMIReplaySignatureControl|CaptureFidelity|DispatchProvenance|APIProviderCaptures|RequestTraceControls)' -count=1 -v
ok github.com/openexec/openexec/pkg/agent 0.738s
ok github.com/openexec/openexec/pkg/execution 8.203s
```

These existing tests exercise actual client serialization/HTTP, execution and
private capture/trace persistence and re-read, refusal/stale/legacy/privacy
branches, Complete/Stream and native/bounded/compatibility/fallback formats.
The dispatch removal control failed its intended behavioral assertions under
the source overlay and passed with restored source. The pinned comparison's
four offline Python controls passed. These are local synthetic controls of
existing behavior, **not a reproduction of hosted provider cause**.

The additional focused configured-route/readiness check also returned **exit 0**:

```text
go test ./internal/cli ./pkg/execution -run 'Test(APIProvider(AdvertisesOnlyConfiguredModel|SelectionIgnoresActiveProvider|UnknownNameListsConfigured|RejectsUnresolvedKeyReference|RequiresName)|LoopbackEndpointGetsUnattendedRequestBudget|APIReadiness.*)$' -count=1 -v
ok github.com/openexec/openexec/internal/cli 0.014s
ok github.com/openexec/openexec/pkg/execution 0.010s
```

It verifies named-model selection, unknown/missing-provider and unresolved-key
refusals, hosted/loopback/lookalike timeout selection, metadata auth/forbidden/
missing-model/malformed/unsupported branches, cancellation and redirects, with
no inference fallback. No live-provider test is selected.

Sibling-path inspection covered Complete/Stream, bounded native completion and
forced-choice/media/unnegotiated compatibility fallbacks; last-request capture
and per-round trace; stdio named configuration/credential resolution versus
pipeline client construction; and metadata readiness versus inference. The
checkpoint replay CLI and offline trace-response replay are different artifact
boundaries, not substitutes for authenticated HTTP replay. No new guard,
matcher, filter, rewrite or correction is introduced on any of them. No failing
before/passing after regression is invented for an unproven local defect.

This report adds two documentation/evidence files and changes no runtime source,
tests, timeout/model/provider/budget defaults, capture controls, credential
resolution, project loading or legacy behavior. No code-change build/lint gate
is applicable. No full declared gate, hosted CI, exact-head independent review,
installation or deployment is claimed by this pass; independent review and
hosted CI remain lane delivery gates. There is no served correction to deploy.

Owning boundary: existing OpenExec API transport/capture/replay. Existing
request builder, configured credential resolution, private capture and replay
controls suffice for the finite diagnosis; no new abstraction is necessary.
Complexity delta: **zero** new controllers, persistent architecture, transitions,
authority or owner decisions; no existing execution machinery replaced.

The [prior Qwen diagnosis](LUMI_HEADER_TIMEOUT_DIAGNOSIS.md), completed diagnoses
169/177 and dispatch work 176 retain their results and limitations. This does
not reopen them, implement Console executor/Reliability/Delivery/private-listener
work, claim diagnostic installation repaired an endpoint, or waive original
requests. Existing Stops and probe accounting remain unchanged. A separately
authorized served correction still requires a natural
owner response and original queued useful work; five clean runs/24h are
supervisor post-deploy measurements, **not a merge requirement** for this finite
diagnosis. Missing causal evidence is an engineering limitation, not an owner
decision or a claim of provider recovery.
