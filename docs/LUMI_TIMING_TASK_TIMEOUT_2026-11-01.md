# LUMI timing-task recurrence, 1 November 2026

**Diagnosis: cause unknown; no faithful failure reproduced and no repair
established.** Signal `sig-8a9440cc6148`. This is a finite diagnosis, not
evidence that the prior diagnosis repaired the endpoint. The original timing
task `3e5763d7ebff997b6dc271d543083561` still owes its authored regression
and retained acceptance criteria; this report does not reset its accounting.

## Original observation

The private capture is `/home/perttu/.agent-console/run-requests/55ae72438a700322b0da4f077d25d55c.json`
(SHA-256 `c5b20838a87cc728dd0421cea4cfa2719da7e412a43cb0ad50c986f3abae6793`).
The separately retained event list is `/tmp/watch-events-55ae72438a700322b0da4f077d25d55c.json`
(SHA-256 `9a7af9e7d958d21a20cf2e6ba4a2ef999f10e99de288147d0ff3400161e42f2a`).
These hashes identify artifacts, not the original wire body. The task/run
association is supplied by the task contract; the capture and event list
identify the run, not the task.

The capture identifies `lumi-qwen38-27b`, model `Qwen/Qwen3.8-27B`, and a
source-known POST transport attempt to
`https://aitta-api.csc.fi/openai/v1/chat/completions`. Its `sentAt` is
17:05:34 UTC and `capturedAt` 17:07:34 UTC. The capture error and both event
errors agree: `Client.Timeout exceeded while awaiting headers`, waited
2m0.029s of a 2m0s request timeout, body 94,887 bytes. On 2026-11-01 UTC the
events are `turn.started` at 17:05:27.157635483, `base.refreshed` at
17:05:30.130840675, `turn.error` at 17:07:34.246688526, and `turn.failed` at
17:07:36.933330041. No assistant or tool event is recorded.

The refresh names `79ca219813e96d3a57b6f278427504dd328510ec` as repository
`origin/main`; it is not an executable attestation. This candidate was
inspected at `a70256ebaffe7ec412183d636b06b7e6dda58e80`; neither revision
establishes the binary used for the failed run. Historical executable build,
digest and launch/configuration provenance are unavailable in the inspected
records.

## Fidelity and comparator

The capture says `truncated=true`, `textLimit=2000`; its retained system and
user strings are each 2,026 characters and it contains 22 tool definitions.
`pkg/execution/request_capture.go` sanitizes and bounds captured text. The
capture is not the original 94,887-byte wire request, and its file hash is
not a wire-body hash. `sentAt` precedes the HTTP call and is not gateway
receipt. The diagnostic reports a serialized buffer length and elapsed
client wait, not completed upload or server receipt.

The supplied supervisor account reports an initial replay rejected for a
missing token (operator error), followed by exactly one authenticated,
configured same-endpoint/model replay that passed within a 120s bound. The
replay result artifacts and comparator implementation were not independently
located in this pass; exact replay bytes, client/build and timing are not
attested here. Even accepting that account, a passing replay of the truncated
capture is neither an equivalent full-request failure nor evidence of the
original cause. The prior [configured diagnosis](LUMI_HEADER_TIMEOUT_DIAGNOSIS.md)
and [authenticated replay diagnosis](LUMI_AUTHENTICATED_REPLAY_TIMEOUT_DIAGNOSIS.md)
describe different incidents and their own comparator limits; neither proves
this recurrence's cause.

## Boundary and next evidence

In the present source, both `Complete` and `Stream` in
`pkg/agent/openai_provider.go` remember the marshaled request before
`http.Client.Do`; the timeout diagnostic includes elapsed wait, configured
budget and body length. The client's timeout can include connection, TLS,
upload and header wait. The observation does not distinguish client/network
stall, authentication processing, gateway queueing or inference/prefill.
These are hypotheses, not measured causes. A later pass or a size alone does
not choose among them.

No safe faithful source-bound discriminating witness is established from
these retained artifacts. No minimization or failing-before/passing-after
repair test is justified without an equivalent failure. Needed evidence is:

1. A privately retained full original serialized request and wire fidelity
   evidence, or a proven equivalent; the sanitized capture cannot supply it.
2. Historical resolved executable build/digest and launch/configuration
   provenance, including credential *reference* resolution, never its value.
3. Correlated client connection, TLS, upload-completion and first-header
   timings, plus gateway receipt, authentication decision, queue/routing,
   prefill and first-header records where available, including cancellation or
   late completion and request identifiers.

These distinguish receipt and transport from server-side work; absent them,
the precise stall and cause remain unknown. No new live request, credential
exposure, timeout/model/provider change, executor, state or repair is part of
this diagnosis. The native client and capture paths are the existing boundary.
Complete and Stream, the capture and the hosted versus loopback budget were
reviewed; no new matcher or guard was introduced. Complexity delta: one
diagnostic document, no runtime abstraction or behavior change.

## Verification

Focused declared `go-test` runs, `-count=1 -v`, exited 0: `./pkg/agent`
`TestOpenAIHeaderTimeout` and `TestOpenAIHTTPStatusErrorHasNoTransportSuffix`;
`./pkg/execution` `TestAPIProviderCaptures`; and `./internal/cli`
`TestLoopbackEndpointGetsUnattendedRequestBudget`. They exercise local client
timeout, status-response near miss, capture sanitization and budget selection;
they do not reproduce this incident or verify the supervisor's replay. No
full gate or endpoint probe ran in this pass. Independent review and hosted
checks remain delivery steps, not results claimed here.
