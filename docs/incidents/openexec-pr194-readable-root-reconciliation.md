# PR #194 (native readable-root identity) reconciliation record

Retrieved 2026-11-21T01:57Z with `gh pr view` (openexec/openexec) and read-only
`git` against this checkout. Records only; no source is changed and PR #194 is
not readied, closed, merged or duplicated. Holds, Stop and accounting are untouched.

## 1. Exact state

| Item | Value | Source |
| --- | --- | --- |
| PR #194 | OPEN, draft, mergeable = CONFLICTING, base `main`, updatedAt 2026-11-01T21:36:51Z | `gh pr view 194 --json state,isDraft,mergeable,updatedAt` |
| PR #194 head | `8b5333c243cf52f09031e998ad19810c2f9e076a` (matches the head named in the task) | `headRefOid` |
| PR #194 commits | `f9f5f06d6f9905e94fb6ea7e210dc7f6aa45e127` "fix: pin API readable roots for each execution"; `8b5333c2` "fix: bind readable root identities across execution protocol"; base `776529df` (#193) | `gh pr view 194 --json commits` |
| origin/main | `6a217d09200f30202f6eac9622d1f6f3f811b4a7` | console fetch at run start; `git log` |
| PR #205 | MERGED 2026-11-14T09:14:29Z, merge commit `cb53f74239869fd4fb720122f64c9c58b93367c5`, "Reconcile the retained native read-root identity candidate before delivery" | `gh pr view 205 --json state,mergedAt,mergeCommit,title` |

## 2. Conflicts (git merge-tree of #194 head into main)

`git merge-tree --write-tree --name-only HEAD 8b5333c2…` exits 1 with CONFLICT in:

1. `internal/cli/execution_stdio_test.go`
2. `pkg/execution/workspace_search.go`
3. `pkg/execution/workspace_tools.go`

Auto-merged cleanly: `internal/cli/execution_stdio.go`, `pkg/execution/api_provider.go`,
`composite_tools.go`, `execution.go`.

## 3. Hunk classification (PR #194 head vs main, `git diff 8b5333c2 HEAD`)

PR #194 touches 10 files (`git diff --stat 776529df 8b5333c2`).

| PR #194 file | Classification | Evidence |
| --- | --- | --- |
| `pkg/execution/root_identity_unix.go` (new) | delivered | no diff between PR head and main; main defines `ReadableRootIdentitySupported`/`ReadableRootIdentityOf` (`linux \|\| darwin`) |
| `pkg/execution/root_identity_unsupported.go` (new) | delivered | no diff; returns false / "readable root identity is unsupported" |
| `pkg/execution/workspace_root_identity_test.go` (new, `linux \|\| darwin`) | delivered | no diff between PR head and main |
| `pkg/execution/execution.go` | delivered | identity types present; only main-side additions (`ReasonContextCheckpoint`, `EventCheckpoint`) differ |
| `pkg/execution/composite_tools.go` | delivered | `pinReadableRoots` present on main (line 79); main adds only `executeToolScan` |
| `pkg/execution/api_provider.go` | delivered | capability check `ok && ReadableRootIdentitySupported()` on main (line 174); main has ~300 further lines of unrelated growth |
| `internal/cli/execution_stdio.go` | delivered | auto-merges; v4 handshake on main |
| `internal/cli/execution_stdio_test.go` | delivered, superset | main contains all PR cases plus v1/v2/future-version/count-mismatch cases; conflict is an add/add of adjacent test rows |
| `pkg/execution/workspace_tools.go` | delivered, with one intentional supersession | main's `pinReadableRoots` keeps the `root.Stat(".")` identity check but drops the PR's extra `os.Lstat` + `os.SameFile` name comparison (`git diff` hunk at line ~105). Task contract: do not change this decision. Main also adds `executeToolScan`; conflict is against that |
| `pkg/execution/workspace_search.go` | delivered / conflicting | PR one-line change vs main's `searchFilesWithProvenance` rework; main's side wins |

Missing on main (not in PR #194 either): a test of the `!linux && !darwin` refusal
(`root_identity_unsupported.go` is not compiled on Linux, and both identity test files are
tagged `linux || darwin`), and an interleave test for a swap between the identity check and
the open. Main additionally has `workspace_root_identity_paths_test.go` (bad identities,
symlinked parent, pinned read/list/search after swap, provider that cannot bind, composite
that cannot pin), which PR #194 does not have.

## 4. Reviews and evidence

- PR #194: `reviews: []`, `comments: []` in the `gh pr view` result. No attached review is
  retrievable from GitHub for this head. The PR body itself says exact-head independent
  review, full Go tests, `make test` and live verification "remain required".
- PR #205: `reviews: []`, `comments: []`. Its check rollup: CI `build (ubuntu-latest, 1.25)`
  SUCCESS (run 43578532323, job 132398043653); `build (macos-latest, 1.25)` SKIPPED
  (job 132399801509). So #205's retrievable evidence is a Linux pass only; macOS did not run.

## 5. Discarded task outcomes

Branches `agent-console/task-2ef0beb6a771-…` and `…task-16c3f057db4c-…` exist locally and on
origin. 16c3f057 history includes "refuse a pinned-root swap between the name check and the
descriptor operation" (f5c48cf5) and "swap-and-restore around a descriptor read" (8cf627ee).
Relative to main, 2ef0beb6 changes `workspace_tools.go` by ~135 lines, the unsupported file,
and both root-identity test files. These show attempted scope (interleave and swap-and-restore
hardening). Their outcomes were discarded, so they establish no reviewed or merged security
boundary, and their `workspace_tools.go` rewrite is not adopted.

## 6. Route

Same-PR continuation is technically possible (rebase #194 onto main keeping only the missing
tests) but the recommended route is: **close #194 as superseded by #205** (owner/lane action;
not done here). Reasons: every functional hunk is delivered; the one intentional difference is
a decided removal; the residual proof is (a) the unsupported-platform refusal, which cannot
execute on the Linux gate and so cannot be shown red/green by a Linux test, and (b) an
interleave test that needs the discarded `workspace_tools.go` hardening to be meaningful.
Rebasing wholesale would reintroduce the removed `SameFile` guard and fight main's
provenance/search changes. If the owner prefers to keep the PR identity, the continuation is
limited to those two tests via a fresh rebase and normal independent review.

## 7. Not established / post-deploy

- macOS runtime refusal of a swapped root: #205's macOS leg was SKIPPED; verify on a
  `build-macos` push-to-main run and a real macOS native execution.
- No test run for this record (docs only); no source change.
