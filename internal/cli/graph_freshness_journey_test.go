package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/mcp"
)

// journeyMCPCall runs one JSON-RPC tools/call through a real MCP server wired
// exactly as mcp-serve wires it (graphSymbolIndex over a real knowledge store)
// and returns the result object.
func journeyMCPCall(t *testing.T, root string, store *knowledge.Store, tool string, args map[string]any) map[string]any {
	t.Helper()
	params, err := json.Marshal(map[string]any{"name": tool, "arguments": args})
	if err != nil {
		t.Fatal(err)
	}
	request, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": json.RawMessage(params)})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	srv, err := mcp.NewServerWithConfig(bytes.NewReader(append(request, '\n')), &out, mcp.ServerConfig{WorkDir: root})
	if err != nil {
		t.Fatal(err)
	}
	srv.SetSymbolIndex(newGraphSymbolIndex(store))
	if err := srv.Serve(); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Result map[string]any `json:"result"`
		Error  any            `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("decode %s response %q: %v", tool, out.String(), err)
	}
	if response.Error != nil {
		t.Fatalf("%s protocol error: %v", tool, response.Error)
	}
	return response.Result
}

func journeyText(result map[string]any) string {
	content, _ := result["content"].([]any)
	var parts []string
	for _, item := range content {
		if entry, ok := item.(map[string]any); ok {
			text, _ := entry["text"].(string)
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func journeyOpen(t *testing.T, root string) *knowledge.Store {
	t.Helper()
	store, err := knowledge.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// TestGraphFreshnessJourneyThroughMCP walks the V2.1 owner path end to end on
// a git worktree, through the real MCP adapter and real store, with each step
// opening its own store as a separate mcp-serve/CLI process would:
// scan → resolve → edit → source (refresh re-resolves) → edit under the
// symbols profile → stale refusal → reopen and check the persisted generation.
func TestGraphFreshnessJourneyThroughMCP(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	ctx := context.Background()
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git init: %v %s", err, out)
	}
	write := func(contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("package main\n\nfunc Target() int { return 1 }\n")

	// Scan (the `openexec knowledge graph scan` process).
	scanner := journeyOpen(t, root)
	scan, err := scanner.ScanRepository(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	scanner.Close()

	// Resolve through MCP (default profile: refresh on read).
	server := journeyOpen(t, root)
	found := journeyMCPCall(t, root, server, "symbol_find", map[string]any{"query": "Target"})
	page, _ := found["result"].(map[string]any)
	pointers, _ := page["pointers"].([]any)
	if len(pointers) != 1 || page["graph_version"] != scan.Generation.ID {
		t.Fatalf("symbol_find did not resolve Target on the scanned graph: %s", journeyText(found))
	}
	symbolID, _ := pointers[0].(map[string]any)["symbol_id"].(string)

	// Edit (line shift + body change), then read source: the read must
	// refresh and re-resolve, answering from a new generation.
	write("package main\n\n// moved down\nfunc Target() int { return 2 }\n")
	read := journeyMCPCall(t, root, server, "symbol_read", map[string]any{"symbol_id": symbolID})
	source, _ := read["result"].(map[string]any)
	refreshed, _ := source["graph_version"].(string)
	if read["isError"] == true || refreshed == "" || refreshed == scan.Generation.ID || source["freshness"] != string(knowledge.FreshnessCurrent) ||
		!strings.Contains(fmt.Sprint(source["content"]), "return 2") {
		t.Fatalf("symbol_read did not re-resolve the edited worktree: %s", journeyText(read))
	}
	server.Close()

	// Symbols profile (refresh disabled), another edit: an explicit stale
	// refusal, not an answer from the outdated pointers.
	symbols := journeyOpen(t, root)
	symbols.SetRefreshOnRead(false)
	write("package main\n\n\n// moved again\nfunc Target() int { return 3 }\n")
	refused := journeyMCPCall(t, root, symbols, "symbol_read", map[string]any{"symbol_id": symbolID})
	if refused["isError"] != true || !strings.Contains(journeyText(refused), "stale") {
		t.Fatalf("symbols-profile read of a drifted worktree was not a stale refusal: %#v", refused)
	}
	symbols.Close()

	// Reopen: the refusal persisted nothing, so restoring the bytes the
	// refreshed generation was built from makes exactly that generation
	// current again.
	write("package main\n\n// moved down\nfunc Target() int { return 2 }\n")
	reopened := journeyOpen(t, root)
	defer reopened.Close()
	reopened.SetRefreshOnRead(false)
	identity, err := reopened.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	state, err := reopened.CurrentRepositoryState(ctx, identity)
	if err != nil || state.GraphVersion != refreshed || state.Freshness != knowledge.FreshnessCurrent {
		t.Fatalf("persisted generation after reopen = %#v, %v; want %s current", state, err, refreshed)
	}
}
