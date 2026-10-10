# Experience-first dogfood — reconciliation with Portfolio Stewardship

- **Status:** machine reconciliation, 2026-10-11. Advisory only; it accepts
  nothing on the owner's behalf.
- **Task:** `300d734044b9006ff93e00a7d3fb91fe` (“Dogfood the experience-first
  operating model, then implement an advisory, provenance-labelled triage…”,
  full wording in `NOTES.md`).
- **Operating model:** [`EXPERIENCE_FIRST_OPERATING_MODEL.md`](EXPERIENCE_FIRST_OPERATING_MODEL.md),
  stages E0 and E1.
- **Run record under review:** `agent-console`
  `docs/dogfood/experience-first/2026-08-17-agent-console/` (`RUN_RECORD.md`,
  `OWNER_REVIEW_PACKET.md`, `EXPERIENCE_CONTRACT_PROPOSAL.md`,
  `FOCUS_CONTRACT_PROPOSAL.md`, `PROJECT_GOAL_AND_DOD.md`).
- **Current destination:** Professional Portfolio Stewardship, owner-adopted
  2026-09-05. The accepted contract lives in Agent Console. Its wording is in
  `PROJECT_INTENT.md` and `GOAL.md` in the owner's primary OpenExec checkout.
  `measured`: as of 2026-10-11 both are uncommitted there (`PROJECT_INTENT.md`
  modified, `GOAL.md` untracked). Committed `main` (`186646ee`) still carries
  the 2026-08-22 Intent. This record cites that working copy and does not
  commit or alter it.

Provenance labels follow the operating model: `owner-stated`,
`project-intent`, `observed`, `measured`, `inferred`, `unanswered`.

## Why this record exists

The approved triage contract for this task requires the project-level
input/output fidelity review to finish before any E1 implementation. It also
says whether E1 automation is warranted is an owner decision. This record does
that review against the *current* destination. It does not restart the stopped
review loop recorded in `GOAL.md` and does not implement E1.

## Where the dogfood stands

- `measured` — Review Gate 1 (Goal and high-level DoD) was accepted on
  2026-08-20. Later accepted revisions (Goal 3 / DoD 4 / Interpretation 3–6)
  are recorded in `RUN_RECORD.md`.
- `measured` — `EXPERIENCE_CONTRACT_PROPOSAL.md` and
  `FOCUS_CONTRACT_PROPOSAL.md` still say “machine proposal, not accepted”.
  Review Gate 2 never happened. Gate 3 (demo/communication) depends on Gate 2.
- `measured` — The rejected M0/M1 guided-setup attempt is kept as negative
  evidence of premature fixture selection (`RUN_RECORD.md`, “Owner scope
  correction”). This record does not reuse it.
- `owner-stated` — The 2026-08-21 instruction (“try already take this approach
  with more autonomy, less me to approve trivial things”) authorized the bounded
  Goal/Ready/Interpretation slice before Gate 2. It did not accept the
  Experience or Focus proposals.
- `measured` — Since then, `PROJECT_INTENT.md` and `GOAL.md` were replaced by
  the 2026-09-05 Portfolio Stewardship destination. The Gate 2 proposals were
  written against the earlier Agent Console Goal revision 1.

## Fidelity review against the current destination

Each finding compares a proposal claim with an owner-authored source. A
`conflict` means the proposal cannot be faithfully accepted as written; this
record does not resolve it.

| # | Proposal claim | Current source | Provenance | Verdict |
|---|---|---|---|---|
| R1 | Hero step 6: “The owner accepts the next task” (`EXPERIENCE_CONTRACT_PROPOSAL.md`, Hero workflow) | Accepted project-contract non-goal “Owner approval of navigator-created tasks”; accepted DoD D3/D4 (“without asking the owner to approve generated tasks”) | project-intent, measured | `conflict` — stale; superseded by already-accepted DoD |
| R2 | Pain/Keep: repeated postponement forces a Finish/Reduce/Park/Stop/Replace decision (`EXPERIENCE…` Pain; `FOCUS…` Keep, “Saying no”) | Intent: “Quiet projects create no guilt or artificial urgency”; revival is event-driven, “not inactivity alone” | project-intent | `conflict` — needs owner judgment (also present in accepted DoD D4) |
| R3 | Return restores “the last meaningful project” (Hero steps 1–2) | Goal: return reviews present genuine outcome changes across the portfolio; compressed attention | project-intent | partial — still compatible at project level; portfolio return not covered |
| R4 | Keep list covers one finish target, run truth, notifications, delivery closure, recovery (`FOCUS…` Keep) | Goal adds parking/revival triggers, validated cross-project learning, evaluated reuse, outcome lifecycle states (waiting/parked/graduated/superseded/ended) | project-intent | gap — Keep and Not-doing predate these outcomes |
| R5 | Primary customer: the owner with a 30+ project portfolio, often away from a screen | Intent: one owner carrying a large creative, software, commercial, learning and personal portfolio | project-intent | faithful; wider than software only |
| R6 | Magical moment: return and see that work moved, what evidence proves, one next decision, no terminal | Goal: leave and return safely; machine work progresses without global owner scheduling | project-intent | faithful |
| R7 | Trust: provenance per claim; agent proposes Done, never self-accepts | Intent invariants: evidence determines Ready; owner holds acceptance | project-intent | faithful |
| R8 | Not doing: shared multi-user console, workflow builder, activity dashboards, progress percentages | Intent/Goal non-goals: progress percentages, coercive prioritization, indiscriminate platform building | project-intent | faithful |

