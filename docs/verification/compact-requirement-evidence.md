# Compact requirement discovery — US-009 / T-US-009-001

## Scope and provenance

This establishes the baseline, not completion of US-010's repair or G-007.
Inspected candidate HEAD is `b17e136dab39bfccd4104bb9264e30595ea9118e`.
The retained plan is
`.openexec/artifacts/plans/f350fbd52b02bf28e883aec989bca57bf4022c732a092a1e7a1d6ecbe9a12bcd.json`;
its US-009 contract and US-010 complete verification command are copied verbatim
into `compact-requirement-scope.json`, because the artifact is local ignored state.
Earlier US-009 recapture evidence concerns a different story and remains historical.
Only this checkout and its git objects/refs were inspected for product evidence;
no retained temporary artifacts or sibling checkout supplied evidence.

Outcome: consistent scalar requirement identity across compact planning.
Observed state: compact schema omission, reusable native planner/review/replay.
Owning loop: OpenExec planning; deterministic delivery belongs to Console.
Reuse GenerateCompactPlan, parseResponse, ReviewPlan, RefinePlan and replayReviewedPlan.
No new runtime abstraction is needed. Complexity delta: zero product concepts,
state, transitions, owner decisions or replaced machinery; only discovery tooling.
Project context was read; its broader current Goal does not expand this selected task.

## Finding decisions

- **Root cause — accepted.** `internal/planner/prompt.go:48` defines
  RequirementIdentityRule, including preservation and “never add fields” guidance.
  CompactStoryGenerationPrompt at line 211 includes that rule at line 221 but its
  exact JSON shape at lines 225–235 omits requirement_id. This is a source-proven
  contradiction, not an observed stochastic model failure.
- **Normal — accepted as a reachable risk.** `Planner.GenerateCompactPlan`
  (`internal/planner/planner.go:157`) selects that shape; `parseResponse` at 165
  uses Story.UnmarshalJSON at 57, which sets omitted/null identity to empty.
  Existing `TestPlanner_GenerateCompactPlan` checks shape selection, not REQ-001
  retention. The parser does not itself require accepted requirement coverage.
- **Review — accepted as an inference, not inevitable rejection.**
  `pkg/manager/planner.go:142–178` sends compact plans to independent ReviewPlan;
  `internal/planner/review.go:21–59` serializes the empty identity and applies
  reviewer and deterministic lint results. Requirement coverage is model guidance,
  not a deterministic rejection in this function. RequestID takes the replay path
  at planner.go:64 instead; it too uses compact generation and independent review.
- **Refinement — qualified agreement.** `RefinePlan` (review.go:65–95) sends the
  rejected plan and findings through StoryFixPrompt; that prompt already lists
  requirement_id at prompt.go:357. It inherits a possibly missing *input identity*,
  not the compact output-shape omission. A corrected answer can recover the ID
  from intent, but no deterministic code guarantees it. `carryGoals` at 100 retains
  goals; `preserveHumanBoundaries` protects retained HITL work.
- **Test gap — accepted.** `contract_consistency_test.go:62–84` asserts null
  semantics and conditional commit ownership on all four prompts, never their
  output shapes. The existing tests pass on the contradictory baseline.
- **Fix — accepted for US-010.** Add scalar requirement_id beside compact goal_id;
  assert output-format fields in generation, compact and refinement separately
  from rule prose. Review outputs approval metadata, not stories, so it must retain
  the shared-rule assertion but must not be required to output requirement_id.
- **Prove — accepted, with contract additions.** US-010 must exercise actual compact
  REQ-001 generation, decoding, independent rejection, refinement and fresh approval,
  plus the complete verification command retained in the scope manifest.
- **Falsify — accepted, pending US-010.** Remove only the compact output key: the
  compact format subtest must fail. Separately remove only its identity-rule
  insertion: the existing null-sentence assertion must fail. Restore source and
  rerun the complete verifier. Discovery does not claim these repairs or mutations.

## Reusable fixtures and protected behavior

The scope manifest names existing executable tests and verifies their presence.
`mockProvider` in internal/planner/planner_test.go captures the last prompt and
returns controlled JSON/errors. Extend it or use response sequences for REQ-001;
use separate reviewer and generator providers to preserve independence.
`TestEnablingRequirementSurvivesRefinementAndReview` checks null-to-empty wire
round-trip, retained goals, conditional commit ownership and explicit rejection;
it uses full generation, not compact accepted-identifier coverage.
`TestPlanner_ParseResponse` covers markdown, both envelopes, array coercion and
named malformed-field errors. The schema-discovery fixtures and provenance in
`pkg/runtime/testdata/planner-schema/` provide sanitized empty/single/multiple
mapping responses and scalar controls; current public acceptance is not bounded repair.

Manager fixtures: `replayPlanFixture`, `replayReviewFixture`, `replayRequest`
(pkg/manager/planner_replay_test.go:10–15), `rejectedReplayReview`
(planner_refinement_replay_test.go:10), `planCompletionFunc` and
`newSchedulerTestEnv`. Reuse these for independently rejected then approved plans,
artifact digests, run_steps review history, task progress and exactly one import.
Preserve cancellation resumption, changed-request refusal, missing-adapter refusal,
atomic import rollback, goal/identity conflicts, unchanged-refinement refusal,
retained review limit (raising config does not replenish it), goal retention,
HITL metadata/dependencies, stale-base checks and conditional commit ownership.

