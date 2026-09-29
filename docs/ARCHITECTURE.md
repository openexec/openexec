# OpenExec Architecture

**Version:** 1.0  
**Last Updated:** 2026-03-31

**Status:** built, but **incomplete as a map of the current system.** What it
describes — the orchestration model, the blueprint engine, provider adapters —
is accurate. What it predates is everything shipped since 2026-03-31, none of
which appears below. If you are looking for one of these, this is not the file:

| Subsystem | Where it is documented |
|---|---|
| Light mode: the story backlog over MCP (`openexec mcp-serve`, `backlog_*` tools) | [LIGHT_MODE.md](LIGHT_MODE.md) |
| Skills, and the propose-then-approve trust boundary (`skill_propose`, `openexec skills approve`) | [SKILLS_SYSTEM.md](SKILLS_SYSTEM.md), [SKILLS_QUICKSTART.md](SKILLS_QUICKSTART.md) |
| SRE/infra command registry (`ansible_run_playbook`, `terraform_plan`/`terraform_apply`, approval gate) | [SRE_ORCHESTRATION_ROADMAP.md](SRE_ORCHESTRATION_ROADMAP.md), [SECURITY_MODEL.md](SECURITY_MODEL.md) |
| Repository symbol tools and the pointer graph (`symbol_find`, `symbol_read`, `symbol_relations`) | [SYMBOL_TOOLS_REVIEW.md](SYMBOL_TOOLS_REVIEW.md), [REPOSITORY_POINTER_GRAPH_PLAN.md](REPOSITORY_POINTER_GRAPH_PLAN.md), [KNOWLEDGE_V2_PLAN.md](KNOWLEDGE_V2_PLAN.md), [KNOWLEDGE_V3_PLAN.md](KNOWLEDGE_V3_PLAN.md) |
| Vertical slices, afk/hitl execution modes, project phases, smart-zone budget | `CLAUDE.md` at the repository root |

Two consequences worth stating plainly. `CLAUDE.md` makes this file the one a
study story must write, so an agent is told to trust it — the gaps above are
therefore load-bearing, not cosmetic. And a reader who takes this document as
the whole system will conclude that several shipped subsystems do not exist.

## Executive Summary

OpenExec is an **AI CLI orchestration platform**, not an LLM client. It wraps existing AI CLI tools (Claude Code, Codex CLI, Gemini CLI) with production-grade infrastructure for deterministic, reliable, and safe AI-assisted development.

**Key Principle:** OpenExec doesn't implement LLM clients - it orchestrates them.

---

## Core Architecture

### High-Level Flow

