package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
	statepkg "github.com/openexec/openexec/pkg/db/state"
)

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
