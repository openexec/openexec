# NOTES

Raw capture. One line per thought, any grammar.

## Now

- [Candidate evidence preparation and Console handoff / US-012 / T-US-012-001, T-US-012-002]
  Current D1 receipts, tested candidate, later evidence-only edits, four findings,
  every case/falsifier, scratch/reload evidence and limitations live once in
  [the delivery record](docs/exhausted-task-delivery.md). Preparation and refusal
  paths reread local artifacts; actual delivery refuses absent coordinator merge
  evidence. Handoff preserves the existing PR/review and Console authority;
  exact task verification, eight self-tests and host lint passed. D2 stays pending;
  separate merge validation refuses absent evidence. No native delivery or HITL
  task was added. No publication, acceptance, merge or deployment is claimed.

- [Trusted operator correction / US-011 / T-US-011-003]
  Shipped `openexec task correct` reuses the operator-session boundary and native
  manager authorization, requiring explicit correction authority and a decision
  reference. Implementation, scratch transcript, mutation proof, compatibility
  evaluation and Console transport follow-up live once in
  [the operator interface record](docs/verification/task-correct.md).

- [Required correction task scripts / US-011 / T-US-011-002]
  Implementation and verification contract: [task-script record](docs/verification/correction-task-scripts.md).
  Native correction now enforces task scripts alongside applicable plan checks,
  including planner-imported and legacy no-plan tasks. Shared recapture command
  resolution, executor effects, native completion and retained bindings remain
  authoritative. No new schema, state, loop, grant or owner decision.
  Verification results are recorded in that single task record.


- [Bounded correction refusals / US-011 / T-US-011-001]
  Native pre-admission correction validation now records consumed terminal
  refusals through a conditional SQLite update, preserving failed status,
  attempt counters and original failure evidence. Invalid retained bindings
  differ from operational store/I/O errors. The existing queue skips recorded
  refusals and drains independent tasks; exhausted failed boundaries expose the
  original evidence_id and attempt_limit without leaking reasons in Error().
  Fresh explicit decisions can replace only never-admitted refusals without
  fresh evidence. Prior decisions remain protected in task_correction_history;
  replay, replacement and admission/refusal races cannot double-admit.
  Reused the native queue, task metadata and SQLite conditions. Complexity:
  no new loop, task state, authority object or owner decision type; one terminal
  correction outcome and an audit array within existing task metadata.
  Live Project context and Simple Loop contract read before implementation.
  Added real manager/SQLite reopen journeys for invalid receipts, candidate,
  branch, state hash, current/newer plans, graph, task binding and malformed
  correction/plan records; independent candidate changes and store failures;
  replacement history and concurrent admission/refusal/authorization tests.
  Strengthened TestCorrectionNativeQueueRefusals for independent completion,
  failed 3/3 retention, zero correction checks, restart and fresh authorization.
  Updated TestTaskQueueBoundaryKeepsLegacyHumanFailureAndLimitsDistinct,
  TestFreshTaskQueueReopensFailedTaskWithAttemptsLeft and
  TestFailedRepairTaskUsesItsOwnAttemptsNotRepairCreation to assert the accepted
  attempt_limit classification (and original receipt), replacing stale failed
  expectations reproduced by host test exit 2. Admission validation assertions
  were added to TestCorrectionInvalidAuthorityAndClosedStore.
  Verification: task script exited 0, including three compiled source-overlay
  controls removing refusal persistence, boundary kind and evidence projection,
  followed by a passing unmodified native queue rerun. Focused admission test
  and final host lint/test exited 0 (Go suite and 635 UI tests).
  Shell syntax and git diff --check passed; no assertions were weakened.
  Compatibility: no schema, project loader, migration or fallback changes;
  legacy absent-correction records retain ordinary scheduling semantics.
  New audit metadata is stripped on untrusted creation and preserved on update.
  Publication, full canonical gate, independent review and merge remain Console
  work after the task queue; this stage makes no delivery or deployment claim.

- [Exhausted runnable records / US-010 / T-US-010-003]
  Current implementation and evidence schema: [records contract](docs/verification/exhausted-task-records.md).
  Structured source/checklist references consume the existing case matrix and
  coverage scope. Records validation is separate from native preparation and
  actual coordinator merge proof; D1/D2 remain outstanding for this candidate.
  Repaired shared planning semantics before verifier implementation; reused
  story contract and native queue without a runtime schema or lifecycle change.
  Fresh `python3 scripts/verify-exhausted-task-records.py --self-test` exited 0:
  14 self-tests passed, including actual native queue checks, SQLite reopen,
  planning round trips and three compiled source-overlay refusal controls with
  restored-source positive reruns. CLI reread validates 35 required cases and
  a source-derived denominator of 49 whole functions / 1434 statements; this is
  scope, not measured repair coverage. Upstream contract verifier also passed.
  Host declared lint and test both exited 0; compatibility, Go/UI type checks
  and git diff --check passed. Final strengthened native fixture was rerun by
  the self-test after the host suite started; no production change followed it.
  Four review findings remain provisional/unrepaired in this selected stage;
  evidence intake refuses missing native runs, falsifiers and coordinator merge.
  No existing assertions weakened. Existing/legacy loading remains unchanged.
  Ordinary Git preserves this stage; Console retains publication, canonical gate,
  review resolution and the owner's external merge decision. No delivery claim.

