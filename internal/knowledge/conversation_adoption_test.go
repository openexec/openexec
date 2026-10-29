package knowledge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func conversationAdoptionFixture(t *testing.T) (string, []string) {
	t.Helper()
	root := t.TempDir()
	entries, err := os.ReadDir("testdata/conversation-adoption")
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join("testdata/conversation-adoption", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, entry.Name(), string(data))
		files = append(files, entry.Name())
	}
	compiler, err := filepath.Abs("../../ui/node_modules/typescript")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(compiler, "lib/typescript.js")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(compiler, filepath.Join(root, "node_modules/typescript")); err != nil {
		t.Fatal(err)
	}
	return root, files
}

// This is a characterization of the unchanged native implementation. It must
// retain the missing adoption expectation, rather than invent a caller edge.
func TestConversationAdoptionDependencyEvidence(t *testing.T) {
	root, files := conversationAdoptionFixture(t)
	extracted, configuration, limitations, err := extractTypeScriptWithCompiler(t.Context(), root, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(limitations) != 0 {
		t.Fatalf("compiler diagnostics: %v", limitations)
	}
	t.Logf("compiler configuration: %s", configuration)
	appSource, err := os.ReadFile(filepath.Join(root, "App.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"useConversations": "useConversations.ts", "adoptProjectChat": "adoptProjectChat.ts", "unrelatedSibling": "unrelated.ts"} {
		found := false
		for _, reference := range extracted["App.tsx"].References {
			if reference.TargetName == name && reference.TargetPath == path && reference.EdgeType == "calls" && reference.Resolution == ResolutionCompilerExact && string(appSource[reference.StartByte:reference.EndByte]) == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("compiler did not extract exact call to %s in %s", name, path)
		}
	}
	for _, reference := range extracted["App.tsx"].References {
		if reference.TargetName == "useConversations" || reference.TargetName == "adoptProjectChat" || reference.TargetName == "unrelatedSibling" {
			t.Logf("extracted App reference: %+v", reference)
		}
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if store != nil {
			store.Close()
		}
	})
	scan, err := store.ScanRepository(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if scan.Generation.Capabilities["typescript.definitions"] != "compiler_exact" {
		t.Fatalf("not compiler exact: %+v", scan)
	}
	identity, err := store.EnsureRepositoryIdentity(t.Context(), root, "")
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(name, file string) SymbolCandidate {
		t.Helper()
		result, err := store.ResolveGraphSymbol(t.Context(), identity, name, file, "function", 10)
		if err != nil || result.Result.Candidate == nil {
			t.Fatalf("resolve %s: %+v, %v", name, result, err)
		}
		return *result.Result.Candidate
	}
	app := resolve("App", "App.tsx")
	hook := resolve("useConversations", "useConversations.ts")
	adopt := resolve("adoptProjectChat", "adoptProjectChat.ts")
	unrelated := resolve("unrelatedSibling", "unrelated.ts")
	boot := resolve("Boot", "Boot.tsx")
	relationships, err := store.FindSymbolRelationships(t.Context(), identity, app.Symbol.ID, false, 1, []string{"calls"}, DefaultGraphLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []SymbolCandidate{hook, adopt, unrelated} {
		found := false
		for _, edge := range relationships.Result.Edges {
			if edge.FromNodeID == app.Occurrence.NodeID && edge.ToNodeID == target.Occurrence.NodeID && edge.Type == "calls" {
				if edge.Resolution != ResolutionCompilerExact || edge.SourceFilePath != "App.tsx" || string(appSource[edge.SourceStartByte:edge.SourceEndByte]) != target.Symbol.DisplayName {
					t.Fatalf("edge lost source evidence: %+v", edge)
				}
				t.Logf("persisted call fork: App --%s [%s:%d:%d, %s]--> %s", edge.Type, edge.SourceFilePath, edge.SourceStartByte, edge.SourceEndByte, edge.Resolution, target.Symbol.DisplayName)
				found = true
			}
		}
		if !found {
			t.Fatalf("persisted App call to %s missing", target.Symbol.DisplayName)
		}
	}
	request := ChangedImpactRequest{Files: []string{"useConversations.ts"}, MaxDepth: 2}
	impact, err := store.ChangedImpactAnalysis(t.Context(), identity, request, DefaultChangedImpactLimits())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(impact)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("depth-2 changed impact: %s", encoded)
	if impact.Truncated || impact.Generation.Freshness != FreshnessCurrent || len(impact.UnresolvedFiles) != 0 || len(impact.ChangedSymbols) != 1 || impact.ChangedSymbols[0].ID != hook.Symbol.ID {
		t.Fatalf("incomplete or wrong roots: %+v", impact)
	}
	for _, caller := range append(append([]ImpactNode{}, impact.Propagation.DirectCallers...), impact.Propagation.AffectedCallers...) {
		if caller.Node.DisplayName == "adoptProjectChat" || caller.Node.DisplayName == "unrelatedSibling" {
			t.Fatalf("false sibling caller: %+v", caller)
		}
	}
	appFound := false
	for _, caller := range impact.Propagation.DirectCallers {
		if caller.Node.ID == app.Occurrence.NodeID && len(caller.Path) == 1 && caller.Path[0].EdgeType == "calls" && caller.Path[0].ToNodeID == hook.Occurrence.NodeID {
			appFound = true
		}
	}
	if !appFound {
		t.Fatal("App caller was not reached")
	}
	if len(impact.Propagation.AffectedCallers) != 1 {
		t.Fatalf("expected only genuine depth-2 caller Boot: %+v", impact.Propagation)
	}
	t.Log("MISSING original expectation: affected_callers=adoptProjectChat.adoptProjectChat; unrelated sibling also absent")
	outer := impact.Propagation.AffectedCallers[0]
	if outer.Node.ID != boot.Occurrence.NodeID || len(outer.Path) != 2 || outer.Path[0].FromNodeID != boot.Occurrence.NodeID || outer.Path[0].ToNodeID != app.Occurrence.NodeID || outer.Path[0].EdgeType != "references" || outer.Path[1].ToNodeID != hook.Occurrence.NodeID {
		t.Fatalf("wrong depth-2 typed path: %+v", outer)
	}
	bootEdges, err := store.FindSymbolRelationships(t.Context(), identity, boot.Symbol.ID, false, 1, []string{"references"}, DefaultGraphLimits())
	if err != nil {
		t.Fatal(err)
	}
	typedPathFound := false
	for _, edge := range bootEdges.Result.Edges {
		if edge.ID == outer.Path[0].EdgeID && edge.Resolution == ResolutionCompilerExact && edge.SourceFilePath == "Boot.tsx" {
			typedPathFound = true
		}
	}
	if !typedPathFound {
		t.Fatal("depth-2 path lacks its persisted compiler-exact edge")
	}
	// The other direction reaches App too, but cannot walk out to the hook.
	reverse, err := store.ImpactAnalysis(t.Context(), identity, []string{adopt.Symbol.ID}, 2, DefaultGraphLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(reverse.Result.DirectCallers) != 1 || reverse.Result.DirectCallers[0].Node.ID != app.Occurrence.NodeID {
		t.Fatalf("adoption's real caller missing: %+v", reverse)
	}
	for _, node := range append(reverse.Result.DirectCallers, reverse.Result.AffectedCallers...) {
		if node.Node.ID == hook.Occurrence.NodeID || node.Node.ID == unrelated.Occurrence.NodeID {
			t.Fatalf("reverse query fabricated sibling: %+v", node)
		}
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	store.SetRefreshOnRead(false)
	reopened, err := store.ChangedImpactAnalysis(t.Context(), identity, request, DefaultChangedImpactLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(impact, reopened) {
		t.Fatalf("close/reopen changed evidence or provenance:\nbefore: %+v\nafter: %+v", impact, reopened)
	}
	persisted, err := store.FindSymbolRelationships(t.Context(), identity, app.Symbol.ID, false, 1, []string{"calls"}, DefaultGraphLimits())
	if err != nil || !reflect.DeepEqual(relationships, persisted) {
		t.Fatalf("close/reopen lost edges: %+v %v", persisted, err)
	}
	t.Log("close/reopen: identical impact, provenance, source ranges and typed edges; refresh disabled")

	t.Run("invalid and bounded requests", func(t *testing.T) {
		for _, invalid := range []ChangedImpactRequest{{Files: []string{"../useConversations.ts"}}, {Files: request.Files, MaxDepth: 4}} {
			if _, err := store.ChangedImpactAnalysis(t.Context(), identity, invalid, DefaultChangedImpactLimits()); err == nil {
				t.Fatalf("accepted invalid request: %+v", invalid)
			} else if _, ok := err.(*ImpactRequestError); !ok {
				t.Fatalf("wrong refusal: %T %v", err, err)
			}
		}
		limits := DefaultChangedImpactLimits()
		limits.MaxDepth = 1
		if _, err := store.ChangedImpactAnalysis(t.Context(), identity, request, limits); err == nil {
			t.Fatal("widened depth budget")
		} else if _, ok := err.(*ImpactRequestError); !ok {
			t.Fatalf("wrong bounded refusal: %T %v", err, err)
		}
		limits = DefaultChangedImpactLimits()
		limits.MaxNodes = 1
		bounded, err := store.ChangedImpactAnalysis(t.Context(), identity, request, limits)
		if err != nil || !bounded.Truncated || len(bounded.Propagation.AffectedCallers) != 0 {
			t.Fatalf("node bound concealed: %+v %v", bounded, err)
		}
		missing, err := store.ChangedImpactAnalysis(t.Context(), identity, ChangedImpactRequest{Files: []string{"absent.ts"}, MaxDepth: 2}, DefaultChangedImpactLimits())
		if err != nil || len(missing.UnresolvedFiles) != 1 || missing.Resolution.Status != "unavailable" {
			t.Fatalf("missing anchor concealed: %+v %v", missing, err)
		}
	})
	// A source mutation gives the helper a REAL incoming dependency on the
	// hook. It must be refused while stale, then reached after an explicit scan.
	// This is a control, not a proposed product change (hooks do not belong in
	// adoption request helpers).
	original, err := os.ReadFile(filepath.Join(root, "adoptProjectChat.ts"))
	if err != nil {
		t.Fatal(err)
	}
	mutated := "import { useConversations } from './useConversations'\n" + strings.Replace(string(original), "return fetch(", "useConversations(true); return fetch(", 1)
	writeTestFile(t, root, "adoptProjectChat.ts", mutated)
	if _, err := store.ChangedImpactAnalysis(t.Context(), identity, request, DefaultChangedImpactLimits()); !IsStaleGraph(err) {
		t.Fatalf("stale graph did not refuse: %v", err)
	}
	if _, err := store.ScanRepository(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	withRealCall, err := store.ChangedImpactAnalysis(t.Context(), identity, request, DefaultChangedImpactLimits())
	if err != nil {
		t.Fatal(err)
	}
	called := false
	for _, caller := range withRealCall.Propagation.DirectCallers {
		if caller.Node.DisplayName == "adoptProjectChat" && len(caller.Path) == 1 && caller.Path[0].EdgeType == "calls" {
			called = true
		}
	}
	if !called {
		t.Fatalf("real helper call was lost: %+v", withRealCall)
	}
	writeTestFile(t, root, "adoptProjectChat.ts", string(original))
	if _, err := store.ScanRepository(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	restored, err := store.ChangedImpactAnalysis(t.Context(), identity, request, DefaultChangedImpactLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, caller := range append(restored.Propagation.DirectCallers, restored.Propagation.AffectedCallers...) {
		if caller.Node.DisplayName == "adoptProjectChat" || caller.Node.DisplayName == "unrelatedSibling" {
			t.Fatalf("removed call persisted: %+v", caller)
		}
	}
	t.Log("real-call control: stale refused; inserted helper→hook call reached; removing it removes reach")
}
