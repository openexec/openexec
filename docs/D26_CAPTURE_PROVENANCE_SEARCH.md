# D26 private capture: provenance search and refusal

Status: **recovery not achieved. Refusal.** This document holds metadata only
(paths, a hash, counts). No capture content is recorded, copied or synthesized.

## What is sought

- Hash sought (sha256): `0cbc9da8b5a6ed7d03db2b4a0a8ac16818f1c648f3db776090c83651bba20708`
- Capture path required by `TestD26PrivateOccurrenceEvidence`
  (`pkg/execution/d26_evidence_test.go`):
  `/home/perttu/.agent-console/run-requests/d26bd137975612543c90e5b983f60db8.json`
- Events file read first by that test:
  `/home/perttu/.agent-console/perttu/scratch/20261031T163533Z/d26-events.json`

## Method

Content-hash matching, not filename matching: each regular file under a root
is hashed in full and compared with the hash sought.

**Executed commands: unrecoverable from the retained transcript.** The exact
commands of run 4cd872d6 are not in this checkout and could not be recovered
by this pass, so none is quoted verbatim here (supervisor answer to question
d26-unrecoverable-commands: ship an explicit unrecoverable statement, invent
nothing). The form below is an **illustration of the method, not the executed
command**; its roots, flags and redirections are not claimed to match the run:

    find ROOT -type f -size -65536k -exec sha256sum {} + 2>ERRFILE | grep HASH | cut -d' ' -f3-

The `-size -65536k` bound (files of at most 64 MiB) is therefore **a bound this
document cannot show the run used**; it is the bound the earlier accepted pass
reported. Other details (whether symlinks were followed, whether `.git` or
`node_modules` were excluded, whether archives were opened, run date and host)
are **not recorded in the transcript available to this pass**.

## Locations searched, results

| Location | Files hashed (<= 64 MiB) | Files over 64 MiB (not hashed) | Hash errors | Matches |
| --- | --- | --- | --- | --- |
| `/home/perttu/.agent-console/run-requests` | 204 | not counted | not recorded | 0 |
| `/home/perttu/.agent-console/perttu` (scratch, archives) | 174185 | not counted | not recorded | 0 |
| `/home/perttu/.agent-console` (whole tree, includes both above and every worktree) | 1732040 | 131 | 0 | 0 |
| `/mnt/data1/projects` (all sibling checkouts incl. `openexec`, `agent-console`) | 4319544 | 360 | 0 | 0 |
| `/mnt/data1/backups` | 104 | 85 | 0 | 0 |
| This task's worktree (earlier Python pass, 1586 files) | 1586 | 0 | 0 | 0 |

Direct existence checks:

- Capture path above: **absent** (`ls`: "No such file or directory").
- Events file above: present; not hashed or read for this search.

Supplementary evidence (supervisor, 2026-11-15): the member lists of
`/mnt/data1/backups/agent-console/daily/agent-console-20261031.tar.gz` and
`agent-console-20261101.tar.gz`, spanning the occurrence date, each hold only
four entries (root, env, auth.json, state.json) and no `run-requests` path or
occurrence filename. This is a filename inventory only.

## Traceability of the figures

- The counts in the table are carried over from the earlier pass, which review
  8962116361a16eae28ba70168b9d1074 accepted as transcript-supported. This pass
  **could not open the run 4cd872d6 transcript** (not in this checkout; the
  host directory holding it is outside this task's authority), so it did **not
  verify any figure and quotes no output line**. Each number is "as carried,
  unverified here"; none is re-measured or newly added.
- Intended mapping, not a verified one: "Files hashed" = count of files at most
  64 MiB; "Files over 64 MiB" = count of files above it; "Hash errors" = number
  of stderr lines from hashing.
- Cells reading "not counted" or "not recorded" mean no figure is available to
  this document. No estimate is given. Result-event counts beyond the table are
  **not recorded** here.
- The 64 MiB bound says nothing about files above it; those are unknown below.
- Fresh whole-member archive hashes (measured later by the supervisor) are new
  measurements, not part of run 4cd872d6, and are not included; two archive
  members are credential files whose hashes and bytes are deliberately not
  published.

## Not searched, therefore unknown

- Files over 64 MiB (131 + 360 + 85 across the roots above), not hashed.
- The *contents* of archives: a capture embedded in a compressed archive (other
  than the two inventoried above, by name only) would not match a whole-file
  hash. Not extracted, not searched.
- Anything outside the roots above: other mounts, other hosts, remote storage,
  other users' home directories, `/tmp`, job-runner log stores not under these
  roots, cloud or off-host backups.
- Files unreadable to the searching user: the table carries 0 hash errors for
  the four wider rows (unverified here) and no error count for the two
  narrower first-pass rows.

## Reason recovery is impossible from this search

The capture path is absent and no regular file of at most 64 MiB in the
searched roots hashes to the sought value. The capture's hash is not recorded
in the test, so the follow-up's hash is the only identity available and a file
is recognised only by hashing it. Absence beyond the searched, bounded set is
**unknown**; no claim of host-wide or off-host absence is made. Without an
authentic copy the capture cannot be provisioned, and the capture must not be
synthesized.

## Position

- The authentic-capture requirement is **retained**.
- The standard gate **stays blocked** on this item.
- **Nothing was synthesized or reconstructed.**
- No change was made to `d26_evidence_test.go`, `testdata/d26-structure.json` or
  any hash.
- If a file with the sought hash is found later (including inside an archive or
  off-host), this refusal is withdrawn and the parent task's provisioning
  criteria apply. The keep-or-relax decision stays with the owner on the parent
  card.
