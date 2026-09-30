# Bounded named-check variants — US-013 / T-US-013-002

This implementation stage extends verification of the existing native recapture
loop. The supplied task scope and Console delivery boundary govern this work;
the project context and Simple Loop architecture contract were read. No runtime,
loader, schema or migration behavior changes. Complexity delta: zero persistent
concepts, transitions, owner decisions or execution mechanisms added or replaced.

`TestNamedRecaptureVariants` exercises 48 queue journeys: lint/test ×
empty/matching phase × twelve outcomes. Each starts from a durable legacy
receipt, closes/reopens SQLite before execution, calls `ExecuteTasks`, then
makes three queue invocations, reopening after each one. Assertions re-read attempts, outcome, original
receipt, repair count, human boundaries and Settings dependency state.

- Success runs a real shell check, resumes A through the ordinary blueprint and
  completes Settings. Those three legitimate check executions stay at three
  across subsequent resumes. A retains three attempts, Settings one; no repair
  or obsolete failure binding survives.
- Registered usable evidence creates one repair without recapture. Historical
  command evidence without a usable failing exit selects the recorded shell
  command instead of the current named definition; the recapture produces one
  repair. Repeated resumes do not duplicate either repair.
- Classification-only failures use the existing maximum of three attempts.
  Starting with one, two or three attempts spent permits two, one or zero further
  dispatches. Exhaustion persists across every reload and resume.
- Multiple validated checks, conflicting phase, HITL, required review,
  cancellation and admission refusal retain terminal review boundaries and
  leave Settings pending. Cancellation/refusal spend the claimed attempt;
  ambiguity and human boundaries dispatch nothing.

The admitted test executor runs real shell commands through the public runtime
receipt/evidence APIs. Success fixtures complete ordinary stages without a
provider; repair fixtures stop at ordinary repair execution. These are native
queue integration journeys, not evidence of Console adapter deployment or a
completed external repair.

The resolver unit table now explicitly accepts lint/test with empty or matching
phases, including an unrelated task verification script. Empty command means
resolve the named check at admission. It is not itself a refusal. The previous
stage already changed the old authoritative-resolution fixture to accept this;
remaining `verify` fixtures with no script correctly stay unresolved.

## Verification

- `bash scripts/verify-verification-repair.sh --case recapture-variants`: exit 0,
  all 48 journeys passed. Its JSON verifier requires every expected completion
  exactly once and rejects failed, missing, skipped or duplicate journeys.
- `python3 -m unittest discover -s scripts/verification -p test_recapture_variants.py -v`:
  exit 0; two verifier tests cover complete evidence and six rejection variants.
- Host `run_declared_check(lint)`: exit 0, Go vet and UI ESLint passed.
- Host `run_declared_check(test)`: exit 0 before and after the changes. The final
  run passed all Go packages (manager: 104.736s), plus all 635 UI tests in 40 files.
  UI emitted existing non-failing React test warnings.
- `bash scripts/verify-verification-repair.sh --case legacy-incident`: exit 0;
  all four original incident journeys passed and both isolated resolver
  negative controls refused at unresolved/needs_review with zero executions.
- `go test ./pkg/manager ./internal/pipeline ./internal/release ./internal/validation -run 'Test(NamedRecapture|LegacyRecapture|Recapture)|Compatibility' -count=1 -timeout=60s`:
  exit 0 in all four packages, using `/tmp/openexec-retention-go-cache` via the
  subprocess environment. Includes the updated resolver unit table and
  protected-format compatibility journeys.
- `bash -n scripts/verify-verification-repair.sh` and `git diff --check`: exit 0.

The initial matrix run found only a fixture counting error: ordinary continuation
also invokes the selected check for A and Settings. The success expectation was
corrected to count those executions; all later resumes must keep that count fixed.
No production failure was reproduced by the declared host checks.

Compatibility evaluation: only tests, verification scripts and evidence change.
Existing `.openexec`, `.uaos` and tasks.json loading and migration code is untouched.
Console retains canonical gates, publication, review, merge decisions and delivery.