`inferred` — R1 is a mechanical correction: the owner already accepted
the replacement wording in DoD D3/D4. R2 is not mechanical. The accepted DoD
and the newer Intent pull in different directions, and only the owner can say
whether a deliberate postponement decision is wanted or is inactivity pressure.

`observed` — `GOAL.md` links its ten validation conditions to
`agent-console/docs/GOAL_V5_PROFESSIONAL_PORTFOLIO_STEWARDSHIP.md`. That file
does not exist in the Agent Console checkout read on 2026-10-11. This review
therefore uses the condition summary in `GOAL.md` and the accepted project
contract, not the V5D1–V5D10 text.

`observed` — The destination this review checks against is not committed to
either repository. Until the owner's working-copy `PROJECT_INTENT.md` and
`GOAL.md` are committed, someone starting from `main` sees the 2026-08-22
Intent, and the R2–R4 findings cannot be reproduced from committed state.
Committing them is the owner's call. This record does not do it.

## Method findings (keep / amend / stop)

These are machine recommendations about the experience-first method, drawn
from the dogfood evidence. Whether to keep, amend or stop the method is an
owner decision.

- **Keep** — Provenance labelling and stable-ID traceability (`OWNER_REVIEW_PACKET.md`).
  `observed`: the matrix made R1–R8 checkable in one pass, without repeating
  the interview.
- **Keep** — Project-level interview before choosing a feature fixture.
  `observed`: the feature-first M0/M1 attempt picked the wrong evaluation unit,
  and the owner rejected it.
- **Keep** — Gates as convergence loops, not one-pass forms (`RUN_RECORD.md`,
  process-change, 2026-08-20).
- **Amend** — Proposals need an explicit binding to the destination revision
  they were derived from, and they need to go stale when that destination
  changes. `observed`: Gate 2 proposals sat “ready for review” for ~7 weeks
  while Intent and Goal were replaced; nothing marked them stale. This is the
  same rule the run record already applies to Interpretation (“a destination
  change leaves it stale rather than silently rebinding it”).
- **Amend** — The E0 bullet that gates E1–E4 on “the Agent Console G2 owner
  gate” no longer matches how authority is granted. Authority moved to the
  accepted Goal/Interpretation envelope, so a gate keyed to one dogfood package
  has no live trigger. `inferred`.
- **Stop (recommended)** — Treating the August Gate 2 package as still pending
  as written. `inferred`: accepting it unchanged would bring R1 back, contrary to
  an accepted DoD and a current non-goal.

## E1 gap assessment (advisory triage)

`measured` — Read-only inspection of `agent-console@1d8aa1d8` (main). Agent
Console owns owner interaction and the existing triage surface, so any E1 work
would land there, not in OpenExec. The existing generic triage fields do not
amount to E1 behavior:

| E1 requirement | Agent Console today | Status |
|---|---|---|
| Read Intent, Goal, accepted contracts | Intent (`internal/server/project_intent.go`) and portfolio Goal/scope (`taskPromptContext`, `internal/server/tasks.go`) are injected; accepted contracts are not | partial |
| Absent Intent → stop with `needs_owner` | `intentSection` only warns the agent (“Declared intent: none…”). Triage still runs. Hard `RequiresIntent` refusal exists only for playbooks (`internal/server/playbooks.go`) | partial |
| Absent Goal → iterative DoD proposal, no task queue | No branch; one read-only triage turn per task | absent |
| Provenance-labelled Experience Contract | `triageTaskPrompt` asks for a free-text *execution* contract; no provenance labels, no Experience Contract | absent |
| Missing questions, one hero workflow, one magical moment, ≥1 removal | Only “any genuine owner decisions” | absent (questions partial) |
| Present for refinement; never self-accept | Run completion always lands in `triage-review`; owner moves it to `ready` (`internal/store/store.go`) | present |
| Preserve revisions as evidence | `TriageContract` is one string overwritten by owner edit and by re-triage; transitions keep only from/to/time | absent |
| Verdicts `ready_for_owner_review / needs_owner / conflict / not_worth_building` | No such enum; `triage-review` approximates the first | absent |

`inferred` — The task-level triage that exists answers a different question
(“how should this task execute?”) from E1's (“what experience is the
initiative for?”). Grafting E1 fields onto every task triage would add an
owner gate to navigator-created tasks, which is a current non-goal. A faithful
E1 would sit at the outcome/initiative level, once per destination revision.
That placement is a design question for after the owner's E1 decision, not
before it.

`inferred` — Two gaps are independent of E1 and cheap. Should the owner want
them, they could be filed as ordinary route work: (a) a `needs_owner` stop for
triage when `PROJECT_INTENT.md` is absent, which matches the existing
playbook refusal; (b) keeping the machine triage draft alongside the owner's
edit, so corrections remain evidence. Neither is implemented here.

## Owner decisions this record does not make

`unanswered` — each needs the owner; the machine recommendation is advisory.

1. **R2 — postponement handling.** Should repeated postponement still lead to a
   deliberate Finish/Reduce/Park/Stop/Replace decision, or does the current
   “no inactivity pressure / event-driven revival” Intent supersede it?
   Recommendation: supersede it with trigger-based revival, because the newer
   Intent states it directly. Keep Stop/Park/Replace available whenever the
   owner chooses to use them.
2. **Experience/Focus proposals.** Regenerate them as revision 2 against
   Portfolio Stewardship (fixing R1 and covering R3/R4), or retire them as
   superseded history. Recommendation: regenerate. The method only produces
   evidence if a current proposal is reviewed.
3. **Method verdict.** Keep, amend (as above) or stop the experience-first
   method.
4. **E1 automation.** Whether the dogfood evidence justifies building E1. See
   the gap assessment above.
