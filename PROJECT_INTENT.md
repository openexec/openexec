# OpenExec ecosystem — project intent

**Status:** accepted 2026-08-22. Machine-synthesized from the owner's accepted
Agent Console intent and 2026-08-22 direction, then explicitly approved by the
owner. This is the umbrella intent for the multi-repository `openexec` owner
project; repository-specific contracts remain subordinate to it.

## Why this exists

OpenExec exists to reduce the executive-function burden of turning evolving
intent into completed, useful outcomes. The owner should be able to describe
or revise a destination, leave, and later return to either a verified outcome
or one prepared question that genuinely requires owner judgment.

The system is a persistent outcome navigator, not a task generator. Plans,
tasks, attempts, agents, and tools are disposable route state. Intent,
accepted destination and Ready revisions, consequential authority, and exact
evidence remain durable and inspectable.

## System boundary

The owner project is deliberately multi-repository. Its current integrated
product boundary is:

- **Agent Console is the owner-facing control plane.** It records accepted
  Intent, Destination, Ready, shared interpretation, consequential authority,
  and owner decisions. It presents completed outcomes, learning, evidence, and
  genuine questions without transferring operational cleanup to the owner.
- **OpenExec is the navigation and execution plane.** It consumes the accepted
  contract, derives and manages the tactical route, executes work, validates
  evidence, performs bounded recovery, and replans inside the authorized
  envelope.
- **The dependency remains directional.** Agent Console invokes OpenExec
  through explicit CLI, API, or MCP contracts. OpenExec does not depend on
  Agent Console's UI implementation. Returned execution state and evidence are
  data flowing back through the contract, not a reverse source dependency.

Other repositories in the owner project are supporting components. A route
may use them only when it records the dependency on the current destination or
completion condition; membership alone is not authority to create unrelated
work.

## Primary user and desired experience

The primary user is the owner: a senior engineer managing many projects, often
away from a full screen. The attended experience is periodic review of
outcomes, learning, interpretation changes, and consequential decisions—not
approval of generated tasks, retries, repair choices, or stale operational
cards.

The system should feel technically trustworthy while hiding avoidable
machinery. Quiet progress remains perceptible. Needs you contains only a
judgment or authority boundary the machine cannot resolve within the accepted
contract.

## Authority

- **Owner-held destination authority:** Intent, Destination, Ready,
  invariants, strategic product choices, and final subjective acceptance.
- **AI-held route authority:** reversible planning, navigator-created tasks,
  sequencing, implementation approach, agent and tool selection, testing,
  bounded recovery, and local replanning inside the accepted envelope.
- **Policy-controlled effect authority:** merge, deploy, spending, destructive
  data changes, security boundaries, and customer-impacting external effects.
- **External observers:** may inspect frozen truth and return request-scoped
  advisory evidence. They cannot alter destination, authority, acceptance, or
  execution state. OpenExec may use validated advice for route-only changes
  already inside its envelope.

## Principles

- Evidence, not task completion, determines whether Ready is satisfied.
- The navigator cannot weaken Ready or rewrite evidence to make its route
  appear successful.
- Material Destination or Ready changes remain versioned and owner-accepted.
- Machine failure, retry, repair, and navigator-task cleanup are not owner
  work.
- Existing evidence remains pinned to the revisions and builds it actually
  evaluated; carrying it forward requires explicit re-evaluation.
- One high-leverage interpretation correction is preferable to many
  downstream task approvals.
- The owner never maintains state the system can derive and is never asked the
  same question twice.

## Current destination relationship

The first dogfood destination is the integrated Agent Console + OpenExec
outcome-navigation journey. Agent Console supplies the instrument panel and
owner authority surface; OpenExec supplies navigation and execution. The route
may change both repositories. Success must be proven through the running
cross-repository journey rather than by either repository passing tests in
isolation.

## Non-goals

- Fully autonomous selection of the owner's purpose or destination.
- Making the owner supervise or approve navigator-created tasks.
- Treating repository membership as permission for unrelated work.
- Duplicating navigation policy inside Agent Console or UI policy inside
  OpenExec.
- Inventing progress percentages from task or agent activity.
- Declaring a merge, build, deployment, or green unit test to be customer
  outcome evidence by itself.
- Requiring every project in the portfolio to use the same destination or
  Ready contract.

## How success is recognized

For an accepted destination and Ready revision:

1. Agent Console persists the exact contract and teaches back the material
   interpretation that will authorize navigation.
2. OpenExec receives those revisions through a bounded interface and derives
   the route without asking the owner to approve internal tasks.
3. Execution, validation, recovery, and replanning continue across the linked
   repositories within the accepted authority and budget.
4. Evidence is bound to the Goal, Ready, repository, worktree, build, and
   exercised journey it actually proves.
5. Agent Console presents completed outcomes, learning, and at most one
   prepared owner question.
6. The state survives leaving, restart, and route recalculation without the
   owner reconstructing project state.

The governing product metric is useful closed loops per owner-attended hour,
with avoidable owner work identified separately from destination judgment,
real authority decisions, and final acceptance.
