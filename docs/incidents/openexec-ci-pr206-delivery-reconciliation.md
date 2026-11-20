# PR #206 delivery reconciliation (openexec-ci incident 4081)

Recorded 2026-11-08 against `origin/main` `e93cbd6a`. Every GitHub fact below
was read with `gh` on 2026-11-08; nothing here is inferred from CI colour.
This record changes no code, does not touch PR #206, and grants no authority.

## Retained candidate (PR #206)

| Fact | Observed |
| --- | --- |
| State / base | OPEN, base `main`, not draft, `mergeable: MERGEABLE`, `mergeStateStatus: CLEAN`, last updated 2026-11-04T15:54:12Z |
| Head | `89c8311497cc7d15da7c2fb8e7fc33173dec84ad`, the required head. One commit (`docs: investigate openexec-ci US-1000 T-US-1000-001`); no commit has been added since |
| Title / body | "Repair signal openexec-ci" / "Deliver retained implementation for accepted Goal 2836bee8…. The exact head was independently reviewed before publication; hosted checks remain required and this publication grants no promotion authority." |
| Diff | Only `docs/incidents/openexec-ci-4081.md`, +55/-0 (`git diff origin/main...89c83114 --stat`); merge-base `d4e65423`, so the PR is behind main but GitHub reports it clean |
| GitHub reviews | `reviews: []`, `comments: []` |
| Hosted run 41862455768 | success on that exact head: `build (ubuntu-latest, 1.25)` 2026-11-04T16:26:40Z, `build (macos-latest, 1.25)` 2026-11-07T19:44:17Z |

**Review 6116e3bd is not attached on GitHub.** The PR body says only that the
head "was independently reviewed"; it does not cite 6116e3bd and GitHub holds
no review object. The review is therefore recorded in the console's own
records (outside this repository), and is **unverified from here**. It was not
re-created and nothing was posted to the PR.

## Consistency with incident 4265 (on main)

No conflict. Both notes report the same `JSON observation failed`, exit -1,
no timeout or truncation, with no underlying error or response body. Both say
the cause is undetermined, that the repository shows no supporting fix, and
that a healthy reading cannot be claimed. 4081 is the 2026-11-04T15:31Z
reading; 4265 is the 2026-11-05 19:19Z reading. They overlap only in narrative
(both cite #216/#217-era CI changes and the absent `scripts/observe/_signal.py`);
merging 4081 would add a second note for an earlier reading, not a
contradiction. 4081 predates the `internal/channeldecl` guard that 4265's
delivery (#221) added, so 4081 does not mention it.

## Delivery route and authority boundaries

Existing route for a reviewed PR in this repository (nothing new is proposed):

1. **Gate**: the host `job_runner.py start` gate over the PR head. Runnable by
   the lane. Hosted CI must also be green on the exact head (it is, per above).
2. **pr-verify**: verifies the PR's exact head, diff and review. Runnable by the
   lane. It needs review 6116e3bd to be resolvable; see the gap above.
3. **Merge**: `gh pr merge 206 --match-head-commit 89c83114…` (fails closed if
   the head moves). **Requires separate promotion authority.**
4. **Deploy**: `deploy.sh` through `job_runner`. **Requires separate
   promotion authority**, and is moot for a docs-only note.

Where it stops: the accepted Goal 2836bee85e83309274239a7bceda7424 forbids
merge and deployment. Decision bcdff55d ("Hand remaining delivery to the lane",
applied 2026-11-07T20:06:55Z) is a delivery handoff, not an override of that
boundary. The owner answered the follow-up on 2026-11-08 with "Keep merge
forbidden". Steps 1 and 2 are within the lane; **steps 3 and 4 are not**, and
need a new explicit owner promotion decision. No allowance reset, bypass,
duplicate PR or push to #206 was made.

## Goal Stop and accounting

This pass changed no Goal, Stop or accounting record and has no read access to
them from this checkout. Before reference: Goal 2836bee8…, PR #206 head
`89c83114`, decision bcdff55d appliedAt 2026-11-07T20:06:55Z. After this pass
the only artifact is this file on the task branch. A byte-level before/after
comparison of the Stop and accounting rows is **unproven here** and must be
read from the console's store.

## Current `openexec-ci` signal health: unverified

Not healthy, not failing, not observable: **unverified**. The authoritative
source is the console's observer reading for channel `openexec-ci`
(declared in `.agent-console/channels.yaml`, schedule 10 min, timeout 10 s,
`OBSERVE_CI_TOKEN`). This sandbox has no console observer reading, no
`scripts/observe/_signal.py` (absent from this checkout and from
`/mnt/data1/projects/openexec/scripts`), and no resolved repository/branch or
token. Missing: a timestamped, sanitized observer response and the deployed
observer revision. Hosted run 41862455768 is **not** evidence of signal
health: it shows the workflow passed, not that the observer reads it.

## Next step

One step, owner: the **operator** (owner of the console/observer environment)
performs the observer diagnostics already listed in incident 4265: obtain the
incident-time resolved repository and branch, the deployed observer revision,
an authenticated sanitized CI response and the matching Actions run, then
record the live `openexec-ci` reading. Merging #206 is deliberately not the
next step: it is forbidden for now, and a docs note would not repair the signal.

## Refresh 2026-11-13 (retained-entry reconciliation)

Read-only. No merge, deploy, new review, allowance change or push to #206.

- **PR #206, re-read with `gh pr view 206` on 2026-11-13**: state OPEN, head
  `89c8311497cc7d15da7c2fb8e7fc33173dec84ad`, one commit, `updatedAt`
  2026-11-04T15:54:12Z (unchanged since publication, so nothing was pushed).
  Body is exactly the original text quoted above. `reviews: []`, `comments: []`.
- **Review 6116e3bd**: GitHub holds no review object and the body does not cite
  the id, so it is not attached on GitHub. The authoritative record is the
  console's review store, which this lane pass cannot read (the console refused
  filesystem access outside the checkout and `observe_current_outcome`
  refused). Exactly what is missing: the console review record for 6116e3bd
  (reviewed head, verdict, timestamp). Status stays **unverified from here**;
  it is not recreated.
- **Goal Stop and accounting before/after**: the before reference is Goal
  2836bee8, head `89c83114`, decision bcdff55d appliedAt 2026-11-07T20:06:55Z.
  After reference: this pass only edits this file on the task branch; it holds
  no write path to Goal, Stop or accounting records. The row-level comparison
  needs the console store and is **unproven here**.
- **Observer criterion**: delivered by PR #227 at `3a4d8c48`
  (`openexec-ci-signal-health-2026-11-11.md`); cited, not re-derived.
- **Next step (unchanged)**: operator runs the incident 4265 observer
  diagnostics; additionally the console owner can attach the 6116e3bd record
  and Stop/accounting rows to this task.
