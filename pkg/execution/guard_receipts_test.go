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
	"regexp"
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
//  3. red: the SAME named tests, guard removed, exit 1, "--- FAIL: <test>" and
//     the guard's intended assertion in the output (a crash is not "red")
//  4. restored: the SAME named tests, no overlay, green again
//
// Run with -v to read the raw receipts.

type guardCase struct {
	name, file, old, new string
	tests                []string
	assertion            string
}

const bindGuard = `complete := completeScans[position] != "" && completeScans[position] == sha256Hex(block.ToolOutput)`

var guardCases = []guardCase{
	{"G1 position+hash binding", "context_recovery.go", bindGuard, `complete := completeScans[position] != ""`, []string{"TestSearchOmissionRequiresPositionBoundScanProvenance", "TestOmissionPreservationClasses"}, "different text at the position: result was omitted or request altered"},
	{"G2 provenance required", "context_recovery.go", bindGuard, `complete := true`, []string{"TestSearchOmissionRequiresPositionBoundScanProvenance", "TestOmissionPreservationClasses"}, "no provenance: result was omitted or request altered"},
	{"G3 unambiguous call-ID pairing", "context_recovery.go", `complete && !ambiguous[block.ToolResultID]`, `complete`, []string{"TestSearchOmissionRequiresPositionBoundScanProvenance"}, "ambiguous reused id: result was omitted or request altered"},
	{"G4 only list_directory/search_files", "context_recovery.go", "\t\t\t\t\t\tcase \"search_files\":\n\t\t\t\t\t\t\tlabels[position] = \"complete search result\"\n", "\t\t\t\t\t\tcase \"search_files\":\n\t\t\t\t\t\t\tlabels[position] = \"complete search result\"\n\t\t\t\t\t\tdefault:\n\t\t\t\t\t\t\tlabels[position] = \"result\"\n", []string{"TestSearchOmissionRequiresPositionBoundScanProvenance", "TestOmissionPreservationClasses"}, "read_file: result was omitted or request altered"},
	{"G5 no tool error", "context_recovery.go", `labels[[2]int{i, j}] == "" || b.ToolError != ""`, `labels[[2]int{i, j}] == ""`, []string{"TestSearchOmissionRequiresPositionBoundScanProvenance", "TestOmissionPreservationClasses"}, "errored result with provenance: result was omitted or request altered"},
	{"G6 minimum size 256", "context_recovery.go", `b.ToolError != "" || len(b.ToolOutput) < 256`, `b.ToolError != ""`, []string{"TestOmissionEligibilityRequiresPairingNoErrorAndSize"}, "short listing was omitted"},
	{"G7 newest result kept", "context_recovery.go", `if kept <= 1 {`, `if kept <= 0 {`, []string{"TestSearchOmissionRequiresPositionBoundScanProvenance", "TestOmissionPreservationClasses"}, "complete scans with provenance: older omitted, newest kept"},
	{"G8 recorder: no tool error", "token_budget.go", "\tif toolErr != nil {\n\t\treturn\n\t}\n\tif (tool", "\tif (tool", []string{"TestOmissionPreservationClasses"}, "a failed listing was recorded"},
	{"G9 recorder: scan-complete flag", "token_budget.go", `(tool == "search_files" && scanComplete)`, `(tool == "search_files")`, []string{"TestOmissionPreservationClasses"}, "an incomplete scan was recorded"},
	{"G10 recorder: vouching executor", "token_budget.go", `(tool == "list_directory" && p.trustedDirectoryResults)`, `(tool == "list_directory")`, []string{"TestListingProvenanceComesFromVouchingExecutor"}, "unvouched listing was recorded"},
	{"G11 call/result pairing present", "context_recovery.go", "if len(calls) > 0 {", "if true {", []string{"TestOmissionEligibilityRequiresPairingNoErrorAndSize"}, "unpaired result must be preserved without panic"},
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

// crashMarker matches what the Go runtime prints when a test binary dies.
var crashMarker = regexp.MustCompile(`(?m)^\s*(panic: |goroutine \d+ \[)`)

// classifyRed accepts only an intended assertion failure: a compiled test binary
// that ran to its own verdict, failed a named test, and printed the guard's
// assertion. A compile failure or a crash also exits 1 but is not a red receipt.
func classifyRed(out string, code int, tests []string, assertion string) error {
	switch {
	case strings.Contains(out, "build failed"):
		return fmt.Errorf("compile failure is not red")
	case crashMarker.MatchString(out):
		return fmt.Errorf("crash is not an intended assertion red")
	case code != 1:
		return fmt.Errorf("expected exit 1, got exit %d", code)
	case !strings.Contains(out, assertion):
		return fmt.Errorf("intended assertion %q is not in the output", assertion)
	}
	for _, name := range tests {
		if strings.Contains(out, "--- FAIL: "+name) {
			return nil
		}
	}
	return fmt.Errorf("none of the named tests %v reported FAIL", tests)
}

// overlayFor writes mutated versions of package files and returns a go overlay
// file that supplies them to the compiler only.
func overlayFor(t *testing.T, mutated map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	replace := map[string]string{}
	for file, content := range mutated {
		abs, err := filepath.Abs(file)
		if err != nil {
			t.Fatal(err)
		}
		mf := filepath.Join(dir, "mutated_"+file)
		if err := os.WriteFile(mf, content, 0600); err != nil {
			t.Fatal(err)
		}
		replace[abs] = mf
	}
	raw, _ := json.Marshal(map[string]map[string]string{"Replace": replace})
	of := filepath.Join(dir, "overlay.json")
	if err := os.WriteFile(of, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return of
}

func mutate(t *testing.T, g guardCase) []byte {
	t.Helper()
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
	return mutated
}

// treeIdentity is where the receipts were taken: the candidate commit and tree,
// the whole repository's index and the bytes of every tracked file.
type treeIdentity struct {
	head, listing, manifest, dirty string
}

func (i treeIdentity) indexSHA() string { return fmt.Sprintf("%x", sha256.Sum256([]byte(i.listing))) }
func (i treeIdentity) manifestSHA() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(i.manifest)))
}

