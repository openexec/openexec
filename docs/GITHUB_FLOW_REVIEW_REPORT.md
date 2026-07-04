# OpenExec GitHub Delivery Flow — Problem Report for External Review

**Date:** 2026-07-04
**Prepared for:** external reviewer
**Scope:** the "ticket → PR" GitHub governance flow (label a GitHub issue → AI
triages, plans, reviews, asks for clarification if needed, implements, opens a
PR for human review/merge). Verified live against a real repo
(`perttu/fotoyks-app`, non-production, rollback-safe).

This report documents the problems found while trying to make the flow run
**autonomously end-to-end** (unattended, cron-driven, one job at a time), our
diagnosis of each, the fix applied, and the open questions we'd like a second
opinion on. It is deliberately blunt about what was broken.

---

## 1. The intended flow

```
Human labels a GitHub issue "AI Fix" (+ optional clarifying comments)
  → import as a governance ChangeRecord
  → deep triage into vertical-slice stories/tasks
  → AI plan review (separation of duties)
       → if the plan has genuine unknowns: post questions on the issue, PARK
  → approve (an OPERATOR-AUTHENTICATED autopilot auto-approves low-risk;
    medium/high need an explicit human /openexec approve — approval always
    requires an operator session, so labelling alone never approves)
  → execute the tasks (AI agent writes code)
  → commit + open a PR
  → human reviews / merges the PR (never auto-merged)
```

The human's job is meant to be: **label, answer occasional clarifications,
review/merge PRs.** Everything else autonomous.

---

## 2. Problems found (most severe first)