Limits: the “Restart” test creates a fresh Manager over the same state handle;
it does not close/reopen SQLite. planner_replay.go:240–272 increments ReviewRound
only after successful refinement/validation/artifact persistence. No consumed
schema-correction attempt is persisted before dispatch, and failed refinements can
repeat. Do not describe these tests as proving bounded array correction or durable
failed-attempt accounting. The accepted schema-recovery work remains a prerequisite;
US-010 must recheck its presence and protect it rather than silently replacing it.
No production or compatibility behavior changes here: .openexec, .uaos and tasks.json
loading remain untouched. Discovery does not prove new compatibility behavior.

## Executable coverage scope

`compact-requirement-scope.json` is the nonempty, additive manifest frozen before
repair. Each function has its path, symbol, baseline line, area and inclusion reason.
Measure whole bodies (including error paths) with statement-weighted Go coverage,
strictly greater than 90 percent across this scope; do not average function rates,
count prompt constants, or drop orchestration because it lowers coverage.
New production helpers in planner/manager must be added before reporting coverage.
Dependencies not changed by the repair remain protected through the named regressions;
changed/new production helpers extend the manifest. Discovery validates symbols,
minimum scope and new helpers against the baseline. It measures no coverage itself.

## G-007 completion and verification

The authoritative completion wording and full implementation verification script
are retained verbatim in the scope manifest. Required repository outcome: generation,
compact and refinement formats declare scalar requirement_id; deterministic REQ-001
survives generation/rejection/refinement/approval; both targeted mutations fail for
their intended reasons; affected executable statement coverage is >90%; protected
schema-recovery, human-boundary, commit-ownership and retained-budget regressions pass.
Run the US-010 complete script on restored source, including targeted tests, coverage,
mutations, planner/manager tests, make test, make compat-test, make type-check and
repair evidence validation. Discovery passing satisfies only this prerequisite.

## PR73 and delivery evidence

`git show be19a5b695db31b9b2146b15bd01e613be55e4c6` records the #73-labeled fix:
Story accepts string/null/list and parseResponse reports the decode error for the
actual envelope. Its planner tests still assert joined multiple IDs. This supports
agreement that the original array-decode symptom was fixed at this base; it does
**not** support a claim that scalar enforcement and bounded schema repair already exist.
`git merge-base --is-ancestor be19a5b6 HEAD` and the same command against the locally
cached `origin/main` both exit 0. At inspection origin/HEAD names origin/main and
origin/main is `3fe65016736b86c59710a350dbf7ab5fa0c36bdd` (#74).
This is local commit content and cached-ref evidence only; ancestry and a #73 subject
are not an independently observed hosted merge receipt or current remote state.

D2 status: **unverified**. No hosted merge/deployment was checked. Earlier notes and
the observed Console revision `6dbeb1fc` are not OpenExec default-branch delivery
proof. Console owns publication to the retained PR75, canonical gate, review
308b33886fe7b51a19c503f324c7387a disposition submission and exact merge decision.
The later agent-console parent retry is external. No replacement PR, review request,
publish, merge, deployment or Navigator task selection belongs to this stage.

## Verification and outstanding questions

Fresh local baseline command (exit 0):
`go test ./internal/planner/ -run 'TestAllPlanningPathsRespectSchemaAndCommitOwnership|TestEnablingRequirementSurvivesRefinementAndReview|TestPlanner_ParseResponse|TestRejectedPlanRefinesThroughExistingPromptAndRequiresFreshReview' -count=1 -v`.
This proves the current test gap and preserved covered behavior, not the new repair.
Fresh manager baseline command (exit 0):
`go test ./pkg/manager/ -run 'TestCompactReviewedPlanUsesExistingPlannerAndReplay|TestReviewedPlanRefinesRejectedReviewAndImportsOnce|TestReviewedPlanRestartResumesRefinementWithinRetainedLimit|TestReviewedPlanReplay' -count=1`.

Task verification: `bash scripts/verify-compact-requirement-evidence.sh discovery`
exited 0 with 10 functions, 19 fixtures, compact output field absent, repair_verified
false and delivery_verified false. The command re-reads saved files, verifies
baseline ancestry/source facts and owner-reserved notes, and reports current HEAD.
`python3 -m unittest discover -s scripts/verification -p compact_requirement_evidence_test.py -v`
exited 0 (five tests, including table-driven negative controls): invocation outside
the working directory, empty/duplicate/missing/nonlocal scope, missing reasons,
changed contract/verification, absent fixtures/dispositions, false delivery,
omitted newly introduced helpers and unsupported modes. `git diff --check` passed.
No existing test assertions were changed; new tests validate discovery fail-closed
behavior. Discovery's own initial fixture-name and newline-comparison errors were
corrected and the full discovery suite rerun. The user's two product prompt
mutations remain explicitly assigned to US-010, not represented by these controls.
The advertised run_declared_check tool is absent from this session's callable catalog;
no host lint/test result is claimed. Canonical full gates remain runner-owned.

Outstanding: US-010 must reconcile the stronger scalar/recovery prerequisite with
current coercion, then prove the complete repair checklist. D2 needs supported
external delivery evidence; neither requires inventing owner approval in this task.
