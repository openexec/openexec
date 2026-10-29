# LUMI GPT-OSS recurrence, 29 October 2026

Signal `sig-d2ae78bd0468`. **Cause unknown. Exact signature not reproduced;
no minimized request, passing causal change or provider repair established.**
This finite diagnosis concerns `lumi-gpt-oss` / `openexec-api` only.

## Observed evidence and unavailable timeline details

The following is the approved task's supervisor-supplied safe terminal extract,
not a new inspection of private events or captures. All times are UTC on
29 October 2026. No private scratch, credential, prompt or schema was read.

| Failed turn | Events 1–5 | Event-stream ending | Total events | `assistant.delta` / `tool.started` | Native timeout / serialized bytes |
| --- | --- | --- | ---: | --- | --- |
| `2df152fb15fa27b6ac0ff02dd12b810c` | Individual types and timestamps not supplied | `07:52:20.041317554Z` | 6 | 0 / 0 | 120s / 39,521 |
| `e3eed1db29f71c2abd80ef4761b48167` | Individual types and timestamps not supplied | `08:01:14.912336474Z` | 6 | 0 / 0 | 120s / 39,521 |

The supplied extract does not name the final event type either. These are the
two available timelines, not invented six-row sequences: turn start, request
start, intermediate events and individual elapsed waits remain unavailable.
Subtracting 120s from a terminal timestamp would not recover those timestamps.
Zero recorded assistant/tool dispatch does not prove zero endpoint processing.
Both errors report native `Client.Timeout` while awaiting headers. Neither
record attests mode, exact resolved model string, principal, historical
credential status or endpoint receipt. The provider name is not model identity.

The supervisor already performed the entire permitted measurement control:
one authenticated Python replay of the `e3eed…` capture, using the existing
current global provider credential, original endpoint/model, timeout 120s,
budget 1. It returned in **120.2s**, `requests=1`, `first.outcome=fail`,
`reproduced=false`. The capture truncates content; its reserialized replay is
not the original **39,521-byte** wire body. Replay wire size is not supplied.
No reduction or passing causal change exists. The private result was not
opened or copied; this account uses only the approved safe extraction.

The extracted signatures are exactly:

| Representation | Class | Normalized message |
| --- | --- | --- |
| Native target wrapper | `timeout` | `server_error: http error: post "<url> context deadline exceeded (client.timeout exceeded while awaiting headers) (waited` |
| Python transport | `timeout` | `http error: request timed out after #.#s` |

The messages differ. A shared timeout class, strict comparison failure and
similar elapsed budgets each establish **no causal equivalence**. The replay
neither reproduces the exact signature nor tests the original full request.

## Prior credit, source identity and hypotheses

[Native169](LUMI_HEADER_TIMEOUT_DIAGNOSIS.md) delivered the finite historical
LUMI Qwen diagnosis and found no cause. [Native175](NATIVE_CAPTURE_REPLAY_FIDELITY_DIAGNOSIS.md)
distinguishes native errors, caller endings and replay parser errors, and
demonstrates why sanitized trees cannot attest original bytes or routes.
[Native176](REQUEST_DISPATCH_PROVENANCE.md) implements the requested dispatch
evidence; it is reused here, not reimplemented. Historical `lumi-qwen38` and
LOCAL Qwen 900s/caller-budget endings are separate incidents, not evidence for
this GPT-OSS recurrence. The original Reliability, Delivery, private-listener
and retained-runner outcomes and their accounting remain unchanged.

Inspected source base is `25d529becc8fc75e155111bc1e95d882c642c2ad`, supplied
`origin/main`. Its native176 merge timestamp is `07:58:08Z`: between the two
stream endings. The supervisor's earlier installed observation is
`5fe138d17a9b`; neither observation binds either failed process to an executable.
A merge, current checkout or `base.refreshed` revision is not launch attestation.
In particular, the later ending does not establish use of native176. Do not
assign `dispatch` evidence retrospectively to either failure.

