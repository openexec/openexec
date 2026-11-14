# `dispatch` control at a149fceb: call path and observed output

Read-and-record only. No product file, test or script was changed.
All file:line citations are from the tree at commit `a149fceb` (merge of
origin/main into `agent-console/task-17da07171611-…`), **not** from current main,
where the same names still exist at different lines (`executeToolScan`:
`workspace_tools.go:238`, `composite_tools.go:125`, `api_provider.go:581`; an
earlier version of this record wrongly said it no longer exists). `a149fceb` is
not an ancestor of main `15791724`; it was read from this checkout's object store.

## 1. Raw job 126662911673 — still UNPROVABLE by this pass, with reasons
- `grep -rlI 126662911673 /mnt/data1/projects/owner-model` (job runner project):
  no file contains the id (command ran, empty output).
- The live Agent Console data directory is `/home/perttu/.agent-console` (an
  earlier version of this record searched `/mnt/data1/projects/agent-console/data`,
  the wrong place). This pass could NOT search it: the sandbox refused `grep` and
  `run_command` on that path ("outside the task's checkout"). `Read` reaches
  `state.json` but it is one ~MB-scale line that cannot be read or searched
  here. A reviewer reported the id appears in `state.json` and in
  `watch-snapshot-*` files only as task/session titles quoting it, not as a raw
  log; this pass could not confirm or refute that. Directories `checks`, `chat`,
  `deploy-evidence` there were not searched by anyone.
- Conclusion: the raw log contents are unprovable from this sandbox. This is a
  limit of access, not a finding that the log does not exist.

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
`git worktree prune` and `git worktree list` are refused by this sandbox, so the
removal of the `.git/worktrees/.tmp-a149` admin entry could not be shown; a
reviewer saw it listed as `prunable`. Pruning it is unprovable here and harmless
(it needs `git worktree prune` by the owner or console). This pass changed only
this document; no `pkg/` or `scripts/` file.
