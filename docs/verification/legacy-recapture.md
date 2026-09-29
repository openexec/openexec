# Legacy recapture implementation — US-009 / T-US-009-001

The native task queue now recognizes a trusted failed receipt without usable
public diagnostics or a readable, registered private command capture. A normal
silent check with exact argv/cwd/exit evidence retains the existing repair path.
The receipt digest remains classification, not diagnostic or command authority.

Command resolution accepts a registered content-addressed private shell command
reference in the same working directory, or the task's authoritative
`VerificationScript` for its `verify`/`verification` stage. It rejects multiple
checks, phase mismatches, ambiguous commands, foreign directories, unreadable
references and unknown identities. Present-day lint/test configuration and task
prose do not identify an unknown historical command. Non-shell argv references
are conservatively unresolved; they are never reconstructed as shell text.

Recapture executes exactly one deterministic stage in the existing native
pipeline. It preserves the admitted executor when configured, never substitutes
a host executor for an admission refusal, skips agent context preparation, and
uses the existing private capture/failure binding. Stage retries are disabled;
each dispatch atomically spends an existing task attempt before execution. The
normal task/story dependency predicate is reused without temporarily changing
persisted status. Settings remains pending until its prerequisites complete.

The existing metadata map records `recapture_outcome`: running, success, failed,
exhausted, unresolved, refused or cancelled. Failed diagnostic capture proceeds
to ordinary same-story repair. Success removes the obsolete receipt binding and
resumes the original task through ordinary completion validation; it does not
mark the task or its dependents done. Exhaustion/unresolved/refusal/cancellation
retain work at the existing needs_review boundary. Restart neither dispatches
these terminals nor refunds an interrupted attempt. Attempt-checked claims and
dispositions preserve concurrent status changes. No schema migration is needed.

## Executed verification

On 2026-09-29 in the candidate workspace, refreshed by repair task
`repair-9d90217db3207e2ed19c0190430c1ab4`:

The supplied persisted failure receipt contains only gate `test` and exit 2;
it does not establish the historical failing assertion. Reproduction found
`TestInjectedExecutorUsesRealQueueAndTrustedRepair/typed_failure` failing with
`verification recapture unresolved: original verification command unresolved`.
Its injected executor returned a bare typed exit from `false`, without command
evidence, but expected immediate repair. That expectation conflicts with the
accepted diagnostic-free receipt behavior. The fixture now retains the actual
silent subprocess's argv, cwd and observed exit through the existing capture
API. Its original repair/resume, attempt-count, no-duplicate and no-host-fallback
assertions remain. The forged-artifact refusal remains, and a separate legacy
receipt case proves that missing identity produces `needs_review`/`unresolved`
without a repair or extra attempt. No production behavior or gate was changed.

The focused command below failed before the fixture repair and passed afterward
(three subcases). Local logs are `/tmp/openexec-repair-repro.log` and
`/tmp/openexec-repair-fixed.log`.

- `bash scripts/verify-retained-verification-evidence.sh --case legacy-recapture`
  passed the explicit ten-completion scenario manifest, including four terminal
  subcases. The verifier rejects missing, skipped, duplicate or unexpected test
  completions and writes fresh JSONL and result JSON under the printed temporary
  evidence directory. Fresh result:
  `/tmp/openexec-legacy-recapture-cfx1o903/result.json`.
- Targeted Go regression execution passed manager task/queue/restart/repair,
  pause/cancellation, retained-result and retention checks; release task and
  runnable selection checks; pipeline retention/classification checks; and
  blueprint capture checks, plus the injected-executor and cancellation cases.
  Pattern and command are below; log: `/tmp/openexec-repair-regression.log`. The selected state
  package had no matching tests; no state-package test coverage is claimed.
- Python verifier suite: 24 tests passed. Dispatcher discovery, shell syntax,
  and `git diff --check` passed.

```sh
export GOCACHE=/tmp/openexec-recapture-go-cache
go test ./pkg/manager -run '^TestInjectedExecutorUsesRealQueueAndTrustedRepair$' -count=1 -timeout=30s
go test ./pkg/manager ./internal/release ./internal/pipeline ./internal/blueprint ./pkg/db/state -run 'TestInjected|TestCancelledInjected|TestLegacyRecapture|TestTask|TestFreshTaskQueue|TestLiveWorkspace|TestPaused|TestStartRefusesTerminal|TestCancelledQueue|TestRetainedResult|TestRetention|TestConfiguredVerification|TestVerificationFailure|TestRunnable' -count=1 -timeout=90s
python3 -m unittest discover -s scripts/verification -p 'test_*.py' -q
bash scripts/verify-retained-verification-evidence.sh --case discovery
bash -n scripts/verify-retained-verification-evidence.sh
git diff --check
```

Broader reproduction is not a full-gate pass: `make test` was interrupted by
sandbox refusal of provider access to `api.anthropic.com`. A direct Go run first
encountered the read-only default cache; the writable cache above resolved that
environment issue. The whole manager suite with `-timeout=60s` exposed the
fixture failure before reaching its timeout during scheduler tests. These runs
are recorded in `/tmp/openexec-repair-test.log` and
`/tmp/openexec-repair-manager.log`. Canonical full gates remain with the
repository runner; the targeted commands above completed successfully.

Real subprocess failure recapture runs through queue → admitted stage → native
failure persistence → database close/reopen → usable private artifact → one
repair with a fresh evidence reference. A successful real subprocess recheck
runs through original task resume and queue convergence, then reloads both A
and Settings as done. Separate success disposition checks prove Settings stays
waiting before A resumes. Terminal cases reopen SQLite, restart the queue and
verify unchanged attempt counts and no extra dispatch. Interrupted exhaustion
starts from a persisted in-progress last attempt. Resolution checks refuse
unregistered/mismatched references and reload a registered original command.

Shared fixture interfaces live in `pkg/manager/recapture_fixture_test.go`:
`newRecaptureFixture`, `recaptureReceipt`, `restart`, `assertTerminal`, and the
admitted `Execute` adapter. Boundary, independent coverage and protected-format
compatibility tasks retain their assigned ownership. Canonical repository gates,
independent review, publication and deployment were not run in this stage.

Compatibility evaluation: no project discovery, `.uaos` or tasks.json migration
path changed. Existing current-evidence repair/restart journeys passed. Only
legacy receipts without usable context take the new bounded native branch.
Complexity delta: no new scheduler, task type, retry budget, persistent table or
owner decision; one optional single-stage pipeline input and existing task
metadata/attempt/status transitions implement deterministic recapture. New
failure outcomes are explicit unresolved identity and exhausted recapture;
existing admission, cancellation and private-artifact failures remain closed.
