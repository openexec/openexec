# TypeScript compiler cancellation: real-extraction evidence (follow-up of PR #165)

Status: **historical kill not reproduced; new extractor-own-deadline supplied
by supervisor; cause unknown, no fix or recovery claimed.** The
blocker stays retained. Evidence only; no compiler semantics, fallback, corpus or
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

## Historical comparisons (limits of control)

| Factor changed | Held constant | Result |
| --- | --- | --- |
| Source: train → main | binary, Node, TypeScript, command, 600 s observer | both exit 0; 65.068 s vs 45.965 s native. Main's native run finished 0.965 s **over** 45 s without being cancelled (see Unknowns). |
| Entry: native CLI → direct extractor | source, Node, TypeScript; native stdin NOT captured | 65.068 s → 9.590 s (train); 45.965 s → 7.831 s (main). This is not a single-factor causal comparison: the native request and surrounding work were not held constant. |

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

The historical driver's request is built from its own manifest, not observed
from the graph scan; byte identity with native stdin is unknown. It is not
valid to assert either equality or inequality without capture. That driver has not been
run against Console from this sandbox (checkout outside its reach); only the
supervisor numbers above are real-run evidence.

## New deadline occurrence (separate from the historical kill)

Supplied supervisor execution, 2026-10-28; not repeated or independently
verified by this pass:

| Field | Failed native scan | Separate direct control |
| --- | --- | --- |
| Public Console revision | `2ebcc487ae0a2cd588840f10902038d34da5fc8d` | same |
| Command argv | `/home/perttu/.local/bin/openexec --data-dir <own-empty-store> knowledge graph scan --directory <own-disposable-checkout> --json` | `node <public-sdk-extractor.mjs> <effective-typescript.js> <same-checkout>`; explicit JSON stdin |
| Binary identity | configured native `5fe138d`; full revision/build/hash not included in supplied safe receipt, unknown here | Node `/usr/bin/node` 22.23.4, TypeScript 5.8.3; full effective compiler path/hash not included, unknown here |
| Request | actual native stdin not captured, unknown | 466 files, SHA-256 `1c1ab212fa2728fc0ad40a74c23f61027b5a85410244fb47aff8b2150cd30274`; serialized/source byte counts not supplied, unknown |
| Script | native embedded script hash not supplied, unknown | `32cdd4664de00d1272f7e18b8a12767af7d12d0ab55e2810f2917b1b04e0b3a4` |
| Configuration | native configuration unknown | same-source/config control per supervisor; exact configuration string not supplied, unknown |
| Child ending | own 45 s deadline, **45.147 s**, `signal: killed`; parent cancellation not reported | exit 0, **16.604 s**, 466 compiler-exact files |
| Whole CLI / outer observer | **459.472 s**, exit 0 / 600 s, did not expire | output 12630385 bytes |
| Resources | process-tree peak 2308992 KiB | maxRSS 944528 KiB |
| Persisted graph | lexical fallback reported; resolved database location and reopened capabilities/limitations not supplied, unknown | no persisted graph replay supplied |

**Exit 0 with `static_lexical` is compiler failure/fallback, not compiler or
impact-gate success.** `--data-dir` does not establish graph isolation:
`internal/cli/knowledge_graph_test.go` exercises the CLI store, and the scan
handler in `internal/cli/knowledge.go` calls `knowledge.NewStore(graphDirectory)`.
`NewStore` opens `<root>/.openexec/openexec.db`. The control below reads
`PRAGMA database_list` and closes/reopens that actual store.

Later supplied recurrences: Console `e1ff52ac8843a68e38b7b67a08b254dc357698e2`,
job 123106369216, 45.261 s; `febdf34826c143a5668743c5c0a6a392e56f69e9`,
job 123141665961, 45.271 s. Both had five matched, four bound-unavailable and
two missing of eleven impact expectations. The latter train's later successful
bounded retry does not establish cause or recovery. No logs from operator-only
scratch were read or committed.

## Observed native request control (this extension)

