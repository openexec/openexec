# OpenExec defect inventory (FIX1)

FIX1 of Goal 14a5a33b "Verified OpenExec defect closure". This file is the
original FIX1 inventory, with FIX2 dispositions recorded below. Each **confirmed**
row is meant to become one FIX2 task.

## Pinned revisions (2026-10-17)

| What | Revision |
|---|---|
| OpenExec `origin/main` (inventoried) | `ca1e34c7bb79c7f9006254cad94bf6fd12ddf612` |
| Running build: `openexec-graph` `/api/health` on :8765 | `ec3cefba` (`0.13.1-154-gec3cefba`). The only difference from `ca1e34c7` is `pkg/manager/manager_test.go`. |
| Agent Console `origin/main` (`79b0ecff`), `go.mod` | `github.com/openexec/openexec v0.13.2-0.20261008125114-cb20c1fe6ae8`. Its tracked `go.work` is `use .`. |
| Integration lag | `git diff --stat cb20c1fe ca1e34c7 -- pkg/ internal/release internal/pipeline internal/blueprint internal/execution` lists only `pkg/manager/manager_test.go`. The packages Agent Console embeds (`pkg/manager`, `pkg/runtime`, `pkg/db/state`, `pkg/outcome`) behave the same as main. |

## Sources checked

- **GitHub:** open issues #17, #18 and #19, and the only open PR, #67.
- **Declared checks on `ca1e34c7`:** `lint` exited 0 (go vet plus UI ESLint). `test` exited 0: every Go package passed, including `internal/validation` (compat), and 635 UI tests in 40 files passed.
- **Skipped tests:** every `t.Skip` depends on the environment or on `-short` mode, or marks a deliberately retired legacy path. The unconditional legacy skips are in `pkg/manager/manager_test.go`, `internal/cli/plan_test.go` and `internal/server/gemini_integration_test.go`. These are test debt, not defects.
- **TODO/FIXME:** no comment names a bug. The only hit is a test fixture in `internal/quality/nostubs`.
- **Historical findings:**
  - `docs/PROJECT_AND_PRO_REVIEW_2026-07-10.md`
  - `docs/SYMBOL_TOOLS_REVIEW.md`
  - `docs/VERIFICATION_FAILURE_INSPECTION.md`
  - `docs/NATIVE_*.md`
  - `docs/AUTONOMOUS_HUMAN_BOUNDARIES_IMPLEMENTATION_STATUS.md`
  - `docs/verification*`
  - `docs/deploy-evidence/`
- **Exercised journeys:**
  - `go run ./cmd/openexec version`.
  - The running graph build's `/api/health`.
  - The recorded graph journey `docs/deploy-evidence/openexec-graph-ef828b15-*`.

  The owner journey through Agent Console (request work, accept Goal/Ready,
  leave and return) runs in the Agent Console repository. It was not
  exercised from this lane.

**Re-run every reproducer** with `scripts/defect-inventory/reproduce.sh`. It
prints `REPRODUCED` or `FIXED?` for each confirmed row and exits 1 when a row
has gone stale.

## Confirmed defects, ordered by owner impact

