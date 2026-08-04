# Impact manifest for Agent Console — engine-side work

Status: plan — not started
Date: 2026-08-04
Related: `KNOWLEDGE_V2_PLAN.md` (two release-acceptance checks still
outstanding), Agent Console `docs/OVERNIGHT_IMPACT_CONTRACT_PLAN.md`
(the consumer this exists for).

## Outcome

Agent Console's overnight lane needs to ask, per nightly run: *given these
changed files, what is the propagation set, and how trustworthy is the
answer?* Console v1 (their plan, phase D1) composes this from existing V2.3
endpoints (`symbols?file=`, `symbols/{id}/impact`) — nothing here blocks
them. This plan (a) closes the V2 release-acceptance debt first, because the
console is about to lean harder on graph conclusions than any consumer so
far, and (b) adds one batch endpoint so the nightly path does not need
N+1 HTTP round-trips per run.

Ordering is deliberate: **E0 before E1.** Do not ship a new consumer surface
on top of an engine whose acceptance evidence is still open.

## E0 — Close the V2 release-acceptance checks

The two checks KNOWLEDGE_V2_PLAN records as blocked-by-host, to be run on a
host that permits what the original build host forbade:

1. **Console Playwright journey (V2.4).** Run the Agent Console explorer
   journey on a host permitting localhost listeners. Record the trace under
   the benchmark's evidence location. If any step fails, the failure is the
   deliverable — file it, do not massage it.
2. **Siivous benchmark (V2.6).** Run the Siivous backend/integration tests in
   the provisioned environment (pytest + Docker available). The persisted
   tender/bid verification report must be reproduced from a named graph
   generation, per the plan's own definition of "benchmark".

verify: KNOWLEDGE_V2_PLAN's implementation-record table updated with the two
statuses and links to the recorded evidence; the standing rule holds — an
unavailable execution check is never recorded as passed.

## E1 — Batch impact endpoint

Where: `internal/knowledge` (query lives beside `graph_impact.go`), HTTP
surface beside the V2.3 handlers, CLI beside the existing
`knowledge graph` verbs.

### Contract

`POST /api/v1/repository-graph/impact/changed` — checkout-authorized exactly
like the existing V2.3 endpoints (same identity resolution, same freshness
enforcement: recompute-or-refuse, never silently stale).

Request:

```json
{
  "files": ["internal/auth/login.go", "ui/src/features/login/Form.tsx"],
  "max_depth": 2
}
```

Bounds (server-enforced, refusal not truncation-silence): ≤ 50 files,
`max_depth` ≤ 3. Over-bounds → 422 with a reason string the console can
surface verbatim.

Response — one envelope, not per-symbol fragments:

```json
{
  "graph_version": "…",
  "freshness": "fresh",
  "provenance": { …same shape as projection Provenance… },
  "changed_symbols": [ …GraphSymbol, grouped resolution statuses intact… ],
  "propagation": {
    "direct_callers":   [ …ImpactNode… ],
    "affected_callers": [ …ImpactNode… ],
    "related_tests":    [ …RelatedTest… ]
  },
  "unresolved_files": ["path → reason"],
  "limitations": ["…"]
}
```

Rules:

- Reuse `Store.ImpactAnalysis` per resolved symbol; de-duplicate the union
  by node ID; keep the shortest path per node. No new traversal logic.
- A file whose symbols cannot be resolved goes to `unresolved_files` with a
  reason — it must not vanish. The console's `incomplete` verdict depends on
  knowing what the graph could NOT see.
- `provenance.extractors` rides in the response so the consumer can grade
  trust per language without a second projection fetch.
- Freshness semantics identical to V2.1: drifted worktree → refresh then
  answer, or typed `stale` refusal. Never a silently outdated propagation.

### CLI twin

`openexec knowledge graph impact --directory <repo> --files a.go,b.tsx
[--max-depth 2] [--json]` — same envelope on stdout. This is the offline
path for a console configured with `AGENT_CONSOLE_OPENEXEC_BIN` but no URL.

verify:
- Endpoint contract tests mirroring the V2.3 suite: authorization, bounds
  refusal (422 + reason), unresolved-file accounting, de-duplication
  (fixture where two changed symbols share a caller — caller appears once),
  stale-refusal path.
- CLI test: `--json` output byte-equivalent in structure to the HTTP
  envelope for the same fixture repo.
- A conservative-liveness regression guard: fixture in a language with
  heuristic call extraction (Python) asserting the response's
  `provenance.extractors` says so and `limitations` names it — the recent
  fix series (`0d9c225`, `e9f5111`) must remain visible truth in this
  surface, not get averaged away.

## E2 — (optional, after E1 ships) console switchover

Agent Console replaces its D1 N+1 composition with one `impact/changed`
call. Their plan already isolates the openexec client behind a small
internal client — the switch is contained there. Not part of this repo's
acceptance; listed so nobody builds E1 without a consumer commitment.

## Non-goals

- No write access for the console; it stays a presentation gateway.
- No auto-derived "safe to merge" boolean in the response. The engine
  reports facts and their limits; verdicts are the console's policy layer.
- No LLM-inferred edges in this path. Propagation here must remain
  structural truth only ("inferred is never presented as structural
  repository truth").
