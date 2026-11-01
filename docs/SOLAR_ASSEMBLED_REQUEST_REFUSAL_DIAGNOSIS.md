# Solar native assembled-request refusal

Occurrence `5ead407d6eac762228dbf53d77f8a1a3`, 2026-11-01 UTC.
**Diagnosis only: the pre-dispatch boundary is established; root cause is
unproved. No faithful original reconstruction, production repair, provider
replay or Solar recovery is claimed.**

The outcome is a finite explanation of this refusal and one discriminating
offline experiment before any repair or continuation of Solar Goal
`df74566ca77ae43d20cda9dbd0fce60e`. The missing useful setup, operation, release
and takeover documentation and all six Solar Ready conditions remain separate
supervisor validation. A merged diagnosis does not satisfy them.

Native task execution owns this boundary; independent native review and
deterministic publication own delivery. Existing admission, observation,
reservation, compaction, capture and trace primitives suffice. This change adds
documentation and tests only. Complexity delta: zero runtime concepts, stores,
transitions, owner decisions or failure modes; no existing machinery replaced.
No cap, output, model, accounting, Stop, allowance or private-input protection
changes; no Goal Continue/reset or production/DNS/migration/spend effects.

## Evidence ledger

The candidate base and supplied default are
`0f66127a9b7dea99019cc52f66932aa159ffb0a6`. Installed-source comparison is with
`6320b338d68038fc8c0c40934877f71adc71cc1b`. `git diff 6320b338 origin/main --
pkg/agent pkg/execution` contains only `d26_diagnosis_test.go`,
`d26_evidence_test.go` and `testdata/d26-structure.json`; production in those
packages is identical. This source comparison does not attest the process that
ran Solar.

Read-only inspection of `/home/perttu/.local/bin/openexec` reports Go
`go1.26.5`, `ui_dist`, version `0.13.1-226-g6320b338`, embedded commit
`6320b338`, build date `2026-10-31T20:47:32Z`, and SHA256
`9788bead029451de78e9a5a963a5c76f24b8e956bc25c7e5662071581ad99fca`.
This is the installed file observed during diagnosis, not a retrospective
executable hash or effective configuration attestation for the failed process.
The occurrence's exact executable/process identity remains unknown from these
artifacts; the supplied task identifies installed `6320b338`.

All paths below are private and relative to `/home/perttu/.agent-console/`.
Only hashes and protocol metadata are published. No private content was copied
into fixtures. Original files were read and re-read without writes.

| Artifact | SHA256 |
| --- | --- |
| `run-requests/5ead407d6eac762228dbf53d77f8a1a3.json` | `3cabaae5f5000ca593f87358aa504b5ecbb5e5204afb7116bff577f056f210a3` |
| `perttu/scratch/20261101T021850Z/solar-replay-report.json` | `0df07166ae499d637733ba3c8978486985988598b89fc086b2d2571c70651f4f` |
| `perttu/scratch/20261101T021850Z/solar-replay-minimal.json` | `e8fdcbe352b77fed6b36b02c181cf0306c7552d3563a86cf728c40f20b666804` |
| `perttu/scratch/20261101T021850Z/solar-postboot-events.json` | `7d5f1ac07f4973f251997a1c4a146bb216e1c1d278e4f9ae6df826d3638d08ac` |

The event file is the supervisor-retained response of admitted
`/api/runs/5ead407d6eac762228dbf53d77f8a1a3/events`. It contains 164 ordered
events: one start, 80 paired tool starts/completions, one assistant delta, one
error and one failed event. The earlier `solar-events.json` is an empty array
from the queued period; it is not failure evidence. A fresh API read was not
verified in this pass: the sandbox read failed and the host inline Python
read was refused as outside the unattended task's authority. The retained file
was independently read by `TestSolarPreflightPrivateEvidence`, including its
hash, ordering, pairing and terminal signatures; it passed without a skip.