### P1 — Autonomous execution silently produced nothing (hitl gating)
**Symptom:** every attempt to execute an approved change produced **0 commits**;
the daemon logged *"22 task(s) held back … requires a human in the loop."*
**Root cause:** the CLI execution path (`openexec run`, `governance work
execute`) dispatches to the daemon's **batch** executor (`POST
/api/v1/runs:execute` → `ExecuteTasks`), which **holds back every hitl task and
its dependents**. The planner **always injects a hitl "study" story** as the
first slice, and the implement tasks depend on it — so the whole change is held
and nothing runs. This is by design for interactive use, but it makes
unattended execution impossible on that path.
**Fix:** run tasks through the **ungated single-task endpoint** (`POST
/api/v1/runs/{taskID}/start` → `Manager.Start`, which the batch hitl gate does
not touch — the same path `openexec chat` uses). Verified: a task ran fully and
the agent produced a real 10 KB `docs/ARCHITECTURE.md`.
**Reviewer question:** is bypassing the hitl gate for autonomous work the right
call, or should the planner emit *all-afk* plans for AI-Fix-labelled issues (no
hitl study), with the study/QA surfaced as a PR checklist instead?

### P2 — Agent work was never committed (config flag + unreliable agent)
**Symptom:** even after a task ran and wrote files, the working tree showed the
files as **untracked** — no commit — so there was nothing to open a PR from.
**Root cause (two layers):**
1. `internal/tools/safe_commit_tool.go` **refuses to commit** unless
   `git_commit_enabled: true` in the project config, and the default was
   effectively **false** (unset). So the commit tool aborted.
2. Even with the flag on, the **agent did not reliably call** `safe_commit` — it
   wrote the files and moved on. Relying on the model's discretion to commit is
   not dependable.
**Fix:** (a) `git_commit_enabled` now defaults **true** (nil-default-true;
explicit `false` disables) — local commits never push or PR, so default-on is
safe; (b) the autopilot **commits deterministically** after the pipeline
(`git add -A && git commit`) rather than trusting the agent.
**Reviewer question:** is deterministic post-run commit acceptable, or should the
blueprint have an explicit "commit" stage that the pipeline (not the model)
owns?

### P3 — Executor self-invocation broke on a relative launch path
**Symptom:** every governed task failed with `fork/exec ./bin/openexec: no such
file or directory`.
**Root cause:** the executor forked `os.Args[0]` with the child's working
directory set to the *project* dir, so a relative launch (`./bin/openexec`)
resolved against the wrong directory.
**Fix:** resolve via `os.Executable()` (always absolute).
**Note:** only surfaced because we launched from source; an installed binary on
PATH hid it. Worth a reviewer eye on other `os.Args[0]` assumptions.

### P4 — Clarifications never reached the GitHub issue
**Symptom:** when the AI review found genuine unknowns, the concerns were
recorded in the governance DB but **not posted to the issue**, so a human would
never see them without using the CLI.
**Fix:** on a `request_changes` verdict for a github-sourced change, the service
now posts the concerns (+ missing acceptance criteria/tests) as an issue comment
and applies the `ai:changes-requested` label. A human replies `/openexec answer
<text>`; a poll ingests it, records it in the audit trail, and folds it into the
change brief for re-planning. **Verified live** on issue #5.

### P5 — There was no way to open a PR autonomously
**Symptom:** the flow could only *record* an externally-opened PR
(`record-pr --url`); it never ran `gh pr create`.
**Fix:** added `governance work open-pr` (push branch + `gh pr create` + post the
governance assessment). **Verified live** — produced real PR #6.

### P6 — The autopilot stopped right after triage (state-machine mismatch)
**Symptom:** the autonomous loop ran triage then reported "no actionable work".
**Root cause:** deep triage sets the change straight to **`plan_ready`**
(`triage_deep.go`), but the autopilot's selector treated `plan_ready` as a
"parked" state and skipped it.
**Fix:** `plan_ready` now maps to the "review" action (review-then-approve).
**Verified:** the autopilot then advanced through review autonomously and parked
on a **legitimate** clarification (the reviewer noticed the README already had a
tagline — adding one would be redundant).

### P7 — Daemon lifecycle: pile-up
**Symptom:** the CLI starts a **new daemon per run** and doesn't reliably reuse
one — three daemons accreted (ports 8080/8081/8765) during testing.
**Status:** not fixed. The autopilot currently requires a single already-running
daemon and errors clearly otherwise, deferring lifecycle to cron setup. A
reviewer should weigh in on proper single-daemon management (or a daemonless
single-task execution mode).

---

## 3. What is verified working

- Import + deep triage of a labelled GitHub issue into vertical-slice stories.
- **AI plan review is genuinely high quality** — it caught a real **GDPR /
  ePrivacy** gap (a consent banner that stored a choice but never blocked
  pre-consent cookies), grep-based tests that pass even if a component is
  commented out, and a redundant-README-tagline. This is the strongest part of
  the system and worth the reviewer's attention as a positive.
- The clarification round-trip on GitHub (post questions → `/openexec answer` →
  audit + re-plan). Verified on issue #5.
- Ungated single-task execution actually runs and produces real files.
- Deterministic commit + `open-pr` → real PR #6 with the assessment posted.
- The autopilot single-slot selector (an in-flight change reports "slot busy").
- Low-risk auto-approve wiring (fires when the review approves; both test issues
  instead triggered legitimate clarifications, so a clean auto-approve→PR run was
  not captured — the mechanism is wired and each hop is individually verified).

---

## 4. Open questions we'd most like reviewed

1. **hitl vs autonomy (P1):** bypass the gate, or generate all-afk plans for
   labelled work? What is the right human checkpoint — the PR, or the study?
2. **Commit ownership (P2):** deterministic post-run commit vs a pipeline commit
   stage vs enforcing the agent to commit. Also: what git identity should
   autonomous commits use? (We used `openexec@local` as a placeholder.)
3. **Approval policy:** an operator-authenticated autopilot auto-approves only
   `risk_profile: low` (the shelled `work approve` requires an operator session —
   labelling alone never approves); medium/high stay human. Is risk-tier the
   right axis, given the operator opted in by running the autopilot and a human
   still gates at merge?
4. **AI review non-determinism:** the reviewer sometimes approves, sometimes
   requests changes on similar inputs. Acceptable variance, or does the reviewer
   prompt need tightening/temperature control?
5. **Release coupling:** the governed `work execute` requires the change to be
   attached to an approved release; the autopilot's ungated path sidesteps this.
   Is losing that release linkage in the autonomous path a governance concern?
6. **Blast radius:** a cron across 10+ repos auto-approving low-risk and opening
   PRs. Is "never auto-merge + label-gated + single-slot + PR-for-review" a
   sufficient safety envelope?

---

## 5. Net assessment

The flow's **judgment layer (triage + review + clarification) works well and is
the differentiator.** The problems were concentrated in the **execution and
plumbing** layer — hitl gating, a disabled commit tool, an unreliable
agent-commit, a relative-path fork, no PR-creation, and a state-machine mismatch
— each now understood and fixed or worked around, and each verified live. The
remaining judgement calls (P1/P2 architecture, daemon lifecycle, approval policy)
are where an external opinion would be most valuable before this runs unattended
across many repositories.

---

## 6. External review — findings addressed (2026-07-04)

An external reviewer raised four findings; all four are resolved:

1. **High — failed runs could still produce a PR.** `waitRunTerminal` treated
   `failed`/`error`/`paused` as proceed-able terminal states and the code
   committed regardless. **Fixed:** only `complete`/`completed` proceeds; any
   other terminal status aborts the change **before** commit/PR. (Follow-up:
   record a structured failure event + block the change from immediate re-pickup —
   today a failed change stays approved and may retry next tick; the critical
   "no PR from a broken run" is fixed.)
2. **Medium — "labeling authorizes implementation" was inaccurate.** The autopilot
   shells `work approve`, which requires an operator session (`ApproveChange` →
   `errNotOperator`). **Fixed (doc/comment):** it is an *operator-authenticated
   autopilot auto-approving low-risk*; labelling alone never approves.
3. **Medium — clarification comment gave the wrong recovery path.** It said "reply
   here" / `/openexec revise`; only `/openexec answer <text>` is ingested (a plain
   reply is ignored). **Fixed:** the posted comment now explicitly instructs
   `/openexec answer <your decisions>`.
4. **Medium/strategic — "single-slot" only meant one *executing* change.** Open AI
   PRs did not count. **Fixed:** added `--max-open-prs` (default 3) — the tick
   pauses new work when that many `pr_open` AI changes exist, so an unattended
   cron cannot pile up unreviewed PRs.