func gitOutput(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git -C %s %s: %v\n%s", dir, strings.Join(args, " "), err, stderr.String())
	}
	return stdout.Bytes()
}

// captureTreeIdentity reads the index at the repository root and hashes every
// tracked file on disk. A receipt without provenance is not a receipt, so a git
// failure fails the test.
func captureTreeIdentity(t *testing.T, root string) treeIdentity {
	t.Helper()
	identity := treeIdentity{
		head:    string(gitOutput(t, root, "rev-parse", "HEAD", "HEAD^{tree}")),
		listing: string(gitOutput(t, root, "ls-files", "-s")),
	}
	var manifest, dirty strings.Builder
	for _, entry := range strings.Split(strings.TrimSuffix(string(gitOutput(t, root, "ls-files", "-s", "-z")), "\x00"), "\x00") {
		meta, path, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 {
			t.Fatalf("unparseable index entry %q", entry)
		}
		content, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			fmt.Fprintf(&manifest, "unreadable %v %s\n", err, path)
			fmt.Fprintf(&dirty, "%s\n", path)
			continue
		}
		blob := gitBlob(content)
		fmt.Fprintf(&manifest, "%x %s %d %s\n", sha256.Sum256(content), blob, len(content), path)
		if blob != fields[1] {
			fmt.Fprintf(&dirty, "%s\n", path)
		}
	}
	identity.manifest, identity.dirty = manifest.String(), dirty.String()
	return identity
}

