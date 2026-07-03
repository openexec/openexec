# OpenExec Core / Module Strategy

## Purpose

OpenExec should become a small, understandable open-source core with optional
commercial modules around it.

The core product is the deterministic AI coding runtime: it takes a scoped task,
runs a bounded workflow, applies typed actions, records evidence, and produces a
code change that can be reviewed. Domain workflows such as governance, SRE,
dashboards, and enterprise integrations should be optional modules.

This keeps the open-source project useful and trustworthy while preserving a
clear commercial path for proprietary modules and hosted services.

## Product Boundary

Core should answer one question:

> Can an AI coding tool produce a scoped code change through a deterministic,
> auditable, quality-gated workflow?

Modules should answer domain-specific questions:

> Can this runtime be connected to GitHub/Jira governance, SRE infrastructure,
> dashboards, team workflows, or enterprise policy?

The core must stay boring, strict, and reliable. Modules may add workflows, but
they must not make the core harder to understand.

## Current Reality

The repository already has partial seams:

- `pkg/runtime` is a facade over planner/release execution.
- `internal/governance/service` depends on `pkg/runtime` rather than directly on
  lower-level planner/release packages.
- Governance has its own hardened status machine, evidence model, decision
  events, and policy gates.
- SRE/infrastructure tools are mostly isolated under `internal/infra`,
  `internal/approval`, and `internal/mcp/infra.go`.

The target architecture is therefore not a simple folder move. The tree still
contains many first-class concepts and several state/audit/approval planes:
release, story, task, change record, blueprint, session, run, tool call,
approval request, governance evidence, and execution audit.

The immediate goal is to enforce boundaries and reduce surface area before any
large restructure.

## Core Rule

The dependency rule has three tiers, not two. The two-tier phrasing "core must
not depend on modules" is only correct once the composition root is named —
otherwise it is red on day one, because `internal/cli` already imports
`governance`, `mcp`, `infra`, and `tui` to mount their commands.

```text
composition root  may import core AND modules   (wiring the binary is its job)
core              may import neither modules nor the composition root
modules           may import core, never each other or the composition root
```

The composition root is the thin wiring layer — `cmd/openexec` and the
command-mount code in `internal/cli` — that assembles the binary. It is allowed
to import modules; mounting `governance`/`sre`/`mcp` commands is its whole
purpose. Keep it thin: it wires, it does not implement.

Enforce this now with import checks and review discipline — not a `core/` and
`modules/` directory migration. The check is only as good as the classification
it reads, so its first artifact is a **package manifest** labelling every package
`core`, `module:<name>`, or `composition-root`, with a short `undecided` bucket
for genuinely ambiguous packages (e.g. `pkg/manager`, and the run-transport part
of `pkg/api` versus its web surface).

Suggested boundary:

```text
openexec/
  cmd/openexec          # CLI wiring
  pkg/runtime           # public runtime facade
  pkg/manager           # current pipeline orchestrator, pending clearer home
  pkg/api               # current API/WebSocket surface, likely future web module
  internal/...          # existing implementation packages
  modules/...           # introduced only when a module is actually extracted
```

The future folder shape can still be:

```text
core/
  runtime
  actions
  policy
  state
  quality
  audit

modules/
  governance
  sre
  mcp
  web
  tui
  memory
  integrations
  multiagent
```

But this should be the result of proven extractions, not the first step.

## Licensing Strategy

Keep the open-source core MIT.

Commercial/proprietary modules can live in private repositories, private Go
modules, licensed distributions, or hosted services:

- `openexec-core`: open-source deterministic AI coding runtime.
- `openexec-governance`: proprietary GitHub/Jira governance module.
- `openexec-sre`: proprietary SRE orchestration and infrastructure safety module.
- `openexec-enterprise`: hosted identity, policy, audit export, org management,
  support, and integrations.

The open-source core must be useful on its own. Commercial value should come from
enterprise workflow, compliance, governance, operability, integrations, and
support.

## Audit Moat Decision

Decide this before extracting governance.

There are two audit layers:

1. Core audit: local run timeline, typed actions, quality evidence, approvals,
   and enough provenance for developer trust.
2. Governance audit: compliance-grade workflow history, authority decisions,
   GitHub/Jira provenance, merge gates, risk acceptance, export bundles,
   retention, and auditor-facing reports.

Recommendation:

- Keep minimal run/action/evidence audit in the MIT core.
- Keep compliance-grade governance audit features in the proprietary governance
  module or enterprise service.

The moat is not only an append-only or hash-chained table. The moat is the full
governed compliance workflow around it: identity, authority, provenance, policy,
retention, export, and auditor usability.

## Core Concepts

The long-term core vocabulary should be small:

```text
WorkItem   # generic input task; no GitHub/Jira coupling
Run        # one execution attempt through a deterministic workflow
Action     # typed operation performed during a run
Evidence   # tests, lint, CI, review, generated artifacts
Approval   # human or policy decision
AuditEvent # append-only record of what happened
Policy     # sandbox/path/tool/approval decision
```

This is aspirational. Do not force governance onto new generic primitives until
those primitives have been proven elsewhere. Governance was recently hardened;
rewriting it now would add risk without immediate product value.

