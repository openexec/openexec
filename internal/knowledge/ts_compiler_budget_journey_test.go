package knowledge

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The opt-in corpus journey reads the authorized sibling Console repository,
// then works ONLY in its own pinned disposable clone. Ordinary declared checks
// run the synthetic persisted journey. No natural capture is replayed.
var consoleBudgetCorpus = flag.Bool("console-budget-corpus", false, "run original c727 corpus using the read-only authorized agent-console sibling")

func TestCompilerDiagnosticBudgetPersistedJourney(t *testing.T) {
	if *consoleBudgetCorpus {
		// In a linked worktree, the common Git directory identifies the registered
		// OpenExec repository, whose sibling holds the explicitly authorized source.
		gitDir, err := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
		if err != nil {
			t.Fatal(err)
		}
		repository := filepath.Join(filepath.Dir(filepath.Dir(strings.TrimSpace(string(gitDir)))), "agent-console")
		compilerBudgetConsoleJourney(t, repository)
		return
	}
	root, _ := diagnosticBudgetFixture(t, 20, 800)
	s, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	scan, err := s.ScanRepository(t.Context(), root)
	if err != nil {
		s.Close()
		t.Fatal(err)
	}
	identity, err := s.EnsureRepositoryIdentity(t.Context(), root, "")
	if err != nil {
		s.Close()
		t.Fatal(err)
	}
	request := ChangedImpactRequest{Files: []string{"early/target.ts"}, MaxDepth: 2}
	before, err := s.ChangedImpactAnalysis(t.Context(), identity, request, DefaultChangedImpactLimits())
	if err != nil {
		s.Close()
		t.Fatal(err)
	}
	found := false
	for _, caller := range before.Propagation.DirectCallers {
		if caller.Node.DisplayName == "caller" && len(caller.Path) == 1 && caller.Path[0].EdgeType == "calls" {
			found = true
		}
	}
	if !found || before.Truncated || len(before.UnresolvedFiles) != 0 {
		s.Close()
		t.Fatalf("real compiler call missing/bounded: %+v", before)
	}
	target, err := s.ResolveGraphSymbol(t.Context(), identity, "target", "early/target.ts", "function", 10)
	if err != nil || target.Result.Candidate == nil {
		s.Close()
		t.Fatalf("target: %+v %v", target, err)
	}
	edges, err := s.FindSymbolRelationships(t.Context(), identity, target.Result.Candidate.Symbol.ID, true, 1, []string{"calls"}, DefaultGraphLimits())
	if err != nil {
		s.Close()
		t.Fatal(err)
	}
	if len(edges.Result.Edges) != 1 || edges.Result.Edges[0].Resolution != ResolutionCompilerExact || edges.Result.Edges[0].SourceFilePath != "later/caller.ts" {
		s.Close()
		t.Fatalf("lost compiler provenance: %+v", edges)
	}
	source, err := os.ReadFile(filepath.Join(root, "later/caller.ts"))
	if err != nil {
		s.Close()
		t.Fatal(err)
	}
	edge := edges.Result.Edges[0]
	if string(source[edge.SourceStartByte:edge.SourceEndByte]) != "target" {
		s.Close()
		t.Fatal("wrong reference source span")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	g := reopenCapturedGraph(t, root, scan.Generation)
	for _, c := range []string{"typescript.definitions", "typescript.imports", "typescript.exports"} {
		if g.Capabilities[c] != "compiler_exact" {
			t.Fatalf("%s=%s", c, g.Capabilities[c])
		}
	}
	reopened, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopened.SetRefreshOnRead(false)
	after, err := reopened.ChangedImpactAnalysis(t.Context(), identity, request, DefaultChangedImpactLimits())
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("impact/provenance changed after reopen: %v", err)
	}
	persisted, err := reopened.FindSymbolRelationships(t.Context(), identity, target.Result.Candidate.Symbol.ID, true, 1, []string{"calls"}, DefaultGraphLimits())
	if err != nil || !reflect.DeepEqual(edges, persisted) {
		t.Fatalf("typed edges changed after reopen: %v", err)
	}
	// The unrelated errors file must never become a caller merely because it
	// shares a configuration with target.
	for _, caller := range append(after.Propagation.DirectCallers, after.Propagation.AffectedCallers...) {
		if caller.Node.DisplayName == "early0" {
			t.Fatal("fabricated sibling caller")
		}
	}
	t.Log("compiler_exact scan; identical impact, typed edges, source spans, limitations and provenance after close/reopen; refresh disabled")
}

