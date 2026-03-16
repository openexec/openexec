# Agent Instructions: Wire Quality Gates into Blueprint Execution Mode
## openexec — Self-Bootstrapping Implementation

**Version:** 1.0  
**Target:** AI agent with shell access and Go toolchain  
**Problem being solved:** Blueprint execution mode completes stages without invoking quality gates, allowing false-positive pipeline runs. Quality gates run correctly in iterative loop mode but are not called from the blueprint executor.

---

## How to read these instructions

Every stage in this document follows the same structure:

1. **Goal** — what must be true when the stage ends
2. **Tasks** — the work to perform
3. **Mandatory verification commands** — shell commands you MUST run and whose output you MUST capture
4. **Proof artifact** — a file you MUST write before moving to the next stage

You cannot proceed to a later stage unless the proof artifact for the current stage exists and contains the expected content. There is no self-certification. If a command fails, you stay in the current stage and fix the problem. The instructions are designed so that the verification harness is built before the implementation, meaning you are never in a position of checking your own work — the harness checks it for you.

---

## Preconditions — run these before starting

Before writing any code, establish a verified baseline. Run all three commands and write their output to `artifacts/baseline.txt`.

```bash
# 1. Verify the project builds cleanly
go build ./... 2>&1

# 2. Verify all existing tests pass
go test ./... 2>&1

# 3. Snapshot the current test count so Stage 4 can prove you added tests
go test ./... -v 2>&1 | grep -c "^--- PASS" > artifacts/baseline_test_count.txt

# 4. Verify the quality gate runner exists and is reachable
grep -rn "RunGates\|RunAll\|QualityGate" --include="*.go" . 2>&1
```

**If the build fails or any test fails at baseline**, stop. Do not proceed. The codebase must be green before you modify it. Report the failures.

Write a file `artifacts/preconditions.txt` containing the output of all four commands and the word `BASELINE_GREEN` on the last line. If you cannot write `BASELINE_GREEN` honestly, stop and report.

---

## Stage 1 — Understand the existing wiring (read-only)

**Goal:** Produce a precise, written specification of exactly which functions need to be connected and why the connection is currently absent.

The purpose of this stage is to prevent assumption propagation — the failure mode where the agent misunderstands the codebase early and then builds confidently on wrong premises. You must read before you write.

**Tasks:**

Locate and read the following components. For each one, note the file path, the relevant function or type name, and what it currently does.

First, find where the blueprint executor handles stage execution. Look for the function that iterates over blueprint stages and runs their commands. This is the function you will modify.

```bash
grep -rn "stage\|Stage\|blueprint\|Blueprint" --include="*.go" . | grep -i "execut\|run\|process" | head -40
```

Second, find the quality gate runner that already works in loop mode. Look for the function that loads gates from configuration and runs them.

```bash
grep -rn "quality\|gate\|Gate\|RunGates\|LoadGates" --include="*.go" . | head -40
```

Third, find where the iterative loop invokes quality gates, so you can replicate that call pattern in the blueprint executor.

```bash
grep -rn "RunGates\|RunAll\|qualityGate\|quality_gate" --include="*.go" . | head -40
```

Fourth, check whether the blueprint executor already has access to a gate runner (via a struct field or parameter), or whether you need to add that dependency.

```bash
grep -rn "struct\|type.*Executor\|type.*Runner\|type.*Blueprint" --include="*.go" . | grep -v "_test" | head -30
```

**Proof artifact — `artifacts/spec.md`**

Write this file before proceeding to Stage 2. It must contain:

```markdown
## Blueprint executor location
File: [path]
Function: [name]
Current behaviour: [one paragraph describing what happens now]

## Quality gate runner location
File: [path]
Function/type: [name]
Signature: [the exact Go function signature]

## How loop mode invokes gates
File: [path]
Lines: [line numbers]
Pattern: [the exact call, copied verbatim from source]

## Dependency gap
Does the blueprint executor struct currently have a field for the gate runner? [yes/no]
If no: what field name and type needs to be added? [answer]

## Proposed wiring — three sentences maximum
[Describe the minimal change needed, naming exact functions and types]
```

Do not proceed to Stage 2 until `artifacts/spec.md` exists and all fields are filled in with real file paths and real function names. Placeholders like "TBD" or "see codebase" are not acceptable.

---

## Stage 2 — Build the verification harness before any implementation

**Goal:** A failing integration test that proves the gap exists. The test must compile. It must run. It must fail with a message that describes the missing behaviour, not a missing symbol.

This is the most important stage. The test you write here becomes the external verifier for all subsequent stages. The agent will not self-certify completion — this test will certify it.

