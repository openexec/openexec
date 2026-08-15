package execution

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGateway answers the JSON-RPC shape Agent Console serves.
type fakeGateway struct {
	mu     sync.Mutex
	calls  []string
	tools  []map[string]any
	result map[string]any
	delay  time.Duration
	status int
}

func (g *fakeGateway) start(t *testing.T) *httptest.Server {
	t.Helper()
	if g.tools == nil {
		g.tools = []map[string]any{{
			"name": "provider_status", "description": "Which providers are ready.",
			"inputSchema": map[string]any{"type": "object"},
		}}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		g.mu.Lock()
		g.calls = append(g.calls, request.Method+":"+request.Params.Name)
		g.mu.Unlock()
		if g.delay > 0 {
			select {
			case <-time.After(g.delay):
			case <-r.Context().Done():
				return
			}
		}
		if g.status != 0 {
			w.WriteHeader(g.status)
			return
		}
		var result any
		switch request.Method {
		case "tools/list":
			result = map[string]any{"tools": g.tools}
		case "tools/call":
			// Assigned through the concrete map, not through `any`: a nil map
			// in an interface is not a nil interface, and the default below
			// would never be reached.
			if g.result != nil {
				result = g.result
			} else {
				result = map[string]any{"content": []map[string]string{{"type": "text", "text": "claude: signed out"}}}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result})
	}))
	t.Cleanup(server.Close)
	return server
}

func TestGatewayOffersOnlyWhatItListed(t *testing.T) {
	gateway := &fakeGateway{}
	server := gateway.start(t)

	executor, err := NewGatewayToolExecutor(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	tools := executor.Tools()
	if len(tools) != 1 || tools[0].Name != "provider_status" {
		t.Fatalf("tools = %+v", tools)
	}

	out, err := executor.ExecuteTool(context.Background(), ToolRequest{
		Name: "provider_status", Sandbox: Sandbox{Mode: SandboxReadOnly},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "signed out") {
		t.Errorf("result = %q", out)
	}

	// A name the gateway never listed is not a tool this run was authorized to
	// call, whoever asked for it.
	if _, err := executor.ExecuteTool(context.Background(), ToolRequest{
		Name: "run_command", Sandbox: Sandbox{Mode: SandboxReadOnly},
	}); err == nil {
		t.Fatal("an unlisted tool was forwarded")
	}
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	for _, call := range gateway.calls {
		if strings.Contains(call, "run_command") {
			t.Error("the unlisted call reached the gateway")
		}
	}
}

// The endpoint arrives on the request. Checking it is what makes the loopback
// rule a boundary rather than a convention — the cost of skipping it is a
// model handed an arbitrary HTTP client.
func TestGatewayRefusesEndpointsThatAreNotLoopback(t *testing.T) {
	for _, endpoint := range []string{
		"http://192.168.1.29:7449/mcp/secret/run",
		"http://example.com/mcp",
		"https://127.0.0.1:7449/mcp",
		"ftp://127.0.0.1/mcp",
		"http://[2001:db8::1]:7449/mcp",
		"::not a url",
	} {
		if _, err := NewGatewayToolExecutor(context.Background(), endpoint); err == nil {
			t.Errorf("accepted %q", endpoint)
		}
	}
	for _, endpoint := range []string{"http://127.0.0.1:7449/mcp", "http://localhost:7449/mcp", "http://[::1]:7449/mcp"} {
		if err := validateGatewayEndpoint(endpoint); err != nil {
			t.Errorf("refused loopback %q: %v", endpoint, err)
		}
	}
}

// A gateway that answers with a redirect is not the gateway that was named,
// and following one turns the loopback check into decoration.
func TestGatewayDoesNotFollowRedirects(t *testing.T) {
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1,
			"result": map[string]any{"tools": []map[string]any{{"name": "anything"}}}})
	}))
	defer elsewhere.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL, http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	if _, err := NewGatewayToolExecutor(context.Background(), redirector.URL); err == nil {
		t.Fatal("a redirected gateway was accepted")
	}
}

