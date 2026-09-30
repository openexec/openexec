# Storage and compatibility unit coverage — US-014 / T-US-014-002

Run `bash scripts/verify-verification-repair.sh --case storage-unit-coverage`.
The gate requires strictly greater than 90% statement coverage over the full
bodies declared in `storage-coverage-scope.json`: every function in the evidence
capture/storage implementation and runtime wrappers, plus the manager's
`diagnosticFreeReceipt`, `recapturePhase` and `resolveRecaptureCommand`.
Whole-file declarations automatically include newly added storage helpers.
This is a storage/reference compatibility scope, not whole-manager coverage.

The gate reuses the existing AST inventory and Go instrumentation evaluator.
It independently instruments the current sources and requires every expected
block, including uncovered blocks, in the generated count profile. Missing,
empty, malformed, wrong-mode, partial-file, partial-body and mismatched
profiles fail. Exactly 90% fails. Required test identities and package passes
are checked; any failure or skip fails. Dedicated storage test files are
inventoried to prevent silently dropping new tests. Stale output is removed
before starting. Output defaults to `/tmp/openexec-storage-coverage` and
contains the profile, full-body scope, test events and result with source
hashes and revision; these transient artifacts are not committed.

Coverage combines existing capture/redaction and reference refusal tests with
new tests for independent SHA-256 identity, idempotent writes, changed-content
identity, exact persisted command reads, preserved public parent permissions,
private child permissions, failed-rename cleanup, malformed identities, and
exclusion of ambient environment and unapproved metadata. Raw command secrets
remain available only through private evidence; the public projection is
redacted. Existing tests require registered protected references after SQLite
reopen and refuse registered legacy paths even if the same hash exists in
protected storage. The separate private-storage journey exercises actual
commands, reload, and Git staging exclusion.

Verified in this candidate:

- Storage gate: exit 0, 175/185 statements (94.59%); all 12 required Go test
  identities passed. Its 12 Python verifier controls passed, including missing
  profiles, absent files/blocks, strict threshold, skipped tests, scope failures
  and removal of stale success artifacts.
- `bash scripts/verify-verification-repair.sh --case private-storage`: exit 0,
  all 15 required journey/refusal identities passed without skips.
- Host declared `lint`: exit 0, Go vet and UI ESLint.
- Host declared `test`: exit 0, full Go suite and 40 UI files / 635 tests passed.
- Shell syntax and `git diff --check`: exit 0.

Compatibility evaluation: only tests, coverage tooling and documentation
changed. Production storage, registered legacy refusal, project loading,
`.openexec`, `.uaos` and tasks-JSON migration behavior are unchanged. No new
runtime abstraction, loop, transition or owner decision is introduced.
Canonical delivery gates and publication remain Console-owned.