**The principle:** write the test you wish you could already run to prove the system works, then make the system satisfy it.

**Tasks:**

Create a new test file. A suitable location is alongside the blueprint executor, for example `executor/blueprint_integration_test.go` or equivalent based on what Stage 1 revealed. Name it clearly as an integration test.

Write a test with the following structure. Adapt the function names to match what you found in Stage 1, but do not change the structural intent.

```go
// TestBlueprintExecutor_InvokesQualityGates proves that when the blueprint
// executor runs a stage, it calls the quality gate runner and propagates
// failures. This test currently FAILS because the wiring does not exist.
// It will pass once Stage 3 is complete.
func TestBlueprintExecutor_InvokesQualityGates(t *testing.T) {
    // Arrange: create a mock/spy gate runner that records whether it was called.
    // Use a simple struct with a boolean flag — no external mocking library.
    gatesCalled := false
    spyRunner := &SpyGateRunner{
        OnRun: func() error {
            gatesCalled = true
            return nil
        },
    }

    // Create a blueprint executor with the spy injected.
    // This will fail to compile if the executor struct does not yet have
    // a GateRunner field — which is expected at this stage.
    executor := NewBlueprintExecutor(WithGateRunner(spyRunner))

    // Act: run a minimal blueprint with one stage that has no commands.
    // A no-command stage is the exact scenario that currently auto-succeeds
    // without invoking gates.
    plan := &Plan{
        Stages: []Stage{
            {Name: "test-stage", Commands: []string{}},
        },
    }
    _ = executor.Execute(context.Background(), plan)

    // Assert: gates were called even though the stage had no commands.
    if !gatesCalled {
        t.Fatal("quality gates were not invoked during blueprint execution — this is the gap being fixed")
    }
}
```

Also write the `SpyGateRunner` type in the same test file or a `_test.go` helper:

```go
type SpyGateRunner struct {
    OnRun func() error
}

func (s *SpyGateRunner) RunAll(ctx context.Context) error {
    if s.OnRun != nil {
        return s.OnRun()
    }
    return nil
}
```

**Mandatory verification — the test must fail correctly:**

```bash
go test ./... -run TestBlueprintExecutor_InvokesQualityGates -v 2>&1
```

Examine the output. You are looking for one of two acceptable failure modes:

- **Acceptable:** The test compiles and runs but fails with `quality gates were not invoked` — this means your spy is wired correctly and the gap is proven.
- **Acceptable at this stage only:** The test fails to compile because `NewBlueprintExecutor` does not yet accept `WithGateRunner` — this means the struct modification in Stage 3 is still needed. Note the compile error explicitly.
- **Not acceptable:** The test passes. If it passes now, the gap does not exist or your spy is broken. Investigate before proceeding.

**Proof artifact — `artifacts/stage2_harness.txt`**

Run the test command above and redirect its full output to this file:

```bash
go test ./... -run TestBlueprintExecutor_InvokesQualityGates -v 2>&1 | tee artifacts/stage2_harness.txt
```

The file must exist. The last line of the file must contain either `FAIL` or a compile error. If it contains `PASS`, stop and investigate.

---

## Stage 3 — Implement the wiring

**Goal:** The integration test from Stage 2 passes. No other tests break. The implementation is the minimum change that satisfies the test — no new packages, no new abstractions, no more than four files modified.

**The scope constraint is not optional.** If you find yourself creating a new package, a new interface with more than two methods, or modifying more than four files, you are over-engineering. Stop, re-read `artifacts/spec.md`, and find the simpler path.

**Tasks, in order:**

**Task 3a — Add the gate runner field to the executor struct.**

Based on what `artifacts/spec.md` says about the dependency gap, add a `GateRunner` interface (if it does not exist) and a field to the blueprint executor struct. The interface should have exactly the methods the existing loop-mode invocation uses — no more.

After this change, run:

```bash
go build ./... 2>&1
```

The build must succeed before you proceed to Task 3b. If it fails, fix it now.

**Task 3b — Add the `WithGateRunner` option or constructor parameter.**

Add the injection mechanism that your Stage 2 test used. This may be a functional option (`WithGateRunner`), a constructor parameter, or a setter — follow the existing pattern in the codebase. Do not invent a new pattern.

After this change, run:

```bash
go build ./... 2>&1
go test ./... -run TestBlueprintExecutor_InvokesQualityGates -v 2>&1
```

The build must succeed. The test will likely still fail (because the gate runner is not yet called in `Execute()`), but it must compile now and fail with `quality gates were not invoked`, not a compile error.

