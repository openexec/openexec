# TypeScript compiler cancellation: real-extraction evidence (follow-up of PR #165)

Status: **historical kill and new extractor-own-deadline not reproduced by the
three actual observed-request controls supplied on 2026-10-29; cause unknown,
no fix or recovery claimed.** The
blocker stays retained. Evidence only; no compiler semantics, fallback, corpus or
impact assertion change, and the 45 s extractor bound is unchanged. The
control fixture now expects the canonical database path for a symlinked root;
its exact location, filesystem identity and refusal assertions remain enforced.
No credentials or file contents are recorded; every source input is a public immutable Console
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
direct replay of its captured bytes per distinct source**, with the existing 45 s child bound and
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

The second line is the reusable extraction argv, not an instruction to keep a
session alive for a multi-minute scan. External execution belongs to the
supervisor using the existing durable runner (the completed run below must
not be repeated):

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

### Earlier refusal, superseded by supplied external execution

The previous pass attempted supported authorized host execution to create its own
disposable public fixture (not the owner checkout):

```json
["git", "clone", "--no-checkout", "https://github.com/openexec/agent-console.git", "/tmp/openexec-native-request-e763-console"]
```

The tool returned:

> Refused: outside this unattended task's authority — git clone "https://github.com/openexec/agent-console.git": a task clones only its own checkout.

That attempt performed no clone or external scan. No denied path was retried
and no operator-only receipt was accessed by this pass. The supervisor then
supplied the terminal execution and safe metadata below; the previous missing
external-execution dependency is no longer pending. The older TypeScript 5.8.3
466-file direct control still has unproved byte identity with incident native
stdin. It is distinct from the new captured-request TypeScript 5.9.3 comparisons.

### Actual observed Console request/control records (supervisor, 2026-10-29)

The supervisor's 02:14Z safe addenda report completed job
`watch-native-request-e763-corrected-20261029T0210`. This pass transcribes those
supplied results in [the structured record](typescript-native-request-20261029.json);
it did not rerun them or open supervisor-local artifacts. The JSON contains
all supplied child measurements, commands, toolchain identities, UTC deadlines,
CPU/rusage, graph metadata and original artifact hashes. Those hashes identify
original artifacts, **not** this transcribed JSON. Full Go build info and exact
`request.json` bytes remain supervisor-retained; only their hashes and request
shape/byte counts were supplied here. No source filenames are invented.
The executable control reconstructs and captures the exact attempted request
from an explicitly authorized matching public checkout; the older manifest
reconstructor is not a substitute for it.

Clean native candidate `a3e10b77c93af56f180ec91def537d4316bffaed` was compiled
with argv:

```json
["go", "test", "-c", "-o", "/home/perttu/.agent-console/watch-20261029T015700Z/native-request.test", "./internal/knowledge"]
```

Recorded build environment: `GOWORK` = `off`, `GOCACHE` =
`/mnt/data1/projects/.agent-console-gate/go-build`. Linux/amd64, Go 1.26.5.
Executable SHA-256:
`55bfa892aecc9c6d6fa0a9248872bc163781a03c98411aa7419ff1615a77ba6c`.
Go build-info SHA-256:
`03791ae8aad7326a7fb111b5f7c85f6f2bee661dff6c5a4fac598071e8a0cdd5`.
This is the **instrumented native Go test executable**, running the production
scanner and real Node child, not the incident installed CLI. Runtime checkout
HEAD and embedded build provenance remain explicitly separate. These runs
precede the fixture assertion correction below; no production code changed
in that correction.

All cases used `/usr/bin/node` v22.23.4 (SHA-256
`94efd062ed066c1a0b332e18249859c905e920eeada5df55d24c99b919629a90`),
TypeScript **5.9.3** resolved to
`/mnt/data1/projects/openexec/ui/node_modules/typescript/lib/typescript.js`
(SHA-256 `3ae902c92cc44dace175c0e69e13a4b0899f6983c6121d76b9ab8dd5795e7675`),
and extractor script SHA-256
`32cdd4664de00d1272f7e18b8a12767af7d12d0ab55e2810f2917b1b04e0b3a4`.
No shared dependency was modified. Configuration on every child was
`inferred,ui/tsconfig.app.json,ui/tsconfig.node.json`.

| Case / exact Console source | Request bytes / files / source bytes | Actual native child | Identical-request direct child | Whole native scan | Node maxRSS KiB native / direct |
| --- | --- | --- | --- | --- | --- |
| New deadline `2ebcc487ae0a2cd588840f10902038d34da5fc8d` | 19342 / 466 / 4003045 | 8.213715667 s | 6.853639745 s | 45.664820245 s | 630996 / 870892 |
| Original train `8ec35a445faec933bc0a129216f6ea45453cbcb9` | 18917 / 456 / 3845255 | 5.321143114 s | 9.350981676 s | 42.861531324 s | 611664 / 870832 |
| Recorded main `bbbda1c6e7d746c4bb605f3ddb736283828811fb` | 19342 / 466 / 4003045 | 10.037453346 s | 8.646289124 s | 59.101986640 s | 630456 / 885428 |

All six children: **exit 0, decoded compiler output, parent_error=none,
child_error=none**; 45 s child, separate 600 s parents, 15 minute test timeout.
Each source received exactly one native scan and one direct replay; no
reductions or unchanged retries. Node maxRSS is per-child Linux rusage, not
the earlier whole-tree peak. CPU, faults, I/O and context-switch counts are in
the structured record; host load/cgroup evidence was not supplied.