| Observation | What it establishes | Limit |
| --- | --- | --- |
| Supervisor: run began 02:27:05Z after queue wait; event 1 is 02:27:09.710890106Z | Queue time and execution are separate | Queue history is supplied context, not native admission state |
| Capture `sentAt=02:39:14Z`, `capturedAt=02:41:58Z` | A prior source-side `POST /api/chat` transport attempt | Dispatch provenance does not prove server receipt or native usage |
| Capture: 100 messages, 3 tools, model `qwen38-27b-mtp2-128k`, `stream=false`, `truncate=false`, `num_ctx=65536`, `num_predict=21998` | Last-dispatched native request configuration | Does not establish the subsequent refused body's configuration or reservations |
| Capture `truncated=true`, `textLimit=2000` | Message text was reduced to head/tail; redaction can also alter content | Not faithful wire evidence; not the refused assembly |
| Event 163, 02:42:07.879089792Z; event 164, 02:42:12.702226001Z | Both retain the same local refusal below | Later 02:43 Console boot cannot be this refusal's restart interruption |
| One replay request, 37 seconds, first outcome `pass`, `reproduced=false`, `partial=false` | The retained replay passed | Its minimal request and report's minimal request equal the truncated captured request; PASS cannot disprove the original refusal |

The original terminal diagnostic is content-free:

```
context_overflow: assembled request bytes=274654 estimated_tokens=69793 context=65536 dispatched=false
```

This differs from the older dispatched-provider refusal, the reasoning-only
2048-output boundary, and a compiler 45-second timeout. Those diagnoses and
Console no-change reconciliation do not supply this request's missing state.

## Native source trace (same production source at both revisions)

1. [`APIProvider.Execute/execute`](../pkg/execution/api_provider.go) owns a
   run-local `boundedAPIAdapter` for positive `TokenBudget`. It validates the
   explicit context limit and hard-budget capability. `completeChecked`
   validates the conversation before calling the adapter. Tool-loop requests
   carry system, replayed history, current messages and configured tools.
2. [`boundedAPIAdapter.Complete`](../pkg/execution/token_budget.go) picks one
   stable power-of-two context up to `MaxAPIContextTokens` (65536), respecting
   any smaller caller ceiling and remaining grant. It requires room for at
   least 2048 output tokens; output is bounded by context, remaining grant and
   explicit `MaxTokens`. It debits input plus output before preparation.
3. [`CompleteBounded/completeLocalOllama`](../pkg/agent/ollama_budget.go) requires
   native capability, input >=2048 and positive output. It serializes the
   actual native body: system, messages, assistant thinking, bound tool results,
   tool-call arguments as objects, schemas, `stream=false`, `truncate=false`,
   and `options.num_ctx/num_predict`. Unsupported media, malformed arguments
   and unbound results are refused. The pinned loopback protocol is 0.32.13;
   the capture alone does not attest the historical server version.
4. The actual admission caller is
   [`admitObservedContext`](../pkg/agent/context_observation.go), not the
   standalone `admitAssembledContext` helper. Both share the
   [`assembledContextEstimate`](../pkg/agent/context_admission.go) conservative
   whole-body estimate: ceil(ASCII bytes * 5/12) + non-ASCII bytes + 256 +
   32/message + 32/tool. JSON escaping, metadata and schemas count. This is
   neither a native tokenizer nor usage accounting.
5. Observation eligibility requires the same non-message shape (model, tools,
   flags and input options included), strictly more messages and exact
   serialized-message prefix hashes. Only `options.num_predict` is excluded
   from the shape. Eligible observations offer a second estimate: prior native
   input tokens + 256 + each appended serialized message's byte count + 32.
   The smaller estimate is used. Admission refuses when estimate + output
   exceeds input. Same-count requests, edited prefixes and changed schemas or
   input options do not qualify. Compaction can invalidate a prefix; whether
   that happened in Solar is unknown.