```
User Intent
    │
    ▼
┌─────────────────────────────────────────────────────────────────┐
│  OpenExec Orchestration Layer                                    │
│  ├─ Input Processing (intent parsing)                            │
│  ├─ Context Assembly (knowledge index, file pruning)            │
│  ├─ Quality Gates (lint/test/format validation)                 │
│  ├─ Blueprint Execution (deterministic workflows)               │
│  │   ├─ Stage 1: Gather Context                                │
│  │   ├─ Stage 2: Implement (spawn AI CLI)                      │
│  │   ├─ Stage 3: Validate (lint/test)                          │
│  │   └─ Stage 4: Review (secondary AI)                         │
│  ├─ Checkpointing (crash recovery)                              │
│  ├─ Memory Extraction (pattern learning)                        │
│  └─ Multi-Agent Coordination (parallel execution)               │
└─────────────────────────────────────────────────────────────────┘
    │
    │ Spawns subprocess via exec.Command()
    ▼
┌─────────────────────────────────────────────────────────────────┐
│  External AI CLI Process                                         │
│  ├─ Claude Code CLI (claude)                                    │
│  ├─ OpenAI Codex CLI (codex)                                    │
│  └─ Google Gemini CLI (gemini)                                  │
└─────────────────────────────────────────────────────────────────┘
    │
    │ Communicates with
    ▼
┌─────────────────────────────────────────────────────────────────┐
│  LLM Provider API (cloud)                                        │
│  ├─ Anthropic API                                               │
│  ├─ OpenAI API                                                  │
│  └─ Google API                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## Subsystem Details

### 1. Model Routing (`internal/runner/`)

**Status: Active, always-on** — Deterministic routing runs on every execution.

**Purpose:** Map abstract model names to concrete CLI commands.

**Key Files:**
- `internal/runner/runner.go` - Model resolution logic
- `internal/runner/runner_test.go` - Resolution tests

**Resolution Logic:**

```go
// Pseudo-code from internal/runner/runner.go
func Resolve(model string) (cmd string, args []string, err error) {
    model = strings.ToLower(model)
    
    // Claude family → "claude" CLI
    if strings.Contains(model, "claude") || 
       strings.Contains(model, "sonnet") ||
       strings.Contains(model, "opus") ||
       strings.Contains(model, "haiku") {
        return "claude", defaultClaudeArgs(), nil
    }
    
    // OpenAI/Codex family → "codex" CLI
    if strings.Contains(model, "gpt") ||
       strings.Contains(model, "codex") ||
       strings.Contains(model, "openai") {
        return "codex", defaultCodexArgs(), nil
    }
    
    // Gemini family → "gemini" CLI
    if strings.Contains(model, "gemini") {
        return "gemini", defaultGeminiArgs(), nil
    }
    
    return "", nil, fmt.Errorf("unknown model: %s", model)
}
```

**Supported Models:**

| Model Name | Resolves To | CLI Required |
|------------|-------------|--------------|
| `claude`, `claude-3`, `sonnet`, `opus`, `haiku` | `claude` | `@anthropic-ai/claude-code` |
| `gpt-4`, `gpt-3.5-turbo`, `codex` | `codex` | `@openai/codex` |
| `gemini`, `gemini-pro`, `gemini-ultra` | `gemini` | Google Gemini CLI |

---

### 2. Process Management (`internal/loop/`)

**Purpose:** Spawn and manage AI CLI subprocesses.

**Key Files:**
- `internal/loop/process.go` - Process spawning
- `internal/loop/loop.go` - Main execution loop

**Process Spawning:**

```go
// From internal/loop/process.go
func StartProcess(ctx context.Context, cfg Config) (*Process, error) {
    // Resolve model to CLI command
    name, args := buildCommand(cfg)
    
    // Spawn subprocess
    cmd := exec.CommandContext(ctx, name, args...)
    cmd.Dir = cfg.WorkDir
    
    // Set up pipes for stdout/stderr
    stdoutPipe, _ := cmd.StdoutPipe()
    stderrPipe, _ := cmd.StderrPipe()
    
    // Start the process
    if err := cmd.Start(); err != nil {
        return nil, fmt.Errorf("start process: %w", err)
    }
    
    return &Process{
        cmd:    cmd,
        Stdout: stdoutPipe,
        Stderr: stderrPipe,
    }, nil
}
```

**Key Point:** OpenExec does NOT implement LLM APIs. It shells out to the CLIs.

---

### 3. Blueprint Engine (`internal/blueprint/`)

**Status: Active, always-on** — Runs on every blueprint-mode execution.

**Purpose:** Deterministic workflow execution.

**Key Files:**
- `internal/blueprint/engine.go` - Core engine
- `internal/blueprint/stage.go` - Stage definitions

**Blueprint Structure:**

```yaml
# Example blueprint
name: feature-implementation
stages:
  - name: gather_context
    agent: claude-3-sonnet
    task: "Analyze codebase and identify relevant files"
    
  - name: implement
    agent: claude-3-sonnet  
    task: "Implement the feature"
    depends_on: [gather_context]
    
  - name: lint
    command: "go vet ./..."
    blocking: true
    
  - name: test
    command: "go test ./..."
    blocking: true
    
  - name: review
    agent: codex
    task: "Review implementation for bugs"
    depends_on: [test]
