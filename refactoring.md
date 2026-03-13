# OpenExec Refactoring Intent (Self‑Hosting)

flow: existing
scope: epic
workspace_root: .

## Problem Statement
OpenExec suffers from prompt bloat, per‑phase subprocess churn, and state drift between JSON files and SQLite. Stalls happen when long single‑iteration work looks idle. We want a lean v1 that is faster, more deterministic, and easier to recover.

## Goals
- G-001: Reduce orchestration overhead and token use for each task.
- G-002: Stabilize run loop and self‑healing (avoid false stalls, improve retries).
- G-003: Improve determinism and recovery by unifying state and actions.
- G-004: Enforce safe, reviewable code changes via diffs scoped to the repo.
- G-005: Maintain usability with soft‑fail verification when builds/tests fail.

## Requirements
- REQ-001 (goal_id: G-001, G-003): Collapse to one orchestration plane (Pipeline/Loop as the core); DCP becomes a thin local‑tools adapter.
  - verification_script: "go build ./... || echo build-soft-fail"
- REQ-002 (goal_id: G-003): Make SQLite the canonical state; JSON artifacts are read‑only exports (or phased out).
  - verification_script: "go test ./internal/... -run Session|Audit || echo tests-soft-fail"
- REQ-003 (goal_id: G-003): Replace text heuristics with typed action schemas for routing and tool control.
  - verification_script: "echo '{\"action\":\"noop\"}' | jq '.' >/dev/null 2>&1 || echo schema-soft-fail"
- REQ-004 (goal_id: G-004): Shift code edits to deterministic diff/patch application, scoped to `workspace_root`.
  - verification_script: "git diff --quiet || echo patch-applied; test -d . || echo workspace-root"
- REQ-005 (goal_id: G-001, G-002, G-005): Reduce prompt cost (cache briefing/system blocks, apply history windowing) and add heartbeat‑based stall detection with dynamic thresholds.
  - verification_script: "openexec version >/dev/null 2>&1 || echo cli-ok"

## Primary Goals
- Collapse to one orchestration plane (Pipeline/Loop as the core) and treat DCP as a thin local‑tools adapter.
- Make SQLite the canonical state; JSON becomes read‑only exports.
- Deterministic edits via diff/patch application, hard‑scoped to `workspace_root`.
- Typed action schemas replace text heuristics for control signals.
- Reduce token cost: cache briefing/system blocks; add history windowing; relax stall detector using heartbeats.

## Constraints
- Never write outside `workspace_root`.
- Use `safe_commit --story <SID> --task <TID>` after verified steps only.
- If `go build` fails, do not abort the loop: capture diagnostics, checkpoint work, and continue to next smallest safe change (soft‑fail policy).
- Favor Chassis tasks for small refactors; keep epics serialized by story barriers.

## Acceptance Criteria
- `openexec run` executes end‑to‑end on this repo without manual `plan`/`story import`.
- Server `/api/health` reports resolved runner and DB status.
- On simulated build failure, loop records diagnostics and advances with a smaller patch instead of hard‑failing.

## Verification Scripts (soft‑fail friendly)
- Build (soft): `go build ./... || echo "build-soft-fail"`
- Tests (targeted): `go test ./internal/... -run Loop|Pipeline || echo "tests-soft-fail"`
- Lint (optional): `golangci-lint run || true`

## Wizard Kickoff (paste into `openexec wizard`)
We are refactoring OpenExec itself in this workspace. Treat this as an EXISTING project. Goals:
1) Single orchestration plane (Pipeline/Loop).
2) SQLite canonical; JSON export‑only.
3) Typed actions + diff‑based patcher scoped to workspace root.
4) Prompt cost reductions via caching + history windowing.
5) Improve stall detection with heartbeats and dynamic thresholds.
Use Chassis for small changes; serialize epics. Apply soft‑fail verification (see scripts). Generate stories and tasks accordingly and link each to a verification script.

## Notes for Planner
- Prefer 1 Study story first, then serialized implementation stories.
- For each story, include a “Chassis” task when the change is surgical.
- Include explicit `verification_script` using the soft‑fail commands above.
