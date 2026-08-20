# Repository Pointer Graph and Verification Evidence Plan

**Status:** Proposed for implementation  
**Scope:** OpenExec V1A-V1C, with a versioned Agent Console read model  
**Last updated:** 2026-08-03

## Governing statement

The repository graph is a versioned structural read model inside OpenExec. It
informs navigation and validation planning, but it does not own source access,
execution policy, evidence, completion authority, or workspace identity.

This plan hardens and extends OpenExec's existing operational-memory pointer
records. It does not create a separate pointer-graph product, database,
validation runner, artifact store, or task-completion authority.

## Objective

Give agents deterministic, bounded repository structure so they can:

- resolve named symbols without relying on broad probabilistic search;
- retrieve the current source range through existing repository authority;
- understand module-level structure and, later, bounded symbol-level impact;
- identify likely related tests without treating structural proximity as proof;
- propose validation scope while OpenExec retains policy and execution authority;
- bind planning, implementation, validation, and completion claims to identifiable
  repository states;
- expose uncertainty and fall back to normal repository inspection when graph
  assistance is unavailable.

Version 1 succeeds only if it improves affected-file identification or prevents
unsupported verification claims without reducing task success, widening
authority, or creating stale-graph false confidence.

## Current baseline

OpenExec already contains the system this plan extends:

- `.openexec/openexec.db` is the canonical transactional state store;
- `symbols` stores name, kind, file, line range, purpose, and signature;
- the current symbol name is the primary key and therefore cannot safely
  represent same-named symbols;
- the current indexer provides Go AST extraction and pattern-based
  TypeScript/JavaScript extraction;
- `read_symbol` resolves pointer metadata but does not yet read current source;
- run steps, quality gates, content-addressed artifacts, and execution capture
  already provide the foundations of authoritative validation evidence.

The implementation must migrate these capabilities additively. Existing
OpenExec projects must remain usable throughout the transition.

## Scope

### V1A: trustworthy navigation

- additive migration from current symbol records;
- repository, checkout, worktree, and workspace identities;
- scan manifests and dirty-worktree freshness;
- atomic graph generations;
- TypeScript compiler-based definitions, modules, imports, and exports;
- existing Go AST definitions and imports, hardened for the new schema;
- repositories, packages, modules, symbols, and occurrences;
- containment and module/package dependency edges;
- bounded symbol and module-dependency queries;
- hash-validated source retrieval through repository authority;
- safe degraded mode.

### V1B: bounded impact

- explicit invalidation rules and incremental refresh;
- reverse-import closure;
- compiler-resolved references where supported;
- statically resolvable call relationships;
- structural test relationships;
- explainable bounded impact paths;
- validation recommendations;
- explicit incompleteness and fallback behavior.

### V1C: governance and presentation

- planning and validation generation binding;
- immutable validation-plan revisions;
- linkage to existing OpenExec execution and artifacts;
- evidence coverage and typed completion claims;
- unsupported-claim prevention;
- Agent Console versioned read model;
- publishing and API compatibility contract;
- controlled baseline-versus-treatment evaluation.

### Explicitly excluded from Version 1

- a separate graph database or service;
- graph-owned command execution or validation receipts;
- copied repository graphs per Agent Console workspace;
- semantic purpose inference, pseudocode, or generated architecture documents;
- automatic refactoring, deduplication, or dead-code deletion;
- comprehensive runtime-impact claims;
- centrality-based context selection;
- Mermaid or DOT as a delivery gate;
- equal extraction sophistication for every supported language;
- model fine-tuning.

## Authority boundaries

### Repository graph

The graph owns graph generations and scan manifests; structural nodes,
occurrences, edges, provenance, and limitations; bounded queries and impact
explanations; validation recommendations; and references to the repository
state from which graph entities were extracted.

The graph does not own source bytes, authorization, execution, evidence, or
completion decisions.

### Repository authority

OpenExec's repository/session authority owns path authorization and
repository-root scoping, symlink and traversal protection, reading current file
bytes, verifying expected hashes, and enforcing session permissions. The
current codebase has filesystem access rules and worktree isolation, but no
single hardened source-range reader is yet the obvious authority for all of
these checks. Phase 0 must inventory the existing paths and Phase 3 must expose
one narrow `RepositoryReader` adapter if no reusable implementation exists.

The graph depends on that interface. It must not call `os.ReadFile` directly or
become a source-code cache merely because source retrieval is presented through
the same agent tool.

Conceptually, source retrieval is:

```text
graph.resolve(symbol_id)
-> repository.read_range(file, range, expected_file_hash, expected_range_hash)
```

