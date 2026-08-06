package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// fakeSymbolIndex stands in for the knowledge graph so these tests cover the
// tool contract — arguments, mode authorization, and the disclosure of
// freshness/truncation/limitations — without building a real repository graph.
type fakeSymbolIndex struct {
	page      SymbolPointerPage
	read      SymbolSourceRead
	relations SymbolRelationsRead
	err       error

	gotQuery     string
	gotSymbolID  string
	gotDirection string
	gotDepth     int
}

func (f *fakeSymbolIndex) FindSymbols(_ context.Context, _, query, _, _ string, _, _ int) (SymbolPointerPage, error) {
	f.gotQuery = query
	return f.page, f.err
}

func (f *fakeSymbolIndex) ReadSymbol(_ context.Context, _, symbolID string) (SymbolSourceRead, error) {
	f.gotSymbolID = symbolID
	return f.read, f.err
}

func (f *fakeSymbolIndex) SymbolRelations(_ context.Context, _, symbolID, direction string, depth int) (SymbolRelationsRead, error) {
	f.gotSymbolID, f.gotDirection, f.gotDepth = symbolID, direction, depth
	return f.relations, f.err
}

func newSymbolTestServer(t *testing.T, index SymbolIndex) (*Server, *bytes.Buffer) {
	t.Helper()
	projDir := t.TempDir()
	srv, out := newBacklogTestServer(t, projDir)
	if index != nil {
		srv.SetSymbolIndex(index)
	}
	return srv, out
}

// advertisedTools returns the names in the tools/list response. It must read
// the "tools" array specifically: the response also carries "toolset_tools"
// (toolset membership for client-side filtering), which names the symbol tools
// whether or not this server has an index to answer them.
func advertisedTools(t *testing.T, srv *Server, out *bytes.Buffer) map[string]bool {
	t.Helper()
	out.Reset()
	srv.handleToolsList(Request{JSONRPC: "2.0", ID: []byte(`1`)})

	var resp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		t.Fatalf("decode tools/list: %v\nraw: %s", err, out.String())
	}
	names := map[string]bool{}
	for _, tool := range resp.Result.Tools {
		names[tool.Name] = true
	}
	return names
}

// TestSymbolToolsAdvertisedOnlyWithIndex: a workspace with no graph must not
// advertise tools that cannot answer.
func TestSymbolToolsAdvertisedOnlyWithIndex(t *testing.T) {
	for _, tc := range []struct {
		name  string
		index SymbolIndex
		want  bool
	}{
		{"no index", nil, false},
		{"with index", &fakeSymbolIndex{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, out := newSymbolTestServer(t, tc.index)
			names := advertisedTools(t, srv, out)
			for _, tool := range []string{"symbol_find", "symbol_read", "symbol_relations"} {
				if names[tool] != tc.want {
					t.Errorf("%s advertised = %v, want %v", tool, names[tool], tc.want)
				}
			}
		})
	}
}

// TestSymbolFindReturnsPointers: the whole point of the tool — a pointer the
// agent can act on without reading the file.
func TestSymbolFindReturnsPointers(t *testing.T) {
	index := &fakeSymbolIndex{page: SymbolPointerPage{
		Pointers: []SymbolPointer{{
			SymbolID: "sym_1", Name: "ResolveGraphSymbol", QualifiedName: "knowledge.ResolveGraphSymbol",
			Kind: "function", File: "internal/knowledge/graph_store.go", StartLine: 412, EndLine: 455,
			Signature: "func ResolveGraphSymbol(ctx context.Context) error", Resolution: "compiler_exact",
		}},
		Total: 1, Page: 1, PageSize: 25, TotalPages: 1,
		GraphVersion: "graph-abc", Freshness: "current",
	}}
	srv, out := newSymbolTestServer(t, index)

	result := callTool(t, srv, out, "symbol_find", map[string]interface{}{"query": "ResolveGraphSymbol"})
	if isToolError(result) {
		t.Fatalf("symbol_find returned error: %s", resultText(result))
	}
	if index.gotQuery != "ResolveGraphSymbol" {
		t.Errorf("query not forwarded: got %q", index.gotQuery)
	}

	text := resultText(result)
	for _, want := range []string{
		"internal/knowledge/graph_store.go:412-455", // the pointer
		"sym_1",             // usable by symbol_read
		"compiler_exact",    // resolution tier disclosed
		"freshness: current", // provenance disclosed
	} {
		if !strings.Contains(text, want) {
			t.Errorf("symbol_find text missing %q\ngot: %s", want, text)
		}
	}
}

