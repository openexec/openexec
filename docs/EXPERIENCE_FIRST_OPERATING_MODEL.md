# Experience-first and completion product operating model

**Status:** Owner direction recorded 2026-08-17; operating model and
implementation plan, not yet enforced by OpenExec.

## Two foundational laws

> **Experience First:** Start from the valuable customer outcome and work
> backwards to the experience and workflow, then derive capabilities,
> architecture, and technology.

> **Finish What Matters:** Optimize execution for reaching an accepted,
> evidenced outcome, not for maximizing activity, generated work, findings, or
> apparent progress.

The first law governs whether the system is building the right thing. The
second governs whether the human-agent system actually finishes it. Either law
without the other is insufficient: completion can efficiently deliver the
wrong outcome, while experience work can produce thoughtful definitions that
never ship.

AI increases the importance of the second law. Agents can generate more ideas,
investigations, improvements, architecture, and follow-up work than a person
can finish. Every useful investigation can expose five more useful
opportunities. Optimizing agent throughput without protecting closure can
therefore increase unfinished work and owner attention even while every local
productivity measure improves.

## Designed for a real person

The owner direction for applying Steve Jobs's customer-experience principle is
to design for one real primary customer before generalizing. This is not an
invented average persona.

For Agent Console, that primary customer is the owner. Agent Console's
`PROJECT_INTENT.md` says the owner has ADHD, a small and variable attention
span, a 30+ project portfolio, and a need to operate away from a full screen.
It also says novelty reliably starts work and the system exists to finish it,
without creating mandatory tasks, notification overload, or guilt piles.
Those are product requirements, not biographical decoration.

Colleagues may later run their own consoles, but Agent Console must first solve
the owner's observed workflow. Generalization comes from additional real
customers and evidence, not by weakening the primary experience into a generic
task manager. For another OpenExec product, the named customer comes from that
product's owner-authored `PROJECT_INTENT.md`; it must not be copied from Agent
Console merely because the technology is shared.

Every initiative also has one **experience owner** with authority over
coherence. That person decides the primary customer, hero workflow, magical
moment, removal boundary, and whether the whole result feels like one product.
Agents may investigate and implement in parallel, but they do not independently
define competing product directions. For Agent Console, the experience owner
is the owner and primary user identified in `PROJECT_INTENT.md`. The system's
job is to preserve that person's accepted decisions so coherence does not
require repeated supervision.

## Authority and decision order

OpenExec exists to turn intent into bounded, trustworthy execution. It must not
let the machinery of AI development become the product definition. Models,
agents, MCP, planners, providers, orchestration frameworks, and infrastructure
are implementation choices. A person cares about the result, how easily they
can reach it, and whether they can trust what happened.

The mandatory decision order is:

```text
Owner Project Definition (`PROJECT_INTENT.md`)
    -> Customer Outcome
    -> Experience Contract
    -> Completion Contract
    -> Accepted Scope
    -> Workflow and Capabilities
    -> Architecture and Technology
    -> Governed Execution
    -> Verification Evidence
    -> Done / Stop / Park / Replace
```

This is an authority chain, not a waterfall. New information may travel
upward. If implementation or verification shows that an accepted experience or
completion condition is wrong, the machine proposes a new version of the
applicable contract and shows the consequences. It must never silently expand
implementation, reinterpret the outcome, or reuse stale evidence against a
changed contract.

The owner-authored Project Definition remains the highest authority. Its
canonical repository artifact is `PROJECT_INTENT.md` at the project root, in
the form established by Agent Console's
`docs/DESIGN_DECISIONS/ADR-021-project-intent.md`. This model derives a
particular initiative or change from it; it must never quietly invent a new
purpose, customer, or trade-off. An unanswered product question is recorded as
`UNANSWERED — owner`, not completed by a model.

OpenExec's root `PROJECT_INTENT.md` was accepted on 2026-08-22. It defines the
multi-repository ecosystem boundary and the Agent Console → OpenExec
control-plane/navigation-plane dependency. Earlier runs correctly treated the
Intent as missing; later runs must read the accepted root artifact rather than
borrowing Agent Console's repository-specific definition or inferring purpose
from source code.

A wizard-generated root `INTENT.md`, if present, is a different artifact; none
is present in OpenExec as of 2026-08-17. `openexec wizard` can generate it and
`internal/intent` validates its goals, requirements, and constraints. It is a
derived execution specification, not owner-authored product ground truth. It
sits below the accepted experience and workflow and must never satisfy the
Project Definition prerequisite by name similarity.