`read_symbol` may present this as one tool, but the internal authority boundary
must remain explicit.

### OpenExec orchestration

OpenExec owns task and run contracts, validation-plan acceptance, execution
permissions and quality-gate policy, command execution, artifacts and receipts,
evidence eligibility, typed completion claims, and completion gates.

Graph recommendations are advisory until accepted by an OpenExec blueprint,
policy, user, or other authorized validation-plan owner.

### Agent Console

Agent Console owns presentation, navigation, and user interaction. It stores
stable OpenExec references and cacheable, deliberately lossy projections. It
does not store authoritative source ranges, graph generations, validation-plan
authority, evidence copies, or mutable graph annotations.

## Contract 1: identity hierarchy

The four identities have different meanings and lifetimes:

| Identity | Meaning | Lifetime |
|---|---|---|
| `repository_id` | Logical source lineage | Long-lived |
| `checkout_id` | One local clone or materialization | Until checkout removal |
| `worktree_id` | One independently mutable worktree | Until worktree removal |
| `workspace_id` | Agent Console access and presentation context | Product-managed |

`repository_id` must not be derived solely from a remote URL. Remote URLs are
discovery hints and aliases, not durable authority.

Repository identity resolution order:

1. an existing OpenExec repository identity;
2. a repository-local persisted UUID;
3. clone metadata linking the checkout to a known repository;
4. normalized remote identity as a discovery hint;
5. creation of a new logical repository identity.

A fork normally receives a new `repository_id` and records
`forked_from_repository_id`. Multiple clones and worktrees may share the same
logical repository while retaining distinct checkout and worktree states.

One repository may appear in multiple Agent Console workspaces without graph
duplication. A workspace is an access and context projection over repository
references.

### Symbol identity and lineage

Human-readable locators are not primary keys. Each symbol has an opaque,
repository-scoped `symbol_id`. Its source representation is a versioned
occurrence:

```yaml
symbol:
  symbol_id: sym_opaque
  repository_id: repo_opaque
  language: typescript
  kind: function
  display_name: authenticateUser
  qualified_name: auth.service.authenticateUser

occurrence:
  graph_version: graph_opaque
  file_path: src/auth/service.ts
  start_line: 84
  end_line: 137
  start_byte: 2140
  end_byte: 3628
  signature: "authenticateUser(email: string, password: string): Promise<Session>"
  file_content_hash: sha256:...
  source_range_hash: sha256:...
```

Identity continuity is traceable rather than assumed:

```yaml
continuity:
  status: preserved | moved | renamed | split | merged | ambiguous | new
  previous_symbol_ids: []
  resolution_method: exact | structural | heuristic | reviewed
```

Matching may use qualified name and file, signature and containing structure,
structural hash, content hash, and conservative rename heuristics. Ambiguous
transformations create new identities plus lineage records. The system must
never attach an old identity to an unrelated symbol merely to preserve
continuity.

## Contract 2: repository state and graph generations

A commit hash alone is not a freshness boundary because agents normally work
against uncommitted files. Every query identifies:

```yaml
repository_state:
  repository_id: repo_opaque
  checkout_id: checkout_opaque
  worktree_id: worktree_opaque
  base_commit: abc123
  worktree_state_hash: def456
  configuration_digest: cfg789
  extractor_version: extractor-v1
  graph_version: graph_opaque
  freshness: current | stale | partial | inconsistent | incompatible | missing
```

The worktree hash covers the indexed input manifest rather than pretending to
describe ignored or inaccessible files.

### Scan manifest

The manifest includes:

- relevant tracked and untracked source files;
- normalized repository-relative paths;
- file type, size, and content hash;
- symlink resolution and boundary result;
- TypeScript configuration and compiler options;
- package manifests and lockfiles;
- Go module and workspace configuration;
- extractor version;
- ignore rules and generated-file policy;
- the included and excluded file sets.

### Atomic generation lifecycle

```text
capture input manifest
-> build unpublished generation
-> resolve configuration and relationships
-> re-hash manifest inputs
-> compare inputs
-> validate generation
-> atomically promote
```

Generation statuses are `building`, `current`, `stale`, `partial`,
`inconsistent`, `failed`, `incompatible`, and `superseded`.

Only a successfully validated generation with stable inputs may become current.
An inconsistent generation is never served as active. The previous active
generation remains readable until promotion succeeds.

## Contract 3: non-destructive migration

Opening an existing database must never perform a destructive migration before
the new schema has been populated and validated.

