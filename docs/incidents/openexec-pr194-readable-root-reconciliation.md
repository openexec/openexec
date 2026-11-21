# PR #194 (native readable-root identity) reconciliation record

Evidence record only. No source is changed. PR #194 is not readied, closed,
merged, rebased or duplicated; Stop, holds and accounting are untouched. Nothing
here declares the parent task delivered.

## 1. Exact state (time-bound)

Retrieved **2026-11-21T02:26Z–02:40Z UTC** (this task pass). Sources: GitHub via
`gh pr view <n> -R openexec/openexec --json …` (read-only, run through the
console's `run_command`), and read-only `git` in this checkout.

| Item | Value | Source |
| --- | --- | --- |
| PR #194 | `OPEN`, `isDraft: true`, `mergeable: CONFLICTING`, base `main`, `mergedAt: null`, `updatedAt: 2026-11-01T21:36:51Z`, title "Pin native API readable roots during each run" | `gh pr view 194 --json state,isDraft,mergeable,baseRefName,mergedAt,updatedAt,title` |
| PR #194 head | `8b5333c243cf52f09031e998ad19810c2f9e076a` — **identical** to the head named in the task; GitHub shows no different head | `headRefOid` |
| PR #194 commits | `f9f5f06d6f9905e94fb6ea7e210dc7f6aa45e127` "fix: pin API readable roots for each execution"; `8b5333c243cf52f09031e998ad19810c2f9e076a` "fix: bind readable root identities across execution protocol"; merge base with main `776529df6f0382dcd2a1311cac87e936df4e46c3` | `gh pr view 194 --json commits`; `git merge-base 8b5333c2 origin/main` |
| main | `6a217d09200f30202f6eac9622d1f6f3f811b4a7` — identical to the expected `6a217d09…` | console fetch at run start; `git rev-parse origin/main` |
| PR #205 | `MERGED` 2026-11-14T09:14:29Z, merge commit `cb53f74239869fd4fb720122f64c9c58b93367c5`, PR head `7cadaed3da24f94d5cb5662a4b75083d93968592`, title "Reconcile the retained native read-root identity candidate before delivery" | `gh pr view 205 --json state,mergedAt,mergeCommit,headRefOid,title` |

These values are a snapshot; re-run the commands above before relying on them later.

## 2. Conflict files

`git merge-tree --write-tree --name-only origin/main 8b5333c2` reports CONFLICT (content) in exactly:

1. `internal/cli/execution_stdio_test.go`
2. `pkg/execution/workspace_search.go`
3. `pkg/execution/workspace_tools.go`

Auto-merged cleanly: `api_provider.go`, `composite_tools.go`, `execution.go`
(and `internal/cli/execution_stdio.go` is not reported as a conflict).

## 3. Hunk classification

Method: (a) `git diff --stat 776529df 8b5333c2` lists PR #194's 10 files;
(b) `git range-diff 776529df..8b5333c2 14c85e49..cb53f742` shows #205 (parent
`14c85e49`) carries the same two #194 commits as one squash plus additions;
(c) `git diff 8b5333c2 origin/main -- <file>` (and `-U0`, lines starting `-`,
i.e. PR-side lines absent on main) per file, to find any PR line not on main;
(d) `git grep` on `origin/main` for the added symbols. Classes: **delivered by
#205** (`cb53f742`), **superseded** (replaced on main by a decided alternative),
**missing** (on neither).

| PR #194 file / hunk | Class | Evidence |
| --- | --- | --- |
| `pkg/execution/root_identity_unix.go` (new, `linux\|\|darwin`) | delivered by #205 | `git diff 8b5333c2 origin/main` for the file is empty |
| `pkg/execution/root_identity_unsupported.go` (new, `!linux&&!darwin`, returns unsupported) | delivered by #205 | empty diff vs main |
| `pkg/execution/workspace_root_identity_test.go` (new, 151 lines) | delivered by #205 | empty diff vs main |
| `pkg/execution/execution.go`: `ReadableRootIdentities` field and `ReadableRootIdentity` type | delivered by #205 | no PR-side lines absent on main (`-U0` shows none) |
| `pkg/execution/composite_tools.go`: `pinReadableRoots` forwarding | delivered by #205 | main line 79; no PR-side lines absent |
| `pkg/execution/api_provider.go`: pinning in `Execute`, `BoundReadableRootsAvailable`, "cannot bind"/"without roots" refusals | delivered by #205 | main lines 169-211. PR-side lines absent on main are only main's unrelated refactor (step loop, token budget, empty-completion recovery) — none are pin lines |
| `internal/cli/execution_stdio.go`: v4 constant, version switch, identity-count refusals, provider capability refusal | delivered by #205 | main lines 31, 260-270, 312-314. PR-side lines absent on main are context-policy/flag edits unrelated to pinning |
| `internal/cli/execution_stdio_test.go`: `TestExecutionProtocolBoundRootsRequiresNewVersionAndProvider` (conflict) | delivered by #205, superset | main adds six rows (v1/v2 without identities, v1/v2 cannot carry identity, v4 count mismatch, future v5) |
| `pkg/execution/workspace_tools.go`: `pinReadableRoots`, identity compare, duplicate/count refusals, pinned-root use (conflict) | delivered by #205 | main line 76 onward; the conflict is against main's separate `executeToolScan` addition |
| `pkg/execution/workspace_tools.go`: `named, nameErr := os.Lstat(path)` + `os.SameFile(opened, named)` name comparison inside `pinReadableRoots` | **superseded** | removed in #205 ("drop redundant SameFile guard", #205 commit message); main now checks only `root.Stat(".")` error (`git diff 8b5333c2 origin/main`, hunk at line ~105). The caller identity compare plus canonical-path refusal remain; `TestPinReadableRootsRefusesSymlinkedParent` covers the symlinked-parent case the guard also touched |
| `pkg/execution/workspace_search.go`: `withinRoots` → `e.withinRoots` (conflict) | delivered by #205 | main line 58; the conflict is against main's `searchFilesWithProvenance` rework |
| Proof of the `!linux && !darwin` refusal | **missing** | not in #194 or main: `root_identity_unsupported.go` is not compiled on Linux and both identity test files are tagged `linux\|\|darwin`; `git grep` finds no test exercising `ReadableRootIdentitySupported() == false` |

