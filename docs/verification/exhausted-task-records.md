# Runnable exhausted-task records — US-010 / T-US-010-003

Run `python3 scripts/verify-exhausted-task-records.py --self-test` from this
repository. Default `--phase records` validates definitions and the source
coverage denominator; it does **not** certify D1, completed repairs, review
resolution or D2. `--list-commands` emits argv for every required exact case.
These are runnable Go invocations, but future test names remain obligations:
zero matching tests is not evidence. The native evidence phase requires the
exact run and pass events, including package passes.

The [records](../../scripts/verification/exhausted-task-records.json) reference
rather than copy the accepted [case matrix](../../scripts/verification/exhausted-task-review-cases.json)
and [coverage scope](../../scripts/exhausted-task-coverage-scope.json).
Each finding has source declarations for Root cause/Fix and complete Prove/Falsify
case references. Case setup, assertions, qualifications and removal controls live
only in the matrix. The verifier also includes its baseline/control cases,
validates upstream discovery dependencies, rejects incomplete definitions and
checks executable/package/selector shape. It never executes a command taken
from an evidence record. All input records, source references and artifacts
must resolve inside this repository, including through symlinks. Duplicate JSON
keys, path traversal, missing declarations and narrowed coverage policies refuse.

## Preparation and delivery are separate checks

`--phase preparation --native-evidence <local-report.json>` requires a fresh
native evidence package. Store its report and artifacts under
`docs/verification/exhausted-task-records-evidence/`; that directory alone is
excluded from the candidate source fingerprint. Files, modes, deletions and
untracked source elsewhere remain bound. The report schema is checked strictly
by `validate_native` in the verifier module:

- `schema_version: 1`, exact `candidate_revision` and `source_sha256`.
- `events` and `profile`: `{path, sha256}` references to native Go JSON events
  and count coverage. Every exact mapped case must run/pass; no fail/skip events.
  The denominator is the AST whole-function union from the accepted inventory,
  correction files and all changed/new production functions. Source-instrumented
  blocks absent from the profile remain uncovered. Coverage must exceed 90%.
- `falsifiers`: one entry per matrix case, containing its exact `mutation`
  description, an `assertion` selected from that case's required assertions,
  `{path, sha256}` `events`, and `source_restored_sha256`. The mutant must run and
  fail that exact case with that assertion in its output. Compilation failures,
  absent cases, skips, unrelated failures and unrestored source do not count.
- `checks`: `lint` and `test`, each with `command: ["run_declared_check", name]`,
  `exit_code: 0`, and a hashed `evidence` reference to the actual host response.
- `scratch`: the built operator CLI argv in `command`, successful `exit_code`
  and hashed JSON `evidence`. Observations include exact supplied/persisted
  `decision_ref`/`persisted_decision_ref`, feature `candidate_branch`,
  `candidate_sha256`, reopened `A` (`done`, attempts/max both 3), reopened
  `Settings` (`done`), and matching original/reopened receipt SHA256 values.

These artifacts are observations from actual runs, not permission to synthesize
successful reports. Self-tests use clearly synthetic fixture packages in temporary
Git repositories; none are recorded as this candidate's repair evidence.
Preparation can pass with D2 outstanding and `goal_complete: false`.

`--phase delivery` additionally requires `--merge-evidence <local-export.json>`.
This argument is a trusted Agent Console export, supplied after its existing
owner decision and promotion controls. A task-authored JSON claiming to be the
coordinator is not an authenticated receipt. The verifier is a read-only evidence
consumer, not an issuer, authentication service or replacement effect gate.
It reuses the existing `delivery.check_merge` receipt contract and additionally
requires issuer, feature and PR identity; a current `observed_at`; the OpenExec
origin and its symbolic default branch; actual local merge/default-branch Git
objects; and candidate integration by ancestry or exact tree equivalence for
squash/rebase (`integration: ancestry|tree`). A coordinator must refresh its
observation and materialize the Git objects before invoking this phase; the
verifier never fetches or merges. Conservative refusal of non-identical rebases
requires fresh verification, not a fabricated equivalence flag.

A local commit, published PR, green tests, owner decision, or Console serving
revision alone cannot satisfy D2. Missing evidence does not prove non-delivery.
The selected task creates no delivery/merge tasks, transport or owner decision.
Publication, canonical gates, review resolution and promotion remain with Console.

## Planning contract repair and regression

Before implementing this verifier, the planning prompts were corrected: their
unqualified Goal-verification requirement conflicted with delivery after queue
completion. `DeliveryBoundaryRule` now applies to generation, compact planning,
review and refinement. The existing story `contract` and acceptance criteria
retain the full Goal and outstanding D2, while native verification checks local
preparation. Compact output now exposes `contract` explicitly. Existing HITL
acceptance identity and owner controls remain intact; no automatic acceptance.

The native queue already returns task-scope completion without a Goal verdict;
it needs no runtime/schema change. The new regression converts a reviewed plan
using `reviewedPlanRows`, runs its actual native pipelines, runs the completed
queue again, closes/reopens SQLite and checks completed tasks plus unchanged
outstanding D2. Planning tests exercise generation, serialization, refinement
and independent approval/rejection with a controlled provider.

The self-test runs these Go regressions and three source-overlay controls:
remove shared delivery guidance, make queue completion wait for merge, and
replace the persisted outstanding contract with a merge claim. Each must compile
and fail its intended assertion; original bytes are checked and positives rerun.
These controls prove this stage's contract, not the four future runtime repairs.

Review dispositions remain the qualified provisional assessments in the case
matrix. In particular, the missing shipped OpenExec authority caller is a local
finding, not proof of missing code/deployment in another repository. Vacuous
no-plan completion is not accepted; nor is an arbitrary different valid decision
reference inherently invalid. This stage does not resolve any finding as repaired.

Complexity delta: one shared prompt rule, one composed records/evidence verifier
and tests; zero new runtime concepts, persistent execution state, transitions,
owner decisions or implementation loops. No existing machinery replaced. Existing
`.openexec`/`.uaos` loading and migration remain unchanged; the persisted plan
schema is reused. Fresh verification results are recorded once in NOTES.md.
