# Configured LUMI header timeouts, 28 October 2026

Status: **not reproduced; cause unknown; no repair or minimum failing request
established.** Signal `sig-8a9440cc6148`. This is a finite diagnosis, not a
provider recovery or delivery of the original chats/factory work.

## Evidence and fidelity

The [metadata extract](evidence/lumi-header-timeouts-2026-10-28.json) was read
from the actual private captures, persisted Console run records and event
files. It contains allowlisted metadata and source paths/file hashes so a
host-authorized reader can compare them without publishing prompts. These
hashes identify capture files, **not original wire bodies**. Private state
root: `/home/perttu/.agent-console`. Capture files are
`run-requests/<full run_id>.json`; run/event locations are in the extract.

All four captures name provider `lumi-qwen38-27b`, model `Qwen/Qwen3.8-27B`,
base URL `https://aitta-api.csc.fi/openai/v1`, POST path `/chat/completions`,
and outcome `failed`. Times below are UTC on **2026-10-28**. Bytes and waits
come from the native error, not the size of the capture file or user input.

| Run prefix | Persisted mode | Request sent | Capture written | Serialized body bytes | Wait / effective client timeout | Captured system/user characters; tools |
| --- | --- | --- | --- | ---: | --- | --- |
| `8a9312c8` | workspace-write | 14:21:07 | 14:23:07 | 39,403 | 120.029s / 120s | 2,026 / 31; 19 |
| `8dfab164` | read-only | 14:24:44 | 14:26:44 | 18,901 | 120.088s / 120s | 2,026 / 4; 3 |
| `f76fd6c8` | read-only | 14:31:41 | 14:33:41 | 18,902 | 120.092s / 120s | 2,026 / 5; 3 |
| `4920a08c` | read-only | 14:51:57 | 14:53:57 | 18,901 | 120.099s / 120s | 2,026 / 4; 3 |

The common error is `context deadline exceeded (Client.Timeout exceeded
while awaiting headers)`, with #167's measured wait, effective timeout and
body-size suffix. Each capture error equals its persisted run error and
`turn.error` text. Each event sequence is exactly `turn.started`,
`base.refreshed`, `turn.error`, `turn.failed`: no recorded assistant output
or tool dispatch. All runs have provenance `owner`. The initial description
of all four as read-only is corrected by the persisted `8a9312c8` mode;
no completed write is inferred.

All four have `truncated=true`, `textLimit=2000`. The 2,026-character system
text includes the cut marker; it is not the original system text.
`pkg/execution/request_capture.go` sanitizes a copy of the last serialized
request, redacts secret-like text, omits images, truncates message strings,
and writes privately (0600). It is not a full-wire archive. The four-character
input in `4920a08c` does not imply a four-character prompt or small wire body.

### Build provenance

This inspection started at supplied `origin/main` and HEAD
`6921931bf4f57d62fe4576a7be6a62349478e825`, containing #167's diagnostics
(`53446c79`). The owner addendum reports that the configured `6921931` binary
was installed/read back in the prior `140308` run. **Exact historical
OpenExec executable identity for each failed chat is unavailable.** Retrieve
it from Console launch/deployment records: resolved executable, build/VCS
revision or digest, config directory/revision, and credential-reference
resolution at launch (reference/provenance only, never its value).

`base.refreshed` records `090ff569af65c52de231aa6a604f199a58c137d4` for the
first three chats and `d791e072d8c350f50fd1b55909e0d3d19cae6c23` for `4920a08c`.
Those are repository refresh records from the Console chats, **not executable
attestations**. Do not assign them to the OpenExec binary.

### Replays and separate signatures

The private supervisor result
`perttu/scratch/20261028T145823Z/lumi-authenticated-replay.json` reports
`reproduced=false`, `requests=1`, `first.outcome=pass`, `seconds=7.4`, with
no reduction attempts or passing causal changes. The supervisor reports the
same endpoint, existing Console-owned configured credential and original
model. Its shape confirms `Qwen/Qwen3.8-27B`, a 2,026-character system message,
four-character user input and three workspace tools. This is a **passing
truncated replay**, not reproduction of the original 18,901-byte request.
The result does not record measured replay wire bytes, effective timeout,
execution timestamp or executable attestation. The scratch directory name
does not supply those missing measurements. `minimalRequest` is a result
field name, not evidence of minimization when reproduction is false.

