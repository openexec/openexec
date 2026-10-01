# Empty-list identity discovery — US-010 / T-US-010-002

This is the discovery contract for the retained candidate, not evidence that the
empty-list repair or D2 delivery has finished. Reuse the native Manager.Plan loop,
release manager, SQLite transactions, reviewed artifacts and receipts. No new
runtime abstraction, schema or authority boundary is necessary (complexity delta
zero). Project context read on 2026-10-01: accepted Goal/Ready revision 4 and
Interpretation revision 10; the supplied narrower identity Goal bounds this task.

## Fixtures and public boundaries

`pkg/manager/testdata/reviewed-identity/retained.json` and
`pkg/manager/testdata/reviewed-identity/subsequent.json` are
repository-local reproductions, not an owner's database. Preserve literal
US-001, US-005 and T-US-001-001; they are incident identities, independent of the
current planning story number. Both retained stories and their first tasks omit
depends_on already. Clone the decoded fixture in each case; clear US-001's
acceptance_criteria for the empty-acceptance variant. Do not edit the shared
fixture to erase changed-content coverage.

Reuse identityFixture and identitySnapshot in
`pkg/manager/planner_identity_lifecycle_test.go`, newSchedulerTestEnv in
`pkg/manager/scheduler_test.go`, and freshQueueManager in
`pkg/manager/task_restart_boundary_test.go`. The generator/reviewer are deterministic
adapters; the public Manager.Plan and release ImportReviewedPlan /
ValidatePlanIdentities boundaries exercise the actual SQLite persistence path.
An empty RequestID, Review=false, AutoImport=true is essential for native Plan;
a nonempty RequestID selects durable reviewed replay even when Review=false.

## Native identical replay

Proposed TestNativeIdenticalReimportPreservesIdentities: call Manager.Plan twice
with the exact retained fixture and the native request above. Assert US-001,
US-005 and T-US-001-001 remain unchanged and all persisted goals/stories/tasks
snapshots match. Include nil acceptance criteria and explicit empty arrays,
plus nonempty lists to prove normalization preserves values and ordering.
Read raw SQLite columns after the first import: empty story acceptance_criteria
and depends_on, and task depends_on must equal JSON `[]`, not `null`. Reopen the
manager and database before a third import and reassert IDs and snapshots.
Native import must create no reviewed-plan-import receipt. Check task-only empty
dependency separately with all story lists nonempty/canonical: one task's false
conflict must not cascade into a moved parent and siblings.

## Legacy JSON null

Seed via native Plan or reviewed import; use bound SQL updates to set individually
stories.acceptance_criteria, stories.depends_on, tasks.depends_on to JSON `null`
only where equivalent incoming lists are empty. For stories.tasks use a separate
empty-story variant with no tasks; never erase genuine ordered membership.
Snapshot after fixture mutation, close/reopen, then replay both native and reviewed
routes without moving IDs. For reviewed Manager.Plan use a fresh RequestID, then
call ValidatePlanIdentities and ImportReviewedPlan with a fresh receipt identity
on the exact reviewedPlanRows output. This bypasses the old receipt fast path and
proves the existing-row SQL predicates run. Validation is rollback-only; import
adds exactly one bound receipt while every retained row stays byte-identical.

## Schema-supported SQL NULL

`internal/release/schema.go` declares the four list columns nullable with default
`[]`; confirm using PRAGMA table_info on the test database before bound updates
set each to SQL NULL. Use the same equivalent-empty cases as JSON null, including
the empty-story tasks variant. Assert raw SQL IS NULL, then ValidatePlanIdentities,
new-receipt import, real close/reopen and public Plan reuse. Do not rebuild tables
or relax schema constraints to manufacture reachability. The repaired SQLite readers
scan these columns into sql.NullString. Verification covers native storage
loading as well as SQL predicate normalization;
a SQL-only comparison test cannot establish reopen support. Preserve stored NULL
bytes; normalization at read/compare time must not silently migrate retained rows.

## Changed-content allocation

Reuse TestReviewedIdentityLifecycle and TestReviewedIdentityChangedGoalAndTask.
The subsequent fixture keeps titles while changing US-001/US-005 content, parent
and membership. Require fresh IDs, consistent goal/story/task references and prose,
retained completed rows unchanged, artifact JSON matching PlanResult, same-request
replay without regeneration/review and distinct-request exact-content reuse.
Nonempty list order, duplicates, mode, strategy-expanded description, priority,
max attempts, contract and verification remain meaningful content. Keep existing
field-by-field comparator tests. Update only obsolete nil-versus-empty inequality
assertions when the repair changes that accepted semantic.

## Concurrent conflict refusal

