# Repository symbol tools for coding agents — review handoff

Status: review rounds 1–5 resolved; committed and deployed
Date: 2026-08-06
Spans two repositories: `openexec` (main) and `agent-console` (branch
`symbol-mcp`, based on `origin/main` @ `8acad70` — the deployed commit)
Related: `KNOWLEDGE_V3_PLAN.md` (this is phase V3.0), `KNOWLEDGE_V2_PLAN.md`, `LIGHT_MODE.md`

## Why this change exists

OpenExec already indexed every symbol's file, line range, signature, resolution
tier and call sites. Three consumers could reach that index; the one that
needed it most could not.

| Consumer | Path | Before |
| --- | --- | --- |
| Agent Console UI (humans) | HTTP proxy → V2.3 query API | had it |
| DCP router | `internal/cli/start.go` (`OPENEXEC_ENABLE_DCP`) | had it |
| OpenExec's own blueprint agents | `loop/mcpconfig.go` → `mcp-serve` | **no graph tools on that server** |
| Agent Console's spawned agents | `providers/claude.go` `--mcp-config` | **only the approval server** |

`SymbolReaderTool` (`internal/tools/symbol_reader.go`) was registered solely in
`newDCPCoordinator`, which runs only when DCP is enabled. The MCP plane — the
one serving coding agents — advertised backlog, memory, skills, approval and
infra tools, and nothing about the repository graph.

So a pointer store built to stop agents grepping was reachable by the human UI
and an optional router, but not by the agents doing the grepping. This change
is a wiring change, not a new capability: no new extraction, no schema change.

## Review round 1 — findings and resolutions

| # | Finding | Resolution |
| --- | --- | --- |
| 1 (P1) | Symbol tool names not reserved; a module could shadow one and be dispatched ahead of the core handler with a read-only session's blessing | Added all three to `reservedCoreToolNames()`. Per-name collision tests, plus `TestReservedNamesCoverEveryAdvertisedTool` so the set cannot drift behind `handleToolsList` again |
| 2 (P1) | Codex unwired, and it is the provider actually installed on the deployment host | Wired via per-run `-c mcp_servers.openexec.*` TOML overrides, on both the fresh and resume paths, in every sandbox mode. Syntax verified empirically against codex-cli 0.114.0 |
| 3 (P1) | Not a read-only server: backlog/skill/patch surface, and a lookup could trigger a full synchronous scan — contradicting the comment in `symbols.go` | New `--profile symbols`: only the three tools advertised **and** authorized, everything else denied at the broker, and `Store.SetRefreshOnRead(false)` so a drifted graph returns an explicit stale refusal. The false comment is corrected |
| 4 (P1) | `--workspace` only partially authoritative; modules, infra allowlist and approvals still used cwd | `resolveServeRoot` computes one root (flag → `WORKSPACE_ROOT` → cwd) used by every repository-scoped resource. The symbols profile never reaches the infra/approval wiring at all |
| 5 (P2) | Invalid `--mode` silently widened to auto-edit | `validateServeMode` fails closed on the flag. An invalid inherited `OPENEXEC_MODE` warns and falls back rather than failing the server |
| 6 (P2) | `AGENT_CONSOLE_OPENEXEC_BIN=""` could not disable; `symbol_relations` promised line numbers it never had and reported requested depth as reached | `envOrUnset` distinguishes unset from deliberately-empty. `Line` removed (the graph stores byte offsets); `DepthReached` renamed `DepthRequested` and rendered "searched to depth N" |

The reviewer's read of finding 4 was sharper than the code: `workDir` did follow
`--workspace`, but when `WORKSPACE_ROOT` was set *without* the flag the server
resolved to it while the other resources still used cwd. The fix removes the
divergence rather than patching the flag case.

### Finding 7 — found by the round trip the review asked for

Both agent CLIs are installed on this host, so the console→provider→MCP path
was finally exercised for real. It failed, for a reason no argv assertion could
have caught:

> "I have a `symbol_find` tool, but plan mode blocks calling it, so I can't
> report the file name and line range." — Claude Code 2.1.220

Claude Code's plan mode refuses to *execute* any MCP tool it cannot tell is
safe. A read-only Agent Console session runs `--permission-mode plan` and
deliberately carries no approval channel, so the agent listed the symbol tools,
declined to call them, and fell back to grep — precisely the behaviour this
work exists to remove. The tools were registered and useless.