| ID | Defect | Reproducer / evidence | Proposed fix (one line) |
|---|---|---|---|
| D1 | **FIX2 repair implemented; pending independent verification.** API runner file tools previously accepted workspace escapes; the security claim was overbroad. | Original tagged reproducer now refuses the first escape; `scripts/defect-inventory/reproduce.sh` prints `FIXED? D1`. `python3 scripts/defect-inventory/verify-d1.py` runs the regular `TestAPIFileToolsWorkspaceBoundary`, which fails on the original implementation and passes with this repair, exercising both tools, refusal before directory creation, and persisted in-workspace round trips. | Fixing PR: openexec/openexec#112. `resolveFilePath` rejects `filepath.Rel` equal to `..` or beginning with a parent-directory component before file effects. SECURITY_MODEL limits its claim to infrastructure/MCP and explicitly describes `sh -c`. Existing code does not resolve symlinks; OS sandboxing remains roadmap. |
| D2 | **Fixed (FIX2):** Cobra command output previously went to stderr because `Execute` did not configure stdout; version pipes were empty (graph deploy guard, #108). | `TestExecuteOutput` calls production `Execute` in fresh subprocesses without `SetOut`: exact version text on stdout, empty stderr; unknown command exits 1 with stderr only. It fails without the fix. `scripts/defect-inventory/reproduce.sh` now prints `FIXED? D2` (62 stdout bytes). | `rootCmd.SetOut(os.Stdout)` in `internal/cli/root.go`. Fix PR: [#114](https://github.com/openexec/openexec/pull/114). Re-run positive, negative-control and CLI reproducer verification with `make test`, which includes `scripts/defect-inventory/verify-d2.py` (use console `run_declared_check("test")` if Go builds are sandbox-blocked). |
| D3 | Fixed in [PR #113](https://github.com/openexec/openexec/pull/113). CI previously built the UI without running its unit tests. | `scripts/defect-inventory/reproduce.sh` now prints `FIXED? D3`. Ordinary regression `TestCIRequiresUIUnitTests` fails without the step and passes with it; it also requires dependency installation, `ui/` as the working directory and unconditional failure propagation. | Added `npx vitest run --fileParallelism=false` in `ui/` after the UI build, for both CI matrix platforms. |
| D4 | **Fixed in [PR #115](https://github.com/openexec/openexec/pull/115).** The hook and standalone panel now append `/v1/approvals` to the API base for list, approve and reject. An exact HTTP 503 `approval gate not configured` response clears stale approvals (hiding the ChatPage panel) and hides the standalone panel; other errors remain visible. | `scripts/defect-inventory/reproduce.sh` prints `FIXED? D4`. Regular regression: `ui/src/components/chat/blueprint/ApprovalsPanel.test.tsx`; browser/real API regression: `ui/e2e/approvals-d4.spec.ts` via `node ui/node_modules/@playwright/test/cli.js test --config ui/playwright.d4.config.ts`. | No production gate was wired. The API still refuses list/approve/reject without a configured gate. Repair: [PR #115](https://github.com/openexec/openexec/pull/115); FIX1 inventory history remains in #111. |
| D5 | **FIX2 repair implemented; pending independent verification.** Knowledge Hub no longer requests the unregistered policies endpoint or displays unsupported policy/enforcement claims. | `KnowledgeHub.test.tsx` covers populated/empty success, either supported endpoint failing, Retry and reopening. `verify-d5.py` (in `make test`) requires four baseline failures, candidate success, UI types and controlled Chromium failure/Retry journeys. It also builds the UI and runs `TestKnowledgeHubBrowser` with real coordinator-backed routes, scoped bearer/checkout authentication and persisted SQLite data; `knowledge-hub-live.journey.ts` checks actual 200 payloads, unauthenticated 401 refusal, navigation away/back, reload and absence of policies requests. Inventory reproducer: `FIXED? D5` (exit 1). Declared checks: `test exited 0` (639 UI tests and three browser journeys), `lint exited 0`; focused live-harness rerun: `PASS: TestKnowledgeHubBrowser`. | Fixing PR: [openexec/openexec#119](https://github.com/openexec/openexec/pull/119). Removes only unsupported policy UI; symbols, environments, loading and genuine errors remain. Live journey configuration: isolated temporary databases, loopback HTTP listener, compiled UI, DCP enabled, preflight skipped, no provider queries, browser-context test bearer and checkout ID. This is local integration evidence, not production deployment. |
| D6 | Fixed in this D6 timeline repair candidate: the UI list omitted default blueprint stages, causing their events to be dropped. | `scripts/defect-inventory/reproduce.sh` now prints `FIXED? D6`. `TestDefaultBlueprintStagesAppearInUI` compares the real default blueprint with the UI list; it fails on the original list. `StageTimeline.test.tsx` exercises all nine stages through the hook and rendered timeline, including failure, retry, reset and unknown-stage rejection. | Added `fix_lint`, `production_readiness_check`, `generate_contract_tests` and `fix_tests` with display metadata. Fix PR: [#116](https://github.com/openexec/openexec/pull/116). |
| D7 | The deterministic router sends the question "what is the difference between a list and a tuple?" to `list_directory`, because of the bare `list` keyword. This affects only the toolset hint in runs without an injected StageExecutor, so it does not affect the Agent Console path. | `internal/router/deterministic_test.go:104` pins this as the actual result, and the test passes. | Match `list` only when a file or directory object follows it, as #20 did for `diff`. |
| D8 | **Repair submitted for verification:** untrack 469 `.gocache` files, `bin/openexec`, root `openexec` and `data/fwus.jso` (472 artifacts). | `git ls-files .gocache bin/openexec openexec data/fwus.jso \| wc -l` now prints `0`; reproducer reports `FIXED? D8`. Ordinary regression: `go test -count=1 ./internal/repositoryhygiene`. | Root-anchored `.gitignore` rules prevent recurrence; no history rewrite. Fixing PR: openexec/openexec#118. |

## Revalidated: fixed, not reproducible, or out of scope

| Finding (source) | Status | Evidence |
|---|---|---|
| Issue #17: bare `diff` misroutes "difference" | fixed in `25e835b0` (#20) | `TestDeterministicRouter_ParseIntent_DiffWordBoundary` passes. The issue is still open and can be closed. |
| Review §1: manager/pipeline races on a shared blueprint | fixed in `48b66b2d` | `pipeline.go:474` calls `bp.Clone()`. `go test -race -run TestConcurrentReadersDoNotRaceRunningPipeline ./pkg/manager/` and `-run TestCloneIsolatesStagesFromTheShared ./internal/blueprint/` both print `ok`. |
| Review §2: empty lint/test commands auto-pass | fixed in `48b66b2d` | `internal/blueprint/executor.go:186-188` fails closed. The comment at `engine.go:159` ("empty = auto-pass") is stale. |
| Review §4: daemon listens on all interfaces | fixed in `12cbd35f` | `serverListenAddress` defaults to `127.0.0.1`, and `start.go:852` defaults `--listen` to `127.0.0.1`. |
| Review §4: unauthenticated HTTP approval handlers (also AHB status) | not reproducible | The handlers have no authentication, but no gate is wired, so every call returns 503 (see D4). The authenticated answer path is roadmap (AHB step 2). |
| Review §4: `--dangerously-skip-permissions` in CLI runners | out of scope: roadmap (OS sandbox) | The flag is a deliberate runner design (`internal/loop/process.go:263`). The overstated documentation claim is covered by D1. |
| Review §3 (unwired capabilities), §5 (Replay placeholder, DCP-only views) | out of scope: roadmap | These are unfinished features, not regressions. |
| Review §5: `golangci-lint` is advisory | out of scope: documented debt | `ci.yml:57-64` records 142 historical issues. |
| Review §6: broken local links in CONFIGURATION.md | not reproducible | All 4 local `.md` links resolve. |
| Review §8: installer downloads without a checksum | not reproducible | `scripts/install.sh:29` builds from source. |
| Review, Pro findings (`executeContractDirect` runs `git add -A`, 80 KB diff cut, and others) | out of scope: separate `openexec-pro` repository, not revalidated | Recorded so they are not lost. They need their own inventory. |
| SYMBOL_TOOLS: `mcp-serve` resolves the allowlist, config and approvals from `os.Getwd()` | fixed in `674b50f0` | `resolveServeRoot` (`internal/cli/mcp_serve.go:31`) gives one root. The working directory is only the last fallback. |
| SYMBOL_TOOLS: `TestPythonIncrementalRefreshPreservesTypeScriptCapability` fails when `tsc` is installed | not reproducible | The test passes with `tsc` on PATH (`/home/perttu/.npm-global/bin/tsc`). |
| SYMBOL_TOOLS: `RelatedTest` is not wired to the test stage; symbol tools are allowed in every mode | out of scope: roadmap / open design question | |
| VERIFICATION_FAILURE_INSPECTION: a populated result is lost when an error is also returned | fixed in `2fc03214` (#66) | `engine.go` calls `failedStageResult`. |
| VERIFICATION_FAILURE_INSPECTION: durable diagnostics handoff | fixed in `4290c0d2` (#69) | `CheckFailure.Command/Output`. |
| VERIFICATION_FAILURE_INSPECTION: in-queue recovery of legacy failures | fixed in `e5d026ab` (#65) and `a7327a56` (#70) | |
| VERIFICATION_FAILURE_INSPECTION: `make check` / `make pr-gate` do not exist | not a defect | The canonical gate is the declared `lint`/`test` checks. |
| NATIVE_EMPTY_RESPONSE: `reasoning_content` vs `thinking` key mismatch | fixed in `63628e85` (#50) | `pkg/execution/api_provider.go:551-552` reads both keys. |
| NATIVE_EMPTY_RESPONSE / NATIVE_CONTEXT: unknown generation cause, second inference refused | not reproducible here | Needs live local-model inference under a separate resource grant. The controller-continuation work is roadmap. |
| AHB status / PR #67: binding terminal completions to task/stage/attempt | out of scope: roadmap (parked Goal 21d181ed) | PR #67 (head `1bee658d`) is 29 commits behind main (merge base `3463de8e`), and its own NOTES report `TestPersistedExitOneRecovery` failing after sync. Its behaviour stays with that goal. |
| verification F2 / D2 (Console private capture, receipts) | out of scope: Agent Console roadmap (US-012) | |
| plan-identity: Agent Console goal-mode verifier mismatch | not reproducible | The source docs also say it was not reproduced. It is on the Agent Console side. |
| Issues #18, #19: missing tests for `pkg/telemetry/redact.go` and `internal/schema` | not a defect: coverage request | Both packages have no test files. |
| Local Agent Console checkout `/mnt/data1/projects/agent-console` | out of scope: owner's uncommitted local state (not touched) | `HEAD` is `1d8aa1d8`, 872 commits behind origin and dirty. Its untracked `go.work` uses `../openexec/.worktrees/autonomous-human-boundaries` (`b1d738c4`), so a build from that checkout would ignore the go.mod pin. Agent Console `origin/main` tracks `go.work` as `use .`. |

### D8 build dependency evaluation

`make build` generates UI assets and then `bin/openexec` from source. CI
(`.github/workflows/ci.yml`), `scripts/install.sh`, `scripts/release.sh`, and
`Dockerfile.build` also compile from source. `scripts/reviewed-deploy-graph.sh`
runs `make build` before executing or copying `bin/openexec`. No build, CI or
deploy script reads the removed root binary, cache entries or `data/fwus.jso`.
The similarly named `data/fwus.json` is not removed or ignored.

Reproduce through `python3 scripts/defect-inventory/verify-d8.py`, also run by
`make test`: a temporary index loaded from inventory baseline `59f009c6`
makes the ordinary regression fail specifically on tracked artifacts; the
current index passes. A temporary source copy excludes all four artifact
locations, links the installed UI dependencies, runs `make build`, and executes
the rebuilt CLI. It never changes the real index or removes local artifacts.

Prior verification removed all four artifact locations from disk, ran `make build`
(exit 0), and executed the newly built `bin/openexec version` (exit 0).
`TestBuildArtifactsUntrackedAndIgnored` failed against the original index and
ignore rules, then passed with the repair. It checks the real Git index, ignore
rules for regenerated files, and that source/data/helper paths remain visible.
No project loading, migration, configuration or runtime behavior changed.

Rework declared verification: `test exited 0`, including the D8 baseline/current
regression and artifact-free build/CLI verifier, Go suite and 635 UI tests;
`lint exited 0`. Independent verification remains pending.

Prior-pass declared verification: `lint exited 0`; final `test exited 0` (`make test`, Go
suite plus 40 UI files / 635 tests). The first test attempt exited 2 at
`TestRetentionUnitNativeCallbacksAndStorageRefusal` with `receipt missing`.
An isolated rerun and then the unchanged full declared check passed; the cause
of the initial failure is not established. `git diff --check` also passed.
