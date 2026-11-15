# D26 private capture: provenance search and refusal

Status: **authentic capture not found in the recorded search.** The two executed
scripts of run 4cd872d6 are recorded verbatim below. This document holds
metadata only (paths, a hash, counts). No capture content is recorded, copied
or synthesized.

## What is sought

- Hash sought (sha256): `0cbc9da8b5a6ed7d03db2b4a0a8ac16818f1c648f3db776090c83651bba20708`
  (recorded in `docs/QWEN_DISPATCHED_CONTEXT_OVERFLOW_DIAGNOSIS.md`).
- Capture path read by `TestD26PrivateOccurrenceEvidence`
  (`pkg/execution/d26_evidence_test.go`):
  `/home/perttu/.agent-console/run-requests/d26bd137975612543c90e5b983f60db8.json`

## Hash-matching method

Content-hash matching, not filename matching: `sha256sum` is run over every
regular file the `find` predicate selects, and the output is filtered with
`grep` for the hash sought; only the path column is printed. A filename, size
or archive member name never counts as a match. Whole-file hashing means a
capture embedded in a compressed archive does not match unless the archive
member itself is hashed.

## Executed commands (verbatim, from run 4cd872d6's native transcript)

Source: native transcript of run 4cd872d67fc61a6066e53d830b54bae5 at
`/home/perttu/.claude/projects/-home-perttu--agent-console-worktrees-25af4744676d36ab6ed8a42309c46e05/d52b9f1a-ba74-4fe6-8d09-c7a28dfa97f6.jsonl`.
The supervisor (addendum 1, 2026-11-15) recovered it and transmitted the two
`Write` tool calls below. This pass did not read the transcript itself (its
tools refuse paths outside the checkout); the scripts are the supervisor's
transmitted copy. Each was written to `d26_search_tmp.sh` and run with
`sh d26_search_tmp.sh`. Result events are API sequences 11-12 (first) and 16-17
(second) per the supervisor.

First script, tool call `toolu_013Tc9CWd5fqmF6xdnsWAGA8`:

```sh
#!/bin/sh
# Read-only sha256 search. Prints counts and matching paths only.
H=0cbc9da8b5a6ed7d03db2b4a0a8ac16818f1c648f3db776090c83651bba20708
for R in /home/perttu/.agent-console/run-requests /home/perttu/.agent-console/perttu; do
  echo "ROOT $R"
  echo "files=$(find "$R" -type f -size -65536k 2>/dev/null | wc -l)"
  find "$R" -type f -size -65536k -exec sha256sum {} + 2>/dev/null | grep "$H" | cut -d' ' -f3-
  echo "done $R"
done
ls -l /home/perttu/.agent-console/run-requests/d26bd137975612543c90e5b983f60db8.json 2>&1
```

Second script, tool call `toolu_016jxALx58c4gaoUP6akPTMW`:

```sh
#!/bin/sh
# Read-only sha256 search. Prints counts and matching paths only.
H=0cbc9da8b5a6ed7d03db2b4a0a8ac16818f1c648f3db776090c83651bba20708
for R in /home/perttu/.agent-console /mnt/data1/projects /mnt/data1/backups; do
  echo "ROOT $R"
  echo "files_le64MiB=$(find "$R" -type f -size -65536k 2>/dev/null | wc -l)"
  echo "files_over64MiB=$(find "$R" -type f -size +65535k 2>/dev/null | wc -l)"
  find "$R" -type f -size -65536k -exec sha256sum {} + 2>/tmp/d26_err.txt | grep "$H" | cut -d' ' -f3-
  echo "hash_errors=$(wc -l < /tmp/d26_err.txt)"
  echo "done $R"
done
```

Supervisor-recorded facts about these runs:

- The first invocation exited 2 because the final `ls` of the target path
  failed; the second invocation exited 0. No matching path was printed.
- The size predicate is the literal `find -size -65536k`. `find` rounds file
  sizes up to whole units, so this is **not** equivalent to "<= 64 MiB"; it is
  recorded as the literal predicate, not as a 64 MiB bound. The over-size count
  in the second script uses the literal `-size +65535k`.
- The first script discards all `sha256sum` errors (`2>/dev/null`). The second
  reports `hash_errors` as the line count of the stderr file, which is not
  necessarily a count of every error.
