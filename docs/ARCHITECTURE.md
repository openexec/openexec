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
| `internal/planner/lint.go` | `LintVerificationScript`, `LintPlanVerification` — deterministic false-green detector (never fails the CLI import; `ReviewPlan` refuses on it); `StaleBaseRefIssue` — shell-aware bare `main`/`master` revision detector for one script |
| `internal/planner/stale_base.go` | `PlanStaleBaseRefIssues` (per-plan issue map keyed by owner), `StaleBaseRefOwners` (sorted owners), `PlanStaleBaseRefError` (the same map as one refusal error) — the stale-base rule shared by every import call site |
| `internal/planner/lint_test.go` | `TestLintVerificationScript` (false-green vs sound samples), `TestStaleBaseRefIssue` (stale vs sound scripts, each for `main` and `master`), `TestShellCommands` (quotes, comments, separators, substitutions), `TestRevisionEndpoints` (`..`/`...` split, `^`/`~N`/`@{…}`/`:path` stripping), `TestLintPlanVerification` (keyed by story/task id) |
| `internal/planner/stale_base_test.go`, `review_test.go` | `TestPlanStaleBaseRefIssues` (story and task owners); `TestReviewPlanRefusesStaleBaseRefDespiteApproval`; `TestReviewPlanUsesExistingDiscipline` (false-green lint forces `Approved=false`) |
| `internal/planner/planner_test.go` | `TestStoryPrompt_RequiresOriginDefaultRef` (rule 8 as rendered by `GeneratePlan`) and `TestCompactPromptRequiresRemoteBaseRef` (compact rule 4 as rendered by `GenerateCompactPlan`) pin the `origin/<default>` clause |
| `pkg/manager/planner_stale_base_test.go` | `TestManagerPlan_RejectsStaleBaseRef` — `Manager.Plan` import routes at story and task level |
| `internal/cli/stale_base_binary_e2e_test.go` | `//go:build e2e`: runs the built `bin/openexec story import --dry-run` in a temp `.openexec` project (goal-less object and legacy array refused naming `story US-001 task T-US-001-001` and `origin/main`; the `origin/main` variant previews the import). Run `go build -o bin/openexec ./cmd/openexec`, then `go test -tags e2e ./internal/cli/ -run TestStoryImportBinaryStaleBaseGate` |
| `pkg/runtime/runtime.go` | Public wrappers `LintPlanVerification`, `RemapPlanIDs` for embedders |
| `pkg/manager/planner.go` | `Manager.Plan`: intent validation → `GeneratePlan`/`GenerateCompactPlan` → optional `ReviewPlan` → plan artifact → `importBoundPlan` (`preparePlanIDs` → `RemapPlanIDs`); a non-empty `RequestID` diverts to `replayReviewedPlan` |
| `pkg/manager/planner_replay.go` | `replayReviewedPlan`: durable reviewed planning (generate/compact → review → refine loop → `rel.ImportReviewedPlan`) with retained receipts in `run_steps` |
| `internal/cli/release.go` | `storyImportCmd` (`openexec story import [file]`) — hosts the PLANNING GATE |
| `internal/cli/release_test.go` | `TestReleaseCmd` drives `release create/show/finish`, `story create/list`, `task create/approve`, `goal verify`; plus `statusIcon`, config loading and `getReleaseManager`; `TestImport_PlanningGate_RejectsStaleBaseRef` drives `story import --dry-run` through the stale-base check for each file shape (with goals, goal-less object, legacy bare array): story-level reject, task-level reject naming the task id, `origin/main` pass |

**Rule rendering.** The prompts are static Go raw strings filled with `fmt.Sprintf`: `GeneratePlan` renders `StoryGenerationPrompt` with two `%s` slots (optional PRD block, then the intent text); `GenerateCompactPlan` renders `CompactStoryGenerationPrompt` with one `%s` (intent). No project configuration reaches the prompt — rules are identical for every repository.

