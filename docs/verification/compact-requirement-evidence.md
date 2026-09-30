# Compact requirement identity — US-010 / T-US-010-001

## Scope and provenance

Discovery US-009 / T-US-009-001 froze baseline
`b17e136dab39bfccd4104bb9264e30595ea9118e`, the completion contract, complete
verification commands and ten executable function bodies in
[the scope manifest](compact-requirement-scope.json). This repair uses only this
candidate's source, local deterministic providers and fresh test output.
Historical recapture stories with reused task numbers are unrelated.

Outcome: preserve accepted requirement identity through compact planning.
Observed defect: compact rules require a field absent from their output example.
OpenExec's existing GenerateCompactPlan, parseResponse, ReviewPlan, RefinePlan and
manager replay own this boundary. Console owns delivery after the task queue.
Project context and the Simple Loop contract were read. No new product concepts,
persistent state, transitions, owner decisions or runtime helpers were added;
complexity delta is zero. Verification tooling reuses the existing Go AST inventory
and full-body coverage evaluator.

## Finding decisions

Review `308b33886fe7b51a19c503f324c7387a`, single LOW finding: **accepted**.

- **Root cause — accepted and repaired.** Baseline compact rules prohibited unlisted
  fields while requiring identity preservation. Its story example now declares
  scalar `requirement_id` beside `goal_id`.
- **Normal — verified.** TestCompactRequirementIdentityThroughReviewAndRefinement
  invokes GenerateCompactPlan with numbered intent, inspects the actual compact
  provider prompt and round-trips the decoded accepted identity.
- **Review — verified, with qualification.** A separate deterministic reviewer
  receives the serialized identity and explicitly rejects the first plan. Missing
  identity does not inevitably cause rejection: coverage is model guidance, not a
  deterministic requirement check. We do not claim that stronger behavior.
- **Refinement — verified, with qualification.** StoryFixPrompt already declared
  the field; the defect was potentially missing input identity. The test checks
  retained findings and identity, decodes the replacement bare array, retains the
  goal, and requires fresh independent approval of the changed plan.
- **Test gap — repaired.** TestAllPlanningPathsRespectSchemaAndCommitOwnership now
  decodes only each generation/compact/refinement output example using a scalar
  string field. Rule prose, intent and current-story input cannot satisfy it.
  Review still checks rules but is not asked to output a story. Existing canonical
  null/omission and CandidateCommitRule assertions remain unchanged.
- **Fix — verified.** One production prompt-line change; no decoder, manager,
  import, authority or compatibility behavior was replaced.
- **Prove — core cases verified; full make test sandbox-blocked.** The planner journey is supplemented
  by TestCompactRequirementIdentityPersistsAcrossManagerReopen: actual compact
  manager selection, independent rejection/refinement/approval, artifact re-read,
  SQLite close/reopen, receipt replay, retained identity and exactly one import.
- **Falsify — verified.** The output-key and identity-rule mutations run separately.
  Each requires exit 1, the named compact subtest failure and its specific diagnostic;
  compilation or unrelated assertion failures are refused. Both restore exact bytes
  in finally blocks, including command failures and catchable termination signals.

## Reusable fixtures and protected behavior

The manifest inventories existing array decoding/discovery, human-boundary, goal
retention, commit ownership, manager replay, cancellation, import atomicity and
retained-limit regressions, plus the new journey and refusal tests. Coverage executes
all listed planner/manager tests and fails if any required test is missing or skipped.
The original array symptom remains fixed by Story.UnmarshalJSON accepting string,
null and string lists; parseResponse preserves the actual envelope's diagnostics.
No test was weakened to replace this compatibility behavior.

Added failure tests cover malformed scalar identity, invalid plans, provider errors,
local native-provider fallback, invalid intent, failed artifact storage, concurrent
receipt mutation, failed database writes, tampered hashes/paths/reviews/history,
unchanged refinement, legacy zero limits, negative limits and lowered limits.
They assert specific refusals and no task import after failure. The native fallback
uses a newly written local shell fixture, never an ambient model provider.

