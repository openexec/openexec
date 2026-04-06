# ADR-002: Toolset Filtering for API Provider Requests

Status: **Accepted (Phase A implemented)**
Date: 2026-04-06

## Context

When OpenExec runs a task through the API provider path (`createAPIAgenticLoop` in
`internal/pipeline/pipeline.go`), every chat completion request to the frontier model
includes the **full hardcoded tool list** from `loop.BuildAPIToolDefinitions()`:

- `read_file`
- `write_file`
- `run_shell_command`
- `git_apply_patch`

The tool definitions are sent on **every turn** of the agentic loop, repeated verbatim
in each request body. For tasks that obviously only need a subset (e.g. a documentation
research task that only ever needs `read_file`), the unused tool schemas are wasted
prompt tokens, paid on every turn.

Meanwhile the codebase already has all the pieces of a router-driven filter:

- `internal/router/route.go` `Route()` returns a `RoutingPlan` whose `Toolset` field
  identifies a specific toolset name (e.g. `repo_readonly`, `coding_backend`).
- `internal/toolset/registry.go` defines six default toolsets keyed by name with
  per-toolset tool name lists and phase metadata.
- `runBlueprintMode` in `pipeline.go` already calls `Route()` once per task — but only
  consumes `RepoZones`, `KnowledgeSources`, and `Sensitivity`. The `Toolset` field is
  computed and then dropped on the floor.

The wiring exists; nothing reads the result.

## Decision: Phase A (this ADR)

Wire the existing `RoutingPlan.Toolset` output through to the API request building
step. When the new opt-in `execution.toolset_filtering` config flag is true and the
selected toolset is non-empty, only the intersection of the toolset's declared tool
names and the API runner's known tool definitions is sent in `req.Tools`. When the
flag is false (the default), behaviour is identical to today.

The change is intentionally narrow:

1. **`internal/loop/api_runner.go`** — refactor the hardcoded `BuildAPIToolDefinitions()`
   into a `name → ToolDefinition` map, plus a `BuildAPIToolDefinitionsFor(names)`
   helper that returns only the requested subset. Unknown names are silently skipped
   (defensive: the toolset registry can declare aspirational tool names like `glob`
   that the API runner does not actually implement yet).
2. **`internal/pipeline/pipeline.go`** — add `SelectedToolset string` to `Config`,
   pass a real toolset registry to `router.Route(...)` (currently nil), and store
   `plan.Toolset` into `cfg.SelectedToolset`. In `createAPIAgenticLoop`, when the
   feature flag is set and `SelectedToolset` is populated, look up the toolset in the
   registry and pass `BuildAPIToolDefinitionsFor(toolset.Tools)` instead of the full
   list. Log the resulting filter (e.g. `[Pipeline] Filtered tools: toolset=repo_readonly,
   1/4 tools`).
3. **`internal/agent/coordinator.go`** — apply the same filter at the second
   `BuildAPIToolDefinitions` callsite so the multi-agent worker path benefits as well.
4. **`internal/project/project.go`** — add `ToolsetFiltering bool` with json tag
   `toolset_filtering` to `ExecutionConfig`. Default false.

### Why opt-in by default

The toolset registry is currently aspirational: most of its tool name lists reference
tools that have no `BuildAPIToolDefinitions()` entry yet (e.g. `glob`, `grep`,
`git_status`, `web_fetch`). Filtering aggressively could reduce the model's available
tools to a single entry, which may starve some workloads. Opt-in lets users measure
token savings on their own workload before flipping the flag globally.

### Why per-task and not per-turn

The router's intent classification is best-effort and a task description rarely shifts
mid-task in a way that would change the toolset choice. The per-task scope matches
where `Route()` is already called and avoids invoking the local router on every turn
of a long agentic loop. If per-stage filtering becomes desirable later, the
infrastructure exists (`toolset.Selector.SelectForPhase`) — see Phase B below.

### What this does NOT change

- The CLI runner path (`createAgenticLoop`) is unaffected — Claude CLI continues to
  receive whatever tools the runner exposes via stdio.
- Tool execution (`MCPToolHandler.ExecuteTool`) is untouched. Sending fewer tool
  definitions just narrows what the model can choose; the dispatcher behind the
  chosen tool is the same.
- The frontier model still selects which tool to call per turn. We are not picking
  tools locally; we are only restricting the menu.
- The deterministic router and BitNet router behave the same way — both feed
  `intent.ToolName` into `selectToolset` which combines a keyword selector with
  intent-based registry lookup. BitNet would change *what* gets selected, not whether
  filtering happens.

## Decision: Phase B (deferred)

Phase B is **local pre-resolution of deterministic lookups**. Instead of (or in
addition to) restricting the frontier model's tool menu, OpenExec would execute
deterministic read-only tools locally before the implement stage and inject their
results as enriched context. The frontier model would never see the lookup tools at
all — only the resolved code/snippets/symbols.

Concretely, Phase B would:

1. Add a `pre_resolve` step before the `implement` stage that:
   - Asks the local router (BitNet or DeterministicRouter) what lookups are needed
     for the task.
   - Executes them locally via the existing `MCPToolHandler.ExecuteTool` paths.
   - Stores the results in the `ContextPack` that `implement` already receives.
2. Extend the existing `predictiveLoader` (`internal/predictive`) to handle structured
   lookups (find_symbol → file:line:snippet), not just predicted file paths.
3. Build a real symbol/pointer index for the project (tree-sitter, ctags, gopls,
   pyright, etc.) so that `code_symbols` is more than a string label in
   `route.go:rankKnowledgeSources`.

Phase B is genuinely architectural and touches the symbol indexing story, which is
its own subproject. It is deferred until Phase A has been measured against a real
workload and the token savings justify the cost.

### What Phase B would buy

For workloads where the model spends the bulk of its turns on simple lookups
("read X, find symbol Y, list dir Z"), Phase B can collapse those turns to zero
frontier calls — the resolved data shows up as context and the model only does the
reasoning step. For implementation-heavy workloads where every turn is judgment
work, Phase B does very little.

### What Phase B would cost

- A real symbol index requires per-language tooling and per-project indexing.
- The `pre_resolve` stage needs careful design so it doesn't run unbounded local
  lookups for vague task descriptions.
- The contract between local-resolved context and the frontier model needs to be
  stable enough that the model can rely on the injected context being correct
  (today, the model can re-read a file if it's unsure; with Phase B, that
  re-read costs an extra round-trip).

## Consequences

**Phase A delivered** (this ADR):

- Token usage on the API path drops by approximately the size of the unused tool
  schemas, multiplied by the number of turns. For a `repo_readonly`-routed task,
  ~3/4 of the tool schema bytes go away on every turn.
- Behaviour is unchanged for existing projects (opt-in).
- Adds a meaningful integration point for BitNet: with the flag on, BitNet's
  intent classification feeds directly into request shaping rather than being a
  side-effect-free advisor.
- The toolset registry is now a load-bearing component on the API path, not just
  documentation.

**Phase B deferred:**

- Tracked in this document. No prior commitment to a timeline. Reconsider once
  Phase A measurements are in.

## Verification

- Unit test coverage for `BuildAPIToolDefinitionsFor` (empty input, single tool,
  multiple tools, unknown tool name skipped).
- Integration test asserting that with the flag enabled and a known task description,
  the constructed `APIRunnerConfig.Tools` contains only the toolset's tools.
- Existing tests for the unfiltered path continue to pass without modification.