func TestGuardRemovalIsRedAndRestoredIsGreen(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns go test subprocesses")
	}
	wd, _ := os.Getwd()
	root := strings.TrimSpace(string(gitOutput(t, wd, "rev-parse", "--show-toplevel")))
	before := captureTreeIdentity(t, root)
	t.Logf("PROVENANCE before any receipt\nworking directory: %s\ngit root: %s\ngit rev-parse HEAD HEAD^{tree}:\n%sgit -C %s ls-files -s:\n%ssha256(git -C %s ls-files -s)=%s\ntracked file manifest (sha256 git-blob bytes path):\n%ssha256(manifest)=%s\ntracked files whose bytes differ from the index:\n%s", wd, root, before.head, root, before.listing, root, before.indexSHA(), before.manifest, before.manifestSHA(), before.dirty)

	// P0 runs FIRST: the original positive assertion on the compiling tree.
	t.Run("P0", func(t *testing.T) { originalPositiveReceipt(t) })
	for _, g := range guardCases {
		t.Run(strings.Fields(g.name)[0], func(t *testing.T) {
			orig, err := os.ReadFile(g.file)
			if err != nil {
				t.Fatal(err)
			}
			mutated := mutate(t, g)
			t.Logf("GUARD %s | file %s | tests %v | intended assertion %q\nsha256(original)=%x git-blob(original)=%s\nsha256(mutated)=%x", g.name, g.file, g.tests, g.assertion, sha256.Sum256(orig), gitBlob(orig), sha256.Sum256(mutated))
			pat := runPattern(g.tests)
			if out, code := runGo(t, g.name+": baseline (guard present)", "test", "-count=1", "-v", "-run", pat, "."); code != 0 {
				t.Fatalf("baseline not green (exit %d): %s", code, out)
			}
			overlay := overlayFor(t, map[string][]byte{g.file: mutated})
			t.Logf("mutation: replace exactly one occurrence of\n%s\nwith\n%s\noverlay file: %s", g.old, g.new, overlay)
			if out, code := runGo(t, g.name+": mutated tree compiles", "test", "-overlay", overlay, "-count=1", "-run", "^$", "."); code != 0 || strings.Contains(out, "build failed") {
				t.Fatalf("mutated tree must compile; a compile failure is not red (exit %d)", code)
			}
			out, code := runGo(t, g.name+": RED (guard removed)", "test", "-overlay", overlay, "-count=1", "-v", "-run", pat, ".")
			if err := classifyRed(out, code, g.tests, g.assertion); err != nil {
				t.Fatalf("guard removal is not an intended red: %v", err)
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

	after := captureTreeIdentity(t, root)
	t.Logf("PROVENANCE after all receipts (final restored source and index)\ngit rev-parse HEAD HEAD^{tree}:\n%ssha256(git -C %s ls-files -s)=%s\nsha256(manifest)=%s\ntracked files whose bytes differ from the index:\n%s", after.head, root, after.indexSHA(), after.manifestSHA(), after.dirty)
	if after != before {
		t.Errorf("source or index identity changed during the receipts\nafter listing:\n%s\nafter manifest:\n%s", after.listing, after.manifest)
	}
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
	overlay := overlayFor(t, map[string][]byte{"context_recovery.go": append(orig, []byte("\n// inert edit\n")...)})
	pat := runPattern([]string{"TestSearchOmissionRequiresPositionBoundScanProvenance", "TestOmissionPreservationClasses"})
	if out, code := runGo(t, "inert edit", "test", "-overlay", overlay, "-count=1", "-run", pat, "."); code != 0 {
		t.Fatalf("an inert edit must stay green, exit %d: %s", code, out)
	}
}

// Near misses for classifyRed, each from a real `go test` run: G11 with the
// test-boundary capture removed really crashes, a syntax error really fails to
// build, and an assertion the run did not print is refused. The intact G11 run
// is the one that must be accepted.
func TestGuardHarnessOnlyAcceptsIntendedAssertionRed(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns go test subprocesses")
	}
	var g11 guardCase
	for _, g := range guardCases {
		if strings.HasPrefix(g.name, "G11 ") {
			g11 = g
		}
	}
	pat := runPattern(g11.tests)
	mutated := mutate(t, g11)

	const testFile = "context_structural_eligibility_test.go"
	const capture = "defer func() { panicked = recover() }()"
	testSource, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(testSource), capture) != 1 {
		t.Fatalf("capture must occur exactly once in %s", testFile)
	}
	uncaptured := overlayFor(t, map[string][]byte{g11.file: mutated, testFile: []byte(strings.Replace(string(testSource), capture, "", 1))})
	out, code := runGo(t, "G11 without the capture", "test", "-overlay", uncaptured, "-count=1", "-v", "-run", pat, ".")
	if err := classifyRed(out, code, g11.tests, g11.assertion); err == nil || !strings.Contains(err.Error(), "crash") {
		t.Errorf("a real crash must be refused as a crash, got %v", err)
	}

	broken := overlayFor(t, map[string][]byte{g11.file: append(mutated, []byte("\nnot go\n")...)})
	out, code = runGo(t, "G11 that does not compile", "test", "-overlay", broken, "-count=1", "-v", "-run", pat, ".")
	if err := classifyRed(out, code, g11.tests, g11.assertion); err == nil || !strings.Contains(err.Error(), "compile") {
		t.Errorf("a real compile failure must be refused as such, got %v", err)
	}

	intact := overlayFor(t, map[string][]byte{g11.file: mutated})
	out, code = runGo(t, "G11 intended red", "test", "-overlay", intact, "-count=1", "-v", "-run", pat, ".")
	if err := classifyRed(out, code, g11.tests, g11.assertion); err != nil {
		t.Errorf("the intended assertion red must be accepted, got %v", err)
	}
	if err := classifyRed(out, code, g11.tests, "an assertion this run never printed"); err == nil || !strings.Contains(err.Error(), "intended assertion") {
		t.Errorf("a different assertion must be refused, got %v", err)
	}
	if err := classifyRed(out, code, []string{"TestSomeOtherTest"}, g11.assertion); err == nil || !strings.Contains(err.Error(), "named tests") {
		t.Errorf("a failure in another test must be refused, got %v", err)
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
	if err := classifyRed(out, code, []string{"TestOriginalPositiveQuoteRemainsBeforeRecall"}, "original quoted governing instruction is absent from the request before recall"); err != nil {
		t.Fatalf("expected the positive assertion to fail on a compiling tree: %v", err)
	}
	for _, tool := range []string{"list_directory", "search_files"} {
		if !strings.Contains(out, "--- FAIL: TestOriginalPositiveQuoteRemainsBeforeRecall/"+tool) {
			t.Fatalf("missing FAIL for %s", tool)
		}
	}
}
