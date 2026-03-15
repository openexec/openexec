# OpenExec Configuration Guide

This guide covers all configuration options for OpenExec, including agent selection, execution settings, quality gates, toolsets, blueprints, and more.

## Configuration File

Configuration is stored in `openexec.yaml` in your project root. Environment variables can override any setting using the `OPENEXEC_*` prefix.

```yaml
# openexec.yaml - Full example with all sections
project:
  name: my-project
  description: My awesome project

# ============================================================================
# AGENT CONFIGURATION
# ============================================================================
agents:
  # Which agent handles task execution (claude, codex, gemini, opencode)
  default: claude

  # Claude Code CLI configuration
  claude:
    model: sonnet                       # Model to use: sonnet, opus (optional)
    timeout: 600                        # Seconds before timeout (1-3600)
    skip_permissions: false             # Run without approval prompts

  # OpenAI Codex CLI configuration
  codex:
    model: gpt-5                        # Model to use (optional)
    timeout: 600                        # Seconds before timeout
    skip_permissions: true              # full-auto mode (no confirmations)

  # Google Gemini CLI configuration
  gemini:
    model: 3.1-pro-preview              # Model to use: 3.1-pro-preview, 3.1-flash-preview
    timeout: 600                        # Seconds before timeout

  # OpenCode (Ollama) configuration
  opencode:
    model: qwen2.5-coder:32b            # Local Ollama model
    timeout: 600                        # Seconds before timeout
    skip_permissions: true              # auto-confirm mode

# ============================================================================
# REVIEW CONFIGURATION
# ============================================================================
review:
  # Agent for code reviews (can be different from execution agent)
  review_agent: codex                   # claude, codex, gemini, opencode

  # Require review before marking tasks complete
  require_review: true

  # Review strictness: lenient, normal, strict
  strictness: normal

  # Focus areas for review (comma-separated)
  focus_areas:
    - security
    - performance
    - style
    - correctness

# Agent for planning operations (PRD parsing, story generation)
planning:
  planning_agent: claude                # Usually want a strong model for planning

# ============================================================================
# EXECUTION CONFIGURATION
# ============================================================================
execution:
  timeout: 600                          # Task timeout in seconds (1-86400)
  grace_period: 10                      # Seconds between SIGTERM and SIGKILL

  # Auto-fix: Generate fix tasks when quality gates fail
  auto_fix: true
  fix_strategy: by_file                 # by_file or by_type
  max_fix_iterations: 2                 # Max fix attempts (1-5)
  fix_timeout: 300                      # Timeout for fix tasks

  # Scoped commits: Require agents to use 'openexec commit'
  scoped_commits: false

# ============================================================================
# RETRY CONFIGURATION
# ============================================================================
retry:
  max_attempts: 3                       # Max retry attempts (1-10)
  base_delay: 1.0                       # Base delay in seconds
  multiplier: 2.0                       # Exponential backoff multiplier
  max_delay: 60.0                       # Maximum delay between retries

# ============================================================================
# QUALITY GATES
# ============================================================================
quality:
  # Built-in gates: lint, typecheck, test, security, format
  gates:
    - lint
    - typecheck
    - test

  # Custom gate definitions
  custom:
    - name: go_fmt
      command: "go fmt ./... && git diff --exit-code -- '*.go'"
      mode: blocking                    # blocking or warning

    - name: go_lint
      command: "golangci-lint run ./..."
      mode: blocking

    - name: go_test
      command: "go test -v -race ./..."
      mode: blocking

    - name: go_sec
      command: "gosec ./..."
      mode: blocking

# ============================================================================
# DAEMON CONFIGURATION (for background/multi-project mode)
# ============================================================================
daemon:
  poll_interval: 30                     # Seconds between task checks (1-86400)
  max_parallel: 1                       # Concurrent tasks (1-32)
  config_reload_interval: 60            # Seconds between config checks
  max_memory_mb: 2048                   # Memory limit per task (optional)

  # Multi-project orchestration
  multi_project: true
  projects_path: "/path/to/projects"    # Directory containing projects
  project_filter:                       # Which projects to include
    - project-a
    - project-b

  # Project prioritization: alphabetical, last_modified, priority_file, pending_tasks
  project_prioritization: alphabetical

  idle_detection_attempts: 3            # Polls before considering project idle
  blocked_retry_delay: 300              # Seconds before retrying blocked project
  project_switch_delay: 5               # Seconds between project switches

  # Health endpoint
  health_port: 8080                     # Port for health checks (optional)
  health_host: "0.0.0.0"

  resume_on_restart: true               # Resume from last project on restart

# ============================================================================
# TOOLSET CONFIGURATION
# ============================================================================
toolsets:
  # Which toolsets are enabled for this project
  enabled:
    - repo_readonly
    - coding_backend
    - debug_ci

  # Custom toolset definitions
  custom:
    my_toolset:
      tools:
        - read_file
        - write_file
        - custom_tool
      risk_level: medium                # low, medium, high
      phases:
        - implement
        - fix_lint

# ============================================================================
# BLUEPRINT CONFIGURATION
# ============================================================================
blueprints:
  # Enable blueprint-driven execution (default: true)
  enabled: true

  # Default blueprint to use
  default: standard_task                # standard_task, quick_fix, or custom

  # Custom blueprint definitions
  custom:
    fast_fix:
      stages:
        - implement
        - lint
        - test
      max_total_retries: 5

# ============================================================================
# MODE CONFIGURATION
# ============================================================================
mode:
  # Default mode for new sessions
  default: chat                         # chat, task, run

  # Auto-escalation settings
  auto_escalate:
    enabled: true
    chat_to_task_confidence: 0.8        # Min confidence to suggest escalation
    task_to_run_confidence: 0.9         # Min confidence to suggest escalation

# ============================================================================
# CONTEXT CONFIGURATION (code indexing for prompts)
# ============================================================================
context:
  enabled: true                         # Include code context in prompts
  max_tokens: 4000                      # Max tokens for context (100-50000)
  min_relevance_score: 0.1              # Minimum relevance (0.0-1.0)

# ============================================================================
# INDEX CONFIGURATION (code search)
# ============================================================================
index:
  auto_refresh: true                    # Refresh after task execution
  watch_debounce_ms: 500                # Debounce for file changes
  staleness_threshold: 3600             # Seconds before index is stale
  include_file_hashes: true             # Store hashes for change detection

# ============================================================================
# TRACE/AUDIT CONFIGURATION
# ============================================================================
trace:
  retention_days: 365                   # Days to keep traces (1-3650)
  keep_failed: false                    # Keep failed traces regardless of age
  prune_schedule: "0 3 * * *"           # Cron for auto-pruning
  include_prompts: false                # Store prompts (for debugging)
  include_responses: false              # Store responses (for replay)

# ============================================================================
# GITFLOW CONFIGURATION
# ============================================================================
gitflow:
  enabled: false                        # Enable GitFlow branch management
  branch_prefix_story: "story/"         # Prefix for feature branches
  branch_prefix_fix: "fix/"             # Prefix for fix branches
  branch_prefix_hotfix: "hotfix/"       # Prefix for hotfix branches
  protected_branches:                   # Branches that require review
    - main
    - release/*
  auto_merge: false                     # Auto-merge after review
  require_review: true                  # Require review for protected branches
  delete_branch_after_merge: true       # Clean up after merge
  push_after_merge: true                # Push to remote after merge

# ============================================================================
# SAFETY CONFIGURATION (multi-agent protection)
# ============================================================================
safety:
  enabled: true                         # Enable safety rules
  allow_parallel: false                 # Allow parallel task execution
  file_locking: true                    # Lock files during edits
  git_stash_allowed: false              # Allow git stash
  git_branch_switch_allowed: false      # Allow branch switching
  git_worktree_allowed: false           # Allow worktree modification
  git_add_all_allowed: false            # Allow 'git add .'
  preserve_unrecognized: true           # Keep external changes
  lock_timeout: 300                     # Stale lock timeout

# ============================================================================
# PARALLEL EXECUTION (multiple stories at once)
# ============================================================================
parallel:
  enabled: true                         # Allow --parallel flag
  max_concurrent: 4                     # Max concurrent stories
  lock_timeout: 600                     # Story lock timeout
  conflict_check: true                  # Check for merge conflicts
  branch_prefix: "story/"               # Branch prefix
  auto_merge: true                      # Auto-merge to release branch
  merge_strategy: squash                # squash, merge, rebase
  fail_fast: false                      # Stop all on failure
  block_dependents: true                # Block dependent stories on failure

# ============================================================================
# OBSERVABILITY (metrics, tracing, logging)
# ============================================================================
observability:
  level: standard                       # minimal, standard, full

metrics:
  enabled: false                        # Enable Prometheus metrics
  host: "0.0.0.0"
  port: 9090

tracing:
  enabled: false                        # Enable distributed tracing
  service_name: openexec
  exporter: console                     # console, otlp, jaeger
  sample_rate: 1.0                      # 0.0-1.0

# ============================================================================
# RESOURCE LIMITS
# ============================================================================
resource_limits:
  max_memory_mb: 4096                   # Memory limit per task
  cpu_affinity: [0, 1, 2, 3]            # CPU cores to use (optional)
```