Fix: the tool definitions now carry MCP annotations (`readOnlyHint: true`,
`destructiveHint: false`, `idempotentHint`, `openWorldHint: false`), declaring
in the protocol what the prose already claimed. That alone made the round trip
pass. An earlier attempt added `--allowedTools` to pre-approve the three tools
by name; it was removed once annotations proved sufficient, since naming tools
in an allowlist is a security-relevant addition that turned out to be
unnecessary.

`TestSymbolToolsDeclareThemselvesReadOnly` guards it. The fix lives in
`internal/mcp/symbols.go`, so it benefits every client — including OpenExec's
own blueprint agents.

## Review round 2 — findings and resolutions

| # | Finding | Resolution |
| --- | --- | --- |
| 1 (P1) | Work sat on an obsolete branch, and redefined `OpenExecBin` — which current main already uses to arm the scan/publish scheduler, deliberately defaulting to empty. A naïve port would have started the 47-repository sweep on every install | Rebranched onto `origin/main` @ `8acad70` (`symbol-mcp`) and re-applied by hand rather than by patch. `OpenExecBin` keeps main's meaning and its empty default; the symbol tools ride the same opt-in switch, since they are useless without the refresh that keeps their graph current. `TestSymbolToolsStayOptIn` fails if a default ever reappears |
| 2 (P1) | Agent Console already owns refresh, but one FIFO queue meant an interactive refresh could wait hours behind the sweep — and the symbol tools are unusable from the first edit until it lands | Split into an interactive queue and a sweep queue. The worker drains interactive first, and an interactive request **preempts** a running sweep by cancelling it; the preempted checkout is requeued so daily convergence still completes. MCP stays fail-fast — no synchronous refresh was reintroduced |
| 3 (P2) | The collision guard compared two hand-maintained lists, so a tool added to `handleToolsList` and forgotten in both would pass | `TestEveryAdvertisedToolIsReserved` drives the real `handleToolsList` with every optional surface enabled (full-auto, infra registry, fork manager, operator session, symbol index, a module provider) and asserts each advertised name is reserved |
| 4 (P2) | "Read-only" was untested for an initialized-but-unscanned checkout: opening the store creates and migrates a database, and resolving identity inserts rows | Both sides now gate on the **graph database**, not on `.openexec`, so an unscanned checkout is never opened. Two acceptance checks added: `.openexec` is byte-identical after three lookups, and an unscanned checkout gets no tools and no database |
| 5 (P2) | The Codex proof used a throwaway `CODEX_HOME` and `codex mcp list`, isolated from the real configuration | The authenticated `codex exec` round trip is the gate and now runs green on the ported branch, alongside Claude — see below |

`envOrUnset` from round 1 was removed: main's `OpenExecBin` already uses plain
`os.Getenv`, so unset and empty both mean off, and the helper would only have
re-introduced a default.

## Review round 3 — findings and resolutions

| # | Finding | Resolution |
| --- | --- | --- |
| 1 (P1) | A queued sweep blocked an interactive request: one `pending` boolean could not distinguish queued-sweep, queued-interactive, running-sweep and running-interactive, so "Scan now" on an already-queued checkout was discarded | Replaced with per-checkout `refreshEntry` state plus an attempt counter. An interactive request now **promotes** a queued sweep — a fresh interactive job is enqueued and the stale sweep entry is skipped when dequeued, so the checkout is scanned once, on the reserved worker |
| 2 (P1) | Killing the scan is the wrong preemption primitive: it discards minutes of extraction, can leave a generation building, logs intentional cancellation as failure, and may orphan child compilers | Cancellation removed entirely. Two workers now run — one reserved for interactive refreshes, one for the sweep — so interactive latency is low without interrupting anything. None of the listed hazards can arise because no scan is ever killed |
| 3 (P2) | `openexec.db` can exist with zero graph generations, so the tools were advertised and then answered "no graph"; the one-byte fixture proved only the file gate | `hasServableGraph` opens the database **read-only** (`mode=ro`, which fails rather than creates) and requires a `current`/`partial` generation for that root. Covered by a Go test using a real store with no generations, and by an acceptance fixture built with `knowledge graph symbol`, which leaves a valid 270 KB database holding no graph |
| 4 (P2) | The acceptance script was not repeatable: it reused a `CODEX_HOME` it did not create and hardcoded `/tmp/openexec-bin` in the Codex override | The script creates and cleans its own `CODEX_HOME` and uses the supplied `BIN`. Now 19/19 from a clean machine |
| 5 (P2) | The coupled switch was undocumented — the QUICKSTART entry was lost in the port | Documented in `docs/QUICKSTART.md` and at length in `docs/OPENEXEC_REPOSITORY_INTELLIGENCE.md`, stating explicitly that one switch enables both refresh and agent symbol tools, and why they are coupled |

