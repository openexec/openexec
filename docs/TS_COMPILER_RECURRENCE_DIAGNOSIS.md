# Compiler deadline recurrence after PR183

Status: finite diagnosis, cause unresolved. No compiler repair or recovery claim.
This concerns Console PR1693, run 41258644434 attempt 1, quality(go-1) job
124707397274. Historical repair task6f4d3ed7/PR183 is separate completed work.

The outcome is useful implementation with honest checked delivery. The observed
failure belongs to the existing OpenExec compiler boundary. Reuse its private
capture, native replay test, independent review and deterministic delivery.
Neither another execution loop nor persistent runtime state is needed.

## What the retained evidence establishes

Read-only verification against the supervisor-retained original checked:

- The executable's actual SHA-256 equals the captured executable identity.
  Its build metadata names `6320b338d68038fc8c0c40934877f71adc71cc1b`
  (PR183). More strongly, the executable contains the **entire captured
  extractor byte sequence**. That sequence equals both the Git object at
  `6320b338:internal/knowledge/ts_compiler_extractor.mjs` and this candidate's
  extractor. It includes `output.diagnostics.length < 20 ?
  ts.getPreEmitDiagnostics(program) : []`. This is not an inference from PATH,
  a module pin, current checkout HEAD or a later successful retry.
- Original request/script hashes equal their child/identity receipt fields.
  Request, script, identity and result file hashes were read and checked without
  rewriting any artifact. Their full hashes, original paths, filenames and
  request remain private in the supervisor evidence named by the task contract.
- The natural child started at 01:13:09.408626068 UTC on 2026-11-01 and ended
  after **45.451508715 s**: `extractor_deadline`, child error
  `context deadline exceeded`, parent error `none`, bound `45s`, exit -1,
  `signal: killed`. User CPU was 29.885926 s and system CPU 0.876571 s.
  Stdout and stderr were both zero bytes; response decoding was false.
  The request was 19,596 bytes for 471 files (4,070,610 source bytes).
- Node v22.23.4 and TypeScript 5.8.3 hashes still equal the captured hashes.
  The retained load/pressure snapshots and approximately 30.76 CPU seconds
  correlate with this failure; they do not identify a cause. Wall time minus
  CPU time is not a measurement of scheduler delay alone.
- The replay receipt identifies the exact `6320b338` driver, exit 1, completed
  at 01:55:59.469819 UTC. Its log says
  `source/configuration/toolchain changed; refusing misleading replay`.
  That branch precedes both reservation of `replay.json` and child launch.
  **No captured-request replay ran.** This is neither nonreproduction nor cure.
- The train separately reported 2,511/2,511 server tests, then impact replay
  reported 9/15 matched, three missing and three unavailable. All three
  unavailable expectations were classified bound-unavailable. Its limitations
  include the 400-edge adjacency, 200-node propagation/related-evidence and
  262,144-byte response bounds. Those response losses are not the compiler's
  45-second deadline and do not prove that all six losses share one cause.
  The final replay message was `impact replay has 3 expectation(s) hidden by
  response bounds; narrow the corpus anchors`. This diagnosis does not adopt
  that message as permission to change anchors or bounds.

The older successful native/direct runs in
[the cancellation evidence](TYPESCRIPT_COMPILER_CANCELLATION_EVIDENCE.md) used
different inputs/toolchains and are historical nonreproductions. PR183's
synthetic real-compiler regression proves its particular later-group diagnostic
discrepancy. Neither establishes the cause of this new guarded-binary deadline.

## Replay comparison and faithful recovery limits

The old admission condition in `replayPrivateCompilerCapture` used one OR over
full `ScanManifest` equality and four toolchain comparisons. The failed
comparison set was not retained. The worker reflog shows captured source
`b95e58c74edc5d7126dcfd354e6a940f7acbc128` moving to
`91030de24991c65c26ad4645724bfefe783b70cd` at **01:47:27 UTC**, before refusal.
The read-only current contract-file comparison differs from its captured hash.
No new manifest scan or compiler invocation was made on that worker.

