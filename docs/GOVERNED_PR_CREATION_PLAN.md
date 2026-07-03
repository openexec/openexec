# Governed PR Creation — Plan

## Objective

OpenExec opens **real** pull requests as part of the governed flow, at a
granularity that matches the work source:

- **GitHub → task-level PRs (trunk-based).** Pick up one task at a time, always
  open a PR. Small, frequent, independently reviewable/mergeable PRs off `main`.
- **Jira → story-level PRs.** Pick up a story, execute its tasks, open **one** PR
  for the story that bundles those task commits. Larger, story-scoped units that
  mirror how Jira teams plan and review.

The differentiator is the **PR unit**: `task` for GitHub, `story` for Jira. It
drives branch naming, when the PR opens, and which commits it bundles.

## Status

- **DONE — core creation capability.** `openexec governance work open-pr
  <change-id>` pushes the change's feature branch and opens a real PR
  (`git push` via `git.Client.PushBranch` + `gh pr create` run in the project
  dir), then records it and posts the governance assessment (reuses `RecordPR` +
  `assessAndPostToPR`). This is **change-level** — the trunk-based GitHub case
  when an issue maps to one change. `record-pr` still links an externally-opened
  PR; `open-pr` opens one.
- **TODO** — the task/story PR-unit routing below.

## The PR unit (source_type → granularity)

`change_records.source_type` already distinguishes `github_issue`, `jira_issue`,
`manual`. Route the PR unit off it:

| Source | PR unit | Branch | Opens when | Bundles |
|--------|---------|--------|------------|---------|
| github_issue | task | `gov/<change>/<task-id>` | task implement completes | that task's commits |
| jira_issue | story | `gov/<change>/<story-id>` | all story tasks complete | the story's task commits |
| manual | change | `gov/<change>` | change execute completes | all change commits (today's `open-pr`) |

Non-goal: mixing units in one PR. A task PR is one task; a story PR is one story.

## Phases

### P1 — creation capability (DONE)
Change-level `open-pr`: push + `gh pr create` + record + assess. Refuses to open
from the base branch. `--branch`/`--base` overrides; base defaults to the
project's `base_branch`.

### P2 — task-level PRs for GitHub (trunk-based)
- Branch per task (`gov/<change>/<task-id>`), created from `base_branch`.
- After a task's implement stage commits, `open-pr` for **that task** (title/body
  from the task; link the parent issue; `Closes #N` only on the last task of the
  change, or never — operator policy).
- One issue can therefore yield several small PRs. Record each PR URL on its task
  (`git_pr_url` is already per-task).
- Acceptance: running two tasks of one change opens two independent PRs off
  `main`, each with its own assessment comment.

### P3 — story-level PRs for Jira
- Branch per story (`gov/<change>/<story-id>`); each task in the story commits to
  it in dependency order.
- Open **one** PR for the story once its tasks complete; body lists the tasks and
  their evidence.
- Requires the Jira connector (see DELIVERY_GOVERNANCE_PLAN P6) so a story is a
  first-class unit with write-back.
- Acceptance: a 3-task story produces exactly one PR containing three commits.

### P4 — auto-open in the execute flow
- After execute produces commits, auto-invoke the correct unit's `open-pr` (gated
  by an operator/config toggle so it stays opt-in until trusted).
- Close the manual gap end to end: intake → triage → review → approve → execute →
  **PR opened automatically** → assessment posted → operator merges.

## Open decisions
1. `Closes #N` semantics for multi-PR GitHub issues — close the issue on the last
   PR, on all, or never (operator policy).
2. Draft vs ready PRs — open as draft until the hitl validation story passes?
3. Auto-open (P4) default: opt-in per project vs per risk tier.
4. Branch cleanup: delete the feature branch on merge (reuse release automerge
   plumbing) or leave to GitHub's auto-delete.
