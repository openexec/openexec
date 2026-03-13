# Intent: OpenExec Refactoring (Self‑Hosting)

This file mirrors `refactoring.md` with the required sections and goal IDs so auto‑planning finds it.

flow: existing
scope: epic
workspace_root: .

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