## KISS Module Approach

Do not build a plugin marketplace or generic runtime registry first.

Start with the simplest model:

- Modules are compiled into the binary.
- Modules are enabled or disabled by config.
- Module commands are mounted under their own namespace.
- Core does not import module implementation packages.
- Modules call core services instead of owning parallel workflow engines.

Use explicit wiring first. Add a registry only when repeated module wiring proves
the need.

Avoid this until there is a forcing function:

```go
type Module interface {
	ID() string
	Register(reg Registry) error
}
```

A registry may be useful later, but building it before the first successful
extraction is speculative.

## State Rules

Hard rules:

- SQLite is the only live source of truth.
- JSON/YAML are inputs, exports, debug artifacts, or config only.
- No module writes directly into another module's tables.
- No prompt controls workflow transitions.
- No module bypasses typed actions, policy checks, approval checks, or audit.
- New tables should use clear ownership prefixes.

Suggested future namespacing:

```text
core_runs
core_actions
core_evidence
core_approvals
core_audit_events
module_governance_changes
module_sre_plans
module_web_sessions
```

Do not rename existing tables only for aesthetics. Rename or migrate when there
is a real extraction, compatibility reason, or licensing boundary.

## Core CLI

The visible core CLI should become small:

```bash
openexec init
openexec run <task>
openexec chat
openexec status
openexec audit
openexec module list
openexec module enable <module>
openexec module disable <module>
```

Module commands should mount under namespaces:

```bash
openexec governance ...
openexec sre ...
openexec mcp serve
openexec web serve
```

Legacy and advanced commands should be hidden, deprecated, or moved behind module
namespaces when that can be done without breaking active workflows.

## What Stays In Core

Keep these in the open-source core:

- Deterministic coding workflow execution.
- Provider adapters for Claude, Codex, Gemini, and OpenAI-compatible APIs.
- Typed actions for file reads, patching, tests, linting, formatting, and git.
- Quality gates.
- Sandbox, path, tool, and approval policy checks.
- SQLite state for local runs, actions, evidence, approvals, and audit.
- Minimal chat and run UX.
- Minimal local audit export.

These are the open-source trust base.

## What Moves To Modules

Move these out only when the extraction is proven and useful:

| Area | Module | Reason |
| --- | --- | --- |
| GitHub/Jira governance | `governance` | Enterprise workflow and compliance value |
| SRE orchestration | `sre` | Powerful but not required for coding runs |
| MCP server | `mcp` | Integration surface, optional for core CLI users |
| Web dashboard/API | `web` | Product surface, optional for local CLI users |
| TUI | `tui` | Operator UX, optional |
| Memory/cache/predictive loading | `memory` | Optimization, not core correctness |
| Telegram/Twilio/botseed | `integrations` | Channel-specific integrations |
| Multi-agent/coordinator/parallel | `multiagent` | Advanced execution mode |
| BitNet/DCP/local router | `routing` or `multiagent` | Experimental until clearly load-bearing |

## First Extraction: SRE

SRE is the best first module because it is valuable, optional, and less coupled
than governance.

Move behind an `sre` module boundary:

- Terraform plan inspection.
- Saved-plan apply flow.
- Ansible/Salt/SSH allowlisted tools.
- Infrastructure approval gates.
- SRE-specific policy.
- SRE docs and examples.

Keep generic primitives in core:

- Approval request/decision abstraction.
- Policy hook abstraction.
- Minimal run evidence/audit model.

**The MCP exposure (`internal/mcp/infra.go`).** SRE is partly surfaced today
through MCP tools. SRE owns the infra logic *and* its own MCP adapter: the tool
definitions and handlers in `internal/mcp/infra.go` move **into the `sre`
module** as its MCP surface. The `mcp` module provides only the server/transport
(JSON-RPC, tool registry) and mounts tools that modules register through the
composition root — so `mcp` never imports `sre` and `sre` never imports `mcp`
(modules do not import each other; the composition root wires them). Rule of
thumb: SRE owns *what* the tool does; MCP owns *how* it is exposed. The same
seam later lets governance register its MCP tools without `mcp` depending on
`governance`.

Target UX:

```bash
openexec module enable sre
openexec sre terraform plan
openexec sre approve list
openexec sre apply <saved-plan>
```

Commercial positioning:

> OpenExec SRE turns AI-assisted infrastructure changes into governed,
> approval-gated, auditable operations.

## Governance Extraction

Governance should eventually be a module, but it should not be the first
extraction.

Move later:

- GitHub issue import.
- Jira issue import.
- Change records.
- Risk tiers.
- AI plan review.
- PR write-back.
- Merge gate extensions.
- Governance audit export.

Do not freeze governance hardening while waiting for generic core primitives.
Governance is a revenue path and should keep improving as long as it respects
the dependency rule.

Before extracting governance, decide:

- Which audit features are MIT core versus proprietary governance.
- Whether governance keeps its current evidence/decision-event model.
- Which core runtime APIs governance needs beyond `pkg/runtime`.
- Whether migration risk is justified by user-facing value.

## Safer Sequencing

