# Planner schema replay — US-008 / T-US-008-003

Outcome: exercise bounded correction through the public runtime and durable
manager route, including independent review and exactly one persisted import.

The existing native planner/refinement loop owns correction; reviewed-plan
receipts own accounting, review history and import replay. This task extends
tests and the existing verification dispatcher. Production code, persistence
schema, transitions and owner decisions are unchanged. Complexity delta: no
runtime abstractions added.

## Executable evidence

Run:

```sh
bash scripts/verify-planner-schema-recovery.sh --case replay
```

The strict verifier discovers each selected Go test exactly once, then requires
run/pass events for every declared parent and child and one package pass.
Missing, duplicate, unexpected, skipped, failed or wrong-package evidence is
refused. Five Python negative-control tests exercise those checks. A missing
subtest cannot be hidden by a passing parent or a successful Go exit code.

The replay gate passed with 8 runtime and 13 manager entries and no skips.
Providers are fixed, local Go implementations; no inference or live credentials
are used. These journeys invoke the candidate's exported runtime and Manager.Plan
directly and the real SQLite/artifact import path.

- Public runtime: generate initial plan, independently reject it, return an array
  where scalar requirement_id is required, request one correction with the exact
  decoder diagnostic, full rejected response, accepted plan, intent and findings,
  then validate and independently review the scalar correction.
- Manager: reserve correction before dispatch, retain the exact diagnostic and
  rejected response, approve independently, import, close and reopen SQLite,
  replay, and assert exactly one import, two review-history entries and unchanged
  in-progress task accounting.
- Refusals: malformed correction, interrupted/exhausted budget, removed boundary,
  invalid execution mode, absent or malformed approval and explicit rejection
  leave zero tasks and zero import receipts after reopen.
- Legacy HITL without a reason remains HITL with no invented reason. Fresh
  review refuses it; correction does not silently authorize automatic work.
- Restart with a larger configured review limit cannot refund the persisted
  budget. The remaining-budget case consumes its last refinement after reopen
  and refuses a second schema correction.
- Existing atomic rollback and missing-adapter/conflict journeys also run.

## Check receipts

Verified in this candidate on 2026-10-01:

- Declared host `lint`: exit 0 (Go vet and UI ESLint).
- Declared host `test`: exit 0 (all Go packages; 40 UI files, 635 tests).
  UI output includes existing React act/style warnings; no test failed.
- `--case recovery`: exit 0; complete existing changed-function scope remains
  222/239 statements covered (92.887029%). Go printed read-only module-cache
  stat warnings, but compilation, tests and coverage evaluation completed.
- Replay command above: exit 0, including all verifier negative controls.
- Shell syntax and `git diff --check`: exit 0.

## Changed-test rationale and compatibility

TestReviewedPlanSchemaCorrection now checks the exact diagnostic instead of a
substring, returns and round-trips an explicit scalar mapping, adds the missing
approval and boundary cases, and verifies retained task progress and review
history after import. No accepted behavior or regression assertion was weakened.

TestPlannerSchemaReplayJourney adds a complete exported-runtime journey.
Its removed-boundary fixture changes both identity and decision reason:
renaming a uniquely matched retained HITL task is intentionally supported, so a
rename alone would not represent a removed boundary.

This task changes no loader, migration or legacy receipt implementation.
Compatibility-sensitive HITL preservation and replay/import accounting are
exercised directly. Canonical gates, independent delivery review, publication
and merge remain Agent Console responsibilities. No deployment is claimed.
