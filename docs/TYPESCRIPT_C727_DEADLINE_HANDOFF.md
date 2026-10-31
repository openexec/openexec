# Console c727 compiler deadline: bounded diagnostic handoff

Status: **nonreproduction; no repair demonstrated**. This is the finite first
artifact for task `8fe8d21460bd0a4a18ae1bf58c01bf19`, not completion of the
recurring deadline repair. No extractor, scanner, timeout, output bound, corpus,
provider, accounting or delivery policy changes accompany it. Raw captures and
compiler output are not included in this public artifact.

## Actual incident timeline

Read directly with the authorized host tool on 2026-10-29:

```text
gh api repos/openexec/agent-console/actions/jobs/123445114980/logs
```

Source: Console train `40873157171`, job `123445114980`, runner
`agent-console-7`; failed tree `c727ba1e44099ee04afea852dfbffc3ea89eff7b`.
The checkout's commit subject is
`train: #1601 at 058548c4783935ad5bee5342d2301a4d5ec35440`.

| UTC, 2026-10-29 | Log evidence |
| --- | --- |
| 12:25:49.9359739 | `go version go1.25.0 linux/amd64` |
| 12:25:50.2336125 | `node: v22.23.3` |
| 12:37:10.3997664 | `go run ./cmd/impact-replay --openexec "openexec" --repository .` followed by `--corpus docs/impact-replay-corpus.json --scan` |
| 12:41:22.5852706 | `TypeScript compiler extraction unavailable; using lexical definitions/imports: TypeScript compiler extractor: signal: killed (extractor deadline 45s exceeded after 45.223s):` |
| 12:41:22.6101845–12:41:22.6105326 | Five cases, 15 expectations, nine matched, three bound-unavailable, three missing, recall 0.6 |
| 12:41:22.6546912 | `make: *** [Makefile:398: check-backend-go-1] Error 2` |

The same extractor diagnostic is repeated in all five entries for one graph,
`graph_d63e7752-37bc-455e-9b41-c3792935d03c`; these are not five measured child
runs. The missing canonical-rule callers include
`AttentionCenter.NeedsYouView` and `TaskBoard.TaskBoard`. The log does not
contain actual child stdin, compiler hash, installed OpenExec binary hash or
per-child resource measurements. The roughly 252 seconds between replay
invocation and report includes work other than the TypeScript child.

## One exact-source capture, no retries

OpenExec source/build checkout:
`7f1692da65e15b6db5f23377fb43340db9fc894d` (unchanged runtime code).
A clean, disposable local clone was pinned to the failed Console SHA above.
Its ordinary ignored `ui/node_modules` directory contained symlinks to the
installed Console dependencies; no owner checkout or dependency was modified.
Both source and installed dependency lockfiles have SHA-256
`adc030dd64c599375382d6f5688381da5767d6f5cea32b4682139fb9879c78d3`.

Build argv: `go test -c -o /tmp/openexec-8fe8-native.test ./internal/knowledge`.
Build environment overrides: `GOWORK=off`,
`GOCACHE=/mnt/data1/projects/.agent-console-gate/go-build`.
The invocation used the existing driver:

```text
/tmp/openexec-8fe8-native.test -test.run=^TestConsoleNativeRequestCapture$ -test.v -test.timeout=15m -console-native-checkout=/tmp/openexec-8fe8-console-c727 -console-capture-dir=/tmp/openexec-8fe8-capture
```

`PATH` was prepended with the incident job's existing
`/mnt/data1/github-actions/agent-console-7/_work/_tool/node/22.23.3/x64/bin`.
The sandbox invocation returned exit 0:
`native=exit_0 direct=exit_0; request=e7a3e66fcc532fa22ffbf36c275caef6b3cef1329acf415807e8acf9173b2009`.
Total test time: 72.71 seconds. Exactly one native scan and one direct child
consumed identical stdin and argv. No reduction or repeated timing probe ran.

| Measurement | Native child | Identical-stdin direct child |
| --- | --- | --- |
| Start UTC | 15:06:21.624473183 | 15:07:20.782570788 |
| Child deadline UTC | 15:07:06.624467603 | 15:08:05.782566327 |
| Elapsed ns | 17714672434 | 12999493761 |
| User / system CPU ns | 29252683000 / 828914000 | 23935587000 / 663962000 |
| Linux maxRSS KiB | 1097100 | 1103328 |
| Stdout / stderr bytes | 12671306 / 0 | 12671306 / 0 |
| Extracted files / diagnostics | 466 / 20 | 466 / 20 |
| Parent / child context error | none / none | none / none |
| Bound / result | 45s / exit 0, decoded | 45s / exit 0, decoded |

The request was 19,342 bytes: 466 files, 4,014,588 source bytes;
keys `files` and `excluded_directories` (null). Configuration:
`inferred,ui/tsconfig.app.json,ui/tsconfig.node.json`.
Node v22.23.3 SHA-256:
`fde6a4bf8d0562f7751d1a2d6cb9b417c4cfe107bbcb0aa3e9a24e125e348f48`.
TypeScript 5.8.3 SHA-256:
`dd17428736a07e1db1a138d8a14295ddb2699ba780ee15038acdd2c6da5373a0`.
Extractor script SHA-256:
`32cdd4664de00d1272f7e18b8a12767af7d12d0ab55e2810f2917b1b04e0b3a4`.
Instrumented Go test binary SHA-256:
`d3a3eba1eeeceb9e7cc216bceea10b9ee236aabbca0f435545934aceeb1085f7`.
Build info records Go 1.26.5, Linux/amd64; it does not embed VCS identity.
The runtime checkout HEAD above is separate evidence, not embedded provenance.