This amends one part of the ordering in OpenExec's
`docs/architecture/ADR-005-INTENT-DECOMPOSITION.md`. Intent should not move
directly to a blueprint containing architecture and dependencies. The
experience and workflow must be accepted before architecture is chosen. The
ADR's bounded stories, tasks, branches, verification, and review loop remain
useful after that correction. The ADR and its registry carry a matching
amendment pointer so neither document silently wins by discovery order.

## Why this is needed

AI-assisted development makes plausible technology very cheap. It also makes a
specific failure mode cheap: a backend, endpoint, configuration option, or
agent capability can exist while the user cannot discover or operate it.

Recent Agent Console examples make the distinction concrete:

- Project archive behavior existed, but the action was not where the owner
  needed it on the project page.
- Local-model support existed, but Qwen was not discoverable and selectable in
  the natural project workflow.
- A new UI could be built while a restart still served an older embedded
  bundle.

Those were technically credible slices and incomplete experiences. Under this
model, none of them is complete until the owner journey works in the running
product.

## Authority and trust

The machine is an adviser during experience triage. The owner is the approval
authority.

The machine may:

- prefill facts already stated in the owner-authored Project Definition or previously accepted
  contracts;
- summarize observed pain from owner reports and dogfooding evidence;
- propose a customer outcome, hero workflow, magical moment, and things to
  remove;
- identify contradictions, missing evidence, and likely cognitive load;
- recommend measurable acceptance and draft an end-to-end journey;
- show which statements are quoted facts, observations, or inferences.

The machine must not:

- approve its own Experience Contract;
- convert an inference into owner intent;
- decide what feeling, trade-off, or magical moment matters to the owner;
- optimize an experience score and declare the experience good;
- begin architecture or implementation because its own draft looks complete;
- ask the owner for information that the Project Definition or an accepted contract
  already supplies.

Every recommendation carries provenance:

```text
owner-stated | project-intent | observed | measured | inferred | unanswered
```

The assisted triage result is a versioned proposal with four possible
verdicts:

- `ready_for_owner_review` — the machine found enough evidence to present a
  coherent recommendation;
- `needs_owner` — a value judgment or missing fact requires the owner;
- `conflict` — evidence and the owner-authored Project Definition disagree;
- `not_worth_building` — the proposed work has no traceable customer value or
  duplicates an existing workflow.

Only the owner can accept or revise the proposal. Acceptance is durable and
auditable; later machine revisions never overwrite it silently.

## Proportional Experience Contract

This is a gate, not a demand for a large document every time.

### Initiative contract

Use for a new product, major workflow, hackathon entry, or material product
direction.

```markdown
# Experience Contract

Customer:
Situation:
Pain today:
Current workaround:
Desired outcome:
Desired feeling:

Hero workflow:
Magical moment:
What becomes unnecessary:
What should be dramatically easier:

Maximum taps, typing, waiting, and context switches:
Failure and recovery:
Trust evidence:
What must not regress:

Success measurement:
Validation source:
Unanswered owner questions:
```

The magical moment is not a slogan. Expand it into the following required
block so product design and communication refer to the same observable
transformation:

```markdown
Magical moment:
What becomes possible:
What frustration disappears:
Time until the person experiences it:
Evidence that makes it believable:
```

“Wow” must come from a meaningful limitation disappearing, not from decorative
animation, inflated claims, or technology presented without a human
consequence.

### Feature contract

Use for a bounded new capability.

```markdown
Customer and situation:
Why the user cares, in one sentence:
Current experience:
Desired experience:
Natural entry point:
Magical moment:
Failure and recovery:
Observable acceptance journey:
Existing journeys that must not regress:
```

### Experience delta

Use for a defect or small change.

```markdown
Pain observed:
Expected behavior:
Where the owner naturally looks:
Smallest complete correction:
Failure branch:
Running-product proof:
```

If the one-sentence reason cannot be written, the feature returns to discovery
or is rejected.

## The Experience Gate

Architecture cannot be accepted and implementation cannot enter an executable
queue until the applicable contract passes owner review.

