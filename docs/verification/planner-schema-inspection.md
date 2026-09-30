# Planner schema inspection

US-007 / T-US-007-001, inspected 2026-09-30. Production baseline:
`be19a5b695db31b9b2146b15bd01e613be55e4c6`. This is inspection evidence,
not schema-recovery implementation or delivery evidence.

Read Console Project context through `openexec_get_project(project="openexec")`,
NOTES.md, repository guidance and the Simple Loop architecture contract.
The project intent remains portfolio stewardship with independent outcome
authority. This accepted slice concerns bounded planner schema correction;
it does not authorize changes to the Console routing project or delivery.
The accepted task decomposition is the local plan artifact
`f87547170766da4b226b88f2387dd8e3e29a780a492d4ae6624a6ee0c61c0fba.json`,
approved by review artifact
`53fdb3bd2e15d3873e71dba51558d4c49b1be275b98021baea0993247a9f2ae4.review.json`.
Its preceding plan and rejected review were also inspected. This task has an
empty verification_script; the discovery verifier and sanitized fixtures belong
to T-US-007-002, working-memory consolidation to T-US-007-003, and production
recovery to US-008. Those tasks are not claimed complete here.

## Retained inputs and provenance

SHA-256 of the exact input files read:

| Input | SHA-256 |
| --- | --- |
| `/tmp/direct-request-planner-response.json` | `c02f1f38f893717faa92d34bdacc136c3c1a820ff8791c93e0c3dd2a79e1df0c` |
| `/tmp/direct-request-planner-failure.txt` | `7b6befdcc87629da7fd0b634b2bf8caab5e4ee06c9c6b59e6177db3f99fc0093` |
| `/tmp/reproduce-direct-request-plan.go` | `45d8d1c75a15b09697d1d104c4feee2e5e5d51c66c425fb3cb4480607663d4ed` |

The failure report's response equals the JSON input after trimming surrounding
whitespace. Its first line is `failed to parse LLM response as JSON: no stories found`.
Mappings are US-001=[], US-002=[REQ-001, REQ-002], US-004=[REQ-003], US-005=[].

The reproducer's original plan and review are outside this OpenExec candidate,
under the read-only inspected directory:

```
/mnt/data1/projects/agent-console/.openexec/outcome-candidates/b5b5611ab14fd20b34b069f97476a5e8/.openexec/artifacts/plans/
```

Their filenames equal their verified byte SHA-256 digests:

- Plan: `4f492fd1858a85b261fe0b73e54802aa857e7ca275f1405db196e5b286268dcd.json`.
- Review: `7a1f3c84afd3528b1ab087b769cce6fc52720c645aa60db64f3977edcb1481ca.review.json`.

The review's PlanDigest matches that plan. It rejects unnecessary serialization,
underspecified verification contracts and comma-separated requirement mapping;
it explicitly preserves terminal HITL task T-US-005-002. The plan has four
stories and one goal; the boundary depends on T-US-005-001 and carries the
owner's exact-candidate merge-decision reason. These artifacts were read, not
rewritten or imported.

Go resolves the import using the invocation working directory, not the directory
of the /tmp source file. In this candidate `go env GOMOD GOWORK` returns this
candidate's go.mod and an empty workspace. Running:

```
go list -f '{{.Dir}} {{.Module.Path}} {{.Module.Main}}' github.com/openexec/openexec/pkg/runtime
```

resolves its local pkg/runtime, module github.com/openexec/openexec, Main=true.
The facade aliases planner types and delegates GeneratePlan/RefinePlan directly
to internal/planner. In the retained Console candidate, the same import resolves
to `/home/perttu/go/pkg/mod/github.com/openexec/openexec@v0.13.2-0.20260930100332-798bc7b11666/pkg/runtime`,
Main=false. Its go.mod pins that version. This proves the historical reproducer
and current candidate exercise different implementations; neither proves a
deployed OpenExec version. The supplied Console serving observation is not
OpenExec deployment evidence.

## Executed offline reproduction