`TestConsoleNativeRequestCapture` in
`internal/knowledge/ts_compiler_native_capture_test.go` runs the real
`ScanRepository → extractManifestFiles → extractTypeScriptWithCompiler` path
in a compiled native Go test executable. The observer receives the same byte
slice assigned to the child's stdin, after `cmd.Run` and before deferred
cancellation. Ordinary scans do not set the private observer context key.
The only production change is that opt-in observation point; compiler code,
request construction/order/exclusions, fallback, cancellation and the 45 s
bound retain their existing behavior.

This is **not the incident installed CLI executable**. Its executable path,
SHA-256, Go build info, repository revision and complete argv are captured.
The observer parses output for counts/configuration and stats source sizes
after the child timer stops. This adds scanner overhead and may alter load;
it does not charge probes against the measured child. Native and direct runs
remain sequential and may differ in host load/cache state. The exact temporary
script pathname is restored with identical embedded bytes for direct replay;
its mtime changes. These differences prevent treating a successful replay as a
causal passing minimum.

Finite budget fixed before external extraction: **one native scan and one
direct replay of its captured bytes**, with the existing 45 s child bound and
a separate 600 s observer parent for each. No unchanged retries, file-set
reductions, Node/compiler switches or deadline changes. If either reaches its
own deadline, preserve artifacts and stop this budget; any reduction requires
a further finite supervisor comparison plan and must retain that ending.

Supervisor commands, from this candidate repository, with only the supervisor's
fresh disposable public source and an absent output directory supplied:

```text
go test -c -o /tmp/openexec-native-request.test ./internal/knowledge
/tmp/openexec-native-request.test -test.run=^TestConsoleNativeRequestCapture$ -test.v -test.timeout=15m -console-native-checkout=<own-fresh-checkout-of-2ebcc487ae0a2cd588840f10902038d34da5fc8d> -console-capture-dir=<new-directory-outside-checkout>
```

The second line is the exact extraction argv, not an instruction to keep a
session alive for a multi-minute scan. After the candidate is recorded, the
supervisor should hand it to the existing durable runner:

```text
python3 /mnt/data1/projects/owner-model/job_runner.py start --name typescript-native-request-e763 --project openexec --cwd <candidate-repository> -- /tmp/openexec-native-request.test -test.run=^TestConsoleNativeRequestCapture$ -test.v -test.timeout=15m -console-native-checkout=<own-fresh-checkout-of-2ebcc487ae0a2cd588840f10902038d34da5fc8d> -console-capture-dir=<new-directory-outside-checkout>
```

The native test executable's own hash/build identity is in `identity.json`;
this command does not use or claim the installed CLI hash. It refuses dirty
sources, pre-existing `.openexec`, output inside source, or an existing output
directory. It never reads or edits an owner/agent Console checkout. No model
or provider call is made.

Artifacts (directory mode 0700, files 0600):

- `identity.json`: native executable hash/build/argv, OpenExec and source HEAD,
  platform and instrumentation disclosure. OpenExec HEAD is explicitly the
  runtime checkout, not an embedded source-provenance assertion. Go test build
  info can omit VCS identity (the local Go 1.26.5 `-buildvcs=true` control did);
  retain the candidate build command alongside the executable hash.
- `request.json`: exact serialized public filenames and exclusions; no source
  bodies. Both run records hash these bytes, preserving order.
- `native-child.json`, `direct-child.json`: actual argv, Node/compiler paths,
  versions/hashes, embedded script hash, source bytes/file count, request
  bytes/hash, configuration, UTC start, elapsed, absolute parent/child
  deadlines, cancellation state, exit code/process-state signal, output sizes,
  extracted/diagnostic counts, user/system CPU and available OS rusage.
  Linux `Maxrss` is KiB; macOS `Maxrss` is bytes. This is the Node process
  resource usage, **not** a whole-tree maximum or historical OOM evidence.
  Unavailable process usage is JSON null, e.g. failure to start; unrecorded
  system load, cgroup limits and incident resources remain unknown.
