package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
	statepkg "github.com/openexec/openexec/pkg/db/state"
)

// Runs the same Cobra scan entry point as the external CLI, with no package
// observer. The synthetic child fails; evidence must survive lexical fallback.
func TestKnowledgeGraphScanPrivateCompilerCapture(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX child fixture")
	}
	root, bin, capture := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.Chmod(capture, 0o700); err != nil {
		t.Fatal(err)
	}
	compilerDir := filepath.Join(root, "node_modules", "typescript", "lib")
	if err := os.MkdirAll(compilerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		filepath.Join(root, "a.ts"):                 "export const PRIVATE_SOURCE = 1\n",
		filepath.Join(compilerDir, "typescript.js"): "// synthetic compiler\n",
		filepath.Join(bin, "node"):                  "#!/bin/sh\ncase \"$1\" in -e) printf 22; exit;; --version) echo v22.0.0; exit;; -p) echo 5.8.3; exit;; esac\ncat > \"$TS_TEST_STDIN\"\necho PRIVATE_STDERR >&2\nexit 3\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OPENEXEC_TS_COMPILER_CAPTURE_DIR", capture)
	t.Setenv("TS_TEST_STDIN", filepath.Join(bin, "consumed.json"))
	oldDirectory, oldJSON := graphDirectory, graphJSON
	oldDepth, oldFiles := graphImpactDepth, graphImpactFiles
	oldOut, oldErr := rootCmd.OutOrStdout(), rootCmd.ErrOrStderr()
	t.Cleanup(func() {
		graphDirectory, graphJSON = oldDirectory, oldJSON
		graphImpactDepth, graphImpactFiles = oldDepth, oldFiles
		rootCmd.SetOut(oldOut)
		rootCmd.SetErr(oldErr)
		rootCmd.SetArgs(nil)
	})
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)
	rootCmd.SetArgs([]string{"knowledge", "graph", "scan", "--directory", root, "--json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var scan knowledge.ScanResult
	if err := json.Unmarshal(output.Bytes(), &scan); err != nil {
		t.Fatal(err)
	}
	if scan.Generation.Capabilities["typescript.definitions"] != "static_lexical" || !strings.Contains(strings.Join(scan.Limitations, "\n"), "exit status 3") {
		t.Fatal("CLI hid compiler failure")
	}
	entries, err := os.ReadDir(capture)
	if err != nil || len(entries) != 1 {
		t.Fatalf("ordinary CLI scan did not capture: %v %v", entries, err)
	}
	dir := filepath.Join(capture, entries[0].Name())
	request, err := os.ReadFile(filepath.Join(dir, "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	consumed, err := os.ReadFile(filepath.Join(bin, "consumed.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(request, consumed) {
		t.Fatal("CLI capture is not actual stdin")
	}
	b, err := os.ReadFile(filepath.Join(dir, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Child struct {
			Ending string `json:"ending"`
			Bound  string `json:"extractor_bound"`
			Exit   int    `json:"exit_code"`
		} `json:"child"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatal(err)
	}
	if result.Child.Ending != "ordinary_nonzero_exit" || result.Child.Exit != 3 || result.Child.Bound != "45s" {
		t.Fatalf("bad actual CLI result: %s", b)
	}
	if bytes.Contains(b, []byte("PRIVATE_STDERR")) || bytes.Contains(b, []byte("PRIVATE_SOURCE")) {
		t.Fatal("private body captured")
	}
	output.Reset()
	rootCmd.SetArgs([]string{"knowledge", "graph", "impact", "--directory", root, "--files", "a.ts", "--max-depth", "1", "--json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var impact knowledge.ChangedImpactResponse
	if err := json.Unmarshal(output.Bytes(), &impact); err != nil {
		t.Fatal(err)
	}
	if impact.Provenance.GraphVersion != scan.Generation.ID {
		t.Fatal("CLI fallback graph did not round-trip")
	}
	entries, err = os.ReadDir(capture)
	if err != nil || len(entries) != 1 {
		t.Fatal("reopen unexpectedly rescanned")
	}

}

type recordingRepositoryContextPublisher struct {
	called     bool
	projection knowledge.RepositoryContextProjection
}

func TestKnowledgeGraphImpactFilesJSONUsesBatchResponse(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc Target() {}\nfunc Caller() { Target() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main_test.go"), []byte("package main\nimport \"testing\"\nfunc TestTarget(t *testing.T) { Target() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := knowledge.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanRepository(t.Context(), root); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	previousDirectory, previousJSON := graphDirectory, graphJSON
	previousDepth, previousFiles := graphImpactDepth, graphImpactFiles
	previousOut, previousErr := rootCmd.OutOrStdout(), rootCmd.ErrOrStderr()
	t.Cleanup(func() {
		graphDirectory, graphJSON = previousDirectory, previousJSON
		graphImpactDepth, graphImpactFiles = previousDepth, previousFiles
		rootCmd.SetOut(previousOut)
		rootCmd.SetErr(previousErr)
		rootCmd.SetArgs(nil)
	})
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)
	rootCmd.SetArgs([]string{"knowledge", "graph", "impact", "--directory", root, "--files", "main.go", "--max-depth", "2", "--json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result knowledge.ChangedImpactResponse
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode changed impact output: %v\n%s", err, output.String())
	}
	if len(result.ChangedSymbols) != 2 || len(result.Propagation.DirectCallers) != 2 || result.Provenance.GraphVersion == "" {
		t.Fatalf("changed impact output = %#v", result)
	}

	output.Reset()
	graphJSON = false
	rootCmd.SetArgs([]string{"knowledge", "graph", "impact", "--directory", root, "--files", "main.go", "--depth", "2"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	human := output.String()
	for _, expected := range []string{"test candidate: main_test.go", "unresolved: dynamic dependency injection", "limitation: file anchors resolve every symbol"} {
		if !strings.Contains(human, expected) {
			t.Fatalf("human changed impact omitted %q:\n%s", expected, human)
		}
	}

	var largeSource strings.Builder
	largeSource.WriteString("package main\n")
	for index := 0; index < knowledge.MaxChangedImpactSymbols+1; index++ {
		largeSource.WriteString(fmt.Sprintf("func Symbol%d() {}\n", index))
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(largeSource.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	graphJSON = false
	rootCmd.SetArgs([]string{"knowledge", "graph", "impact", "--directory", root, "--files", "main.go", "--max-depth", "1"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	human = output.String()
	for _, expected := range []string{"unresolved file: main.go: symbol expansion truncated", "result truncated by graph limits"} {
		if !strings.Contains(human, expected) {
			t.Fatalf("human bounded impact omitted %q:\n%s", expected, human)
		}
	}
}

// Deterministic disposable fixture (no provider): the public dependencies
// command must disclose a clamped depth and an edge-limit cut in both JSON and
// text output, and must not flag a graph that fits the limits exactly.
func TestKnowledgeGraphDependenciesDisclosesDepthAndEdgeCuts(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("leaf.ts", "export function leaf(): number { return 1 }\n")
	write("mid.ts", "import { leaf } from './leaf'\nexport const mid = () => leaf()\n")
	write("top.ts", "import { mid } from './mid'\nexport const top = () => mid()\n")
	store, err := knowledge.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanRepository(t.Context(), root); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	previousDirectory, previousJSON := graphDirectory, graphJSON
	previousDepth, previousReverse := graphDepth, graphReverse
	previousOut, previousErr := rootCmd.OutOrStdout(), rootCmd.ErrOrStderr()
	t.Cleanup(func() {
		graphDirectory, graphJSON = previousDirectory, previousJSON
		graphDepth, graphReverse = previousDepth, previousReverse
		rootCmd.SetOut(previousOut)
		rootCmd.SetErr(previousErr)
		rootCmd.SetArgs(nil)
	})
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)
	runJSON := func(depth string) knowledge.QueryEnvelope[knowledge.ModuleDependencyResult] {
		t.Helper()
		output.Reset()
		graphJSON = false
		rootCmd.SetArgs([]string{"knowledge", "graph", "dependencies", "top.ts", "--directory", root, "--depth", depth, "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatal(err)
		}
		var result knowledge.QueryEnvelope[knowledge.ModuleDependencyResult]
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatalf("decode dependencies output: %v\n%s", err, output.String())
		}
		return result
	}
	runText := func(depth string) string {
		t.Helper()
		output.Reset()
		graphJSON = false
		rootCmd.SetArgs([]string{"knowledge", "graph", "dependencies", "top.ts", "--directory", root, "--depth", depth})
		if err := rootCmd.Execute(); err != nil {
			t.Fatal(err)
		}
		return output.String()
	}
	depthNote := "requested depth 99 exceeds the traversal limit 2; traversal bounded to depth 2"

	clamped := runJSON("99")
	if !clamped.Truncated || !strings.Contains(strings.Join(clamped.Limitations, "\n"), depthNote) {
		t.Fatalf("--depth 99 JSON = truncated %t, limitations %q; want depth note and truncated", clamped.Truncated, clamped.Limitations)
	}
	if len(clamped.Result.Edges) != 2 || len(clamped.Result.Nodes) != 2 {
		t.Fatalf("--depth 99 should return the depth-2 reach, got %d edges %d nodes", len(clamped.Result.Edges), len(clamped.Result.Nodes))
	}
	if text := runText("99"); !strings.Contains(text, "result truncated by graph limits") {
		t.Fatalf("--depth 99 text output hid the depth cut:\n%s", text)
	}
	atLimit := runJSON("2")
	if atLimit.Truncated || strings.Contains(strings.Join(atLimit.Limitations, "\n"), "exceeds the traversal limit") {
		t.Fatalf("--depth 2 JSON = truncated %t, limitations %q; want complete", atLimit.Truncated, atLimit.Limitations)
	}
	if text := runText("2"); strings.Contains(text, "result truncated") {
		t.Fatalf("--depth 2 text output reported a cut on a complete graph:\n%s", text)
	}

	// The CLI uses the default edge limit; one file with exactly that many
	// import statements is complete, one more import is a disclosed cut. The
	// imports share 100 packages so the default node limit is not reached.
	maxEdges := knowledge.DefaultGraphLimits().MaxEdges
	importing := func(count int) string {
		var source strings.Builder
		for index := 0; index < count; index++ {
			source.WriteString(fmt.Sprintf("import p%d from 'pkg-%03d'\n", index, index%100))
		}
		source.WriteString("export const top = () => 1\n")
		return source.String()
	}
	write("top.ts", importing(maxEdges))
	exact := runJSON("1")
	if exact.Truncated || len(exact.Result.Edges) != maxEdges || len(exact.Result.Nodes) != 100 {
		t.Fatalf("exactly %d imports = truncated %t, %d edges; want complete", maxEdges, exact.Truncated, len(exact.Result.Edges))
	}
	write("top.ts", importing(maxEdges+1))
	over := runJSON("1")
	if !over.Truncated || len(over.Result.Edges) != maxEdges {
		t.Fatalf("%d imports = truncated %t, %d edges; want truncated at %d", maxEdges+1, over.Truncated, len(over.Result.Edges), maxEdges)
	}
	if text := runText("1"); !strings.Contains(text, "result truncated by graph limits") {
		t.Fatalf("edge-limit cut hidden in text output:\n%s", text)
	}
}

func TestKnowledgeGraphCallsDirections(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc Target() {}\nfunc Caller() { Target() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := knowledge.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanRepository(t.Context(), root); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	identity, err := store.EnsureRepositoryIdentity(t.Context(), root, "")
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, name := range []string{"Target", "Caller"} {
		found, err := store.FindGraphSymbols(t.Context(), identity, name, "", "", 1, 25)
		if err != nil {
			_ = store.Close()
			t.Fatal(err)
		}
		for _, candidate := range found.Result.Candidates {
			if candidate.Symbol.DisplayName == name {
				ids[name] = candidate.Symbol.ID
			}
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if ids["Target"] == "" || ids["Caller"] == "" {
		t.Fatalf("symbol ids = %v", ids)
	}

	previousDirectory, previousJSON := graphDirectory, graphJSON
	previousDirection, previousDepth := graphCallDirection, graphCallDepth
	previousOut, previousErr := rootCmd.OutOrStdout(), rootCmd.ErrOrStderr()
	t.Cleanup(func() {
		graphDirectory, graphJSON = previousDirectory, previousJSON
		graphCallDirection, graphCallDepth = previousDirection, previousDepth
		rootCmd.SetOut(previousOut)
		rootCmd.SetErr(previousErr)
		rootCmd.SetArgs(nil)
	})
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)
	run := func(args ...string) (string, error) {
		output.Reset()
		graphJSON = false
		// Restore the declared flag defaults rather than assigning values here,
		// so the no-flag run exercises the command's own --direction default.
		for _, name := range []string{"direction", "depth"} {
			flag := knowledgeGraphCallsCmd.Flags().Lookup(name)
			if err := flag.Value.Set(flag.DefValue); err != nil {
				t.Fatal(err)
			}
			flag.Changed = false
		}
		rootCmd.SetArgs(append([]string{"knowledge", "graph", "calls", "--directory", root}, args...))
		err := rootCmd.Execute()
		return output.String(), err
	}

	// The default direction is outgoing: Caller's callees.
	callees, err := run(ids["Caller"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(callees, "outgoing calls for main.Caller") || !strings.Contains(callees, "main.Target") {
		t.Fatalf("outgoing calls output:\n%s", callees)
	}
	callers, err := run(ids["Target"], "--direction", "incoming")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(callers, "incoming calls for main.Target") || !strings.Contains(callers, "main.Caller") {
		t.Fatalf("incoming calls output:\n%s", callers)
	}
	deep, err := run(ids["Target"], "--direction", "incoming", "--depth", "9")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(deep, "limitation: requested depth 9 exceeds the traversal limit") || !strings.Contains(deep, "result truncated by graph limits") {
		t.Fatalf("clamped depth output:\n%s", deep)
	}
	if _, err := run(ids["Target"], "--direction", "sideways"); err == nil || !strings.Contains(err.Error(), "direction must be incoming or outgoing") {
		t.Fatalf("invalid direction error = %v", err)
	}
}

func TestKnowledgeGraphValidationCLIRoundTripsLifecycle(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc Target() {}\nfunc Caller() { Target() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main_test.go"), []byte("package main\nimport \"testing\"\nfunc TestTarget(t *testing.T) { Target() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := knowledge.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanRepository(t.Context(), root); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	previousDirectory, previousJSON := graphDirectory, graphJSON
	previousTask, previousRun := graphTaskID, graphRunID
	previousFiles, previousSymbols := validationFiles, validationSymbolIDs
	previousDepth, previousMode := validationDepth, validationMode
	previousPhase, previousStatus := validationPhase, validationStatus
	previousEvidenceStatus, previousIteration := validationEvidenceStatus, validationIteration
	previousItem, previousStep, previousFinalize := validationItemID, validationStepID, validationFinalize
	previousRequired, previousOptional, previousRejected := validationRequiredItems, validationOptionalItems, validationRejectedItems
	previousOut, previousErr := rootCmd.OutOrStdout(), rootCmd.ErrOrStderr()
	t.Cleanup(func() {
		graphDirectory, graphJSON = previousDirectory, previousJSON
		graphTaskID, graphRunID = previousTask, previousRun
		validationFiles, validationSymbolIDs = previousFiles, previousSymbols
		validationDepth, validationMode = previousDepth, previousMode
		validationPhase, validationStatus = previousPhase, previousStatus
		validationEvidenceStatus, validationIteration = previousEvidenceStatus, previousIteration
		validationItemID, validationStepID, validationFinalize = previousItem, previousStep, previousFinalize
		validationRequiredItems, validationOptionalItems, validationRejectedItems = previousRequired, previousOptional, previousRejected
		rootCmd.SetOut(previousOut)
		rootCmd.SetErr(previousErr)
		rootCmd.SetArgs(nil)
	})
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)

	graphJSON = false
	graphRunID = ""
	rootCmd.SetArgs([]string{"knowledge", "graph", "validation", "propose", "--directory", root, "--task", "console-task", "--files", "main.go", "--max-depth", "2", "--json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var proposed statepkg.ValidationPlanRevision
	if err := json.Unmarshal(output.Bytes(), &proposed); err != nil {
		t.Fatalf("decode proposal: %v\n%s", err, output.String())
	}
	if proposed.Status != "proposed" || len(proposed.Items) == 0 || proposed.ImpactQuery.Files[0] != "main.go" {
		t.Fatalf("CLI proposal = %#v", proposed)
	}

	output.Reset()
	graphJSON = false
	rootCmd.SetArgs([]string{"knowledge", "graph", "validation", "accept", proposed.ID, "--directory", root, "--run", "console-run", "--require-item", proposed.Items[0].ID, "--json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var accepted statepkg.ValidationPlanRevision
	if err := json.Unmarshal(output.Bytes(), &accepted); err != nil {
		t.Fatalf("decode accepted plan: %v\n%s", err, output.String())
	}
	var acceptedItem statepkg.ValidationItem
	for _, item := range accepted.Items {
		if item.Disposition == "accepted" {
			acceptedItem = item
		}
	}
	if accepted.Status != "accepted" || accepted.RunID != "console-run" || acceptedItem.ID == "" || acceptedItem.Requirement != "required" {
		t.Fatalf("CLI accepted plan = %#v", accepted)
	}

	output.Reset()
	graphJSON = false
	rootCmd.SetArgs([]string{"knowledge", "graph", "validation", "run", "console-run", "--directory", root, "--mode", "workspace-write"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	rootCmd.SetArgs([]string{"knowledge", "graph", "validation", "step", "console-run", "console-step", "--directory", root, "--phase", "verify", "--iteration", "1", "--status", "completed"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	rootCmd.SetArgs([]string{"knowledge", "graph", "validation", "evidence", accepted.ID, "--directory", root, "--item", acceptedItem.ID, "--run", "console-run", "--step", "console-step", "--status", "passed"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	output.Reset()
	graphJSON = false
	validationFinalize = false
	rootCmd.SetArgs([]string{"knowledge", "graph", "validation", "completion", accepted.ID, "--directory", root, "--finalize", "--json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var report statepkg.CompletionReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode completion report: %v\n%s", err, output.String())
	}
	if report.ID == "" || !report.CanComplete || len(report.Verified) == 0 {
		t.Fatalf("CLI completion report = %#v", report)
	}

	output.Reset()
	graphJSON = false
	rootCmd.SetArgs([]string{"knowledge", "graph", "validation", "show", accepted.ID, "--directory", root, "--json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var reloaded statepkg.ValidationPlanRevision
	if err := json.Unmarshal(output.Bytes(), &reloaded); err != nil || reloaded.ID != accepted.ID || reloaded.ImpactSummary.ChangedSymbolIDs[0] == "" {
		t.Fatalf("CLI plan reload = %#v, err=%v\n%s", reloaded, err, output.String())
	}
}

func (p *recordingRepositoryContextPublisher) Publish(_ context.Context, _, _, _ string, projection knowledge.RepositoryContextProjection) error {
	p.called = true
	p.projection = projection
	return nil
}

func TestPublishRefreshesGraphBeforeSendingRepositoryContext(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := knowledge.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	identity, err := store.EnsureRepositoryIdentity(context.Background(), root, "")
	if err != nil {
		t.Fatal(err)
	}
	publisher := &recordingRepositoryContextPublisher{}
	projection, err := refreshAndPublishRepositoryContext(context.Background(), store, identity, "https://console.example", "project-1", "token", nil, "", "", "", publisher)
	if err != nil {
		t.Fatal(err)
	}
	if !publisher.called {
		t.Fatal("repository context was not published")
	}
	if projection.GraphVersion == "" || publisher.projection.GraphVersion != projection.GraphVersion {
		t.Fatalf("published graph version = %q, projection = %q", publisher.projection.GraphVersion, projection.GraphVersion)
	}
	state, err := store.CurrentRepositoryState(context.Background(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if state.GraphVersion != projection.GraphVersion || state.Freshness != knowledge.FreshnessCurrent {
		t.Fatalf("persisted graph state = %#v, projection = %#v", state, projection)
	}
}

func TestPublishIncludesTheNamedFrozenCompletionReport(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	graph, err := knowledge.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()
	identity, err := graph.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := graph.RefreshRepository(ctx, root); err != nil {
		t.Fatal(err)
	}
	current, err := graph.CurrentRepositoryState(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := openValidationState(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := canonical.CreateRun(ctx, "run-publish", "", "", root, "workspace-write"); err != nil {
		t.Fatal(err)
	}
	if err := canonical.AddRunStep(ctx, "step-publish", "run-publish", "trace", "verify", 1, "completed"); err != nil {
		t.Fatal(err)
	}
	if err := canonical.RecordArtifact(ctx, "artifact-publish", "test_log", ".openexec/artifacts/publish.log", 10); err != nil {
		t.Fatal(err)
	}
	plan, err := canonical.CreateValidationPlanRevision(ctx, statepkg.ValidationPlanRevision{
		TaskID: "task-publish", RunID: "run-publish", GenerationID: current.GraphVersion,
		WorktreeStateHash: current.WorktreeStateHash, Status: "accepted",
		Items: []statepkg.ValidationItem{{Source: "user", Disposition: "accepted", Requirement: "blocking", Criterion: "The owner journey passes", Scope: "owner_journey"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := canonical.LinkValidationEvidence(ctx, statepkg.ValidationEvidenceLink{
		ValidationItemID: plan.Items[0].ID, RunID: "run-publish", RunStepID: "step-publish",
		ArtifactHash: "artifact-publish", WorktreeStateHash: current.WorktreeStateHash, Status: "passed",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := canonical.FinalizeEvidenceCoverage(ctx, plan.ID); err != nil {
		t.Fatal(err)
	}
	if err := canonical.Close(); err != nil {
		t.Fatal(err)
	}

	publisher := &recordingRepositoryContextPublisher{}
	projection, err := refreshAndPublishRepositoryContext(ctx, graph, identity, "https://console.example", "project-1", "token", nil, "task-publish", "run-publish", plan.ID, publisher)
	if err != nil {
		t.Fatal(err)
	}
	if !projection.ValidationSummary.CanComplete || len(projection.ValidationSummary.Verified) != 1 ||
		projection.ValidationSummary.Verified[0] != "The owner journey passes" || len(projection.ValidationSummary.NotVerified) != 0 {
		t.Fatalf("published validation summary = %#v", projection.ValidationSummary)
	}

	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() { println(\"changed\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stalePublisher := &recordingRepositoryContextPublisher{}
	if _, err := refreshAndPublishRepositoryContext(ctx, graph, identity, "https://console.example", "project-1", "token", nil, "task-publish", "run-publish", plan.ID, stalePublisher); err == nil ||
		!strings.Contains(err.Error(), "belongs to repository state") {
		t.Fatalf("stale completion report publish error = %v", err)
	}
	if stalePublisher.called {
		t.Fatal("stale completion report was published against the refreshed repository")
	}
}

func TestPublishNamesAnUnfinalizedValidationPlan(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	graph, err := knowledge.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()
	identity, err := graph.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := graph.RefreshRepository(ctx, root); err != nil {
		t.Fatal(err)
	}
	current, err := graph.CurrentRepositoryState(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := openValidationState(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := canonical.CreateValidationPlanRevision(ctx, statepkg.ValidationPlanRevision{
		TaskID: "task-unfinalized", GenerationID: current.GraphVersion,
		WorktreeStateHash: current.WorktreeStateHash, Status: "accepted",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := canonical.Close(); err != nil {
		t.Fatal(err)
	}

	publisher := &recordingRepositoryContextPublisher{}
	if _, err := refreshAndPublishRepositoryContext(ctx, graph, identity, "https://console.example", "project-1", "token", nil, "", "", plan.ID, publisher); err == nil ||
		!strings.Contains(err.Error(), "has no frozen completion report") {
		t.Fatalf("unfinalized completion report error = %v", err)
	}
	if publisher.called {
		t.Fatal("unfinalized completion report was published")
	}
}