Reuse TestReviewedIdentityPostReviewConflictAtomicRetry: mutate G-002's description
inside the reviewer after allocation and before import. Require the exact genuine
content refusal, no partial stories/tasks or new receipt, and preservation of the
concurrent writer's goal value. Restore that value, close/reopen and retry with
adapters that fail if invoked: persisted reviewed content must import without
regeneration or rereview. Also preserve receipt conflicts for changed run ID,
digest or receipt bytes and verify no durable writes after each refused call.

## Persisted snapshots and receipts

Use identitySnapshot with `SELECT * FROM goals ORDER BY id`, and the equivalent
queries for stories and tasks, including all lifecycle/evidence/timestamp columns.
Snapshot reviewed import receipts separately with agent='reviewed-plan-import';
other planning run_steps may legitimately grow for a distinct request. Re-read
artifact bytes and hash, close Manager and state DB, create freshQueueManager and
repeat SQL snapshots. Cache refresh alone is not reopen evidence. Receipt checks
bind id, run_id, inputs_hash, metadata, phase, agent and status: same request adds
nothing, a new reviewed import adds one receipt, validation and refusal add none.

## Mutation expectations

1. Remove native nil-to-empty writes AND old-side Go normalization: native exact
   replay must fail on moved US-001/added rows. Independently removing writer-only
   normalization must fail the raw `[]` assertion even if the comparator hides it.
2. Remove only SQL CASE normalization: legacy-null validation/new-receipt import
   must fail with `reviewed stories US-001 conflicts with retained content`.
   Include task and empty-story membership variants to cover every list predicate.
3. Remove SQL NULL reader handling: schema-supported reopen must fail; do not
   accept a skipped setup or a fixture that converted NULL back to JSON first.
4. Remove full-content conflict allocation: existing changed-content lifecycle
   must fail. Remove atomic retained-content refusal: concurrent-conflict test
   must fail. Keep these separate from empty-value compatibility controls.

Run these controls with `scripts/verify-plan-identity-unit-coverage.sh --mutations`.
The runner copies repository sources into a disposable directory, requires a
passing unmodified test, applies each exact mutation, separately requires
successful compilation, then requires exit 1 and the named behavioral assertion
in that test's Go JSON events. Sources are restored between controls. Build
errors, panics, missing execution and unrelated test failures cannot qualify.
Additional controls independently remove old-side comparison and revert the SQL
predicate to text-only null matching. The latter catches legacy native BLOB
`null`, which a direct column='null' comparison misses. Coverage and compatibility
results are recorded once in NOTES.md; discovery alone claims no runtime result.

## Coverage inventory

`scripts/verification/plan-identity-discovery.json` is the machine-readable
function/source-block inventory. It includes every retained identity function,
native importBoundPlan storage normalization, SQL insert-predicate construction
and array serialization closures, and SQL NULL story/task readers. Cover complete
functions, including closures, not just edited lines. Every inventoried function
must have statement coverage strictly greater than 90% (90.00% fails), with all
instrumented blocks present and no skipped mandatory tests. Reuse
retentioncoverage plus retention_unit_coverage.expected_blocks/evaluate and the
baseline diff completeness check in reviewed_plan_identity.py. Any newly changed
helper must join the inventory; do not narrow it to make coverage green. Named
source blocks are additional audit anchors within their whole-function scope.
The discovery script validates inventory existence/completeness.
`scripts/verify-plan-identity-unit-coverage.sh` measures this inventory with fresh
Go coverage profiles and requires every named test to pass without skips. It
retains public Plan/reopen/receipt journeys and adds list and SQL scalar matrices.
Its fail-closed controls reject missing functions, source anchors, coverage
blocks, zero executed tests, omitted changed logic, and coverage at or below the
threshold for any function. Native raw JSON assertions cover nil and empty
inputs independently of comparator tolerance. SQL list matrices include native
BLOB and text JSON, SQL NULL only in nullable columns, equal nonempty lists,
different values, order and duplicates; refused validation is re-read to prove
the retained value was not rewritten.

## Console follow-up and delivery boundary

Agent Console owns the goal-mode verifier mismatch in its own repository. It must
verify its Goal-mode acceptance projection against native task verification and
record its own regression/delivery evidence. This follow-up is reported, not
independently reproduced here; no other checkout was accessed. The supplied
Console process revision fa197cf8 is not proof of OpenExec deployment. D2 requires
Console's later exact-candidate publication, canonical gate and owner-controlled
merge evidence. Local task completion does not establish merge or delivery.

## Verification

Run `bash scripts/verify-plan-identity-discovery.sh`. It validates local fixture
semantics, source declarations/blocks, required case evidence and coverage scope;
it also exercises rejection of missing fixtures, substituted identities, omitted
native/SQL coverage, weakened coverage thresholds and missing case evidence.
The host run_declared_check tool is not exposed in this session's tool catalog;
no host lint/test result is claimed. Full repository gates remain Console-owned.