## Common Configuration Scenarios

### Scenario 1: Claude for coding, Codex for review

```yaml
agents:
  default: claude
  claude:
    model: sonnet
    timeout: 600
  codex:
    model: gpt-5
    timeout: 600

review:
  review_agent: codex
  require_review: true
  strictness: normal
```

### Scenario 2: Fast iteration with Gemini

```yaml
agents:
  default: gemini
  gemini:
    model: 3.1-flash-preview
    timeout: 300

review:
  require_review: false           # Skip reviews for speed

execution:
  auto_fix: true
  max_fix_iterations: 3
```

### Scenario 3: Local-only with Ollama (OpenCode)

```yaml
agents:
  default: opencode
  opencode:
    model: qwen2.5-coder:32b      # Or: codellama, deepseek-coder
    timeout: 900                   # Local models may be slower
    skip_permissions: true

review:
  review_agent: opencode          # Same agent for review
  require_review: true
```

### Scenario 4: Multi-project orchestration

```yaml
project:
  name: orchestrator
  type: meta

daemon:
  multi_project: true
  projects_path: "/path/to/projects"
  project_filter:
    - frontend
    - backend
    - shared-lib
  max_parallel: 2
  poll_interval: 30

agents:
  default: claude
  claude:
    timeout: 600

review:
  review_agent: codex
  require_review: true
```