All Go commands used a Python subprocess with
`os.environ['GOCACHE']='/tmp/openexec-planner-discovery-cache'`, after the default
cache refused a write to the read-only home filesystem. No shell environment
prefix, repository module edit, external model or provider process was used.
The response provider's Complete method returns only the retained file bytes.

1. Run `go run /tmp/reproduce-direct-request-plan.go` from this candidate.
   Exit 1: `panic: expected original failure` (program exit status 2).
   The current decoder accepted the array; the historical assertion stopped
   execution before refinement and scalar controls. This is a stale reproducer
   expectation, not evidence that current generation fails.
2. Run that unchanged command from the retained Console candidate. Exit 0:

   ```text
   Pinned engine: failed to parse LLM response as JSON: no stories found
   Exact refinement replay: failed to parse LLM response as JSON: no stories found
   Underlying decoder: json: cannot unmarshal array into Go struct field Story.requirement_id of type string
   Diagnostic control (only requirement_id made scalar): 4 stories parsed
   Scalar refinement control: 4 stories, 1 goals retained
   ```

3. Create /tmp/reproduce-candidate-planner-inspection.go from the original:
   make its two retained artifact paths absolute using the directory above;
   replace each expected-failure panic with logging success or the first error
   line; replace `var strict []runtime.PlanStory` with
   ``var strict []struct { RequirementID string `json:"requirement_id"` }``.
   All provider calls and scalar-control transformations remain unchanged.
   Run `go run /tmp/reproduce-candidate-planner-inspection.go` from this candidate.
   Exit 0:

   ```text
   Candidate generation: accepted array mappings
   Candidate refinement: accepted array mappings
   Underlying decoder: json: cannot unmarshal array into Go struct field .requirement_id of type string
   Diagnostic control (only requirement_id made scalar): 4 stories parsed
   Scalar refinement control: 4 stories, 1 goals retained
   ```

The local shadow struct exposes the ordinary Go scalar-field diagnostic; it is
not the current public decoder. The scalar control joins array elements with
comma-space in memory solely to isolate the field mismatch. It does not establish
that coercion is an acceptable repair or persist changed mappings.

## Current source paths and repair constraints

