# Documentation index

**Status:** current. Verified against the code on 2026-08-17.

What is built, what is planned, and what is only a record of a moment. Every
document also carries its own status line; this table exists so you do not have
to open thirty files to find out which kind you are reading.

Authority depends on the question. An owner-authored root `PROJECT_INTENT.md`
outranks every derived product, experience, architecture, and implementation
artifact; OpenExec does not yet have that file. A wizard-generated root
`INTENT.md`, if present, is a derived execution specification and cannot
substitute for it; OpenExec has no such file as of 2026-08-17.
For current implementation facts, `CLAUDE.md` at the repository root is the
working brief for agents and `docs/ARCHITECTURE.md` is the architectural map a
study story must write — and that map currently predates several shipped
subsystems, which its own header now says.

If a document and the code disagree, the code is right and the document is a
bug. Fix it in the same change, or move it under *Historical record* with the
date it stopped being true.

## Built — describes what exists

| Document | Subsystem |
|---|---|
| [ARCHITECTURE.md](ARCHITECTURE.md) | Orchestration model, blueprint engine, provider adapters. **Incomplete**: predates light mode, skills, infra tools and symbol tools — see its header for where each is documented. |
| [STATE_MACHINE.md](STATE_MACHINE.md) | The stage sequence the blueprint engine runs. |
| [LIGHT_MODE.md](LIGHT_MODE.md) | The story backlog over MCP: `openexec mcp-serve` and the `backlog_*` tools. |
| [SKILLS_SYSTEM.md](SKILLS_SYSTEM.md) | The skills engine. Was the design document; shipped. Does **not** cover the propose-then-approve gate — its header says where that lives. |
| [SKILLS_QUICKSTART.md](SKILLS_QUICKSTART.md) | Using skills in five minutes. |
| [SRE_ORCHESTRATION_ROADMAP.md](SRE_ORCHESTRATION_ROADMAP.md) | The infra command registry. Phases 1–4 are implemented and marked as such in the document; later phases are proposals. |
| [SECURITY_MODEL.md](SECURITY_MODEL.md) | How an agent is contained when it touches real infrastructure. Companion to the phases above. |
| [CONTEXT_PRUNING.md](CONTEXT_PRUNING.md) | File selection and ranking. |
| [KNOWLEDGE_BASE.md](KNOWLEDGE_BASE.md) | The Deterministic Control Plane. Built, opt-in, off by default. |
| [EXECUTION_PROVIDER_CONTRACT.md](EXECUTION_PROVIDER_CONTRACT.md) | `pkg/execution.Provider`, the public execution boundary. |
| [CONVERSATIONAL_ORCHESTRATION.md](CONVERSATIONAL_ORCHESTRATION.md) | `openexec chat`. Written in March; details indicative, code authoritative. |
| [REUSABILITY_LIBRARIES.md](REUSABILITY_LIBRARIES.md) | How `blueprints/`, `intent-compiler/packs/` and the skills engine relate. Spans three repositories. |
| [API_REFERENCE.md](API_REFERENCE.md) | The daemon's HTTP and WebSocket surface. **Does not cover the MCP tools** — `internal/mcp/` is the list. |
| [CONFIGURATION.md](CONFIGURATION.md) | `openexec.yaml`. **Partial**: newer configuration lives elsewhere, listed in its header. |

## Mixed — shipped in part, proposed in part

Each of these states its own boundary in its status line. Read that line before
citing anything below it as built.

| Document | Boundary |
|---|---|
| [KNOWLEDGE_V2_PLAN.md](KNOWLEDGE_V2_PLAN.md) | Implementation complete; two host-constrained release checks outstanding. |
| [KNOWLEDGE_V3_PLAN.md](KNOWLEDGE_V3_PLAN.md) | V3.0 implemented; V3.1 onward proposed, awaiting review. |
| [SYMBOL_TOOLS_REVIEW.md](SYMBOL_TOOLS_REVIEW.md) | Review rounds 1–5 resolved; committed and deployed. Spans this repository and agent-console. |
| [REPOSITORY_POINTER_GRAPH_PLAN.md](REPOSITORY_POINTER_GRAPH_PLAN.md) | V1 plan plus a status correction; read with the two knowledge plans. |

## Planned — none of this exists yet

| Document | State |
|---|---|
| [EXPERIENCE_FIRST_OPERATING_MODEL.md](EXPERIENCE_FIRST_OPERATING_MODEL.md) | Owner-directed Experience First and Finish What Matters operating model, with E0–E4 and F1–F3 implementation stages. Documented, not enforced. OpenExec still lacks its owner-authored root `PROJECT_INTENT.md`. |
| [IMPACT_MANIFEST_FOR_CONSOLE_PLAN.md](IMPACT_MANIFEST_FOR_CONSOLE_PLAN.md) | Plan, not started (2026-08-04). |
| [future/](future/) | Explicitly speculative: operational memory layer, policy sandbox contract, timeline replay. |

## Historical record

| Document | Date |
|---|---|
| [PROJECT_AND_PRO_REVIEW_2026-07-10.md](PROJECT_AND_PRO_REVIEW_2026-07-10.md) | 2026-07-10 review of openexec and openexec-pro. True on its date. |
| [archive/](archive/) | Superseded designs: the five-phase pipeline, JSON storage, earlier intent refactors. Kept so a reader can tell what was replaced and why. |

## Reference and process

[GET_STARTED.md](GET_STARTED.md) · [AGENTS.md](AGENTS.md) ·
[VISUALIZATION_GUIDE.md](VISUALIZATION_GUIDE.md) ·
[architecture/](architecture/) — 19 ADRs plus runtime references, each with its
own status.