- [Exhausted correction review contract / US-010 / T-US-010-002]
  Single contract: [review checklist and scope](docs/verification/exhausted-task-review-contract.md).
  Four source-supported provisional accepted dispositions have explicit
  qualifications; 35 cases map exact test selectors, assertions and falsifiers.
  Missing future tests are unverified obligations, not passes. Coverage unions
  the existing inventory, correction production files and every changed/new
  production function, including CLI/shared registration; source statements
  missing from profiles count uncovered. No coverage exclusions are permitted.
  Fresh `bash scripts/verify-exhausted-task-review-contract.sh` exited 0:
  six contract-tool tests passed and the persisted JSON resolved 49 whole
  functions. Empty native test events and empty count coverage were separately
  exercised through the verifier CLI and refused. Host declared lint exited 0
  (Go vet and UI ESLint); git diff --check passed. Go emitted a read-only module
  stat-cache warning while its AST helper build still exited 0.
  No production/Go tests changed; current and legacy project behavior unchanged.
  Production repair journeys, their mutants, operator scratch flow and final
  review resolution remain for implementation/verification, not this definition
  stage. No D1/D2 or deployment claim. Local Git preserves this stage; Console
  retains publication, canonical gate and owner merge decision.

- [Exhausted task delivery handoff / US-009 / T-US-009-001, T-US-009-002]
  Single current package: [delivery preparation](docs/exhausted-task-delivery.md)
  and its JSON companion. Preparation complete; D2 and delivery pending until
  Console supplies actual OpenExec default-branch merge evidence. The package
  specifies verified-source → candidate/PR head → merged revision and target
  branch evidence, including squash/rebase equivalence. Missing evidence does
  not establish non-delivery; Console serving revision establishes neither.
  Console owns candidate commits, publication, canonical gate, independent
  review and requesting the owner's exact merge decision. Preparation is not
  approval. No decision or delivery references were fabricated.
  Persisted package reread, preparation verifier and three boundary test methods
  (eleven persisted refusal fixtures) passed. Fresh host lint exited 0. Native
  checks remain preparation-stage evidence. No tests/production behavior changed;
  no acceptance or delivery action taken; later task commits preserve the handoff.

- [Exhausted task correction / US-008 / T-US-008-001, T-US-008-002, T-US-008-003]
  Native correction/disposition interfaces remain documented in
  [the implementation record](docs/verification/exhausted-task-reconciliation.md).
  Current inventory reconciliation, fail-closed coverage, source-overlay removal
  proof and fresh required-check results belong once in
  [the final verification record](docs/exhausted-task-reconciliation-evidence.md).
  Fresh default acceptance (coverage plus exact-refusal overlay), independent
  discovery, host test/lint, compatibility, type-check and embedded build passed.
  The record contains measured coverage, rollback/reopen assertions, verifier
  refusal controls, actual exit results and limitations; temporary overlays were
  removed and candidate production source was unchanged.
  This concerns exhaustion, not the older identity/retention stories sharing task
  numbers. Console claims in older notes are historical; publication, canonical
  gates, independent review, owner acceptance and merge remain Console-owned.

- [Exhausted correction review discovery / US-010 / T-US-010-001]
  Current source assessment and all four review completion checklists live once
  in [native integration discovery](docs/exhausted-task-discovery.md).
  Replaced stale pre-implementation assertions; distinguished source observations,
  frozen baseline evidence, proposed repairs and cross-repository uncertainty.
  Read repository instructions, working memory, Simple Loop contract and live
  Project context; its portfolio roadmap does not expand this selected stage.
  The discovery-time absence of a shipped correction caller is superseded by
  T-US-011-003 and its operator interface record. That earlier stage recorded
  discovery rather than implementation or review-resolution evidence.
  Fresh task verifier `bash scripts/verify-exhausted-task-discovery.sh` exited 0:
  five verifier-control tests passed, the frozen native journey passed, and its
  replacement JSON was reread and validated. This reproduces the baseline, not
  the four newly reviewed repair scenarios. Host run_declared_check(lint) exited
  0 with Go vet and UI ESLint; git diff --check passed. Full gates, new correction
  regression/negative controls and operator scratch journey were not run here.
  No production code or tests changed; current/legacy loading is unchanged.
  Ordinary Git preserves this stage; Console retains publication, canonical gate,
  review resolution, owner acceptance and default-branch merge evidence.

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

- Exhausted correction: OpenExec verification and dispositions are recorded in
  [the delivery record](docs/exhausted-task-delivery.md). Console authority
  transport, live incident bindings, canonical review and actual merge evidence
  remain external follow-ups; no new owner decision is requested by this stage.

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
