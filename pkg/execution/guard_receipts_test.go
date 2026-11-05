package execution

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// This file turns the red/restored-green guard receipts into a test a declared
// check runs. Each guard is removed through `go test -overlay`, so the working
// tree bytes are never modified: the overlay supplies the mutated file to the
// compiler only. For each guard it runs, with full unfiltered output logged:
//  1. baseline: the named tests, green, no overlay
//  2. compile: the mutated tree builds (a build failure is not "red")
//  3. red: the SAME named tests, guard removed, exit 1 and "--- FAIL: <test>"
//  4. restored: the SAME named tests, no overlay, green again
//
// Run with -v to read the raw receipts.

type guardCase struct {
	name, file, old, new string
	tests                []string
}

const bindGuard = `complete := completeScans[position] != "" && completeScans[position] == sha256Hex(block.ToolOutput)`

var guardCases = []guardCase{
	{"G1 position+hash binding", "context_recovery.go", bindGuard, `complete := completeScans[position] != ""`, []string{"TestSearchOmissionRequiresPositionBoundScanProvenance", "TestOmissionPreservationClasses"}},
	{"G2 provenance required", "context_recovery.go", bindGuard, `complete := true`, []string{"TestSearchOmissionRequiresPositionBoundScanProvenance", "TestOmissionPreservationClasses"}},
	{"G3 unambiguous call-ID pairing", "context_recovery.go", `complete && !ambiguous[block.ToolResultID]`, `complete`, []string{"TestSearchOmissionRequiresPositionBoundScanProvenance"}},
	{"G4 only list_directory/search_files", "context_recovery.go", "\t\t\t\t\t\tcase \"search_files\":\n\t\t\t\t\t\t\tlabels[position] = \"complete search result\"\n", "\t\t\t\t\t\tcase \"search_files\":\n\t\t\t\t\t\t\tlabels[position] = \"complete search result\"\n\t\t\t\t\t\tdefault:\n\t\t\t\t\t\t\tlabels[position] = \"result\"\n", []string{"TestSearchOmissionRequiresPositionBoundScanProvenance", "TestOmissionPreservationClasses"}},
	{"G5 no tool error", "context_recovery.go", `labels[[2]int{i, j}] == "" || b.ToolError != ""`, `labels[[2]int{i, j}] == ""`, []string{"TestSearchOmissionRequiresPositionBoundScanProvenance", "TestOmissionPreservationClasses"}},
	{"G6 minimum size 256", "context_recovery.go", `b.ToolError != "" || len(b.ToolOutput) < 256`, `b.ToolError != ""`, []string{"TestOmissionEligibilityRequiresPairingNoErrorAndSize"}},
	{"G7 newest result kept", "context_recovery.go", `if kept <= 1 {`, `if kept <= 0 {`, []string{"TestSearchOmissionRequiresPositionBoundScanProvenance", "TestOmissionPreservationClasses"}},
	{"G8 recorder: no tool error", "token_budget.go", "\tif toolErr != nil {\n\t\treturn\n\t}\n\tif (tool", "\tif (tool", []string{"TestOmissionPreservationClasses"}},
	{"G9 recorder: scan-complete flag", "token_budget.go", `(tool == "search_files" && scanComplete)`, `(tool == "search_files")`, []string{"TestOmissionPreservationClasses"}},
	{"G10 recorder: vouching executor", "token_budget.go", `(tool == "list_directory" && p.trustedDirectoryResults)`, `(tool == "list_directory")`, []string{"TestListingProvenanceComesFromVouchingExecutor"}},
	{"G11 call/result pairing present", "context_recovery.go", "if len(calls) > 0 {", "if true {", []string{"TestOmissionEligibilityRequiresPairingNoErrorAndSize"}},
}

func goBinary() string {
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	return filepath.Join(runtime.GOROOT(), "bin", "go")
}

func gitBlob(b []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(b))
	h.Write(b)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// runGo runs `go <args>` in the package directory and returns raw combined
// output and the exit code. Nothing is filtered.
func runGo(t *testing.T, label string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(goBinary(), args...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("%s: could not run go: %v", label, err)
		}
		code = ee.ExitCode()
	}
	t.Logf("--- %s\ncommand: go %s\nexit=%d\nraw output:\n%s", label, strings.Join(args, " "), code, out.String())
	return out.String(), code
}

func runPattern(tests []string) string { return "^(" + strings.Join(tests, "|") + ")$" }

func overlayFor(t *testing.T, file string, mutated []byte) string {
	t.Helper()
	abs, err := filepath.Abs(file)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	mf := filepath.Join(dir, "mutated.go")
	if err := os.WriteFile(mf, mutated, 0600); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]map[string]string{"Replace": {abs: mf}})
	of := filepath.Join(dir, "overlay.json")
	if err := os.WriteFile(of, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return of
}

// provenance logs where the receipts were taken: working directory, candidate
// commit, its tree, a hash of the index (`git ls-files -s`) and the hashes of
// the product files under test. Nothing is filtered; a git failure is logged
// raw, not hidden.
func provenance(t *testing.T, label string) {
	t.Helper()
	wd, _ := os.Getwd()
	t.Logf("PROVENANCE %s\nworking directory: %s", label, wd)
	git := func(args ...string) string {
		cmd := exec.Command("git", args...)
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		err := cmd.Run()
		return fmt.Sprintf("git %s -> err=%v\n%s", strings.Join(args, " "), err, out.String())
	}
	t.Logf("%s%s", git("rev-parse", "HEAD", "HEAD^{tree}"), git("ls-files", "-s", "."))
	idx := exec.Command("git", "ls-files", "-s", "--", ".")
	if raw, err := idx.Output(); err == nil {
		t.Logf("sha256(git ls-files -s -- pkg/execution)=%x", sha256.Sum256(raw))
	}
	for _, f := range []string{"context_recovery.go", "token_budget.go", "guard_receipts_test.go", "context_original_positive_test.go", "context_structural_eligibility_test.go", "context_preservation_test.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Logf("manifest %s: %v", f, err)
			continue
		}
		t.Logf("manifest %s sha256=%x git-blob=%s bytes=%d", f, sha256.Sum256(b), gitBlob(b), len(b))
	}
}

