# Native exhausted-task discovery — US-007 / T-US-007-001

This is discovery, not the correction mechanism. The authoritative executed
result is [exhausted-task-discovery-result.json](exhausted-task-discovery-result.json).
The fixture is synthetic, based on this task's accepted incident shape; it is
not an export of the owner's database or evidence of product deployment.

The requested outcome is bounded verification of an explicitly corrected,
retained candidate. The observed baseline blocks it at the native task queue's
repair boundary. This reproduction reuses `Manager.ExecuteTasks`, native repair
insertion, task snapshots, SQLite failure binding and runnable selection. No
new production abstraction is needed for discovery. Complexity delta: zero
production concepts, persistent state types, transitions, owner decisions or
runtime loops; no production machinery replaced. Live Project context was read
(accepted portfolio Goal revision 4); that broader context does not expand this
stage. Console retains delivery and the later default-branch merge requirement.

## Inspected lifecycle

- `pkg/manager/scheduler.go`: `ExecuteTasks` delegates `TaskOriented` requests
  to `executeTaskQueue`.
- `pkg/manager/task_queue.go`: acquires queue ownership and workspace lock,
  validates scope, reconciles interruption, then processes failed tasks with
  retained evidence **before** asking for runnable tasks. The retained-failure
  branch returns repair errors directly. A newly failing pipeline wraps them
  with `repair task refused:`; the fixture deliberately exercises restart of a
  retained failed task, not that different error surface.
- `pkg/manager/events.go` and `task_failure.go`: terminal event handling calls
  `persistTaskVerificationFailure` synchronously; queue-owned failures use
  `state.RecordTaskFailureStep` to bind status and receipt atomically.
  `repairTaskFromRetainedFailure` checks run identity, failed status, agent and
  receipt integrity. Diagnostic-bearing failures call `CreateFailureRepair`.
- `internal/release/failure_repair.go`: `CreateFailureRepair` checks an existing
  evidence-specific disposition first, then rejects exhausted original attempts
  or repair counts. Completed repairs do not refund original attempts. A repair
  created with budget remaining becomes a prerequisite and reopens the original
  to pending; it inherits the candidate branch and remaining attempt budget.
- `RunnableTasks` requires pending, AFK, remaining attempts, live story and done
  dependencies. The fixture also checks pending-at-3/3 exclusion independently
  of failed-status exclusion.
- `task_execution_lock.go`: interruption reconciliation preserves attempt counts
  and does not reopen exhausted failed work. `task_boundary.go` projects retained
  work when no task is selected; this receipt-bearing case refuses earlier.
- `task_recapture.go`: diagnostic-free receipts take a separate bounded recapture
  path, whose exhaustion error/status differ. This is not that path: the receipt
  retains an exact command and timeout diagnostic. Context cancellation or a
  killed process is not necessarily a typed check failure; here a real `timeout`
  subprocess returns normal exit 124, which the existing classifier accepts.

## Executed journey and provenance

The scenario file records accepted scope and explicit fixture-owner correction
permission, tied to the same candidate path. These are fixture facts retained
in task metadata, not a claim that baseline OpenExec interprets them as new
runtime authority. No owner credential, external provider or deployment is used.

The test runs a real shell check that times out, retains its observed command,
exit and diagnostic, creates a native prerequisite repair while the original
has an attempt left, completes that repair, and atomically binds the timeout
receipt to the original's final attempt. It then applies the authorized local
correction and proves the check succeeds in the same candidate. Native queue
execution still refuses. It checks the exact refusal, zero dispatches, unchanged
full task snapshot (including branch, commits, dependencies, metadata and counts),
unchanged receipt, completed prerequisite and no invented repair. It closes and
reopens the actual database and repeats the queue call and assertions. The
corrected file and machine result are reread from disk.

`scripts/fixtures/exhausted-task/provenance.json` records the source revision,
archive digest and every original file's SHA-256. `baseline.tar.gz` contains the
smallest **package dependency closure** for compiling the unmodified native
manager on Linux: 255 non-test source/module/embedded files. It excludes the
rest of the repository, production test suites, Git history and build output.
This is preserved source, not a compiled artifact. Inspect it with
`tar -tzf scripts/fixtures/exhausted-task/baseline.tar.gz` or extract it for review.
The Go toolchain and dependencies pinned by its original go.mod/go.sum remain
prerequisites; no historical Git objects or future production files are needed.

The verifier extracts that source into a fresh temporary module, verifies all
hashes, installs only the named reproduction test and scenario, disables Go
workspace inheritance, and runs uncached Go JSON tests. The `.go.txt` suffix
keeps the fixture outside the normal repository Go package scan. The script
requires exactly one named run/pass and a package pass, rejects any skip/fail,
requires fresh result creation, validates the receipt digest and full timeout
payload, and rereads the final persisted result. Its negative controls reject
missing tests, skipped reproduction, missing evidence and an absent receipt even
with passing test events. It invokes no US-008 script and is independent of any
future correction implementation.

## Verification

- `scripts/verify-exhausted-task-discovery.sh`: passed the real native journey
  and fail-closed controls; machine evidence is linked above.
- Host `run_declared_check(check="lint")`: first exited 2 because the harness
  `.go` file was scanned outside its native package. Corrected its fixture suffix
  and loader; rerun exited 0 (Go vet and UI ESLint).
- No existing tests or production behavior changed. New tests assert the
  historical refusal deliberately; they live only in the preserved baseline,
  so future accepted production behavior need not retain the defect.
- Compatibility evaluation: this stage changes only discovery fixtures,
  verification tooling and evidence. Current/legacy project loading, migration,
  retry policies and production task execution are unchanged.
- Full canonical gates, publication, independent review and merge remain
  Console/runner-owned; local discovery success does not claim delivery.