**Rule 8 (VERIFIABILITY)** in `StoryGenerationPrompt` requires every story to carry an executable `verification_script` that checks its linked goal and exits non-zero on failure, and forbids false-green shapes: `|| <fallback>` after a test/assertion, `A && B || C`, `2>/dev/null` on the checked command, `grep -q` piped into another command, and non-specific assertions. Its last bullet is the **`origin/<default>` clause**: a script that compares against the default branch MUST use the remote ref `origin/<default>` (e.g. `git diff --name-only origin/main...HEAD`), NEVER a bare local branch name such as `main` — task worktrees are synced to `origin/<default>` and do not advance the local branch, so a stale local `main` fails correct work or passes wrong work. **Compact prompt clause:** `CompactStoryGenerationPrompt` rule 4 (the story and every task MUST have a concrete `verification_script` that fails when the change is broken) ends with the same sentence verbatim, so compact planning receives the instruction too (pinned as rendered by `TestCompactPromptRequiresRemoteBaseRef`, next to `TestStoryPrompt_RequiresOriginDefaultRef` for rule 8). The prompts are instructions a model may ignore; the deterministic stale-base check (`PlanStaleBaseRefIssues`, below) is what refuses a plan, and it runs at the call sites listed under **Stale-base call sites** regardless of which prompt produced the plan.

**`LintVerificationScript(script string) []string`** matches four regexes (`falseGreenPatterns`): a test/grep command followed by `||`; `2>/dev/null`; `grep -q… |`; and `&& … ||`. Empty scripts return nil; an empty result means "no known anti-pattern", not "sound". **`LintPlanVerification(plan) map[string][]string`** runs it over every story and task script and returns issues keyed by story/task id. It never fails the CLI import, but it is not purely advisory: `Planner.ReviewPlan` (`internal/planner/review.go`) calls it after parsing the reviewer's verdict and forces `Approved=false` (appending `; verification lint refused: …` to the assessment) when it returns any issue, so on the reviewed `Manager.Plan` routes a false-green script blocks import (`TestReviewPlanUsesExistingDiscipline`). It is also exported through `pkg/runtime` for embedders. `LintPlanVerification` does **not** call `StaleBaseRefIssue`.

**PLANNING GATE (hard fail)** lives in `storyImportCmd.RunE`, after schema-version checks (accepted: `1.0`, `1.1`, `legacy` bare array; missing version only warns) and before the dry-run print or any DB write. Checks 1–2 (goal coverage) run only when the file has `goals`; for each goal they return an error when:
1. `PLANNING GATE FAILED: Primary goal <id> (<title>) has no supporting stories` — no story has that `goal_id`;
2. `PLANNING GATE FAILED: Primary goal <id> (<title>) has no stories with a verification_script` — none of those stories has a non-empty script.

3. **Stale-base check (hard fail).** Outside the goals branch, so it applies to every file shape (goal-less objects and legacy bare arrays included), the gate runs `planner.PlanStaleBaseRefIssues` (`StaleBaseRefIssue` per script) over every story's `verification_script` and every task-object's `verification_script` (task entries that are bare id strings are skipped). The first owner in sorted order returns `PLANNING GATE FAILED: story <story-id>: <issue>` or `PLANNING GATE FAILED: story <story-id> task <task-id>: <issue>`, where `<issue>` is ``verification script diffs against the bare local `main` ref; use `origin/main` — candidate worktrees are synced to origin/<default>, where the local `main` is stale or missing`` (with `master` substituted when that is the ref found).