On finding 3, Agent Console keeps a plain file-existence check. Repeating the
generation probe there would mean linking a SQLite driver into a binary whose
only dependency today is a YAML parser, to re-derive an answer the server
already gives correctly; the cost of being wrong is one spawned process that
advertises nothing. That trade-off is stated in the code.

## Review round 4 — findings and resolutions

| # | Finding | Resolution |
| --- | --- | --- |
| 1 (P1) | Two workers could scan one checkout concurrently: with a sweep running, an interactive request passed every dedup case and the reserved worker started it immediately. Both jobs mutated one `refreshEntry`, and OpenExec's mutex is process-local | Refresh is now single-flight per checkout. An interactive request during a running sweep sets `followUp` and returns — it does not occupy the interactive worker — and is queued when that scan finishes. `startKnowledgeJob` re-checks `running` under the lock as a second guard. A request during a running *interactive* scan still dedups. `TestInteractiveRequestDoesNotRaceARunningSweep` records start/end markers, so an overlap is observed rather than inferred |
| 2 (P1) | Scheduler and provider were proven separately but never joined: no test covered edit → scheduler → real scan → next provider turn | `TestLiveSchedulerToProviderRoundTrip` runs the whole path with nothing stubbed — real `openexec` binary, real scheduler, real HTTP console receiving the publish, real authenticated agent CLI. It builds the graph through the scheduler, finds a symbol, edits the worktree, observes an explicit stale refusal, schedules the interactive refresh, and proves the next turn finds a symbol that did not exist when the graph was built |
| 3 (P2) | Per-entry attempt numbers restarted at one after the entry was forgotten, and could collide with a superseded job still buffered in a channel | Attempt IDs come from a server-wide monotonic counter, so a recreated entry can never reuse a number a buffered job still carries |
| 4 (P3) | A comment still claimed the gained capabilities included backlog bookkeeping | Corrected: the profile refuses backlog writes, skill proposals, patches, shell and file writes |
| 5 | `.openexec/` relied on a manual git exclusion | Added to Agent Console's `.gitignore` with the reason (the console registers itself, so dogfooding recreates it) |

Two probes in the new integration test were wrong before they were right, and
the corrections matter for reading it:

- Checking "did the lookup rebuild the graph?" with
  `openexec knowledge graph symbol` **rebuilt the graph**, because that command
  reads through the refreshing path. The probe reported its own side effect and
  failed the test. It now hashes the database file, observing without touching.
- Waiting for the scheduled refresh with the same command would have made
  step 4 pass even if the scheduler did nothing. It now waits for the database
  fingerprint to change, and the real proof is step 5 — a provider turn through
  the non-refreshing symbols profile.

## Review round 5 — findings and resolutions

| # | Finding | Resolution |
| --- | --- | --- |
| 1 (P1) | Dropping every interactive request while `runningInteractive` deduplicated a double-click correctly, but also discarded a refresh from a second workspace-write run that finished mid-scan — realistic on a large repository | Requests now carry a `refreshKind`. `refreshManual` during a running interactive scan is still dropped (nothing changed between two presses). `refreshAfterEdit` is **never** dropped for a running scan — that scan may have read the tree before the edit — and becomes a follow-up instead. The post-run and post-pull call sites use the edit-driven path. `TestPostEditRequestIsNeverDroppedForARunningScan` covers both halves |
| 2 (P1) | The delivery test waited on database existence and byte changes, both of which occur while a generation is still building | It now polls `store.GetRepositoryContext` for a persisted graph version: `""` → a version for the first refresh, then a *different* version for the second. That proves scan completion, promotion, HTTP publication and console persistence rather than the start of a scan |

The second correction changed the result, not just the wording: waiting on
publication took the test from 33 s to 51 s, which is the interval it had
previously been skipping. The database fingerprint is still used, but only for
the assertion it can support — that a stale lookup changed *nothing*.

## What changed

### openexec