The store was closed and reopened. Generation
`graph_a1c3a33f-5cad-42dc-b7df-bef8e0904441` remained current, with TypeScript
definitions/imports/exports `compiler_exact`, compiler fallback false, and
persisted capabilities/limitations matching the scan. Whole scan: 58.674117999s.
Source/configuration stayed unchanged after the direct replay:

* Manifest: `sha256:d740b81a0eae8121fe818ff6828b3ddb96d54552fe38fa9dd0f78969a177f83f`.
* Configuration: `sha256:e8c83e50c85205099ec16e0ca9ff95e469b5c1a58ff5fbe019736277eb2dd917`.

## Original corpus on the reopened graph

The normal backend build (`go build -buildvcs=false -o
/tmp/openexec-8fe8-cli ./cmd/openexec`) exited 0. Binary SHA-256:
`e257ac94ec13bea3c8c5054d2a00c5da0a18a82f4c3d2542e6919fba7b17671d`.
The failed Console source's original `cmd/impact-replay` was built and run in
two separate processes, each with these arguments:

```text
--openexec /tmp/openexec-8fe8-cli --repository /tmp/openexec-8fe8-console-c727 --corpus /tmp/openexec-8fe8-console-c727/docs/impact-replay-corpus.json
```

There was no `--scan`: this deliberately reused the driver's saved graph,
avoiding a second compiler scan. Both replays exited 0 on the same generation:
five cases, 15 expectations, 15 matched, zero missing, zero unavailable,
zero bound-unavailable, zero unresolved anchors, recall 1. Two cases still
reported truncated optional reach; no expectation was waived or hidden by a
bound. Corpus SHA-256:
`1c05a3b070b9a90baaf2da7420dc6161cbb6e994def201881a6cfb8a6ec37b86`.
Building the Console replay printed this warning and still exited 0:
`go: writing stat cache: open [module-cache]/github.com/openexec/agent-console/@v/v0.177.10-0.20261029112657-c727ba1e4409.info132048178.tmp: read-only file system` (local cache prefix redacted).

The following focused checks also executed successfully:

* Declared `go-test`, args `./internal/knowledge -run TestNativeCapture -count=1 -v`:
  exit 0, 21.150s. Includes real compiler round-trip/reopen, consumed-request
  and persisted-fallback checks, source/toolchain refusal controls, and
  success/deadline/caller/signal/ordinary/malformed-output endings.
* Declared `go-test`, args `./internal/knowledge -run TestExtractorFailureNamesWhoEndedIt -count=1 -v`:
  exit 0, 0.514s; deadline, external kill, caller cancel and plain failure.
* `go vet ./internal/knowledge/...`: exit 0.

These are existing behavior checks, not a new deadline regression. No full
gate, Linux/macOS hosted result or independent exact-head review was obtained
in this diagnostic pass. No new skip or waived assertion was introduced.

## Source-bound hypothesis and unmatched conditions

`extractManifestFiles` in `internal/knowledge/graph_scan.go` calls the single
`extractTypeScriptWithCompiler` boundary. The CLI's explicit scan and context
initialization both use `ScanRepository`; there is no separate CLI compiler.
The embedded `ts_compiler_extractor.mjs` groups by configuration, creates a
program/checker per group and calls `getPreEmitDiagnostics` before walking
references. Program construction, semantic diagnostics and reference walking
are candidate cost centers, **not measured causes**. Nothing here supports
changing diagnostics, compiler versions, grouping or reference extraction.

The exact public source and Node version match the supplied incident; actual
incident stdin, compiler/configuration/binary identities and host contention
remain unpaired. This run used a Go 1.26.5 test executable instead of the
incident CLI; the job's Go setup was 1.25.0, which does not establish the CLI's
build version. Dependency symlinks, absolute root, warm caches, restored script
mtime and sequential load differ. No CPU/cgroup state from the failed child
was logged. These unknowns prevent establishing the causal local discrepancy.
No faithful failing synthetic input or fail/pass/fix-removal regression can
be claimed. The successful corpus replay is diagnosis, not repair acceptance.

## Single bounded supervisor handoff

Within the existing supervisor's next already-authorized failed-host attempt,
run the existing `TestConsoleNativeRequestCapture` driver once on a fresh,
clean, supervisor-owned disposable checkout at the failed SHA, with the
failed worker's actual Node/compiler/dependency configuration. Use the build
and driver argv above with fresh owned output paths. Keep the 45s child,
600s parents and 15m outer test bound. Do not reset tasks, create a backlog,
rerun/cancel the active train, add a billed endpoint or repeat broad probes.

Before running, retain the incident CLI's resolved path, SHA-256, build info
and source identity if still recoverable; label current replacements as
current, never incident. Retain actual PATH/Node/compiler resolution, dependency
lock/configuration hashes and host CPU/cgroup/load measurements with the
capture's stdin/argv, process endings and resource measurements. Keep raw
captures private; return source-safe hashes, counts, timings and identities.
An unavailable historical identity must stay unknown rather than be inferred.
The driver remains an instrumented test executable, not the incident CLI.

Return exactly one captured native/direct comparison. If it fails, reduce
the captured real input while retaining the signature, then require a real
compiler regression failing before and with only the fix removed, passing
after, persisted graph reopen and the unchanged 15/15 production corpus. If
it succeeds, return nonreproduction and the remaining unmatched conditions;
do not call the repair complete or repeat the same probe. This handoff creates
no controller, persistent state, transition or owner decision. Post-deploy
served observation remains separate from merge acceptance.
