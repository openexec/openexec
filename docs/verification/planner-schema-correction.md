# Planner scalar schema correction

Verified in the task candidate on 2026-10-01. The requested outcome is to survive
malformed refinement without coercing mappings or bypassing review. The observed
checkout had diagnostic preservation but also a custom Story decoder that joined
arrays into strings. The owning boundary is native OpenExec reviewed planning;
reuse RefinePlan, run_steps receipts, human-boundary checks and reviewed import.

Changes:

- Remove the coercing decoder. Bare story arrays and plan objects use the declared
  scalar field. ResponseDecodeError wraps the concrete JSON error and full response;
  empty plans remain distinct and no partial decode is returned.
- Generation, compact generation, refinement and review share an explicit scalar
  requirement rule. Reviewer prose cannot redefine that schema; preserve coverage,
  splitting stories where needed, rather than joining or dropping identifiers.
- RefinePlan permits one additional schema-correction completion containing the
  original intent, original plan, rejected reviewer evidence, exact diagnostic and
  entire malformed response. Corrected output still passes human-boundary checks.
- Durable reviewed planning reserves each refinement before dispatch, including
  failed/interrupted attempts. One correction per retained request is reserved by
  saving the diagnostic before dispatch. ReviewLimit remains retained; raising
  process configuration does not extend it. Completed legacy rounds initialize
  consumed attempts. Immutable review archives exclude changing dispatch accounting.
  A correction has no approval authority: a changed artifact needs fresh independent
  review before the existing transactional import.

## Executed evidence

Fresh verification for US-008 / T-US-008-001 on 2026-10-01:

- `run_declared_check(check="test")`: exit 0, full Go and UI tests. The manager
  package passed in 116.240 seconds. This resolves the previous attempt's
  90-second local timeout; no production failure was reproduced by the host check.
- `scripts/verify-planner-schema-recovery.sh --case discovery`: exit 0.
- `scripts/verify-planner-schema-recovery.sh --case recovery`: exit 0 after the
  final test changes. Exercises the public runtime and actual Manager.Plan route
  with controlled completion adapters and SQLite. Covers exact type/syntax errors,
  partial decode rejection, empty plans, failed correction admission, correction
  success/failure/cancellation, human-boundary removal, fresh independent review,
  consumed correction/refinement budgets after database close/reopen, and a single
  approved import with persisted task counts. No live model or deployment claimed.
- The added admission-refusal test initially used an invalid empty original plan;
  it was corrected to use a valid original story so it reaches the admission
  boundary. The final recovery verifier passed.
- Host lint was attempted but refused by the Console tool: `the stage's candidate
  is unavailable: origin does not point at GitHub; the Goal's repository has no
  verified GitHub origin`. This is a check-dispatch refusal, not a lint failure.
  `make lint compat-test type-check GOCACHE=/tmp/openexec-compact-go-cache`
  subsequently exited 0 locally: Go vet, UI ESLint, current `.openexec`, legacy
  `.uaos` and tasks.json fallback tests, Go build and UI TypeScript checks.

Updated existing tests deliberately replace array concatenation with scalar-schema
rejection (`TestPlanner_ParseResponse/Requirement_ID_As_List`) and replace a refunded
interrupted refinement with durable consumption
(`TestReviewedPlanRestartDoesNotRefundInterruptedRefinement`). Neither change
weakens the retained review or import requirements.

## Compatibility and complexity

No database migration, native plan schema change, project-loading change or new
Console recovery mechanism. Scalar, null and omitted requirement IDs retain their
previous canonical behavior. Array acceptance was explicitly removed because it
violates this request; legacy plan receipts written by the removed decoder already
serialize a scalar. Existing human-boundary, goal-carry and reviewed import remain.

Concepts added: none in scheduling/authority. Removed: permissive mapping coercion.
New persistent state: two optional fields on the existing planning receipt for
spent refinements and the correction diagnostic/response. New transitions: one
bounded completion inside refinement. New owner decisions: none. New failure modes:
exhausted correction is refused; interrupted dispatches consume their reservation.
Existing machinery replaced: only the custom decoder; review/import stay native.
No deployment, merged revision or successful retry of the parent Goal is claimed.
