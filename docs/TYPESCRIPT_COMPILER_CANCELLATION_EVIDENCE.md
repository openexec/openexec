# TypeScript compiler cancellation: real-extraction evidence (follow-up of PR #165)

Status: **not reproduced, cause unknown, no fix or recovery claimed.** The
blocker stays retained. Evidence only; no compiler, fallback, corpus or
assertion change, and the 45 s extractor bound is unchanged. No credentials or
file contents are recorded; every source input is a public immutable Console
commit.

## What was run (supervisor, 2026-10-28, operator host, own disposable worktrees)

| Field | Console train | Console current main |
| --- | --- | --- |
| Commit | `8ec35a445faec933bc0a129216f6ea45453cbcb9` | `a0fdd79aa09a95a07dbd959068aee17c7152b3c0` |
| Native executable | `/home/perttu/.local/bin/openexec`, same binary for both | same |
| Build identity | `f3310a1a21895b3adb5998d286f9b56b016d93a8`, `v0.13.1-207-gf3310a1a`, built 11:14:07Z | same |
| Executable SHA-256 | `a28cabe6faffaa96e3f5045b39a2b809ea9904c9e5ac57bbf0b42e9ae1c4d0e2` | same |
| Node / TypeScript | `/usr/bin/node` v22.23.4 / TypeScript 5.8.3 from the home cached `node_modules` | same cache |
| Native command (argument list) | `strace -f -e trace=execve,kill,wait4 -o <private-trace> openexec --data-dir <private-scratch-store> knowledge graph scan --directory <disposable-checkout> --json` | same |
| Native parent / child deadline | outer observer 600 s (not reached) / extractor 45 s | same |
| Native result | exit 0, **65.068 s**, whole-child-tree peak 2236676 KiB | exit 0, **45.965 s**, peak 2135232 KiB |
| Cancellation state | none observed; no signal | none observed; no signal |

Direct public-extractor controls (same source, configuration and toolchain,
`node <sdk-extractor>.mjs <typescript.js> <checkout>`, JSON request on stdin):

| Field | Train | Main |
| --- | --- | --- |
| Request | 456 tracked TS/JS files, 19383 bytes, SHA-256 `3586d9eef035926595624eab41260696278f34694509579a1b9b6604f2faea78` (identical on both) | same |
| Extractor script SHA-256 | `32cdd4664de00d1272f7e18b8a12767af7d12d0ab55e2810f2917b1b04e0b3a4` | same |
| Configuration | inferred, `ui/tsconfig.app.json`, `ui/tsconfig.node.json` | same |
| Result | exit 0, **9.590 s**, maxRSS 1077172 KiB, 12033187 response bytes, 456 extracted, 20 diagnostics | exit 0, **7.831 s**, maxRSS 866880 KiB, 12126763 response bytes, 456 extracted, 20 diagnostics |

Request manifest rule: `git ls-files` tracked plus untracked-not-ignored, extensions
`.ts .tsx .js .jsx`, excluding `.git .openexec .gocache node_modules vendor dist
build coverage _archive` and hidden path parts except `.github`.

Full receipts (identity, commands, strace, direct request and responses) are
operator-only under the supervisor scratch directory and are deliberately not
committed.

## Controlled comparisons (one factor at a time)

| Factor changed | Held constant | Result |
| --- | --- | --- |
| Source: train → main | binary, Node, TypeScript, command, 600 s observer | both exit 0; 65.068 s vs 45.965 s native. Main's native run finished 0.965 s **over** 45 s without being cancelled (see Unknowns). |
| Entry: native CLI → direct extractor | source, Node, TypeScript | 65.068 s → 9.590 s (train); 45.965 s → 7.831 s (main). The native scan includes work beyond the extractor; the extractor alone is far under 45 s. |

An unchanged successful retry is not treated as causal evidence. No factor
produced `signal: killed`, so there is no failing configuration to minimize and
no minimized fixture is delivered; minimizing a failure that did not occur would
be invention.

## Differences from the incident

- Operator host shell, not the original GitHub job.
- Current cached Node/TypeScript and installed binary versus unrecorded incident identities.
- `strace` observation and fresh private stores.
- The native CLI's raw extractor stdin was not separately captured; the direct
  request is a separate explicit control and is not asserted byte-identical to
  the native wire request.
- Resource peaks are observed process-tree maxima, not an incident memory record.

## Unknowns

- Who ended the incident's compiler (parent cancellation, the 45 s child
  deadline, or an outside kill such as the OOM killer): the incident ending text
  `signal: killed` cannot tell these apart, which is why PR #165 now names the
  ender in the error.
- The incident's Node, TypeScript and native binary hashes.
- Why the native scan took 65.068 s / 45.965 s while the extractor alone took
  under 10 s, and whether the 45 s bound applies to that whole span or only to
  the child (the native runs exited 0 despite exceeding 45 s total, so the
  bound is evidently per extractor call; this was not separately measured).
- A process that succeeded now does not exclude a historical OOM kill under
  load; peak RSS of about 2.1 GiB is the only memory evidence.

## Public reproducer

`internal/knowledge/ts_compiler_console_replay_test.go`:

    go test ./internal/knowledge -run TestConsoleCheckoutExtractionRecord -count=1 -v -args -console-checkout=<authorized public Console checkout>

It rebuilds the bounded request from the supplied checkout (rule above), runs the
production `extractTypeScriptWithCompiler` unchanged with the 45 s bound, and logs
a content-free JSON record: head, Go/Node/TypeScript paths and versions, file
count and bytes, request SHA-256, parent/child deadlines, elapsed, extracted
count, configuration, error and cancellation state. It **fails only** with the
incident signature (`signal: killed`); any other outcome, including success, is
recorded as a non-reproduction. Without the flag it skips, so ordinary
`go test` stays offline. Its offline tests pin the manifest rule (including near
misses such as `src/distance.ts` and `.github`), manifest determinism over a git
fixture, and that only `signal: killed` counts as the signature.

The driver's request is built from the manifest, not by the graph scan, so its
hash will differ from the native scan's request. The driver itself has not been
run against Console from this sandbox (checkout outside its reach); only the
supervisor numbers above are real-run evidence.
