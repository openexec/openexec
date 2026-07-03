# GitHub / Jira Delivery-Governance Flow

**Status:** implemented and live-verified for GitHub; Jira is a designed extension (not yet built).
**Branch:** `feat/release-governance` (proprietary layer — see [Two-layer model](#two-layer-model)).
**Audience:** external reviewer. This document describes the flow *as built*, marks what has been proven live, and lists the open questions we want reviewed.

---

## 1. Purpose

OpenExec turns a tracked work item (a GitHub issue today, a Jira issue tomorrow) into a reviewed, governed, audited code change — without letting an AI agent ship unapproved work. The governance layer decides **what may be built, by whom, and whether it may merge**; the execution engine does the code work. Every decision and every AI risk evaluation is recorded in a tamper-evident audit trail.

The design goal is a delivery pipeline a regulated org can defend: **audit trail + Jira-native governance + operability/merge gate**, compatible with trunk-based development and ISO 27001-style change control.

---

## 2. Two-layer model

| Layer | License | Role |
|-------|---------|------|
| **Runtime** (`internal/planner`, `internal/loop`, `internal/blueprint`, backlog, `openexec run`) | Open (MIT) | Plans and executes code changes. Knows nothing about governance. |
| **Governance** (`internal/governance/*`, `pkg/runtime` facade) | Proprietary | Decides what may run, records evidence + decisions, gates the merge, writes back to the tracker. Imports the runtime only through the `pkg/runtime` facade — no `internal/planner`/`internal/release` imports. |

The governance layer is a **control system**, not just a control plane: triaged tasks are created `hitl` (human-in-the-loop) so the ungoverned runtime scheduler never auto-builds them. Only the governance execute path un-holds a change's tasks, and only after the approval gate has passed.

---

## 3. The end-to-end flow

```
 Tracker item (GitHub issue / Jira issue)
        │  import  (idempotent)
        ▼
 ChangeRecord (candidate)              kind = feature|bug|ops|... derived from labels
        │  triage
        ├─ deep triage ─────► planner decomposition (vertical-slice stories/tasks, held hitl)
        └─ lightweight lane ► single task, no planner            } → plan_ready
        │  assessment  (impact + operability)   ── recorded in audit trail
        │  review (required by risk tier)       ── AI reviewers: bugbot, security_ai
        ▼
 human approval gate  (operator session)         changes_requested ⇄ plan_ready
        │  approve                                └─ no operator-override; revision is mandatory
        ▼
 approved_for_ai → claim → execute (openexec run) → commits on an isolated branch
        │  open PR (draft by default)
        ▼
 pr_open  ── record-pr AUTO-posts the governance assessment to the PR
        │  evidence (CI/test), ready_for_test
        ▼
 merge gate  ── never auto-merges by default; operator + approve-authority, or policy opt-in
        ▼
 done   ── every transition + decision is a hash-chained audit event
        │
        └─ write-back: the tracker item's status label + comment mirror OpenExec state
```

### Change status machine

`candidate → (planned) → plan_ready → approved_for_ai → implementing → pr_open → ready_for_test → done`

Triage advances a change to `plan_ready` (deep triage sets it directly; `planned` is an intermediate the validator permits). Off the happy path: `changes_requested` (from a reviewer; only exits back to `plan_ready` via revision), `rejected`, `deferred` (terminal). `blocked` is a defined status with an `ai:blocked` label but no wired change-level transition today (reserved).

**There is no operator-override from `changes_requested` to approval.** An AI reviewer's `request_changes` is binding: the plan must be revised. (A deliberate strictness; an auditable human override remains a possible future addition.)

---

## 4. Two triage lanes

A one-line change should not incur an 8-story plan plus a round of planner-output review. Two lanes:

| | **Deep triage** (`work triage --deep`) | **Lightweight lane** (`work quickplan`) |
|---|---|---|
| Decomposition | Full planner: goals → vertical-slice stories → tasks | One story, one task, from the intent — no planner |
| Risk | AI-classified | AI-classified; **high/critical refused** (must use deep triage) |
| Review | Required reviews per risk tier (see below) | **Operator approval waives AI review** — the operator is the reviewer; the waiver is recorded in the audit trail |
| For | Non-trivial features | Trivial low/medium changes |

Both lanes produce the impact + operability assessment (§6) before the human approval gate — deep triage generates it during triage; the lightweight lane generates it at `quickplan` time — and **both stop at a human approval gate** (there is no AI/auto approval; see §4 note).

### Risk tiers → required reviews (default policy)

| Tier | AI review | Security review | Human approval | Auto-merge |
|------|-----------|-----------------|----------------|------------|
| low | – | – | **required** | policy opt-in only |
| medium | required | – | required | policy opt-in only |
| high | required | required | required | never (destructive → human) |
| critical | required | required | required + explicit risk acceptance | never |

**Every change stops at a human approval gate** — `ApproveChange` always requires an operator session, at every tier. The policy's per-tier `AIApprovalAllowed`/`CanAutoApprove` flags are **not wired**: no code path auto-approves a change. (This is the intentional product truth; low-risk auto-approval would be a separate, explicit path if ever added.)

Workspace policy is **clamped, never relaxed**: an operator-owned `~/.openexec/governance-policy.yaml` may tighten tiers; a workspace file can only be at least as strict.

---

## 5. Trust & security boundaries

These are the load-bearing invariants — the main thing to review.

- **Human vs AI attribution.** Seeded authorities: `pm` (human), `developer` (human), `bugbot` (ai), `tester_ai` (ai), `security_ai` (ai), `ci_verifier` (verifier). A human-typed authority can only be attributed in an **operator session** (`OPENEXEC_OPERATOR_SESSION=1`); an agent session cannot forge a human-attributed audit record.
- **Operator session gate, GitHub-identity bound, fails closed.** Approvals (change + release), merges, and risk-acceptance require an operator session. When an operator allowlist (`~/.openexec/operators.yaml`) is configured, the session is bound to the current `gh`-authenticated login (OAuth-backed) — the spoofable `OPENEXEC_OPERATOR_SESSION=1` env var is ignored, and survives only as a dev/pilot shortcut when *no* allowlist exists. A present-but-unreadable/invalid allowlist **fails closed** (no operator session, no env-var fallback). The resolved identity (`github:<login>`) is stamped into approval/merge/risk-accept audit events. The MCP plane never sets an operator session, so an agent over MCP cannot self-approve or merge.
- **Governance is the sole un-holder.** Triaged tasks are `hitl`; only `ExecuteChange` flips a change's approved+claimed tasks to `afk`. Unapproved work is never auto-built by `openexec run`/the scheduler/backlog tools.
- **Critical risk acceptance is enforced.** A critical-tier change cannot be approved or merged without a **current** human risk-acceptance on record (`work risk-accept`, operator + human authority holding `risk_accept`). A plan revision bumps the proposal version and invalidates a stale acceptance.
- **Append-only, tamper-evident, atomic audit.** `decision_events` and `evidence` are append-only (DB triggers reject UPDATE/DELETE). Decision events form a SHA-256 **hash chain** (`hash = SHA256(prev_hash ‖ canonical(event))`); `VerifyAuditChain` detects any alteration/deletion/reorder; ordering is by rowid. A **change's** status change and its decision event are written in **one transaction** (`TransitionChange`) — either both land or neither — for approve, mark-done, record-PR, ready-for-test, reject/defer, revise, claim→implementing, triage→plan_ready, and AI-review→changes_requested. (Release-level events are recorded separately; lower stakes.) Merge records `merge_authorized` before the `gh` call and `merged`/`merge_failed` after, so an external side effect is never silent.
- **Merge gate fails closed, on freshly-fetched CI.** No tier auto-merges under default policy. Auto-merge requires: policy opt-in **AND** status `done` **AND** operability clear (rollback-safe, no destructive migration, low deploy risk) **AND** the CI check-runs on the **current PR head commit** are green, **re-fetched live from GitHub at merge time**. Re-fetching (rather than trusting a stored evidence row) means green results recorded before a later push cannot satisfy the gate — the checks that matter are the ones on the exact commit being merged. Trusted evidence is separately recorded for the audit trail by `SyncGitHubChecks` (fetches check-runs, records SHA + payload hash); manual `record-evidence` refuses a `github`/`webhook` source. Otherwise a human operator with an approve authority must authorize the merge. Draft-PR-by-default embodies "never accidentally merge."
- **Skill/lesson proposals are propose-then-approve.** Agents may propose durable lessons; only a human activates them.

---

## 6. AI risk assessment (impact + operability) — recorded in the audit trail

Every change is assessed and the assessment is **both** posted to the PR **and** recorded as a hash-chained decision event, so a reviewer can later see *how the AI evaluated the risk*.

- **Impact** (`ai.AnalyzeImpact`): the exact files the change will create/modify, each with a reason. The model is forbidden to invent paths; unknowns are stated as notes.
- **Operability** (`ai.AnalyzeOperability`): `rollback_safe` (yes/conditional/no), `db_migration` (none/additive/destructive), `deploy_risk` (low/…/critical), mitigations, monitoring.

`work record-pr` auto-generates the assessment, records an `assessed` decision event (actor `risk_assessor`/ai), and posts a "🔒 OpenExec governance assessment" comment to the PR. `work pr-assess [--print]` re-runs or previews it. Sections that were not produced render as **"not assessed"** (so absence is visible, never silently omitted).

The merge gate consumes the operability report: an operationally-risky change (destructive migration, not rollback-safe, high deploy risk) can never auto-merge even if the risk tier would otherwise allow it.

---

## 7. The GitHub connector (implemented)

All GitHub interaction goes through a `Runner` abstraction that shells to the `gh` CLI (reusing the operator's existing auth). No GitHub App is required (that is [future work](#9-what-is-not-built)).

**Intake**
- `governance work import-github --repo owner/repo --issue N` — import one issue (idempotent; a partial unique index prevents duplicates). Kind is derived from labels: `enhancement`/`feature` → feature, `bug`/`defect` → bug, etc.
- `governance github sync --repo owner/repo --project P [--label ai:triage] [--no-triage]` — discover open issues (optionally by label), import each, and (unless `--no-triage`) deep-triage freshly-imported ones into stories/tasks awaiting approval. Idempotent; safe to schedule (cron / `/loop`).

**Inbound commands (steer from GitHub)**
- `governance github poll --repo owner/repo --project P` — process new `/openexec` slash-commands (review, approve, reject, defer, revise, ready-for-test) in issue comments. Author-gated by an operator-owned approver map (`~/.openexec/github-approvers.yaml`, login → authority); approve also requires an operator session. Unmapped commenters are ignored.

**Write-back (mirror OpenExec state to the tracker)**
- `governance work sync-github <change>` and best-effort mirroring on status changes: sets exactly one `ai:*` status label (`ai:triage`, `ai:plan-ready`, `ai:approved`, `ai:pr-open`, `ai:done`, …) and posts a status + next-action comment on the issue. The `ai:*` labels are **self-provisioned** on first sync (created idempotently), so write-back works on a repo that was never set up.

**Assessment on the PR** — see §6.

---

## 8. Jira mapping (designed, not yet built)

The pipeline is **source-agnostic after intake**: everything downstream of `ChangeRecord` (triage, assessment, review, approval, execute, merge gate, audit) is independent of where the item came from. `ChangeRecord.SourceType` already distinguishes sources (`github_issue` today). Adding Jira means:

| GitHub (built) | Jira (planned) |
|----------------|----------------|
| `import-github` / `github sync` (gh CLI) | `import-jira` / `jira sync` (Jira REST API, connector mirroring `connectors/github`) |
| Kind from labels | Kind from Jira issue type (Story/Bug/Task/Improvement) |
| `ai:*` status labels | Jira workflow status transitions (governance status → Jira status) |
| Issue comment write-back | Jira comment + field write-back |
| `/openexec` slash-commands in comments | Jira comment commands or a Jira automation webhook |

Jira is the more natural fit for the target buyer: **development work, not defects** — governance's `feature` kind and Story/Improvement mapping reflect that. No Jira code exists yet; this table is the contract we intend to implement.

---

## 9. What is / isn't built

**Implemented & live-verified (on `perttu/fotoyks-app`, a real Medusa storefront):**
- GitHub intake (`enhancement` → `feature` ChangeRecord), deep triage (8 vertical slices, risk auto-classified), file-level impact, two required AI reviews (bugbot + security_ai) that **blocked** a risky plan with senior-level findings.
- Lightweight lane end-to-end: quickplan → operator approval (AI review waived, recorded) → execute → **draft PR #3** → merge gate correctly **refused** an unauthorized merge → audit chain intact.
- Write-back: issue got the `ai:pr-open` label + status comment (labels self-provisioned).
- AI risk assessment posted to PR #3 and recorded as an `assessed` audit event (`rollback=yes, db_migration=none, deploy_risk=low`; the AI also flagged the reverse-proxy body-size coupling).
- Inbound `github sync` discovery + idempotency.

**Not built:**
- Jira connector (§8).
- GitHub App (uses `gh` CLI / personal auth; no org-installable App).
- Per-story/slice execute scoping (execute is change-level; the lightweight lane sidesteps it).
- Auto-ingest webhook (inbound is scheduled `sync` / comment `poll`, not event-driven).

---

## 10. Review response (findings closed)

A design/code review (see git history around this doc) raised seven findings. Status:

| # | Finding | Resolution |
|---|---------|------------|
| 1 | Critical risk acceptance documented but not enforced | **Fixed** — `work risk-accept` records a version-stamped `risk_accepted` event; `ApproveChange`/`MergeChange` require a current one for critical. |
| 2 | Trusted evidence provenance spoofable from the CLI | **Fixed** — manual `record-evidence` refuses `github`/`webhook`; only `SyncGitHubChecks` (fetches real check-runs + records SHA/payload-hash) produces trusted evidence. |
| 3 | Assessment produced after execution, not before approval | **Fixed** — `quickplan` now generates impact + operability before the gate; deep triage already did. Doc §4/§6 aligned. |
| 4 | State transition and audit event not atomic | **Fixed** — `TransitionChange` commits state + event in one transaction; merge records authorized-before / result-after. |
| 5 | `OPENEXEC_OPERATOR_SESSION` not a defensible identity | **Fixed (pragmatic)** — operator sessions bind to the `gh` OAuth login via an operator allowlist; env var demoted to dev fallback. Full device-flow/token-signing can layer on later. |
| 6 | Docs/policy ambiguity on low-risk approval | **Fixed** — product truth is "every change stops at a human gate"; doc table updated; `AIApprovalAllowed` documented as not-wired. |
| 7 | Status-machine doc mismatch | **Fixed** — §3 aligned with the validator (`candidate → planned → plan_ready`; `blocked` reserved). |

**Second review round** (three follow-up findings, all fixed):

| # | Finding | Resolution |
|---|---------|------------|
| 1 | Auto-merge could use stale CI evidence (green before a later push) | **Fixed** — auto-merge re-fetches the current PR head's check-runs live and requires green; no stored evidence can satisfy freshness. |
| 2 | "Atomic state+event" not universal (claim, triage, quickplan, AI-review still split) | **Fixed** — all four now use `TransitionChange`; doc narrowed to name exactly which transitions are atomic. |
| 3 | Malformed operator allowlist failed open to the env-var | **Fixed** — a present-but-invalid allowlist fails closed; only a *missing* one falls back to the env var. |

Remaining open (reviewer's own recommendations, lower priority): the deterministic planner-verification linter (`planner.LintVerificationScript`) is advisory, not a hard reject; per-story execute scoping and a full cryptographic operator-identity model are still future work.

---

## 11. Command reference (worked example)

```bash
# Intake (GitHub issue → governed change)
openexec governance work import-github --project P --repo owner/repo --issue 2
#   or, scheduled discovery:
openexec governance github sync --project P --repo owner/repo --label ai:triage

# Triage
openexec governance work triage  <change> --deep        # full decomposition
openexec governance work quickplan <change>             # lightweight lane (trivial)
openexec governance work impact  <change>               # file-level impact (review)

# Review (required by risk tier) + human approval (operator session)
openexec governance work review-plan <change> --reviewer bugbot
openexec governance work review-plan <change> --reviewer security_ai
openexec governance work risk-accept <change> --by pm --note "..."   # critical only
#   operator session = a gh login listed in ~/.openexec/operators.yaml
openexec governance work approve <change> --by pm

# Release scope + execute
openexec governance release create R-1 && \
  openexec governance release add R-1 <change> && \
  OPENEXEC_OPERATOR_SESSION=1 openexec governance release approve R-1 --by pm
openexec governance work execute <change>               # isolated branch, draft PR

# PR lifecycle (record-pr AUTO-posts the assessment to the PR + audit trail)
openexec governance work record-pr <change> --url <pr-url> --branch <branch>
openexec governance work pr-assess <change> --print     # re-run / preview assessment
openexec governance work sync-checks <change>           # fetch CI checks → trusted evidence
openexec governance work ready-for-test <change>

# Merge gate (fails closed) + write-back
openexec governance work merge <change> --by pm --method squash   # operator session required
openexec governance work sync-github <change>           # mirror status to the issue

# Audit
openexec governance audit verify                        # re-verify the hash chain
openexec governance audit export --out audit.json       # sealed, optionally HMAC-signed
```

---

*Generated 2026-07-03 from the `feat/release-governance` implementation. Live evidence: `perttu/fotoyks-app` issues #1–#2, PR #3.*