// TestSymbolFindDisclosesTruncationAndLimitations: a bounded answer must
// never present itself as complete.
func TestSymbolFindDisclosesTruncationAndLimitations(t *testing.T) {
	index := &fakeSymbolIndex{page: SymbolPointerPage{
		Total: 300, Page: 1, PageSize: 25, TotalPages: 12,
		Truncated:   true,
		Freshness:   "partial",
		Limitations: []string{"Python uses static lexical extraction"},
	}}
	srv, out := newSymbolTestServer(t, index)

	text := resultText(callTool(t, srv, out, "symbol_find", map[string]interface{}{"query": "handler"}))
	if !strings.Contains(text, "Truncated") {
		t.Errorf("truncation not disclosed\ngot: %s", text)
	}
	if !strings.Contains(text, "Python uses static lexical extraction") {
		t.Errorf("limitations not disclosed\ngot: %s", text)
	}
	if !strings.Contains(text, "freshness: partial") {
		t.Errorf("freshness not disclosed\ngot: %s", text)
	}
}

// TestSymbolFindCapsLimitations: graph limitations are generation-wide, so a
// repository with an unbuildable vendored template produces dozens that have
// nothing to do with the lookup. Printing them all would cost more context
// than the file read these tools replace — cap them, but keep the count so a
// capped answer is still honest.
func TestSymbolFindCapsLimitations(t *testing.T) {
	many := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		many = append(many, fmt.Sprintf("diagnostic number %d", i))
	}
	srv, out := newSymbolTestServer(t, &fakeSymbolIndex{page: SymbolPointerPage{
		Pointers:    []SymbolPointer{{SymbolID: "sym_1", Name: "X", File: "a.go", StartLine: 1, EndLine: 2}},
		Total:       1,
		Freshness:   "current",
		Limitations: many,
	}})

	text := resultText(callTool(t, srv, out, "symbol_find", map[string]interface{}{"query": "X"}))
	if strings.Contains(text, "diagnostic number 20") {
		t.Errorf("limitations were not capped — the whole list was rendered\ngot: %s", text)
	}
	if !strings.Contains(text, "diagnostic number 0") {
		t.Errorf("first limitations must still be shown\ngot: %s", text)
	}
	if !strings.Contains(text, "25 more limitation") {
		t.Errorf("omitted limitations must be counted\ngot: %s", text)
	}
}

// TestSymbolRelationsClaimsOnlyWhatTheGraphKnows: the graph stores a byte
// offset per edge, not a line, and the query layer does not report the depth
// actually consumed. Rendering either would send an agent to a fabricated
// location or overstate what the result proves.
func TestSymbolRelationsClaimsOnlyWhatTheGraphKnows(t *testing.T) {
	srv, out := newSymbolTestServer(t, &fakeSymbolIndex{relations: SymbolRelationsRead{
		Root: "pkg.Target", Freshness: "current", DepthRequested: 3,
		Relations: []SymbolRelation{{
			EdgeType: "calls", QualifiedName: "pkg.Caller", File: "internal/server/x.go",
		}},
	}})

	text := resultText(callTool(t, srv, out, "symbol_relations", map[string]interface{}{"symbol_id": "sym_1"}))
	if strings.Contains(text, "x.go:") {
		t.Errorf("rendered a line number the graph does not hold\ngot: %s", text)
	}
	if !strings.Contains(text, "internal/server/x.go") {
		t.Errorf("file path missing\ngot: %s", text)
	}
	if strings.Contains(text, "depth reached") {
		t.Errorf("requested depth must not be reported as reached\ngot: %s", text)
	}
	if !strings.Contains(text, "searched to depth 3") {
		t.Errorf("search bound not disclosed\ngot: %s", text)
	}
}