```

---

### 4. Quality Gates (`internal/quality/`)

**Status: Opt-in** — Enabled via `quality_gates_v2` in config.json.

**Purpose:** Block execution on lint/test/format failures.

**Key Files:**
- `internal/quality/gates.go` - Gate definitions and execution

**Gate Types:**
- **Lint:** Static analysis (go vet, eslint, flake8)
- **Test:** Test suites (go test, pytest, jest)
- **Format:** Format checkers (gofmt, black, prettier)
- **Security:** Security scans (gosec, bandit)
- **Custom:** Arbitrary commands

**Gate Modes:**
- **Block:** Prevent execution on failure
- **Warn:** Allow execution with warning
- **Ignore:** Silently ignore failures

---

### 5. Checkpointing (`internal/checkpoint/`)

**Status: Opt-in** — Enabled via `checkpoint_enabled` in config.json.

**Purpose:** Crash recovery and state persistence.

**Key Files:**
- `internal/checkpoint/manager.go` - Checkpoint management

**Features:**
- Automatic checkpoint after each stage
- File state hashing (SHA256)
- Stale detection (detects file changes)
- Corruption detection (checksum verification)
- Resume from last valid checkpoint

---

### 6. Context Pruning (`internal/context/`)

**Status: Active, always-on** — Runs after context assembly on every execution.

**Purpose:** Intelligent file selection to reduce token usage.

**Key Files:**
- `internal/context/pruner.go` - Pruning logic

**Algorithm:**
1. Score files by relevance to task
2. Apply token budget
3. Select top-N most relevant files

**Scoring Factors:**
- Symbol matching (10x weight)
- Content similarity (5x weight)
- Path relevance (3x weight)
- Recency (2x weight)

**Results:** 70-95% token reduction

---

### 7. Predictive Loading (`internal/predictive/`)

**Status: Opt-in** — Enabled via `predictive_load` in config.json.

**Purpose:** Pre-load files before LLM asks for them.

**Key Files:**
- `internal/predictive/loader.go` - Loading logic

**How It Works:**
1. Analyze task description
2. Extract symbols (CamelCase, snake_case)
3. Match symbols to files
4. Pre-load likely files into cache
5. Serve from cache when LLM requests

**Benefit:** Eliminates round-trips between LLM and filesystem.

---

### 8. Memory System (`internal/memory/`)

**Status: Opt-in** — Enabled via `memory_enabled` in config.json.

**Purpose:** Learn and apply patterns across sessions.

**Key Files:**
- `internal/memory/system.go` - Memory management
- `internal/memory/manager.go` - Entry management

**Layers:**
1. **Managed Memory:** System-curated patterns
2. **User Memory:** User-defined preferences
3. **Project Memory:** Project-specific patterns
4. **Local Memory:** Session-only context

---

### 9. Caching (`internal/cache/`)

**Status: Opt-in** — Enabled via `cache_enabled` in config.json.

**Purpose:** Avoid redundant computation.

**Key Files:**
- `internal/cache/knowledge.go` - Knowledge cache
- `internal/cache/tools.go` - Tool result cache

**Cache Levels:**
- Knowledge cache (symbol lookups)
- Tool result cache (idempotent tools)
- SQLite-backed with TTL

---

### 10. Planner prompt rules & planning gate (`internal/planner/`, `internal/cli/release.go`)

**Purpose:** Turn an intent document into goals/stories/tasks, and keep unverifiable plans out of the backlog.

**Module map:**
| File | Role |
|------|------|
| `internal/planner/prompt.go` | Prompt constants: `StoryGenerationPrompt` (full plan, rules 1–12), `CompactStoryGenerationPrompt` (one story, rules 1–6), `StoryReviewPrompt`, `StoryFixPrompt`, `WizardSystemPrompt` |
| `internal/planner/planner.go` | `Planner.GeneratePlan` / `GenerateCompactPlan` render the prompts and parse the JSON response |
| `internal/planner/lint.go` | `LintVerificationScript`, `LintPlanVerification` — deterministic false-green detector (warning-only); `StaleBaseRefIssue` — bare `main`/`master` base-ref detector used as a hard gate |
| `internal/planner/lint_test.go` | `TestLintVerificationScript` (false-green vs sound samples), `TestStaleBaseRefIssue` (stale vs sound base refs), `TestLintPlanVerification` (keyed by story/task id) |
| `internal/planner/planner_test.go` | `TestStoryPrompt_RequiresOriginDefaultRef` — checks rule 8 of the prompt as rendered by `GeneratePlan` carries the `origin/<default>` clause |
| `pkg/runtime/runtime.go` | Public wrappers `LintPlanVerification`, `RemapPlanIDs` for embedders |
| `pkg/manager/planner.go` | `Manager.Plan`: intent validation → `GeneratePlan`/`GenerateCompactPlan` → optional `ReviewPlan` → plan artifact → `importBoundPlan` (`preparePlanIDs` → `RemapPlanIDs`); a non-empty `RequestID` diverts to `replayReviewedPlan` |
| `pkg/manager/planner_replay.go` | `replayReviewedPlan`: durable reviewed planning (generate/compact → review → refine loop → `rel.ImportReviewedPlan`) with retained receipts in `run_steps` |
| `internal/cli/release.go` | `storyImportCmd` (`openexec story import [file]`) — hosts the PLANNING GATE |
| `internal/cli/release_test.go` | `TestReleaseCmd` drives `release create/show/finish`, `story create/list`, `task create/approve`, `goal verify`; plus `statusIcon`, config loading and `getReleaseManager`; `TestImport_PlanningGate_RejectsStaleBaseRef` drives `story import --dry-run` through the stale-base check for each file shape (with goals, goal-less object, legacy bare array): story-level reject, task-level reject naming the task id, `origin/main` pass |

**Rule rendering.** The prompts are static Go raw strings filled with `fmt.Sprintf`: `GeneratePlan` renders `StoryGenerationPrompt` with two `%s` slots (optional PRD block, then the intent text); `GenerateCompactPlan` renders `CompactStoryGenerationPrompt` with one `%s` (intent). No project configuration reaches the prompt — rules are identical for every repository.

**Rule 8 (VERIFIABILITY)** in `StoryGenerationPrompt` requires every story to carry an executable `verification_script` that checks its linked goal and exits non-zero on failure, and forbids false-green shapes: `|| <fallback>` after a test/assertion, `A && B || C`, `2>/dev/null` on the checked command, `grep -q` piped into another command, and non-specific assertions. Its last bullet is the **`origin/<default>` clause**: a script that compares against the default branch MUST use the remote ref `origin/<default>` (e.g. `git diff --name-only origin/main...HEAD`), NEVER a bare local branch name such as `main` — task worktrees are synced to `origin/<default>` and do not advance the local branch, so a stale local `main` fails correct work or passes wrong work. The compact prompt's rule 4 is a short form of the verifiability requirement and does not carry this clause. The deterministic stale-base check enforces it on every import route regardless of which prompt produced the plan — see **Plan import routes** below.

**`LintVerificationScript(script string) []string`** matches four regexes (`falseGreenPatterns`): a test/grep command followed by `||`; `2>/dev/null`; `grep -q… |`; and `&& … ||`. Empty scripts return nil; an empty result means "no known anti-pattern", not "sound". **`LintPlanVerification(plan) map[string][]string`** runs it over every story and task script and returns issues keyed by story/task id. It never fails the CLI import, but it is not purely advisory: `Planner.ReviewPlan` (`internal/planner/review.go`) calls it after parsing the reviewer's verdict and forces `Approved=false` (appending `; verification lint refused: …` to the assessment) when it returns any issue, so on the reviewed `Manager.Plan` routes a false-green script blocks import. It is also exported through `pkg/runtime` for embedders. `LintPlanVerification` does **not** call `StaleBaseRefIssue`.

**PLANNING GATE (hard fail)** lives in `storyImportCmd.RunE`, after schema-version checks (accepted: `1.0`, `1.1`, `legacy` bare array; missing version only warns) and before the dry-run print or any DB write. It runs only when the file has `goals`; for each goal it returns an error when:
1. `PLANNING GATE FAILED: Primary goal <id> (<title>) has no supporting stories` — no story has that `goal_id`;
2. `PLANNING GATE FAILED: Primary goal <id> (<title>) has no stories with a verification_script` — none of those stories has a non-empty script.

3. **Stale-base check (hard fail).** Outside the goals branch, so it applies to every file shape (goal-less objects and legacy bare arrays included), the gate runs `planner.PlanStaleBaseRefIssues` (`StaleBaseRefIssue` per script) over every story's `verification_script` and every task-object's `verification_script` (task entries that are bare id strings are skipped). The first owner in sorted order returns `PLANNING GATE FAILED: story <story-id>: <issue>` or `PLANNING GATE FAILED: story <story-id> task <task-id>: <issue>`, where `<issue>` is ``verification script diffs against the bare local `main` ref; use `origin/main` — candidate worktrees are synced to origin/<default>, where the local `main` is stale or missing`` (with `master` substituted when that is the ref found).

`StaleBaseRefIssue(script string) string` (`internal/planner/lint.go`) tokenizes the script as shell (quotes, comments, command separators) and inspects only git revision arguments: positionals of `git diff|log|show|merge-base|rev-list|rev-parse|cherry|range-diff` before `--`, skipping global and subcommand option values. Each revision is split on `..`/`...` and an endpoint that is exactly `main` or `master` is refused; `origin/main`, `upstream/main`, `feature-main` pass, as do main/master in comments, in data for other commands (`grep -Fq 'main...HEAD'`) and as pathspecs after `--`. It returns `""` when the script is acceptable. Apart from this check the CLI gate never inspects script content — it does not call the false-green linter. `openexec plan` (`internal/cli/plan.go`) prints its own `PLANNING GATE FAILED:` banner, but that one reports intent-validation issues from `Manager.Plan`, which does not run the goal-coverage gate; its stale-base check fails as an import error (routes 2–6 below).

**Plan import routes.** Every code path that persists planner-generated goal/story/task rows, and whether it runs `StaleBaseRefIssue` today. Routes 2–6 run it through `planner.PlanStaleBaseRefError` (`internal/planner/stale_base.go`) at the last step before persistence, so neither a reviewer's approval nor a retained receipt bypasses it; `TestManagerPlan_RejectsStaleBaseRef` (`pkg/manager/planner_stale_base_test.go`) covers them at story and task level.

| # | Route | Entry points | Row writer | Script checks before write | `StaleBaseRefIssue` |
|---|-------|--------------|------------|----------------------------|---------------------|
| 1a | CLI story import, file has non-empty `goals` | `openexec story import [file]` (`storyImportCmd.RunE`, `internal/cli/release.go`) | `mgr.CreateGoal` / `mgr.CreateStory` / task creation in the same `RunE` | Goal coverage + stale-base check (story and task-object scripts) | **Yes** — `PLANNING GATE FAILED` |
| 1b | CLI story import, no `goals` (object without goals, or legacy bare array → `schema_version` `legacy`) | same command | same writer | Stale-base check only (goal coverage sits inside `if len(sf.Goals) > 0`; the stale-base check runs after it) | **Yes** — `PLANNING GATE FAILED` |
| 2 | Non-reviewed auto-import | `openexec plan` (`AutoImport: true`, no review); `POST` plan handler (`pkg/api/handlers.go` `handlePlan`) with `auto_import` and no `request_id`/`review` | `Manager.importBoundPlan(plan, false)` (`importPlan` is a thin wrapper with no callers) → `rel.CreateGoal`/`CreateStory`/`CreateTask` | `plan.Validate()` (structure), then the stale-base check first in `importBoundPlan`, before `preparePlanIDs` | **Yes** — import error |
| 3 | Reviewed `Manager.Plan` without a `RequestID` (full or compact) | `Manager.Plan` with `Review` + `AutoImport` | `importBoundPlan(plan, true)` (refuses if `preparePlanIDs` changed IDs after review) | `ReviewPlan` → `LintPlanVerification` (false-green), the stale-base check and `LintHumanBoundaries` force `Approved=false`; `importBoundPlan` re-checks stale base | **Yes** — review refusal, then import error |
| 4 | Reviewed replay (`RequestID` set) — fresh generate or compact (`planner_replay.go`, `retained.Result == nil` block) | `Manager.Plan` → `replayReviewedPlan` (the route the PR #63 review reports Agent Console's task loop uses, with `Review` + `AutoImport`) | `rel.ImportReviewedPlan` (`internal/release/reviewed_plan_import.go`, atomic insert of `reviewedPlanRows`) | `ReviewPlan` refusal as in route 3 (its assessment names the owner and the `origin/` fix, which feeds refinement); stale-base check again before `ImportReviewedPlan` | **Yes** — review refusal, then import error |
| 5 | Reviewed replay — refined plan (`RefinePlan` after a rejected review, bounded by `ReviewLimit`/`MaxReviewCycles`) | same | `rel.ValidatePlanIdentities` (identities only) at refinement, then `rel.ImportReviewedPlan` once a later review approves | With `AutoImport`, the stale-base check runs on the refined plan before `preparePlanIDs`/`ValidatePlanIdentities` and fails the request before re-review | **Yes** — error after refinement |
| 6 | Reviewed replay — retained result (receipt already holds an approved plan+review; replay re-verifies artifact/review hashes and imports) | same, repeated call with the same `RequestID` | `rel.ImportReviewedPlan` | The stale-base check runs before `ImportReviewedPlan` even on an approval recorded before the rule existed; it refuses without rewriting the receipt or plan artifact | **Yes** — import error |

Not plan imports (single rows, no planner output): `openexec story create` (`release.go` `CreateStory` at the story-create command) and `backlog_add_task` (`internal/mcp/backlog.go`, rolling `US-MAINT` story). `openexec doctor` calls `Manager.Plan` with `AutoImport: false` and persists nothing.

Consequence: on every route above (1a–6) a plan whose story- or task-level script uses a bare `main...HEAD` is refused before any row is written, and the diagnostic names the owner (`story US-1` or `story US-1 task T-2`) and the `origin/main` fix.

**Default branch threading.** None: no project setting names the default branch to the planner. The only related setting is `base_branch` (`project.ProjectConfig.BaseBranch`, `release.Config.BaseBranch`, default `"main"`), consumed by the release manager and `safe_commit`; it is never passed to `planner.GeneratePlan`/`GenerateCompactPlan`. Rule 8 therefore speaks of `origin/<default>` generically, and the gate recognises only `main`/`master` as bare default-branch names.

---

## Data Flow

### Typical Execution Flow

```
1. User: "openexec run --task 'Add auth middleware'"
   │
   ▼
