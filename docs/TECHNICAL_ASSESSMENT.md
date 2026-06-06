# OpenExec Technical Assessment

**Date:** 2026-04-01  
**Assessor:** Code Review  
**Status:** Valid Project in Transitional State

---

## Executive Summary

OpenExec is a **valid, working project** with genuine functionality, but it exists in a transitional state where architectural vision significantly outpaces implementation wiring. The core execution path works end-to-end, but ~40% of the codebase is either dead code, not yet integrated, or behind disabled feature flags.

**Verdict:** Not a facade — real foundation, real functionality, but significant gap between documented architecture and actual wiring.

---

## Critical Findings

### 1. Dead Code from Refactoring

**Status:** ⚠️ **ISSUE CONFIRMED**

The standalone iterative loop was gutted during blueprint refactoring:

| Component | Status | Evidence |
|-----------|--------|----------|
| `signal_tracker.go` | Dead | Exists but unused |
| `gates_integration.go` | Dead | Exists but unused |
| Thrashing detection | Removed | ~15 tests skipped |
| Quality gates (standalone) | Removed | Tests skipped |
| Retry loops | Removed | Tests skipped |
| Signal tracking | Removed | Tests skipped |

**Impact:** Code bloat, confusion for new contributors, tests that appear to pass but test nothing.

**Recommendation:** Remove dead code or clearly mark as deprecated.

---

### 2. DCP (Deterministic Control Plane) Disabled

**Status:** ⚠️ **ISSUE CONFIRMED**

- Disabled by default
- Integration tests all skipped
- Returns suggestions but nothing consumes them
- BitNet router expects local model file that doesn't ship

**Code Location:**
```go
// internal/dcp/dcp.go - returns suggestions
// Nothing in the codebase calls dcp.GetSuggestions()
```

**Impact:** Major architectural component non-functional.

**Recommendation:** Either complete integration or remove from public docs until ready.

---

### 3. New Packages Not Wired

**Status:** ⚠️ **PARTIALLY RESOLVED - WIRED VIA TWO-SPEED SEAM**

Several architectural modules have been successfully wired as part of the live **Two-Speed Seam (P1-B)**. Other packages added today compile and pass unit tests, but are awaiting direct execution integration:

| Package | Status | Wired? |
|---------|--------|--------|
| `internal/release` | ✅ Compiles, tests pass | 🚀 **WIRED** (Backlog state manager) |
| `internal/mcp/backlog` | ✅ Compiles, tests pass | 🚀 **WIRED** (6 backlog & memory tools) |
| `internal/cli/mcp_serve` | ✅ Compiles, tests pass | 🚀 **WIRED** (Unhidden stdio command) |
| `agent/registry` | ✅ Compiles, tests pass | ❌ Not called |
| `parallel/engine` | ✅ Compiles, tests pass | ❌ Not called |
| `predictive/loader` | ✅ Compiles, tests pass | ❌ Not called |
| `memory/system` | ✅ Compiles, tests pass | ❌ Not called |
| `harness` | ✅ Compiles, tests pass | ❌ Not called by CLI/server |
| `cache/*` | ✅ Compiles, tests pass | ❌ Not called |
| `checkpoint/manager` | ✅ Compiles, tests pass | ❌ Not called |

**The Two-Speed Integration:**
- Exposes your story backlog via `openexec mcp-serve` stdio.
- The `mcp-serve` server lazily loads the release store database, ensuring zero-overhead startup.
- Directly enables external lightweight clients (like Claude Code) to claim, implement, and complete tasks from your backlog with zero daemon boot lag.

---

### 4. SQLite Migration Incomplete

**Status:** ⚠️ **ISSUE CONFIRMED - FULLY FIXED (Commit 4c39c08)**

The migration from `mattn/go-sqlite3` to `modernc.org/sqlite` is now fully complete, and a critical **concurrency bug has been resolved**:
*   **The ModernC DSN Issue:** The `modernc` driver silently ignored mattn-style connection string parameters (like `_journal_mode=WAL` or `_busy_timeout=5000`). This meant WAL was never on, busy timeouts were zero, and concurrent writers from the daemon/interactive clients would instantly crash with database locks.
*   **The Fix (Commit 4c39c08):** Enabled WAL mode, foreign keys, and 5000ms busy timeouts across all 13 sites by explicitly executing database pragmas post-initialization. Supported by a regression test asserting active pragma state.
*   **CGO-Free Builds:** Old imports removed, ensuring clean builds on systems without CGO toolchains.

