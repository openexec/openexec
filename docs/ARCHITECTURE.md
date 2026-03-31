# OpenExec Architecture

**Version:** 1.0  
**Last Updated:** 2026-03-31

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
│  ├─ Input Processing (PII shielding, intent parsing)            │
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

### 9. Multi-Agent Coordination (`internal/agent/`, `internal/parallel/`)

**Purpose:** Run multiple agents in parallel.

**Key Files:**
- `internal/agent/registry.go` - Agent management
- `internal/parallel/engine.go` - Parallel execution

**Features:**
- Parallel stage execution
- Dependency-aware scheduling
- Agent pooling
- Result aggregation

---

### 10. Caching (`internal/cache/`)

**Purpose:** Avoid redundant computation.

**Key Files:**
- `internal/cache/knowledge.go` - Knowledge cache
- `internal/cache/tools.go` - Tool result cache

**Cache Levels:**
- Knowledge cache (symbol lookups)
- Tool result cache (idempotent tools)
- SQLite-backed with TTL

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
- PII shielding before any data leaves machine
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

1. **Direct API Mode** (optional)
   - For users who prefer direct API calls
   - Bypass CLI overhead
   - Would require implementing `pkg/agent/` interfaces

2. **Local Model Support**
   - Ollama integration
   - LM Studio support
   - Fully offline operation

3. **Advanced Routing**
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