2. OpenExec CLI parses intent
   │
   ▼
3. Quality Gates run (go vet, go test -short)
   │   ├─ Pass → Continue
   │   └─ Fail → Block (if GateModeBlock)
   │
   ▼
4. Context Assembly
   │   ├─ Load knowledge index
   │   ├─ Predict and preload files
   │   └─ Prune to relevant subset
   │
   ▼
5. Blueprint Execution
   │   ├─ Stage 1: gather_context
   │   │   └─ Spawn: claude --prompt "Analyze auth patterns..."
   │   │
   │   ├─ Stage 2: implement  
   │   │   └─ Spawn: claude --prompt "Implement middleware..."
   │   │
   │   ├─ Stage 3: lint
   │   │   └─ Run: go vet ./...
   │   │
   │   ├─ Stage 4: test
   │   │   └─ Run: go test ./...
   │   │
   │   └─ Stage 5: review
   │       └─ Spawn: codex --prompt "Review for bugs..."
   │
   ▼
6. Checkpoint created after each stage
   │
   ▼
7. Memory extraction (patterns, decisions)
   │
   ▼
8. Results returned to user
```

---

## Common Misconceptions

### ❌ "OpenExec implements LLM clients"

**✅ Reality:** OpenExec shells out to existing CLIs (claude, codex, gemini). It doesn't implement LLM APIs directly.

**Evidence:**
- `internal/loop/process.go:37` - `exec.CommandContext(ctx, name, args...)`
- `internal/runner/runner.go` - Maps models to CLI commands

---

### ❌ "pkg/agent/ contains LLM implementations"

**✅ Reality:** `pkg/agent/` contains abstraction interfaces and types. Actual execution is in `internal/loop/`.

**Evidence:**
- `pkg/agent/provider.go` - Interface definitions only
- `internal/loop/process.go` - Actual process spawning

---

### ❌ "OpenExec replaces Claude Code/Codex"

**✅ Reality:** OpenExec **enhances** Claude Code/Codex with orchestration, safety, and reliability features.

**Analogy:**
- Claude Code = Engine
- OpenExec = Car (engine + chassis + safety systems + navigation)

---

### ❌ "OpenExec relies on the AI CLI's tool support"

**✅ Reality:** OpenExec provides its own tools via MCP (Model Context Protocol) server!

**How it works:**
1. OpenExec starts an MCP server (`internal/mcp/server.go`)
2. MCP server exposes 20+ tools (read_file, write_file, git_apply_patch, run_shell_command, etc.)
3. AI CLI connects to MCP server via stdio or HTTP
4. AI CLI requests tool calls through MCP
5. OpenExec executes tools and returns results

**This means:** Any AI client that speaks MCP can use OpenExec's tools, regardless of whether the AI has native tool support!

**Tools provided by OpenExec:**
- `read_file` - Read file contents
- `write_file` - Write file contents  
- `git_apply_patch` - Apply git patches
- `run_shell_command` - Execute shell commands
- `git_status` - Check git status
- `git_diff` - Get git diffs
- `git_log` - View git history
- `glob` - File globbing
- `grep` - Text search
- `list_directory` - Directory listing
- And more...

---

## Integration Points

### Adding a New AI CLI

To add support for a new AI CLI (e.g., `mistral`):

1. **Update Model Resolution** (`internal/runner/runner.go`):
```go
func isMistralModel(model string) bool {
    return strings.Contains(model, "mistral")
}

