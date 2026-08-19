package knowledge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type pythonTestReader struct{ root string }

func (reader pythonTestReader) ReadRange(_ context.Context, request SourceReadRequest) (SourceReadResult, error) {
	data, err := os.ReadFile(filepath.Join(reader.root, filepath.FromSlash(request.FilePath)))
	if err != nil {
		return SourceReadResult{}, err
	}
	if request.StartByte < 0 || request.EndByte < request.StartByte || request.EndByte > len(data) {
		return SourceReadResult{}, fmt.Errorf("invalid source range")
	}
	rangeData := data[request.StartByte:request.EndByte]
	if hashBytes(data) != request.FileHash || hashBytes(rangeData) != request.RangeHash {
		return SourceReadResult{}, fmt.Errorf("source hash mismatch")
	}
	return SourceReadResult{FilePath: request.FilePath, StartByte: request.StartByte, EndByte: request.EndByte, Content: string(rangeData), FileHash: request.FileHash, RangeHash: request.RangeHash}, nil
}

func TestSvelteAndPythonExtractionProvidesRoutesAndBackendStructure(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, ".gitignore", ".openexec/\n")
	writeTestFile(t, root, "code/frontend/src/routes/(protected)/tenders/[id]/+page.svelte", `<script lang="ts">
import { loadTender } from '$lib/api'
export const openBid = () => loadTender()
export const query = () => true
</script>
<button on:click={openBid}>Open</button>`)
	writeTestFile(t, root, "code/backend/app/services/tender.py", `def persist_offer(db, offer):
    db.add(offer)
    db.commit()
    return offer
`)
	writeTestFile(t, root, "code/backend/app/services/multiline.py", `def create_offer(
    provider_id: int,
    tender_id: int,
):
    db.query()
    saved = persist_offer(provider_id, tender_id)
    return saved

def next_function():
    return None
`)
	writeTestFile(t, root, "code/backend/app/api/offers.py", `from app.services.tender import persist_offer

def create_offer(db, offer):
    return persist_offer(db, offer)
`)
	writeTestFile(t, root, "_archive/old.py", "def retired():\n    return True\n")

	manifest, err := BuildScanManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range manifest.Inputs {
		if input.FilePath == "_archive/old.py" {
			t.Fatal("_archive source was included in active graph manifest")
		}
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	result, err := store.ScanRepository(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation.Status != GraphCurrent || result.Generation.Capabilities["svelte.routes"] != "configuration_derived" || result.Generation.Capabilities["python.definitions"] != "static_lexical" {
		t.Fatalf("polyglot generation = %#v", result.Generation)
	}
	identity, err := store.EnsureRepositoryIdentity(t.Context(), root, "")
	if err != nil {
		t.Fatal(err)
	}
	route, err := store.ResolveGraphSymbol(t.Context(), identity, "/tenders/[id] [+page.svelte]", "", "route", 10)
	if err != nil || route.Result.Candidate == nil {
		t.Fatalf("Svelte route resolution = %#v, %v", route, err)
	}
	handler, err := store.ResolveGraphSymbol(t.Context(), identity, "create_offer", "code/backend/app/api/offers.py", "function", 10)
	if err != nil || handler.Result.Candidate == nil || handler.Result.Candidate.Symbol.Language != "python" {
		t.Fatalf("Python handler resolution = %#v, %v", handler, err)
	}
	dependencies, err := store.FindModuleDependencies(t.Context(), identity, "code/backend/app/api/offers.py", false, 1, DefaultGraphLimits())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, node := range dependencies.Result.Nodes {
		if node.QualifiedName == "code/backend/app/services/tender.py" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Python module import was not resolved: %#v", dependencies.Result)
	}
	multiline, err := store.ResolveGraphSymbol(t.Context(), identity, "create_offer", "code/backend/app/services/multiline.py", "function", 10)
	if err != nil || multiline.Result.Candidate == nil {
		t.Fatalf("multiline Python function was not resolved: %#v, %v", multiline, err)
	}
	reader := pythonTestReader{root: root}
	source, err := store.ReadGraphSymbol(t.Context(), identity, multiline.Result.Candidate.Symbol.ID, reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source.Result.Source.Content, "return saved") || strings.Contains(source.Result.Source.Content, "next_function") {
		t.Fatalf("multiline Python source range = %q", source.Result.Source.Content)
	}
	relationships, err := store.FindSymbolRelationships(t.Context(), identity, multiline.Result.Candidate.Symbol.ID, false, 1, []string{"calls"}, DefaultGraphLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range relationships.Result.Nodes {
		if node.Language == "svelte" {
			t.Fatalf("Python heuristic call crossed language boundary: %#v", node)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "_archive", "old.py")); err != nil {
		t.Fatal(err)
	}
}

func TestPythonIncrementalRefreshPreservesTypeScriptCapability(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "frontend/main.ts", "export const main = 1\n")
	writeTestFile(t, root, "backend/main.py", "def main():\n    return 1\n")
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	scan, err := store.ScanRepository(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	initialCapability := scan.Generation.Capabilities["typescript.definitions"]
	capabilityStrength := map[string]int{"static_lexical": 1, "compiler_exact": 2}
	if capabilityStrength[initialCapability] == 0 {
		t.Fatalf("unexpected initial TypeScript capability: %#v", scan.Generation.Capabilities)
	}
	writeTestFile(t, root, "backend/main.py", "def main():\n    return 2\n")
	refresh, err := store.RefreshRepository(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	refreshedCapability := refresh.Generation.Capabilities["typescript.definitions"]
	if capabilityStrength[refreshedCapability] < capabilityStrength[initialCapability] {
		t.Fatalf("Python-only refresh weakened TypeScript capability from %q to %q: %#v", initialCapability, refreshedCapability, refresh.Generation.Capabilities)
	}
}
