package tract

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/mcp"
)

// mockServer reads JSON-RPC requests from r and writes canned responses to w.
// It handles initialize, notifications/initialized, and tools/call for tract_brief.
func mockServer(r io.Reader, w io.Writer, briefData []byte) {
	decoder := json.NewDecoder(r)
	encoder := json.NewEncoder(w)

	for {
		var req mcp.Request
		if err := decoder.Decode(&req); err != nil {
			return // EOF or pipe closed
		}

		switch req.Method {
		case "initialize":
			encoder.Encode(mcp.Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]interface{}{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]interface{}{},
					"serverInfo": map[string]interface{}{
						"name":    "tract",
						"version": "1.0.0",
					},
				},
			})

		case "notifications/initialized":
			// Notification — no response.

		case "tools/call":
			var params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			json.Unmarshal(req.Params, &params)

			if params.Name != "tract_brief" {
				encoder.Encode(mcp.Response{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error:   &mcp.RPCError{Code: -32602, Message: "unknown tool: " + params.Name},
				})
				return
			}

			// Wrap the brief JSON inside MCP tool response format.
			encoder.Encode(mcp.Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]interface{}{
					"content": []interface{}{
						map[string]interface{}{
							"type": "text",
							"text": string(briefData),
						},
					},
				},
			})

		default:
			encoder.Encode(mcp.Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &mcp.RPCError{Code: -32601, Message: "method not found: " + req.Method},
			})
		}
	}
}

func TestBriefRoundTrip(t *testing.T) {
	briefData, err := os.ReadFile("testdata/brief_response.json")
	if err != nil {
		t.Fatalf("read brief fixture: %v", err)
	}

	// Set up pipes: client writes to serverIn, reads from serverOut.
	serverInR, serverInW := io.Pipe()
	serverOutR, serverOutW := io.Pipe()

	go mockServer(serverInR, serverOutW, briefData)

	client := NewClient(serverInW, serverOutR)

	brief, err := client.Brief("FWU-01.1.01")
	if err != nil {
		t.Fatalf("Brief: %v", err)
	}

	// Verify FWU fields.
	if brief.FWU.ID != "FWU-01.1.01" {
		t.Errorf("FWU.ID = %q, want %q", brief.FWU.ID, "FWU-01.1.01")
	}
	if brief.FWU.Name != "Database infrastructure" {
		t.Errorf("FWU.Name = %q, want %q", brief.FWU.Name, "Database infrastructure")
	}
	if brief.FWU.Status != "pending" {
		t.Errorf("FWU.Status = %q, want %q", brief.FWU.Status, "pending")
	}

	// Verify boundaries.
	if len(brief.Boundaries) != 4 {
		t.Errorf("len(Boundaries) = %d, want 4", len(brief.Boundaries))
	}

	// Verify dependencies.
	if len(brief.Dependencies) != 1 {
		t.Errorf("len(Dependencies) = %d, want 1", len(brief.Dependencies))
	}

	// Verify design decisions.
	if len(brief.DesignDecisions) != 1 {
		t.Errorf("len(DesignDecisions) = %d, want 1", len(brief.DesignDecisions))
	}
	if brief.DesignDecisions[0].Decision != "Which SQLite driver?" {
		t.Errorf("DesignDecisions[0].Decision = %q", brief.DesignDecisions[0].Decision)
	}

	// Verify interface contracts.
	if len(brief.InterfaceContracts) != 1 {
		t.Errorf("len(InterfaceContracts) = %d, want 1", len(brief.InterfaceContracts))
	}
	if brief.InterfaceContracts[0].Direction != "produces" {
		t.Errorf("InterfaceContracts[0].Direction = %q, want %q", brief.InterfaceContracts[0].Direction, "produces")
	}

	// Verify verification gates.
	if len(brief.VerificationGates) != 2 {
		t.Errorf("len(VerificationGates) = %d, want 2", len(brief.VerificationGates))
	}

	// Verify reasoning chain.
	if brief.ReasoningChain == nil {
		t.Fatal("ReasoningChain is nil")
	}
	if brief.ReasoningChain.Feature.ID != "F-01.1" {
		t.Errorf("ReasoningChain.Feature.ID = %q, want %q", brief.ReasoningChain.Feature.ID, "F-01.1")
	}
	if len(brief.ReasoningChain.Goals) != 1 {
		t.Errorf("len(ReasoningChain.Goals) = %d, want 1", len(brief.ReasoningChain.Goals))
	}

	// Verify dependency status.
	if len(brief.DependencyStatus) != 1 {
		t.Errorf("len(DependencyStatus) = %d, want 1", len(brief.DependencyStatus))
	}
	if brief.DependencyStatus[0].TargetFWUName != "Domain models" {
		t.Errorf("DependencyStatus[0].TargetFWUName = %q", brief.DependencyStatus[0].TargetFWUName)
	}

	// Verify predecessor specs.
	if len(brief.PredecessorSpecs) != 1 {
		t.Errorf("len(PredecessorSpecs) = %d, want 1", len(brief.PredecessorSpecs))
	}
	if brief.PredecessorSpecs[0].EntityName != "DatabaseConfig" {
		t.Errorf("PredecessorSpecs[0].EntityName = %q", brief.PredecessorSpecs[0].EntityName)
	}

	// Verify prior ICs.
	if len(brief.PriorICs) != 1 {
		t.Errorf("len(PriorICs) = %d, want 1", len(brief.PriorICs))
	}
	if brief.PriorICCount != 1 {
		t.Errorf("PriorICCount = %d, want 1", brief.PriorICCount)
	}

	serverInR.Close()
	serverOutW.Close()
}