The earlier `replay-4920a08c4a987f9861466d873eec49c4.json` reports one request,
0.1s, `reproduced=false`, `first.outcome=fail`, class `http-400`, response
`access token missing`. This unauthenticated request is invalid evidence
about the authenticated runtime timeout. It does not show the runtime used
a missing credential.

LOCAL `qwen-27b`'s inspection-only two-hour caller-budget ending is a separate
signature: see [the native budget diagnosis](NATIVE_INSPECTION_ONLY_BUDGET_DIAGNOSIS.md).
Those runs did dispatch inspection tools and ended `inconclusive:budget_exhausted`;
these LUMI runs failed before the first response at the 120s HTTP client
boundary. Neither the LOCAL history nor #167's loopback 15-minute examples
are LUMI reproductions.

## Native boundary and discriminating evidence

`internal/cli/execution_stdio.go` selects the named API entry from
`Request.ConfigDirectory()` (or the envelope directory), not a guessed
worktree config or the active provider. `LoadProjectConfig` reads
`.openexec/config.json`, with the existing `.uaos/project.json` fallback.
`resolveAPIKeyReference` accepts a literal or resolves `$VAR`; an empty
resolution refuses before HTTP. `configuredOpenAIAdapter` retains the named
model/base URL. Hosted endpoints receive `DefaultOpenAITimeout` (120s);
only actual loopback hosts get the existing 15-minute budget. This explains
the configured budget, not why the endpoint failed to answer.

`pkg/agent/openai_provider.go`'s `Complete` and `Stream` each validate, build
and marshal once, remember the serialized body before `Do`, attach the
configured credential in the Authorization header, and measure elapsed
`Do` time. The reported body bytes are the marshaled buffer length. They do
**not** prove upload completion or server receipt. The remembered `sentAt`
precedes `Do` and is formatted to seconds in a capture; it is not a
server-side receipt timestamp. `http.Client.Timeout` includes connection,
upload and header wait; the error does not isolate queueing, inference,
DNS/TLS or a network stall. No new classifier or inferred cause is added.

| Hypothesis | Observation that would discriminate it | What is actually available |
| --- | --- | --- |
| Endpoint congestion, queueing or slow prefill | Correlated gateway receipt/request ID, backend/model routing, queue entry/start, prefill start/end and first-header time, including late/cancelled requests | None of those endpoint records; only client start, full-budget elapsed time and no headers |
| Client construction or transport | Privately retained original serialized request plus effective config/build; transport phase timings for connection, TLS, write completion and first response byte, correlated to server receipt | Serialized byte count, capture shape and client elapsed time; full body and phase timings absent. Serialization reached `Do`; semantic validity and completed upload unproven |
| Missing/invalid credential | Historical config/reference resolution and Console injection provenance, plus correlated HTTP authentication response or gateway auth decision | Runtime reached HTTP; no auth response received. Later configured authenticated replay passed; unauthenticated replay was rejected. Neither proves the original credential's value/status |