**Task 3c — Call the gate runner in the executor's Execute function.**

In the blueprint executor's `Execute` function (or equivalent stage-running loop), add the call to `GateRunner.RunAll()` after each stage completes. Propagate the error — do not swallow it with `_ =`.

The call should be placed so that every stage invokes it, not just stages with non-empty command lists. The specific requirement is: **a stage with zero commands must still invoke quality gates**.

Add error propagation:

```go
// Call quality gates after every stage, regardless of command count.
// Stages with no commands previously auto-succeeded without verification —
// this call closes that gap.
if g.gateRunner != nil {
    if err := g.gateRunner.RunAll(ctx); err != nil {
        return fmt.Errorf("stage %q failed quality gates: %w", stage.Name, err)
    }
}
```

After this change, run all three checks:

```bash
go build ./... 2>&1
go test ./... -run TestBlueprintExecutor_InvokesQualityGates -v 2>&1
go test ./... 2>&1
```

All three must succeed before proceeding.

**Task 3d — Wire the real gate runner at the composition root.**

Find where the blueprint executor is instantiated in production code (not test code). Inject the real gate runner there, using the same pattern the iterative loop mode uses to obtain its gate runner. This is the step that makes the fix real — without it, the test passes but production behaviour is unchanged.

```bash
grep -rn "NewBlueprintExecutor\|blueprintExecutor{" --include="*.go" . | grep -v "_test" 2>&1
```

Modify that instantiation site. Then run a smoke test: if there is a CLI command that runs a blueprint, run it against a minimal blueprint and verify it exits cleanly.

**Proof artifact — `artifacts/stage3_results.txt`**

```bash
{
  echo "=== Build ==="
  go build ./... 2>&1
  echo "=== Integration test ==="
  go test ./... -run TestBlueprintExecutor_InvokesQualityGates -v 2>&1
  echo "=== All tests ==="
  go test ./... 2>&1
  echo "=== Test count ==="
  go test ./... -v 2>&1 | grep -c "^--- PASS"
} | tee artifacts/stage3_results.txt
```

The file must contain `PASS` for the integration test. It must not contain `FAIL` anywhere. The test count must be higher than the number in `artifacts/baseline_test_count.txt`.

---

## Stage 4 — Verify the wiring is complete and cannot regress

**Goal:** A meta-test that prevents future execution paths from bypassing gates. Proof that the audit trail captures gate execution. Confirmation that no shortcuts were taken.

**Tasks:**

**Task 4a — Write the regression guard test.**

This test enumerates all blueprint executors (or all implementations of the executor interface) and verifies that each one is wrapped with gate enforcement. Its purpose is to make the wiring durable: any future contributor who adds a new execution mode will be forced by a test failure to add gate enforcement.

```go
// TestAllBlueprintExecutors_HaveGateEnforcement prevents regression by
// verifying that every registered executor is wrapped with gate enforcement.
// If you add a new executor and this test fails, you must wire gates before merging.
func TestAllBlueprintExecutors_HaveGateEnforcement(t *testing.T) {
    // This test's implementation depends on how executors are registered
    // in the specific codebase. The pattern to follow:
    //
    // Option A (if executors are registered in a map or slice):
    //   iterate the registry and check the type of each entry.
    //
    // Option B (if the blueprint executor is a single struct):
    //   instantiate it and verify that attempting to Execute() without
    //   a gate runner either panics or returns a configuration error,
    //   documenting the requirement explicitly.
    //
    // Choose the option that matches the codebase structure from Stage 1.
    t.Log("gate enforcement regression guard — adapt to codebase structure")
}
```

Adapt this test to the actual codebase structure. The key requirement is: the test must fail if someone removes gate enforcement, and must produce a message that explains why gate enforcement is mandatory.

**Task 4b — Verify that no error suppression was introduced.**

Run this check and ensure it produces zero results:

```bash
grep -rn "_ = .*[Gg]ate\|_ = .*RunAll\|//nolint.*gate" --include="*.go" . 2>&1
```

Zero matches means no gate errors are being silently swallowed. If you find matches, fix them.

**Task 4c — Verify that quality gates cannot be bypassed by empty command lists.**

This is the original bug. Write a direct test for it:

