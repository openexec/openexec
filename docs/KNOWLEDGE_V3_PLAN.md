# Knowledge Graph V3 — implementation plan

Status: V3.0 implemented; V3.1 onward proposed and awaiting review
Date: 2026-08-06
Related: `KNOWLEDGE_V2_PLAN.md` (V2 non-goals list is this plan's scope),
`REPOSITORY_POINTER_GRAPH_PLAN.md` (V1), `IMPACT_MANIFEST_FOR_CONSOLE_PLAN.md`,
Agent Console repository-graph proxy contract.

## Outcome

Two capabilities, stated as the questions a user asks:

1. **"If I change this, what breaks and where?"** — blast radius that reaches
   through interfaces, across service boundaries, and across repositories.
2. **"Show me how this works."** — dependency and call-flow diagrams plus
   generated documentation, each keyed to a graph version and refused when the
   graph is not current.

V3 is accepted when both questions are answered on a real repository pair
(openexec + agent-console) with a persisted, spot-checked report — not when the
features individually exist.

## Background — why now, and why this shape

V2 delivered a graph the system can defend: content-addressed generations,
read-time freshness enforcement, resolution tiering, and bounded query
envelopes that disclose their own limits. That is the hard part and it is done.

What V2 deliberately deferred is exactly what these two use cases need. Three
findings drove this plan:

**The graph cannot see interface dispatch.** The edge vocabulary is `calls`,
`references`, `imports`, `contains` (`graph_scan.go`). Change a Go interface
method and every implementer is affected, invisibly. `graph_impact.go` already
confesses this at runtime — it appends `"dynamic dependency injection and
runtime registration may not be resolved"` to every impact result. An unstated
gap became a stated one; V3 closes it.

**Blast radius stops at two hops.** `DefaultGraphLimits{MaxDepth: 2}`. Two hops
answers "who calls this", not "what breaks". Reachability is the whole question.

**The most consequential edges are not symbol edges.** Route → handler →
persistence, config key → reader, table/column → query site. A schema or route
change has enormous blast radius and zero footprint in a symbol graph. We just
lived the failure: `api/openapi.yaml` exists in both openexec and agent-console,
and changing the contract in one silently breaks the other.

### Why adopt SCIP rather than extend our extractors

We currently hand-write one extractor per language: Go compiler-exact
(`go_typed.go`), TypeScript compiler-exact (`ts_compiler.go`), Python
static-lexical (`polyglot_extract.go`, self-declared as not resolving
decorators or dynamic dispatch). Every new language is new code on a treadmill.

SCIP (Sourcegraph Code Intelligence Protocol) is the established interchange
format for exactly this data, with existing compiler-backed indexers for
TypeScript, Java, Python, Go, Ruby, C#, C/C++ and others. Adopting it as an
**input** format buys three things we need anyway:

- `implements` relationships → interface blast radius (finding 1)
- documentation strings → generated documentation (use case 2)
- language coverage without per-language code (removes the treadmill)

### Why SCIP does not replace our model

SCIP symbols are deterministic natural keys derived from package and descriptor
path. Ours are surrogate UUIDs (`sym_<uuid>`) with explicit continuity tracking
— `preserved / moved / renamed / ambiguous` — computed structurally in
`findOrCreateSymbol`. These are different jobs:

| | SCIP symbol | Our symbol ID |
| --- | --- | --- |
| Portable across machines and repositories | yes | no |
| Survives a rename | no — becomes a new symbol | yes — that is its purpose |
| Exists for a dirty, uncommitted worktree | no | yes |

An agent mid-task is *always* in a dirty worktree. SCIP therefore enters as an
**alias**, never as the primary key. It also has no notion of freshness,
resolution tiering, or bounded refusal — our novelty layer — and no notion of
control flow, so it does not serve flowcharts (see Non-goals).

### What is deliberately kept

Resolution tiering, the generation/freshness contract, symbol continuity, the
query envelope's `Limitations`/`Truncated`/`can_complete`, `RepositoryIdentity`,
and bounded traversal limits. V3 adds no capability that cannot state its own
resolution tier.

## Requirements

| # | Requirement | Justification |
| --- | --- | --- |
| R1 | Blast radius traverses interface implementation | Findings 1; today's impact is silently incomplete |
| R2 | Blast radius depth is configurable and defaults deeper than 2 | Reachability question, not neighbour question |
| R3 | Every impact answer states depth reached, truncation, and resolution mix | Existing invariant; must not regress under deeper traversal |
| R4 | Route, persistence and configuration edges are first-class | The highest-blast-radius changes are not symbol changes |
| R5 | Non-compiler-derived edges carry a lower resolution tier | A regex match on SQL is not ground truth |
| R6 | Blast radius spans repositories in one portfolio | openexec ↔ agent-console contract drift is a live failure |
| R7 | Documentation and diagrams are generated from facts, keyed to a graph version | Docs that drift are worse than absent docs |
| R8 | Generated artefacts refuse to render from a non-current graph | Matches V2.1 stale-refusal contract |
| R9 | Symbol documentation (doc comments) is captured | We parse comments today and discard them |
| R10 | New languages arrive by configuration, not code | Removes the extractor treadmill |

## Product language (extends V2)

- **Call flow** is a bounded call/dependency diagram. We do not say
  "flowchart"; V2.4 set this rule and V3 keeps it.
- **Blast radius** is the bounded reverse-reachable set from a changed symbol,
  with a path and reason per hop, and its truncation disclosed.
- **Authority order** ranks evidence for a derived edge, highest first. A lower
  authority never overwrites a higher one; it only fills absence.
- **Portfolio** is a named set of repositories queried as one graph.

## Non-negotiable invariants

1. No answer is served from a non-current generation without an explicit typed
   refusal (V2.1).
2. Every edge carries a `ResolutionStatus`. Derived edges are never presented
   as `compiler_exact`.
3. Bounded lists disclose truncation and selection scope; nothing bounded may
   present itself as complete.
4. Third-party indexers are executed as validated argv arrays, never via a
   shell, under the deny-by-default posture of `internal/infra`.
5. Legacy `.uaos/` and existing `.openexec/` projects keep loading; schema
   migrations are additive and `make compat-test` gates every phase.

## Phases (strictly ordered — each gates the next)

### V3.0 Connect the index to the agents  [implemented 2026-08-06]

The gap that motivated the whole plan was not missing capability but a missing
wire. `SymbolReaderTool` (`internal/tools/symbol_reader.go`) was registered
only in `newDCPCoordinator` (`internal/cli/start.go`), which runs only when
DCP is enabled. The V2.3 query API served the Agent Console over HTTP. The MCP
plane — the one serving coding agents, including Claude Code via `mcp-serve` —
exposed backlog, memory, skills, approval and infra tools and no graph access
at all. The pointer store built to stop agents grepping was reachable by the
human UI and the optional router, but not by the agents doing the grepping.

Delivered: `symbol_find`, `symbol_read` and `symbol_relations` on the MCP
plane (`internal/mcp/symbols.go`), backed by the existing
`FindGraphSymbols` / `ReadGraphSymbol` / `FindSymbolRelationships` queries
through a `SymbolIndex` seam adapted in the composition root
(`internal/cli/symbol_index.go`), so `internal/mcp` does not import
`internal/knowledge`. Added to the five toolsets that already carry `grep`,
and allowed in every permission mode.

Both agent populations are wired. OpenExec's own blueprint agents get the
tools automatically, because `internal/loop/mcpconfig.go` already points them
at `openexec mcp-serve`. Agent Console's spawned agents are wired separately
(`internal/server/symbol_server.go`, `internal/providers/claude.go`), pinned
to read-only mode on argv. Codex sessions are excluded — that CLI takes
`-c` overrides rather than `--mcp-config`.

Full change record and reviewer checklist: `SYMBOL_TOOLS_REVIEW.md`.

**Definition of done** — met
- Tools advertised only when the workspace has an index; a workspace without
  `.openexec` is never given one implicitly.
- Read-only in every mode, including suggest; graph refresh as a side effect
  is documented in the broker alongside the backlog exception.
- Freshness, resolution tier, truncation and limitations disclosed on every
  response; limitations capped with the omitted count retained.
- Schema audit extended per contract C3.
- Driven end to end against a real 803-file, 9599-symbol graph over stdio
  JSON-RPC: find returns a pointer, relations returns a compiler-exact caller,
  read returns bounded source, and both refusal paths were exercised.

**Not yet done, and deliberately deferred**: `RelatedTest` from
`graph_impact.go` is still not wired into the blueprint test stage, so
"given this change, run these tests" remains unavailable. That is the other
half of the original motivation and should precede V3.1.

### V3.1 Symbol natural key

Add `repository_symbol_aliases(symbol_id, alias_type, alias_value)` and teach
the existing Go and TypeScript extractors to emit SCIP-format symbol strings
alongside current output. Use the alias as the first exact-match probe in
`findOrCreateSymbol`, ahead of `priorKey(language, kind, qualified, file)`.

No user-visible change. This is foundational: identity is what every later
phase keys on, and the alias immediately improves continuity, because today a
file move breaks the exact-match key and downgrades to structural matching.

**Definition of done**
- Migration is additive; `make compat-test` passes.
- Alias is populated for every Go and TypeScript symbol in a full scan.
- A rename-plus-move regression test that currently reports `ambiguous` or
  `new` reports `preserved` or `renamed`.
- Symbol ID stability across two consecutive generations is unchanged for
  untouched files (no churn introduced).

### V3.2 SCIP as an extraction provider

`internal/knowledge/scip_import.go` maps a SCIP index into `GraphSymbol`,
`SymbolOccurrence`, `GraphEdge` and documentation, behind the existing
`provider.go` / `source.go` seam. Adds the `implements` edge type and a
`documentation` field on the symbol record. Indexer invocation goes through the
allowlist posture of invariant 4.

**Definition of done**
- Differential test: `scip-go` over openexec versus `go_typed.go` output —
  symbol sets agree within a documented, justified delta.
- `implements` edges present for Go interfaces and TypeScript classes.
- Documentation captured for exported symbols; empty is recorded as empty, not
  as missing.
- A project whose indexer is unavailable or whose build fails degrades to the
  existing extractor at its existing lower tier, with the reason surfaced in
  `Limitations` — never a hard failure.
- `ExtractorVersion` bumped; generations from the previous extractor are marked
  `incompatible`, not silently reused.

### V3.3 Blast radius that reaches

Traverse `implements` in `graph_impact.go`; raise `DefaultGraphLimits.MaxDepth`
and make depth a per-query parameter bounded by a server maximum; report depth
reached alongside `Truncated`.

**Definition of done**
- Changing an interface method returns every implementer and their callers.
- Impact response states depth requested, depth reached, truncation, and the
  distinct resolution methods contributing.
- The `"dynamic dependency injection..."` blanket caveat is narrowed to the
  cases that genuinely remain unresolved.
- Latency budget: impact query on the openexec repository stays within the
  suite's existing budget, or the budget is revised with justification.

### V3.4 Cross-boundary edges

Extract route, persistence and configuration edges with an explicit authority
order, per V2's deferred design:

| Edge | Authority order (highest first) |
| --- | --- |
| route → handler | OpenAPI spec → route registration → handler signature → labelled inference |
| symbol → table/column | migration/DDL → ORM model → query string analysis |
| symbol → config key | typed config struct → literal key reference → inference |

Tiers map to existing `ResolutionStatus` values: spec/registration derived →
`configuration_derived`; string analysis → `static_lexical`; inference →
`heuristic`. Never `compiler_exact`.

**Definition of done**
- Changing an `openapi.yaml` path returns its handler and that handler's callers.
- Changing a column returns the query sites that name it.
- Every such edge carries a sub-`compiler_exact` tier and names its authority
  source in edge metadata.
- A lower authority never overwrites a higher one — covered by test.

### V3.5 Portfolio graph

Query across repositories in one named portfolio: cross-repository edges join
on published contract identity (OpenAPI operation, shared schema), not on
symbol name coincidence.

**Definition of done**
- Changing a response schema in openexec surfaces the agent-console call sites.
- Cross-repository edges are tiered no higher than `configuration_derived`.
- A portfolio whose members have mismatched freshness refuses per-repository
  and says which member is stale, rather than answering partially in silence.

### V3.6 Generated documentation and diagrams

Generate architecture documentation and Mermaid diagrams from graph facts,
keyed to `graph_version`. Extends V2.4's existing Mermaid export rather than
replacing it.

**Definition of done**
- Output embeds the graph version, base commit and generation timestamp.
- Generation from a non-current graph refuses with the V2.1 typed refusal.
- Regenerating from an unchanged graph version is byte-identical (diffable in
  version control).
- Every generated sentence traces to a graph field; no invented scores — the
  V2.5 owner-summary rule applies unchanged.

### V3.7 Acceptance gate

A persisted report over the openexec + agent-console portfolio: an interface
change, a route-contract change, and a cross-repository schema change, each
with its blast radius, generated diagram, and manually verified findings.

**Definition of done**
- Report exists, is reproducible from its named graph version, and its
  spot-checked claims hold.
- Claims the system could not verify are listed as not verified, per the V2.6
  precedent — unavailable checks are never recorded as passed.

## Contracts between system parts

**C1 — Extraction provider → knowledge store.** A provider returns symbols,
occurrences and edges each carrying a `ResolutionStatus` and the file content
hash they were derived from. A provider may not write to the store directly and
may not invent a tier above what its evidence supports. Adding a provider must
not change existing providers' output for the same input.

**C2 — Knowledge store → query layer.** Every query returns
`QueryEnvelope[T]`: `Generation` (graph version and freshness), `Resolution`
(status and contributing methods), `Limitations`, `Truncated`, optional
`Pagination`. New query types extend this envelope; none may return a bare
result. Breaking change rule: adding a field is additive; changing the meaning
of `Freshness` or `ResolutionStatus` values is a `GraphSchemaVersion` bump.

**C3 — Knowledge → MCP tool plane.** New graph tools are registered in
`internal/mcp` with schema and struct kept consistent; `allToolDefs()` and the
struct-pair list in `schema_audit_test.go` must be extended in the same change.
Graph reads are permitted in read-only mode; graph *refresh* is a workspace
side effect and follows existing permission rules.

**C4 — OpenExec → Agent Console.** The console reaches the graph only through
the fixed proxy surface (`/api/projects/{id}/repository-graph/{path...}`),
which supplies the checkout identity already authorised for the signed-in user.
The console never receives an arbitrary checkout or upstream URL, never touches
SQLite, and renders freshness and limitations verbatim rather than
interpreting them. Adding a graph endpoint requires the proxy allowlist and
`api/openapi.yaml` to be updated together.

**C5 — Indexer execution boundary.** Third-party indexers are invoked as
`exec.CommandContext` argv arrays with validated arguments, no shell, resolved
against a configured directory, under the `internal/infra` deny-by-default
model. Indexer failure is a degraded tier plus a stated limitation, never a
partial graph presented as current.

**C6 — Schema and compatibility.** Migrations are additive. `.uaos/` and
existing `.openexec/` projects keep loading, proven by `make compat-test` in
every phase. An extractor change that alters facts bumps `ExtractorVersion`,
which marks prior generations `incompatible` rather than silently reusing them.

## Non-goals

- **Control-flow-graph extraction (true flowcharts).** Decision diamonds,
  loops and early returns require AST/IR-level extraction per language; SCIP,
  LSIF and Kythe do not provide it. Blast radius is a reachability question,
  not a branching question. We continue to say "call flow".
- Replacing the surrogate symbol ID with the SCIP symbol.
- Adopting RDF/OWL/SPARQL or a dedicated graph database. Access patterns are
  bounded-depth traversals; SQLite with recursive CTEs is sufficient and every
  comparable system (Kythe, Glean, CodeQL) chose relational or Datalog.
- SCIP *export*. Lower value than import; revisit only for external interop.
- LLM-inferred diagrams without the mandatory `inferred` label (V2 rule).

## Risks

- **Indexers need a working build.** `scip-java` needs the build to succeed;
  `scip-python` needs dependencies installed. In a dirty agent worktree this
  will often fail, which makes the fallback tiers load-bearing rather than a
  nicety. V3.2 must not delete the existing extractors.
- **Deeper traversal changes the performance profile.** V3.3 touches the same
  hot paths V2.1 did; regressions present as slowness or spurious truncation.
  Keep the scan benchmark and add an impact latency budget.
- **Cross-boundary extraction is where false confidence enters.** R5 and the
  authority order exist to contain it; review V3.4 tiering specifically.
- **Portfolio scope can expand without limit.** V3.5 ships with two
  repositories and the openexec/agent-console contract only.
- **Indexer supply chain.** Third-party binaries executing against source is a
  new trust surface; C5 is the mitigation and should be reviewed as a security
  change, not a build change.
- **Indexer maturity varies by language and moves quickly.** Verify current
  coverage and quality per language before committing V3.2 to a language list.
