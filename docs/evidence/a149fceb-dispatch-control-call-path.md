# `dispatch` control at a149fceb: call path and observed output

Read-and-record only. No product file, test or script was changed.
All file:line citations are from the tree at commit `a149fceb` (merge of
origin/main into `agent-console/task-17da07171611-…`), **not** from current main,
where `executeToolScan` no longer exists in `pkg/`. `a149fceb` is not an ancestor
of main `1721cdf0`/`15791724`; it was read from this checkout's object store.

## 1. Raw job 126662911673 — UNPROVABLE
The raw log was not read. A search for the id `126662911673` under
`/mnt/data1/projects/owner-model`, `/mnt/data1/projects/openexec/.openexec` and
`/mnt/data1/projects/agent-console/data` found no file containing it, and the
job runner's store was not otherwise reachable from this sandbox. Its contents,
and anything said about it by earlier tasks, are therefore unprovable here.

## 2. Material actually read
- Commit `a149fceb` (task 17da source tree) and its parents `0bbd3f2d` (task 17da
  feature commit) and `d4e65423` (#203), via `git log` / checkout.
- At `a149fceb`: `scripts/verify-workspace-search.py`,
  `pkg/execution/workspace_search_test.go`, `workspace_tools.go`,
  `composite_tools.go`, `api_provider.go`.
- Safety child bde7666d / PR207: **not read** — not located in this checkout;
  unprovable here.

## 3. Pre-edit sequence
Retained events (job log, task event history) were unavailable, so the
historical pre-edit read sequence is **unprovable**. All that is established is
the commit-level state above and the run in section 4.

## 4. Real output of `python3 scripts/verify-workspace-search.py` at a149fceb
Run in a temporary checkout of `a149fceb`; exit status 1. Stdout/stderr:

```
advertisement EXPECTED RUNTIME FAILURE
Traceback (most recent call last):
  File ".../scripts/verify-workspace-search.py", line 30, in <module>
    assert run.returncode!=0 and assertion in run.stdout,(label,run.returncode,run.stdout)
AssertionError: ('dispatch', 0, 'ok  \tgithub.com/openexec/openexec/pkg/execution\t0.008s\n')
```

The tuple `('dispatch', 0, 'ok …')` **does** appear, as the `AssertionError`
argument of the script's own line 30: label `dispatch`, `go test` exit 0, stdout
`ok github.com/openexec/openexec/pkg/execution`. I.e. with
`case "search_files":` (workspace_tools.go:202) mutated to
`case "unavailable_search":`, `TestWorkspaceSearchFindsDefinitionAndCompletesTriage`
still **passed**, so the control is not a real negative control at this head.
(The script prints `dispatch EXPECTED RUNTIME FAILURE` only when the test fails.)

## 5. Call path the selected test takes: `executeToolScan`, not `ExecuteTool`
Test: `pkg/execution/workspace_search_test.go:56`.
1. It builds the provider with `ToolExecutor: NewWorkspaceToolExecutor()`
   (a `*WorkspaceToolExecutor`) and calls `provider.Execute`.
2. `api_provider.go:543-544`: the executor satisfies `scanReportingExecutor`
   (interface, `workspace_tools.go:174-176`), so the provider calls
   `scanner.executeToolScan(...)`, not `ExecuteTool` (the `else` at line 545-546).
3. `workspace_tools.go:182-191`: `executeToolScan` sees `request.Name ==
   "search_files"`, so it skips the `request.Name != "search_files"` branch
   (183-186, the only place that delegates to `ExecuteTool`), validates access
   (187) and returns `e.searchFilesWithProvenance` (190).
4. `ExecuteTool`'s switch (`workspace_tools.go:193-208`, `case "search_files"` at
   202) is never reached, which is why mutating it leaves the test green.

`composite_tools.go:109-123` (`CompositeToolExecutor.executeToolScan`, delegating
at line 118) is the composite wrapper; it is **not** on the test's path, since
the test uses the bare workspace executor. `ExecuteTool` is called directly in
other tests (e.g. `workspace_search_test.go` ~line 110), not in this one.
Consistent with test line 56 and with the observed exit 0.

## 6. Cleanliness
Temporary checkout: `.tmp-a149` (git worktree, detached at `a149fceb`) inside this
task worktree. `git status` there was empty for `pkg/` and `scripts/` after the
run (the script works from overlays). The directory was deleted with `rm -rf`;
`git worktree prune` was refused by the sandbox, so a stale
`.git/worktrees/.tmp-a149` admin entry may remain until pruned (not verified).
`git status --short` in the task worktree is clean apart from this document.