```text
detect legacy symbols schema
-> create additive tables
-> backfill a migration generation
-> compare legacy and new resolution results
-> enable compatibility resolver
-> switch the default reader
-> retain legacy storage for rollback
```

Required compatibility behavior:

- existing `.openexec/openexec.db` files open without manual repair;
- legacy pointer records remain readable during migration;
- migration is restart-safe and idempotent;
- migration failure leaves the previous reader usable;
- repeated migrations do not create duplicate identities;
- name-only lookup never silently chooses among multiple candidates;
- rollback does not require reconstructing deleted legacy data.

Ambiguous name resolution returns candidates:

```yaml
resolution_status: ambiguous
candidates:
  - symbol_id: sym_one
    display_name: Run
    safe_location: internal/a/service.go:42
  - symbol_id: sym_two
    display_name: Run
    safe_location: internal/b/service.go:18
```

Legacy storage may be removed only through a later, explicit deprecation with
compatibility evidence.

## Contract 4: structural model and resolution provenance

### V1A nodes

- repository;
- package;
- module;
- symbol;
- external package.

Symbol kinds initially include function, method, class, struct, interface,
type, exported variable, constant, and meaningful registered or exported
handler. Anonymous callbacks are not top-level nodes unless they represent a
stable registered behavior.

### V1A relationships

```text
repository contains package
package contains module
module contains symbol
module imports module
module exports symbol
package depends on package
```

V1A calls these module dependencies. It does not claim symbol-level dependency
or impact coverage.

### V1B relationships

```text
symbol references symbol
symbol calls symbol
test structurally relates to symbol
type inherits type
type implements interface
method overrides method
route resolves to handler
```

V1B edges are implemented only where the language extractor can explain the
resolution. Framework-specific routes are opt-in extractor capabilities rather
than a universal graph promise.

Every relationship records its origin occurrence, target, extraction method,
source location when applicable, graph version, and resolution status.

User-facing resolution categories are `compiler_exact`, `ast_exact`,
`configuration_derived`, `static_lexical`, `heuristic`, and `unresolved`.
Numeric confidence may be used internally for ranking only if calibrated. It
must not replace explainable provenance in query results.

## Contract 5: bounded query and retrieval

Initial query capabilities:

```text
resolve_symbol(name, file?, kind?)
read_symbol(symbol_id)
find_module_dependencies(module_id, depth=1)
find_module_dependants(module_id, depth=1)
```

V1B adds:

```text
find_symbol_references(symbol_id, depth=1)
find_callers(symbol_id, depth=1)
find_related_tests(symbol_id, depth=2)
impact_analysis(changed_symbol_ids, max_depth=2)
validation_recommend(change_identity)
impact_explain(impact_result_id)
```

Each response uses a common envelope:

```yaml
query:
  type: impact_analysis
  roots: [sym_123]

generation:
  graph_version: graph_456
  freshness: current
  base_commit: abc123
  worktree_state_hash: def456

result:
  nodes: []
  paths: []

resolution:
  status: partial
  methods: [compiler_exact, static_lexical]

limitations:
  - dynamic dependency injection not resolved

truncated: false
```

Server-side limits apply to depth, nodes, edges, bytes, execution time, and
source ranges. Results are deterministically ordered and paginated where
necessary. Cycles cannot cause unbounded traversal.

### Retrieval security

- paths are normalized and repository-relative;
- absolute paths are never accepted from agent tool input;
- traversal and symlink escapes are rejected;
- repository and worktree access is inherited from the active session;
- source retrieval verifies current file and range hashes;
- a hash mismatch causes re-resolution or a stale result, never a best-effort
  read at the old lines;
- generated, vendor, declaration, and ignored-file policies are explicit;
- graph availability never widens filesystem or network authority.

## Contract 6: invalidation and incremental refresh

Invalidation events carry a scope and cause:

```yaml
invalidation:
  scope: file | reverse_dependencies | package | repository
  cause: source_change | config_change | dependency_change | extractor_change
```

Inputs include added, modified, renamed, deleted, and untracked files. Git is a
discovery source, not the complete truth about worktree state.

| Change | Minimum invalidation |
|---|---|
| File body with unchanged exports | File and outgoing edges |
| Export or import change | File plus reverse-import closure |
| File rename or deletion | File, importers, and affected package |
| Barrel/re-export change | Module plus reverse-import closure |
| `tsconfig` or compiler-option change | Package or repository rescan |
| Package manifest or lockfile change | Package dependency graph and affected packages |
| `go.mod` or `go.work` change | Module/workspace rescan |
| Extractor or schema version change | Full compatible rebuild |
| Ignore/generated policy change | Full manifest comparison and required rescan |