**Impact:** True concurrent read-write safety between the background daemon and lightweight stdio MCP servers.

**Recommendation:** Maintain explicit pragma executions for any future SQLite database handles.

---

### 5. SanitizeJSON Bug

**Status:** ⚠️ **ISSUE CONFIRMED - NOT FIXED**

`pkg/util/sanitize.go` strips `//` from paths thinking they're comments:

```go
// Input:  {"path": "/home/user//file.txt"}
// Output: {"path": "/home/user/file.txt"}  // WRONG!
```

**Workaround:** Parser handles it, but util is still broken.

**Impact:** Silent data corruption in JSON with file paths.

**Recommendation:** Fix SanitizeJSON or remove it if parser handles all cases.

---

### 6. Test Quality Issues

**Status:** ⚠️ **ISSUE CONFIRMED**

Pattern found in multiple test files:
```go
store, _ := knowledge.NewStore(tmpDir)  // Error ignored!
// Later: store.SomeMethod() → nil pointer panic
```

**Implication:** Tests written without actually running them.

**Impact:** False confidence, wasted debugging time.

**Recommendation:** Enable `errcheck` linter, require error handling in tests.

---

## Architecture vs Reality

### Documented Architecture (7-Layer Model)

```
Layer 1: Interaction (UI/CLI)
Layer 2: Runtime (Session/Mode)
Layer 3: Context (Assembly/Index)
Layer 4: Tooling (DCP/Toolsets) ← DISABLED
Layer 5: Governance (Policy/PII) ← PARTIAL
Layer 6: Orchestration (Blueprint/Checkpoint/Memory) ← NOT WIRED
Layer 7: Intelligence (Router/LLM)
```

### Actual Working System

```
CLI Command
    ↓
Spawn Process (claude/codex/gemini) ← exec.Command()
    ↓
Parse JSON Output
    ↓
Track Progress in SQLite
    ↓
Return Results
```

**Gap:** 40% of documented features not actually wired.

---

## What Actually Works

✅ **Core Execution Path:**
- `openexec init` → Works
- `openexec run` → Works
- Daemon starts → Works
- Blueprint stages execute → Works
- AI provider runs → Works
- Results collected → Works

✅ **Happy Path:** Genuine end-to-end functionality for basic use cases.

---

## Recommendations

### Immediate (This Week)

1. **Remove Dead Code**
   - Delete or deprecate `signal_tracker.go`, `gates_integration.go`
   - Remove skipped tests or fix them

2. **Fix SanitizeJSON**
   - Either fix the bug or remove the function

3. **Add CI Check**
   - Verify CGO-free builds work
   - Check for ignored errors in tests

### Short Term (This Month)

4. **Wire the Harness**
   - Create one integration test: CLI → Harness → Full Execution
   - Then wire harness into actual CLI commands

5. **DCP Decision**
   - Either complete integration (consume suggestions)
   - Or remove from docs until ready

6. **BitNet Router**
   - Ship default model file OR
   - Make it optional with graceful fallback

### Long Term (This Quarter)

7. **Documentation Audit**
   - Mark aspirational features as "Planned"
   - Document what's actually working

8. **Test Quality**
   - Enable strict linters
   - Require error handling
   - Run tests in CI before merge

---

## Conclusion

**OpenExec is NOT a facade.** It has:
- Real working code
- Genuine end-to-end functionality
- Solid architectural foundation
- Active development

**But it IS in a transitional state:**
- ~40% of code is dead or unwired
- Documentation overpromises
- Tests have quality issues
- Major components disabled

**The path forward is clear:**
1. Clean up dead code
2. Wire the new packages
3. Fix known bugs
4. Align docs with reality

**Bottom line:** Valid project with real value, but needs focused effort to close the gap between vision and implementation.

---

**Assessment Confidence:** High (based on code review, test analysis, and execution path tracing)