Current [Complete and Stream](../pkg/agent/openai_provider.go) marshal before
`Client.Do`, set Authorization separately, remember body/time/initial target
immediately before the attempt, and report buffer length and elapsed client
wait on transport error. This rules out a pre-marshal rejection for that
current error path; it does not prove historical body validity or upload
completion. `http.Client.Timeout` spans connection, upload and header wait;
the wrapper does not isolate DNS, TLS, network, authentication, admission,
queueing or inference. Configuration/reference validation can reject before
HTTP, but a header timeout cannot prove which credential reached the gateway.

| Competing hypothesis | Distinguishing evidence | Present evidence limit |
| --- | --- | --- |
| Endpoint admission/congestion or slow processing | Correlated gateway receipt, auth result, model/backend routing, queue/admission/start and first-header/cancellation times | No endpoint timeline supplied; elapsed timeout alone cannot select it |
| Client construction/serialization or transport | Attested executable/config plus matching request shape, initial dispatch and gateway receipt/parser decision; existing write/connection records if retained | Only original byte count and lossy capture; no full-wire or transport-phase witness |
| Credential/reference failure | Historical reference-resolution provenance and correlated gateway auth decision or actual auth response | Current authenticated replay does not attest historical principal or validity; no auth response supplied |

No local provider defect has been reproduced. The comparison mismatch is an
existing representation limitation, not proof that normalization is defective.
Any proposed Console comparator change would require a separate precisely
scoped review; broadening matching to call this reproduction would weaken the
evidence. No such candidate or provider repair is implemented here.

## Offline signature control

`TestLUMIReplaySignatureControl` in the declared Go suite executes
[the supplied Python excerpt](../pkg/execution/testdata/lumi-signature/console_signature_excerpt.py),
not a Go approximation. Supervisor source revision:
`002e792e92bcc334fccd7dec530fb4cbf66dca6f`; full `scripts/api-replay.py` SHA256:
`544bad733f239a063d7f1871e2175fd495ead1d194d3e19d141fdc6dda5b9195`.
That is the supplied full-file identity, not a locally verified hash of this
excerpt. The credential-free signature functions and comparison closure were
provided in the task appendix; the rest of the replayer was not inspected.

The [harness](../pkg/execution/testdata/lumi-signature/control.py) appends only
`return same` to expose the otherwise unchanged closure. It drives real capture
validation, target outcome selection, normalization and both-field comparison.
Invented public errors use `example.invalid` and an invented body size, with
no historical request content. Socket creation is refused during the control.
The native-shaped wrapper produces exactly the supplied target message; the
Python-shaped timeout produces exactly the supplied replay message. They do
not match in either direction. Identical errors and existing numeric/URL
normalization match. A distinct timeout, HTTP 401, HTTP 503 containing the
word timeout, cancellation and caller-budget ending do not match. Equal text
with a different target outcome/class also refuses. Invalid capture shapes
exercise the original validation refusal.

Thus these two representations cannot match under the pinned rules. Other
inputs can match after existing normalization; even such a match would be
signature equivalence only, never proof of a shared historical cause.

## Smallest next natural observation

After separately authorized normal activation of native176, inspect **one next
naturally failing run**, without retrying an original or enabling more tracing:

1. Bind the run ID to its launch record: resolved executable/build revision or
   digest, config identity and credential-reference provenance, never the value.
2. Read its already-enabled private last-request capture: `provider`, `baseURL`,
   `sentAt`, `capturedAt`, `outcome`, `reason`, `error`, `request`, `truncated`,
   `textLimit`, and optional `dispatch.method`, `dispatch.url`,
   `dispatch.provenance`. The recognized provenance is exactly
   `source-known-transport-attempt`. Associate that target with that body only.
   If an existing trace is retained, use that specific `entries[i].dispatch`
   alongside `wire`, `sentAt`, error/response; consult
   `redactionChangedContent`, `imagesOmitted`, `incomplete`, `droppedEntries`.