V1A may use full scans while proving correctness. V1B introduces incremental
refresh only after the invalidation matrix has fixtures and a full-scan oracle.
An incremental generation must produce the same externally observable graph as
a clean full scan of the same inputs.

## Contract 7: planning and validation repository states

Tasks bind planning and validation separately:

```yaml
repository_state:
  planning:
    graph_version: graph_before
    base_commit: abc123
    worktree_state_hash: state_before

  validation:
    graph_version: graph_after
    base_commit: abc123
    worktree_state_hash: state_after
    patch_hash: patch_456
```

Every execution receipt records:

```yaml
executed_against:
  repository_id: repo_opaque
  checkout_id: checkout_opaque
  worktree_id: worktree_opaque
  base_commit: abc123
  worktree_state_hash: state_after
  patch_hash: patch_456
```

Evidence is eligible for a completion claim only when it matches the accepted
validation repository state. A subsequent worktree change makes earlier
evidence ineligible unless an explicit policy proves and records safe evidence
reuse. Evidence is never silently rebound to newer code.

## Contract 8: validation recommendations and accepted obligations

```text
graph recommendation
-> OpenExec validation-plan proposal
-> blueprint, policy, user, or other authorized acceptance
-> governed execution
-> evidence coverage
-> typed completion claims
```

A validation item records:

```yaml
validation_item:
  id: validation_item_opaque
  source: graph | blueprint | policy | user | agent
  disposition: suggested | accepted | rejected
  requirement: optional | required | blocking
  criterion: selected authentication tests pass
  repository_state: state_after
  graph_paths: []
  limitations: []
```

An accepted validation plan is immutable for one validation attempt. Changes
create a new revision with lineage rather than silently changing the completion
contract.

The graph does not execute commands. OpenExec resolves authorized commands from
project configuration, blueprints, quality gates, policy, and explicit user
input. Existing run steps and content-addressed artifacts remain authoritative.

## Contract 9: evidence coverage and typed claims

V1C adds read-only graph-to-evidence comparison:

```text
evidence_coverage(validation_plan_revision)
completion_claim_report(validation_attempt)
```

Evidence coverage compares accepted obligations with eligible existing
OpenExec evidence. It does not create a second execution record or artifact.

```yaml
claim:
  predicate: validation_item_passed
  validation_item_id: validation_item_opaque
  scope: affected_authentication_tests
  status: supported | unsupported | inconclusive | not_run | unavailable
  evidence_artifact_ids: []
  repository_state: state_after
```

Human-readable reports are rendered from typed claims:

```text
Verified:
- Selected authentication unit and route tests passed.

Not verified:
- The full repository test suite was not run.
- Runtime dependency-injection paths were not exercised.
```

Free-form agent assertions cannot promote an unsupported claim. Broad phrases
such as "everything works," "fully verified," "no regressions," or "all
affected functionality passed" are rejected or normalized unless accepted
obligations and eligible evidence support their exact scope.

## Contract 10: degraded mode

Graph failure is a normal operating condition, not an exceptional path.

| Graph state | Required behavior |
|---|---|
| Current | Use the graph normally |
| Stale but readable | Show stale status; use only as a discovery aid |
| Partial | Use resolved portions and disclose missing coverage |
| Inconsistent | Do not use for impact conclusions |
| Incompatible | Trigger rebuild or fall back |
| Missing | Use standard repository inspection |
| Required symbol unresolved | Fall back to search; block only when accepted policy requires exact resolution |

Central invariants:

- graph absence may reduce assistance but must not silently reduce repository
  inspection;
- missing edges never mean "no impact";
- stale or partial data cannot support completeness claims;
- fallback uses the same repository authority and permissions;
- graph failure blocks completion only when an accepted criterion explicitly
  requires valid graph evidence or a required pointer cannot otherwise be
  resolved.

Telemetry records the fallback reason and subsequent repository searches so the
evaluation can distinguish safe degradation from silent graph dependence.

## Contract 11: Agent Console read model

OpenExec publishes a versioned, deliberately lossy projection:

```yaml
repository_context:
  schema_version: 1
  source_system: openexec
  repository_id: repo_opaque
  checkout_id: checkout_opaque
  worktree_id: worktree_opaque
  graph_version: graph_opaque
  freshness: current

  resolved_symbols:
    - display_name: authenticateUser
      kind: function
      safe_location: src/auth/service.ts:84
      resolution_status: compiler_exact

  module_dependencies: []
  validation_summary:
    verified: []
    not_verified:
      - "Repository validation is unevaluated: no completion report was supplied."
    can_complete: false
  limitations: []

  openexec_reference:
    task_id: task_opaque
    run_id: run_opaque
    resource_version: 1
```

