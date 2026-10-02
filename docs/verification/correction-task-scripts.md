# Required task verification during correction

US-011 / T-US-011-002 extends the existing native correction path. It does
not introduce a task state, execution loop, grant, schema or owner decision.

A nonblank task `VerificationScript` is a required `verify` check. Correction
and historical recapture share command resolution; only recapture supplies
historical receipt artifacts. Task verification uses the current script, never
an unrelated command from the original failure receipt. The existing pipeline
and configured executor retain cancellation, Stop and effect enforcement.

Current accepted-plan required/blocking items remain mandatory. Supported
plan checks are named lint/test checks and exactly three-element `sh -c` or
`/bin/sh -c` argv. Shell bodies retain their bytes, quoting and operators.
Unsupported required argv is refused visibly, including extra shell arguments;
it is never silently shortened. Identical checks share one execution and each
obligation retains a separate proof. Optional/suggested items are not promoted
to requirements.

No-plan tasks omit both PlanID and StateHash. Omitting an existing accepted
plan is refused, including at transactional completion. Task, retained receipt,
Git candidate, branch, explicit decision and one-use allowance bindings remain
required. Plans with no mandatory items follow native CanCompleteTask rules;
no artificial validation obligation is added.

Task verification persists a correction-decision-tagged run step with the
exact script and candidate digest, including failed verification. Completion
requires a completed task-script proof independently of validation-item links.
Changing or removing a script after its proof is recorded is refused. Every
applicable plan obligation still needs fresh evidence and native completion
claims. Ordinary attempts, original receipts and completed repairs are retained.

## Verification

Verified in this candidate on 2026-10-01:

- Task verification script: exit 0; correction/store tests, six compiled
  negative controls and the restored-source native journeys passed.
- Host `run_declared_check(test)`: exit 0; Go suite and 635 UI tests passed.
- Host `run_declared_check(lint)`: exit 0; Go vet and UI ESLint passed.
- `make compat-test type-check`: exit 0; current `.openexec`, legacy `.uaos`,
  legacy config/tasks fallbacks, Go build and TypeScript checks passed.
- Shell syntax and `git diff --check`: exit 0.

The initial host test run exited 2 on an intermediate implementation that
passed no receipt into historical recapture's identity validator. Factoring
its existing command resolver into a shared helper preserved historical
receipt validation while allowing fresh task-script resolution. The final
host run verifies that repair. The planner-script import assertion was added
while the host suite was running and subsequently passed in the task script's
restored-source rerun. No production behavior changed after the final host run
was started; only a comment, test assertion and documentation followed.


The executable acceptance entry point is
`scripts/verify-exhausted-task-correction.sh`. It runs the correction/store
suite, including real local commands, native queue execution and SQLite close /
reopen journeys. New TestCorrectionTaskScriptJourneys covers accepted-plan,
planner-imported and legacy no-plan tasks: success, absent file, exit 3,
script/plan disagreement, quoted shell bodies, unsupported argv, absent authority
and effect refusal. The fixture checks the planner-imported script before
lifecycle updates. It counts exact script invocations, verifies the persisted
script proof, dependent execution/blocking, terminal outcomes and no replay.

Existing exhaustion and execution-control journeys cover unchanged failures,
completed repairs, attempt preservation, independent draining, Stop,
cancellation, candidate drift and unfulfilled validation. The execution-control
matrix now also removes the task script during execution. Direct store testing
rejects no-plan completion without script evidence. No-check journeys verify
native completion for both absent and optional-only plans.

TestCorrectionPlanRefusalCoverage changes its empty/optional-plan expectations:
native completion permits these plans, while task scripts remain required.
Existing check-count assertions remain intact because duplicate commands execute
once. Shared fixtures gain planner-import and command-observation support;
no failure assertion or authority requirement was weakened.

Compiled source overlays remove task-script execution and its transactional
completion guard separately. Another restores actual native repair creation
for an exhausted original task and requires the resulting repair-attempt-limit
refusal to fail the journey. Existing refusal/boundary controls remain in the
same script. The script reruns unmodified journeys after the overlays.

Compatibility: no project loader, migration, fallback, schema or import contract
changes. Existing projects retain native scheduling and recapture behavior;
legacy no-plan tasks gain the same authorized correction verification path.

This stage prepares code and evidence only. Console publication, canonical
gates, independent review and the owner's merge decision remain downstream.