func TestGuardRemovalIsRedAndRestoredIsGreen(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns go test subprocesses")
	}
	provenance(t, "before any receipt")
	// P0 runs FIRST: the original positive assertion on the compiling tree.
	t.Run("P0", func(t *testing.T) { originalPositiveReceipt(t) })
	for _, g := range guardCases {
		t.Run(strings.Fields(g.name)[0], func(t *testing.T) {
			orig, err := os.ReadFile(g.file)
			if err != nil {
				t.Fatal(err)
			}
			if n := strings.Count(string(orig), g.old); n != 1 {
				t.Fatalf("guard text must occur exactly once in %s, found %d", g.file, n)
			}
			mutated := []byte(strings.Replace(string(orig), g.old, g.new, 1))
			if bytes.Equal(orig, mutated) {
				t.Fatal("mutation is a no-op")
			}
			t.Logf("GUARD %s | file %s | tests %v\nsha256(original)=%x git-blob(original)=%s\nsha256(mutated)=%x", g.name, g.file, g.tests, sha256.Sum256(orig), gitBlob(orig), sha256.Sum256(mutated))
			pat := runPattern(g.tests)
			if out, code := runGo(t, g.name+": baseline (guard present)", "test", "-count=1", "-v", "-run", pat, "."); code != 0 {
				t.Fatalf("baseline not green (exit %d): %s", code, out)
			}
			overlay := overlayFor(t, g.file, mutated)
			t.Logf("mutation: replace exactly one occurrence of\n%s\nwith\n%s\noverlay file: %s", g.old, g.new, overlay)
			if out, code := runGo(t, g.name+": mutated tree compiles", "test", "-overlay", overlay, "-count=1", "-run", "^$", "."); code != 0 || strings.Contains(out, "build failed") {
				t.Fatalf("mutated tree must compile; a compile failure is not red (exit %d)", code)
			}
			out, code := runGo(t, g.name+": RED (guard removed)", "test", "-overlay", overlay, "-count=1", "-v", "-run", pat, ".")
			if code != 1 || strings.Contains(out, "build failed") {
				t.Fatalf("guard removal must fail the tests with exit 1, got exit %d", code)
			}
			failed := false
			for _, name := range g.tests {
				failed = failed || strings.Contains(out, "--- FAIL: "+name)
			}
			if !failed {
				t.Fatalf("exit 1 but none of the named tests %v reported FAIL", g.tests)
			}
			if out, code := runGo(t, g.name+": GREEN (guard restored, same tests)", "test", "-count=1", "-v", "-run", pat, "."); code != 0 {
				t.Fatalf("restored tree not green (exit %d): %s", code, out)
			}
			after, _ := os.ReadFile(g.file)
			if !bytes.Equal(after, orig) {
				t.Fatal("working-tree bytes changed; the overlay must never write the source")
			}
		})
	}
	provenance(t, "after all receipts (final restored source and index)")
}

// Near miss: a semantically inert edit is not "red", so the harness does not
// call everything red.
func TestGuardHarnessInertEditStaysGreen(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns go test subprocesses")
	}
	orig, err := os.ReadFile("context_recovery.go")
	if err != nil {
		t.Fatal(err)
	}
	overlay := overlayFor(t, "context_recovery.go", append(orig, []byte("\n// inert edit\n")...))
	pat := runPattern([]string{"TestSearchOmissionRequiresPositionBoundScanProvenance", "TestOmissionPreservationClasses"})
	if out, code := runGo(t, "inert edit", "test", "-overlay", overlay, "-count=1", "-run", pat, "."); code != 0 {
		t.Fatalf("an inert edit must stay green, exit %d: %s", code, out)
	}
}

// The original positive requirement ("the governing quote is present before
// recall") is run for real, on the compiling tree, under the originalpositive
// build tag. It is expected to FAIL; the failure is the preserved executable
// contradiction with structural-only eligibility, with raw output logged.
func originalPositiveReceipt(t *testing.T) {
	t.Helper()
	src, err := os.ReadFile("context_recovery.go")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("tree: context_recovery.go sha256=%x git-blob=%s", sha256.Sum256(src), gitBlob(src))
	out, code := runGo(t, "ORIGINAL POSITIVE: quote present before recall", "test", "-tags", "originalpositive", "-count=1", "-v", "-run", "^TestOriginalPositiveQuoteRemainsBeforeRecall$", ".")
	if code != 1 || strings.Contains(out, "build failed") {
		t.Fatalf("expected the positive assertion to fail with exit 1 on a compiling tree, got exit %d", code)
	}
	for _, tool := range []string{"list_directory", "search_files"} {
		if !strings.Contains(out, "--- FAIL: TestOriginalPositiveQuoteRemainsBeforeRecall/"+tool) {
			t.Fatalf("missing FAIL for %s", tool)
		}
	}
}