Native/direct request SHA-256 is identical within each pair:

- New deadline and recorded main:
  `e7a3e66fcc532fa22ffbf36c275caef6b3cef1329acf415807e8acf9173b2009`.
- Original train:
  `4307b1db19213202ac1256640f5d0e685719783b82556c298a543480f4ea2913`.

Each actual request has keys `files` and `excluded_directories` (null).
The native scanner's order is retained in the captured bytes, not inferred
from `git ls-files`. Each pair holds those bytes, source/configuration digests,
argv (including temporary script path), Node/TypeScript/script hashes and
45 s bound constant. Only the entry point changes (scanner child versus direct
replay). Sequential cache/load and restored script mtime remain uncontrolled;
these successful comparisons cannot identify cause. Across source revisions,
the binary, Node/compiler/script hashes and bounds stay constant while the
source revision and its file set/content change. Absolute source roots and
temporary script paths differ across cases, and sequential load/cache is
uncontrolled. They are not minimized failing/passing controls.

All three stores were actually closed and reopened: status `current`,
TypeScript definitions/imports/exports `compiler_exact`, compiler_fallback=false,
source_inputs_unchanged_after_direct=true, scan_error=false. All persisted
capabilities and limitations matched the scan result. Each has 29 limitations,
SHA-256 `f0fd6fddb9ccd199975b2cdefe98325b6730a5e96543027a8e5ff9b55e3b7e23`;
other capability levels remain explicit in the JSON (e.g. `effects.static` is
still `static_lexical`; compiler success does not make every capability exact).
Actual resolved databases are beneath each supervisor-owned source root:

```text
/mnt/data1/ac-watch/.watch-worktrees/console-native-capture-new-deadline-20261029/.openexec/openexec.db
/mnt/data1/ac-watch/.watch-worktrees/console-native-capture-original-train-20261029/.openexec/openexec.db
/mnt/data1/ac-watch/.watch-worktrees/console-native-capture-recorded-main-20261029/.openexec/openexec.db
```

This proves compiler success for these controls rather than inferring it from
CLI exit 0. It does not change the supplied incident's lexical-fallback outcome.
The original runner refused all three sources **before any extraction** because
its dependency symlink was untracked. The supervisor corrected only its owned
fixture to an ordinary ignored `node_modules` directory containing the exact
TypeScript symlink and checked empty git status/pinned HEAD. Cleanliness
refusals remain enabled; they are negative controls, not failed extractions.

**Non-reproduction and remaining unknowns.** Neither the new own-deadline
signature nor historical kill occurred. No reduction was attempted and no
minimized failure or passing minimum is claimed. Differences from the new
incident include the instrumented candidate versus configured installed
`5fe138d`, TypeScript 5.9.3 versus supplied 5.8.3, locked dependency resolution,
operator host versus CI, fresh resolved stores, observation overhead and
sequential cache/load. The exact incident native stdin, full binary/toolchain
hashes, ancestor cancellation and load remain unknown. The old reconstructed
466-file direct success does not establish request identity with that incident.
The eleven-expectation impact journey, causal repair and original recovery
remain parent obligations; the blocker stays retained.

### Portable database assertion regression

Hosted run `40803649257` on candidate `a3e10b77…` reported Ubuntu success and
macOS failure in `TestNativeCaptureDriverArtifactsAndRefusals`: SQLite reported
`/private/var/.../.openexec/openexec.db`, while the test expected the equivalent
`/var/...` spelling. The real-compiler roundtrip passed on macOS. The corrected
fixture supplies a public symlink alias on every OS, resolves its expected root,
checks the **exact** `.openexec/openexec.db` beneath it and verifies `os.SameFile`
through the supplied alias. No production path handling or authority changed.

On Linux the alias fixture with the previous raw-path expectation failed at
`persisted graph evidence missing` despite reopened/current/compiler-exact
metadata (2.903 s). With only the expectation correction and filesystem identity
check it passed (7.542 s), including all existing refusal/mutation controls.
A removal control restoring only the raw-root expectation failed again
(2.489 s, the same persisted graph evidence error). The correction was restored;
the alias regression remains in ordinary `go-test` for Linux and macOS. Hosted
checks on the final head and independent exact-head review remain lane
obligations.

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
  evidence, **not Console evidence**. Its source-root alias also exercises
  canonical database identity on Linux as well as macOS.
- `TestNativeCaptureStartFailureIsNotDeadline` rejects misleading error text.
- `TestNativeCaptureSuppliedConsoleRecordIntegrity` reads the committed three-case
  JSON in the declared suite and checks request/toolchain/argv equality, deadline
  consistency, decoded exits and reopened graph metadata. It checks transcription
  integrity; it does not authenticate unavailable private artifacts or re-execute
  Console. The supplied supervisor execution remains the external evidence.

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
`signal: killed` substring classifier. No corpus filter was added. The only raw
database-path expectation in the capture tests is now canonicalized; production
scan/identity/store code already handles symlink roots. Artifact containment
still resolves symlink redirects before refusing them, and its negative
controls remain unchanged.

Complexity delta: one private observation point, one opt-in test control;
no runtime controller, persistent runtime state, execution/continuation
transition, owner decision, or replaced machinery. The explicit diagnostic
artifacts are the requested evidence, not runtime authority.