```text
[ ] Customer and situation are known.
[ ] The pain is owner-stated, observed, or otherwise evidenced.
[ ] The desired outcome is expressed without implementation terminology.
[ ] The simplest successful workflow is visible.
[ ] One magical moment is named.
[ ] Failure, recovery, persistence, and trust are addressed.
[ ] The interaction and attention budget is explicit.
[ ] Existing experiences that could regress are named.
[ ] Every proposed capability is necessary for the outcome.
[ ] Owner accepted or refined the machine recommendation.
```

A feasibility spike may investigate whether an experience is possible, but its
output is evidence for the gate, not permission to turn the spike's technology
into the product direction.

## Proportional Completion Contract

The Experience Contract says why the work matters and what the person should
experience. The Completion Contract is the system's first-class, versioned
representation of Definition of Done: it says where the current outcome stops
and what evidence permits a terminal decision. The machine proposes it from
existing intent, experience, repository evidence, and the request. The owner
must not be handed another blank form to maintain.

### Initiative or feature completion

```markdown
# Completion Contract

Outcome:
Customer-visible capability after completion:
Accepted scope:

Done when:
- [ ] customer-visible outcome works
- [ ] accepted journey passes
- [ ] failure and recovery behavior are verified
- [ ] persistence or restart behavior is verified where relevant
- [ ] protected existing behavior still works
- [ ] the production-shaped or deployed build is exercised where relevant
- [ ] remaining owner judgment is explicitly accepted

Not required for Done:
- ...

Known unknowns:
Evidence required for each condition:
```

`Not required for Done` is load-bearing. Agents are good at finding worthwhile
adjacent work; the contract gives the system permission to say, “good idea,
not required for this outcome.” “No blocking defects” is not an acceptable
unbounded condition. Use the bounded statement: no known defect prevents an
accepted outcome or violates protected existing behavior.

### Defect completion delta

```markdown
Outcome:
Done when:
- the reported path works in the running product
- the relevant failure branch is covered
- protected behavior still works
- the fix is present in the build the owner can actually reach

Not required:
- unrelated redesign, refactoring, or feature expansion
```

### Research or plan completion

Research is complete when it produces an evidenced decision, explicit unknowns,
and a terminal disposition or bounded next action. A plan is not complete
because a document exists: its authority must be accepted, its executable work
must be linked, and the owner must know what decision or action follows.

## Accepted focus and scope protection

Agent Console should present one owner-attended portfolio outcome as **What we
are finishing**. Projects may have subordinate tasks, and bounded autonomous
work may run elsewhere without asking for attention, but repositories, agents,
conversations, models, tokens, tools, and branches are implementation
machinery. They must not dominate the primary attention surface.

Starting or discovering another major outcome does not automatically replace
the current focus. The system recommends one of four explicit choices:

```text
Park quietly | Replace current outcome | Expand accepted scope | Dismiss
```

The default is to park a valuable but nonessential idea. A parking area must
not become another guilt pile: an item has a trigger or review date, duplicate
ideas are merged, quiet expiry and permanent discard are available, and no
notification appears unless its trigger occurs.

This is an attention default, not a prohibition. The owner remains free to
change direction. The product makes the change explicit so it can distinguish
intentional replacement from accidental loss of focus.

## Closure pressure

As an accepted outcome approaches closure, the threshold for introducing new
work increases. Closure pressure is applied to scope and agent behavior, never
emotionally to the owner. The interface must not use shame, overdue streaks,
decaying scores, queue-size pressure, or repeated reminders.

The execution phases are policy states, not estimated percentages:

| Phase | System objective | Agent behavior |
|---|---|---|
| **Explore** | Create option value and bound important unknowns | Investigate alternatives and question assumptions. |
| **Build** | Create the accepted capability | Implement the chosen path and surface material conflicts. |
| **Finish** | Reduce entropy and close accepted gaps | Refuse unrelated refactors, dependencies, speculative features, and UI expansion by default; recommend parking them. |
| **Ship** | Establish real-world evidence | Verify the target environment, customer journey, failure, recovery, persistence, and protected behavior. |

Phase changes require observable conditions and, where they alter scope or
owner attention, owner acceptance. They are never inferred from an invented
percentage or an unreliable estimate of “steps remaining.” A Finish-mode
request outside the contract receives a recommendation such as:

```text
Terraform support does not close a remaining completion condition.
Recommendation: park it quietly.

[Park quietly] [Replace current outcome] [Expand scope]
```

