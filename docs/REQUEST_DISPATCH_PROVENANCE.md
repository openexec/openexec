# Request dispatch provenance

Future failed/inconclusive/cancelled API diagnostics can retain an optional
`dispatch` object beside the corresponding serialized body:

```json
{
  "method": "POST",
  "url": "http://127.0.0.1:12345/api/chat",
  "provenance": "source-known-transport-attempt"
}
```

This example is synthetic metadata, not an endpoint to contact. A capture has
`dispatch` beside `request`. A trace has `entries[i].dispatch` beside that
entry's `wire`; there is no run-wide target to apply to earlier rounds.
The envelopes remain version 1. `baseURL` remains the configured base, subject
to diagnostic privacy filtering, and is not dispatch evidence.

`OpenAIProvider.LastRequest()` keeps its existing signature and configured-base
meaning. The optional `LastRequestWithDispatch()` accessor returns the body,
time, configured base and dispatch in one locked snapshot, with copies for
callers. Capture and trace use this snapshot through the existing bounded-adapter
unwrapping. Older adapters remain supported without dispatch evidence.

The source records the actual HTTP request's method and initial URL immediately
before the existing `Client.Do` call. Native ordinary/bounded completion uses
`/api/chat`; compatibility completion and streaming use their existing
`/chat/completions` construction, including forced-choice, media and
unnegotiated fallbacks. Serialization, token admission, HTTP timeouts,
cancellation, response limits and redirect policy are unchanged. This records
a **transport attempt**, not server receipt, inference, successful execution or
any redirected destination. A request refused before reaching `Do` contributes
no fresh body/target snapshot. The existing run/round timestamps exclude old
snapshots from new captures and trace entries.

Before persistence, dispatch evidence must carry the exact recognized provenance,
POST method and an absolute HTTP(S) URL with a host. Missing, malformed or
privacy-altered evidence is omitted and therefore unknown. Userinfo, any query
(including unknown parameter names), fragments, and secret-shaped strings in
URLs or decoded paths make the entire dispatch unavailable. Do not sanitize a
URL and then attest it as the actual target. The configured base's userinfo,
query and fragment are removed independently; existing secret redaction still
applies. The raw accessor, like the raw remembered body, is private diagnostic
input and must not be published or logged.

## Downstream Agent Console consumption contract

The smallest separate replayer change is to read the capture's `dispatch` or the
specific trace entry's `dispatch`, validate its provenance/method/absolute URL
and privacy constraints, and use that exact initial target with its matching
body. Apply the replayer's existing endpoint authority/credential policy too:
source provenance is not authority to send to an arbitrary host. Do not append
`/chat/completions`, infer a route from argument types, stringify native objects,
or fall back to another schema. Absent, malformed or privacy-altered evidence
must report target unavailable or refuse automatic replay before sending.
Preserve and consult all existing truncation/redaction/omission/incomplete and
dropped-entry flags; a known target does not restore lost content. Captures
can be redacted even when `truncated` is false, and trace JSON trees are not
raw byte or header witnesses.

The supervisor pins its current Console replayer at SHA256
`544bad733f239a063d7f1871e2175fd495ead1d194d3e19d141fdc6dda5b9195`
and reports that `Endpoint.__init__` appends `/chat/completions`. That is supplied
evidence, not an inspected or modified foreign source in this change. Console
implementation belongs to a separate repository task.

## Evidence and limits

The declared Go suite includes production serializer/local HTTP controls in
`TestCaptureFidelitySerializerDispatchAndReplay`, real two-round failed
`APIProvider.Execute` capture/trace round trips (native, bounded and compatible)
in `TestDispatchProvenanceExecuteRoundTrip`, mixed-route per-round trace
association, malformed/legacy/privacy/stale controls and refused-before-attempt
controls. They use public synthetic inputs and local `httptest` servers only.
The sibling-path audit covers all three remember/send sites, ordinary and bounded
native completion, compatibility completion/streaming, forced-choice/media/
unnegotiated fallbacks, bounded-adapter unwrapping and both diagnostic writers.
CLI configuration delegates to these providers. The existing offline trace
replay fixture compares requests and returns recorded responses without HTTP;
the checkpoint `replay` CLI and command/session evidence record different
artifacts. Other provider adapters do not expose this remembered-body interface.
None requires a parallel routing or capture implementation for this correction.
The removal control uses a disposable Go source overlay to remove the real
provider provenance assignment. Dispatch and persistence tests must fail with
`dispatch provenance mismatch`; the same selection with restored source must
pass. It executes inside the declared Go test suite, without altering the tree.
Existing capture/trace/cancellation tests continue to exercise privacy,
truncation, omission, private file permissions and dropped-entry limits.

Legacy fixtures and prior records are not rewritten. The truncated historical
c6e capture has no target/wire receipt; these fields cannot reconstruct its
route, cancellation cause, earlier rounds or original useful execution. The
live measurement allowance is spent. No live endpoint measurement, original
retry, installation or deployment belongs to this task. A supervisor may inspect
a naturally failing future capture after separately authorized normal activation.
This diagnostic correction earns no original product recovery credit.

Complexity delta: one optional diagnostic evidence object (`dispatch` with
method/URL/provenance), one compatible additive snapshot accessor, zero new
execution loops, controllers, transitions, owner decisions or authority. Existing
remember/capture/trace machinery is reused; no execution machinery is replaced.
