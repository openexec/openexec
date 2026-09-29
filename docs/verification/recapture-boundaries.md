# Recapture boundary evidence — US-009 / T-US-009-002

Dedicated journeys in `pkg/manager/recapture_boundaries_test.go` exercise the
public `ExecuteTasks` entry using T-US-009-001's reusable admitted executor and
SQLite restart fixture. Failure and success execute real local shell checks;
refusal and cancellation are injected at the admitted executor boundary. This
is native-loop integration evidence, not proof of deployed service behavior.

Run `bash scripts/verification/recapture-boundaries.sh` from any directory.
Optional `--output PATH` requires a new directory, refusing reuse of old proof.
The script emits `tests.jsonl` and `result.json`: exact passing scenario names
and task/Settings snapshots read after database close/reopen. Missing, skipped,
duplicate or unexpected scenario completions and missing/duplicate state
checkpoints fail verification. No coverage result or compatibility report is
used as boundary evidence. Shared dispatcher composition remains T-US-009-005.

## Verified on 2026-09-29

The standalone script passed nine named completions (eight leaf scenarios)
and 22 persisted checkpoints. Fresh artifacts:
`/tmp/openexec-recapture-boundaries-8rqa4gev/`.

| Scenario | Persisted A attempts | Verification dispatches | Repairs | Persisted boundary |
| --- | ---: | ---: | ---: | --- |
| Failed recapture with diagnostics | 2 | 1 | 1 | A pending; Settings pending |
| Exhaustion | 3 | 2 | 0 | needs_review / exhausted |
| Restart with one remaining attempt | 3 | 1 | 0 | needs_review / exhausted |
| Restart with all attempts spent | 3 | 0 | 0 | needs_review / exhausted |
| Unresolved command identity | 1 | 0 | 0 | needs_review / unresolved |
| Admitted launch refusal | 2 | 1 | 0 | needs_review / refused |
| Cancellation | 2 | 1 | 0 | needs_review / cancelled |
| Successful recapture plus completion obligation | 3 | 1 | 0 | Completion refused until supported claim; then both done |

Every terminal scenario checks its initial explicit error reason, then starts
the native queue twice more across database reopens: no new dispatch, attempt
refund, repair or Settings attempt is permitted. Interrupted attempts are seeded
as durable in-progress recaptures, exercising actual restart reconciliation.

The failure journey checks fresh receipt binding, readable private stderr,
exact command/cwd/exit, and one repair referencing the fresh evidence after
reopen. A second queue invocation cannot create another repair or recapture.

The success journey starts with a persisted accepted blocking validation item.
The real recheck passes and the ordinary task stages execute, but the existing
completion gate refuses A because its required supported claim is absent.
After reopen, success and receipt removal persist, A is not done, and Settings
has zero attempts. Inserting the supported claim permits the existing release
completion API to complete A; reopening and running the native queue then
executes Settings. This verifies both sides of the existing dependency gate.

Validation commands:

```sh
bash scripts/verification/recapture-boundaries.sh
export GOCACHE=/tmp/openexec-retention-go-cache
go test ./pkg/manager -run '^Test(LegacyRecapture|RecaptureBoundaries)' -count=1 -timeout=60s
python3 -m unittest discover -s scripts/verification -p 'test_*.py' -q
bash -n scripts/verification/recapture-boundaries.sh
git diff --check
```

Compatibility evaluation: this task changes only tests, a dedicated verifier,
and evidence documentation; production loading, migrations, legacy project
formats and recapture behavior remain unchanged. The implementation-owned
recapture journeys also passed, along with all 31 Python verifier tests
(including seven dedicated boundary-verifier controls), shell syntax and diff
checks. Canonical full gates remain with the repository
runner; publication, independent review and deployment are outside this stage.
Complexity delta: no production concepts, persistent state, transitions, owner
decisions or replacement machinery added.