Agent Console uses this projection in task triage, conversations,
implementation activity, and completion views. It persists stable references
and safe presentation summaries only. Unknown compatible fields are ignored;
incompatible schema versions fail visibly rather than being interpreted as
current data.

An Agent Console workspace may expose selected repositories to different
conversations, but it cannot use workspace membership to expand an OpenExec
session's repository authority.

## Proposed canonical storage

The exact SQL belongs in implementation migrations, but the logical tables are:

```text
repositories
repository_aliases
repository_lineage
checkouts
worktrees
graph_generations
graph_scan_inputs
graph_scan_errors
graph_nodes
symbols
symbol_occurrences
symbol_lineage
graph_edges
task_graph_bindings
validation_plan_revisions
validation_items
validation_evidence_links
completion_claims
```

These tables live in the canonical OpenExec database and reference existing
tasks, runs, run steps, and artifacts. They do not copy artifact bodies or
execution status.

Graph-generation publication is transactional. SQLite WAL configuration,
foreign keys, migration ordering, retention, and indexes follow the existing
canonical store conventions. The active generation and any generation
referenced by a live task, run, validation plan, claim, or retained receipt
cannot be garbage-collected.

## Implementation connection map

The plan extends existing seams and assigns one owner to each new contract:

| Concern | Existing connection | Planned authority |
|---|---|---|
| Canonical schema and artifacts | `pkg/db/state/schema.go`, `pkg/db/state/store.go` | `pkg/db/state` migrations and transactions |
| Pointer records and indexing | `internal/knowledge/store.go`, `internal/knowledge/indexer.go` | `internal/knowledge` graph model, store, and query facade |
| Language extraction | `internal/knowledge/provider.go` | Extractors behind a capability-reporting interface |
| Agent symbol tool | `internal/tools/symbol_reader.go` | Tool orchestration over graph resolution plus `RepositoryReader` |
| Worktree isolation | `internal/worktree` | Existing worktree owner; graph stores references only |
| Context assembly | `internal/context` and blueprint stage inputs | Bounded graph context adapter, not graph-owned prompts |
| Validation execution | `internal/quality`, `internal/blueprint`, pipeline execution | Existing OpenExec execution owners |
| Evidence | Run steps and content-addressed `artifacts` | Existing canonical state store |
| Agent Console | Versioned external publishing seam | OpenExec projection publisher; Agent Console presentation cache |

Package boundaries must prevent a new graph monolith:

```text
language extractors
-> normalized extraction batch
-> graph generation builder
-> canonical graph store
-> bounded query facade

agent tools/context/validation recommendation
-> bounded query facade only

graph source retrieval
-> RepositoryReader authority interface

validation execution and evidence
-> existing OpenExec orchestration and state packages
```

Extractors do not import task, runner, UI, or Agent Console packages. Agent
Console adapters do not import extractor internals. Validation execution does
not depend on a graph implementation; it accepts an immutable validation-plan
revision so degraded fallback remains possible.

## Extractor architecture

Extractors implement a language-neutral interface and publish capabilities:

```text
definitions
imports
exports
references
calls
inheritance
tests
routes
```

A query may rely only on capabilities reported for the current generation.

### TypeScript and JavaScript

V1A replaces pattern-only graph extraction with the TypeScript compiler API or
a thin maintained wrapper. It loads applicable `tsconfig` files and project
references and records module-resolution inputs. Repositories without
TypeScript configuration receive an explicit inferred configuration and
limitation, not an undocumented default.

V1A covers definitions, imports, exports, and containment. V1B adds references
and calls where compiler resolution is available. JavaScript support declares
whether `allowJs`/`checkJs` and inferred projects were used.

### Go

V1A reuses and hardens the existing Go AST provider for definitions, packages,
files, imports, containment, signatures, and occurrences. V1B may use Go type
information for references, calls, interfaces, and tests. Basic Go coverage is
required so Agent Console and OpenExec are not presented as frontend-only
graphs.

## Delivery phases

### Phase 0: contracts and fixtures

Deliver versioned schemas for identities, repository states, query envelopes,
validation items, typed claims, and Agent Console projection; migration,
ambiguity, dirty-worktree, symlink, multi-worktree, configuration-change, and
degraded-mode fixtures; a full-scan comparison format; performance budgets; and
a compatibility inventory of current pointer consumers.

Exit:

- every contract has one named authoritative package;
- schemas reject invalid state combinations;
- review agrees which legacy behavior is protected;
- no implementation phase depends on an unresolved identity or authority
  decision.

### Phase 1: V1A schema and migration

Deliver additive canonical migrations; identity, generation, symbol,
occurrence, and lineage persistence; idempotent backfill; compatibility
resolution; ambiguity-aware lookup; and rollback-safe behavior.

Exit:

- an existing database opens and migrates without losing legacy resolution;
- migration interruption resumes safely;
- duplicate names return candidates;
- legacy storage remains available for rollback;
- compatibility tests cover existing `.openexec` projects.

### Phase 2: V1A extraction and generation publication

Deliver scan manifests; TypeScript definitions/imports/exports; hardened Go
definitions/imports; containment and module dependencies; generation
validation; atomic promotion; and freshness reporting.

Exit:

- repeated full scans of unchanged inputs are structurally deterministic;
- mutation during scanning cannot create a current mixed generation;
- config, lockfile, module, untracked, rename, and deletion fixtures behave as
  specified;
- a failed scan leaves the previous active generation readable;
- a clean full scan is the correctness oracle.

### Phase 3: V1A bounded query and source retrieval

Deliver symbol and module query APIs, the common query envelope, deterministic
limits, `read_symbol` through repository authority, hash mismatch handling, CLI
JSON output, and degraded fallback.

Exit:

- supported Go and TypeScript symbols resolve to current source;
- ambiguous, stale, inaccessible, escaped, and changed-range cases fail
  honestly;
- traversal and symlink tests pass;
- missing graph data triggers normal repository inspection;
- existing permission modes are not widened.

### Phase 4: V1B incremental refresh and impact

Deliver the invalidation engine, reverse-import closure, incremental refresh,
supported references/calls/tests, bounded impact paths, limitations, and
validation recommendations.

Exit:

- incremental and clean full scans are observably equivalent;
- each impact item explains its graph path and method;
- deliberately missing dynamic edges are disclosed;
- test links are labelled as selection hints;
- no result claims completeness outside declared capabilities.

### Phase 5: V1C validation and claim governance

Deliver planning and validation graph bindings, immutable plan revisions,
accepted obligations, existing evidence linkage, eligibility checks, typed
claims, and normalized reports.

Exit:

- evidence from different code cannot satisfy an obligation;
- changing a plan creates a revision;
- an unexecuted check cannot be rendered as passed;
- changing code invalidates or explicitly requalifies old evidence;
- verified and not-verified results round-trip through persistent state.

### Phase 6: V1C Agent Console integration

Deliver a versioned adapter; repository context in task triage; bounded context
references in conversations; graph-state presentation; receipt-linked claims;
and unavailable, stale, partial, and incompatible states.

Exit:

- Agent Console restarts and reloads the same references and status;
- cached projections never override newer OpenExec resource versions;
- one repository appears in multiple workspaces without graph duplication;
- workspace membership cannot expose an unauthorized repository;
- schema compatibility behavior is tested.

### Phase 7: controlled evaluation

Deliver a preregistered task set over at least two repositories and frontend
and backend changes; paired baseline and treatment runs; independent affected-
file and validation ground truth; failure trials; and a build, revise, or stop
recommendation.

Exit:

- outcomes and guardrails are measured from persisted artifacts;
- graph overhead is included in cost and duration;
- failures degrade to inspection rather than false certainty;
- further investment follows the predefined decision rule.

## Test and proof matrix

Unit tests prove local parsing and transitions. Version 1 is accepted only
through complete journeys against persisted state.

| Contract | Required proof |
|---|---|
| Repository identity | Clone, second clone, worktree, fork, no-remote, and remote-alias fixtures |
| Symbol identity | Same name, method, move, rename, split, merge, delete/recreate, ambiguity |
| Scan consistency | Mutation during scan, config mutation, extractor upgrade, interrupted scan |
| Migration | Legacy DB copy, interrupted backfill, retry, ambiguity, rollback reader |
| Extraction | Repeated deterministic scans for Go and TypeScript fixtures |
| Invalidation | Added, untracked, renamed, deleted, barrel, config, lockfile, module changes |
| Retrieval security | Traversal, absolute path, symlink escape, unauthorized repository, hash mismatch |
| Bounded queries | Cycles, node/edge/byte/time limits, pagination, deterministic ordering |
| Degraded mode | Stale, partial, inconsistent, incompatible, missing, unresolved required symbol |
| State binding | Planning differs from validation; evidence matches only validation state |
| Validation authority | Suggested/rejected/accepted items; immutable revision; blocking requirement |
| Evidence eligibility | Unrun, pass, fail, inconclusive, missing artifact, changed worktree, explicit reuse |
| Claims | Supported, unsupported, not-run, inconclusive, unavailable, prose normalization |
| Agent Console | Publish, reload, restart, stale cache, schema compatibility, multi-workspace |