The owner can override the recommendation. Expanding scope creates a new
Completion Contract revision and identifies which prior evidence remains
valid.

## Done is an evidence-backed claim

An agent may propose Done but may not write an authoritative terminal state
directly.

```text
agent proposes Done
    -> evaluate the accepted Completion Contract revision
    -> attach evidence to each condition
    -> check protected behavior and blast radius
    -> verify the customer journey in the reachable build
    -> obtain any required owner-visible verdict
    -> accept Done
```

Each Completion Contract revision records its accepted scope, source revision,
acceptance authority, and time. Evidence is attached to a normalized condition
and that exact revision. When upstream evidence changes the Experience or
Completion Contract, the machine proposes a new revision, explains the delta,
and reevaluates existing evidence. Evidence that no longer proves the revised
condition becomes visibly unverified; it is never silently carried forward.

Accepted Done is durable history. A later regression creates a new reopening
or repair outcome rather than rewriting what was known at the original
decision.

The four terminal primitives are:

- **Done** — the intended outcome was achieved with sufficient evidence.
- **Stop** — the owner consciously abandons the outcome; no backlog residue or
  justification ceremony is required.
- **Park** — the outcome is irrelevant now and resurfaces only for its accepted
  trigger.
- **Replace** — another outcome explicitly receives the owner's attention;
  useful evidence and unfinished conditions remain linked.

Changing direction is therefore not classified as failure. The system records
whether focus changed intentionally.

## Evidence, not apparent progress

Agent Console must not display guessed completion percentages or unreliable
step estimates. It reports accepted conditions and evidence:

```text
WHAT WE ARE FINISHING

Repository intelligence v1

5 verified | 1 blocked | 1 unknown

Next closure action:
Verify restart persistence from the web client.

[Continue to Done]
```

Supporting sections may show autonomous work that needs no attention, one
`Needs you` decision, and quiet parked work with triggered-item counts. The
primary screen answers what deserves attention now to finish something
valuable.

Useful completion measurements are:

- accepted work cleared versus work arriving over a week;
- owner-attention touches required per terminal outcome;
- time blocked without a named cause and next action;
- conditions reopened because evidence was insufficient;
- accepted completion conditions actually verified;
- outcomes intentionally stopped, parked, or replaced.

Activity volume, generated findings, agent count, token use, queue length, and
synthetic experience or productivity scores are not completion measures.

## Don't make me think: cognitive-load review

Steve Krug's principle complements experience-first design: the intended
outcome is not enough if the interface makes the person interpret the system,
remember hidden state, or search for the next action. The checklist below is an
OpenExec policy synthesis for owner review, inspired by that principle; it is
not presented as Krug's wording or as a summary derived from the publisher
page. Owner acceptance, not attribution to Krug, makes a checklist item policy.

The review asks:

- Is the primary action where the person naturally looks?
- Are labels written in the person's vocabulary rather than the architecture's
  vocabulary?
- Is the next action visually unambiguous?
- Does the system derive information it already has?
- Are defaults useful and safe?
- Does the person have to remember state from another screen?
- Are choices reduced to the ones that matter now?
- Are waiting, success, failure, and recovery states explicit?
- Can recovery happen where the failure is shown?
- Is the same concept represented once, consistently?
- Can words, controls, approvals, or screens be removed?
- Can a first-time user succeed without documentation?

Measure concrete experience debt instead of producing a synthetic score:

```markdown
Experience debt introduced:
- +2 taps before the primary action
- one additional decision with no useful default
- recovery leaves the product and requires a terminal
- waiting has no time or progress explanation

Experience debt removed:
- project action moved to the project page
- configuration becomes selectable immediately after save
- success survives reload and is visibly confirmed
```

Useful measurements include taps, required typing, scroll distance, choices,
waiting time, repeated questions, context switches, terminal escapes,
ambiguous terms, and unrecoverable states. CI can enforce mechanical bounds;
owner dogfooding decides whether the experience actually feels clear.

## Focus and subtraction

Every initiative chooses:

- one primary customer;
- one painful situation;
- one desired outcome;
- one hero workflow;
- one memorable moment.

It also carries a `Not doing` list. If removing a capability does not weaken the
hero experience, it leaves the current slice. Focus is evaluated by what was
deliberately excluded, not by how many ideas were generated.

The machine should always recommend at least one removal or simplification. The
owner decides whether to accept it.

This is the **Focus Gate**:

> If removing a capability does not weaken the hero experience, remove it from
> the current slice.

For a hackathon, one unforgettable complete workflow is preferred to several
partially demonstrated capabilities. Secondary personas, advanced
configuration, general-purpose builders, competing demonstrations, and
infrastructure without a trace to the hero journey belong in `Not doing` by
default.

## Demo-first design and communication

The demo is a design tool before it becomes marketing. Write the shortest
honest demonstration before architecture. If the transformation is not clear
in the demonstration, more machinery will not repair the product idea.

### Demo Contract

```markdown
Audience:
One-sentence promise:
Pain shown:
Before state:
Magical action:
After state:
Trust evidence:
Why this matters now:
What the audience should remember tomorrow:
```

### Ninety-second structure

```text
0-15 seconds   A recognizable person and painful situation
15-30 seconds  The desired outcome and one clear promise
30-60 seconds  The live magical action
60-75 seconds  Result, persistence, and evidence
75-90 seconds  Why the person's work or life is now different
```

The demonstration must not fake the implemented capability. It may use an
experience prototype before implementation, provided it is clearly identified
as a prototype. The final gate repeats the journey against the running,
production-shaped product.

### Website message

The same contract determines website structure for OpenExec, Siivous, or any
other project:

```text
Hero       -> the accepted one-sentence customer promise
Problem    -> the observed pain and current workaround
How        -> the three-step hero workflow
Moment     -> the transformation, shown rather than described
Trust      -> evidence, limits, recovery, and what remains under user control
Proof      -> measured result or real dogfooding example
Technology -> supporting detail after the value is understood
```

Exact claims for each product must come from that product's owner-authored
`PROJECT_INTENT.md`. The machine can propose message variants, but it cannot
invent the customer or promise from repository technology. A generated
`INTENT.md` cannot authorize a website claim by itself.

### One experience, several communication forms

The accepted Experience Contract and Demo Contract are the source for product
communication, not merely inputs to implementation. They derive:

- the product's one-sentence promise;
- the website hero, problem, workflow, proof, and recovery story;
- the 90-second live demonstration;
- the pitch deck narrative;
- hackathon submission copy and judging presentation;
- onboarding, release, and stakeholder communication.

These are different lengths of the same claim. They must not invent different
customers, promises, magical moments, or evidence for different channels.
Technology appears only where it explains why the accepted benefit is now
possible or trustworthy.

A minimal pitch deck follows the experience rather than the architecture:

```text
1. Person and painful situation
2. Cost of the current workaround
3. One clear promise
4. Hero workflow
5. Magical moment
6. Result and evidence
7. Failure, recovery, and user control
8. Why this matters now
9. Technology and defensibility, traced to the experience
10. The one thing the audience should remember or do next
```

If a slide, website section, or technical diagram cannot be traced to a beat in
the accepted experience, it waits. Communication is therefore an early design
instrument: difficulty explaining the transformation is evidence that the
product definition is still unclear, not a marketing problem to solve after
implementation.

## Whole-product review

A capability is reviewed from discovery through recovery:

```text
discover -> understand -> configure -> act -> wait -> verify -> recover -> return
```

The review covers:

- how a person discovers the capability;
- setup on an empty machine and useful defaults;
- the first successful use;
- empty, loading, partial, failure, and success states;
- persistence across navigation, reload, and restart;
- recovery without an undocumented escape hatch;
- communication of evidence and limitations;
- what attention the product still requires afterward.

Backend presence, a settings field, a merged bundle, and isolated unit tests are
intermediate evidence. Completion requires the accepted journey through the
running product, including a failure branch and a persistence check.

## Taste review

Correctness proves that behavior satisfies a specification. It does not prove
that the experience is clear, coherent, desirable, or memorable. Taste is an
explicit owner review and cannot be replaced by a test suite or a score an
agent can optimize.

The review asks:

- Is the hierarchy immediately obvious?
- Is the primary action where the person naturally looks?
- Does the copy communicate a benefit rather than machinery?
- Is anything competing with the magical moment?
- Do empty, waiting, failure, recovery, and success feel intentional?
- Is the whole experience coherent on a phone?
- Does setup, use, evidence, and recovery feel like one product?
- What would the experience owner remove if thirty percent had to disappear?

The output is a small list of owner judgments, removals, and unresolved taste
questions. It is kept separate from functional defects so green tests cannot
be misreported as product coherence.