// Files and console state are two different authorizations. A run that holds
// both is the one case where either boundary stops being the only one in play.
func TestGatewayRefusesWorkspaceWrite(t *testing.T) {
	executor, err := NewGatewayToolExecutor(context.Background(), (&fakeGateway{}).start(t).URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := executor.ValidateAccess("/tmp", Sandbox{Mode: SandboxWorkspaceWrite}, nil); err == nil {
		t.Error("workspace-write was accepted alongside a gateway")
	}
	if err := executor.ValidateAccess("/tmp", Sandbox{Mode: SandboxReadOnly}, []string{"/tmp"}); err == nil {
		t.Error("writable roots were accepted alongside a gateway")
	}
	if _, err := executor.ExecuteTool(context.Background(), ToolRequest{
		Name: "provider_status", Sandbox: Sandbox{Mode: SandboxWorkspaceWrite},
	}); err == nil {
		t.Error("a workspace-write call was forwarded")
	}
}

// Cancelling a run must cancel what it is waiting on, or a cancelled turn sits
// on a socket until the timeout it was supposed to skip.
func TestGatewayCallStopsWhenTheRunIsCancelled(t *testing.T) {
	gateway := &fakeGateway{delay: 2 * time.Second}
	server := gateway.start(t)
	executor, err := NewGatewayToolExecutor(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, callErr := executor.ExecuteTool(ctx, ToolRequest{
			Name: "provider_status", Sandbox: Sandbox{Mode: SandboxReadOnly},
		})
		done <- callErr
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a cancelled call returned success")
		}
	case <-time.After(time.Second):
		// Comfortably inside the handler's own delay: waiting it out would
		// prove the call ended, not that cancelling ended it.
		t.Fatal("the call outlived its cancelled context")
	}
}

func TestGatewayRefusesAnUnusableToolList(t *testing.T) {
	empty := &fakeGateway{tools: []map[string]any{}}
	if _, err := NewGatewayToolExecutor(context.Background(), empty.start(t).URL); err == nil {
		t.Error("a gateway offering nothing was accepted")
	}
	nameless := &fakeGateway{tools: []map[string]any{{"description": "no name"}}}
	if _, err := NewGatewayToolExecutor(context.Background(), nameless.start(t).URL); err == nil {
		t.Error("a tool with no name was accepted")
	}
	broken := &fakeGateway{status: http.StatusInternalServerError}
	if _, err := NewGatewayToolExecutor(context.Background(), broken.start(t).URL); err == nil {
		t.Error("a failing gateway was accepted")
	}
}

// Everything a gateway returns is about to become model context.
func TestGatewayCapsWhatItReturns(t *testing.T) {
	gateway := &fakeGateway{result: map[string]any{
		"content": []map[string]string{{"type": "text", "text": strings.Repeat("x", gatewayMaxToolResponse*2)}},
	}}
	executor, err := NewGatewayToolExecutor(context.Background(), gateway.start(t).URL)
	if err != nil {
		t.Fatal(err)
	}
	out, err := executor.ExecuteTool(context.Background(), ToolRequest{
		Name: "provider_status", Sandbox: Sandbox{Mode: SandboxReadOnly},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) > gatewayMaxToolResponse+len("\n\n[truncated]") {
		t.Fatalf("result was %d bytes", len(out))
	}
	if !strings.HasSuffix(out, "[truncated]") {
		t.Error("the cut was silent")
	}
}

// A descriptor is how a caller chooses. Advertising workspace-write because
// *an* executor exists sends editing work to a provider that will refuse it at
// the turn — the refusal arriving after the choice instead of shaping it.
func TestDescriptorReportsTheExecutorsRealCapability(t *testing.T) {
	gateway, err := NewGatewayToolExecutor(context.Background(), (&fakeGateway{}).start(t).URL)
	if err != nil {
		t.Fatal(err)
	}
	withGateway, err := NewAPIProvider(APIProviderConfig{
		Adapter: &fakeAPIAdapter{}, Tools: gateway.Tools(), ToolExecutor: gateway,
	})
	if err != nil {
		t.Fatal(err)
	}
	capabilities := withGateway.Descriptor().Capabilities
	if capabilities.WorkspaceWrite {
		t.Error("a gateway-backed provider advertised workspace-write, which it refuses")
	}
	if !capabilities.ToolCalling {
		t.Error("a gateway-backed provider denied tool calling, which is all it does")
	}

	withFiles, err := NewAPIProvider(APIProviderConfig{
		Adapter: &fakeAPIAdapter{}, Tools: WorkspaceTools(Sandbox{Mode: SandboxWorkspaceWrite}),
		ToolExecutor: NewWorkspaceToolExecutor(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !withFiles.Descriptor().Capabilities.WorkspaceWrite {
		t.Error("a filesystem-backed provider denied workspace-write")
	}

	// No executor at all is a conversation, and cannot write anything.
	plain, err := NewAPIProvider(APIProviderConfig{Adapter: &fakeAPIAdapter{}})
	if err != nil {
		t.Fatal(err)
	}
	if plain.Descriptor().Capabilities.WorkspaceWrite || plain.Descriptor().Capabilities.ToolCalling {
		t.Error("a provider with no executor advertised tools")
	}
}
