# Legacy recapture implementation — US-009 / T-US-009-001

The native task queue now recognizes a trusted failed receipt without usable
public diagnostics or a readable, registered private command capture. A normal
silent check with exact argv/cwd/exit evidence retains the existing repair path.
The receipt digest remains classification, not diagnostic or command authority.

Command resolution accepts a registered content-addressed private shell command
reference in the same working directory, or the task's authoritative
`VerificationScript` for its `verify`/`verification` stage. US-013 now infers a
missing phase from a sole validated gate and uses the current admitted/native
lint/test definition when no historical reference exists; see
[named recapture](named-recapture.md). It rejects multiple
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

On 2026-09-29, continuation of `T-US-009-001` verified the existing
implementation and injected-executor fixture repair already committed in this
candidate. No further production change was needed. Added the missing restart
journey with one remaining attempt: a persisted interrupted recapture reloads,
executes exactly one real failing subprocess, exhausts the existing task budget,
and remains terminal across two more database reopens without a repair or extra
dispatch. Settings stays pending with zero attempts.

- `bash scripts/verify-retained-verification-evidence.sh --case legacy-recapture`
  passed the explicit eleven-completion scenario manifest, including four
  terminal subcases. The verifier rejects missing, skipped, duplicate or
  unexpected completions. Fresh JSONL and result JSON:
  `/tmp/openexec-legacy-recapture-xu3q4loz/`.
- Targeted manager, release, pipeline and blueprint regression checks passed
  using the command below. Log:
  `/tmp/openexec-recapture-resume-regression.log`.
- Python verifier suite: 24 tests passed. Dispatcher discovery, shell syntax,
  and `git diff --check` passed.

```sh
export GOCACHE=/tmp/openexec-recapture-go-cache
bash scripts/verify-retained-verification-evidence.sh --case legacy-recapture
go test ./pkg/manager ./internal/release ./internal/pipeline ./internal/blueprint -run 'TestInjected|TestCancelledInjected|TestLegacyRecapture|TestTask|TestFreshTaskQueue|TestLiveWorkspace|TestPaused|TestStartRefusesTerminal|TestCancelledQueue|TestRetainedResult|TestRetention|TestConfiguredVerification|TestVerificationFailure|TestRunnable' -count=1 -timeout=90s
python3 -m unittest discover -s scripts/verification -p 'test_*.py' -q
bash scripts/verify-retained-verification-evidence.sh --case discovery
bash -n scripts/verify-retained-verification-evidence.sh
git diff --check
```

Canonical full gates remain with the repository runner and were not rerun here.
These results demonstrate native queue and database journeys with admitted
fixture executors and real local subprocess checks, not a deployed service.

Real subprocess failure recapture runs through queue → admitted stage → native
failure persistence → database close/reopen → usable private artifact → one
repair with a fresh evidence reference. A successful real subprocess recheck
runs through original task resume and queue convergence, then reloads both A
and Settings as done. Separate success disposition checks prove Settings stays
waiting before A resumes. Terminal cases reopen SQLite, restart the queue and
verify unchanged attempt counts and no extra dispatch. Interrupted exhaustion
starts from persisted in-progress attempts with both zero and one remaining
attempt, proving restart neither refunds nor exceeds the budget. Resolution checks refuse
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
