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

**Source.** The commands and output lines below are transcribed from run
4cd872d6 (`4cd872d67fc61a6066e53d830b54bae5`), events sequences 11-12 and 16-17,
as supplied by the supervisor on 2026-11-18 (task addendum 8). They are
transmitted evidence: this pass had no direct host access to the transcript and
re-measured nothing. In the supplied text the hash is abbreviated
(`0cbc9da8...a20708`); it is the hash sought above. Each command was run as
`sh d26_search_tmp.sh`, a script written by a Write tool call beforehand.

### Invocation 1 (exit 2 after 49.54s, because the final `ls` failed)

    H=0cbc9da8...a20708 ; for R in /home/perttu/.agent-console/run-requests /home/perttu/.agent-console/perttu; do echo "files=$(find "$R" -type f -size -65536k 2>/dev/null | wc -l)"; find "$R" -type f -size -65536k -exec sha256sum {} + 2>/dev/null | grep "$H" | cut -d' ' -f3- ; done ; ls -l /home/perttu/.agent-console/run-requests/d26bd137975612543c90e5b983f60db8.json 2>&1

Output lines recorded: `files=204` (root `run-requests`), `files=174185` (root
`perttu`), then `ls: No such file or directory`. No matching path was printed.
Hash errors were suppressed (`2>/dev/null`), so no error count exists for this
invocation.

### Invocation 2 (exit 0 after 14m26.795s)

Roots `/home/perttu/.agent-console`, `/mnt/data1/projects`,
`/mnt/data1/backups`. Per root, as recorded (the transcript text supplied names
the per-root command, with `R` the root):

    echo "files_le64MiB=$(find "$R" -type f -size -65536k 2>/dev/null | wc -l)"; echo "files_over64MiB=$(find "$R" -type f -size +65535k 2>/dev/null | wc -l)"; find "$R" -type f -size -65536k -exec sha256sum {} + 2>/tmp/d26_err.txt | grep "$H" | cut -d' ' -f3- ; echo "hash_errors=$(wc -l < /tmp/d26_err.txt)"

Output lines recorded:

- `/home/perttu/.agent-console`: `files_le64MiB=1732040`, `files_over64MiB=131`, `hash_errors=0`
- `/mnt/data1/projects`: `files_le64MiB=4319544`, `files_over64MiB=360`, `hash_errors=0`
- `/mnt/data1/backups`: `files_le64MiB=104`, `files_over64MiB=85`, `hash_errors=0`

No matching path was printed in either invocation.

### Size predicates and caveats (stated as recorded)

- Hashed set: `find -size -65536k` (strictly under 65536 KiB, below 64 MiB).
- Counted but **not hashed**: `find -size +65535k` (more than 65535 KiB). A file
  of exactly 64 MiB (65536 KiB) therefore falls in the "over" count.
- `hash_errors` is a count of stderr lines, not necessarily every error.
- Not recorded in the transcript: whether symlinks were followed, whether
  `.git` or `node_modules` were excluded, whether archives were opened, and any
  result-event count other than the output lines above.

## Locations searched, results

Every number is the quoted output line named in its last column.

| Location | `-size -65536k` files hashed | `-size +65535k` files not hashed | Hash errors | Matches | Source line |
| --- | --- | --- | --- | --- | --- |
| `/home/perttu/.agent-console/run-requests` | 204 | not recorded | not recorded (stderr discarded) | 0 | invocation 1, `files=204` |
| `/home/perttu/.agent-console/perttu` | 174185 | not recorded | not recorded (stderr discarded) | 0 | invocation 1, `files=174185` |
| `/home/perttu/.agent-console` | 1732040 | 131 | 0 | 0 | invocation 2, `files_le64MiB=` / `files_over64MiB=` / `hash_errors=` |
| `/mnt/data1/projects` | 4319544 | 360 | 0 | 0 | invocation 2, same three lines |
| `/mnt/data1/backups` | 104 | 85 | 0 | 0 | invocation 2, same three lines |

"Matches 0" means no matching path was printed. The rows for
`run-requests` and `perttu` lie inside the `.agent-console` tree. An earlier
Python pass over this task's worktree (1586 files) is not part of run 4cd872d6
and is omitted as untraceable to it.

Direct existence check (invocation 1, final `ls`):

- Capture path above: **absent** (`ls: No such file or directory`).
- Events file above: not hashed or read by the recorded commands.

Supplementary evidence (supervisor, 2026-11-15, not part of run 4cd872d6): the
member lists of
`/mnt/data1/backups/agent-console/daily/agent-console-20261031.tar.gz` and
`agent-console-20261101.tar.gz` each hold only four entries (root, env,
auth.json, state.json) and no `run-requests` path or occurrence filename. This
is a filename inventory only. Fresh whole-member archive hashes are separate new
measurements, not included; two members are credential files whose hashes and
bytes are deliberately not published.

## Not searched, therefore unknown

- Files over 64 MiB (131 + 360 + 85 across the three wide roots), counted, not
  hashed.
- The *contents* of archives: a capture embedded in an archive would not match a
  whole-file hash. Not extracted, not searched.
- Anything outside the roots above: other mounts, other hosts, remote storage,
  other users' home directories, `/tmp`, other log stores, off-host backups.
- Files unreadable to the searching user: invocation 1 discarded stderr, and
  invocation 2's `hash_errors=0` counts stderr lines only.

## Reason recovery is impossible from this search

The capture path is absent and no regular file under `-size -65536k` in the
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
