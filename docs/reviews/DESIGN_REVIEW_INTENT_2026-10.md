# Design review 1/6 — declared intent and what it leaves unanswered

**Date:** 2026-10-24. **Task:** `d539d60d28bf0bb255b4efc8db5e9647` (first task of
the OpenExec `design-review` run listed in Agent Console
`docs/EXPERIENCE_FIRST_DOGFOOD_PLAN.md:187-199`).

This report reads the owner's declarations and records what they say and what
they do not. It answers nothing on the owner's behalf, changes no intent,
grouping or configuration, and is not evidence of Goal Ready or of any served
behaviour.

## Sources and provenance

| ID | Source | Revision / identity | Authorship as stated by the source |
|---|---|---|---|
| **N** | native `openexec` `origin/main` `PROJECT_INTENT.md` | commit `35e2125bca8544582d84c20c0cb5dc38a5041eea`; blob `73de905a2cf8da6652c5ef63dc772d30bc03574f`; last changed by `75e3bd87` (2026-08-22); first added by `00916e45`, deleted by `ffde08ee`, re-added by `90569c7f` (all 2026-08-22) | "accepted 2026-08-22. Owner-authored through the recorded interview. The answers below are the owner's words" (lines 3-5) |
| **Q** | native `origin/main` `docs/OPENEXEC_PROJECT_INTENT_QUESTIONS.md` | same commit | interview record; "The machine organized the questions and transcribed the answers; it did not supply or polish them" (lines 5-6) |
| **L** | owner checkout `/mnt/data1/projects/openexec/PROJECT_INTENT.md`, **uncommitted** working-tree modification | file blob `508f1df45c683a69bf78f3c097077c16f88f378f`, mtime 2026-09-05 22:50 +0300; checkout HEAD `186646ee` (ancestor of N's commit, reported "behind 68") | "updated by owner instruction on 2026-09-05 … a synthesis of the owner's adopted Professional Portfolio Stewardship goal … It is not a verbatim quote." |
| **G** | owner checkout `GOAL.md`, **untracked** | file blob `c3afa418fe7270eff32d2cb413f176981d25926e`, same mtime | "Owner-adopted: 2026-09-05. Professional Portfolio Stewardship." |
| **A** | owner checkout `docs/archive/PROJECT_INTENT_2026-08-22.md`, untracked | blob `73de905a…` — byte-identical to N | archive of N |
| **C** | Agent Console project context delivered with this task (Intent, Goal, Scope, Problem, Success measures, Constraints, Non-goals, Principles, interpretation revision 10) | no repository revision; persisted console record. The context names **L** as its declaration source ("from /mnt/data1/projects/openexec/PROJECT_INTENT.md") | owner-authored per the console; the interpretation is "Interpretation revision 10 (Goal 4 / DoD 4)" |

Source-access limits: Agent Console was read from its local checkout (HEAD
`1d8aa1d8f120c8347d0431210c92475085182a2a`, reported "behind 1097" its
origin/main, with many uncommitted changes); its current `origin/main` SHA was
not available to this pass. The cited dogfood plan file has no local
modification. Bash access to that checkout was refused; reads used the file
reader and the console's read-only repository inspection.

**Central provenance finding:** the declaration the console serves as this
project's intent (**C**, sourced from **L**) is not on native `origin/main`.
`origin/main` still holds the 2026-08-22 interview text (**N**). The newer text
exists only as an uncommitted edit in the owner checkout, by its own status a
machine synthesis made on owner instruction, not the owner's verbatim words.
This report does not reconcile the two.

## Template being answered

N, Q and L share the same six-question template (Q gives each question's
prompt): *Why does OpenExec exist? · Who is it for? · What experience should it
create? · What must remain true? · What is deliberately not OpenExec's
purpose? · How is success recognized?* The template has no separate "jobs"
question. "What experience should it create?" (prompt, Q:36-37: "What should
the customer be able to do, leave, and return to?") is the nearest thing, and
is read as jobs below.

## What is declared

### Purpose
- **N:7-11** (owner quote): "Openexec is to solve the problem that ai cant code production quality data". Q:62-67 records the owner's clarification of the *success* line ("Shops is ships and iso27001"). The word "data" in the purpose line is not clarified.
- **L / C**: "Enable one owner to pursue many independent outcomes without becoming the AI organization's memory, scheduler, task manager or execution bottleneck, while preserving freedom over attention, direction and authority." C adds Problem: "The owner must not become the memory, scheduler, task manager or execution bottleneck of an AI organization carrying many independent outcomes."

### Primary user
- **N:13-17**: "primary user is me."
- **L**: "The primary user is the owner, carrying a large creative, software, commercial, learning and personal portfolio through one system."

### Jobs (experience)
- **N:19-25**: "Openexec has structured way to split intent into tasks and then implement and test that deliverable works and outcome is predictable and process is reviewable."
- **L**: "The system carries the portfolio … The owner can leave, switch projects and return without reconstructing execution state or coordinating tasks, retries and repositories. Agent Console provides owner interaction and orientation; OpenExec owns navigation and execution."
- **G / C Goal**: deliver and dogfood Professional Portfolio Stewardship across Agent Console + OpenExec (orientation, advancing authorized work, parking/reviving, validated learning transfer, safe reuse, compressed portfolio awareness).

### What must remain true
- **N:27-32**: "It should handle coding like senior engineers do and process is ISO 27001 compliant."
- **L:25-35**: four bullets: per-outcome Intent/Goal/Ready/authority/route/evidence/lifecycle; the owner holds destination, values, acceptance and consequential authority, and "Machine scheduling stays within delegated authority and budgets" (L:30); evidence determines Ready, with senior-engineering practice, tested working code, reviewable delivery and ISO 27001 retained ("this declaration is not evidence of compliance or certification"); quiet projects create no guilt or artificial urgency.
- **G:23**: "Effect and spending authority are unchanged."
- **C Constraints**: exclusive repository membership; one authoritative ProjectOutcome; consequential effects policy-controlled.

### Non-goals
- **N:34-36**: `UNANSWERED — owner`.
- **L**: "The adopted goal excludes coercive human prioritization, mandatory owner task coordination, cross-project authority leakage, indiscriminate platform building and permanent multi-agent bureaucracy … the original interview's unanswered broader exclusions remain in its archive."
- **C Non-goals**: separate Agent Console and OpenExec portfolio projects; owner approval of navigator-created tasks; duplicated navigation logic in Agent Console; progress percentages from task or agent counts; coercive owner prioritization or inactivity pressure; cross-project authority leakage, indiscriminate platform extraction and permanent multi-agent committees.

### Success measures
- **N:38-42**: "Main think is that ships tested and working code" (clarified "ships", Q:64-67).
- **L**: one owner carries many outcomes without becoming the AI organization's project manager; authorized outcomes progress professionally, others remain coherently waiting/parked/graduated/superseded/ended; return reviews present genuine outcome changes; learning improves work without adding weight; "Software delivery still ships tested working code."
- **C Success measures**: the ten listed measures (reconstructible portfolio state … the AI organization gets better without getting heavier).

## UNANSWERED and partial entries

Rows are measured against the template prompts in Q. The last column says
whether the newer record (L/C) answers the gap. Where it does, the gap is not
open, but it is answered only off `origin/main` (see the provenance finding).

| # | Template question / prompt part | In N (origin/main) | Answered by L/C? |
|---|---|---|---|
| U1 | Non-goals: "Which adjacent products, users, or automation outcomes should not be used to justify work…" (Q:51-52) | explicit `UNANSWERED — owner` (N:36) | Partly. L/C list goal boundaries, but L says "the original interview's unanswered broader exclusions remain in its archive". **The broader exclusions are still unanswered by L's own wording.** |
| U2 | Who: "whose needs may be deliberately secondary?" (Q:27-28) | not addressed | No. L names the owner's portfolio only. **Unanswered.** |
| U3 | Experience: "do, leave, and return to" (Q:36) | "do" only | Yes (L: leave, switch, return). |
| U4 | Experience: "Which parts should remain under owner judgment?" (Q:37) | not addressed | Yes (L: owner holds destination, values, acceptance, consequential authority; C authority interpretation). |
| U5 | Must remain true: authority, safety, evidence, compatibility, **cost**, product boundaries (Q:44-45) | engineering practice and ISO 27001 only | Authority, evidence: yes (L). Cost/spend: partly yes. L:30 "Machine scheduling stays within delegated authority and budgets"; G:23 "Effect and spending authority are unchanged"; C's authority interpretation reserves spend to the owner and bounds route work by a persisted NavigationBudget. The newer record therefore answers cost at least in part. Compatibility: present only in C's interpretation constraints (text truncated in the delivered context), not in L or G. **Not declared in L, G or N: a compatibility boundary.** |
| U6 | Success: owner-observable evidence "independently of task counts, agent activity, commits, or green unit tests" (Q:58-60) | "ships tested and working code" (a delivery outcome, not an observable test) | Yes at goal level (L, C success measures). G says the ten validation conditions live in `../agent-console/docs/GOAL_V5_PROFESSIONAL_PORTFOLIO_STEWARDSHIP.md`. That file is **absent** from the local Agent Console checkout and has no history there. **The conditions' source is unverified.** |
| U7 | Purpose: the meaning of "production quality data" (N:11) | not clarified | Superseded in L by a different purpose (see X1). Unanswered for N. |

No answer is supplied for any of the open entries above. Per the triage
contract, no owner-decision card is raised for them.

## Contradictions and unknown provenance

- **X1: Purpose differs between N and L/C.** N:11 (code-quality problem) and L ("Why does OpenExec exist?", portfolio stewardship) state different purposes. L says it is a non-verbatim synthesis. The owner instruction it cites is not in any committed source this pass could read, and the text is uncommitted. Native `origin/main` docs do mention the stewardship goal (`docs/ARCHITECTURE.md:36`, `docs/VERIFICATION_FAILURE_INSPECTION.md:13`, `docs/verification/repair-study.md:20`), so `origin/main` contains both a stewardship destination and an August intent that does not mention it.
- **X2: Served versus committed.** C is sourced from L, a working-tree file in the owner checkout. Anything that reads `origin/main` sees N. The two disagree on every one of the six answers.
- **X3: Dead reference.** G:9 links a validation document that does not exist in the local Agent Console checkout (see U6).
- **X4: Stale plan statement.** Agent Console `docs/EXPERIENCE_FIRST_DOGFOOD_PLAN.md:195` (local HEAD `1d8aa1d8`, last changed `790249fa`) says "OpenExec has no owner-authored root `PROJECT_INTENT.md`". That was true at its 2026-08-17 snapshot. Native `origin/main` has had one since `90569c7f` (2026-08-22).
- **X5: Repository grouping.** C notes that Agent Console declares its own `PROJECT_INTENT.md` while being grouped under this project, and C's constraints and non-goals declare the two repositories one portfolio project. This report records both statements. Changing grouping is out of scope.

## History: the earlier resolver defect

The dogfood plan (line 194) records that an earlier pass of this task found
Agent Console's intent being served as OpenExec's. The source fix is Agent
Console commit `ae22eda485220eaea22e0543370ba2abc507d9e8` (2026-08-14, "fix: one
project's declaration is never another's ground truth"; touches
`internal/server/project_intent.go`, `interview.go`, `project_intent_test.go`),
followed by `655aa943` ("a repository that declares its own intent says so out
loud"). Both appear in the history of the local Agent Console HEAD. This pass
did not rebuild, re-test or deploy anything. Whether the corrected build is
served is not established here. The plan reserves that confirmation to the
owner. The context delivered with this task does name the native project's own
file (L) as its source, not Agent Console's.

## Downstream

The five later audits (`c2986112…`, `344c8390…`, `bb19d2e4…`, `869ef988…`,
`8a800117…`) keep their dependency on this report. Their findings are not
anticipated here.