| Comparison in the old rule | Established evidence | Limit |
| --- | --- | --- |
| `ScanManifest.Inputs` | Captured contract input has different bytes now; captured configuration input `go.work.sum` is now absent | Reflog records HEAD, not every uncommitted file at 01:55 |
| `ManifestHash` | `finishScanManifest` hashes input path/kind/size/content/symlink target; current input drift invalidates equivalence | No retained per-field replay result or fresh worker manifest |
| `WorktreeStateHash` | `finishScanManifest` sets it to the manifest hash | Same historical limit |
| `ConfigurationDigest` | Includes contract/configuration/test-manifest inputs; contract drift and missing configuration invalidate equivalence | Same historical limit; this digest is broader than TypeScript options |
| identity Node hash vs result Node hash | Retained values equal | Does not record what file was read by the replay at 01:55 |
| identity TypeScript hash vs result TypeScript hash | Retained values equal | Same historical limit |
| current Node file hash vs result Node hash | Read-only file hash equals retained value now | Current equality is not historical equality |
| current TypeScript file hash vs result TypeScript hash | Read-only file hash equals retained value now | The compiler module hash does not cover installed declarations/dependencies |

Thus source/configuration drift is established, and the source-manifest arm
explains a refusal on the observed current state. The complete mismatch set at
the original refusal remains **unknown**. This pass splits that same test-only
admission rule into named comparisons; it cannot retroactively recover results.
It neither weakens admission nor retries/reserves the original capture.

Historical Git-object comparison (no checkout, scan or generated files):
all **471 requested source files** match captured size/content hashes exactly.
Of **2,081 manifest inputs**, 2,080 match and none has mismatching bytes.
The remaining input is configuration `go.work.sum`, absent from that commit
and current worker. The exact request bytes/order/exclusions and extractor are
retained. Source reconstruction is therefore possible, but claiming full replay
equivalence is premature: missing configuration, installed dependency contents,
external/inherited config resolution, default libraries, generated declarations
and other compiler-read context are not all authenticated by this manifest.
`node_modules` and hidden generated directories are excluded from its scan;
`configuredDeclarations` can still admit generated declarations within root.

The precise remaining supervisor evidence request is the original missing
configuration bytes matching its retained digest, any already retained
dependency/generated-context identities, and any per-comparison log retained
at refusal. Do not rescan the busy worker to manufacture historical evidence.
If those records do not exist, mark equivalence unknown; an exact historical
replay cannot be honestly claimed. Do not delete or retry `replay.json`.

## Source phase map and changed diagnostic strategy

`extractTypeScriptWithCompiler` in `internal/knowledge/ts_compiler.go` starts the
45-second child only after script/capture setup. Its observer measures the
whole `cmd.Run`; `childMeasurement` decodes counts only after successful exit.
Hash/version probes and process snapshots are outside the measured child or
after `Wait`. The retained records contain **no phase timings**.

Within `internal/knowledge/ts_compiler_extractor.mjs`:

| Phase | Real source boundary | What PR183 does |
| --- | --- | --- |
| Bootstrap | Read request, require TypeScript, canonicalize root | No guard |
| Configuration/grouping | `configForFile`, cached `parseConfig`, `configuredDeclarations`, population of `groups` | No guard; configuration diagnostics accumulate before groups run |
| Program construction | `ts.createProgram` for every group, including admitted declarations | No guard |
| Checker construction | `program.getTypeChecker()` for every group | No guard; TypeScript may also do lazy work in later checker calls |
| Diagnostics | Guarded `ts.getPreEmitDiagnostics(program)`, then filtering/flattening | Skips a later group's diagnostics only when accumulated output diagnostics already number at least 20; a single admitted call is not interrupted at 20 |
| Extraction/reference traversal | Source-file loop, module resolution, symbol extraction, `visit`, `recordReference`, alias resolution | All groups still traversed with real checker |
| Serialization/output | Sort files, `JSON.stringify(output)`, sole final stdout write | No guard; no earlier stdout progress |

Zero stdout therefore cannot locate the stall. A zero decoded diagnostic count
on failure is the Go record's default, not evidence that the guard's budget
was empty or full. A costly first eligible diagnostic group, repeated program
construction, lazy checker work, config discovery, traversal and serialization
remain distinct hypotheses. Do not choose among them from pressure alone.