**Shell-aware detector.** `StaleBaseRefIssue(script string) string` (`internal/planner/lint.go`) is a hard gate, so it looks only at executable git revision arguments rather than regex-matching the whole script. It tokenizes the script as shell — single/double quotes, backslash continuations, `#` comments, and `;`/`&&`/`||`/`|`/newline/subshell/command-substitution boundaries — and, per command, skips prefixes (`!`, `if`, `time`, `NAME=value` …) to find `git`, skips git global options with values (`-C`, `-c`, `--git-dir`, …), and continues only for `diff|log|show|merge-base|rev-list|rev-parse|cherry|range-diff`. Positionals before `--` are revisions; subcommand options with a separate value (`-n`, `--grep`, `--since`, …) are skipped. Each revision is split on `..`/`...`, and each endpoint loses a leading `^` and any `@{…}`, `^`/`~N` or `:path` suffix; an endpoint that is exactly `main` or `master` is refused. So `main...HEAD`, `'main' HEAD`, `"main..HEAD"`, `^main`, `main~1`, `main:README.md`, `master@{u}...HEAD`, and a bare ref mixed with a remote one (`git diff origin/main...HEAD && git diff main...HEAD`) are refused; `origin/main`, `upstream/main` and `feature-main` pass, as do main/master in comments, in data for other commands (`grep -Fq 'main...HEAD' docs/…`, `echo git diff main...HEAD`), as option values (`--grep main`), in non-revision git subcommands (`git checkout main`) and as pathspecs after `--` (`git diff --exit-code origin/main...HEAD -- main`). It returns `""` when the script is acceptable. Every example in this paragraph is a case in `TestStaleBaseRefIssue` (run for both `main` and `master`); tokenizing is pinned by `TestShellCommands` and endpoint stripping by `TestRevisionEndpoints`. Apart from this check the CLI gate never inspects script content — it does not call the false-green linter. `openexec plan` (`internal/cli/plan.go`) prints its own `PLANNING GATE FAILED:` banner, but that one reports intent-validation issues from `Manager.Plan`, which does not run the goal-coverage gate; its stale-base check fails as an import error (routes 2–6 below).

**Stale-base call sites.** `PlanStaleBaseRefIssues(plan)` (`internal/planner/stale_base.go`) applies `StaleBaseRefIssue` to every story and task `verification_script` of a `ProjectPlan` and returns a map from owner (`story US-001`, `story US-001 task T-US-001-002`) to diagnostic; it never reads goals, so goal-less and legacy plans are checked like full ones. `PlanStaleBaseRefError` wraps the same map as one `stale base ref refused: <owner>: <issue>; …` error in `StaleBaseRefOwners` (sorted) order. It is called at exactly these sites:
- **CLI import** — `storyImportCmd.RunE` (`internal/cli/release.go`) calls `PlanStaleBaseRefIssues(generatedStoriesPlan(stories))` outside the goal-coverage branch and reports the first owner as `PLANNING GATE FAILED` (routes 1a, 1b).
- **`ReviewPlan`** (`internal/planner/review.go`) calls `PlanStaleBaseRefError` after parsing the reviewer's verdict and forces `Approved=false`, appending the error to the assessment so refinement receives the owner and `origin/` fix (routes 3–4).
- **`importBoundPlan`** (`pkg/manager/planner.go`) calls `PlanStaleBaseRefError` first, before `preparePlanIDs` and any `rel.Create*` write (routes 2–3).
- **`replayReviewedPlan`** (`pkg/manager/planner_replay.go`) calls it at two steps: the **refinement step**, on the `RefinePlan` result when `AutoImport` is set, before `preparePlanIDs`/`ValidatePlanIdentities` (route 5); and the **retained/import step**, on the approved `result.Plan` immediately before `rel.ImportReviewedPlan`, which covers fresh, compact and retained-receipt replays alike and refuses without rewriting the receipt or plan artifact (routes 4, 6).

**Plan import routes.** Every code path that persists planner-generated goal/story/task rows, and which of the call sites above guards it. `TestManagerPlan_RejectsStaleBaseRef` (`pkg/manager/planner_stale_base_test.go`) covers routes 2–6 at story and task level, each for full and compact generation: `{full,compact}/native` (route 2), `/reviewed-direct` (route 3: `Review` + `AutoImport`, no `RequestID`), `/reviewed` (route 4, via `replayRequest()`), `/refined` (route 5) and `/retained` (route 6). `TestImport_PlanningGate_RejectsStaleBaseRef` and the e2e binary test cover 1a/1b.

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

Consequence: on every route above (1a–6) a plan whose story- or task-level script uses a bare `main...HEAD` is refused before any row is written, and the diagnostic names the owner (`story US-1` or `story US-1 task T-2`) and the `origin/main` fix. Routes 2–6: every `TestManagerPlan_RejectsStaleBaseRef` subtest (asserts the owner and `origin/` in the error, and zero imported tasks after reopening the DB). Routes 1a/1b: `TestImport_PlanningGate_RejectsStaleBaseRef` and `TestStoryImportBinaryStaleBaseGate`.