// TestSymbolFindRequiresQuery and friends: missing arguments produce a tool
// error, never a silent empty answer.
func TestSymbolToolsRejectMissingArguments(t *testing.T) {
	for _, tc := range []struct{ tool, want string }{
		{"symbol_find", "requires a query"},
		{"symbol_read", "requires a symbol_id"},
		{"symbol_relations", "requires a symbol_id"},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			srv, out := newSymbolTestServer(t, &fakeSymbolIndex{})
			result := callTool(t, srv, out, tc.tool, map[string]interface{}{})
			if !isToolError(result) {
				t.Fatalf("%s accepted empty arguments", tc.tool)
			}
			if !strings.Contains(resultText(result), tc.want) {
				t.Errorf("%s error missing %q: %s", tc.tool, tc.want, resultText(result))
			}
		})
	}
}

// TestSymbolReadReturnsBoundedSource: symbol_read returns the definition, not
// the whole file, with its provenance.
func TestSymbolReadReturnsBoundedSource(t *testing.T) {
	index := &fakeSymbolIndex{read: SymbolSourceRead{
		Pointer: SymbolPointer{
			SymbolID: "sym_1", Name: "Execute", QualifiedName: "tools.Execute", Kind: "method",
			File: "internal/tools/symbol_reader.go", StartLine: 60, EndLine: 119,
		},
		Content:      "func (t *SymbolReaderTool) Execute() {}",
		GraphVersion: "graph-abc", Freshness: "current",
	}}
	srv, out := newSymbolTestServer(t, index)

	result := callTool(t, srv, out, "symbol_read", map[string]interface{}{"symbol_id": "sym_1"})
	if isToolError(result) {
		t.Fatalf("symbol_read returned error: %s", resultText(result))
	}
	if index.gotSymbolID != "sym_1" {
		t.Errorf("symbol_id not forwarded: got %q", index.gotSymbolID)
	}
	text := resultText(result)
	if !strings.Contains(text, "func (t *SymbolReaderTool) Execute() {}") {
		t.Errorf("source missing\ngot: %s", text)
	}
	if !strings.Contains(text, "internal/tools/symbol_reader.go:60-119") {
		t.Errorf("location missing\ngot: %s", text)
	}
}

// TestSymbolRelationsDirection: incoming is the default (the blast-radius
// question), and an explicit invalid direction is refused.
func TestSymbolRelationsDirection(t *testing.T) {
	index := &fakeSymbolIndex{relations: SymbolRelationsRead{
		Root: "knowledge.ResolveGraphSymbol", DepthRequested: 2, Freshness: "current",
		Relations: []SymbolRelation{{
			Direction: "incoming", EdgeType: "calls", QualifiedName: "tools.Execute",
			File: "internal/tools/symbol_reader.go", Resolution: "compiler_exact",
		}},
	}}
	srv, out := newSymbolTestServer(t, index)

	text := resultText(callTool(t, srv, out, "symbol_relations", map[string]interface{}{"symbol_id": "sym_1"}))
	if index.gotDirection != "incoming" {
		t.Errorf("default direction = %q, want incoming", index.gotDirection)
	}
	if !strings.Contains(text, "tools.Execute") || !strings.Contains(text, "calls") {
		t.Errorf("relation not rendered\ngot: %s", text)
	}

	result := callTool(t, srv, out, "symbol_relations", map[string]interface{}{
		"symbol_id": "sym_1", "direction": "sideways",
	})
	if !isToolError(result) {
		t.Fatal("invalid direction accepted")
	}
}

// TestSymbolRelationsEmptyDoesNotImplyDeadCode: absence of callers in a
// bounded, heuristic graph is not proof the symbol is unused.
func TestSymbolRelationsEmptyDoesNotImplyDeadCode(t *testing.T) {
	srv, out := newSymbolTestServer(t, &fakeSymbolIndex{relations: SymbolRelationsRead{
		Root: "pkg.Orphan", Freshness: "current",
		Limitations: []string{"dynamic dispatch is not resolved"},
	}})

	text := resultText(callTool(t, srv, out, "symbol_relations", map[string]interface{}{"symbol_id": "sym_1"}))
	if !strings.Contains(text, "dynamically") {
		t.Errorf("empty result must warn against concluding dead code\ngot: %s", text)
	}
	if !strings.Contains(text, "dynamic dispatch is not resolved") {
		t.Errorf("limitations not disclosed\ngot: %s", text)
	}
}

