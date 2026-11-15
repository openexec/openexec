# Historical run of `scripts/verify-workspace-search.py` at `a149fceb`

Evidence record for the follow-up to task 9672102c. Nothing here is invented;
every block below is the tool output as returned.

## Method

`git worktree add` was refused by the console for this unattended task, so a
disposable clone was used instead (the refusal message's own suggested route):

1. `git clone -n . .base`
2. `git -C .base checkout a149fceb -- .` (replaces the working files and index with the `a149fceb` tree; `.base`'s own HEAD stays at the clone's default, so `git rev-parse HEAD` there is NOT `a149fceb`. Identity of the tree is shown instead by the empty `git diff --cached --stat a149fceb -- pkg scripts` below.)
3. `git -C .base cat-file -t a149fceb` -> `commit`
4. `python3 .base/scripts/verify-workspace-search.py` (the script resolves its root from `__file__`, line 4, so it ran against the `.base` tree).

Temporary location: `.base/` inside this task worktree (a clone, not a registered
`git worktree`, so `git worktree list` never listed it).

## Cleanliness of `pkg/` and `scripts/`

- Before run: `git -C .base diff --cached --stat a149fceb -- pkg scripts` -> (no output)
- After run: `git -C .base diff --cached --stat a149fceb -- pkg scripts` -> (no output); `git -C .base diff --stat -- pkg scripts` -> (no output)
- Note: plain `git -C .base status --porcelain -- pkg scripts` before the run listed staged `D`/`M` entries. That is the index (`a149fceb`) compared with the clone's HEAD, not a modification of `a149fceb` content; the comparison against `a149fceb` itself is empty.
- This task worktree: `git status --porcelain` after removal -> (no output). Nothing under `pkg/` or `scripts/` was edited.

## Raw result (exit status 1)

The console returned stdout and stderr merged in one stream; they cannot be
separated here (unavailable). Verbatim:

```
advertisement EXPECTED RUNTIME FAILURE
Traceback (most recent call last):
  File "/home/perttu/.agent-console/worktrees/75d7cbfefe74a3aa13da02529530c388/.base/scripts/verify-workspace-search.py", line 30, in <module>
    assert run.returncode!=0 and assertion in run.stdout,(label,run.returncode,run.stdout)
           ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
AssertionError: ('dispatch', 0, 'ok  \tgithub.com/openexec/openexec/pkg/execution\t0.004s\n')
```

Run duration reported: 573ms. Exit status: **1**.

## Answers

- **`dispatch` line:** no `dispatch EXPECTED RUNTIME FAILURE` line was printed. The only stdout line was `advertisement EXPECTED RUNTIME FAILURE`; the run aborted at the `dispatch` control.
- **Does `('dispatch', 0, 'ok …')` appear?** Yes — verbatim in the output above, as the `AssertionError` message, emitted on the failure stream (the stdout/stderr split is unavailable). It is NOT printed by a passing control. It is the assertion tuple `(label, run.returncode, run.stdout)` formatted by the assert at `scripts/verify-workspace-search.py:30` at `a149fceb`: label `dispatch`, return code 0, and `go test` stdout `ok  \tgithub.com/openexec/openexec/pkg/execution\t0.004s`.
- **Meaning:** at `a149fceb` the mutated `dispatch` control (`case "search_files":` -> `case "unavailable_search":` in `workspace_tools.go`) still exited 0 with `go test` reporting `ok`, so that control did not fail as the script expects. Why it passed (e.g. the selected test not reaching the mutated case, or the 0.004s `ok` indicating little ran) was not investigated here and is unproven.
- **Unavailable:** separate stdout/stderr streams; `git rev-parse HEAD == a149fceb` (replaced by the tree-equality check above); `git worktree list` evidence (no registered worktree was used).

## Removal

`rm -rf .base`, then `ls -d .base` -> `ls: cannot access '.base': No such file or directory` (exit 2).

## Supervisor independent run at a registered detached worktree (exact `a149fceb` HEAD)

This section supersedes the clone-based run above for criteria 1, 3, 4 and 5.
It was NOT executed by the task agent (the agent could not create a worktree);
it was executed by the supervisor as systemd unit `ac-watch-75d7-go-20261115`
(receipt `/mnt/data1/projects/owner-model/.jobs/watch-75d7-go-20261115.json`,
raw files under `/home/perttu/.agent-console/watch-jobs/75d7-go-20261115/`,
which this task agent cannot read). The text below is copied verbatim from the
supervisor's addendum to this task (2026-11-15 06:32Z); the agent did not
re-run or alter it. The first attempt omitted Go from the systemd PATH and
failed with `FileNotFoundError`; the second added `/usr/local/go/bin`.
Parent task 9672102c is not claimed delivered by this record.

```
HEAD is now at a149fceb Merge remote-tracking branch 'origin/main' into agent-console/task-17da07171611-split-the-same-task-passes-through-the-n
a149fceb90b9265ecad4c4d33a624cc946f1fac0
worktree /home/perttu/.agent-console/watch-jobs/75d7-go-20261115/clone
HEAD 186646ee320886726512b6468f229d53da1d7714
branch refs/heads/main

worktree /home/perttu/.agent-console/watch-jobs/75d7-go-20261115/historical
HEAD a149fceb90b9265ecad4c4d33a624cc946f1fac0
detached

Before status and diff
script exit: 1
stdout:
advertisement EXPECTED RUNTIME FAILURE

stderr:
Traceback (most recent call last):
  File "/home/perttu/.agent-console/watch-jobs/75d7-go-20261115/historical/scripts/verify-workspace-search.py", line 30, in <module>
    assert run.returncode!=0 and assertion in run.stdout,(label,run.returncode,run.stdout)
           ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
AssertionError: ('dispatch', 0, 'ok  \tgithub.com/openexec/openexec/pkg/execution\t0.003s\n')

After status and diff
Removal status: 0
worktree /home/perttu/.agent-console/watch-jobs/75d7-go-20261115/clone
HEAD 186646ee320886726512b6468f229d53da1d7714
branch refs/heads/main

Historical path absent
```

Reading of that record:

- **Checkout identity:** `git worktree list` shows the `historical` worktree at detached HEAD `a149fceb90b9265ecad4c4d33a624cc946f1fac0`. The "Before status and diff" and "After status and diff" headings carry no lines beneath them, i.e. empty (the supervisor's addendum shows no output for them; it does not name the paths, so it is not path-scoped to `pkg/` and `scripts/`).
- **Exit status:** script exit 1.
- **stdout (separate):** exactly one line, `advertisement EXPECTED RUNTIME FAILURE`. No `dispatch EXPECTED RUNTIME FAILURE` line was printed, because the script aborted at the `dispatch` control.
- **stderr (separate):** the traceback above.
- **Tuple `('dispatch', 0, 'ok …')`:** it does NOT appear on stdout. It appears on stderr, as the `AssertionError` message from the assert at `scripts/verify-workspace-search.py:30`, which formats `(label, run.returncode, run.stdout)`. Its `go test` stdout field is `ok  \tgithub.com/openexec/openexec/pkg/execution\t0.003s` (the earlier clone run gave `0.004s`; the difference is timing only).
- **Removal:** the second `git worktree list` no longer lists `historical`, and the record prints `Removal status: 0` and `Historical path absent`.
- **Correction to the section above:** the earlier statement that the tuple was on "the failure stream (split unavailable)" is now resolved: it is stderr.
- **Still unproven:** why the mutated `dispatch` still passed `go test`. Not investigated.