// In Resolve():
if isMistralModel(model) {
    return "mistral", defaultMistralArgs(), nil
}
```

2. **Add CLI Detection** (`internal/runner/runner_test.go`):
```go
func TestResolve_MistralModels(t *testing.T) {
    if _, err := exec.LookPath("mistral"); err != nil {
        t.Skip("mistral CLI not in PATH")
    }
    // Test resolution
}
```

3. **Document** (`docs/MODELS.md`):
Add installation and usage instructions.

---

### BitNet Routing (Opt-in)

**Status: Opt-in** — Enabled via `bitnet_routing` in config.json.

OpenExec includes an optional **BitNet Router** that uses a local 1-bit LLM for intent-based tool selection.

**Key behaviors:**
- The model **auto-downloads on first use** to `~/.openexec/models/`.
- Any GGUF model can be used, but the routing prompt is tuned for the default model.
- **Falls back to deterministic routing** if the model is unavailable or fails to load.

When enabled, BitNet can classify user intent and select tools locally, reducing round-trips to the cloud LLM. When disabled (the default), deterministic routing handles all classification.

---

## Performance Characteristics

| Subsystem | Overhead | Bottleneck |
|-----------|----------|------------|
| Model Routing | <1ms | N/A |
| Process Spawning | ~100-500ms | CLI startup time |
| Quality Gates | 5-30s | Lint/test execution |
| Context Pruning | ~50ms | SQLite queries |
| Checkpointing | ~10-100ms | File hashing |
| Predictive Loading | ~100ms | File I/O |

**Note:** Actual LLM inference time ( Claude/Codex/Gemini) dominates execution time.

---

## Security Model

### Local-First Design

- All orchestration happens locally
- No cloud service dependencies (except LLM APIs)
- User controls all data

### CLI Isolation

- Each AI CLI runs in separate subprocess
- Sandboxed by OS process boundaries
- Environment variables controlled by OpenExec
- Working directory restricted

---

## Future Directions

### Potential Enhancements

1. **Local Model Support**
   - Ollama integration
   - LM Studio support
   - Fully offline operation

2. **Advanced Routing**
   - Cost-based model selection
   - Capability-based routing
   - A/B testing between models

---

## References

- [README.md](../README.md) - Project overview
- [CONTRIBUTING.md](../CONTRIBUTING.md) - Contribution guidelines
- `internal/runner/runner.go` - Model resolution
- `internal/loop/process.go` - Process spawning
- `pkg/agent/provider.go` - Provider abstractions

---

**Document Maintainer:** OpenExec Core Team  
**Questions?** Open an issue or discussion on GitHub.