## Building taste through outside domains

OpenExec should not become another agent framework whose only references are
other agent frameworks. During experience work, deliberately collect useful
patterns from domains with different strengths:

- games for feedback, progression, and legible state;
- aviation for operational clarity and recovery under pressure;
- consumer devices for setup and useful defaults;
- film for narrative, pacing, and revelation;
- banking for trust, confirmation, and reversibility;
- emergency systems for escalation and recovery;
- hospitality for anticipating needs without demanding attention.

Inspiration is evidence for an option, not authority to copy a surface. Every
borrowed pattern still has to reduce the real customer's effort or strengthen
the hero experience. The distinctive opportunity is the combination of senior
engineering judgment, ADHD-aware attention design, governance, and remote
operation—not novelty in agent technology alone.

## Traceability into architecture and implementation

Once the Experience Contract is accepted, each lower-level decision must link
upward:

```text
technical choice
    -> capability it enables
    -> workflow step it supports
    -> experience requirement it satisfies
    -> customer outcome it advances
```

A technical decision with no trace is removed or explicitly classified as
platform maintenance. Platform maintenance still states which protected
experience or operating constraint it preserves.

Pre-change impact analysis includes experience blast radius:

- affected customer journeys and demo steps;
- entry points and surfaces;
- attention, tap, waiting, and recovery budgets;
- empty/loading/error/success states;
- persisted state and cross-device behavior;
- website or product claims that could become false;
- end-to-end evidence required after the change.

## Review stages

### 1. Customer Experience Review

The owner reviews the machine's recommendation and adjusts the outcome,
workflow, magical moment, feeling, and trade-offs.

### 2. Focus and Completion Review

Remove capabilities that do not strengthen the hero workflow. Review the
machine-proposed Completion Contract, accepted scope, evidence requirements,
and `Not required for Done` list. The owner accepts or refines the boundary.

### 3. Cognitive-load Review

Inspect the actual interface and wording. Record concrete experience-debt
deltas. Do not replace this with a numerical score.

### 4. Taste Review

The experience owner reviews hierarchy, coherence, benefit-led language,
intentional states, phone use, and what can still be removed. Functional tests
are evidence but cannot pass this judgment.

### 5. Architecture Review

Now choose how to deliver the accepted experience. Every significant choice
includes its customer-value trace.

### 6. Closure Review

When the accepted capability exists, enter Finish explicitly. Review remaining
conditions, park unrelated work, name blockers, and identify the next smallest
closure action. The machine may propose Done but cannot accept it.

### 7. Running Journey Review

Exercise the accepted path in the production-shaped build on its target device,
including failure, recovery, and persistence. Only this stage can mark the
experience delivered and provide evidence for an accepted Done verdict.

## Planned OpenExec integration

Implementation should be incremental and supervised.

### E0 — Manual contract and dogfooding

- Read OpenExec's accepted root `PROJECT_INTENT.md` and bind every initiative
  to its multi-repository authority and dependency boundary. A route may use a
  sibling repository only when it records how that dependency advances the
  current destination or completion condition.
- Use the proportional templates on real OpenExec and Agent Console changes.
- Write the demo before implementation on one hackathon-sized initiative.
- Record which questions were useful, repetitive, or impossible to answer.
- If the Agent Console G2 owner gate authorizes a bounded implementation slice,
  run it manually from Experience Contract through Completion Contract, phase
  changes, scope decisions, evidence, and a terminal owner verdict. Do not
  implement completion automation first.
- If that completion run is authorized, capture at least one attractive new
  idea and verify that it can be preserved without displacing the accepted
  closure action.
- Do not add a schema until dogfooding stabilizes the fields.

If G2 declines the bounded implementation slice, E0 completes with that
negative owner decision and its reasoning rather than becoming permanently
incomplete. E1–E4 proceed only if G2 separately authorizes them; F1–F3 remain
blocked until a manual completion reference run exists.

### E1 — Advisory experience triage

- Read root `PROJECT_INTENT.md` and previously accepted contracts. If the root
  artifact is absent, stop with `needs_owner`; never substitute generated
  `INTENT.md`.
- Produce a provenance-labelled Experience Contract proposal.
- Recommend missing questions, one hero workflow, one magical moment, and at
  least one removal.
- Present the proposal for owner refinement; never self-accept.
- Preserve revisions so owner corrections become future evidence.