No hunk was unclassifiable. Main additionally has
`workspace_root_identity_paths_test.go` (from #205), which #194 lacks.

## 4. Reviews and evidence (retrievable references)

- **#194 attached review:** none retrievable. Re-fetched 2026-11-21T02:30Z with
  `gh pr view 194 -R openexec/openexec --json body,reviews,comments,latestReviews,headRefOid,state`:
  `reviews: []`, `latestReviews: []`, `comments: []`, head still
  `8b5333c243cf52f09031e998ad19810c2f9e076a`, state OPEN. GitHub holds no review
  for this head. The PR body itself says exact-head review, full Go tests,
  `make test` and live verification "remain required".
- **Console review of #194 (parent task `0e65901e02d33f7805bf907a33715538`):**
  not retrieved. This pass has no read access to the console's review store
  (reads under `/home/perttu/.agent-console` are refused for this lane task, and
  no repository file or commit message names a parent-task review ID:
  `git grep` for `0e65901e` finds nothing). Its ID and contents are therefore
  **unknown here** and are not stated. Gap: whoever holds console access should
  add the review ID and its retrieval reference.
- **Reviews of this follow-up (not of #194):** `9b2839bf3457367bc2f44f0a9ae794bd`
  and `c3c040f883700e0cf91a56065c4d9263` are cited in this task's contract as
  reviews of an earlier candidate of this record (branch identical to main, record
  missing). They are not reviews of #194 and are not evidence about its code;
  their text is not reproduced here.
- **#205 review:** `reviews: []`, `comments: []`. **Evidence:** body (retrievable
  via `gh pr view 205 --json body`) records base-red/restored-green runs of
  `go test ./pkg/execution/ -run 'Identity|PinReadable|PinnedReadable|Bound'` and
  `go test ./pkg/execution/ ./internal/cli/`, and states it did not run the full
  gate and that per-guard removal-red was not re-run. Check rollup, workflow CI
  run 43578532323: `build (ubuntu-latest, 1.25)` SUCCESS (job 132398043653);
  `build (macos-latest, 1.25)` **SKIPPED** (job 132399801509). The retrievable
  hosted evidence is therefore a Linux pass only.
- #205's body states the original #194 was never updated by that lane and that
  its own PR "is not delivery of #194".

## 5. Discarded tasks

Branches (local and on origin):
`agent-console/task-2ef0beb6a771-split-each-listed-fail-safe-case-has-a-t`
tip `83466b7e0fd06012f4677cff5a57e2e16357e93d`;
`agent-console/task-16c3f057db4c-split-each-listed-fail-safe-case-has-a-t`
tip `95f9eea862dcadb73d212c88332fb89f5cc95662`. Neither is merged to main.

**What they show:** attempted scope. Both add interleave, postcheck,
unsupported-platform and no-`/proc` tests under `pkg/execution`, a
`root_walk_unix.go`, CLI guard-removal and legacy-runtime refusal tests, and a
rewrite of `workspace_tools.go` (≈135 / ≈175 lines changed); 16c3f057 also adds
`root_watch_{linux,darwin}.go`. Commits include "refuse a pinned-root swap
between the name check and the descriptor operation" (`f5c48cf5`) and "refuse a
pinned-root swap-and-restore around a descriptor read" (`8cf627ee`). Their
`git diff --stat` against main is dominated by deletions because the branches
are stale relative to main; the deletions are not their intent.

**What they do not establish:** no reviewed, merged or hosted-verified security
boundary. This record found no retrievable passing review or hosted check for
either tip, did not run their tests, and adopts none of their code. Their
unsupported-platform tests are a lead for the missing proof, not evidence that
it exists on main or that the boundary holds under swap races.

## 6. Preserved, and not established

- Parent PR #194 identity (draft, `8b5333c2`, conflicting), hold, Stop and
  accounting: unchanged.
- Route decision, implementation and platform tests belong to the parent task;
  this record only supplies its inputs.
- Not established: macOS runtime behaviour (#205's macOS leg was skipped); that
  the unsupported-platform refusal works (no test exists on main).
