# Plan identity delivery record — US-012 / T-US-012-001

## Retained outcome and provenance

Retained Goal G-007: preserve full-content plan identity across native and reviewed
imports, repair empty-list compatibility without weakening genuine conflict
refusal, and preserve completed work. This record prepares Console delivery;
it does not perform it. Scope is the supplied OpenExec candidate only.

Locally inspected on 2026-10-01:

- Branch: `outcome/6fbdb8c642f3b58c1788c74e84fa0860`.
- Inspected stage base: `81f168c50383bd5b4012e230c1bcfc4a794e7efd`.
- Pre-repair comparison baseline, verified present in Git:
  `38d20b4255926d311b295ff944b82e3673a89ac4`.
- Existing repair commits: `8715b6c5` (normalization), `3ff7c794`
  (coverage and BLOB handling), `81f168c5` (compatibility verification).
- This stage changes documentation/evidence only; production and tests are unchanged.

Supplied historical context, not a fresh remote observation:

- Candidate/feature: `6fbdb8c642f3b58c1788c74e84fa0860`.
- PR: https://github.com/openexec/openexec/pull/78; reviewer said it was open.
  Current remote PR state was not queried and is not inferred from that statement.
- Single advisory review: `f320866fc267dc32777060cc8c16c646`.
- Agent Console serving revision `fa197cf8`, started `2026-10-01T02:57:45Z`:
  supplied process observation, not OpenExec deployment or merge evidence.
- Earlier NOTES entries report Project Goal/Ready revision 4 and Interpretation 10;
  these are historical reads, not a new context/deployment attestation.

The outcome is unchanged exact replay with preserved retained rows. The owning
boundary is native preparation/import; existing Manager.Plan, identity comparisons,
SQLite transactions, snapshots and verification scripts supply the proof.
No new runtime abstraction, schema, loop or owner decision is introduced.

## Reproducible content binding

[plan-identity-content.sha256](plan-identity-content.sha256) binds 184 tracked files:
all source/tests/fixtures in internal/planner, internal/release and pkg/manager;
verification helpers and identity entry scripts; CLAUDE.md, Makefile and Go module
inputs. It includes the full repaired source/test content, not merely a patch.
Documentation-only changes here do not invalidate the executable-content binding.
The manifest itself has SHA-256:

`d93a6ad07acefc29b9aea442b73fc93af62497dcddd6445856a74a4af0299e6b`

From the repository root, reproduce with:

```sh
sha256sum docs/verification/plan-identity-content.sha256
sha256sum -c docs/verification/plan-identity-content.sha256
```

Each manifest line is SHA-256 of raw file bytes, two spaces, repository-relative
path and newline; paths are sorted lexicographically. The manifest digest hashes
those exact UTF-8 bytes. No temporary logs or another checkout are required.

## Single finding disposition

**ACCEPTED — MEDIUM empty-list replay incompatibility.** The existing repair is
supported by fresh passing public journeys and independently failing mutations.
Native writes normalize story acceptance/dependency lists and task dependencies;
Go comparison normalizes both sides; SQL predicates normalize SQL NULL and JSON
null, including native BLOB JSON. Nullable readers preserve reachable SQL NULL
rows without rewriting them. CLAUDE.md now describes full-content identity.

Two details refine the review rather than accepting it verbatim: native constructors
already copy DependsOn (omitted input makes it nil); and the proposed text-only
`column='null'` predicate misses native BLOB null. Current `json(column)='null'`
handling is necessary; its isolated negative control fails on the BLOB fixture.
The review's inference is now reproduced by the native writer+comparator mutation.
Formal remote review resolution remains Console-owned; this is its local disposition.

The tests cover native AutoImport repeated Plan calls, nil/empty acceptance lists,
task-only cascade protection, retained sibling/completed evidence, exact row
snapshots, store close/reopen, and unchanged identities. Legacy tests cross native
and reviewed routes with text/BLOB JSON null and schema-supported SQL NULL;
ValidatePlanIdentities and fresh-receipt import exercise the SQL predicate instead
of receipt short-circuiting. Validation/refusal preserve rows and receipts.
Reviewed same-title US-001/US-005 changed-content allocation, reference rewriting,
exact replay and post-review concurrent conflict atomic retry remain tested.
The list matrix checks 144 retained/incoming combinations over all four list
columns; scalar identity predicates also remain covered.

No tests changed in this stage. The repair dependency deliberately changed two
nil-versus-empty inequality expectations to equality, as required by the accepted
semantics; value, order, duplicate and genuine-conflict assertions remain strict.
Canonical raw-storage assertions prevent tolerant comparison from hiding bad writes.

## Executed D1 verification

Fresh commands executed in this worktree on 2026-10-01 (each exit 0):

| Command | Observed result |
| --- | --- |
| `bash scripts/verify-plan-identity-compat.sh targeted` | 22 mandatory tests passed; native/reviewed public paths, saved artifacts, reopened SQLite state, strict refusal and retry. |
| `bash scripts/verify-plan-identity-compat.sh coverage` | Fresh instrumented execution; all mandatory tests passed without skips; 14 coverage/discovery controls passed, plus three driver controls. |
| `bash scripts/verify-plan-identity-compat.sh mutations` | Nine independently compiled behavioral mutants failed with their exact expected assertions; positives passed and disposable copies were removed. |
| `bash scripts/verify-plan-identity-compat.sh compat-test` | Runs make compat-test; all three named compatibility tests passed for .openexec, legacy .uaos and tasks.json fallback. |
| `bash scripts/verify-plan-identity-compat.sh restored` | All 22 mandatory tests passed again after mutation cleanup. |

The Go module stat-cache emitted a read-only warning; command exit codes were 0.
No live provider or deployed OpenExec instance is claimed: deterministic adapters
exercise real public planning/import methods and persisted SQLite/artifact state.

