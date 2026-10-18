# NOTES

Raw capture. One line per thought, any grammar.

## Now

- [D4 integration, 2026-10-18, PR #115] Resolve the notes-only conflict
  with supplied main 25906e2d, preserving both sides and incoming D2/D6
  implementation, inventory dispositions and verification targets. D4 approval
  behavior is unchanged. Fresh declared checks: "test exited 0" (661 UI
  tests / 44 files, FIXED? D4, seven expected original-source failures and
  passing real-503/reload/refusal browser journey); "lint exited 0".
  TypeScript and whitespace checks passed. Main notes/inventory/Makefile
  preservation assertions passed; MERGE_HEAD equals supplied main.
  Independent published-head verification remains pending.

- [D4 current-main integration, 2026-10-18, PR #115] Resolved the pending
  merge of 40e5d930 in NOTES and inventory by retaining both D4 and D5
  dispositions/evidence. Main's D1/D3/D5/D8 code and checks are preserved;
  only D4 paths differ from supplied main. Approval behavior is unchanged.
  Fresh declared checks returned "test exited 0" (651 UI tests, FIXED? D4,
  seven expected original-source failures, passing real-503/reload/refusal
  browser journey) and "lint exited 0". Preservation assertions and
  git diff --check passed. Current contract identifies this task's PR as #115.
  Typed continue_sync refused the nightly task ID: "feature_key must name an
  accepted Goal in this repository: not found". The existing MERGE_HEAD is
  supplied main; preserve it for the nightly publication workflow. Independent
  review of the resulting published head remains pending; no completion claim.

- [D4 integration repair, 2026-10-18] Preserve D1/D3/D8 main changes and D4
  evidence; remove leftover merge markers without changing approval behavior.
  Fresh declared test and lint both exited 0: 647 UI tests, FIXED? D4,
  seven expected baseline failures and the browser reload/refusal journey pass.
  Main sibling files and notes preservation checked. Exact published-head
  review remains pending; typed preflight refused the task ID as an accepted Goal.

- [D4 PR report rework, 2026-10-18] Replace PR #115 description with the
  final scope and reproducible evidence using describe_pull_request. Preserve
  both D4 and incoming D1 notes during the main sync. Fresh declared checks:
  "test exited 0" (647 UI tests, FIXED? D4, seven expected negative-control
  failures, browser journey passed); "lint exited 0". TypeScript and diff
  checks passed. describe_pull_request confirmed GitHub reports the new body.
  Independent verification remains pending.

- [D4, 2026-10-18, PR #115] Approval URLs use the API base plus
  `/v1/approvals`. Exact not-configured 503 clears stale hook approvals and
  hides the standalone panel; ordinary failures stay visible. No production
  gate wiring, schema, migration, project loading or compatibility behavior changed.
  Browser rework opens the right panel, shows a seeded pending approval, starts
  real polling via a stage event, observes the real Go API's unconfigured 503
  remove the approval while the panel remains open, then reloads and reopens.
  Both action routes still refuse with 503. Initial host `make d4-test` passed.
  `scripts/defect-inventory/verify-d4.py` requires reproducer exit 1 plus
  `FIXED? D4`, and runs the regular regression against original production
  sources in an isolated temporary copy: seven of twelve tests fail as expected.
  `make test` now includes this proof and browser check through `d4-test`, making
  them reproducible with the declared test runner. Inventory links PR #115.
  UI TypeScript check passed. Declared test returned `test exited 0`: Go tests,
  647 UI tests, the reproducer assertion, seven expected original-source
  failures and the strengthened browser test passed. Declared lint returned
  `lint exited 0` (Go vet and UI ESLint).
  Independent verification remains pending; no acceptance claimed.

- [D6 integration, 2026-10-18] Resolve the notes-only conflict with main
  29e88a7c, retaining both D6 and incoming D2 records. Preserve all incoming
  implementation, inventory dispositions and Makefile verification targets.
  Fresh declared checks returned "test exited 0" (649 UI tests / 43 files)
  and "lint exited 0". TypeScript and diff checks exited 0; inventory
  reproducer printed FIXED? D6 (documented exit 1). The host test log was
  truncated. Independent verification remains pending.

- [D6 integration, 2026-10-18] Resolve conflicts with main 40e5d930
  by preserving both sides of NOTES.md and the repaired D5 and D6 inventory
  rows. Retain incoming D5 implementation and verification alongside D6.
  Fresh declared checks returned "test exited 0" (649 UI tests / 43 files)
  and "lint exited 0". TypeScript and diff checks exited 0; inventory
  reproducer printed FIXED? D6 with documented exit 1. Independent acceptance
  remains pending.

- [D6 integration, 2026-10-18] Resolve the notes-only conflict with main
  230b1d45 by retaining D6 and incoming D3 entries. Preserve D1/D3/D8
  implementation and inventory changes. Fresh declared checks returned
  "test exited 0" (645 UI tests / 42 files) and "lint exited 0".
  TypeScript and diff checks passed; reproducer printed FIXED? D6 (exit 1
  by contract). Independent verification remains pending.

- [D6 PR report, 2026-10-18] Replace PR #116 narration with the current
  change, reproducible acceptance evidence and fixture limitation. Resolved
  current-main notes conflict by retaining both D6 and D8 entries. Fresh
  declared checks returned "test exited 0" (645 UI tests / 42 files) and
  "lint exited 0"; TypeScript and diff checks passed. Reproducer returned
  FIXED? D6 (exit 1 by contract). Independent verification remains pending.

- [D6 verification access, PR #116, 2026-10-18] make test now invokes
  scripts/defect-inventory/verify-d6.py and ui/scripts/verify-d6.mjs so the
  independent reviewer can reproduce both through run_declared_check(test).
  The negative control uses the inventory baseline in a temporary Go test
  overlay, changes only the input file path, requires all four missing-stage
  diagnostics, and then runs the unchanged regression against the current list.
  Local verifier exited 0; browser sandbox attempt returned listen EPERM.
  Declared host test returned "test exited 0": original-list failure, repaired
  list PASS, "D6 browser PASS" (real page/WebSocket fixtures, failure, retry,
  completion, unknown stage, panel reopen and reload reset), Go suite and
  645 UI tests / 42 files. Declared lint returned "lint exited 0"; TypeScript
  and diff checks exited 0. Reproducer printed FIXED? D6 with exit 1.
  Preserved both D6 and incoming D1 notes when resolving their text conflict.

- [D6 timeline repair, PR #116, 2026-10-18] Rework adds a ChatPage
  guard against reprocessing the latest event on blueprint state updates.
  ChatPage.test.tsx exercises real WebSocket decoding, hooks and page rendering
  with only network fixtures; includes failure/retry/completion, unknown-stage
  rejection, panel reopening and remount reset. ui/scripts/verify-d6.mjs passed
  in Chromium via run_command after sandbox listen EPERM; real application,
  deterministic network fixtures, reload reset, no page errors. This is not a
  provider-backed blueprint execution claim. Timeline state is transient.
  TestDefaultBlueprintStagesAppearInUI executed with origin/main 59f009c6's
  original list: exit 1, all four omissions named; restored list: exit 0.
  The page regression also fails with the event guard removed (effect-loop
  diagnostic), then passes restored. Reproducer prints FIXED? D6 (exit 1 by
  its documented contract); TypeScript and diff checks pass. Inventory links
  #116. Declared test passed (645 tests / 42 UI files plus Go); lint passed.
  Final declared rerun after the bounded effect-loop assertion: test exited 0
  (645 UI tests / 42 files), lint exited 0. Original-list reproducer also
  executed and printed REPRODUCED D6; restored file re-read and diff checked.

- [D2 current-main integration, 2026-10-18] Resolved NOTES.md against
  supplied main 40e5d930, retaining both sides and D1/D2/D3/D5/D8 records.
  All four Makefile defect verifiers remain. Fresh declared checks returned
  "test exited 0" and "lint exited 0"; UI: 41 files / 639 tests passed
  with React warnings. D2 uncached Execute cases passed; missing-SetOut
  overlay failed with empty stdout and left source unchanged; real CLI
  emitted 62 stdout bytes. Inventory reports FIXED? D1/D2/D3/D5/D8 and
  REPRODUCED D4/D6/D7. Main's D3/D5 implementation and checks are preserved;
  D5 controlled and coordinator-backed browser journeys passed. Whitespace
  check passed. Independent verification and acceptance remain pending.

- [D2 integration rework, 2026-10-18] Resolve supplied main 230b1d45
  conflicts retaining D1/D2/D3/D8 records, D3 CI step and regression,
  and all three Makefile defect verifiers. Fresh declared checks returned
  "test exited 0" and "lint exited 0" (40 UI files / 635 tests, React
  warnings). Focused D2 verifier exited 0: both Execute cases pass, overlay
  without SetOut fails with empty stdout, real CLI emits 62 stdout bytes,
  and inventory reports FIXED? D1/D2/D3/D8; D4-D7 remain REPRODUCED.
  D3 CI workflow and regression match supplied main exactly. Whitespace
  check passes. Independent verification remains pending.

- [D2 integration, 2026-10-18] Resolved the in-progress integration of
  origin/main a6afc8a0, retaining D1/D2/D8 inventory and working-memory
  records. make test retains D1 and D8 verification and adds D2 verification.
  Fresh declared checks returned "test exited 0" and "lint exited 0";
  UI: 40 files / 635 tests passed (React warnings emitted). Focused D2
  verifier also exited 0: uncached Execute cases pass, missing-SetOut
  overlay fails with empty stdout, real CLI emits 62 stdout bytes, and
  inventory prints FIXED? D1/D2/D8 with D3-D7 still REPRODUCED.
  Source remains unchanged by the overlay; git diff --check passes.
  Independent verification remains pending.

- [Defect D2 / FIX2 declared-check rework, 2026-10-18] Added the existing
  D2 verifier to make test so read-only cross-examiners can reproduce both
  the missing-SetOut negative control and the real CLI/inventory path via
  run_declared_check("test"). Updated verification instructions. Fresh
  declared test and lint both exited 0. Test output includes the expected
  negative-control failure (stdout empty), fixed CLI output of 62 bytes,
  FIXED? D2, and 40 UI files / 635 tests passed. UI emitted React warnings
  and a WebSocket port-in-use message. Independent acceptance remains pending.

- [Defect D2 / FIX2 rework, 2026-10-18] PR #114 inventory link corrected.
  Added `python3 scripts/defect-inventory/verify-d2.py`: uncached Execute
  test passes; Go overlay removing only SetOut fails the version assertion
  with empty stdout, without editing checkout sources. Sandbox and host
  run_command both passed this verifier; real CLI prints 62 stdout bytes;
  inventory reports FIXED? D2 and all other rows REPRODUCED. Fresh declared
  test/lint checks both exited 0 (Go suite; 40 UI files / 635 tests).
  UI printed React warnings and a WebSocket port-in-use message despite
  passing. Independent verification remains required.

- [Defect D2 / FIX2, 2026-10-18] Configure stdout in CLI Execute.
  TestExecuteOutput reproduces the missing wiring in a fresh process and
  verifies version stdout plus unknown-command stderr/exit 1 after the fix.
  Inventory reproducer reports FIXED? D2 with 62 stdout bytes (exit 1 is
  expected for a fixed row); other rows still reproduce. Declared test
  (make test: Go plus 635 UI tests) and lint both exited 0. This task enters
  independent verification, not acceptance.
- [D5 merge conflict repair, 2026-10-18] Resolved the NOTES.md conflict by
  retaining D5 implementation/live-server evidence and main's D3 notes.
  Main's CI step, regression and D3 inventory disposition remain intact;
  no executable changes in this pass. Focused CI regression exited 0.
  Fresh declared checks returned "test exited 0" (including D5 baseline/current,
  UI types, controlled and coordinator-backed browser journeys, Go suite and
  639 UI tests) and "lint exited 0". Inventory reproducer printed FIXED? D5
  with expected exit 1. git diff --check passed. Independent verification
  remains pending; no production deployment claim.

- [FIX2 D5 live-server rework, 2026-10-18] Added tagged
  TestKnowledgeHubBrowser, invoked by verify-d5.py/make test, serving the
  compiled UI with actual coordinator-backed API routes and isolated SQLite.
  Browser context supplies test bearer/checkout credentials; no API interception.
  Checks actual supported 200 payloads, missing-bearer 401, open/away/back/reload,
  persisted records and no policies requests. Controlled failure/Retry tests
  remain. Declared checks returned "test exited 0" (639 UI tests, Go suite,
  baseline four failures/current four passes, types and three browser journeys)
  and "lint exited 0". Renamed live journey to .journey.ts so generic E2E
  discovery cannot run it without its Go harness; focused host rerun returned
  "1 passed (1.6s)" and "PASS: TestKnowledgeHubBrowser". Inventory script
  printed FIXED? D5, exit 1. Local sandbox browser startup failed; host checks
  succeeded. Server also logged D4's /api/api/v1/approvals 404; no D4 changes.
  Inventory references PR #119 and exact local server configuration. No
  production deployment claim. Independent verification pending.

- [FIX2 D5, 2026-10-18] Removed Knowledge Hub policies request, state/type and
  unsupported enforcement section. Regular regression failed all four cases on
  baseline a6afc8a, then passed after repair. verify-d5.py exposes the same
  comparison, UI type check and isolated Chromium journey through make test.
  Browser responses are controlled; no live coordinator/deployment claim.
  Inventory reproducer returned exit 1 with FIXED? D5. Declared lint exited 0.
  First declared test failed on an overly strict browser request count (18 vs
  10); app main.tsx uses StrictMode. Test now checks every request is supported
  and each endpoint was requested for all loads. Final declared test exited 0:
  baseline/current comparison, UI types, both Chromium journeys, Go suite and
  639 Vitest tests in 41 files passed. Independent verification pending.
- [D3 merge conflict repair, 2026-10-18] Retained both D1 and D3 notes and
  inventory dispositions while resolving main 960edd36621d. No executable
  change in this pass. Focused workflow regression and seven negative controls
  pass. Full inventory reproducer exits 1 with FIXED? D1/D3 and REPRODUCED
  D2/D4-D8; D3 predicate on supplied main returns REPRODUCED. Direct Vitest
  with CI flags passes 635 tests in 40 files (exit 0), with React warnings and
  WebSocket listen EPERM output. Fresh declared gates returned "test exited 0"
  (including main's D1 baseline/current check, Go suite and 635 UI tests) and
  "lint exited 0". git diff --check passed; no conflict markers remain.
  Independent verification and GitHub-hosted execution remain pending.

- [Defect D3, 2026-10-18] PR #113 adds required serial Vitest to GitHub CI
  after UI dependency installation/build. Rework extracts the existing workflow
  validator and adds seven ordinary in-memory negative controls: removed step,
  conditional step, ignored failure, wrong directory, missing installation,
  conditional job and ignored job failure. Focused tests pass; the regular
  workflow test fails against origin/main 59f009c6 with
  `CI build is missing required UI vitest step` and passes after restoration.
  Full inventory reproducer prints `FIXED? D3` and exits 1 as documented;
  other rows still print REPRODUCED. Inventory now links #113.
  Host declared gates returned `test exited 0` (make test, Go suite and
  635 UI tests in 40 files) and `lint exited 0` (Go vet and UI ESLint).
  React test warnings did not fail the gate. Hosted CI remains unverified.
  Submitted for independent verification, not task completion.

- [D8, 2026-10-18] Remove 472 tracked build/cache artifacts without history
  rewriting; add exact root ignore rules and an ordinary Go Git-index/ignore
  regression. Artifact-free `make build` and rebuilt CLI version passed; D8
  reproducer reports FIXED? with zero tracked artifacts. Declared lint exited 0.
  First declared test exited 2: TestRetentionUnitNativeCallbacksAndStorageRefusal,
  `receipt missing`; isolated rerun passed, then unchanged declared test exited 0
  (Go suite and UI 40 files / 635 tests). Cause of initial failure not established.
  Inventory references PR #118. Rework adds verify-d8.py to make test for
  reproducible baseline-index failure and an artifact-free build/CLI journey.
  Local verifier exited 0: baseline regression fails on tracked artifacts,
  current regression passes; fresh make build and rebuilt version pass.
  Rework declared checks: test exited 0 (including D8 verifier and 635 UI
  tests / 40 files); lint exited 0. Independent verification pending.

- [FIX2 D1 verification access, 2026-10-18] make test now runs verify-d1.py
  before the Go/UI suites, exposing the baseline/current comparison through
  the existing host run_declared_check(test). Fresh host result: "test exited 0",
  including "PROVEN: original code permits outside reads and writes" and
  "PROVEN: current code passes the same ordinary regression", followed by
  Go packages and 635 UI tests across 40 files. Declared lint exited 0.
  Local inventory rerun exited 1 with FIXED? D1 and REPRODUCED D2-D8; the
  prior review's FIXED? D7 discrepancy remains unexplained. Direct D1 tagged
  test exited 1 with "D1 not reproduced" and "outside workspace".
  This pass changes verification wiring only; independent acceptance pending.

- [FIX2 D1, 2026-10-18] API file tools now reject lexical workspace escapes
  before reads, writes or directory creation via resolveFilePath. Dispatcher
  regression TestAPIFileToolsWorkspaceBoundary failed before the repair (outside
  contents returned and overwritten), then passed with persisted in-workspace
  round trips and refusal side-effect checks. Original defect-tag reproducer
  passed before; after repair it fails with "D1 not reproduced" and the inventory
  script prints FIXED? D1 (exit 1). This implementation run printed
  REPRODUCED for D2-D8; the independent review printed FIXED? D7 despite
  its standalone D7 test passing. Cause unknown; no D7 repair is claimed.
  SECURITY_MODEL scopes its guarantee to infrastructure/MCP and documents sh -c,
  lexical-only checks and the OS-sandbox roadmap. Existing resolver does not
  resolve symlinks; no symlink-resolution or sandbox mechanism added.
  No project format, loading, migration or legacy fallback changes.
  Declared checks returned "test exited 0" (make test: Go packages and 635 UI
  tests in 40 files) and "lint exited 0" (go vet and UI ESLint). UI output
  included a WebSocket port-in-use warning and React warnings without failures.
  git diff --check passed. Independent verification remains required.
  Rework: inventory now names openexec/openexec#112. The executable
  python3 scripts/defect-inventory/verify-d1.py uses a temporary Go overlay
  of api_runner.go from baseline 59f009c690e859e2accd259cdadc5e00c4b34439.
  It exited 0: the ordinary regression failed with unauthorized outside reads
  and writes on the baseline, then passed all 15 cases on current code. It
  rejects build failures as evidence and leaves repository sources untouched.
  Fresh declared checks after rework returned "test exited 0" (Go packages,
  635 UI tests across 40 files) and "lint exited 0". git diff --check passed.

- [CI repair verification manifest, 2026-10-01]
  Durable job openexec-identity-ci-repair verified b9c7dc97704e with Go 1.25.14:
  `go test -v ./...` passed, and race-enabled API duplicate-start plus both Wait
  regressions passed ten repetitions. The next step refused with `changed
  function missing from manifest: pkg/manager/manager.go:(*Manager).Wait`.
  Added Wait to the existing whole-function coverage scope and both regression
  tests to mandatory execution and selection. Coverage thresholds, baseline,
  missing-function refusal and importer behavior remain unchanged. Fresh identity
  verification passed all 22 mandatory tests; Wait is 10/10 covered statements,
  and every scoped function exceeds 90%. Compatibility/UI/type checks remain
  pending in the resumed durable job.

- [Fresh review of identity CI repair, 2026-10-01]
  Hosted run 36817948767/job 110227023325 fails
  TestWaitReturnsOnlyAfterTheRunGoroutine: `Wait on an unknown run: pipeline
  absent not found`. The cleanup repair changed the existing absent-run no-op
  contract. The matching correction appeared in the shared worktree during this
  review and was preserved: absent runs have nothing to join. Host verification
  of TestWaitReturnsOnlyAfterTheRunGoroutine, TestWaitJoinsStoppedAttempt and
  TestHandleStartRunDuplicate passed five repetitions each. Prior detached
  verification has no available progress/result record; full checks are pending.

- [Identity delivery CI repair, 2026-10-01]
  Hosted job 110221956165 (run 36816292665, Go 1.25.14) failed in
  TestHandleStartRunDuplicate with `TempDir RemoveAll cleanup: unlinkat ...:
  directory not empty`; its pipeline finished after test cleanup began.
  API fixtures now stop and join all attempts before closing SQLite/removing
  temporary files. Manager.Wait exposes the existing attempt completion channel,
  including event persistence; Stop's terminal status is not a join. No new
  persistent concepts, controllers, lifecycle transitions or identity changes.
  Focused duplicate-start test passed five times on Go 1.26.5; a real injected
  pipeline test proves stopped status cannot release Wait, cancellation of a
  wait is bounded, shutdown writes persist, and repeat waits succeed.
  Full Go 1.25.14 CI test command and compatibility/identity checks are pending
  durable verification. Existing identity repair and completed work retained.

- [Validated identity delivery preparation / US-012 / T-US-012-002]
  The single current record is docs/verification/plan-identity-delivery.md with
  machine record plan-identity-delivery.json and repository-local stored logs.
  scripts/verify-plan-identity-delivery-evidence.sh validates dispositions,
  candidate/log digests, mandatory checks/tests, per-function coverage >90%,
  all expected mutation failures and consistent outcomes. Complete preparation
  passes independently of pending D2; --goal-complete refuses absent current
  coordinator merge evidence. The --self-test fixtures exercise complete,
  incomplete and stale records, CLI rereads and refusal paths. Existing Go
  repairs/tests are retained; no production changes in this stage. See the single
  record for executed results and same-branch/PR #78 Console handoff. The ordinary
  local commit preserves work only; publication, canonical gates and D2 remain
  Console-owned. No native delivery actions or tasks were added.

- [Reviewed-wave delivery preparation / US-009 / T-US-009-001, 2026-10-01]
  Authoritative consolidated repair and delivery evidence, including dependency
  US-008 / T-US-008-001; older similarly numbered stories below concern other
  work. Local preparation is complete; D2 remains pending. Repair baseline:
  `38d20b4255926d311b295ff944b82e3673a89ac4`. Used the US-007 discovery entry
  below to construct `pkg/manager/testdata/reviewed-identity/{retained,subsequent}.json`.
  These are repository-local reproductions of its documented incident shape,
  not a claim to possess the original owner's database or deployed artifacts.
  Both same-title stories US-001 and US-005 change content, parent and task
  membership. In particular, US-001 moves from G-001 to G-002 and from retained
  T-US-001-001 to proposed T-US-001-002. A separate lifecycle case changes a
  same-title Goal and changes only task technical strategy to exercise propagation.

  Outcome: reviewed waves allocate compatible identities before persistence and
  import, while completed work and exact reviewed replay remain intact. Observed
  defect: title-only Goal/story matching disagreed with the importer's full-row
  equality, and occupied tasks moved even on exact content replay. OpenExec's
  native preparation/import boundary owns the repair. Reused ExistingLookup,
  RemapPlanIDs, next-free allocation, reviewedPlanRows, SQLite transactions,
  artifact persistence and import receipts. Read the Simple Loop contract and
  live Project context (accepted portfolio Goal revision 4); that wider context
  does not expand this selected identity task. No new architectural abstraction
  is needed. Complexity: zero new persistent concepts, transitions, owner
  decisions or runtime loops; replace the one-pass remapper with fixed-point
  resolution over its existing transient ID maps. Existing machinery retained.

  Implementation: preparation reloads retained state and refuses ambiguous
  duplicate/empty source IDs. The lookup compares all importer columns for
  Goals, stories and tasks, including ordered task membership, parent IDs,
  priorities, technical-strategy-expanded descriptions, max attempts and mode.
  Nil and empty arrays now compare equally, including retained JSON/SQL null.
  Strings and array order/duplicates remain significant. Absent/non-string mode
  defaults to afk; explicit empty mode remains distinct. Lifecycle, commit,
  approval and unrelated metadata do not participate in content equality.
  Iteration resolves changes propagated through structured dependencies and
  prose, rendering from the original bytes each time to prevent cascading
  substitutions. Allocations reserve incoming and retained IDs, including task
  prefix destinations. Exact task content can be reused. The exported legacy
  title-only lookup remains supported; native preparation supplies full content.
  The atomic importer's conflict/receipt enforcement was not relaxed.

  Corrected historical reproduction: the refused identity is US-001, not the
  planning story US-007. The rejected plan had substituted the planning-story
  number into incident evidence; the accepted plan restores US-001 and US-005.
  Historical incident text is not itself an executed reproduction. The following
  dependency-stage runs reproduced that exact refusal with repository fixtures:

  - Before production edits, `go test ./pkg/manager -run
    '^TestReviewedIdentityLifecycle$' -count=1` exited 1 with
    `reviewed stories US-001 conflicts with retained content`.
  - After implementation, temporarily removed only the Conflicts callback from
    preparePlanIDs, restoring title-only matching. The same command again
    exited 1 with that exact error (lifecycle test line 53 at that revision).
    Host `run_declared_check(check="test")` with that mutation returned
    `test exited 2`, `FAIL .../pkg/manager`, and make's test-target error. The host
    output was truncated; exact-error evidence comes from the focused command.
    Restored the saved source in a finally block, then reran the verifier.
  - The initial coverage measurement refused preparePlanIDs at 89.47% and the
    importer at 86.79%. Added real closed-store, cancellation, corrupt-metadata,
    incompatible-mode, malformed-JSON and receipt-FK refusal cases, verifying
    rollback rather than reducing the threshold. A newly written conversion
    assertion incorrectly expected an explicit afk metadata key; corrected it
    to assert the existing absent-key representation (the importer defaults it).
    That interim host run exited 2; the corrected assertion passed the dedicated
    verifier and final host test rerun.

  End-to-end evidence (real Manager.Plan, artifact files and SQLite): initial
  reviewed import; completed tasks with retained timestamp, attempts, branch,
  commit list and metadata; second-wave preparation/review/persistence/import;
  whole-column retained-record snapshots; artifact reread and canonical import
  validation; actual manager/store close and state.db reopen; both request
  replays without provider/reviewer calls; unchanged full-table/receipt snapshots;
  exact allocated-plan preparation; distinct request with exact allocated
  content and no duplicate rows. Initial import receipt remains byte-exact
  through subsequent waves; mismatched receipt bytes, input digest and run ID
  each refuse. Goal/story/task references, all supported prose fields, story
  and task dependencies and reserved IDs are asserted. Titles/decision metadata
  retain their established non-rewriting behavior; longer numeric tokens remain
  unchanged. Another journey injects a genuine post-review Goal conflict,
  proves no partial stories/tasks/receipt, restores the conflicting record and
  resumes the persisted approved plan after reopen without regeneration. Existing
  late-task-trigger rollback/retry and interrupted-review replay tests also run.

  Deliberately updated existing test:
  `TestReviewedPlanRefinementConflictRefusesBeforeRereview` is now
  `TestReviewedPlanRefinementAllocatesBeforeRereview`. Same-title changed content
  seen before rereview must receive fresh IDs and reach review, while rollback-only
  preflight persists no stories and the retained Goal stays unchanged. Genuine
  post-review conflicts still refuse in their separate atomic tests.

  Verification gate: `scripts/verify-reviewed-plan-identity.sh` runs two Python
  fail-closed controls and fresh instrumented Go tests, then requires all 15
  named tests in `scripts/verification/reviewed-plan-identity-scope.json` to
  execute/pass with no skips. The explicit 18-function manifest is nonempty,
  rejects omitted changed production functions against the baseline, and checks
  every expected instrumentation block. Each full function body must exceed
  90%, independently of aggregate coverage. Observed final statement coverage:

  | Scoped function | Covered / statements | Statement coverage |
  | --- | --- | --- |
  | RemapPlanIDs | 15 / 15 | 100.00% |
  | rewriteIDRefs | 6 / 6 | 100.00% |
  | nextFreeID | 4 / 4 | 100.00% |
  | nextFreeTaskID | 8 / 8 | 100.00% |
  | remapContentIDs | 59 / 59 | 100.00% |
  | rewritePlanRefs | 31 / 31 | 100.00% |
  | ReviewedGoalEqual | 1 / 1 | 100.00% |
  | reviewedArrayEqual | 5 / 5 | 100.00% |
  | ReviewedStoryEqual | 1 / 1 | 100.00% |
  | reviewedMode | 3 / 3 | 100.00% |
  | ReviewedTaskEqual | 1 / 1 | 100.00% |
  | preparePlanIDs | 18 / 19 | 94.74% |
  | reviewedIdentityConflicts | 12 / 12 | 100.00% |
  | uniquePlanIDs | 15 / 15 | 100.00% |
  | reviewedPlanRows | 17 / 17 | 100.00% |
  | ImportReviewedPlan | 1 / 1 | 100.00% |
  | ValidatePlanIdentities | 1 / 1 | 100.00% |
  | importReviewedPlan | 98 / 106 | 92.45% |

  Named lifecycle/canonical results (fresh execution, all PASS, no skips):

  - `pkg/manager:TestReviewedIdentityLifecycle`: PASS.
  - `pkg/manager:TestReviewedIdentityChangedGoalAndTask`: PASS.
  - `pkg/manager:TestReviewedIdentityDuplicateRefusal`: PASS.
  - `pkg/manager:TestReviewedPlanReplayAtomicImportFailure`: PASS.
  - `pkg/manager:TestReviewedPlanReplayRefusesConflictAndMissingAdapters`: PASS.
  - `pkg/manager:TestReviewedPlanRefinementAllocatesBeforeRereview`: PASS.
  - `pkg/manager:TestReviewedIdentityPostReviewConflictAtomicRetry`: PASS.
  - `internal/release:TestReviewedIdentityAtomicRefusals`: PASS.
  - `pkg/manager:TestReviewedPlanReplaySurvivesCancelledReviewAndCompletedImport`: PASS.
  - `pkg/manager:TestReviewedIdentityClosedStoreRefusesPreparation`: PASS.
  - `pkg/manager:TestReviewedRowsCanonicalConversion`: PASS.
  - `internal/planner:TestRemapContentReservationsAndReplay`: PASS.
  - `internal/planner:TestRewritePlanRefsAllFields`: PASS.
  - `internal/release:TestReviewedCanonicalFields`: PASS.
  - `internal/release:TestReviewedCanonicalNormalizationAndRetention`: PASS.

  Commands and observed results (refreshed for US-009 on 2026-10-01 unless
  explicitly labelled dependency-stage evidence):

  - `scripts/verify-reviewed-plan-identity.sh`: exit 0,
    `Reviewed identity verification PASS`; all manifest tests passed, none skipped.
    Scoped aggregate: 294/303 statements (97.03%); the threshold applies to
    each full function, not this aggregate. Lowest function coverage 92.45%.
    Python controls explicitly refuse absent coverage blocks and empty
    instrumentation. Dependency-stage temporary empty-manifest and
    omitted-RemapPlanIDs-manifest mutations both exited nonzero with their
    expected diagnostics; restored the manifest and reran successfully.
  - Host `run_declared_check(check="lint")`: `lint exited 0`, Go vet and UI ESLint.
  - Dependency-stage final host `run_declared_check(check="test")`: `test exited 0`; this
    executes `make test` (Go `go test ./...` plus UI Vitest). Manager passed
    in 114.879s; UI reported 40 files and 635 tests passed.
  - `make compat-test`: exit 0; existing `.openexec` and legacy `.uaos` status
    CLI journeys, LegacyProjectConfigFallback and LegacyTasksJSONFallback passed.
  - `make type-check`: exit 0; Go build and UI `tsc --noEmit` passed.
  - `git diff --check`: exit 0. Local Go commands use writable
    `GOCACHE=/tmp/openexec-identity-go-cache` via export, not command prefixes.

  Compatibility evaluation: no schema, migration, project discovery or fallback
  code changed. The protected current/legacy/JSON compatibility tests passed;
  full retained SQL rows, metadata and receipts survive new waves and reopen.
  Changed content intentionally obtains fresh IDs before review. Exact allocated
  content and same-request replay reuse existing identities. A new request that
  again supplies different content under historical occupied IDs is a new wave;
  this is not a cross-ID content-deduplication index. Canonical comparison tests
  cover every compared field and normalization distinction.

  Candidate binding: verification ran against implementation commit
  `74f5b0b96e492b2b417cf8320b80f5d27abce3ff`; this preparation changes only
  NOTES.md. No production code, tests, fixtures, manifests or executable scripts
  changed, so the fresh scoped verifier and compatibility/type/lint results
  apply to the final candidate's identical executable content. Full host test
  evidence above is inherited from that dependency commit, not claimed as a
  fresh US-009 full-suite run. The scoped Go run emitted a read-only module-cache
  stat warning but completed with exit 0 and every required test/coverage result.
  The accepted task verifier is `git diff --check && test -s NOTES.md` (exit 0).
  Final reread and Git diff establish that the evidence persisted and this is a
  documentation-only preparation. No tests were altered in US-009; the deliberate
  US-008 test change and its rationale are recorded above.

  Console handoff: the current stage's explicit ordinary-Git persistence rule
  supersedes the accepted artifact's older no-preparation-commit wording. This
  task commit preserves local work only. Console still owns publication, the
  canonical gate, independent review, presentation of the exact PR candidate to
  the owner (T-US-009-002), and authorized merge execution/evidence. Neither the
  local commit nor task completion nor owner acceptance satisfies D2. No actual
  default-branch merge evidence was supplied or established here; keep D2 pending
  until Console records the merged revision and corresponding default-branch/PR
  evidence for the verified candidate. The observed Console serving revision
  fa197cf8 (started 2026-10-01T02:57:45Z) does not attest OpenExec deployment.

  Separate follow-up: Agent Console's goal-mode verifier mismatch belongs in
  Agent Console's repository. It is distinct from the repaired engine import
  conflict. This task neither edits nor verifies that checkout, and OpenExec task
  statuses do not establish parent Goal delivery. Console must resolve/verify
  that follow-up independently; local preparation does not claim it complete.

- [Reviewed-plan identity discovery / US-007 / T-US-007-001, 2026-10-01]
  Inspection baseline: `e33bab1d3551da1236c9075dc153766c991e3467`.
  Implementation observations below are historical baseline findings; the
  consolidated US-009 entry above records the US-008 repair and verification.
  This entry is the authoritative record for this discovery task; similarly
  numbered schema/evidence stories below concern other work. Read root
  `AGENTS.md`, `AGENTS.local.md`, this memory, and
  `docs/OPENEXEC_SIMPLE_LOOP_ARCHITECTURE_CONTRACT.md`. Live Console
  `openexec_get_project(openexec)` reports accepted Goal revision 4, Professional
  Portfolio Stewardship, with unfinished V3 milestones; that portfolio context
  does not enlarge this task. The supplied slice is discovery for content-safe
  reviewed-plan allocation. OpenExec's native planning/import boundary owns it;
  reuse remapping, reviewed row conversion, SQLite transactions and receipts.
  No new abstraction is needed. Complexity delta: zero concepts, persistent
  state, transitions, owner decisions or runtime changes. Console retains
  delivery; the explicit current task instruction requires an ordinary candidate
  commit despite the older plan artifact's no-commit wording.

  **Observed interfaces and call paths (source inspection):**
  `internal/planner/remap.go` exports `ExistingLookup` with
  `GoalTitle/StoryTitle func(string) (string, bool)` and
  `TaskExists func(string) bool`; `RemapPlanIDs(*ProjectPlan, ExistingLookup) int`
  mutates the plan and returns the changed-ID count (nil plan returns zero).
  `pkg/manager/planner.go:preparePlanIDs(*planner.ProjectPlan) error` obtains
  the cached release manager and supplies its `GetGoal/GetStory/GetTask` lookups.
  It has no content comparator. Goal/story equality is raw title equality;
  occupied task IDs always move, even beneath an unchanged same-title story.
  `nextFreeID` scans from 001 and reserves retained and incoming IDs;
  `nextFreeTaskID` scans suffixes from 001, stripping a numeric trailing suffix
  or appending one otherwise. Thus allocation fills holes, not max-ID-plus-one.
  A story-prefix task rewrite only checks retained occupancy before suffix
  allocation; incoming reservations participate in the allocator, not every
  directly rewritten task ID. Duplicate incoming IDs are not rejected here.

  `Plan` dispatches a nonempty RequestID to `replayReviewedPlan` in
  `pkg/manager/planner_replay.go`. The durable path reloads the release cache,
  generates/validates, prepares IDs before persisting the artifact and reviewing,
  and converts approved plans with `reviewedPlanRows` before atomic import.
  Rejected refinements pass stale-base validation, preparation and
  `ValidatePlanIdentities` before artifact persistence and rereview. Initial
  generated plans do not have that rollback-only preflight. The non-request
  reviewed path also prepares before review; `importBoundPlan` prepares again
  and refuses changed reviewed bytes. Its sequential CreateGoal/CreateStory/
  CreateTask path is not the atomic reviewed-request importer, skips occupied
  rows and clears unknown goal references. Do not conflate these paths.

  **Observed canonical comparison and normalization:**
  `reviewedPlanRows(*planner.ProjectPlan)` returns
  `([]*release.Goal, []*release.Story, []*release.Task)`.
  `internal/release/reviewed_plan_import.go` compares existing rows by ID and
  these fields, not by title alone:

  | Row | Compared fields |
  | --- | --- |
  | Goal | title, description, success_criteria, verification_method |
  | Story | goal_id, title, description, acceptance_criteria, verification_script, contract, depends_on, story_type, priority, tasks |
  | Task | story_id, title, description, verification_script, depends_on, priority, max_attempts; metadata mode separately |

  The converter fixes story_type to feature, uses zero-based story/task indexes
  as priorities, sets max_attempts to 3, and builds each story's ordered task-ID
  list. Nonblank technical_strategy is appended verbatim to task description
  after `\n\nTechnical strategy:\n`; whitespace-only strategy is omitted.
  Other strings are not trimmed or case-folded. SQL compares scalar columns
  with `COALESCE(column,'')=?`; empty goal_id inserts as NULL. Incoming nil
  arrays serialize as `[]`; SQL `json(column)=json(?)` normalizes JSON formatting
  but preserves order and duplicates. Retained JSON `null` is not equivalent
  to `[]`. `Task.ExecutionMetadata` emits mode/decision_reason/decision_ref
  when present. Missing or non-string mode compares as afk, while an explicit
  empty string remains empty (it is not defaulted). Only mode is compared;
  decision metadata and other existing metadata are retained, not compared or
  replaced. Malformed retained task metadata refuses import. New task metadata
  is serialized, with nil becoming `{}`. SchemaVersion and RequirementID belong
  to the plan artifact but are not columns in this canonical comparison.

  **Observed reference rewriting:**
  Goal maps update story GoalID; story maps update story IDs and DependsOn;
  embedded story IDs in task IDs follow via first substring replacement;
  task maps update task DependsOn in a separate pass. Combined maps rewrite
  goal description/success criteria/verification method, story description/
  contract/verification script/acceptance criteria, and task description/
  technical strategy/verification script. Titles, RequirementID and decision
  metadata are not rewritten. `rewriteIDRefs` matches numeric task, story and
  goal patterns with greedy digits and task alternative first, so US-0011
  survives a US-001 mapping. The regex has no surrounding word boundaries:
  arbitrary prefixes or suffixes are not a proven exclusion. Historical IDs
  inside prose are also rewritten; fixtures must retain their own historical
  identifiers independently of this discovery story's numbering.

  **Observed transaction, retention and replay contracts:**
  `ImportReviewedPlan(ctx, goals, stories, tasks, stepID, runID, inputDigest,
  receipt) error` calls the private `importReviewedPlan(..., validateOnly bool)`;
  `ValidatePlanIdentities(ctx, goals, stories, tasks) error` uses the same code
  with blank receipt identifiers and validateOnly=true. Both require SQLite,
  hold manager/store locks and start one transaction with deferred rollback.
  Validation performs real constraint/insertion checks and rolls everything
  back; it creates no receipt or cache refresh. Dangling goal references are
  explicitly named. Other reference guarantees depend on the actual schema;
  JSON dependencies are not comprehensively validated by this function.
  Conflicting retained canonical rows or mode fail without overwriting them.
  Existing lifecycle status, timestamps, attempts, candidate/branch/PR data,
  approvals, commit associations and unrelated metadata are outside the insert
  comparison and have no update here. An empty retained metadata string fails
  decoding before the new-row metadata update can run.

  Import writes goals, stories, tasks and the run_steps import receipt in the
  same transaction. Any pre-commit failure rolls all these writes back;
  previously persisted plan/review artifacts and review receipts are outside
  that transaction and remain. Cache refresh happens after commit and unlock:
  a refresh error can be returned after durable success. Receipt replay looks
  up stepID first and requires exact inputs_hash, metadata receipt bytes, run_id,
  phase=plan, agent=reviewed-plan-import and status=completed. A matching receipt
  returns immediately without rechecking rows; a mismatch refuses. It is not a
  general database integrity scan and does not refresh the cache on that return.

  Durable request identity uses the RequestID digest for run/step IDs and a
  separate intent/options digest (compact adds a mode distinction). It checks
  run scope, retained input identity, plan/review hashes, exact artifact bytes
  and paths. Interrupted generation/review resumes retained results; review
  versions are archived, refinement dispatch accounting is persisted, and an
  approved replay imports with the exact retained receipt rather than allocating
  again. A new request with identical generated content does allocate again:
  receipt replay and allocator idempotence are distinct requirements.

  **Existing tests and fixture/reopen entry points:**
  `internal/planner/remap_test.go` covers fresh input, same-title story without
  tasks, different-title backlog collisions, dependencies, selected prose and
  suffix allocation. Its IdempotentReimport test does not prove populated-plan
  replay. `pkg/manager/planner_review_test.go` covers pre-review allocation and
  review races; `planner_replay_test.go` covers cancelled review, retained
  lifecycle/branch/PR state, input conflicts, missing adapters and a trigger
  abort on the second task with rollback/retry. Those replay tests construct a
  fresh Manager around the same state handle, not a true store reopen.
  `planner_refinement_replay_test.go` covers rollback-only conflict refusal
  before rereview, interrupted refinement accounting and dangling goals.
  `internal/release/reviewed_plan_import_test.go:TestReviewedStoryWithoutAGoalImports`
  only calls ValidatePlanIdentities despite its name; it is no persisted-import
  proof. Use its state.NewStore then release.NewManager setup to get the ledger
  foreign-key schema, not only a release-only schema.
  `newSchedulerTestEnv` in `pkg/manager/scheduler_test.go` builds temp workdir,
  `.openexec`, state.db, admitted provider fixtures and shared release manager.
  `planCompletionFunc`, `fixedPlanCompletion`, `replayRequest`, replayPlanFixture
  and replayReviewFixture provide deterministic adapters without real inference.
  For actual reopen use `e.mgr.Close(); e.closeState(); freshQueueManager(t,e)`
  from `task_restart_boundary_test.go`, as exercised by
  `planner_schema_recovery_test.go`; it opens the same state.db anew. Query
  `state.GetDB()` for durable snapshots rather than trusting cached getters.

  **Repository-local reproducer strategy (proposed, not yet executed):**
  Seed canonical retained goals, same-title stories US-001 and US-005, and
  T-US-001-001/T-US-005-001 tasks through the real reviewed importer. Snapshot
  their rows, completed lifecycle/commit/candidate metadata and exact receipts.
  Generate a new request with US-001's title unchanged but description/contract
  changed; use real preparePlanIDs -> artifact/review -> reviewedPlanRows ->
  ImportReviewedPlan and capture the retained-content conflict. Add a wave
  involving both historical stories and a same-title changed parent goal.
  Cover changed task content alone, changed ordered task membership, and parent
  changes even when titles remain equal. The expected pre-fix refusal follows
  from source, not an executed historical incident reproduction in this stage.
  After repair, assert fresh collision-free IDs and consistent structured/prose
  references, preserved old rows, exact request replay without provider calls,
  and no duplicates after genuine close/reopen. Replay an already allocated
  exact plan separately from restarting the same request. Inject a late SQL
  trigger failure, mismatched receipt and incompatible post-review row; require
  atomic refusal, unchanged old receipts and successful retry where appropriate.

  Proposed function-level unit scope: RemapPlanIDs, nextFreeID, nextFreeTaskID,
  rewriteIDRefs, preparePlanIDs, reviewedPlanRows, ImportReviewedPlan,
  ValidatePlanIdentities and importReviewedPlan, plus every new comparison or
  allocation helper introduced by the later repair. Enforce greater-than-90%
  statement coverage for each complete named function body with a frozen manifest
  that fails on missing functions; report each function and aggregate coverage.
  Table-test every canonical field, nil/empty arrays, ordered differences,
  parent mappings, technical strategy, mode defaults/conflicts, metadata
  retention, ID reservations and reference boundaries. Keep lifecycle/reopen
  tests alongside this unit gate; coverage alone cannot prove atomicity. If the
  later implementation intentionally allows changed-content allocation before
  rereview, reassess RefinementConflictRefusesBeforeRereview's expectation and
  retain a separate genuine post-review conflict test. No test changed here.

  **Protected compatibility and evidence limits:**
  `internal/project/project.go:LoadProjectConfig` tries `.openexec/project.json`
  then `.uaos/project.json`; `project_test.go` covers both canonical paths.
  `internal/release/manager.go:Load` bootstraps JSON only when story count is
  zero, logs bootstrap errors and refreshes SQLite caches. Bootstrap reads
  `.openexec/tasks.json` with a tasks array after goals/stories; it is distinct
  from reviewed import. Separately, `internal/tui/file_source.go:readProjectState`
  falls back to tasks.json progress when SQLite is unavailable; this is the
  reader exercised by LegacyTasksJSONFallback, not the bootstrap function.
  `internal/validation/compatibility_test.go` supplies
  existing-project status CLI fixtures plus LegacyProjectConfigFallback and
  LegacyTasksJSONFallback; `make compat-test` targets Compatibility there.
  Future allocation changes must preserve these paths and persisted completed
  work. This documentation-only diff cannot change their behavior; it does not
  claim a separately run compatibility gate or delivery. Historical plan artifact
  `83ff344c502c37740ec5e064c2328631a89152d66480c5fb35e095a771571c7a.json`
  supplies the discovery verifier and historical fixture names. Its incident
  descriptions and earlier NOTES delivery claims are historical assertions,
  not proof of this checkout's deployment. The supplied Console serving revision
  is not an OpenExec deployment attestation.

  **Executed verification:** host `run_declared_check(check="test", args=
  ["./internal/planner", "./internal/release", "./pkg/manager", "-run",
  "TestReviewed", "-count=1"])` returned `test exited 0`. Output showed
  `go test ./...` and the UI suite, rather than a filtered package invocation;
  do not interpret the supplied arguments as proof of targeted selection.
  Planner, release, manager and validation packages passed in that run. The
  initial request with a pipe-separated test regex was rejected by the check's
  argument allowlist before execution; retrying with an accepted argument
  obtained real exit evidence. No source or tests changed. The task's own
  `git diff --check && test -s NOTES.md` verifier passed after this edit.
  No identity repair, new lifecycle reproducer, coverage measurement, merge or
  deployment is claimed by this discovery stage.

- [Planner schema delivery / US-008 / T-US-008-004] Isolated repair-disabled
  regression, restored full verifier, host test/lint and compatibility/type checks
  passed. Exact commands, coverage, reopened import/accounting, refusal scope and
  delivery limitations live once in
  [schema delivery evidence](docs/verification/planner-schema-delivery.md).

- [Planner schema replay / US-008 / T-US-008-003] Public-runtime and manager
  correction journeys, strict discovery/no-skip replay verification and database
  reopen/import accounting evidence live once in
  [schema replay](docs/verification/planner-schema-replay.md).

- [Planner schema coverage / US-008 / T-US-008-002] Added parser/prompt/failure
  cases and a complete changed-function coverage gate. Scope, negative controls
  and current verification live in
  [schema unit coverage](docs/verification/planner-schema-unit-coverage.md).

- [Planner schema correction / US-008 / T-US-008-001] Strict scalar decoding,
  concrete diagnostics and bounded correction now reuse native refinement and
  durable reviewed-plan receipts. Fresh verification, restart/import evidence,
  changed-test rationale and check-dispatch limits live once in
  [schema correction evidence](docs/verification/planner-schema-correction.md).

- [Compact delivery / US-011 / T-US-011-001] D2 assessment, evidence limits,
  G-007 distinction and Console-owned candidate/PR75 delivery plus later
  agent-console parent retry live in the existing
  [compact delivery record](docs/verification/compact-requirement-evidence.md#pr73-and-delivery-evidence).
  Delivery mode checks that record structurally; it cannot certify an unobserved merge.

- [Compact planning / US-009 / T-US-009-001] Frozen discovery scope and
  [US-010 / T-US-010-001 repair evidence](docs/verification/compact-requirement-evidence.md)
  share one authoritative record. Compact output now declares scalar requirement_id;
  real compact generation/rejection/refinement/approval and manager database reopen
  preserve REQ-001. Both named reviewer mutations restore exact source. The recorded
  ten-function coverage is 365/384 statements (95.052083%); final command receipts,
  per-function counts, finding qualifications and runner limits live in that record.
  Targeted/package/compatibility/type/lint checks pass; full make test was interrupted
  by sandbox denial of api.anthropic.com, with no process exit code. The advertised
  host-check tool is absent; runner test verification and D2 remain outstanding.
  No decoder/authority/migration behavior changed; Console retains delivery.

- [Planner schema evidence / US-007 / T-US-007-003] Dependency inspection and
  fixture provenance are historical baseline evidence in
  [inspection](docs/verification/planner-schema-inspection.md) and
  [discovery](docs/verification/planner-schema-discovery.md). Their pre-repair
  array-coercion observations and missing-accounting findings are superseded by
  the US-008 implementation record above. They do not describe current behavior
  or prove deployment. Console retains delivery ownership.

- [US-020 / T-US-020-001] Merge origin/main while preserving the US-019 sync
  map and candidate planner/review documentation. Resolution and verification
  results are recorded in `docs/ARCHITECTURE.md` under the sync map.

- [US-015 / T-US-015-001] Strict aggregate, exact-candidate receipt validation,
  finding dispositions and completed evidence live once in
  [delivery evidence](docs/verification-evidence-delivery.md). D2 remains incomplete;
  actual Console adapter adoption is deferred until this API merges.

- [US-013 / T-US-013-003] Expanded recapture scope, strict coverage and story
  proof are maintained once in [recapture evidence](docs/verification-evidence-recapture.md).
  Native runtime behavior is unchanged; canonical delivery remains Console-owned.

- [US-013 / T-US-013-002] Added named-check legacy/restart matrix and strict
  task verifier. Current scope and executed evidence live once in
  [bounded recapture variants](docs/verification/recapture-variants.md).

- [US-012 / T-US-012-005] Admitted API and fixture proof live in
  [admitted evidence](docs/verification/admitted-evidence.md); current aggregate
  and deferred Console adoption disposition supersede earlier completion claims
  in [delivery evidence](docs/verification-evidence-delivery.md).
  Local strict coverage lives in
  [admitted unit coverage](docs/verification/admitted-unit-coverage.md).

- [US-014 / T-US-014-003] Storage checklist, strict coverage, isolated old-directory
  source/staging negative control and host checks verified. Consolidated commands,
  permissions, round trips and legacy policy live in
  [private storage](docs/verification/private-storage.md); coverage scope lives in
  [storage unit coverage](docs/verification/storage-unit-coverage.md).

- [US-013 / T-US-013-001] Requested resolver and admitted end-to-end fixture
  are present. Current task verifier and both resolver negative controls pass;
  refreshed host, dispatcher and compatibility evidence is maintained once in
  [named recapture](docs/verification/named-recapture.md). Historical receipts
  do not identify their executed command/revision; no deployment claim is made.

- [US-010 / T-US-010-001] Current G-007 review study and exact provenance live in
  [repair study](docs/verification/repair-study.md). All three source defects are
  accepted provisionally; repair proofs remain with US-012/013/014, aggregate
  and external D2 with US-015/Console. Historical G-006 fixture passes do not
  cover the empty-phase incident or actual Console adapter adoption.

- [US-011 / T-US-011-001] Final fail-closed technical composition and native
  journeys are maintained in [delivery evidence](docs/verification-evidence-delivery.md).
  Current baseline results, per-slice coverage, mutation proof and pending external
  D2 are recorded there; Console retains canonical gate and delivery ownership.

- [US-009 / T-US-009-005] Shared recapture dispatcher and fail-closed aggregate
  implemented. Fresh scenario, persisted boundary, coverage and protected-format
  evidence is maintained in [recapture evidence](docs/verification-evidence-recapture.md).
  Canonical gates and delivery remain Console/runner-owned.

- [US-009 / T-US-009-004] Protected-format recapture journeys and standalone
  verifier implemented. Exact verification results and source-backed limits
  live in [recapture compatibility](docs/verification/recapture-compatibility.md).
  Production behavior is unchanged; full canonical gates remain runner-owned.

- [US-009 / T-US-009-003] Dedicated recovery unit tests and the full-body
  fail-closed coverage verifier are complete. Scope, verification results and
  aggregation contract live in
  [recapture unit coverage](docs/verification/recapture-unit-coverage.md).
  Production behavior is unchanged; canonical gates remain runner-owned.

- [US-009 / T-US-009-002] Dedicated native recapture boundary journeys and
  standalone verifier complete. Persisted retry/terminal/repair and Settings
  completion-obligation evidence lives once in
  [recapture boundaries](docs/verification/recapture-boundaries.md).
  Shared composition is recorded in the T-US-009-005 evidence above.

- [US-009 / T-US-009-001] Existing authoritative recapture implementation and
  injected-executor repair verified on continuation. Added the remaining-budget
  interrupted restart journey to the required verifier manifest: exactly one
  remaining dispatch, durable exhaustion, no repair or restart refund, Settings
  still waiting. Current verification and implementation evidence live once in
  [legacy recapture](docs/verification/legacy-recapture.md).

- [US-008 / T-US-008-005] Shared retention dispatcher and fail-closed aggregate
  complete. Fresh real-command/reopen/repair, coverage and both discard mutation
  checks passed with an explicit no-skip scenario manifest. Commands, results,
  reload assertions and runner-owned gates are recorded once in
  [retention evidence](docs/verification-evidence-retention.md).

- [US-008 / T-US-008-004] Standalone isolated discard mutation verification
  and dedicated admitted repair fixtures are complete. Results and verifier
  controls are maintained in [mutation evidence](docs/verification/retention-mutations.md).
  Shared composition is recorded in the T-US-008-005 evidence above.

- [US-008 / T-US-008-003] Added dedicated retained-evidence unit tests and the
  standalone full-body coverage gate. Scope, baseline, executed results and
  aggregation artifact contract are maintained in
  [retention unit coverage](docs/verification/retention-unit-coverage.md).
  Real command, persisted repair/reopen, negative cases and verifier controls
  passed. No production behavior changed; canonical gates remain with the
  repository runner. Shared dispatcher and mutation ownership are unchanged.

- [US-008 / T-US-008-002] Implemented shared production bounded capture, private
  artifact reads, explicit toolchain allowlisting, redacted public diagnostics,
  and artifact-reference propagation separate from classification digests.
  evidence-boundaries and retained-result verifiers passed real commands through
  database reopen and persisted repair state; targeted gate/capture, engine,
  native deterministic command, terminal receipt, atomic task failure,
  repair/restart and admitted cancellation regressions passed. Discovery, shell
  syntax and diff checks passed. Broader package execution attempted blocked
  provider access; full canonical gates remain with the repository runner.
  Adapter contract and exact bounds are maintained in docs/ARCHITECTURE.md.
  Compatibility: no project-loading, schema or migration changes; existing
  legacy receipt parsing and repair/restart checks passed. Complexity: reuse of
  artifact files, run_steps and typed failures; no new scheduler concepts,
  transitions or owner decisions. New internal capture helper/private artifact
  payload only; capture-storage failure refuses repair authority. Independent
  coverage and mutation verification retain their assigned task ownership.

- [US-008 / T-US-008-001] Both engine APIs retain non-nil failed results.
  Pipeline terminal failure carries existing artifact references, stage identity,
  output and diagnostics; typed receipts alone authorize repair. References and
  evidence are persisted before repair generation. The retained-result verifier
  passed an actual admitted exit-2 process, both engine branches, database reopen
  and repair creation with exact argv/cwd, bounded streams and diagnostic marker.
  Existing forged-artifact, cancellation, atomic failure binding and repair/restart
  journeys passed, as did targeted engine/pipeline/gate tests, discovery, shell
  syntax and diff checks. Broader package execution attempted provider network
  access and was blocked by the sandbox; canonical gates remain with the runner.
  Compatibility: no project loading/schema/migration changes; legacy receipt
  parsing and existing repair/restart journeys remain supported. Complexity:
  no new persistent concepts, transitions or owner decisions; existing machinery
  reused. Independent mutation/coverage work retains its
  assigned task ownership in docs/ARCHITECTURE.md.

- [US-007 / T-US-007-002] Source-backed architecture, normalized requirement
  mapping and independent test/helper ownership are in
  [ARCHITECTURE.md](docs/ARCHITECTURE.md). Discovery passed (22 source declarations); nine verifier tests, shell syntax
  and diff checks passed. Full gates and live product delivery were not run; pending retention, recapture and delivery evidence belong to the
  task owners in that map. Earlier source inspection remains in
  [inspection evidence](docs/VERIFICATION_FAILURE_INSPECTION.md).

- [Simple Loop contract, 2026-09-14] `fix/native-reviewed-planning` exposes
  existing compact generation through `Manager.Plan`; native reviewed-plan
  replay/refinement/import and task execution remain canonical. Compact
  generation/review/import/replay plus real persisted-file task execution pass;
  removing compact selection makes the test fail. No Console Goal loop is
  required for that small-task journey. Full gates/review/publication and native
  brownfield acceptance remain pending. Do not reuse the exhausted PR185 live
  window. Contract: `docs/OPENEXEC_SIMPLE_LOOP_ARCHITECTURE_CONTRACT.md`.

- [Console handover seam, 2026-09-13] Building on the validated PR #54 slice:
  `Manager.Config.StageExecutor` admits Console execution without native/host
  fallback; `PlanRequest.RequestID` + accepted `Intent` retain exact reviewed
  plan IDs and import atomically using existing SQLite records. Console owns
  only initial product assessment and fresh Goal review at stable boundaries.
  Focused queue/repair, cancelled-review restart and import rollback checks
  pass. Composed canonical review/publication and live brownfield delivery are
  still outstanding; preserve the c7c2265 first-slice evidence as historical.

- [OpenExec-first task route, 2026-09-13] Current feature:
  `feat/task-oriented-goal-loop`, design/acceptance in
  `docs/TASK_ORIENTED_GOAL_ROUTE.md`. Reuse the planner/reviewer prompts,
  SQLite task ledger and blueprint engine. Opt-in sequential queue derives
  one same-story repair from a trusted failed-check receipt and resumes the
  original task; controlled-provider integration and restart tests exercise
  actual processes and files. This is not native/deployed Goal convergence.
  Next: finish independent review/full compatibility validation, then bridge
  Console admission/effects to this engine; do not enable the old unrestricted
  host command path or inherit expired grants. The outer fresh Goal review,
  second planning pass and long brownfield proof remain unfinished.

- [Non-inference readiness, 2026-09-12] APIProvider.Probe must never generate
  tokens outside an execution reservation. OpenAI-compatible adapters now use
  bounded GET model metadata; absent support is explicitly unknown, never a
  completion fallback. Console's unknown-state consumer must remain compatible.
  Focused HTTP fixtures and a restored-inference negative control passed; no
  live inference was used. CLI probes use auth-status commands, with unknown
  for unsupported status shapes. Both advertise non_inference_readiness so
  Console can refuse older inference-producing probe implementations before
  invoking them. Full validation and paired deployment remain.

- [Bounded execution context ceiling, 2026-09-12] A larger cumulative grant
  must not enlarge a bounded worker's authorized model context. Optional
  ContextTokenLimit now caps native admission independently of TokenBudget;
  hard_context_limit advertises enforcement, zero preserves legacy behavior.
  Focused tests and a removed-ceiling falsifier passed. Live paired verification
  remains owned by the Console recovery route; no provider was dispatched here.

- [V3 retrieval-context repair, 2026-09-10] Paired with Agent Console's bounded
  record retrieval/remote-fresh repair. Assembled native requests now receive
  padded context estimation and typed refusal; exact duplicate results can be
  reduced once without losing unique facts or granting capacity. Agent/execution
  suites and negative controls passed. Exact review/deployment and live native
  task advancement remain; estimates are not proof of tokenizer fit.

- [V3 bounded repair, 2026-09-10] Shrinking-context HTTP 400 diagnosed and
  locally repaired on the existing token-admission line. Preserve the owner's
  non-renewing grant; remaining delivery is exact-head review, guarded promotion
  and a reserved live request. See docs/HARD_TOKEN_ADMISSION.md. No new grant.

- [owner-experience evidence, 2026-09-03] Autonomous runs stopping after roughly
  30 minutes on a navigation budget defeats the core benefit: the owner should
  be able to leave while the system continues its task loop toward completion.
  This has interrupted both OpenExec and Rahoitettava work; after leaving for a
  walk, the owner returned to work that had stopped and could not progress
  without intervention. For local or otherwise unmetered providers, elapsed,
  cycle, tool-call, token, and estimated-cost limits should be visible warnings
  and telemetry by default, not execution blockers. Hard stops should remain
  for genuine safety, authority, consequential-effect, or explicitly metered
  spending boundaries. Another agent already owns the budget repair; preserve
  this as acceptance evidence and do not start a duplicate implementation.

- [task] Dogfood the experience-first operating model, then implement an
  advisory, provenance-labelled triage that the owner must refine and accept
  before architecture or implementation begins.
- Run the preregistered 20–30 task pointer-graph baseline/treatment evaluation
  on at least two repositories before deciding whether to fund Version 2.
- [task] V2.1: enforce freshness on every graph resolve/read, with stale re-resolution (trust gap from 2026-08-03 audit)
- [task] V2.3: secure graph query API for Agent Console + calls --direction outgoing (feeds Explore graph UX)
- [task] Supply OpenExec's read-only evidence dependency for Agent Console's
  external advisory MCP plan: complete V2.1 freshness and V2.3 secured graph
  access, then expose typed checkout-bound reads with body provenance.

## Questions

- Identity delivery evidence and the remaining coordinator requirement are tracked
  in [the delivery record](docs/verification/plan-identity-delivery.md#completion-boundary-and-external-follow-up).

- Compact planning: unverified D2 delivery and canonical runner checks
  are tracked in [compact requirement evidence](docs/verification/compact-requirement-evidence.md#verification-and-outstanding-questions).

- US-010 concrete integration/evidence gaps are maintained once in the
  [study resource boundary](docs/verification/repair-study.md#resources-authority-and-delivery):
  authorized Console writes/adapter execution, binary attestation and canonical
  gate/Settings reconciliation command mapping. No new owner decision requested.

- US-007 implementation interface questions are maintained in the
  [inspection evidence](docs/VERIFICATION_FAILURE_INSPECTION.md#unresolved-interface-questions-for-implementation).

## For me
- [me] Laki env for Juha