### Scenario 5: High-security with full audit

```yaml
agents:
  default: claude
  claude:
    skip_permissions: false       # Require approval prompts

execution:
  scoped_commits: true            # Track exactly what agent commits

trace:
  retention_days: 730             # Keep 2 years
  include_prompts: true           # Full audit trail
  include_responses: true

safety:
  enabled: true
  file_locking: true
  git_add_all_allowed: false      # No bulk adds
```

### Scenario 6: Blueprint-driven execution with custom stages

```yaml
blueprints:
  enabled: true
  default: standard_task

toolsets:
  enabled:
    - repo_readonly
    - coding_backend
    - debug_ci

agents:
  default: claude
  claude:
    model: sonnet
    timeout: 600
```

### Scenario 7: Quick fixes without full test cycle

```yaml
blueprints:
  enabled: true
  default: quick_fix
  custom:
    quick_fix:
      stages:
        - implement
        - verify                  # Just build, no full test suite

toolsets:
  enabled:
    - coding_backend

execution:
  auto_fix: true
  max_fix_iterations: 2
```

### Scenario 8: Read-only exploration mode

```yaml
mode:
  default: chat                   # Never auto-escalate

toolsets:
  enabled:
    - repo_readonly               # Only read operations

blueprints:
  enabled: false                  # No automated execution
```

