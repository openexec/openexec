# Knowledge Graph V2 — implementation plan

Status: implementation complete; release acceptance has two host-constrained checks outstanding
Date: 2026-08-03
Related: `REPOSITORY_POINTER_GRAPH_PLAN.md` (V1 + status correction),
Agent Console integration contract, 2026-08-03 freshness audit.

## Implementation record

| Phase | Delivered evidence | Status |
| --- | --- | --- |
| V2.1 | Read-time manifest comparison, serialized refresh/re-resolution, typed stale refusal, edit/move/rename/line-shift/current-source tests | Implemented; focused Go tests pass |
| V2.2 | Projection provenance, worktree state, extractor capabilities, exact totals and per-list selection/truncation scopes; console rendering | Implemented; Go/React tests pass |
| V2.3 | Checkout-authorized paginated symbol/detail/relationship/impact/source API and incoming/outgoing CLI calls | Implemented; endpoint contract tests pass |
| V2.4 | Console Explore Overview, Dependencies, Symbols, Call flow, Impact and Source views plus Mermaid export | Implemented; type, lint, unit and production build pass; Playwright could not start because this host forbids listener sockets |
| V2.5 | Fact-derived owner summary and dead-candidate projection into existing Attention -> triage -> supervised execution lifecycle | Implemented; server and owner tests pass |
| V2.6 | Python/Svelte extraction, `_archive` exclusion and persisted named-generation tender/bid audit in Siivous | Implemented and manually spot-checked; Python behavior tests were not run because `pytest` is absent and Docker access is denied |

The implementation is not allowed to turn those two unavailable execution
checks into passed evidence. Release acceptance requires rerunning the console
Playwright journey on a host that permits localhost listeners and the Siivous
backend/integration tests in its provisioned environment. The persisted
benchmark records exactly what was and was not verified.

## Outcome

Turn the verified V1 graph engine into a trustworthy, interactive product
with two audiences: agents and engineers (queries, impact, source), and
non-technical owners (direction and quality in sentences). V2 is accepted
when the Siivous business-flow benchmark passes — not when its features
individually exist.

## Product language

- **Fresh** means the answer was recomputed against the current worktree at
  read time, not that a current-labelled generation exists.
- **Conclusion** means a bounded derived answer (dead code, hotspot, flow)
  with its heuristic limits disclosed alongside it.
- **Inferred** labels anything an LLM added; it is never presented as
  structural repository truth.
- **Benchmark** means the persisted Siivous tender/bid verification report,
  reproducible from a named graph version.

## Phases (strictly ordered — each gates the next)

### V2.1 Freshness enforcement and stale re-resolution  [filed: openexec task]

The trust gap: `graph_store.go` resolve and `source.go` reads load the
active generation without recomputing repository freshness, so the
"edit -> detect stale -> re-resolve -> read current source" journey does
not exist. Contract:

- Every resolve/read first compares the current worktree state hash; a
  drifted pointer triggers incremental refresh and re-resolution before
  answering, or returns an explicit `stale` refusal when refresh is
  impossible (never a silently outdated answer).
- Cancellation and concurrent-scan behavior inherit the existing
  single-writer generation rules.
- Acceptance evidence: an automated journey — edit a file, resolve the
  moved symbol, read its source — asserting the answer reflects the edit,
  plus the dirty-worktree, line-shift, and rename variants from the audit.

### V2.2 Provenance and scope metadata in the projection

Additive fields beside `totals`: base commit, worktree state (clean or
dirty count), extractor capabilities actually used (compiler vs lexical
per language), selection scope for every bounded list, and truncation
counts. The console renders all of it; nothing bounded may present itself
as complete. Acceptance: panel shows provenance for a real repository and
the e2e fixture asserts the truncation phrasing.

### V2.3 Secure graph query API  [filed: openexec task]

OpenExec serves bounded queries for the console: find-symbol (paginated),
symbol detail (identity, resolution, occurrences), dependencies and
reverse dependencies, `calls --direction outgoing|incoming --depth N`
(new), impact, and hash-validated source ranges. Rules:

- Same authority boundary as publish: the console never touches the
  SQLite store; per-checkout authorization mirrors the publish scope; the
  existing repository source authority validates every read.
- Every response carries graph version and freshness state (per V2.1).
- Acceptance: contract tests per endpoint including ambiguity (multiple
  candidates returned, none auto-selected) and stale refusal.

### V2.4 Console Explore experience

An "Explore graph" action per repository: Overview (package groups,
expand on click — never all 786 modules at once), Dependencies,
Symbols (search over the paginated API, not the cached projection),
Call flow (bounded, labelled "call flow", not "flowchart"), Impact,
Source. Clickable nodes resolve identity, resolution method, location,
and limitations. Renderer: interactive graph component; Mermaid export
for documentation. Acceptance: the audit's click-through journey on a
real repository, plus viewport usability on the phone profiles.

### V2.5 Conclusions put to work

- Dead-code pipeline: candidate -> attention item -> prefilled task ->
  read-only cross-examination -> supervised deletion branch via the
  execution lane. First target: the Siivous legacy code already found.
- Owner-language summary card on the Project overview  [file under the
  console's owner project]: direction and quality in sentences derived
  only from projection facts (growth, unused count, riskiest area by
  inbound-vs-tests, freshness date). No invented scores; every sentence
  traces to a graph field.

### V2.6 Siivous benchmark — the V2 acceptance gate  [filed: siivous task]

Prerequisites: exclude `_archive` from the active graph, load the real
SvelteKit TypeScript configuration and aliases, resolve installed
frontend dependencies, report route-extraction coverage, reach a current
non-partial graph. Then produce the tender/bid verification report:
state machines, guards, endpoint-to-handler-to-persistence traces, test
coverage per transition, and findings (e.g. "late bids may not be
rejected") — manually verified the first time, persisted with its graph
version. V2 is complete when this report exists and its spot-checked
claims hold; the generic 20-30-task experiment is superseded by it.

## Non-goals (V3 candidates)

Control-flow-graph extraction (true flowcharts), HTTP/service API
reference (route/schema/middleware extraction with authority order:
OpenAPI > route registration > handler signatures > labelled inference),
cross-repository portfolio graphs, generated architecture documentation
keyed to graph versions, Jira adapter citing graph evidence. LLM-inferred
diagrams may be offered earlier only with the mandatory "inferred" label.

## Risks

- V2.1 touches the hottest read paths; regressions look like slowness or
  spurious refusals — keep the 6.6s scan benchmark and add a resolve
  latency budget to the suite.
- The query API doubles the attack surface of the graph; reuse the
  publish authorization model wholesale rather than inventing a second.
- Explore UX can absorb unlimited effort; V2.4 ships with the six views
  bounded and nothing else.
- Siivous graph hygiene may reveal extraction gaps (SvelteKit routing);
  treat those as V2.6 scope, not blockers hidden in V2.3.