| File | Change |
| --- | --- |
| `internal/mcp/symbols.go` (new) | `symbol_find`, `symbol_read`, `symbol_relations` tool defs + handlers, and the `SymbolIndex` seam |
| `internal/mcp/server.go` | `symbolIndex` field, `toolContext()` helper, advertisement when an index exists, three dispatch cases |
| `internal/mcp/broker.go` | The three tools allowed in every permission mode |
| `internal/toolset/registry.go` | Added to the five toolsets that already carry `grep` |
| `internal/cli/symbol_index.go` (new) | Adapter from `internal/knowledge` to the seam; `symbolIndexRoot` |
| `internal/cli/mcp_serve.go` | Wires the index; adds `--mode` and `--workspace` flags |
| `internal/mcp/schema_audit_test.go` | Extended per contract C3 |

### agent-console

| File | Change |
| --- | --- |
| `internal/providers/provider.go` | `MCPServerSpec` type; `TurnRequest.SymbolServer` |
| `internal/providers/claude.go` | Builds an `mcpServers` map so the symbol server registers in **every** mode, alongside the existing console approval server |
| `internal/server/symbol_server.go` (new) | `symbolServerFor` — the policy: when to offer it and how it is pinned |
| `internal/server/config.go` | `OpenExecBin` (`AGENT_CONSOLE_OPENEXEC_BIN`, default `openexec`) |
| `internal/server/server.go` | Sets `SymbolServer` on every turn |

## Design decisions a reviewer should weigh

**1. A seam, not an import.** `internal/mcp` does not import `internal/knowledge`.
The tools depend on a `SymbolIndex` interface; the composition root
(`internal/cli/symbol_index.go`) adapts the graph. This mirrors `memoryLoader`
and keeps the MCP package testable with a fake. Cost: an extra DTO layer.

**2. Read-only in every permission mode.** Justified the same way `read_file`
is: locating code is what a read-only session most needs. Note the tools *can*
trigger a graph refresh, because the V2.1 read gate recomputes a drifted
generation rather than answering from a stale one. That writes `.openexec`
bookkeeping, never workspace files — the same documented exception the backlog
writes rely on. This is stated in the broker comment; **reviewers who consider
that exception too broad should say so here**, since it is the one place this
change widens what a read-only session does.

**3. Registered in the toolsets that already carry `grep`.** The tools are the
cheaper substitute for grep, so they belong wherever grep is allowed. Without
this the toolset filter would deny them while `tools/list` advertised them.

**4. Pinned on argv, not env.** `symbolServerFor` passes
`--mode suggest --workspace <path>`. `mcp-serve` defaults to **auto-edit**
(`broker.go:71`), which permits `git_apply_patch`. A session Agent Console
labels read-only must not acquire a patch tool through a side channel. The pin
is a security control, so it must not depend on whether the agent CLI
propagates an env map from its MCP config — argv always survives. The
`--mode`/`--workspace` flags were added to `mcp-serve` for this reason and
outrank the environment.

**5. Gated on `.openexec` existing.** Agent Console runs agents against
arbitrary checkouts, most without an OpenExec project. Skipping avoids spawning
a process per turn to answer nothing, and `knowledge.NewStore` would otherwise
create `.openexec` in a directory the user never initialized.

## Security analysis

Agent Console spawns `mcp-serve --profile symbols --mode suggest --workspace <path>`.
Under that profile the capability delta is exactly three read-only tools:

| Capability | State | Evidence |
| --- | --- | --- |
| `symbol_find` / `symbol_read` / `symbol_relations` | available | acceptance checks 1–4 |
| `backlog_*` writes, `skill_propose` | denied at the broker | acceptance checks 5–7 |
| `git_apply_patch`, `run_shell_command`, `write_file` | denied at the broker | acceptance checks 8–10 |
| `memory_read`, `read_file`, control plane | denied at the broker | acceptance checks 11–12 |
| infra tools, approval tools | never wired — the profile returns before that code | code path |
| synchronous stale-graph scan | refused in ~2 ms | acceptance |
| any write to `.openexec` during lookups | none — byte-identical before and after three lookups | acceptance |
| database creation in an unscanned checkout | none — no tools advertised, no database created | acceptance |

The deny is enforced in `ToolBroker.Authorize`, not only in `handleToolsList`,
because advertisement is not authorization — a client that already knows a tool
name can call it without ever listing.

The earlier round's residual risk (Agent Console sessions reaching the backlog
and skill-proposal surface) is closed by the profile: those tools are now
refused rather than merely unadvertised.

