# Dispatched Qwen context overflow diagnosis

`sig-32277734e7dd`, occurrence `d26bd137975612543c90e5b983f60db8`,
2026-10-31. **NOT REPRODUCED; causal correction unknown.** The retained run
shows inspection without authoring, followed by a native context refusal.
Neither the estimated token count nor dispatch metadata establishes why the
endpoint refused. No runtime repair, replay, inference, model change or resource
policy change is part of this diagnosis.

The outcome remains the original metrics source/UI work passing its retained
proof. Native request assembly, admission and continuation own this boundary.
Existing native task/plan/review, request capture and private request tracing
are sufficient for the next diagnostic step. No controller, worker, store,
grant or new authority is needed. Architecture complexity delta: zero concepts,
persistent state, transitions, owner decisions or runtime failure modes added;
no existing machinery replaced.

## Evidence identity and limits

This candidate starts at supplied `origin/main`
`6320b338d68038fc8c0c40934877f71adc71cc1b`. Triage inspected
`78aa37f682e3b794822ec8ba92dde0a31f0a4809`; that is earlier evidence, not the
candidate base. `git diff 78aa37f HEAD --` for the native files linked below
is empty. These source identities do not attest the historical process.

`go version -m /home/perttu/.local/bin/openexec` now reports build commit
`6320b338`, version `0.13.1-226-g6320b338`, build date
`2026-10-31T20:47:32Z`, Go `go1.26.5`, `ui_dist`. Its SHA256 is
`9788bead029451de78e9a5a963a5c76f24b8e956bc25c7e5662071581ad99fca`.
Triage recorded installed `78aa37f`, built `08:51:13Z` that day. The installed
file has since changed. **The executable path, hash, source revision and
configuration used by d26 are unavailable in the inspected occurrence
artifacts.** Current installation is not retrospective attestation.

Private sources were read locally without copying their content into the
repository. SHA256 identities:

| Private artifact | SHA256 |
| --- | --- |
| `perttu/scratch/20261031T163533Z/d26-events.json` | `b089c85d67c841d464a7e3a1f0eb199c1b9472682ddb75c683d4f86768260014` |
| `run-requests/d26bd137975612543c90e5b983f60db8.json` | `0cbc9da8b5a6ed7d03db2b4a0a8ac16818f1c648f3db776090c83651bba20708` |
| `perttu/scratch/20261031T163533Z/d26-replay-private.json` | `c2d828eab3d813d886c046bf48df7cec101f125383af256c0b20742c65f7544b` |
| `perttu/scratch/20261031T163533Z/f081bc00498e1bc6f7ee5b63a0282418.json` | `037e3901d4000c2513c6bf2af75237f6da0541f662ecf5debb89bfe128924bb8` |
| `/mnt/data1/projects/owner-model/.jobs/perttu-replay-d26-163533.json` | `56d14354c458983b9f6a1dea94256a618a917f3da8ee2a6b26a6f2a1c2064d25` |

The first four paths are relative to `/home/perttu/.agent-console/`. The public
[event projection](../pkg/execution/testdata/d26-structure.json) retains all
227 sequence numbers, times, event kinds, tool names, paired call ordinals,
command executables and output-tail byte lengths. It contains no prompt,
arguments, tool text, source text, endpoint address or credentials. It is not a
request fixture. `TestD26PrivateOccurrenceEvidence` independently re-reads the
originals and compares every projected event. On hosts without these private
files that test explicitly skips; a public fixture alone cannot attest its
historical origin.

## Input and chronological inspection

