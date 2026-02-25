package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func sendAndReceive(t *testing.T, lines ...string) []Response {
	t.Helper()
	input := strings.Join(lines, "\n") + "\n"
	in := strings.NewReader(input)
	out := new(bytes.Buffer)

	srv := NewServer(in, out)
	if err := srv.Serve(); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	var responses []Response
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var resp Response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("unmarshal response %q: %v", line, err)
		}
		responses = append(responses, resp)
	}
	return responses
}

func TestInitialize(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, ok := resps[0].Result.(map[string]interface{})
	if !ok {
		t.Fatalf("result is not a map: %T", resps[0].Result)
	}

	if result["protocolVersion"] != protocolVersion {
		t.Errorf("protocolVersion = %v, want %v", result["protocolVersion"], protocolVersion)
	}

	serverInfo, _ := result["serverInfo"].(map[string]interface{})
	if serverInfo["name"] != "axon-signal" {
		t.Errorf("serverInfo.name = %v", serverInfo["name"])
	}
}

func TestToolsList(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	tools, _ := result["tools"].([]interface{})
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	tool, _ := tools[0].(map[string]interface{})
	if tool["name"] != "axon_signal" {
		t.Errorf("tool name = %v", tool["name"])
	}

	schema, _ := tool["inputSchema"].(map[string]interface{})
	props, _ := schema["properties"].(map[string]interface{})
	if props["type"] == nil {
		t.Error("missing 'type' in input schema properties")
	}
}

func TestToolsCallValid(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"axon_signal","arguments":{"type":"phase-complete","reason":"All tests pass"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	if len(content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(content))
	}

	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if !strings.Contains(text, "phase-complete") {
		t.Errorf("text = %q, want to contain 'phase-complete'", text)
	}
}

func TestToolsCallUnknownSignalType(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"axon_signal","arguments":{"type":"bad-type"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	// Tool errors return result with isError, not a JSON-RPC error.
	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for unknown signal type")
	}
}

func TestToolsCallUnknownTool(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"other_tool","arguments":{}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestNotificationNoResponse(t *testing.T) {
	input := `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"
	in := strings.NewReader(input)
	out := new(bytes.Buffer)

	srv := NewServer(in, out)
	srv.Serve()

	if out.Len() != 0 {
		t.Errorf("expected no output for notification, got %q", out.String())
	}
}

func TestFullHandshake(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"axon_signal","arguments":{"type":"progress","reason":"Step 1 done"}}}`,
	)

	// 3 responses (initialize, tools/list, tools/call). Notification produces none.
	if len(resps) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(resps))
	}

	// Verify IDs match.
	for i, wantID := range []string{"1", "2", "3"} {
		got := string(resps[i].ID)
		if got != wantID {
			t.Errorf("response[%d] id = %s, want %s", i, got, wantID)
		}
	}
}

func TestEOFCleanExit(t *testing.T) {
	in := strings.NewReader("")
	out := new(bytes.Buffer)

	srv := NewServer(in, out)
	err := srv.Serve()

	if err != nil {
		t.Fatalf("Serve on empty input: %v", err)
	}
}

func TestToolsCallAllSignalTypes(t *testing.T) {
	types := []string{
		"phase-complete", "blocked", "decision-point", "progress",
		"planning-mismatch", "scope-discovery", "route",
	}

	for _, st := range types {
		resps := sendAndReceive(t,
			`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"axon_signal","arguments":{"type":"`+st+`"}}}`,
		)
		if len(resps) != 1 {
			t.Fatalf("type %q: expected 1 response, got %d", st, len(resps))
		}
		if resps[0].Error != nil {
			t.Errorf("type %q: unexpected error: %v", st, resps[0].Error)
		}
	}
}

func TestToolsCallRouteWithTarget(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"axon_signal","arguments":{"type":"route","target":"spark","reason":"Test failures found"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}
}