```go
// TestBlueprintExecutor_EmptyStage_StillRunsGates is the regression test
// for the original bug: a stage with no commands auto-succeeded without
// invoking quality gates.
func TestBlueprintExecutor_EmptyStage_StillRunsGates(t *testing.T) {
    gatesCalled := false
    executor := NewBlueprintExecutor(WithGateRunner(&SpyGateRunner{
        OnRun: func() error { gatesCalled = true; return nil },
    }))

    plan := &Plan{Stages: []Stage{{Name: "empty", Commands: []string{}}}}
    err := executor.Execute(context.Background(), plan)

    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !gatesCalled {
        t.Fatal("an empty-command stage bypassed quality gates — original bug has regressed")
    }
}
```

**Task 4d — Run the complete final verification.**

```bash
{
  echo "=== Final build ==="
  go build ./... 2>&1
  
  echo "=== Lint ==="
  golangci-lint run ./... 2>&1 || go vet ./... 2>&1
  
  echo "=== All tests including new regression guards ==="
  go test ./... -v 2>&1 | tail -30
  
  echo "=== No silenced gate errors ==="
  grep -rn "_ = .*[Gg]ate\|_ = .*RunAll" --include="*.go" . 2>&1 || echo "CLEAN"
  
  echo "=== Test count delta ==="
  BASELINE=$(cat artifacts/baseline_test_count.txt)
  CURRENT=$(go test ./... -v 2>&1 | grep -c "^--- PASS")
  echo "Baseline: $BASELINE, Current: $CURRENT, Delta: $((CURRENT - BASELINE))"
  
  echo "=== Proof artifacts present ==="
  ls -la artifacts/*.txt 2>&1
  
} | tee artifacts/stage4_final_verification.txt
```

**Proof artifact — `artifacts/stage4_final_verification.txt`**

This file is the audit trail of the entire implementation. It must contain:

- No `FAIL` lines
- A positive test count delta (more tests than baseline)
- `CLEAN` from the error-suppression check, or zero matches
- All five artifact files listed by `ls`

---

## The self-referential checkpoint: dogfooding via openexec itself

Once Stages 1–4 are complete, there is one final verification that closes the dogfooding loop: **run a blueprint through openexec itself and confirm that the quality gates are invoked**.

Create a minimal blueprint at `.openexec/blueprints/self-test.yaml`:

```yaml
name: gate-wiring-self-test
description: Proves that blueprint execution invokes quality gates
stages:
  - name: empty-stage-must-still-gate
    commands: []
  - name: build
    commands:
      - go build ./...
  - name: test
    commands:
      - go test ./...
```

Run it:

```bash
openexec blueprint run .openexec/blueprints/self-test.yaml 2>&1 | tee artifacts/dogfood_run.txt
```

Examine the output. You are looking for evidence that the gate runner was invoked for the `empty-stage-must-still-gate` stage. If the system has logging for gate execution, those lines must appear. If gate execution is silent (no log output), add a single `slog.Info("quality gates invoked", "stage", stage.Name)` line in the wiring code and re-run.

The blueprint run must exit 0. The artifact `artifacts/dogfood_run.txt` must exist and must contain log evidence of gate invocation.

---

## What "done" means for this task

The following conditions must ALL be true. Check each one explicitly:

First, `go test ./...` passes with zero failures, and the test count is strictly higher than `artifacts/baseline_test_count.txt`. This proves tests were added, not just code.

Second, `TestBlueprintExecutor_InvokesQualityGates` passes. This is the primary proof that the gap is closed.

Third, `TestBlueprintExecutor_EmptyStage_StillRunsGates` passes. This is the regression guard for the original bug.

Fourth, the composition root wires the real gate runner — not just a test-only spy. Production blueprints must also invoke gates.

Fifth, `artifacts/dogfood_run.txt` exists and contains log evidence that the self-test blueprint invoked quality gates on the empty-command stage.

Sixth, all five proof artifact files exist in the `artifacts/` directory.

If any of these conditions is false, the task is not complete. Partial completion is not acceptable. Return to the stage where the condition is unmet and fix it.

---

## If you encounter a gap in these instructions themselves

These instructions anticipate the most common ways Go orchestration wiring tasks go wrong. But every codebase is different. If you encounter a situation not covered here — for example, the gate runner uses a different calling convention than expected, or the executor is not a struct but a function — apply the following general principle:

**Find the equivalent pattern in the existing codebase and follow it.** The iterative loop mode already invokes quality gates correctly. If these instructions describe a pattern that does not fit the codebase, look at how the loop mode does it and replicate that approach in the blueprint executor. Document your deviation in `artifacts/spec.md` under a section called `Deviations from instructions`.

The goal is not to follow these instructions literally. The goal is to produce a blueprint executor that invokes quality gates on every stage, with tests that prove it, and an audit trail that confirms the tests ran. Any path that produces those outcomes is acceptable.