func compilerBudgetConsoleJourney(t *testing.T, repository string) {
	t.Helper()
	const pinned = "c727ba1e44099ee04afea852dfbffc3ea89eff7b"
	const corpusHash = "1c05a3b070b9a90baaf2da7420dc6161cbb6e994def201881a6cfb8a6ec37b86"
	root := filepath.Join(t.TempDir(), "console-c727")
	run := func(dir string, env []string, name string, args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), name, args...)
		cmd.Dir = dir
		cmd.Env = env
		var out, stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s %v: %v\n%s\n%s", name, args, err, stderr.String(), out.String())
		}
		return out.Bytes()
	}
	env := append(os.Environ(), "GOWORK=off")
	run("", env, "git", "clone", "--quiet", "--shared", "--no-checkout", "--", repository, root)
	run("", env, "git", "-C", root, "checkout", "--quiet", "--detach", pinned)
	if strings.TrimSpace(string(run("", env, "git", "-C", root, "rev-parse", "HEAD"))) != pinned {
		t.Fatal("wrong Console source")
	}
	for _, rel := range []string{"ui/node_modules", "node_modules"} {
		dep, err := filepath.Abs(filepath.Join(repository, rel))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(dep); os.IsNotExist(err) {
			continue
		} else if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(dep, filepath.Join(root, rel)); err != nil {
			t.Fatal(err)
		}
	}
	corpus := filepath.Join(root, "docs/impact-replay-corpus.json")
	if captureFileHash(corpus) != corpusHash {
		t.Fatal("original five-case corpus changed")
	}
	workspace, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "openexec")
	run(workspace, env, "go", "build", "-buildvcs=false", "-o", binary, "./cmd/openexec")
	replay := filepath.Join(t.TempDir(), "impact-replay")
	run(root, env, "go", "build", "-buildvcs=false", "-o", replay, "./cmd/impact-replay")
	capture := privateCaptureParent(t)
	env = append(env, compilerCaptureEnv+"="+capture)
	args := []string{"--openexec", binary, "--repository", root, "--corpus", corpus}
	first := run(root, env, replay, append(append([]string{}, args...), "--scan")...)
	_, child := readPrivateCapture(t, capture)
	if child.Ending != "exit_0" || !child.ResponseDecoded || child.Bound != "45s" || child.FileCount != child.ExtractedFiles {
		t.Fatalf("compiler-backed production extraction unproved: %+v", child)
	}
	before := consoleBudgetGeneration(t, root)
	second := run(root, env, replay, args...) // separate processes, NO --scan
	after := consoleBudgetGeneration(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("persisted generation/capabilities/limitations/provenance changed")
	}
	if !reflect.DeepEqual(consoleBudgetReport(t, first, before.ID), consoleBudgetReport(t, second, before.ID)) {
		t.Fatal("original corpus results changed after process reopen")
	}
	readPrivateCapture(t, capture) // exactly one child: queries never rescanned
	t.Logf("original Console %s corpus=%s; candidate binary=%s script=%s Node=%s TypeScript=%s child=%dns; both processes: 5 cases, 15 matched, 0 missing/unavailable/bound-unavailable/unresolved anchors; same generation and provenance, one scan", pinned, corpusHash, captureFileHash(binary), child.ScriptSHA256, child.NodeVersion, child.CompilerVersion, child.ElapsedNS)
}

func consoleBudgetGeneration(t *testing.T, root string) GraphGeneration {
	t.Helper()
	s, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	identity, err := s.EnsureRepositoryIdentity(t.Context(), root, "")
	if err != nil {
		t.Fatal(err)
	}
	g, err := s.activeGeneration(t.Context(), identity.WorktreeID)
	if err != nil {
		t.Fatal(err)
	}
	if g.Status != "current" {
		t.Fatalf("generation status: %s", g.Status)
	}
	for _, c := range []string{"typescript.definitions", "typescript.imports", "typescript.exports"} {
		if g.Capabilities[c] != "compiler_exact" {
			t.Fatalf("%s=%s", c, g.Capabilities[c])
		}
	}
	return g
}

func consoleBudgetReport(t *testing.T, data []byte, generation string) map[string]any {
	t.Helper()
	var r map[string]any
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	if r["graphVersion"] != generation || r["worktreeStateHash"] == "" {
		t.Fatal("report lost graph/provenance")
	}
	m := r["metrics"].(map[string]any)
	for k, want := range map[string]float64{"caseCount": 5, "expectationCount": 15, "matchedExpectations": 15, "missingExpectations": 0, "unavailableExpectations": 0, "boundUnavailableExpectations": 0, "engineUnresolvedAnchors": 0, "declaredGapExpectations": 0} {
		if m[k] != want {
			t.Fatalf("original corpus %s=%v, want %v\n%s", k, m[k], want, data)
		}
	}
	cases := r["cases"].([]any)
	if len(cases) != 5 {
		t.Fatal("five original cases required")
	}
	for _, entry := range cases {
		c := entry.(map[string]any)
		if c["graphVersion"] != generation || c["freshness"] != "current" {
			t.Fatal("case graph/freshness changed")
		}
		for _, expectation := range c["expectations"].([]any) {
			if expectation.(map[string]any)["status"] != "matched" {
				t.Fatal("original expectation not matched")
			}
		}
		delete(c, "endToEndCliMilliseconds")
	}
	delete(r, "generatedAt")
	delete(m, "medianEndToEndCliMilliseconds")
	delete(m, "slowestEndToEndCliMilliseconds")
	return r
}
