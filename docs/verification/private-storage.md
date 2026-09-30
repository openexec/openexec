# Private verification storage — US-014 / T-US-014-001

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

- `bash scripts/verify-verification-repair.sh --case private-storage`: exit 0,
  all 15 required test identities passed. Six actual gate/deterministic exit-2
  journeys cover fresh initialization, old managed ignores without reinit, and
  whole-state-tree ignores. Each rereads exact argv/cwd/exit and raw stderr,
  checks public redaction and permissions, verifies clean Git status, performs
  `git add .`, and verifies the index excludes evidence.
- Reader tests reject malformed identities, corrupt/missing files, public modes,
  symlink files and symlink/non-directory parents. Manager tests reopen SQLite
  and accept protected references while preserving and refusing legacy ones.
  Existing native recapture failure/reload/repair coverage passes with the new
  paths. An additional resolver case rejects a legacy registration even when
  its hash also exists in protected storage.
- Isolated Go source-overlay negative control restored the original writer.
  Exit 1; all six journeys independently detected the old path, missing ignore,
  dirty status and evidence entering the index. The candidate source remained
  unchanged by this control.
- Host declared `lint`: exit 0 (Go vet and UI ESLint).
- Host declared `test`, after the correction below: exit 0, full Go suite and
  40 UI test files / 635 UI tests passed.
- `make compat-test`: exit 0; current `.openexec`, legacy `.uaos`, configuration
  and tasks-JSON fallback coverage passed. `make type-check`: exit 0, Go build
  and TypeScript `tsc --noEmit`. Both used a writable temporary Go cache.

The first host declared `test` returned exit 2 in `pkg/manager`. A separate
package run identified `TestInjectedExecutorUsesRealQueueAndTrustedRepair`'s
stale legacy expectation. The existing named-check resolver already recaptures
through the admitted executor, so that test now asserts exhaustion of the
existing attempt budget, no repair, and no host-command fallback. This is a test
expectation correction; named-check execution behavior was not changed here.

Canonical repository delivery gates,
publication, review and merge remain Console-owned; this implementation record
makes no deployment or whole-story completion claim.
