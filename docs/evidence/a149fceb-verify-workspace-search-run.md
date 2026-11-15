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