3. Correlate already-retained endpoint records by request ID if available, or
   narrowly by time/target and private authorized provenance. A trace's
   `requestID` is a client identifier, not proof the endpoint received it.
   Gateway receipt/authentication/parser rejection distinguishes construction
   or credential refusal; successful auth and queued/admitted work followed
   by delayed headers supports an endpoint delay. Without a reliable link or
   endpoint records, retain **unknown cause**. Missing receipt logs alone do
   not prove client failure.

Native176 provides the initial source-side target, not upload completion,
redirect destination, receipt, auth decision, queue state or phase timings.
Its [capture/trace boundary](../pkg/execution/request_capture.go) omits malformed,
stale, pre-attempt and privacy-altered dispatch. Userinfo, query, fragment or
secret-shaped URL/path means unavailable, not a sanitized attested target.
Absence remains unknown. Captures are private 0600 sanitized JSON with bounded
message text, media omission and possible redaction even if `truncated=false`;
trace trees are not raw bytes/headers. Keep body, prompts, schemas and credential
material private; publish only allowlisted metadata. No new endpoint request,
host measurement, trace enablement, full-body retention, timeout/model/provider/
credential/cap/allowance/Stop change, public exposure or paid resource is needed
or authorized by this diagnosis. This evidence recipe does not authorize access
to endpoint records or future execution.

## Verification and delivery

Focused tests executed successfully with `-count=1 -v`:

| Selection | Evidence |
| --- | --- |
| `pkg/execution`, `TestLUMIReplaySignatureControl` | Four Python controls, including nonmatches and validation refusal, `OK`; Go `PASS` |
| `pkg/agent`, `TestOpenAIHeaderTimeout` and `TestOpenAIHTTPStatusErrorHasNoTransportSuffix` | Real local Complete/Stream header stalls, received-body byte count, status-response near miss |
| `pkg/execution`, `TestCaptureFidelity`, `TestDispatchProvenanceExecuteRoundTrip`, `TestDispatchProvenanceTraceBindsEachRound` | Real serializers, native/bounded/compatible routes, lossy replay and capture/trace re-read |
| `pkg/execution`, `TestDispatchProvenanceUnknownAndPrivacy`, `TestDispatchProvenancePrivateURLExecute`, `TestDispatchProvenanceRefusedExecuteExcludesPreviousAttempt`, `TestAPIProviderCaptures`, `TestRequestTraceControls` | Legacy/stale/malformed/privacy/pre-attempt refusal, private persistence, redaction/truncation and trace controls |

Direct focused runs returned `PASS`, `pkg/agent 0.710s` and
`pkg/execution 0.891s`. Declared `go-test` with
`./pkg/execution -run TestLUMIReplaySignatureControl -count=1 -v` separately
returned **`go-test exited 0`**, four Python tests `OK`, package `0.106s`.

The Python control is invoked by a Go test, so existing `.github/workflows/ci.yml`
`go test -v ./...` and declared focused `go-test` include it; it is not an
unconnected verification script. These controls prove client/evidence boundaries,
not the recurrence's cause. No full local gate or live provider call ran.
Independent review of the exact published candidate and green required hosted
CI remain delivery gates; this diagnosis does not assert their future result.
No runtime deployment is part of this documentation/test-only candidate.

Sibling-path audit reused native176's three send/remember sites: compatible
Complete/Stream and native ordinary/bounded completion, plus forced-choice,
media and unnegotiated fallbacks; both capture and per-round trace writers and
bounded-adapter unwrapping. CLI named configuration delegates to these clients.
Offline trace replay and checkpoint replay are different artifacts; neither
contains Console's signature comparison. No production guard/filter/rewrite
was added. The pinned comparator's near misses are covered without changing it.
No runtime source change means no failing-before/passing-after/removal repair
claim is applicable. Existing legacy project/config/task behavior is unchanged.

Owning boundary: native request/error/capture/trace evidence and Console's
separate replay comparison, delivered through existing deterministic review/CI.
Complexity delta: one finite diagnosis and test-only offline controls; zero new
runtime abstractions, loops, controllers, persistent state or authority. No owner
decision is needed. Original useful outcomes are neither delivered nor credited
by this diagnosis.