Body size and timeout alone discriminate none of these causes. A small
user input or a later fast pass cannot select between them. In particular,
the older [#167 diagnosis](QWEN_HEADER_TIMEOUT_DIAGNOSIS.md)'s suggestion
that wait/budget/size separates endpoint stall from oversize input is not
sufficient causal evidence for this incident.

Readiness is independently bounded: `pkg/agent/readiness.go` performs only
authenticated `GET <baseURL>/models`, under a five-second context, without
following redirects. `pkg/execution/api_provider.go` reports the metadata
state and never substitutes inference when metadata is unsupported.
Catalog presence can establish metadata visibility/authentication for that
request; it cannot establish completion availability, queue latency or
ability to process the original prompt. No historical readiness result is
asserted here.

## Supported replay route and stopping point

The supported **native configured route** is Console's existing OpenExec
API provider process: `execution-stdio --provider api --api-provider
lumi-qwen38-27b`, with the registered configuration directory and existing
Console-owned credential resolver. `internal/providers/openexec.go` in the
authorized sibling Console repository constructs the child environment
and injects that single credential after resolving the config directory.
OpenExec consumes the reference through `newConfiguredAPIProvider`. An
operator shell without that injection is not an equivalent route. A new
native execution must retain the original model, endpoint, effective 120s
timeout and authorized request/tools, and needs existing run/provider
allowance; this diagnosis renews none. Reconstructing from the truncated
capture is explicitly not full-request replay.

The supervisor's retained artifact replay is a separate route. A historical
Console `scripts/api-replay.py` snapshot is inspectable under
`/tmp/claude-1000/perttu-shared-decision-145823/`; it is not present in this
OpenExec checkout or the currently registered Console checkout. It uses
Python `urllib`, JSON reserialization and an explicit private key file;
it does not exercise the native Go HTTP client or automatically resolve
Console credentials. The retained result does not attest the replay script's
exact revision. Do not treat its generic timeout as the native Go
signature or its passing capture replay as exact-wire evidence. The
existing authenticated result is retained evidence, not a new live request
by this pass. No credential is read/copied into this repository, no remote
request is claimed, and no minimization is attempted.

The next precise evidence needed is the endpoint's receipt/auth/queue/
prefill/first-header timeline for these request windows, correlated by
request ID where retained, plus historical launch/config provenance. For
equivalent reproduction, first establish a privately retained full original
request (or that it is unavailable) and its fidelity, then use the supported
configured route within existing authority/allowance. Record actual replay
bytes, timeout, model, endpoint, build and timing. Minimize only after the
equivalent native failure reproduces. If it passes or cannot be replayed
faithfully, record nonreproduction and stop. Do not enable broader tracing
automatically. Existing opt-in `OPENEXEC_REQUEST_TRACE` also redacts/omits
content and reports fidelity flags; it cannot recover these past requests.

## Verification and scope

No runtime source or tests changed: the demonstrated gap is missing external
evidence, not a failing client invariant. There is no failing-before/passing-
after repair test because there is no repair. Existing tests drive the real
client/execution/configuration paths against local HTTP fixtures. The focused
declared `go-test` checks below all returned **`go-test exited 0`**, with
`-count=1 -v`:

| Package / `-run` | Verified boundary |
| --- | --- |
| `./pkg/agent` / `TestOpenAIHeaderTimeout` | Complete and Stream header stalls retain original error/code/retryability and measured wait/budget/size; large-body received size equals diagnostic bytes |
| `./pkg/agent` / `TestOpenAIHTTPStatusErrorHasNoTransportSuffix` | Near miss: actual HTTP status response keeps its separate path; default timeout unchanged |
| `./pkg/execution` / `TestAPIProviderCaptures` | Real failed request is retained privately, redacted and truncated; success/refusal/unset capture do not produce a failure capture; last request at cap is retained |
| `./pkg/execution` / `TestAPIReadiness` | Auth, forbidden, missing model, malformed, unsupported metadata, redirect and cancellation branches; no completion fallback or sensitive response leakage |
| `./internal/cli` / `TestLoopbackEndpointGetsUnattendedRequestBudget` | Hosted/non-loopback hosts keep 120s; actual loopback hosts receive existing budget; lookalike hostname does not |
| `./internal/cli` / `TestAPIProvider` | Named-provider model/config selection, config persistence, unresolved credential and unknown/missing name refusal. `TestAPIProviderLiveOllama` explicitly SKIPPED: separate resource authorization required |

These fixtures verify the existing diagnostics and boundaries, **not the LUMI
outage**. No full local gate ran. Linux/macOS hosted CI and independent exact
candidate review remain publication gates, not claimed results of this pass.

Sibling-path review covered Complete/Stream, stdio named configuration and
pipeline client construction (`internal/pipeline/pipeline.go`, which also
resolves `$VAR` and uses the default client timeout), bounded last-request
capture versus opt-in trace, and metadata readiness versus inference.
No guard/filter/rewrite/fix was introduced on any path. Legacy project
loading and all timeout/error/readiness/privacy behavior are untouched.
Complexity delta: two documentation/evidence artifacts; zero runtime
abstractions, executors, persistent state, authority or continuation changes.
No owner decision is needed to deliver this diagnosis; cause, exact-wire
reproduction and original chat/factory outcomes remain unproven/outstanding.