| Concern | Current implementation and implications |
| --- | --- |
| Parsing | internal/planner/planner.go: Story.RequirementID is string, but Story.UnmarshalJSON accepts strings, null, absent fields and string arrays, joining arrays. This behavior arrived in baseline commit be19a5b6 (#73). parseResponse extracts an object/array envelope, tries both shapes, rejects partial unmarshal results, and reports the matching shape's decode error. Empty stories yield no stories found. The older masked diagnostic is already repaired, but scalar enforcement is not. |
| Prompts/correction | internal/planner/prompt.go: StoryGenerationPrompt and StoryFixPrompt show scalar examples; StoryFixPrompt requests a bare story array. GeneratePlan, GenerateCompactPlan and RefinePlan each make one Complete call. There is no schema-correction attempt, correction prompt or correction counter. RefinePlan already supplies original intent, serialized plan and rejected review and returns parse errors immediately. Reuse that route for the accepted repair. |
| Validation/boundaries | ProjectPlan.Validate defaults schema_version and checks nonempty goals/stories and story ID/title; it is not full task/schema validation. RefinePlan requires a rejected review and valid original, then parses, calls preserveHumanBoundaries and carryGoals. human_boundary.go restores retained HITL metadata/description/strategy and surviving dependency edges, refuses removed retained neighboring work and supports unambiguous renumbering. carryGoals retains cited goals. Manager validates refined output before persistence. |
| Independent review | review.go: ReviewPlan validates input and requires explicit approved and nonblank assessment, then applies verification, stale-base and human-boundary lint. Replay uses admitted PlanGenerator and PlanReviewer separately, refuses missing adapters/native fallback, clears Review on changed plans and calls the reviewer afresh. Adapter independence is a caller responsibility; these fields alone do not prove different model identities. |
| Callers | pkg/runtime/runtime.go is the direct public facade, with no manager persistence. pkg/manager/planner.go selects replayReviewedPlan only for a nonempty RequestID. pkg/api/handlers.go exposes Manager.Plan through POST /api/v1/runs:plan; internal/cli/plan.go and doctor.go also call Manager.Plan. The ordinary CLI plan call imports without setting Review or RequestID. The non-replay route has optional review but no iterative refinement. |
| Retained state | pkg/manager/planner_replay.go stores retainedPlanRequest in run_steps.metadata: RequestID, InputDigest, Result, ReviewRound, ReviewLimit. Run and step identities derive from the request ID; input identity covers intent and import/review/validation flags, with separate compact-mode hashing. Replay verifies run scope, receipt identity, plan bytes/hash/path and review bytes/path/verdict, under the execution lock. save uses a metadata/input-hash compare-and-swap. pkg/db/state/schema.go defines the existing runs/run_steps tables. |
| Budget gap | A new request starts ReviewRound=1 and retains MaxReviewCycles (default 3); a lower current limit can restrict it, a higher one cannot extend the retained nonzero limit. Legacy zero limits initialize on rejected review. The rejected round is archived before refinement, but ReviewRound increments only after successful refinement, validation and artifact write. Parse/provider/boundary failures return before that increment, so repeated failed attempts are not consumed. There is no persisted schema-correction budget or retained malformed-response record. Reuse existing replay metadata and persist consumption before dispatch for the accepted repair; do not treat review-round persistence as proof that failed attempts are bounded. |
| Atomic import | Replay validates remapped identities through release.ValidatePlanIdentities in a rollback-only transaction before rereview; unchanged refinements are refused. After fresh approval and stale-base checks, reviewedPlanRows carries execution metadata into release.ImportReviewedPlan. internal/release/reviewed_plan_import.go transactionally inserts goals/stories/tasks and a reviewed-plan-import receipt, checks existing content/modes/foreign goals, refuses receipt conflicts and avoids duplicate imports. Legacy importBoundPlan is a different incremental path and must not be described as that atomic replay import. |

Outcome: recover malformed refinements while retaining scalar schema, diagnostics,
budgets, human boundaries, fresh review and atomic import. The owning loop is
OpenExec planning/review, using existing Planner and Manager replay primitives;
no Console recovery controller or new task/authority model is needed.
Inspection adds no runtime concepts, persistent state, transitions, owner
decisions or failure modes and replaces no execution machinery.

## Verification and handoff

Host `run_declared_check(check="lint")`: exit 0 (Go vet and UI ESLint).
Its npm installation reported 14 dependency vulnerabilities; this is not an
audit repair task and lint did not fail.

Focused baseline command, exit 0:

```text
go test ./internal/planner ./pkg/runtime ./pkg/manager -run 'TestPlanner_ParseResponse|TestPlanner_GeneratePlan|Test.*HumanBoundar|Test.*Refine|TestReviewedPlan|TestCompactReviewedPlan' -count=1
ok github.com/openexec/openexec/internal/planner 0.006s
ok github.com/openexec/openexec/pkg/runtime 0.003s [no tests to run]
ok github.com/openexec/openexec/pkg/manager 3.276s
```

The runtime behavior was exercised by go run above, not by the empty runtime
test selection. Existing manager tests cover rejection/conflicts, transactional
rollback, exact review history, import idempotency and retained review limits.
Their fresh Manager instances reuse the same state handle; they are not proof
of database close/reopen durability. The later recovery tests must actually
close and reopen state, cover exhausted/failed attempts and re-read receipts.

No production code or tests changed here. The existing
TestPlanner_ParseResponse/Requirement_ID_As_List explicitly expects array
coercion; the scalar-schema repair must replace that expectation with rejection
and correction coverage, not retain it as the accepted contract. The cancelled
refinement restart test also currently allows another attempt after cancellation;
future accounting changes must explain their updated expected consumption.

Compatibility evaluation: documentation-only changes cannot alter .openexec,
.uaos or tasks.json loading/migration, schemas or execution. Full canonical
gates, recovery coverage and repair-disabled proof remain with their assigned
later tasks/Console runner. Discovery fixtures/verifier are now recorded in
[planner schema discovery](planner-schema-discovery.md). No publication, review
approval, merge or deployment is claimed.