The saved original task identifies f081, provider `qwen-27b`, implementation
run d26, blocked status and PR
[agent-console #1682](https://github.com/openexec/agent-console/pull/1682).
Its description (3858 characters), triage contract (31564 characters) and
23 addenda are retained in the saved JSON. They declare the
full metrics outcome and source-first obligations, rather than a new backend
task. The supervisor also attests the retained instructions and executed four
RED ingress cases were available. The final wire capture has one system and
one user message, both cut to head/tail with omission markers; the user head/tail
identifies f081. **The exact full original dispatched input, declaration
assembly and first-request tool advertisement cannot be reconstructed from
that capture.** The task registry is declaration evidence, not proof of exact
bytes sent. Repeating instructions is not a demonstrated correction.

Times below are UTC on October 31. The event projection supplies every event;
these partitions summarize the progression without exposing captured text.

| Event range | Time range | Started tools | Retained output-tail bytes | Cumulative bytes |
| --- | --- | --- | ---: | ---: |
| 1–41 | 15:43:13–15:45:39 | 8 command, 4 search, 3 inspect, 3 read, 1 list | 13743 | 13743 |
| 42–119 | 15:46:23–15:52:56 | 31 command, 5 read | 36632 | 50375 |
| 120–187 | 15:55:30–16:02:06 | 17 command, 12 read, 1 route judgment, 1 search | 21127 | 71502 |
| 188–225 | 16:05:16–16:17:47 | 16 command, 2 read | 20558 | 92060 |
| 226–227 | 16:17:52–16:18:00 | error then failed | — | — |

Event 1 is `turn.started`; event 2 is `base.refreshed`. Overall there are
104 starts and 104 matching completions, 15 assistant deltas (1614 retained
characters), and one each of start, base refresh, error and failed. Calls are
72 `run_command`, 22 `read_file`, 5 `search_files`, 3 `inspect_repository`,
1 `list_directory`, 1 `record_route_judgment`. Command executables are 40
`grep`, 22 `sed`, 4 `cat`, 3 `git` (log/diff/show), 2 `ls`, 1 `wc`.
There is no recorded production edit, authoring call or executed UI regression
in this run. The saved four REDs are prior witnesses, not d26 execution.

Inspection repeatedly revisited metrics UI, tests and schema/backend sources:
`MetricsPage.tsx` and `OutcomeMeasures.tsx` each appear three times among
explicit file operands, `MetricsPage.test.tsx` four times and
`TaskReliabilityIngress.test.tsx` twice. Reads later broadened across chart,
report, routing and component sources without a write. This establishes
inspection nonconvergence as observed behavior; it does not establish a model
defect, absent write authority, or the cause of the endpoint refusal. The final
22-tool advertisement includes `write_file` and `run_declared_check`; advertised
capability is not proof of effective permission on an earlier round.

32 completion tails have at least 2000 characters. All 22 read-file event
tails are the same short representation; they cannot reveal the actual file
result content. The final capture retains 104 tool messages, 58 assistant,
one system and one user (164 total). Retained content bytes by role are
130039 tool, 1614 assistant, 2030 system and 2030 user. These are transformed
content lengths, not request sizes or native token counts. The 92060 event
tail bytes are also not full result growth. Per-round serialized sizes, exact
prefixes, duplicate-result eligibility and the first divergence from intended
authoring remain unavailable.

At 16:17:47 the retained dispatch records `POST /api/chat`, provenance
`source-known-transport-attempt`. Capture time is 16:17:51. Events 226 and 227
both retain the exact terminal diagnostic:

```
context_overflow: assembled request bytes=693918 estimated_tokens=296242 context=0 dispatched=true
```

The capture redacts the token-labeled estimate via the existing secret pattern;
the events retain it. The captured model is `qwen38-27b-mtp2-128k`, with `stream=false`,
`truncate=false`, 22 function schemas and an empty `options` object. There is
no explicit completion `num_predict` or context `num_ctx` in that request.
The current loop defaults to 160 tool rounds when no configured maximum is
provided. A round can contain multiple calls; 104 calls and 58 retained
assistant messages do not identify the occurrence's configured round maximum.
The endpoint's effective context size, default output allowance, model/template
revision, tokenizer behavior, load state and original raw refusal body/status
are not retained. The model's name does not attest its effective limits.

## Native source boundaries

References below apply to both inspected revisions, not an attested d26 binary.

| Path and functions | What the source establishes |
| --- | --- |
| [CLI assembly](../internal/cli/execution_stdio.go), `newConfiguredAPIProvider` | Loads configured API entry, resolves credential, negotiates local Ollama, creates workspace/gateway/composite tools and `APIProvider`, using configured max turns. No occurrence-specific resolved configuration is available here. |
| [API loop](../pkg/execution/api_provider.go), `execute`, `replayMessages`, `completeChecked` | Builds history plus user query, retains system and schemas, appends assistant responses and each bound tool result on successive calls. The transport conversation validator precedes `Complete`; trace wraps that actual call. Positive `TokenBudget` alone installs the run-local bounded adapter. |
| [Provider selection](../pkg/agent/openai_provider.go), `Complete` | Negotiated text-only requests with auto/none/empty tool choice use native transport with caps `(0,0)` for ordinary calls. Other formats/choices use the compatible route. |
| [Native serializer](../pkg/agent/ollama_budget.go), `completeLocalOllama` | Serializes system, messages, tool names, object arguments and schemas; `truncate=false`. Zero caps retain configured endpoint defaults. Only positive input caps invoke observed-context admission. Empty generation options do not establish a zero output budget. |
| [Admission](../pkg/agent/context_observation.go), `admitObservedContext`; [error](../pkg/agent/context_admission.go), `ContextOverflowError` | Positive-cap preflight uses a padded whole-request estimate or a successful native prefix observation. A local refusal has `dispatched=false`. The error formatter's `context` is the caller-supplied cap, not an endpoint capacity measurement. |
| [Provider refusal](../pkg/agent/ollama_budget.go), non-200 branch | Reads at most 8192 response bytes; HTTP 400 with case-insensitive `context` and either `exceed` or `too long` becomes typed overflow with `Dispatched=true`. Other HTTP/transport failures keep different classifications. Raw refusal text is discarded for privacy. |
| [Bounded continuation](../pkg/execution/token_budget.go), `boundedAPIAdapter.Complete`; [compaction](../pkg/execution/context_recovery.go) | Only non-dispatched overflow can adjust reply reservation and remove exact repeated successful results of at least 1024 bytes, preserving bound history and authority. Dispatched errors remain uncertain usage; they are not automatically compacted/retried. Ordinary zero-grant calls do not enter this adapter. |
| [Failure capture](../pkg/execution/request_capture.go); [private tracing](../pkg/execution/request_trace.go) | Capture is a bounded sanitized final-request tree. Trace can retain all loop requests/responses/events and sanitized remembered native wire trees, with fidelity flags. Both are best effort, not endpoint receipts or raw-byte witnesses. |

`context=0` is therefore compatible with ordinary native dispatch; it does
not mean zero endpoint capacity. Given this source, the error type additionally
implies a matching provider HTTP 400, unlike dispatch provenance by itself,
which only records an attempted transport. Historical attribution remains
conditional on the missing executable attestation. The 296242 estimate is
computed locally from serialized bytes; it is neither provider usage nor a
proof of actual token overflow. No raw server evidence resolves whether the
provider correctly rejected capacity, applied a template/limit unexpectedly,
or classified another condition using those words.

Sibling-route audit: the two native callers are ordinary `Complete` and
`CompleteBounded`; both share serialization/refusal classification. Compatible
`Complete`/`Stream`, media and forced tool choice fallbacks are distinct paths,
covered by existing unbounded/fidelity tests. No-tools streaming, empty-response
recovery and final synthesis are different API-loop branches; existing trace
and synthesis tests cover their boundaries. `admitAssembledContext` is a
baseline helper used by tests; the live bounded serializer uses
`admitObservedContext`. No guard/filter/rewrite was added by this diagnosis.

The supervisor's Solar observation is disconfirming adjacent evidence:
`1ebca0e1…` ended with 261272 bytes, estimated 113570, context 65536,
**dispatched=false**; 151 events, 72 inspection calls, no authoring. Its one
truncated-capture replay also timed out differently. These supplied facts are
not a shared root cause or a second target. Selector aa8, header-timeout 91f
and compiler replay 6f remain separate delivered/diagnostic work.

## Reproduction finding

The original capture explicitly has `truncated=true`, `textLimit=2000`.
Native argument objects and tool protocol structure survive, but text and
argument leaves can be missing or redacted; images can be omitted. There is
no full per-round wire trace among the supplied occurrence artifacts. A
faithful privacy-safe minimal endpoint reproducer cannot be obtained from
these records. Publishing synthetic text with the same counts would not
restore missing content or establish the same failure.

The existing replay wrapper invoked the same captured endpoint/dispatch shape
once with `--timeout 60 --budget 1`. Its durable unit is
`ac-perttu-replay-d26-163533.service`. Receipt: one attempt, finished; replay
summary: exit 2, elapsed 60.2 s. Private result: one request, 60.1 s,
`reproduced=false`, `partial=false`, first signature class `timeout`, no
minimization attempts or passing changes. `partial=false` describes the
minimizer's execution, not capture completeness. **NOT REPRODUCED.** No smallest
passing change or causal recovery exists. Neither this replay nor Solar's was
rerun in this pass.

## Bounded next native diagnostic plan

Use the existing diagnostic task and normal independent plan review. This is
a concrete dependency/continuation proposal, not another implementation
admission or a new product task:

1. Recover any already-retained full occurrence trace, process launch record,
   resolved stdio request/configuration and endpoint refusal/config logs. Bind
   run ID, executable path/hash/build revision and endpoint model/template
   identity to that occurrence. If absent, record each as unavailable rather
   than substituting current defaults.
2. Arrange existing `OPENEXEC_REQUEST_TRACE` and `OPENEXEC_REQUEST_CAPTURE`
   **in the actual native child's environment**, with unique private paths
   (0700 directory/0600 files), for one separately lawful diagnostic execution.
   Do not admit f081 again merely to acquire it. In the currently inspected
   Console `internal/providers/openexec.go`, `call` resolves the executable,
   runs `execution-stdio --provider api --api-provider ...`, and uses
   `safeEnv` from `internal/providers/process.go`. That allowlist excludes
   both trace variables. Exporting them in a parent shell is insufficient.
   Console launcher coordination or any code change needed to arrange scoped
   tracing is a separate dependency, not an OpenExec controller or selector
   change. No such dependency is executed here. Preserve original Stop,
   cumulative allowance and authority; absent a lawful diagnostic allocation,
   acquire archived evidence only and report the dependency unresolved.
3. Before interpreting a trace, require terminal refused request present,
   matching method/path/model/options/tool protocol, no dropped entries,
   `incomplete=false`, `imagesOmitted=false`, and assess
   `redactionChangedContent`. Retain immutable originals privately. Even clean
   JSON trees do not attest wire byte order, headers or server processing.
   Recover the original response status/class and effective endpoint limits
   through existing server instrumentation; trace alone cannot retain the raw
   refusal that the adapter discards. Read structural rounds using existing
   `TestTraceStructureReader`; its offline recorded-response replay is not
   endpoint inference or causal reproduction.
4. Only if faithful evidence and existing diagnostic authority permit it,
   make one baseline endpoint reproduction preserving native shape, options,
   model, limits and protocol. Stop on a different signature; do not repeat
   either already nonreproducing truncated replay. If the same refusal persists,
   reduce one field at a time, preserving call/result validity and retaining a
   reduction only while the same signature persists. Record each change and
   the smallest demonstrated passing delta. Keep all owner/tool text private;
   validate any public surrogate independently against the same failure.
5. If causality is then established, propose the smallest native correction
   through task-plan review. Require the **same meaningful failure** red on
   baseline, green after correction, and red with correction removed, through
   the real assembly/endpoint path. Preserve Stop, cumulative accounting,
   context/output caps, provider/model, authority and tool-history integrity.
   Until then recommend further diagnosis, not a guessed repair. A mocked HTTP
   400, a smaller unrelated prompt or a passing source guard is not recovery.

For eventual lawful SAME-task f081 continuation, the materially changed
execution must be identified and proved before another metrics implementation:
a causally reviewed native correction or other demonstrated execution change,
with source authoring and the retained four ingress REDs actually exercised.
Repeated instructions, trace enablement alone, a new queue entry, or this
diagnosis are insufficient. No fifth pass, allowance reset or duplicate task
is proposed. Capacity limits stay in force.

When that prerequisite is met within existing authority, keep original PR
#1682 and candidate `afd0bfd805c09036beda141b489680f5cf1378a9`, delivered backend
bd2, original hard allowance/Stop/accounting, retained criteria 3–5 and missed
deadlines. The source pickup remains `taskReliability` props in BOTH
`MetricsPage`/`DeferredOutcomeMeasures` and `OutcomeMeasures`, then the four
saved `TaskReliabilityIngress.test.tsx` witnesses (`firstTimeSuccess`,
`firstCandidatePass`, `autonomousConvergence`, `completion`). The full original
UI/source/schema/filter/window/group/count links/definitions/unknown,
three-viewport, negative, build/lint/full CI, independent exact-head review,
normal train, checked served path and reload obligations remain owed. Backend
work is not reopened. No implementation pickup is filed or retitled here.

## Verification and acceptance

`TestD26DiagnosticNativeAssemblyAndRefusalBoundary` drives production
`APIProvider.Execute` through negotiated native HTTP, three large identical
tool results, provider refusal and persisted capture/trace. It verifies empty
options, no truncation, retained system/query/schema/tool history, context 0,
no dispatched retry, and private diagnostics. Its overflow trace also exposes
`redactionChangedContent=true`: the existing secret pattern matches the
failure event's token-labeled estimate, even with public wire content. A true
flag therefore requires inspecting which fields changed, rather than assuming
the model input was altered. Near misses are a different
HTTP 400 body and the same context words at HTTP 503; a successful response
is a separate protocol control. These tests explain the current mechanism,
not the historical endpoint failure or its correction.

`TestD26StructuralChronology` checks the complete public event projection.
`TestD26PrivateOccurrenceEvidence` re-reads originals, compares the projection,
checks the terminal diagnostic, capture shape/fidelity and replay result.
Relevant existing tests additionally exercise bounded preflight, native
default/options/fallback/cancellation, compaction eligibility, final-synthesis
accounting, and trace redaction, omitted images and dropped entries. Commands
and actual results belong in this pass's PR description. Full repository
gates belong to the lane/hosted workflow and are not run by this pass.

| Criterion | Evidence and disposition |
| --- | --- |
| 1 | Diagnosis reconstructs all 227 structural events and input/provenance limits; private-evidence and chronology tests reproduce counts and exact signature. Source paths distinguish nonconvergence, preflight and provider classification. Occurrence binary, full input and endpoint limits explicitly unavailable. |
| 2 | Private-evidence test confirms truncated capture and different replay timeout/exit 2. No minimum/passing change claimed; bounded acquisition procedure above names missing evidence and launcher dependency. |
| 3 | No runtime correction proposed or implemented. The next diagnostic plan preserves all resource/authority/history constraints and makes a same-failure red/green/removal regression prerequisite to correction. Public protocol controls are explicitly not that regression. |
| 4 | Finite diagnosis and next plan above are ready for normal independent review; original f081 remains blocked with same PR and all original obligations. This document cannot demonstrate original recovery. |

Post-deploy effectiveness remains supervisor-only: after a causally proved,
normally delivered correction, five clean same-kind Qwen runs or 24 hours
without recurrence. No such repair exists yet. Independent review and all
original implementation/delivery evidence remain required; diagnosis delivery
alone changes none of those states.
