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

Content-hash matching, not filename matching: walk each root, skip symlinks,
`node_modules` and `.git`, skip files over 64 MiB, compute sha256 of every other
regular file and compare with the hash sought. Existence of the two named paths
was checked directly.

## Locations actually searched (2026-11-15, read-only)

| Location | Result |
| --- | --- |
| Capture path above (existence check, file Read tool and `os.path.exists`) | **absent** |
| Events file above (`os.path.exists`) | present (not hashed, not read for this search) |
| This task's worktree, `.` (1586 regular files hashed, 0 read errors, 0 skipped over 64 MiB) | **0 matches** |

## Locations NOT searched, and why

The unattended lane refused every command naming a path outside this task's
checkout ("outside this unattended task's authority"). Therefore **no hash
search was made** of, and nothing is known about the contents of:

- `/home/perttu/.agent-console/run-requests/` (other files than the absent path)
- `/home/perttu/.agent-console/perttu/` (scratch and archives)
- other `/home/perttu/.agent-console/` directories and other worktrees
- `/mnt/data1/projects/openexec` and `/mnt/data1/projects/agent-console`
- backups, job logs, any other host location

A first, broader attempt covering these roots (run from the checkout without
arguments) was killed after the 25 minute timeout and produced no output, so it
is not counted as a search either.

## Reason recovery is impossible from this pass

The capture path is absent, and no file in the one searched accessible location
hashes to the sought value. The capture's hash is not recorded in the test, so
the follow-up's hash is the only identity available, and a file can only be
recognised by hashing it. The remaining locations could not be reached by this
lane. Absence there is **unknown**, not established. No claim of host-wide
absence is made.

## Position

- The authentic-capture requirement is **retained**.
- The standard gate **stays blocked** on this item.
- **Nothing was synthesized or reconstructed.**
- No change was made to `d26_evidence_test.go`, `testdata/d26-structure.json` or
  any hash.
- A host-side search by hash of the unsearched locations above, run by someone
  with access, would complete this record. If a file with the sought hash is
  found, this refusal is withdrawn and the parent task's provisioning criteria
  apply. The keep-or-relax decision stays with the owner on the parent card.
