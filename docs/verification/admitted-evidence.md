# Public silent verification tracer — US-012 / T-US-012-001

The runtime now exports `VerificationCommandFailureWithEvidence(ctx, name,
err, hash, path)`, delegating to the existing typed gates failure. Adapters first
call `RetainCommandEvidence`, then attach its returned reference with this API.
Evidence does not change the failure-classification digest or grant repair
authority to cancellation, launch/transport refusal, reserved exits or mixed
errors. Existing `VerificationCommandFailure` callers remain compatible.

The public-runtime-only executor in
`internal/testutil/admittedevidence/executor.go` runs `/bin/sh -c 'exit 2'` with
an additional argv containing whitespace, at the candidate WorkDir. It captures
separate empty streams and supplies no result artifacts, output or diagnostic
header. Both lint and test paths are exercised. Pipeline assertions require the
64-hex reference on `EventBlueprintFailed`; manager assertions require the
registered artifact and failed run step after closing and reopening SQLite,
exact argv/cwd/exit retrieval, `diagnosticFreeReceipt == false`, and a reread
repair description containing the run-step identity, private reference/path and
instructions to reproduce and diagnose the check. Each fixture executes once.

Both engine APIs already retained populated failed results in this candidate;
no additional engine change was necessary. Existing populated/nil result/error
matrix and persisted-repair journeys were rerun. The tracer verifier executes
independent source copies for each engine discard mutation and the wrapper
attachment mutation. It checks completed test identities and assertion failures;
a compile error, skipped test, or unrelated failure cannot satisfy a control.
The wrapper mutant cannot recover through `StageResult.Artifacts` because the
silent fixture deliberately never supplies that fallback.

## Verification

Executed 2026-09-29 in this candidate:

- `bash scripts/verify-verification-repair.sh --case admitted-tracer`: passed.
  Positive public journeys and existing engine journeys passed. Each isolated
  negative control failed at its required assertions: ExecuteStage lost the
  result/error pair and persisted evidence; Execute lost failed history and
  persisted evidence; wrapper removal lost terminal attachment and made the
  reloaded silent receipt diagnostic-free. The command prints the structured
  results and replaces no source files.
- `go test ./pkg/runtime ./internal/execution/gates ./internal/blueprint
  ./internal/pipeline ./pkg/manager -run
  'Test(Public|RetainedResult|Retention|VerificationFailureEvidence|TerminalVerification|ConfiguredVerification)'
  -count=1 -timeout=60s`: all five packages passed, using
  `GOCACHE=/tmp/openexec-retention-go-cache` set in the subprocess environment.
  An initial default-cache invocation could not write the read-only home cache;
  the writable-cache rerun completed successfully.
- `python3 -m unittest discover -s scripts/verification -p 'test_*.py' -q`:
  74 passed.
- Shell syntax and `git diff --check`: passed.

## Scope and review disposition

F2's missing public typed-error attachment is accepted and repaired by this
tracer. The absolute claim of no public attachment path is rejected: existing
`StageResult.Artifacts` references already reach the terminal event independently
of classification. This tracer proves the new typed-error path without that
fallback. It does not attest actual Console adapter adoption.

This is only T-US-012-001, not completion of US-012 or the advisory review.
Diagnostic-tail handling, actual Console adapter/dependency execution and the
full-body story coverage proof remain assigned to the other retained US-012
tasks. F1 legacy recapture remains US-013; F3 ignored private evidence storage
remains US-014; aggregate evidence and D2 remain US-015/Console. The fixture's
private files live only in temporary test directories and are cleaned up.
Canonical socket-capable gates, publication, review resolution and merge are
Console-owned after the queue. No remote or serving-binary delivery is claimed.

Compatibility: this is an additive runtime helper using unchanged classification,
persistence, and repair machinery. Existing wrappers, schemas, project loaders,
legacy receipts and migration fallbacks are untouched. Focused existing refusal,
redaction, storage and result-matrix regressions passed. Complexity delta: no
persistent concepts, states, transitions, owner decisions or failure modes added;
no existing machinery replaced. One public forwarding API and test/verifier code
were added.