- The shared `-size` predicate means files at or above 65536 KiB (as `find`
  rounds) were not hashed in either script.

## Locations searched, by those commands

1. `/home/perttu/.agent-console/run-requests` (script 1)
2. `/home/perttu/.agent-console/perttu` (script 1)
3. `/home/perttu/.agent-console` (whole tree, script 2)
4. `/mnt/data1/projects` (script 2)
5. `/mnt/data1/backups` (script 2)

Roots 1 and 2 are subsets of root 3 and were hashed again there. A direct
`ls -l` of the capture path was run at the end of script 1 and failed (script 1
exited 2): the capture path is **absent**.

## Counts

Source: the supervisor's reading (addendum 2, 2026-11-15) of
`GET /api/runs/4cd872d67fc61a6066e53d830b54bae5/events`, sequences 11-12 (first
script) and 16-17 (second script). This pass transcribes those figures; it did
not read the events itself. They are counts of what the scripts printed, not
proof about unsearched data or archive contents.

First script (`sh d26_search_tmp.sh`, exit 2 after 49.54 s; the exit status comes
from the final `ls` failing):

| Root | `files=` (`-size -65536k`) |
| --- | --- |
| `/home/perttu/.agent-console/run-requests` | 204 |
| `/home/perttu/.agent-console/perttu` | 174185 |

The final `ls -l` of the capture path returned "No such file or directory".

Second script (exit 0 after 14 m 26.795 s):

| Root | `files_le64MiB=` (`-size -65536k`) | `files_over64MiB=` (`-size +65535k`) | `hash_errors=` |
| --- | --- | --- | --- |
| `/home/perttu/.agent-console` | 1732040 | 131 | 0 |
| `/mnt/data1/projects` | 4319544 | 360 | 0 |
| `/mnt/data1/backups` | 104 | 85 | 0 |

Reading these: the `files_over64MiB` column is the number of files the second
script found but did **not** hash (literal predicate `-size +65535k`; the labels
in the script's output are the script's own, and the predicates are not
equivalent to an exact 64 MiB bound). `hash_errors=0` is a stderr line count for
that script only. In the first script the `files=` count and the hash run use the
same predicate, but its `sha256sum` errors were discarded, so no error count
exists for it. No matching path was printed by either script.

## Archive inventory

Separately from the scripts above, the supervisor's addenda report that the two
archives adjacent to the occurrence date,
`/mnt/data1/backups/agent-console/daily/agent-console-20261031.tar.gz` and
`agent-console-20261101.tar.gz`, list only four members (root, env, auth.json,
state.json) with no `run-requests` path, and that member searches found no
match. **Individual member hashes were not recorded** (supervisor addendum 1), so the
whole-member hash values requested by the task are **missing**: none are given
here and none are invented. That part of the evidence remains unproven; the
archive contents stay in the unknown list below.

## Not searched or not established: unknown

- Files at or above the `-size` cut-off in the hashed roots.
- Files `sha256sum` could not read (errors discarded in script 1; script 2
  records only a stderr line count).
- Contents of every other archive, and any record embedded in an archive or in
  another file (such as a job log) that a whole-file hash would not match.
- Other hosts, CI runners, off-host or cloud backups, other users' homes,
  `/tmp`, other mounts.
- Whether the capture was pruned or never retained.

No claim is made that these were ruled out.

## Reason recovery is not achieved

The capture path is absent, and the searched accessible files produced no path
whose sha256 equals the sought value. Because the unknown locations above
remain open, this is "not found in the recorded search", not proof that no
authentic copy exists. Without an authentic copy the capture cannot be
provisioned and must not be synthesized.

## Position

- The authentic-capture requirement is **retained**.
- The standard gate **stays blocked** on this item.
- **Nothing was synthesized or reconstructed.**
- No change was made to `d26_evidence_test.go`, `testdata/*` or any hash.
- `go test ./pkg/execution -run TestD26` is unchanged and fails on
  `TestD26PrivateOccurrenceEvidence` (required private artifact unavailable).
  This document does not claim it passes.
- If a file with the sought hash is found (including inside an archive or
  off-host), this refusal is withdrawn and the parent task's provisioning
  criteria apply.