6. A preflight refusal returns before HTTP request construction, `remember`
   and `Client.Do`. The limiter can reduce output once per preparation only
   when `input-estimate >=2048` and smaller than the reserved output. It
   restores only that unused output reservation and prepares again. If refusal
   persists, [`compactRepeatedResults`](../pkg/execution/context_recovery.go)
   makes one copy with earlier exact duplicate successful tool outputs of at
   least 1024 bytes replaced by hash references to the last full copy. Unique
   results, tool errors, system/owner text and retained history are preserved.
   It prepares that copy once, with the same bounded output-adjustment rule.
   A final preflight refusal refunds the remaining input/output reservation.
   No dispatched refusal is retried by these paths.
7. On admission, [`remember`](../pkg/agent/openai_provider.go) saves body, time
   and initial transport method/URL immediately before `Client.Do`. An HTTP
   400 with context plus exceed/too-long wording becomes a typed
   `Dispatched=true` overflow. Other statuses/text are near misses. Raw
   provider text is withheld. Failed dispatch leaves usage uncertain.
8. Only a parsed `done=true` response with present nonnegative native input
   and output counts within bounds reaches `observeNativeContext`. A positive
   input count stores shape/message hashes and native tokens on that provider
   instance; zero does not replace the previous observation. There is no
   observation persistence in the last-request capture. The instance may
   outlive an Execute; the immediately preceding request need not be the last
   positive observation. Estimates never become accounting. Native usage
   refunds unused reservation; terminal `EventUsage.UsageFinal` is false when
   usage is uncertain. Final synthesis preserves typed overflow/grant refusal
   instead of replacing it with a round-limit outcome.
9. On failure/inconclusive/cancellation,
   [`captureRequest`](../pkg/execution/request_capture.go) unwraps the limiter
   and saves the most recent remembered request from this run, atomically,
   privately (0600, newly created directories 0700), with bounded/redacted
   message text. It has no refused-body hook: the error can accompany an
   earlier dispatched body. An earlier run's remembered request is excluded by
   start time. Safe dispatch metadata excludes credentials/query/fragment.
10. Opt-in [`runTrace`](../pkg/execution/request_trace.go) records each tool-loop
    request and response/error through `completeChecked`. It attaches wire
    and dispatch only when remembered time belongs to that call. A preflight
    refusal can retain the original loop request without a wire/dispatch;
    it does not retain every limiter preparation or private observation.
    Trace flags disclose redaction, media omission and dropped entries. A
    redacted, incomplete or reconstructed trace is not automatically faithful.

Established discrepancies are the earlier dispatched body versus refused
assembly, truncated replay versus full input, and stateless single replay
versus observation-dependent native admission. Also, under this source the
whole-body estimate for 274654 bytes is at least 114696 tokens before message
and tool overhead, whereas the error reports 69793. Conditional on the
historical process using this source, that is consistent with the observation
branch lowering the estimate. It does not establish prefix contents, observation
tokens, actual native fit, a tokenizer defect or the refusal's cause. The
reported estimate itself already exceeds 65536; changing output alone cannot
make that particular prepared attempt fit. No bound change is proposed.

## One bounded offline reconstruction experiment

**Specification only; stopped at missing prerequisites. No live inference is
authorized.** Reuse native serialization/admission and the limiter/compaction
tests, with existing private trace/capture formats as evidence sources. Do not
add a controller or a new production capture store.

Prerequisite, all tied to this occurrence and provider instance:

- The complete unaltered request that established the last positive native
  observation, its validated native input count and request/response ordering.
- The complete refused loop `agent.Request`, including full system/history,
  schemas, thinking, tool arguments/results and actual flags; plus the exact
  final serialized refused body or independently provable reserialization.
- Original input/output reservations, remaining grant/context ceiling,
  explicit request maximum, and which output-adjustment/compaction preparations
  ran. Needed to associate the final error with the correct prepared body.
- Source/build and endpoint identity sufficient to bind the reconstruction;
  the preceding observation must belong to the same native provider instance.

