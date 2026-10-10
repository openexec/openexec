# External advisory MCP — OpenExec work

The authoritative cross-product implementation plan is owned by Agent Console:

[Agent Console external advisory MCP plan](../../agent-console/docs/EXTERNAL_ADVISORY_MCP_IMPLEMENTATION_PLAN.md)

OpenExec's scope is limited to the repository-evidence dependency in that
plan: complete V2.1 freshness enforcement and V2.3 secured graph query access,
then expose a typed, authenticated, checkout-bound **read-only** adapter for
current symbols, source pointers, relations, and selected validation reads.
It must expose no validation mutation and must return authoritative freshness
and `provenance.graph_version` in the response body.

The first read adapter is implemented behind
`OPENEXEC_REPOSITORY_EVIDENCE_TOKEN`. It registers authenticated GET routes
under `/api/v1/external-evidence/` for symbols, source, dependencies, calls and
impact. The token must match Agent Console's
`AGENT_CONSOLE_OPENEXEC_EVIDENCE_TOKEN`; use a different secret from every web,
provider and external-MCP credential. A separate
`OPENEXEC_REPOSITORY_GRAPH_TOKEN`, matched by Agent Console's
`AGENT_CONSOLE_OPENEXEC_GRAPH_TOKEN`, protects repository-context and every
legacy repository-graph route, including scan, changed-impact, and validation
writes. Agent Console retains both server credentials and never exposes either
through the external advisory profile. OpenExec binds to loopback by default;
each route family fails closed when its own credential is absent.

## Freshness contract (Phase 1B exit, OpenExec side)

Every evidence read passes the V2.1 read gate (`freshGeneration`): it
recomputes the scan manifest, refreshes a drifted worktree before answering,
or refuses. Every success and refusal body carries `provenance` — the
generation that answered (`graph_version`, `freshness`, `checkout_id`,
`worktree_state_hash`, …). `generation` still carries the same values for
existing internal consumers. Refusals carry a machine-readable `reason`:

| Status | `reason` | `provenance.freshness` | Meaning |
| --- | --- | --- | --- |
| 409 | `graph_stale` | `stale` | Drift detected and refresh impossible or disabled; nothing is answered from the old pointers |
| 404 | `graph_missing` | `missing` | No generation exists for this checkout (never scanned) |
| 404 | `not_found` | — | The graph exists but the symbol does not |

`internal/server/repository_evidence_test.go` proves the exit criterion through
the external routes: an edit after publication yields a refreshed answer citing
a new `graph_version`; with refresh disabled the same reads refuse as stale;
missing graph and missing symbol stay distinguishable. Ambiguous names return
every candidate, and none is auto-selected (V2.3 contract tests).

Agent Console currently turns every non-2xx evidence response into a generic
"refused with status N" tool error. To keep stale, missing and not-found
distinguishable end to end, it has to pass through `reason` and `provenance`
(Agent Console work, outside this repository).
