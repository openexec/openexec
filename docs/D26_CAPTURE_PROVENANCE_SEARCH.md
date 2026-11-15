# D26 private capture: provenance search and refusal

Status: **recovery not achieved; exact executed commands NOT recovered.**
This document holds metadata only (paths, a hash, counts). No capture content
is recorded, copied or synthesized.

## What is sought

- Hash sought (sha256): `0cbc9da8b5a6ed7d03db2b4a0a8ac16818f1c648f3db776090c83651bba20708`
  (recorded in `docs/QWEN_DISPATCHED_CONTEXT_OVERFLOW_DIAGNOSIS.md`).
- Capture path read by `TestD26PrivateOccurrenceEvidence`
  (`pkg/execution/d26_evidence_test.go`):
  `/home/perttu/.agent-console/run-requests/d26bd137975612543c90e5b983f60db8.json`

## Hash-matching method

Content-hash matching, not filename matching: a file is a candidate only if the
SHA-256 of its whole bytes equals the hash sought. A filename, size or member
name never counts as a match. Whole-file hashing means a capture embedded in a
compressed archive does not match unless the archive member itself is hashed.

## What survives of run 4cd872d6, and what does not

Run 4cd872d67fc61a6066e53d830b54bae5 executed the search. This pass (an
unattended lane pass) could not open that run's checkout script or native
transcript: its tools refuse every path outside this checkout (including
`/home/perttu/.agent-console`) and refuse shell expansion. Within this
repository and its git branches, the only surviving accounts are two prose
records, neither of which contains the executed commands:

1. `docs/D26_CAPTURE_PROVENANCE_SEARCH.md` on branch
   `agent-console/task-25af4744676d-split-if-recovery-is-impossible-a-writte`
   (commits `b425b32b`, `c1d65411`). It paraphrases a `find ... -exec sha256sum`
   pipeline from a throwaway script that "was removed afterwards". Review
   b77cec95 found its stated "64 MiB" bound inaccurate. That paraphrase is
   **not** reproduced here as the executed command.
2. `docs/D26_PRIVATE_EVIDENCE_PROVISIONING.md` on branch
   `agent-console/task-3ee595299406-repair-missing-d26-private-evidence-prov`
   (commit `7a2543ea`), which records the supervisor's host-side searches.

**The exact executed commands are unrecovered (unproven).** Nothing below is a
verbatim command. Counts are attributed to the record that states them and are
not independently re-derived; the two records disagree on counts and size
bounds (for example run-requests: 204 files hashed up to "64 MiB" in record 1,
212 files up to 50 MB in record 2), and that disagreement is unresolved. Neither
size bound is adopted.

## Locations recorded as searched (as reported, unverified here)

| Location | Record 1 (b425b32b/c1d65411) | Record 2 (7a2543ea, supervisor) | Matches reported |
| --- | --- | --- | --- |
| `/home/perttu/.agent-console/run-requests` | 204 files hashed | 212 files | 0 |
| `/home/perttu/.agent-console/backups` | not listed | 58 files | 0 |
| `/home/perttu/.agent-console/perttu/scratch` | 174185 files hashed (under `perttu`) | 151,399 regular non-symlink files up to 50 MB plus 55 larger files (3,905,690,883 bytes) | 0 |
| `/home/perttu/.agent-console` (whole tree) | 1732040 hashed, 131 over bound not hashed | not listed | 0 |
| `/mnt/data1/projects` | 4319544 hashed, 360 over bound not hashed | not listed | 0 |
| `/mnt/data1/backups` | 104 hashed, 85 over bound not hashed | 94 files up to 50 MB | 0 |

Record 1 reports 0 hash errors for the whole-tree and `/mnt/data1/*` rows and
"not recorded" for the narrower rows; record 2 reports zero read errors.

Direct existence check (record 1): the capture path above is **absent**
("No such file or directory"). Two occurrence-named files under
`perttu/scratch` (`…153647Z`, `…160203Z`) hash differently and must not be
provisioned (record 2).

Archive inventory (supervisor addenda, via record 1): the two archives
adjacent to the occurrence date,
`/mnt/data1/backups/agent-console/daily/agent-console-20261031.tar.gz` and
`agent-console-20261101.tar.gz`, each list only four members (root, env,
auth.json, state.json), with no `run-requests` path. The supervisor addenda
report whole-member hashes for them; those hash values are not available to
this pass and are not reproduced, so they are **unverified here**.

## Not searched or not established: unknown

- The executed commands themselves, and therefore the precise file-size
  exclusions and the exact counts of excluded files per location.
- Contents of every other archive, and any record embedded in an archive or in
  another file (such as a job log) that a whole-file hash would not match.
- Other hosts, CI runners, off-host or cloud backups, other users' homes,
  `/tmp`, other mounts.
- Whether the capture was pruned or never retained.

No claim is made that these were ruled out.

## Reason recovery is not achieved

On the recorded evidence the capture path is absent and no accessible file
searched hashes to the sought value. Because the executed commands, size
bounds and several locations are unknown, this is "not found in the recorded
search", not a proof that no authentic copy exists. Without an authentic copy
the capture cannot be provisioned and must not be synthesized.

## Position

- The authentic-capture requirement is **retained**.
- The standard gate **stays blocked** on this item.
- **Nothing was synthesized or reconstructed.**
- No change was made to `d26_evidence_test.go`, `testdata/*` or any hash.
- If a file with the sought hash is found (including inside an archive or
  off-host), this refusal is withdrawn and the parent task's provisioning
  criteria apply.
- To close the gap on the commands, someone with host access must supply run
  4cd872d6's checkout script or native transcript; it can then be pasted here
  verbatim.
