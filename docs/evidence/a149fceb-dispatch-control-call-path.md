# `dispatch` control at a149fceb: call path and observed output

Read-and-record only. No product file, test or script was changed.
All file:line citations are from the tree at commit `a149fceb` (merge of
origin/main into `agent-console/task-17da07171611-…`), **not** from current main,
where the same names still exist at different lines (`executeToolScan`:
`workspace_tools.go:238`, `composite_tools.go:125`, `api_provider.go:581`; an
earlier version of this record wrongly said it no longer exists). `a149fceb` is
not an ancestor of main `15791724`; it was read from this checkout's object store.

## 1. Raw job 126662911673 — UNAVAILABLE (no raw log retained)
Basis: a search by the supervisor outside the lane sandbox (addendum 1, ~13:55Z
2026-11-14) of the live data directory `/home/perttu/.agent-console`. This pass
did not repeat it (the lane sandbox refuses `grep`/`run_command` there); it is
the supervisor's reported result, not something this pass observed.
- `checks/`, `chat/` and `deploy-evidence/`: no file mentions the id.
- `state.json`: mentions it once, as a task/session title quoting the id, not a log.
- `watch-snapshot-*`: title references only (matches the reviewer's reading).
- `grep -rlI 126662911673 /mnt/data1/projects/owner-model`: no file (this pass ran it).
- Earlier versions of this record searched `/mnt/data1/projects/agent-console/data`,
  the wrong place; that search is void.
- Conclusion: no raw log for job 126662911673 is retained. Its contents were not
  read and are not invented here.

## 2. Material actually read (via `inspect_repository`, local branches)
- `a149fceb` (17da source merge) and parents `0bbd3f2d`, `d4e65423`; at that
  commit: `scripts/verify-workspace-search.py`, `pkg/execution/workspace_search_test.go`,
  `workspace_tools.go`, `composite_tools.go`, `api_provider.go`.
- Current 17da branch `agent-console/task-17da07171611-…` at `88906ebc`: commit
  touches `docs/NATIVE_SEARCH_CONTINUATION.md`, `api_provider.go`,
  `context_continuation_test.go`, `context_recovery.go` (+110/-4).
- Prior reproduction record branch `agent-console/task-a4c4cea97ece-…` at
  `054bae19` (merge of origin/main; no new reproduction record in that merge).
- Safety child `agent-console/task-bde7666d7984-split-system-and-owner-instructions-file`
  at `17ff2ff9`: touches only `docs/OMISSION_ELIGIBILITY_DELTA.md` (+21/-8,
  "record complete code and test line delta against PR204 baseline"). Its
  relation to "PR207" is not stated in the commit; the PR207 number is
  unprovable here (no PR metadata available).

## 3. Pre-edit sequence
Recoverable from retained git history only: `0bbd3f2d` (17da feature commit) →
`a149fceb` (merge with #203) → the run in section 4. The read/edit order of the
original sessions, tool calls and the job log's events are **unprovable** (see 1).

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
Temporary checkout `.tmp-a149` (detached at `a149fceb`) showed no changes under
`pkg/` or `scripts/` after the run; its directory was deleted with `rm -rf`.
Its stale admin entry was still listed as `prunable` in this worktree. This pass
ran:

```
$ git worktree prune -v
Removing worktrees/-tmp-a149: gitdir file points to non-existent location
$ git worktree list | grep -c a149
0
```

The supervisor also reports 0 matches for `a149` in the main checkout's
`git worktree list`. This pass changed only this document; no `pkg/` or
`scripts/` file.