- `graph.json`: resolved database path, whole-scan elapsed/error, reopened
  generation status/capabilities, limitation count/hash and explicit compiler
  fallback flag, manifest/configuration digests and post-replay source-input
  revalidation. Toolchain hashes must also remain equal. The reopen asserts equality with all persisted limitations;
  their messages are not exported. A scan error is recorded as such and cannot
  claim a successful reread. Child deadline capture survives a later scan error.

Compiler stdout/stderr and diagnostic messages are not exported. A compiler
failure intentionally makes the control fail even if the scan returns nil:
classification is structured (`extractor_deadline`, `caller_cancellation`,
`external_signal`, `ordinary_nonzero_exit`, `start_or_io_error`, `exit_0`),
never a substring match on `signal: killed`. A zero exit with malformed output
has `response_decoded=false` and is not compiler success.

### External proof status and exact refusal

This pass attempted supported authorized host execution to create its own
disposable public fixture (not the owner checkout):

```json
["git", "clone", "--no-checkout", "https://github.com/openexec/agent-console.git", "/tmp/openexec-native-request-e763-console"]
```

The tool returned:

> Refused: outside this unattended task's authority — git clone "https://github.com/openexec/agent-console.git": a task clones only its own checkout.

No clone or external scan was performed, no denied path was retried, and no
operator-only scratch receipt was accessed. The actual Console native request,
new child measurements and reopened graph remain **unproven pending the
supervisor-owned execution above**. There is no invented exact Console
request or minimized failure. The supplied 466-file direct success still has
unproved byte identity with native stdin. Unknown incident binary/toolchain
identity, load, ancestor context and resolved storage are retained explicitly.

### Offline executable evidence

The existing declared `go-test` / hosted `go test -v ./...` includes these tests:

- `TestNativeCaptureObservesConsumedRequestAndPersistedFallback` drives the
  real scanner and real child stdin, compares child-read bytes to the observer,
  exercises own deadline, caller cancellation, external SIGKILL, ordinary exit
  3, success and malformed output with exit 0, and reopens the graph after successful and fallback scans.
  Short test-only deadlines make failure branches bounded; these are plumbing
  tests using a controlled child, not a reproduction of the 45 s incident.
- `TestNativeCaptureRealCompilerRoundTrip` uses the locked installed TypeScript
  compiler on a tiny public fixture with a non-default tsconfig, replays the
  exact captured request/argv and verifies persisted `compiler_exact`.
- `TestNativeCaptureDriverArtifactsAndRefusals` invokes the native test
  executable's public control entry point against its own synthetic clean git
  checkout, re-reads every artifact and actual database location, verifies
  executable/request identities, and exercises refusal of output inside source
  (including symlink redirects), dirty checkouts, existing output and an
  existing graph. Separate comparisons deliberately change source or toolchain
  between native and direct runs and must be refused as invalid comparisons;
  both still delegate extraction to real Node/TypeScript. This is control
  evidence, **not Console evidence**.
- `TestNativeCaptureStartFailureIsNotDeadline` rejects misleading error text.

Tests skip the optional public-source control unless explicitly supplied. The
real-compiler tests explicitly skip when Node or the locked TypeScript package
is unavailable; they install nothing and use no network. Full Linux/macOS CI
and independent committed-head review remain lane verification obligations;
this pass does not claim their results. No deployment is needed for diagnosis.
The original blocker and all eleven parent recovery expectations remain open.

Sibling-path review: full scan and incremental refresh both call
`extractManifestFiles`; that function is the only production caller of
`extractTypeScriptWithCompiler`. The observer covers both without changing
either route. The historical reconstructed-manifest driver stays a historical
control; the new observed driver does not reuse its filename filter or
`signal: killed` substring classifier. No corpus filter was added.

Complexity delta: one private observation point, one opt-in test control;
no runtime controller, persistent runtime state, execution/continuation
transition, owner decision, or replaced machinery. The explicit diagnostic
artifacts are the requested evidence, not runtime authority.