### End-to-end acceptance journeys

1. Open a legacy database, migrate, resolve legacy symbols, encounter a
   same-name ambiguity, select a candidate, restart, and resolve it again.
2. Scan a dirty TypeScript worktree, resolve and read a symbol, edit the file,
   reject the stale range, refresh, and retrieve the new range.
3. Make the extractor incompatible, request task context, observe the
   limitation, and continue with normal authorized repository inspection.
4. Bind planning state, change a Go or TypeScript symbol, refresh, inspect an
   explainable affected set, and produce validation recommendations.
5. Accept obligations, execute them through OpenExec, persist artifacts, and
   render exactly what was and was not verified.
6. Modify the worktree after a passing check and prove that old evidence no
   longer supports the completion claim.
7. Publish task context and claims to Agent Console, restart both products, and
   confirm stable references and correct authority.

## Evaluation and go/no-go

### Primary outcome 1: affected-file identification

Measure recall of genuinely affected files. Graph assistance must improve
recall or preserve it while materially reducing repository exploration.

Record affected files found and missed, files opened, searches, graph paths,
task success, post-completion regressions, tokens, tool calls, duration, and
graph overhead.

### Primary outcome 2: unsupported verification claims

Measure completion claims not supported by eligible executed OpenExec evidence.
The target after typed-claim normalization is zero.

### Stale-graph safety outcome

When graph data is stale, partial, inconsistent, incompatible, or missing, the
system must fall back to repository inspection rather than confidently produce
an incorrect bounded result.

### Guardrails

Graph assistance must not lower task success, cause stale-location edits, hide
uncertainty, widen authority, materially increase cost without compensating
benefit, or require unnecessary full scans during normal operation.

Proceed beyond Version 1 only when a predefined primary outcome improves
materially, neither primary outcome regresses, safety guardrails pass, and
maintenance overhead remains proportionate to the measured benefit. Do not
proceed because diagrams look useful.

## Observability

Record structured, bounded telemetry per task: graph and extractor versions,
queries and resolution statuses, safe locations returned, scan/update duration,
fallback and resolution failures, subsequent repository searches, validation
recommendations and accepted obligations, evidence eligibility, claims rejected
or normalized, and truncation limits reached.

Telemetry follows repository access and retention policy and does not copy
source content merely for measurement.

## Risks and controls

| Risk | Control |
|---|---|
| Locator treated as identity | Opaque IDs and versioned occurrences |
| False dirty-worktree freshness | Manifest hash and pre-promotion revalidation |
| Mixed generations | Unpublished build and transactional promotion |
| Destructive migration | Additive backfill, compatibility reader, rollback data |
| Missing edge means no impact | Capabilities, provenance, limitations, degraded mode |
| Graph becomes execution authority | Recommendations only; OpenExec accepts and executes |
| Evidence belongs to different code | State and patch binding on receipts |
| Agent Console duplicates truth | Lossy projection and OpenExec references only |
| Graph reads beyond scope | Repository authority and bounded normalized reads |
| Incremental drift | Full-scan oracle and invalidation fixtures |
| Semantic scope creep | Explicit exclusions and phase exits |

## Reviewer checklist

1. Does every stateful record have one authoritative owner?
2. Are repository, checkout, worktree, workspace, graph, and symbol identities
   distinct in every API and table?
3. Can a generation become current while any scan input changed?
4. Can an existing database lose pointer access during migration?
5. Can name-only lookup silently choose the wrong symbol?
6. Can graph failure reduce normal repository inspection without disclosure?
7. Can a recommendation execute without becoming an accepted obligation?
8. Can evidence from different code satisfy a completion claim?
9. Can Agent Console become a second source of graph or evidence truth?
10. Can a graph query read beyond session repository authority?
11. Does every impact result expose provenance, freshness, limitations, and
    truncation?
12. Does released end-to-end proof cover persistence, reload, failure, and safe
    fallback rather than only components?

## Version 1 Definition of Done

Version 1 is complete when an existing OpenExec project can be migrated without
losing pointer access; a task can bind planning and validation to identifiable
repository states; current source can be retrieved through existing repository
authority; bounded impact recommendations can become explicit OpenExec
validation obligations; executed evidence can support typed completion claims;
Agent Console can display the result without becoming another authority; and
stale or incomplete graph data demonstrably falls back safely.