**Default branch threading.** None: no project setting names the default branch to the planner. The only related setting is `base_branch` (`project.ProjectConfig.BaseBranch`, `release.Config.BaseBranch`, default `"main"`), consumed by the release manager and `safe_commit`; it is never passed to `planner.GeneratePlan`/`GenerateCompactPlan`. Rule 8 therefore speaks of `origin/<default>` generically, and the gate recognises only `main`/`master` as bare default-branch names.

### Review 474755f1 finding map

Review `474755f1d4af4c2b4e1faa5240c74345` (PR #63) worked by code inspection only: the reviewer ran no commands. Its evidence line ranges (`lint.go:26-60`, `release.go:918-959`, `planner_replay.go:240-275`, `prompt.go:162-168`, `ARCHITECTURE.md:350`) come from the tree before US-012/US-013. Commits `e63086c9` (tokenized detector), `62a728d5` (CLI gate outside the goals branch), `c09f3b5f`/`1ac5b539` (`ReviewPlan` and every `Manager.Plan` import path) and `a029f1d4` (compact rule 4) changed those regions afterwards. The table below maps every listed case to the code and test at HEAD; every test named here passed in the US-018 validation run below.

**HIGH — native planning and replay bypass the gate**

| Case | HEAD code | Covered by |
|------|-----------|------------|
| Compact native planning gets no remote-ref instruction | `CompactStoryGenerationPrompt` rule 4 carries the `origin/<default>` clause verbatim | `TestCompactPromptRequiresRemoteBaseRef` (rendered by `GenerateCompactPlan`) |
| Compact native planning gets no deterministic rejection | `importBoundPlan` → `PlanStaleBaseRefError` before `preparePlanIDs` | `TestManagerPlan_RejectsStaleBaseRef/compact/native/{story,task}` |
| Full native planning imports a response that ignores the instruction | same `importBoundPlan` check | `…/full/native/{story,task}` |
| Reviewer-approved plan with `main...HEAD` through `replayReviewedPlan` (Agent Console's `Review`+`AutoImport`+`RequestID` route), full and compact | `ReviewPlan` forces `Approved=false`; `PlanStaleBaseRefError` runs again before `rel.ImportReviewedPlan` | `…/{full,compact}/reviewed/{story,task}`, `TestReviewPlanRefusesStaleBaseRefDespiteApproval` |
| Reviewed `Manager.Plan` without a `RequestID` (route 3), full and compact | `ReviewPlan` refusal; `importBoundPlan` re-checks | `…/{full,compact}/reviewed-direct/{story,task}` |
| Refined plan | `PlanStaleBaseRefError(refined)` before `preparePlanIDs`/`ValidatePlanIdentities` and before re-review | `…/{full,compact}/refined/{story,task}` (asserts no re-review) |
| Retained-plan replay, refused without rewriting accepted artifacts | `PlanStaleBaseRefError(result.Plan)` before `ImportReviewedPlan`; the receipt is not rewritten | `…/{full,compact}/retained/{story,task}` (compares receipt metadata and plan artifact bytes before and after) |
| Manual import of a goal-less object or legacy bare array, story and task level | `storyImportCmd.RunE` runs `PlanStaleBaseRefIssues` outside `if len(sf.Goals) > 0` | `TestImport_PlanningGate_RejectsStaleBaseRef` (`goal-less object:`/`legacy array:` subtests), `TestStoryImportBinaryStaleBaseGate` |
| Rule 8 in full generation | `StoryGenerationPrompt` rule 8 | `TestStoryPrompt_RequiresOriginDefaultRef` |
| Owner and `origin/` fix in the diagnostic; nothing imported after reopening the DB; `origin/main` persists | — | `requireStaleRefusal` + `importedTaskCount` (reopens `release.Manager` on the same DB) in every `TestManagerPlan_RejectsStaleBaseRef` subtest; each subtest also runs `origin/main` and requires persisted tasks |
| Docs claim "every imported plan" | Coverage is now stated per route in **Plan import routes** | — |

**MEDIUM — the gate confuses script text and pathspecs with Git refs**

| Reviewer script (each for `main` and `master`) | Expected | `TestStaleBaseRefIssue` template | CLI import (`TestImport_PlanningGate_RejectsStaleBaseRef`) |
|------|----------|----------------------------------|------------|
| `git diff --exit-code origin/main...HEAD -- main` | accept | sound `git diff --exit-code origin/REF...HEAD -- REF` | `reviewer_scripts/{main,master}/{story,task}/sound/0` |
| `grep -Fq 'main...HEAD' docs/ARCHITECTURE.md` | accept | sound `grep -Fq 'REF...HEAD' docs/ARCHITECTURE.md` | `reviewer_scripts/…/sound/1` |
| Comment-only mention followed by a valid assertion | accept | sound `"# never use REF...HEAD\ngit diff --exit-code origin/REF...HEAD -- internal/"` | `reviewer_scripts/…/sound/2` (`"# never REF...HEAD\ngit diff --exit-code origin/REF...HEAD"`) |
| `git diff 'main' HEAD` | reject | stale `git diff 'REF' HEAD` | `reviewer_scripts/…/stale/0` |
| Quoted two-dot/three-dot ranges | reject | stale `git diff "REF...HEAD"`, `git diff 'REF..HEAD'` | `reviewer_scripts/…/stale/1` (`"REF..HEAD"`), `…/stale/2` (`'REF...HEAD'`) |
| Mixed remote/bare comparison | reject | stale `git diff origin/REF...HEAD && git diff REF...HEAD` | `reviewer_scripts/…/stale/3` |
| Unquoted bare range, story and task owner | reject | stale `git diff --name-only REF...HEAD …`, `git log REF..HEAD` | with goals: story `main...HEAD`, task `main..HEAD`; goal-less object and legacy array: story `main...HEAD`, task `master..HEAD` |

The `reviewer_scripts` group runs each template for `main` and `master`, owned by the story or by task `T-US-001-002`, through `storyImportCmd.RunE` (28 leaves). Stale leaves require the owner (`story US-001` or `T-US-001-002`) and `origin/<ref>` in the error; sound leaves require `✓ Planning Gate passed.`.

**Remaining gaps at HEAD**

None. Both findings' cases are exercised at their gate, and the HIGH and MEDIUM falsify controls are recorded below.

**Validation run (US-018, 2026-09-29, candidate worktree at `8ed32d3b`)**

| Command | Result |
|---------|--------|
| `go vet ./...` | clean, no output |
| `go test ./... -count=1` | `ok` for every package with tests (`internal/planner`, `internal/cli`, `pkg/manager` included); no `FAIL` |
| `make compat-test` | `TestCompatibility_ExistingProjects_StatusCLI` (current `.openexec` and legacy `.uaos`), `TestCompatibility_LegacyProjectConfigFallback`, `TestCompatibility_LegacyTasksJSONFallback` — PASS |
| `go build -o bin/openexec ./cmd/openexec`, then `go test -tags e2e -count=1 ./internal/cli/ -run TestStoryImportBinaryStaleBaseGate -v` | PASS: goal-less object and legacy array refused with `PLANNING GATE FAILED: story US-001 task T-US-001-001`, naming the `origin/main` fix; the `origin/main` variant previews 1 story |
| `go test -count=1 ./internal/planner/ ./internal/cli/ ./pkg/manager/ -run 'StaleBase\|OriginDefaultRef\|RemoteBaseRef\|PlanningGate' -v` | PASS: `TestStaleBaseRefIssue`, `TestStoryPrompt_RequiresOriginDefaultRef`, `TestCompactPromptRequiresRemoteBaseRef`, `TestReviewPlanRefusesStaleBaseRefDespiteApproval`, `TestPlanStaleBaseRefIssues` (2 subtests), `TestImport_PlanningGate_RejectsStaleBaseRef` (9 subtests + 28 `reviewer_scripts` leaves), `TestManagerPlan_RejectsStaleBaseRef` (20 subtests) |

**Dispositions for `resolve_feature_review`** (review `474755f1d4af4c2b4e1faa5240c74345`)

- **HIGH — native planning and replay bypass the gate: accepted, repaired.** The shared validator `PlanStaleBaseRefIssues`/`PlanStaleBaseRefError` (`internal/planner/stale_base.go`) now runs in `ReviewPlan`, `importBoundPlan`, and both `replayReviewedPlan` steps (refined plan; approved/retained plan before `ImportReviewedPlan`, receipt not rewritten), and in the CLI import outside the goals branch. Compact rule 4 carries the `origin/<default>` clause. Evidence: `TestManagerPlan_RejectsStaleBaseRef` (full/compact × native/reviewed/reviewed-direct/refined/retained × story/task, DB reopened, `origin/main` persists), `TestImport_PlanningGate_RejectsStaleBaseRef` goal-less/legacy subtests, `TestStoryImportBinaryStaleBaseGate`, `TestCompactPromptRequiresRemoteBaseRef`; negative controls (a)–(c) above fail as required. Docs: the "every imported plan" claim is replaced by the per-route table. Commits `c09f3b5f`, `1ac5b539`, `62a728d5`, `a029f1d4`, `ae986da4`.
- **MEDIUM — gate confuses script text and pathspecs with Git refs: accepted, repaired.** `StaleBaseRefIssue` now tokenizes shell (quotes, comments, separators, substitutions) and inspects only revision arguments of revision-taking git subcommands before `--`. Evidence: `TestStaleBaseRefIssue`, `TestShellCommands`, `TestRevisionEndpoints`, and the 28 `reviewer_scripts` leaves of `TestImport_PlanningGate_RejectsStaleBaseRef` (reviewer's exact scripts, `main`/`master`, story/task); negative controls (a) regexes restored and (b) `return ""` fail 16/28 leaves each. Commits `e63086c9`, `8ed32d3b`.

### Review 474755f1 — HIGH negative controls

Run 2026-09-29 in the candidate worktree (US-016). Each mutation was applied on its own, the matching tests were run with `-v`, and the production file was restored with `git checkout -- <file>` before the next one. None of these mutations is committed. Baseline and final rerun: `go test ./internal/planner/ ./internal/cli/ ./pkg/manager/` → `ok` for all three packages, and every stale-base test passes (including the 20 `TestManagerPlan_RejectsStaleBaseRef` subtests).

**(a) Remove the shared validation** — delete the `PlanStaleBaseRefError` calls in `importBoundPlan` (`pkg/manager/planner.go`) and at both steps of `replayReviewedPlan` (`pkg/manager/planner_replay.go`). `go test ./pkg/manager/ -run TestManagerPlan_RejectsStaleBaseRef -v`:
```
--- FAIL: TestManagerPlan_RejectsStaleBaseRef
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/full/native/story
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/full/refined/story
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/full/retained/story
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/compact/native/story
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/compact/refined/story
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/compact/retained/story
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/full/native/task
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/full/refined/task
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/full/retained/task
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/compact/native/task
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/compact/refined/task
    --- FAIL: TestManagerPlan_RejectsStaleBaseRef/compact/retained/task
planner_stale_base_test.go:98: stale base ref plan was accepted: &{... Valid:true Issues:[] ...}
planner_stale_base_test.go:181: stale refined plan was not refused: &{... Valid:false Issues:[Required verification represented; stale base ref refused: story US-1: ...] ...}
planner_stale_base_test.go:229: retained stale plan was not refused: &{... Valid:true Issues:[] ...}
```
The `{full,compact}/reviewed/*` and `/reviewed-direct/*` subtests still pass under (a) because `ReviewPlan` refuses the approved stale plan on its own (`TestReviewPlanRefusesStaleBaseRefDespiteApproval`); the import-side check is the second layer there. On the refined route the stale plan now reaches re-review (refused there, but with no error before re-review, which the subtest forbids).

**(b) Restore the goals-only CLI condition** — move the `PlanStaleBaseRefIssues` block in `storyImportCmd.RunE` (`internal/cli/release.go`) inside `if len(sf.Goals) > 0`. `go test ./internal/cli/ -run TestImport_PlanningGate -v`:
```
--- FAIL: TestImport_PlanningGate_RejectsStaleBaseRef
    --- FAIL: TestImport_PlanningGate_RejectsStaleBaseRef/goal-less_object:_story-level_bare_main_is_rejected
    --- FAIL: TestImport_PlanningGate_RejectsStaleBaseRef/goal-less_object:_task-level_bare_master_is_rejected
    --- FAIL: TestImport_PlanningGate_RejectsStaleBaseRef/legacy_array:_story-level_bare_main_is_rejected
    --- FAIL: TestImport_PlanningGate_RejectsStaleBaseRef/legacy_array:_task-level_bare_master_is_rejected
release_test.go:266: expected the planning gate to reject a bare main ref; output: Note: This performs a one-time import. ...
release_test.go:278: expected the planning gate to reject a task-level bare master ref; output: Note: This performs a one-time import. ...
```
The with-goals subtests (`bare_main_is_rejected`, `task-level_bare_main_is_rejected_with_the_task_ID`) still pass, as they should: the check still runs when goals are present.

**(c) Remove the compact clause** — delete the `origin/<default>` sentence from `CompactStoryGenerationPrompt` rule 4 (`internal/planner/prompt.go`). `go test ./internal/planner/ -run 'OriginDefaultRef|RemoteBaseRef' -v`:
```
--- FAIL: TestCompactPromptRequiresRemoteBaseRef
planner_test.go:174: rule 4 of the rendered compact prompt is missing "origin/<default>"
planner_test.go:174: rule 4 of the rendered compact prompt is missing "git diff --name-only origin/main...HEAD"
planner_test.go:174: rule 4 of the rendered compact prompt is missing "NEVER a bare local branch name such as 'main'"
```
`TestStoryPrompt_RequiresOriginDefaultRef` (full-prompt rule 8) still passes.

No CLI route gap was open for the HIGH finding (goal-less and legacy shapes were already covered), so `TestImport_PlanningGate_StaleBaseRoutes` was not added.

### Review 474755f1 — MEDIUM negative controls

Run 2026-09-29 in the candidate worktree (US-017). Each mutation of `StaleBaseRefIssue` (`internal/planner/lint.go`) was applied on its own and `go test ./internal/cli/ -count=1 -run 'TestImport_PlanningGate_RejectsStaleBaseRef/reviewer_scripts' -v` was run. `git checkout -- internal/planner/lint.go` then restored the file, and `git diff --exit-code HEAD -- internal/planner/lint.go` was clean before the next step. Neither mutation is committed. Baseline and final rerun: all 28 `reviewer_scripts` leaves pass, and `go test ./internal/planner/ ./internal/cli/` → `ok`.

**(a) Restore the whole-script regexes** (the pre-`e63086c9` `staleBaseRefPatterns` loop). 16 of 28 leaves fail: every sound leaf, plus `stale/0`. The quoted ranges and the mixed comparison still match the range regex, so they stay rejected:
```
--- FAIL: TestImport_PlanningGate_RejectsStaleBaseRef/reviewer_scripts/{main,master}/{story,task}/sound/{0,1,2}
--- FAIL: TestImport_PlanningGate_RejectsStaleBaseRef/reviewer_scripts/{main,master}/{story,task}/stale/0
release_test.go:369: sound story script "git diff --exit-code origin/main...HEAD -- main" was refused: PLANNING GATE FAILED: story US-001: verification script diffs against the bare local `main` ref; ...
release_test.go:369: sound task script "grep -Fq 'master...HEAD' docs/ARCHITECTURE.md" was refused: PLANNING GATE FAILED: story US-001 task T-US-001-002: ...
release_test.go:369: sound story script "# never main...HEAD\ngit diff --exit-code origin/main...HEAD" was refused: ...
release_test.go:381: stale task script "git diff 'master' HEAD" was accepted; output: Note: This performs a one-time import. ...
```

**(b) `return ""`** (the detector always accepts). 16 of 28 leaves fail: every stale leaf. Every sound leaf still passes:
```
--- FAIL: TestImport_PlanningGate_RejectsStaleBaseRef/reviewer_scripts/{main,master}/{story,task}/stale/{0,1,2,3}
release_test.go:381: stale story script "git diff \"main..HEAD\"" was accepted; ...
release_test.go:381: stale task script "git diff 'master...HEAD'" was accepted; ...
release_test.go:381: stale story script "git diff origin/main...HEAD && git diff main...HEAD" was accepted; ...
```

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
