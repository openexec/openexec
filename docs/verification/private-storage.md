# Private verification storage — US-014 / T-US-014-003

New private command evidence lives only in `.openexec/data/verification`.
The existing initialization ignore for `.openexec/data/` protects both fresh
and previously initialized projects; ignoring the whole `.openexec/` tree
also protects it. No target reinitialization or source ignore edit is needed.
This guarantee assumes the project's existing ignore rules have not been
removed or explicitly overridden; it does not prevent `git add -f`.

The shared evidence writer creates nested directories one component at a time,
rejecting symlinks and non-directories. Existing state parents retain their
permissions; the evidence directory must be 0700 and files must be regular 0600
files. Private reads validate lowercase SHA-256 identity, content digest and
permissions. Creation and reading reuse the existing rooted filesystem and
bounded capture primitives. No execution loop, persistent concept, transition
or owner decision was added.

Legacy policy is explicit refusal, not migration or fallback. Root-level
`.openexec-verification` files remain untouched, as do their persisted artifact
references. Even a registered legacy reference cannot authorize recapture or
borrow a same-hash protected file. A protected reference must match the
candidate's exact storage path and registration. Missing, corrupt, public-mode,
foreign or symlinked evidence does not authorize replay. Existing root-level
files are not newly protected by this relocation, and previously tracked or
published evidence is not removed from Git history. This change makes no
cleanup claim about such files. New captures always use the protected location.

Verification uses generated credential fixtures kept inside temporary repos.
No raw argv, stderr or credential values are included in this record or the
verifier's output.

Consolidated verification was rerun on 2026-09-30 from base revision
`54053d41095a94f60d9149d27fff66d5912709d8`, with this task's verifier changes.
Production code was unchanged. This record supersedes earlier story run totals.

| Command | Result and assertions |
| --- | --- |
| `bash scripts/verify-verification-repair.sh --case private-storage` | Exit 0; all 15 required identities passed, none missing or skipped. Four negative-control evaluator tests passed. |
| `bash scripts/verify-verification-repair.sh --case storage-unit-coverage` | Exit 0; 181/192 statements (94.27%), above the strict 90% threshold; all 12 required Go identities and 12 Python verifier controls passed. |
| Host `run_declared_check` named `lint` | Exit 0; Go vet and UI ESLint. |
| Host `run_declared_check` named `test` | Exit 0; full Go suite and 40 UI files / 635 tests passed. |
| `make compat-test type-check` | Exit 0; current `.openexec`, legacy `.uaos`, legacy configuration and `.openexec/tasks.json` fallback; Go build and TypeScript `tsc --noEmit`. |
| `git diff --check` | Exit 0. |

Coverage and Make commands used `GOCACHE=/tmp/openexec-retention-go-cache`
via Python's subprocess environment. Go emitted a read-only module stat-cache
warning during coverage; the gate completed successfully. Full coverage scope,
per-function counts, command, source hashes and revision are in the generated
`/tmp/openexec-storage-coverage/result.json`; transient artifacts are not staged.
The scope and enforcement contract live in [storage unit coverage](storage-unit-coverage.md).

Six real exit-2 subprocess journeys cover gate and deterministic execution in
freshly initialized, previously initialized managed-ignore, and whole-state-tree
ignored repositories. The old managed-ignore fixture does not reinitialize.
Every journey rereads exact private argv/cwd/exit and raw stderr from disk,
checks credential redaction in public results and references, checks directory
0700/file 0600, requires `git check-ignore` success and clean status, runs
`git add .`, then requires an empty staged diff and no tracked verification file.

Storage and reader checks cover independent SHA-256 identity, repeat-write
identity, changed-content identity, cleanup after failed rename, retained parent
permissions, exclusion of ambient environment/unapproved metadata, malformed
identity, corruption, missing files, public permissions, symlinks and non-directory
parents. Manager journeys reopen SQLite and accept registered protected paths;
registered legacy paths remain persisted but are refused, including a legacy
registration whose hash also exists in protected storage. Missing, foreign and
unregistered references cannot authorize replay. Native recapture still runs
through failure, reload and repair.

The private-storage verifier now retains the isolated negative control. A Go
source overlay changes only the storage directory constant, directory walk and
permission-name check back to `.openexec-verification`; reader and writer remain
aligned so a read failure cannot stand in for source exposure. The candidate's
production file is checked unchanged and the overlay is removed afterwards.
The overlaid journey command exited 1. All six layout/executor identities failed
with every required diagnostic: `private artifact not ignored`,
`capture dirtied repository`, `capture entered index`, and
`private artifact tracked`. The evaluator refuses success, compilation/path-only
failures, skipped/missing journeys or any missing exposure/staging assertion.
Only dispositions and assertion names are printed, never private fixture output.

Compatibility evaluation: this task changes verification tooling and evidence
only. No project loader, migration, production behavior, execution loop,
persistent concept, transition or owner decision changed. Complexity delta is
one isolated regression control and its evaluator tests, using existing journeys.
Local persistence uses ordinary Git in the authorized candidate worktree, per
the stage instruction replacing the unavailable daemon safe-commit workflow.
Canonical repository gates, publication, independent review and merge remain
Console-owned. These results make no deployment claim.