// TestSymbolIndexErrorSurfaces: a stale-graph refusal from the knowledge layer
// reaches the agent instead of being swallowed into an empty result.
func TestSymbolIndexErrorSurfaces(t *testing.T) {
	srv, out := newSymbolTestServer(t, &fakeSymbolIndex{
		err: errors.New("graph is stale; re-resolve before trusting this answer"),
	})

	result := callTool(t, srv, out, "symbol_find", map[string]interface{}{"query": "anything"})
	if !isToolError(result) {
		t.Fatal("index error did not surface as a tool error")
	}
	if !strings.Contains(resultText(result), "stale") {
		t.Errorf("refusal text lost: %s", resultText(result))
	}
}

// TestSymbolToolsDeclareThemselvesReadOnly guards a fix that is invisible
// until an agent actually runs: Claude Code's plan mode refuses to execute any
// MCP tool it cannot tell is safe. Without readOnlyHint the agent lists the
// symbol tools, declines to call them, and falls back to grep — verified live
// against Claude Code 2.1.220, where adding the annotation was the difference
// between refusal and a correct answer.
func TestSymbolToolsDeclareThemselvesReadOnly(t *testing.T) {
	for _, def := range []map[string]interface{}{
		SymbolFindToolDef(), SymbolReadToolDef(), SymbolRelationsToolDef(),
	} {
		name, _ := def["name"].(string)
		annotations, ok := def["annotations"].(map[string]interface{})
		if !ok {
			t.Errorf("%s has no annotations; plan-mode agents will refuse to call it", name)
			continue
		}
		if readOnly, _ := annotations["readOnlyHint"].(bool); !readOnly {
			t.Errorf("%s does not declare readOnlyHint", name)
		}
		if destructive, _ := annotations["destructiveHint"].(bool); destructive {
			t.Errorf("%s declares itself destructive", name)
		}
		if title, _ := annotations["title"].(string); title == "" {
			t.Errorf("%s has no annotation title", name)
		}
	}
}

// TestSymbolsOnlyProfileAdvertisesNothingElse: the profile Agent Console runs
// must not hand an agent the backlog, skills or patch surface.
func TestSymbolsOnlyProfileAdvertisesNothingElse(t *testing.T) {
	srv, out := newSymbolTestServer(t, &fakeSymbolIndex{})
	srv.SetSymbolsOnly(true)

	names := advertisedTools(t, srv, out)
	for _, want := range []string{"symbol_find", "symbol_read", "symbol_relations"} {
		if !names[want] {
			t.Errorf("%s missing from symbols-only profile", want)
		}
	}
	if len(names) != 3 {
		t.Errorf("symbols-only profile advertised %d tools, want exactly 3: %v", len(names), names)
	}
}

// TestSymbolsOnlyProfileDeniesAtTheBroker is the load-bearing assertion:
// advertisement is not authorization. A client that already knows a tool name
// can call it without ever listing, so the deny must live in Authorize.
func TestSymbolsOnlyProfileDeniesAtTheBroker(t *testing.T) {
	srv, _ := newSymbolTestServer(t, &fakeSymbolIndex{})
	srv.SetSymbolsOnly(true)

	for _, tool := range []string{
		"backlog_claim_story", "backlog_complete_task", "backlog_add_task",
		"skill_propose", "git_apply_patch", "write_file", "run_shell_command",
		"memory_read", "read_file", "openexec_signal", "approval_decide",
	} {
		if allowed, _ := srv.broker.Authorize(tool, "{}"); allowed {
			t.Errorf("symbols-only profile authorized %s", tool)
		}
	}
	for _, tool := range []string{"symbol_find", "symbol_read", "symbol_relations"} {
		if allowed, reason := srv.broker.Authorize(tool, "{}"); !allowed {
			t.Errorf("symbols-only profile denied %s: %s", tool, reason)
		}
	}
}

// TestSymbolToolsAllowedInReadOnlyMode: the tools must work in suggest mode —
// locating code is exactly what a read-only session needs.
func TestSymbolToolsAllowedInReadOnlyMode(t *testing.T) {
	srv, _ := newSymbolTestServer(t, &fakeSymbolIndex{})
	if srv.broker.Mode() != ModeSuggest {
		t.Fatalf("expected suggest mode, got %s", srv.broker.Mode())
	}
	for _, tool := range []string{"symbol_find", "symbol_read", "symbol_relations"} {
		if allowed, reason := srv.broker.Authorize(tool, "{}"); !allowed {
			t.Errorf("%s denied in read-only mode: %s", tool, reason)
		}
	}
}