## Environment Variables

Override any config with environment variables:

```bash
# Agent selection
export OPENEXEC_DEFAULT_AGENT=codex
export OPENEXEC_AGENT_CLAUDE_MODEL=opus
export OPENEXEC_AGENT_CLAUDE_TIMEOUT=900

# Review settings
export OPENEXEC_REVIEW_AGENT=claude
export OPENEXEC_REVIEW_STRICTNESS=strict

# Execution
export OPENEXEC_EXECUTION_AUTO_FIX=true
export OPENEXEC_EXECUTION_TIMEOUT=1800

# Daemon
export OPENEXEC_DAEMON_MAX_PARALLEL=4
export OPENEXEC_DAEMON_POLL_INTERVAL=60

# API Keys (secrets - only via env vars, never in files)
export OPENEXEC_CLAUDE_API_KEY=sk-ant-...
export OPENEXEC_CODEX_API_KEY=sk-...
export OPENEXEC_GEMINI_API_KEY=AIza...
```

## Agent Comparison

| Feature | Claude Code | Codex | Gemini | OpenCode |
|---------|-------------|-------|--------|----------|
| Provider | Anthropic | OpenAI | Google | Ollama/Local |
| CLI Command | `claude` | `codex` | `gemini` | `opencode` |
| Auth | API key | API key | API key | None (local) |
| Cost | Per token | Per token | Per token | Free |
| Speed | Fast | Fast | Fast | Varies |
| Best For | General coding | Code completion | Multi-modal | Privacy/offline |
| Models | sonnet, opus | gpt-4.1, gpt-5 | 3.1-pro-preview, 3.1-flash-preview | Any Ollama |

## Quality Gate Configuration

### Built-in Gates

```yaml
quality:
  gates:
    - lint          # Uses configured linter
    - typecheck     # Type checking (mypy, tsc, etc.)
    - test          # Run test suite
    - security      # Security scanning
    - format        # Code formatting check
```

### Custom Gates by Language

**Go:**
```yaml
quality:
  gates: [go_fmt, go_lint, go_test, go_sec]
  custom:
    - name: go_fmt
      command: "go fmt ./... && git diff --exit-code -- '*.go'"
      mode: blocking
    - name: go_lint
      command: "golangci-lint run ./..."
      mode: blocking
    - name: go_test
      command: "go test -v -race ./..."
      mode: blocking
    - name: go_sec
      command: "gosec ./..."
      mode: blocking
```

**Python:**
```yaml
quality:
  gates: [lint, typecheck, test]
  custom:
    - name: lint
      command: "ruff check src/"
      mode: blocking
    - name: typecheck
      command: "mypy src/"
      mode: blocking
    - name: test
      command: "pytest --cov=src"
      mode: blocking
```

**TypeScript/JavaScript:**
```yaml
quality:
  gates: [lint, typecheck, test, audit]
  custom:
    - name: lint
      command: "npm run lint"
      mode: blocking
    - name: typecheck
      command: "npm run type-check"
      mode: blocking
    - name: test
      command: "npm test"
      mode: blocking
    - name: audit
      command: "npm audit --omit=dev"
      mode: blocking
```

**Rust:**
```yaml
quality:
  gates: [fmt, clippy, test]
  custom:
    - name: fmt
      command: "cargo fmt --check"
      mode: blocking
    - name: clippy
      command: "cargo clippy -- -D warnings"
      mode: blocking
    - name: test
      command: "cargo test"
      mode: blocking
```

## Switching Agents Mid-Project

You can change agents at any time by updating `openexec.yaml`:

```bash
# Edit config
vim openexec.yaml

# Or override with env var for a single run
OPENEXEC_DEFAULT_AGENT=codex openexec run T-001
```

The daemon automatically reloads config every 60 seconds (configurable via `config_reload_interval`).