The released workflow must operate end to end:

```text
existing repository
-> additive migration
-> consistent graph generation
-> bounded symbol/module context
-> planning-state binding
-> implementation
-> post-change graph refresh
-> explainable impact recommendation
-> accepted validation-plan revision
-> governed OpenExec execution
-> eligible existing evidence
-> typed verified/not-verified claims
-> versioned Agent Console projection
-> reload and safe degraded-mode proof
```

Passing individual parser, database, or UI tests is necessary but not
sufficient. Done means the persisted, released user journey passes the complete
acceptance matrix.

## Implementation status (2026-08-03)

V1A, V1B, and V1C are implemented behind additive schema and API changes.
The canonical implementation is in `internal/knowledge`, the repository read
capability is in `internal/repository`, and validation/evidence authority stays
in `pkg/db/state`. The legacy `symbols` table and callers remain available
during migration.

Available operator flow:

```bash
openexec knowledge graph scan --directory /path/to/repository
openexec knowledge graph symbol SymbolName --directory /path/to/repository --read
openexec knowledge graph impact SYMBOL_ID --directory /path/to/repository

export AGENT_CONSOLE_TOKEN='the configured console token'
openexec knowledge graph publish \
  --directory /path/to/repository \
  --console-url http://127.0.0.1:8080 \
  --console-project AGENT_CONSOLE_CHECKOUT_ID \
  --symbol SymbolName
```

Without `--plan`, the published validation summary explicitly reports that
repository validation is unevaluated; an empty array must not imply success.
Pass `--plan VALIDATION_PLAN_REVISION_ID` only after its immutable completion
report exists. The publisher verifies that the plan belongs to this checkout
and to the freshly scanned graph/worktree state, then includes that frozen
report's verified, not-verified, and completion state. A stale plan is refused;
its green result is never paired with a newer repository snapshot.

Use `--console-token-file` instead of the environment variable when a managed
secret file is available. Publishing sends the bounded schema-v1 projection,
first observes Agent Console's ETag, and updates with `If-Match`. It never sends
source bytes, authoritative byte ranges, mutable validation plans, or copied
evidence.

Implemented proof includes additive and idempotent migration, dirty and
untracked manifest inputs, configuration invalidation, interrupted/mutating
scan refusal, atomic promotion, stable and ambiguous symbol resolution,
hash-validated bounded reads, traversal and symlink refusal, deterministic
incremental/full-scan parity, bounded transitive impact paths, test
recommendations, immutable accepted validation revisions, repository-state
evidence eligibility, completion gating and claim normalization, OpenExec API
restart persistence, Agent Console schema/scope/ETag enforcement, UI rendering,
reload persistence, and viewport usability ownership.

The controlled 20–30 task baseline/treatment evaluation remains a post-release
product experiment, not an implementation prerequisite. It decides whether to
invest in Version 2; it must not be reported as completed until its persisted
evaluation artifacts exist.

## Status correction and Version 2 outline (2026-08-03)

An independent audit found the earlier "V1A-V1C implemented" statement too
strong by this plan's own acceptance rule: symbol resolution and source
reads load the active generation without recomputing repository freshness
first, so the complete "edit -> detect stale -> re-resolve -> read current
source" journey does not exist yet. V1 is a verified structural graph
engine with a bounded, honest projection - not yet a trustworthy
interactive product.

Delivered with this correction: the projection now carries graph totals
(shown-versus-held honesty), dead-code candidates, hotspots, and a
top-level module sketch - conclusions instead of inventory, with heuristic
boundaries disclosed as limitations.

### Version 2 milestone (ordered)

1. Enforce freshness on every resolve/read; implement stale re-resolution.
2. Publish selection scope, base commit, worktree state, and extractor
   capabilities beside the existing totals and truncation disclosures.
3. Secure OpenExec query/source API for Agent Console (find-symbol,
   source retrieval, ambiguity selection, dependency navigation).
4. Clickable symbols in Agent Console with source, identity, resolution,
   and ambiguity views; symbols grouped by module.
5. Dead-code deletion pipeline: candidate -> attention item -> task ->
   read-only cross-examination -> supervised deletion branch.
6. Run the full dirty-worktree, line-shift, move, rename, ambiguity,
   restart, and degraded-mode journey end to end and persist the evidence.

### Version 3 candidates (unscoped)

Cross-repository graphs over the portfolio; per-question diagrams and
regenerated documentation; Jira-adapter tasks citing graph evidence; the
20-30 task baseline/treatment experiment gates all of it.