Do this order instead of a big-bang restructure:

1. Enforce the dependency rule (composition root / core / module).
   First write the **package manifest** at a fixed path — `internal/quality/architecture/package_boundaries.yaml`
   (machine-readable so the check consumes it directly; it lives with the quality
   gates that enforce it, and can be rendered into `docs/PACKAGE_BOUNDARIES.md`
   for humans). It classifies every package as `core`, `module:<name>`,
   `composition-root`, or `undecided`. Then add an import check that reads it:
   core imports no module and no composition-root package; modules import core
   only, never each other; the composition root (cmd + CLI wiring) may import
   both. Governance already passes core-vs-module — the manifest is what turns the
   rule into an enforceable check and surfaces where the CLI needs to slim to a
   pure wiring layer.

2. Audit and remove dead code.
   Identify unused predictive loading, DCP/router/BitNet paths, dead parallel
   paths, old JSON state paths, and skipped tests that only cover dead behavior.

3. Shrink the CLI surface.
   Hide or namespace legacy release/story/task commands, experimental routing,
   daemon, memory, and direct blueprint commands where possible.

4. Extract SRE first.
   Use compile-time wiring and config flags. Prove one real module boundary
   before adding a registry or moving the whole tree.

5. Resolve the audit moat.
   Document which audit capabilities stay MIT and which belong to governance or
   enterprise.

6. Revisit governance extraction.
   Migrate governance only when the boundary is clear and the product value is
   worth the risk.

7. Revisit repository shape.
   Move to `core/` and `modules/` only after the module pattern is proven.

## Success Criteria

The simplification is successful when:

- `openexec run` works with all optional modules disabled.
- A new user can understand the core flow in five commands.
- SRE can be disabled without affecting coding runs.
- Governance can be disabled without affecting local execution.
- Core tests do not require GitHub, Jira, SRE, web, TUI, MCP, or integrations.
- Module tests can run independently.
- Documentation clearly separates open-source core from optional modules.
- Commercial modules add value without making the core harder to reason about.

## Status And Next: Product-Boundary Hardening

**Architecture separation is real; commercial separation is emerging but not
complete.** The hard part — removing core imports of module internals — is
largely done: the boundary ratchet baseline is empty (`package_boundaries.yaml`),
governance's MCP adapter is extracted to `internal/governance/mcpgov` behind the
`mcptool` provider seam, and core `internal/mcp` imports no module. Core is
shippable without module code.

What remains is **not another refactor** — it is small product-boundary
hardening. Two gaps block calling this a clean commercial separation:

### H1. Module enable/disable + license gates

Modules are wired implicitly today:

- Governance is **always** registered in `mcp-serve`
  (`cli/mcp_serve.go` calls `srv.RegisterProvider(mcpgov.New())` unconditionally).
- SRE is enabled by mere **presence** of `.openexec/infra.yaml`.

That is fine functionally, but a paid module needs an explicit gate. Add module
config (e.g. an `openexec.yaml` `modules:` block or per-module enable flags) that
the composition root reads before registering a provider, and a place for a
license/entitlement check to hook in. Acceptance: a module can be turned off by
config and its tools then do not appear in `tools/list`; core behavior is
unchanged with all modules off.

### H2. Reserved core tool names + provider collision checks

`RegisterProvider` is last-write-wins with no checks
(`internal/mcp/provider.go`: `s.providerTools[name] = t`), and `dispatchProvider`
runs **before** the core `backlog_*`/`memory_read` switch — so a module tool can
**shadow a core tool at call time**, and two modules can silently clobber each
other's names. Before treating the module API as stable:

- Maintain a reserved set of core tool names; reject registration of a provider
  tool that collides with a core name.
- Reject cross-module duplicate names (fail loud at registration, not silently
  last-write-wins).
- Optionally namespace module tools (e.g. `gov.*`) to make collisions
  structurally impossible.

Acceptance: registering a colliding tool fails at startup with a clear error;
`schema_audit`-style test asserts no provider name intersects the core set.

### H3. License/packaging decisions

Decide how proprietary modules are distributed (build tag, separate repo/module,
or runtime license key) and how the MIT core is packaged without them. This is a
product decision, not a code change — record it before the first paid release.

Sequence: H1 and H2 are small, self-contained hardening tasks that can land any
time; H3 gates the first commercial release. None require reopening the boundary
work.

## Business Model

Recommended product split:

| Product | License | Buyer |
| --- | --- | --- |
| OpenExec Core | MIT/open-source | Developers, open-source users |
| Governance Module | Proprietary | Engineering managers, regulated teams |
| SRE Module | Proprietary | Platform/SRE teams |
| Enterprise Service | Proprietary/SaaS | Organizations needing audit, policy, integrations |

This gives users a real open-source tool while preserving a clear commercial
path:

- Open-source core builds trust and adoption.
- Proprietary modules solve expensive organizational problems.
- Hosted enterprise service packages identity, policy distribution, audit export,
  support, and integrations.

## Guiding Principle

Core produces high-quality code through deterministic execution and evidence.

Modules add domain workflows.

Favor boundaries, deletion, and namespacing before abstraction.