The discovery note's stronger scalar-enforcement/bounded-schema-repair prerequisite
is not a requirement of this prompt repair and is not implemented at this baseline.
In particular failed refinement attempts do not consume a persisted schema budget.
We preserve existing recovery/coercion and retained successful-review limits without
claiming a new durable failed-attempt accounting guarantee. This qualifies the
historical discovery inference rather than inventing extra production machinery.
Existing .openexec, .uaos and tasks.json loaders and migration code are untouched.

## Executable coverage scope

The ten frozen full function bodies remain in scope; no functions or error branches
were removed. No production helper was introduced. Python-only verification helpers
are tested independently as specified by the frozen scope policy.
The coverage script generates a fresh count profile, inventories Go bodies with the
existing AST helper, independently instruments expected blocks, and rejects absent
functions, missing/mismatched profile blocks and empty denominators. Its threshold
is statement-weighted `10 * covered > 9 * total`, not rounded percentages or averaged
function percentages. Per-function counts are retained in the repair result.
Initial measurement was 301/384 (78.39%); refusal coverage raised it to 365/384
(95.052083%). The final source-bound result is recorded below.

## G-007 completion and verification

The scope manifest retains the authoritative completion contract and full verification
script verbatim. `scripts/verify-compact-requirement-story.sh` executes those exact
commands, with `--step 1` through `--step 8` supporting separate runner dispatch.
Every command gets an exit-code receipt and log under the candidate-local ignored
`.openexec/compact-requirement-checks/` directory. Mutations precede final clean-source
package/full/compatibility/type checks. Coverage and mutation receipts bind the
production/test/verifier source digest; repair evidence refuses stale receipts.
Discovery mode still certifies discovery only. Repair mode certifies the local
prompt/journey/coverage/mutation slice, never remote delivery or the canonical gate.

## PR73 and delivery evidence

Local base ancestry contains #73's array-coercion and decode-diagnostic repair
`be19a5b695db31b9b2146b15bd01e613be55e4c6`. This supports agreement that the original
array failure is already fixed in code. It is not a hosted merge/deployment receipt.
D2 status: **unverified**. The observed Console process revision is not OpenExec
production evidence. Publication to retained PR75, canonical gate, review-disposition
submission and the exact merge decision remain Console-owned. This stage uses an
ordinary candidate-worktree commit; it does not publish or request another review.

## Verification and outstanding questions

Final command receipts and repair evidence are recorded in
[compact requirement results](compact-requirement-results.json).
The Python verifier suite checks absent/narrowed/duplicate scope, missing helper,
missing fixture/disposition, stale proof, exact 90% refusal, empty denominator,
missing profile blocks, unrelated mutation failures and restoration after command
failure. The unknown-mode test now uses an actually unsupported mode because repair
is deliberately supported. No existing product assertion was relaxed.

The advertised `run_declared_check` tool was not present in this session's callable
catalog (searched by name and check capabilities); no host check result is invented.
Full canonical gates and external D2 remain runner/Console responsibilities.

Final local checks: targeted planner contract/journey, whole planner and manager
packages, strict coverage, both mutations, 11 Python verifier controls, `make lint`,
`make compat-test`, `make type-check`, shell syntax and `git diff --check` pass.
The eight-command story script was dispatched with its step interface;
all commands were attempted. `make test` was interrupted twice by the execution
sandbox with `Network access to "api.anthropic.com" was blocked: domain is not on
the allowlist for the current sandbox mode.` The interrupted command has no exit
code; its streamed log and started receipt are retained, and the committed result
marks it sandbox-blocked, not passed. Remaining commands were still executed.
A host `test` check is needed to complete this broad verification; no credential,
owner judgment, publication or expanded effect was requested for the local repair.
The sandbox refusal is not represented as a product regression or delivery failure.

The initial 90-second whole-manager coverage timeout was corrected by selecting
all relevant named planning regressions for coverage (full function scope unchanged)
and keeping the separate whole-package test. An initial legacy-limit test fixture
was corrected to update its synthetic history consistently with its seeded retained
receipt; production history-conflict protection remains asserted in its own case.
Default Go cache writes were refused; reruns use a writable temporary Go cache.
No altered expected behavior or skipped product tests were used to obtain coverage.
