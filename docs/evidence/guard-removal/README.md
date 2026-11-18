# Guard-removal controls for overlong `real_example` references

Run on the tree of main 8d370908 (PR 224 merged; its original head was 0f82a32e).
Each mutation was applied to production code, the tests run, the raw output
saved unedited (`N-*.txt`), the exact mutation saved (`N-*.mutation.diff`), and
the mutation reverted with `git checkout`. No production file differs from main.
All three mutated trees compiled; every failure below is a test assertion.

| # | Mutation | Command | Failing assertions (from the raw output) |
|---|----------|---------|------------------------------------------|
| 1 | `pkg/manager/planner_replay.go`: repair routing `RepairablePlanIssues` replaced by `VerificationPlanIssues` | `go test ./pkg/manager/ -run 'ReferenceLength\|OverlongRealExample' -count=1` | `TestOverlongRealExampleIsRepairedOnceWithTheBound`: "overlong reference was not repaired: the plan fixed after its review still fails required checks…"; `TestUnrepairedOverlongRealExampleStopsAfterBoundedRepairs` fails likewise |
| 2 | `internal/planner/review.go`: the bound/repair paragraph in `CheckRepairInstruction` blanked (`reference = ""`) | same | `TestOverlongRealExampleIsRepairedOnceWithTheBound`: `repair instruction lacks "change only that story's real_example"` |
| 3 | `internal/planner/outcome_shape.go`: length/line-break lint case disabled (`case false && …`) | `go test ./internal/planner/ ./pkg/manager/ -run 'ReferenceLength\|OverlongRealExample' -count=1` | `TestReferenceLengthIssueNamesTheBoundAndTheFix`: `issues = []` (overlong reference accepted); `TestOverlongRealExampleIsRepairedOnceWithTheBound`: `generated=2` (no repair requested); `TestUnrepairedOverlongRealExampleStopsAfterBoundedRepairs`: error `<nil>` |

Unmutated, the same command passes in both packages.