### E2 — Durable gate

- Add explicit `proposed`, `accepted`, `superseded`, and `rejected` revisions.
- Require an accepted contract before planning an implementation initiative.
- Allow a bounded `experience_delta` path for defects and small changes.
- Keep feasibility spikes separate from accepted product direction.
- Refuse implementation when accepted `PROJECT_INTENT.md` and the proposal conflict.

### E3 — Demo and journey evidence

- Attach the Demo Contract and end-to-end journey to the accepted revision.
- Verify the journey against a production-shaped build.
- Record failure, recovery, and persistence evidence.
- Feed journey surfaces into pre-change impact and regression analysis.

### E4 — Communication derivation

- Generate website and presentation message proposals from accepted contracts.
- Show the source field behind every claim.
- Require owner approval before publishing copy.
- Detect when implementation or evidence no longer supports a public claim.

### F1 — Advisory completion contract

- Propose a proportional Completion Contract from accepted experience,
  repository evidence, and the request.
- Recommend accepted scope, `Not required for Done`, protected behavior, and
  condition-level evidence.
- Show one portfolio-level owner-attended outcome and its next closure action.
- Classify new ideas against the accepted contract and recommend quiet parking
  by default when they do not close a condition.

### F2 — Phase-aware execution

- Make Explore, Build, Finish, and Ship explicit execution policies.
- Change prompts, planning, tool authority, review criteria, and surfaced work
  according to the accepted phase.
- Require a contract revision rather than silently widening Finish-mode scope.
- Preserve owner override and the non-mandatory nature of focus.

### F3 — Durable completion authority

- Version Completion Contracts and bind evidence to normalized conditions and
  exact revisions.
- Prevent an agent from setting authoritative Done directly.
- Evaluate protected behavior, running-journey evidence, and remaining owner
  judgment before accepting Done.
- Support Done, Stop, Park, and Replace as first-class terminal decisions.
- Preserve accepted history and represent later regressions as new work.

## Success criteria

This model succeeds when:

- architecture discussions reliably begin from an accepted customer outcome;
- the machine recommends and the owner retains product judgment;
- technically present but unreachable features fail before implementation is
  called complete;
- hackathon work converges on one demonstrable transformation;
- product websites explain benefits before technology;
- people need fewer decisions, less memory, and less recovery outside the
  product;
- owner corrections are remembered and not asked again;
- one valuable owner-attended portfolio outcome is easier to resume and finish
  than starting another outcome;
- new ideas are preserved without silently expanding accepted scope;
- Done consistently names the accepted contract revision and evidence that
  supports it;
- Stop, Park, and Replace remove guilt-producing residue without hiding what
  happened.

## Sources and influences

- Steve Jobs, 1997 WWDC Q&A: start with the customer experience and work back
  to technology. A searchable transcript is available at
  <https://sebastiaanvanderlans.com/steve-jobs-wwdc-1997/>.
- Steve Jobs Archive, *Make Something Wonderful*: focus, subtraction, craft,
  process, detail, and the role of a coherent team in making products.
  <https://book.stevejobsarchive.com/>
- Steve Krug, *Don't Make Me Think, Revisited*: inspiration for reducing
  unnecessary thought and testing usability. The checklist in this document is
  an OpenExec policy synthesis, not a quotation or reproduction of the book.
  Publisher overview only:
  <https://www.pearson.com/en-us/subject-catalog/p/don-t-make-me-think-revisited-a-common-sense-approach-to-web-usability/P200000000385/9780321965516>
- NICE, *Attention deficit hyperactivity disorder: diagnosis and management*:
  environmental modifications, written instructions, visual reminders,
  structure, and shorter focus periods. These inform the owner-specific design
  reasoning; they do not prove a particular Agent Console interface.
  <https://www.nice.org.uk/guidance/ng87/chapter/recommendations>
- *Promoting the translation of intentions into action by implementation
  intentions*: evidence that concrete if-then plans can support translating
  intentions into action, including studies in populations with
  executive-control difficulties.
  <https://pmc.ncbi.nlm.nih.gov/articles/PMC4500900/>
- *Once established, goal reminders provide long-lasting and cumulative
  benefits for lower working memory capacity individuals*: evidence that
  external goal reminders can reduce goal neglect.
  <https://pubmed.ncbi.nlm.nih.gov/36201799/>