## Contracts

- **C1** Extraction provider → store: unchanged.
- **C2** Store → query layer: unchanged; the adapter only reshapes
  `QueryEnvelope` and never widens a resolution tier or hides freshness.
- **C3** Knowledge → MCP plane: three tool defs and three request structs added
  to `allToolDefs()` and the struct-pair list in `schema_audit_test.go`.
- **C4** OpenExec → Agent Console: unchanged. The console UI still reaches the
  graph only through the proxy; this change gives its *agents* a separate stdio
  path scoped by `--workspace`.
- **C5** Indexer execution boundary: not applicable (no third-party indexers).
- **C6** Schema and compatibility: no migration; `make compat-test` passes.

## Verification evidence

Run in this session:

- openexec: `go build`, `go vet ./...`, `internal/mcp`, `internal/cli`,
  `internal/toolset`, and the `make compat-test` gate — all pass.
- agent-console: `go build`, `go vet ./...`, full `go test ./...` — all pass.
- **Live, over stdio JSON-RPC against a real 803-file / 9599-symbol graph**:
  `symbol_find` returned a pointer, `symbol_relations` returned a
  `compiler_exact` caller, `symbol_read` returned bounded source, and both
  refusal paths fired.
- **Live security pin**: `mcp-serve` spawned with the exact argv
  `symbolServerFor` generates, from cwd `/`, with a hostile ambient
  `OPENEXEC_MODE=danger-full-access` and `WORKSPACE_ROOT=/`. The flags won:
  symbol tools worked against the correct workspace, `git_apply_patch` and
  `run_shell_command` were refused.

Four defects were found by driving it rather than by unit tests, and fixed:
generation-wide limitations flooding every response (now capped with the
omitted count kept); `file:0` rendered for edges that carry byte offsets;
an unknown `symbol_id` wrongly reported "no graph, run scan"; and `symbol_read`
placing caveats above the source. A fifth — the composition root probing
`.openexec` under cwd while the server scopes to `WORKSPACE_ROOT`, which would
have made the tools silently absent for exactly the Agent Console case — is
covered by `TestSymbolIndexRootUsesResolvedWorkspace`.

### Acceptance test

`scripts/acceptance-symbols-profile.mjs` is the repeatable form of the
reviewer's requested acceptance run. It builds a real graph, drives the server
over stdio with exactly the argv `symbolServerFor()` generates, and asserts the
positives and every negative. **14/14 pass**, including the stale-graph refusal
(3 ms, versus a synchronous extraction) and Codex accepting the generated
overrides.

It is run with a hostile ambient environment — `OPENEXEC_MODE=danger-full-access`,
`WORKSPACE_ROOT=/`, cwd `/` — so it also proves the argv pins outrank both.

```
go build -o /tmp/openexec-bin ./cmd/openexec
node scripts/acceptance-symbols-profile.mjs
```

### Live round trip (console → provider → agent CLI → MCP)

`internal/providers/live_roundtrip_test.go` drives the real provider adapters
against the real CLIs, in a repository whose symbol name (`Zibbleflux`) exists
nowhere else — so a correct answer can only come from the graph, not from model
recall. Both pass:

```
--- PASS: TestLiveClaudeReachesSymbolTools   (Claude Code 2.1.220)
--- PASS: TestLiveCodexReachesSymbolTools    (codex-cli 0.114.0)
```

Codex's JSON stream shows the call and the server's reply verbatim:

```
"type":"mcp_tool_call","server":"openexec","tool":"symbol_find"
  → 1 match(es) for "Zibbleflux" — freshness: current
    main.Zibbleflux (function) main.go:3-3 [ast_exact]
```

Gated on `OPENEXEC_BIN`, so an ordinary `go test ./...` skips them:

```
OPENEXEC_BIN=/path/to/openexec go test ./internal/providers -run Live -v
```

The negatives are proven at the server (acceptance script, 14/14) rather than
through the CLIs, since a model declining to call a tool is not evidence the
tool is unreachable. The two together cover the reviewer's acceptance criteria.

### Deployment caveat: snap-packaged CLIs

A snap-packaged CLI gets a private `/tmp`, so a snap `codex` cannot see an
OpenExec binary or checkout under `/tmp`. The standalone Codex at
`~/.codex/packages/standalone/current/bin/codex` works; `/snap/bin/codex` does
not, failing with `No such file or directory (os error 2)` when the MCP server
is launched. An operator running the snap must keep both the binary and the
checkout where the snap can reach them. The live test skips rather than fails
on a snap path, and says why.

