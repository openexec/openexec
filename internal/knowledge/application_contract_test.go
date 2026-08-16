package knowledge

import (
	"strings"
	"testing"
)

func TestApplicationContractsReachHandlersUIEvidenceAndEffects(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module example.com/contracts\n\ngo 1.25\n")
	writeTestFile(t, root, "api/openapi.yaml", `openapi: 3.1.0
paths:
  /api/settings:
    post:
      operationId: updateSettings
      responses:
        '204': { description: Saved }
`)
	writeTestFile(t, root, "internal/server/server.go", `package server
import (
  "net/http"
  "os"
)
type Server struct{}
func (s *Server) routes(mux *http.ServeMux) { mux.HandleFunc("POST /api/settings", s.updateSettings) }
// openexec:effect restart
func (s *Server) updateSettings(http.ResponseWriter, *http.Request) { _ = os.WriteFile("settings.json", nil, 0600) }
`)
	writeTestFile(t, root, "internal/server/model.go", `package server
type Settings struct { Enabled bool `+"`json:\"enabled\"`"+` }
func load(settings Settings) bool { return settings.Enabled }
`)
	writeTestFile(t, root, "ui/package.json", `{"scripts":{"api:generate":"openapi-typescript ../api/openapi.yaml -o src/api/generated.ts"}}`)
	writeTestFile(t, root, "ui/src/api/generated.ts", "export interface paths { '/api/settings': unknown }\n")
	writeTestFile(t, root, "ui/src/TaskBoard.tsx", `import { api } from './api/client'
export function TaskBoard() { return api('/api/settings', { method: 'POST' }) }
`)
	writeTestFile(t, root, "ui/src/api/client.ts", "export function api(path: string, init?: unknown) { return { path, init } }\n")
	writeTestFile(t, root, "ui/e2e/surfaces.json", `{"surfaces":[{"id":"tasks","owner":"task.spec.ts","sources":["ui/src/TaskBoard.tsx"],"states":["ready"]}]}`)
	writeTestFile(t, root, "ui/e2e/task.spec.ts", "export const taskJourney = 'tasks'\n")

	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	scan, err := store.ScanRepository(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := store.EnsureRepositoryIdentity(t.Context(), root, "")
	if err != nil {
		t.Fatal(err)
	}

	operation := requireGraphSymbol(t, store, identity, "updateSettings", "http_operation")
	impact, err := store.ChangedImpactAnalysis(t.Context(), identity, ChangedImpactRequest{Files: []string{"api/openapi.yaml"}, MaxDepth: 2}, DefaultChangedImpactLimits())
	if err != nil {
		t.Fatal(err)
	}
	if operation.Symbol.ID == "" || !impactHasNode(impact.Propagation.DirectCallers, "updateSettings") || !impactHasNode(impact.Propagation.DirectCallers, "TaskBoard") {
		t.Fatalf("OpenAPI operation did not reach handler and UI call site: %#v", impact.Propagation.DirectCallers)
	}
	if !impactHasNode(impact.Propagation.ModuleDependants, "ui/src/api/generated.ts") {
		t.Fatalf("OpenAPI operation did not reach generated client: %#v", impact.Propagation.ModuleDependants)
	}

	handler := requireGraphSymbol(t, store, identity, "updateSettings", "method")
	handlerImpact, err := store.ImpactAnalysis(t.Context(), identity, []string{handler.Symbol.ID}, 2, DefaultChangedImpactLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !impactHasNode(handlerImpact.Result.DirectCallers, "TaskBoard") {
		t.Fatalf("handler did not reach its contract-proven UI consumer: %#v", handlerImpact.Result.DirectCallers)
	}
	if !impactHasNode(handlerImpact.Result.OperationalEffects, "filesystem") {
		t.Fatalf("handler filesystem effect is absent: %#v", handlerImpact.Result.OperationalEffects)
	}
	if !impactHasNode(handlerImpact.Result.OperationalEffects, "restart") {
		t.Fatalf("typed restart effect is absent: %#v", handlerImpact.Result.OperationalEffects)
	}
	if !relatedTestExists(handlerImpact.Result.RelatedTests, "ui/e2e/task.spec.ts") {
		t.Fatalf("component did not reach registered browser journey: %#v", handlerImpact.Result.RelatedTests)
	}

	field := requireGraphSymbol(t, store, identity, "Enabled", "persisted_field")
	fieldImpact, err := store.ImpactAnalysis(t.Context(), identity, []string{field.Symbol.ID}, 1, DefaultChangedImpactLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !impactHasNode(fieldImpact.Result.DirectCallers, "load") || !impactPathUses(fieldImpact.Result.DirectCallers, "uses_persisted_field") {
		t.Fatalf("persisted field did not reach its loader: %#v", fieldImpact.Result.DirectCallers)
	}

	surface := requireGraphSymbol(t, store, identity, "tasks", "ui_surface")
	surfaceImpact, err := store.ImpactAnalysis(t.Context(), identity, []string{surface.Symbol.ID}, 1, DefaultChangedImpactLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !impactHasNode(surfaceImpact.Result.DirectCallers, "taskJourney") || !impactPathUses(surfaceImpact.Result.DirectCallers, "exercises_surface") {
		t.Fatalf("surface contract did not reach owning browser journey: %#v", surfaceImpact.Result.DirectCallers)
	}

	if !containsString(scan.Limitations, "cross-repository generated-package consumers") {
		t.Fatalf("unsupported portfolio wiring was not reported: %#v", scan.Limitations)
	}
}

func TestRemovingSurfaceRegistrySourceRemovesBrowserImpactEdge(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "package.json", `{}`)
	writeTestFile(t, root, "ui/src/TaskBoard.tsx", "export function TaskBoard() { return null }\n")
	writeTestFile(t, root, "ui/e2e/task.spec.ts", "export const taskJourney = 'tasks'\n")
	manifest := "ui/e2e/surfaces.json"
	writeTestFile(t, root, manifest, `{"surfaces":[{"id":"tasks","owner":"task.spec.ts","sources":["ui/src/TaskBoard.tsx"]}]}`)
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.ScanRepository(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	identity, _ := store.EnsureRepositoryIdentity(t.Context(), root, "")
	taskBoard := requireGraphSymbol(t, store, identity, "TaskBoard", "function")
	before, err := store.ImpactAnalysis(t.Context(), identity, []string{taskBoard.Symbol.ID}, 1, DefaultChangedImpactLimits())
	if err != nil || !relatedTestExists(before.Result.RelatedTests, "ui/e2e/task.spec.ts") {
		t.Fatalf("registry fixture setup did not create browser edge: impact=%#v err=%v", before, err)
	}
	writeTestFile(t, root, manifest, `{"surfaces":[{"id":"tasks","owner":"task.spec.ts"}]}`)
	if _, err := store.RefreshRepository(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	taskBoard = requireGraphSymbol(t, store, identity, "TaskBoard", "function")
	after, err := store.ImpactAnalysis(t.Context(), identity, []string{taskBoard.Symbol.ID}, 1, DefaultChangedImpactLimits())
	if err != nil {
		t.Fatal(err)
	}
	if relatedTestExists(after.Result.RelatedTests, "ui/e2e/task.spec.ts") {
		t.Fatalf("browser impact survived removal of its registry edge: %#v", after.Result.RelatedTests)
	}
}

func TestPersistedFieldTypingDoesNotRelabelSameNamedFunctionCall(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module example.com/field-call\n\ngo 1.25\n")
	writeTestFile(t, root, "model.go", `package fieldcall
type Record struct { Name string `+"`json:\"name\"`"+` }
func Name() {}
func call() { Name() }
func load(record Record) string { return record.Name }
`)
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.ScanRepository(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	identity, _ := store.EnsureRepositoryIdentity(t.Context(), root, "")

	function := requireGraphSymbol(t, store, identity, "Name", "function")
	functionImpact, err := store.ImpactAnalysis(t.Context(), identity, []string{function.Symbol.ID}, 1, DefaultChangedImpactLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !impactHasNode(functionImpact.Result.DirectCallers, "call") || !impactPathUses(functionImpact.Result.DirectCallers, "calls") {
		t.Fatalf("same-named function call was lost or relabelled: %#v", functionImpact.Result.DirectCallers)
	}
	if impactPathUses(functionImpact.Result.DirectCallers, "uses_persisted_field") {
		t.Fatalf("function call was rewritten as a persisted-field use: %#v", functionImpact.Result.DirectCallers)
	}

	field := requireGraphSymbol(t, store, identity, "Name", "persisted_field")
	fieldImpact, err := store.ImpactAnalysis(t.Context(), identity, []string{field.Symbol.ID}, 1, DefaultChangedImpactLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !impactHasNode(fieldImpact.Result.DirectCallers, "load") || !impactPathUses(fieldImpact.Result.DirectCallers, "uses_persisted_field") {
		t.Fatalf("actual persisted-field use was not typed at its resolved target: %#v", fieldImpact.Result.DirectCallers)
	}
}

func TestOpenAPIPathMetadataIsLegalAndKeepsGenerationCurrent(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module example.com/openapi-metadata\n\ngo 1.25\n")
	writeTestFile(t, root, "openapi.yaml", `openapi: 3.1.0
paths:
  /api/items:
    summary: Item operations
    parameters:
      - name: projectId
        in: query
        schema: { type: string }
    servers:
      - url: https://example.invalid
    get:
      operationId: listItems
      responses:
        '200': { description: Listed }
`)
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	scan, err := store.ScanRepository(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if scan.Generation.Status != GraphCurrent {
		t.Fatalf("legal OpenAPI path metadata made the generation non-current: %#v", scan.Generation)
	}
	identity, _ := store.EnsureRepositoryIdentity(t.Context(), root, "")
	requireGraphSymbol(t, store, identity, "listItems", "http_operation")
}

func TestIncrementalRefreshDerivesContractsOnlyFromCompleteSnapshot(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module example.com/incremental-contract\n\ngo 1.25\n")
	writeTestFile(t, root, "openapi.yaml", `openapi: 3.1.0
paths:
  /api/items:
    get:
      operationId: listItems
`)
	handlerPath := "server.go"
	writeTestFile(t, root, handlerPath, `package incrementalcontract
import "net/http"
func routes(mux *http.ServeMux) { mux.HandleFunc("GET /api/items", listItems) }
func listItems(http.ResponseWriter, *http.Request) {}
`)
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.ScanRepository(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, handlerPath, `package incrementalcontract
import "net/http"
func routes(mux *http.ServeMux) { mux.HandleFunc("GET /api/items", listItems) }
// changed handler body; the OpenAPI file itself is unchanged.
func listItems(http.ResponseWriter, *http.Request) {}
`)
	refresh, err := store.RefreshRepository(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if containsString(refresh.Limitations, "registered HTTP routes without an OpenAPI operation") {
		t.Fatalf("changed-subset derivation emitted a false missing-operation limitation: %#v", refresh.Limitations)
	}
}

func TestUnsupportedRouteRegistrationIsAnExplicitLimitation(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module example.com/unsupported-route\n\ngo 1.25\n")
	writeTestFile(t, root, "server.go", `package unsupportedroute
import "net/http"
func routes(mux *http.ServeMux) { mux.HandleFunc("/api/items", listItems) }
func listItems(http.ResponseWriter, *http.Request) {}
`)
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	scan, err := store.ScanRepository(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(scan.Limitations, "not method-prefixed ServeMux patterns") {
		t.Fatalf("unsupported route wiring looked fully covered: %#v", scan.Limitations)
	}
}

func TestMalformedOpenAPIIsScopedInsteadOfMakingAllImpactUnavailable(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module example.com/malformed-contract\n\ngo 1.25\n")
	writeTestFile(t, root, "main.go", "package malformedcontract\nfunc Live() {}\n")
	writeTestFile(t, root, "openapi.yaml", "openapi: 3.1.0\npaths: [not-a-path-map\n")
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	scan, err := store.ScanRepository(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if scan.Generation.Status != GraphCurrent || !containsString(scan.Limitations, "parse OpenAPI contract") {
		t.Fatalf("contract parse failure was not a scoped current-generation limitation: status=%s limitations=%#v", scan.Generation.Status, scan.Limitations)
	}
	identity, _ := store.EnsureRepositoryIdentity(t.Context(), root, "")
	live := requireGraphSymbol(t, store, identity, "Live", "function")
	if _, err := store.ImpactAnalysis(t.Context(), identity, []string{live.Symbol.ID}, 1, DefaultChangedImpactLimits()); err != nil {
		t.Fatalf("scoped contract parse failure refused unrelated source impact: %v", err)
	}
}

func requireGraphSymbol(t *testing.T, store *Store, identity RepositoryIdentity, name, kind string) SymbolCandidate {
	t.Helper()
	result, err := store.FindGraphSymbols(t.Context(), identity, name, "", kind, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range result.Result.Candidates {
		if candidate.Symbol.DisplayName == name {
			return candidate
		}
	}
	t.Fatalf("symbol %s (%s) not found: %#v", name, kind, result.Result.Candidates)
	return SymbolCandidate{}
}

func impactHasNode(nodes []ImpactNode, fragment string) bool {
	for _, node := range nodes {
		if strings.Contains(node.Node.DisplayName, fragment) || strings.Contains(node.Node.QualifiedName, fragment) {
			return true
		}
	}
	return false
}

func impactPathUses(nodes []ImpactNode, edgeType string) bool {
	for _, node := range nodes {
		for _, edge := range node.Path {
			if edge.EdgeType == edgeType {
				return true
			}
		}
	}
	return false
}

func relatedTestExists(tests []RelatedTest, path string) bool {
	for _, test := range tests {
		if test.FilePath == path {
			return true
		}
	}
	return false
}

func containsString(values []string, fragment string) bool {
	for _, value := range values {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}