The smallest next experiment is **one separately admitted instrumented child**
through the existing private native harness, not an unchanged replay:

1. First establish admitted source/configuration/toolchain/context identity in
   a disposable supervisor-owned reconstruction. If incomplete, stop with the
   exact missing comparison; do not run a substitute as the original capture.
2. Use exact retained stdin and ordering, original Node/TypeScript and options,
   the same 45-second child bound, and no new scan, retry, provider or corpus
   change. Extend only the harness's transient script/measurement, not the
   production producer or Console wiring. Preserve the original artifacts and
   use a separate newly admitted private experiment destination.
3. Emit synchronous metadata at entry/exit of bootstrap, configuration/grouping,
   program creation, checker creation, diagnostics, traversal and serialization.
   Use numeric group ordinals (not paths), fixed phase/event names, monotonic
   elapsed nanoseconds, cumulative `process.cpuUsage()` user/system microseconds
   and diagnostic-budget count. For diagnostics also record whether the guard
   selected or skipped the call. No per-symbol/per-file events, source bodies,
   diagnostic bodies, filenames, environment or compiler output enter telemetry.
   Retain only these metadata records privately through the harness; stdout
   remains the original compiler response. Entry records survive a killed child;
   a missing exit identifies unfinished work without requiring successful JSON.
4. Label the changed script hash, timing/write overhead, reconstructed context,
   script mtime/path and cache/load differences. Save the classified child result
   and completed phase deltas even on deadline. If it succeeds, report only a
   nonreproduction and measured phase distribution, never a repair.

The last entered phase/group, its budget count, and CPU/wall deltas discriminate
work before diagnostics, one eligible diagnostic call, work after budget fill,
and output construction. If no bootstrap event appears, runtime startup remains
outside the instrumented interval. This experiment can localize work, not by
itself prove that pressure caused the historical failure. It is specified here,
**not executed or installed**, pending faithful context and a new admission.

## Verification, sibling routes and delivery limits

`TestCompilerPrivateCaptureReplayComparisonDetails` invokes the real native test
executable entry point on synthetic captures. It changes every manifest field,
both retained toolchain comparisons, both actual toolchain files, source and
configuration bytes. Each must report exactly the failed comparisons without
private values, with no `replay.json` or child-launch marker. The near miss
changes mtime with identical bytes and must reach the child and decode output.
`TestCompilerPrivateCaptureReplayRefusals` retains request/record refusal and
single-use replay coverage. These are harness tests, not natural reproductions.

Sibling audit: full scans and incremental refresh share `extractManifestFiles`
and the single production compiler invocation. Neither changes. The older
reconstructed-file-list driver (`ts_compiler_console_replay_test.go`) is an
explicit historical control; it does not admit private captures. The disposable
native scan/direct driver checks source/toolchain equality after its own runs;
that comparison detects a different interval and is not the prelaunch private
capture rule. No filters, classifiers, capture producer or wiring are added.
The named comparison helper lives only in `_test.go`; the production script,
deadlines, caps, fallback, graph schema, compatibility paths and APIs are intact.

No causal discrepancy has been established for this recurrence, so there is no
next production repair in this candidate. A later repair requires independently
reviewed privacy-safe **real compiler** fail-before/pass-after/fix-removed failure
at unchanged bounds, equivalent output/provenance, persisted graph reopening
and the unchanged pinned five-case 15/15 journey. Existing successful controls
or this harness improvement cannot substitute for that causal regression.

Exact-candidate review and normal native publication remain lane delivery steps.
No served compiler change is present to deploy. After any later deployed repair,
the supervisor still owes the original PR1693 train pass/landing, checked
activation of the original three blocked briefs and child/retained work, and
the original `6b→19a` full result. None is proved here or waived by this diagnosis.

Complexity delta: test-only comparison reporting and this diagnosis; zero new
production concepts, persistent state, transitions, owner decisions or replaced
machinery. The existing refusal remains fail-closed; no new runtime failure mode.