### Not verified

- **Deployment.** The running console is `/mnt/data1/agent-console` on `main`
  at `8acad70`; this work is uncommitted on `codex/graph-projection` in
  `/mnt/data1/projects/agent-console`, so the running UI does not contain it.
- **The blueprint agents' own path.** OpenExec's `loop/mcpconfig.go` points its
  agents at the full `mcp-serve`, not the symbols profile, and that path was
  not exercised live. The annotation fix applies to it, but whether its agents
  prefer the tools over grep is unmeasured.
- `internal/knowledge` has one **pre-existing** failure,
  `TestPythonIncrementalRefreshPreservesTypeScriptCapability`, which expects
  the fixture to find no TypeScript compiler but this host has a global one.
  Confirmed identical at HEAD in a clean worktree; unrelated to this change,
  but it will fail CI on any machine with `tsc` installed.

## Known gaps / follow-ups

1. **Agents may not use the tools.** Exposure is not adoption: if a model
   reaches for `grep` from habit, this changes nothing. The tool descriptions
   steer explicitly ("prefer this over grep/ripgrep"), but the implement-stage
   prompt was not changed. Measuring iterations-to-green before and after is
   the only way to know whether the thesis holds.
2. **Test selection is still unwired.** `graph_impact.go` computes
   `RelatedTest`; nothing feeds it to the blueprint test stage, so "given this
   change, run these tests" does not exist. This is the other half of the
   original motivation.
3. `mcp_serve.go` still resolves the infra allowlist, project config and
   approvals DB from `os.Getwd()` rather than the server's resolved root. That
   inconsistency predates this change and was left alone deliberately — those
   are security-relevant paths — but it is the same class of bug that was just
   fixed for the symbol index and is worth a follow-up.

## Refresh ownership (answered)

Round 1 left "who refreshes a stale graph?" open. Agent Console owns it, and
already did: after a successful workspace-write run, on the UI's manual
refresh, and on a daily sweep. What was missing was timeliness — a single FIFO
queue meant an interactive refresh could sit behind every registered checkout.

Interactive refreshes now run on a worker reserved for them, so the window in
which the symbol tools are stale after an edit is one refresh, not one sweep —
and no running scan is interrupted to achieve it. An interactive request for a
checkout the sweep has already queued promotes it rather than being discarded,
and the superseded sweep entry becomes a no-op so the checkout is scanned once.

Covered by `TestInteractiveRequestPromotesQueuedSweep`,
`TestInteractiveRequestDeduplicatesWhileRunning`, and
`TestSweepStillConvergesAlongsideInteractiveWork`.

MCP remains fail-fast. No synchronous refresh was reintroduced into the symbol
server, so a lookup can still be refused as stale — that is the intended
behaviour, and the scheduler is what closes the gap.

## Review checklist (round 3)

- [ ] Is `--profile symbols` the right boundary, or should the profile be a
      separate binary/subcommand so the wider surface cannot be reached by
      flag alone?
Round 3's three questions were answered: two concurrent checkouts is an
accepted initial cap of two, `partial` generations are served with their
limitations disclosed, and the cheap file pre-filter stands. Remaining:

- [ ] Peak memory under the accepted cap of two: two simultaneous TypeScript or
      Go extractions on large repositories were not measured on the deployment
      host.
- [ ] The follow-up path runs one extra scan after a sweep of the same
      checkout. Correct — the sweep may have read the pre-edit worktree — but it
      means an edit landing mid-sweep costs two scans of that repository.
- [ ] The integration test uses whichever non-snap CLI is installed first
      (Codex here). Claude is covered by the provider-level live test but not by
      the full scheduler path.
- [ ] The full (non-symbols) `mcp-serve` still allows the symbol tools in every
      permission mode, where a refresh *can* happen. Acceptable for OpenExec's
      own blueprint agents, which already have a workspace?
- [ ] Are `--mode` / `--workspace` / `--profile` the right shape, given they
      now outrank the environment?
- [ ] Is the `.openexec`-exists gate the right trigger, or should it be an
      explicit per-project opt-in?
- [ ] Dropping `Line` from `symbol_relations` costs precision. Worth adding
      back later by joining occurrences, or is file-level granularity right?
