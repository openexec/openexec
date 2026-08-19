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
provider and external-MCP credential. Broader V2.1 freshness enforcement is
still open and must not be inferred from this adapter.