Historical dependency results, not rerun or certified by this stage: the replaced
US-011 NOTES entry at the inspected stage base recorded
make lint and make type-check exit 0; make test was attempted but network access to
api.anthropic.com was refused. Full default compatibility driver and full repository
gate are not claimed passed. The available tool catalog contains no
run_declared_check, so no host lint/test receipt is fabricated. Canonical host checks
belong to the later socket-capable repository runner, outside this stage's scope.

Artifact verification: `sha256sum -c docs/verification/plan-identity-content.sha256`
verified all 184 entries (exit 0); `git diff --check && test -s NOTES.md` passed.
The saved record was re-read and checked against the fresh command outputs.

## Coverage inventory and measurement

The authoritative inventory is
[scripts/verification/plan-identity-discovery.json](../../scripts/verification/plan-identity-discovery.json).
It selects whole functions including closures, with changed-function completeness
checks against the pre-repair baseline and mandatory execution/no-skip checks.
Measured aggregate: **413/428 statements = 96.4953271028%**.
Every one of the 21 functions must independently exceed 90%; aggregate alone
cannot pass. Lowest function: 91.18%. No inventory was narrowed in this stage.

| Whole function | Covered/statements | Coverage |
| --- | --- | --- |
| `internal/planner/remap.go:RemapPlanIDs` | 15/15 | 100.00% |
| `internal/planner/remap.go:rewriteIDRefs` | 6/6 | 100.00% |
| `internal/planner/remap.go:nextFreeID` | 4/4 | 100.00% |
| `internal/planner/remap.go:nextFreeTaskID` | 8/8 | 100.00% |
| `internal/planner/remap_content.go:remapContentIDs` | 59/59 | 100.00% |
| `internal/planner/remap_content.go:rewritePlanRefs` | 31/31 | 100.00% |
| `internal/release/reviewed_identity.go:ReviewedGoalEqual` | 1/1 | 100.00% |
| `internal/release/reviewed_identity.go:reviewedArrayEqual` | 5/5 | 100.00% |
| `internal/release/reviewed_identity.go:ReviewedStoryEqual` | 1/1 | 100.00% |
| `internal/release/reviewed_identity.go:reviewedMode` | 3/3 | 100.00% |
| `internal/release/reviewed_identity.go:ReviewedTaskEqual` | 1/1 | 100.00% |
| `pkg/manager/planner.go:(*Manager).preparePlanIDs` | 18/19 | 94.74% |
| `pkg/manager/planner_identity.go:reviewedIdentityConflicts` | 12/12 | 100.00% |
| `pkg/manager/planner_identity.go:uniquePlanIDs` | 15/15 | 100.00% |
| `pkg/manager/planner_replay.go:reviewedPlanRows` | 17/17 | 100.00% |
| `internal/release/reviewed_plan_import.go:(*Manager).ImportReviewedPlan` | 1/1 | 100.00% |
| `internal/release/reviewed_plan_import.go:(*Manager).ValidatePlanIdentities` | 1/1 | 100.00% |
| `internal/release/reviewed_plan_import.go:(*Manager).importReviewedPlan` | 100/106 | 94.34% |
| `pkg/manager/planner.go:(*Manager).importBoundPlan` | 49/53 | 92.45% |
| `internal/release/sqlite_store.go:(*SQLiteStore).getStoryInternal` | 35/36 | 97.22% |
| `internal/release/sqlite_store.go:(*SQLiteStore).getTaskInternal` | 31/34 | 91.18% |

## Negative-control outcomes

All nine controls compiled successfully, then exited 1 at the intended behavioral
assertion. Compilation errors, missing/skipped tests, unrelated assertions and
unexpected success do not count; driver controls enforce this distinction.

- story SQL NULL reader compiled; expected assertion: converting NULL to string is unsupported
- task SQL NULL reader compiled; expected assertion: converting NULL to string is unsupported
- content allocation compiled; expected assertion: reviewed stories US-001 conflicts with retained content
- atomic conflict refusal compiled; expected assertion: genuine conflict not refused
- native replay compiled; expected assertion: identical native replay moved US-001
- native JSON writer compiled; expected assertion: got null want []
- legacy comparator compiled; expected assertion: exact legacy replay changed plan identities/content
- legacy BLOB predicate compiled; expected assertion: reviewed stories US-001 conflicts with retained content
- SQL predicate compiled; expected assertion: reviewed stories US-001 conflicts with retained content

The candidate is never mutated: each control uses a disposable repository copy,
restores original sources between mutations and checks cleanup. Source manifest
verification and the restored public journeys bind the final unchanged content.

## Completion boundary and external follow-up

D2 status: **pending**. No current candidate-matched coordinator evidence of merge
to OpenExec's default branch is supplied or established here. This is absence of
delivery proof, not a fresh claim that the historical PR is still open.

G-007 completion requires BOTH verified D1 repair evidence AND current coordinator
evidence satisfying D2. Native task completion, this local record, a local commit,
owner acceptance, historical review prose or a Console serving revision cannot
substitute for D2. Console must link this verified content to the exact published
candidate/PR, canonical gate, review disposition, owner's standing promotion
controls and default-branch merge revision/ancestry. Until then G-007 is not complete.

Agent Console's goal-mode verifier mismatch is a separate external follow-up:
Console must verify its Goal-mode acceptance projection against native task
verification in its own repository and record its regression/delivery evidence.
That checkout was neither inspected nor changed; the mismatch is supplied context,
not independently reproduced or claimed repaired here.

The current stage wrapper explicitly requires ordinary Git persistence, superseding
the embedded older no-commit instruction for this local artifact. Publication,
delivery tools, formal remote resolution and merge remain outside this stage.
