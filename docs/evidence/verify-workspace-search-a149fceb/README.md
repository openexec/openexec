# Command and path-scope evidence: clean `pkg/` and `scripts/` at `a149fceb`

Retained for the criterion "`git status --porcelain` and `git diff` are empty
for `pkg/` and `scripts/` in the working tree used, both before and after the
run". Companion to PR 232 / parent 9672102c; this record changes neither, and
makes no claim that the parent was delivered. PR 211 does not establish this
evidence. Nothing was rerun for this record.

## Files (verbatim, byte for byte as supplied)

| File | What it is |
| --- | --- |
| `run.sh.txt` | The executed `run.sh` (stored with a `.txt` suffix and Git mode 100644 so it is not executable). |
| `receipt.json` | Runner receipt for job `watch-75d7-go-20261115`: command `/home/perttu/.agent-console/watch-jobs/75d7-go-20261115/run.sh`, pid 3195767, status `finished`, attempts 1. |
| `record.txt` | The log `run.sh` wrote (`exec >"$log" 2>&1`). |

## Path scope and command identity

The three commands ran in the temporary detached worktree
`.../75d7-go-20261115/historical` at `a149fceb90b9265ecad4c4d33a624cc946f1fac0`
(`rev-parse HEAD` in `record.txt`), not in a working tree used for editing:

1. `git -C "$wt" status --porcelain -- pkg scripts`
2. `git -C "$wt" diff -- pkg scripts`
3. `git -C "$wt" diff --cached -- pkg scripts` (the staged diff is included)

Position: all three ran right after the heading `Before status and diff`
(before the script), and again right after `After status and diff` (after the
script). The pathspec is `pkg scripts`; nothing else in the tree is covered.
In `record.txt` the spans under both headings are blank as printed: under
"Before" the next line is `script exit: 1`, under "After" the next line is
`Removal status: 0`.

## Provenance limits

- `run.sh`, the receipt and `record.txt` are supervisor-provided. The task
  agent has no access to the source paths and did not independently re-fetch or
  re-verify them.
- Blank output was observed, but `run.sh` captures no exit code for any of the
  git commands. Emptiness is evidenced by the absence of output, not by a
  recorded exit status. Stderr was merged into the log and no error text
  appears there.
- `git status --porcelain` omits ignored files.
- The script itself exited 1 (`script exit: 1`). Its stdout was
  `advertisement EXPECTED RUNTIME FAILURE`. The
  `('dispatch', 0, 'ok ...')` tuple does not appear in stdout; it appears only
  in the stderr `AssertionError` raised at line 30 of
  `scripts/verify-workspace-search.py`.
- Temporary worktree removal: `Removal status: 0`, `worktree list` without the
  path, and `Historical path absent`.