The four retained artifacts cannot recover the omitted message middles,
redacted strings, intervening full responses/appended results, the refused
body and its full pre-compaction input, exact prefix hashes, last positive
native observation or final output reservation/preparation sequence. Event
tails are not full tool results or per-round native usage. Availability in
other uninspected retention is **unknown**; it is not asserted globally lost.
No complete original trace was established here. Truncation cannot be reversed
by padding, joining event tails, guessing source text or treating a replay as
the original. This missing-input result satisfies the finite experiment's
stop branch now; no future observation is a merge condition for this diagnosis.

If all prerequisites are already retained and fidelity verified, perform one
offline comparison, bounded to one predecessor and one refused call (up to
four native preparations from the existing limiter), zero network requests
and a 120-second wall limit:

1. Hash inputs before reading; verify trace omission/redaction/incomplete flags
   and reserialization hashes. Stop on any unknown/differing prerequisite.
2. Seed the existing provider observation from that retained successful body
   and validated count; pass the exact next request through existing admission
   and bounded limiter/compaction with a transport that refuses any dispatch.
   Keep endpoint identity and all original bounds unchanged.
3. Compare the unobserved whole-body control with observation eligibility and
   selected estimate; record only hashes/counts, eligible/ineligible branch,
   input/output reservations, preparation ordinal, duplicate count and whether
   dispatch would be attempted. This is one paired offline experiment, not a
   second provider replay. Never print request/response/tool content or URLs.
4. Require the final refusal's body hash/length and typed error to agree with
   retained evidence. A divergence identifies the exact missing state or first
   differing preparation; stop there. Re-read hashes to verify preservation.

This can distinguish lost/ineligible observation, an append that remains too
large, output reservation, or compaction effects **in the reconstructed native
admission**. It cannot establish actual provider fit or a model defect. If it
would dispatch, stop offline; do not send it. A later production repair still
requires a faithful same-endpoint failure/pass/removal witness with the
original unchanged bounds and separately authorized effects. None exists here.

## Verification and sibling paths

[`assembled_diagnosis_test.go` (agent)](../pkg/agent/assembled_diagnosis_test.go)
drives the real serializer/admission at context 65536 and output 21998 after
a synthetic native observation. It covers eligible append/output changes,
same-count and edited-prefix near misses, changed model/tools/context,
oversized append, invalid native usage, and unchanged earlier dispatch after
refusal. These are explicitly synthetic controls, not reconstructed Solar text.

[`assembled_diagnosis_test.go` (execution)](../pkg/execution/assembled_diagnosis_test.go)
drives Execute through negotiation, reservation adjustment, near-duplicate
refusal versus exact-duplicate compaction, provider refusal, Stop, actual usage,
and re-opened capture/trace. It checks redaction, file permissions, fidelity
flags, no stale wire provenance, no prior-run capture, and full attempted loop
request persistence. [`solar_preflight_evidence_test.go`](../pkg/execution/solar_preflight_evidence_test.go)
reads the four private artifacts, asserts the chronology/configuration/replay
distinction and re-reads hashes. It explicitly skips on hosts without the
private capture; such a skip supplies no occurrence proof.

Source search found one production observation admission caller and one native
observation writer. Bounded Stream delegates to Complete; final synthesis uses
the same limiter and preserves overflow. Ordinary native Complete uses zero
caps and retains provider `truncate=false`; compatible Complete/Stream and
unsupported native choices/media retain their existing routes. Tool-less
execution bypasses completeChecked's trace hook. Capture/trace share the
dispatch snapshot and privacy transformations. No runtime matcher, guard or
rewrite was added on any route. Existing native/compatible dispatch, capture
fidelity, cancellation, native usage, grant and synthesis tests supply sibling
coverage; no new provider-shape support is claimed.

Verification commands/results and the exact candidate's independent review and
native publication provenance belong in the delivery report. Full declared
gates and hosted results are not inferred from their workflow definitions.
No source repair is deferred to a future Solar observation.