func TestInitializeHandshakeFormat(t *testing.T) {
	// Capture what the client sends to verify the initialize request format.
	serverInR, serverInW := io.Pipe()
	serverOutR, serverOutW := io.Pipe()

	var capturedRequests []mcp.Request

	go func() {
		decoder := json.NewDecoder(serverInR)
		encoder := json.NewEncoder(serverOutW)
		for {
			var req mcp.Request
			if err := decoder.Decode(&req); err != nil {
				return
			}
			capturedRequests = append(capturedRequests, req)

			switch req.Method {
			case "initialize":
				encoder.Encode(mcp.Response{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result: map[string]interface{}{
						"protocolVersion": "2024-11-05",
					},
				})
			case "tools/call":
				briefData, _ := os.ReadFile("testdata/brief_response.json")
				encoder.Encode(mcp.Response{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result: map[string]interface{}{
						"content": []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": string(briefData),
							},
						},
					},
				})
			}
		}
	}()

	client := NewClient(serverInW, serverOutR)
	_, err := client.Brief("FWU-01.1.01")
	if err != nil {
		t.Fatalf("Brief: %v", err)
	}

	serverInR.Close()
	serverOutW.Close()

	// Verify initialize request format.
	if len(capturedRequests) < 3 {
		t.Fatalf("expected at least 3 requests, got %d", len(capturedRequests))
	}

	initReq := capturedRequests[0]
	if initReq.Method != "initialize" {
		t.Errorf("first request method = %q, want %q", initReq.Method, "initialize")
	}
	if initReq.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want %q", initReq.JSONRPC, "2.0")
	}
	if initReq.ID == nil {
		t.Error("initialize request ID is nil")
	}

	// Verify initialize params contain required fields.
	var initParams map[string]interface{}
	json.Unmarshal(initReq.Params, &initParams)
	if initParams["protocolVersion"] != "2024-11-05" {
		t.Errorf("protocolVersion = %v", initParams["protocolVersion"])
	}
	clientInfo, ok := initParams["clientInfo"].(map[string]interface{})
	if !ok {
		t.Fatal("clientInfo not found in initialize params")
	}
	if clientInfo["name"] != "axon" {
		t.Errorf("clientInfo.name = %v, want %q", clientInfo["name"], "axon")
	}

	// Verify notifications/initialized is second.
	notifReq := capturedRequests[1]
	if notifReq.Method != "notifications/initialized" {
		t.Errorf("second request method = %q, want %q", notifReq.Method, "notifications/initialized")
	}
	if notifReq.ID != nil {
		t.Error("notification should not have ID")
	}

	// Verify tools/call is third.
	callReq := capturedRequests[2]
	if callReq.Method != "tools/call" {
		t.Errorf("third request method = %q, want %q", callReq.Method, "tools/call")
	}
	var callParams struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	json.Unmarshal(callReq.Params, &callParams)
	if callParams.Name != "tract_brief" {
		t.Errorf("tool name = %q, want %q", callParams.Name, "tract_brief")
	}
	var args map[string]string
	json.Unmarshal(callParams.Arguments, &args)
	if args["fwu_id"] != "FWU-01.1.01" {
		t.Errorf("fwu_id = %q, want %q", args["fwu_id"], "FWU-01.1.01")
	}
}

func TestBriefMCPError(t *testing.T) {
	serverInR, serverInW := io.Pipe()
	serverOutR, serverOutW := io.Pipe()

	go func() {
		decoder := json.NewDecoder(serverInR)
		encoder := json.NewEncoder(serverOutW)
		for {
			var req mcp.Request
			if err := decoder.Decode(&req); err != nil {
				return
			}

			switch req.Method {
			case "initialize":
				encoder.Encode(mcp.Response{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result: map[string]interface{}{
						"protocolVersion": "2024-11-05",
					},
				})
			case "tools/call":
				encoder.Encode(mcp.Response{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error:   &mcp.RPCError{Code: -32602, Message: "FWU not found"},
				})
			}
		}
	}()

	client := NewClient(serverInW, serverOutR)
	_, err := client.Brief("FWU-NONEXISTENT")
	if err == nil {
		t.Fatal("expected error from Brief")
	}
	if !strings.Contains(err.Error(), "FWU not found") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "FWU not found")
	}

	serverInR.Close()
	serverOutW.Close()
}

func TestNewClientFromIO(t *testing.T) {
	serverInR, serverInW := io.Pipe()
	serverOutR, serverOutW := io.Pipe()

	client := NewClient(serverInW, serverOutR)
	if client.cmd != nil {
		t.Error("cmd should be nil for io-created client")
	}
	if client.nextID != 1 {
		t.Errorf("nextID = %d, want 1", client.nextID)
	}

	serverInR.Close()
	serverOutR.Close()
	serverInW.Close()
	serverOutW.Close()
}

func TestCloseWithoutSubprocess(t *testing.T) {
	serverInR, serverInW := io.Pipe()
	serverOutR, serverOutW := io.Pipe()

	client := NewClient(serverInW, serverOutR)
	err := client.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	serverInR.Close()
	serverOutR.Close()
	serverInW.Close()
	serverOutW.Close()
}

func TestExtractToolText(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		input := map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "hello world",
				},
			},
		}
		got, err := extractToolText(input)
		if err != nil {
			t.Fatal(err)
		}
		if got != "hello world" {
			t.Errorf("got %q, want %q", got, "hello world")
		}
	})

	t.Run("Empty Content", func(t *testing.T) {
		input := map[string]interface{}{
			"content": []interface{}{},
		}
		_, err := extractToolText(input)
		if err == nil {
			t.Error("expected error for empty content")
		}
	})

	t.Run("Malformed", func(t *testing.T) {
		input := map[string]interface{}{
			"something": "else",
		}
		_, err := extractToolText(input)
		if err == nil {
			t.Error("expected error for malformed input")
		}
	})
}
