package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	// Set WORKSPACE_ROOT to allow access to temp directories in tests.
	// Without this, path validation would reject temp file paths.
	// Use filepath.EvalSymlinks to resolve any symlinks (e.g., /var -> /private/var on macOS)
	// to ensure consistent path comparison.
	tmpDir := os.TempDir()
	if resolved, err := filepath.EvalSymlinks(tmpDir); err == nil {
		tmpDir = resolved
	}
	os.Setenv("WORKSPACE_ROOT", tmpDir)

	// Enable danger mode for shell command tests
	os.Setenv("OPENEXEC_MODE", "danger-full-access")

	os.Exit(m.Run())
}

func sendAndReceive(t *testing.T, lines ...string) []Response {
	t.Helper()
	input := strings.Join(lines, "\n") + "\n"
	in := strings.NewReader(input)
	out := new(bytes.Buffer)

	srv, err := NewServer(in, out)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
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

// sendAndReceiveWithWorkspace sets WORKSPACE_ROOT to the given directory
// before running the MCP server, then restores the original value.
func sendAndReceiveWithWorkspace(t *testing.T, workspaceRoot string, lines ...string) []Response {
	t.Helper()
	oldRoot := os.Getenv("WORKSPACE_ROOT")
	os.Setenv("WORKSPACE_ROOT", workspaceRoot)
	defer os.Setenv("WORKSPACE_ROOT", oldRoot)
	return sendAndReceive(t, lines...)
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
	// In danger-full-access mode (set in TestMain), we expect 7 tools:
	// 5 core tools + write_file + run_shell_command
	if len(tools) != 7 {
		t.Fatalf("expected 7 tools in danger-full-access mode, got %d", len(tools))
	}

	// Check openexec_signal tool
	tool, _ := tools[0].(map[string]interface{})
	if tool["name"] != "openexec_signal" {
		t.Errorf("tool[0] name = %v, want openexec_signal", tool["name"])
	}

	schema, _ := tool["inputSchema"].(map[string]interface{})
	props, _ := schema["properties"].(map[string]interface{})
	if props["type"] == nil {
		t.Error("missing 'type' in openexec_signal input schema properties")
	}

	// Check read_file tool
	tool2, _ := tools[1].(map[string]interface{})
	if tool2["name"] != "read_file" {
		t.Errorf("tool[1] name = %v, want read_file", tool2["name"])
	}

	schema2, _ := tool2["inputSchema"].(map[string]interface{})
	props2, _ := schema2["properties"].(map[string]interface{})
	if props2["path"] == nil {
		t.Error("missing 'path' in read_file input schema properties")
	}

	// Check git_apply_patch tool (index 2)
	tool3, _ := tools[2].(map[string]interface{})
	if tool3["name"] != "git_apply_patch" {
		t.Errorf("tool[2] name = %v, want git_apply_patch", tool3["name"])
	}

	schema3, _ := tool3["inputSchema"].(map[string]interface{})
	props3, _ := schema3["properties"].(map[string]interface{})
	if props3["patch"] == nil {
		t.Error("missing 'patch' in git_apply_patch input schema properties")
	}

	// Check openexec_result tool (index 3)
	tool4, _ := tools[3].(map[string]interface{})
	if tool4["name"] != "openexec_result" {
		t.Errorf("tool[3] name = %v, want openexec_result", tool4["name"])
	}

	// Check openexec_action tool (index 4)
	tool5, _ := tools[4].(map[string]interface{})
	if tool5["name"] != "openexec_action" {
		t.Errorf("tool[4] name = %v, want openexec_action", tool5["name"])
	}

	// Check write_file tool (index 5, only in full-auto mode)
	tool6, _ := tools[5].(map[string]interface{})
	if tool6["name"] != "write_file" {
		t.Errorf("tool[5] name = %v, want write_file", tool6["name"])
	}

	schema6, _ := tool6["inputSchema"].(map[string]interface{})
	props6, _ := schema6["properties"].(map[string]interface{})
	if props6["path"] == nil {
		t.Error("missing 'path' in write_file input schema properties")
	}
	if props6["content"] == nil {
		t.Error("missing 'content' in write_file input schema properties")
	}

	// Check run_shell_command tool (index 6, only in full-auto mode)
	tool7, _ := tools[6].(map[string]interface{})
	if tool7["name"] != "run_shell_command" {
		t.Errorf("tool[6] name = %v, want run_shell_command", tool7["name"])
	}

	schema7, _ := tool7["inputSchema"].(map[string]interface{})
	props7, _ := schema7["properties"].(map[string]interface{})
	if props7["command"] == nil {
		t.Error("missing 'command' in run_shell_command input schema properties")
	}
}

func TestToolsCallValid(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"openexec_signal","arguments":{"type":"phase-complete","reason":"All tests pass"}}}`,
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
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"openexec_signal","arguments":{"type":"bad-type"}}}`,
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

	srv, err := NewServer(in, out)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
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
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"openexec_signal","arguments":{"type":"progress","reason":"Step 1 done"}}}`,
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

	srv, err := NewServer(in, out)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	err = srv.Serve()

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
			`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"openexec_signal","arguments":{"type":"`+st+`"}}}`,
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
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"openexec_signal","arguments":{"type":"route","target":"spark","reason":"Test failures found"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}
}

// Tests for read_file tool

func TestReadFileBasic(t *testing.T) {
	// Create a temporary file with known content
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Hello, World!"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`"}}}`,
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
	if text != testContent {
		t.Errorf("text = %q, want %q", text, testContent)
	}
}

func TestReadFileWithOffset(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Hello, World!"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`","offset":7}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if text != "World!" {
		t.Errorf("text = %q, want %q", text, "World!")
	}
}

func TestReadFileWithLength(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Hello, World!"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`","length":5}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if text != "Hello" {
		t.Errorf("text = %q, want %q", text, "Hello")
	}
}

func TestReadFileWithOffsetAndLength(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Hello, World!"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`","offset":7,"length":5}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if text != "World" {
		t.Errorf("text = %q, want %q", text, "World")
	}
}

func TestReadFileBinaryEncoding(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.bin")
	testContent := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE}
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`","encoding":"binary"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)

	// Should be base64 encoded
	decoded, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		t.Fatalf("failed to decode base64: %v", err)
	}
	if string(decoded) != string(testContent) {
		t.Errorf("decoded content = %v, want %v", decoded, testContent)
	}
}

func TestReadFileNotFound(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"/nonexistent/file.txt"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	// Tool errors return result with isError, not a JSON-RPC error
	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for non-existent file")
	}
}

func TestReadFileMissingPath(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	// Tool errors return result with isError
	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for missing path")
	}
}

func TestReadFileInvalidEncoding(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`","encoding":"invalid"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for invalid encoding")
	}
}

func TestReadFilePathTraversal(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"../../../etc/passwd"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for path traversal attempt")
	}
}

func TestReadFileDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+tmpDir+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for reading a directory")
	}
}

func TestReadFileEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.txt")
	if err := os.WriteFile(testFile, []byte{}, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if text != "" {
		t.Errorf("text = %q, want empty string", text)
	}
}

func TestReadFileLargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "large.txt")

	// Create a file with 10KB of content
	largeContent := strings.Repeat("abcdefghij", 1024)
	if err := os.WriteFile(testFile, []byte(largeContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if text != largeContent {
		t.Errorf("text length = %d, want %d", len(text), len(largeContent))
	}
}

func TestReadFileUTF8WithSpecialChars(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "utf8.txt")
	// Content with various Unicode characters
	testContent := "Hello, 世界! こんにちは 🌍 émojis: 🚀🎉"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`","encoding":"utf-8"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if text != testContent {
		t.Errorf("text = %q, want %q", text, testContent)
	}
}

func TestReadFileASCIIEncoding(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "ascii.txt")
	testContent := "Hello, ASCII World! 123"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`","encoding":"ascii"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if text != testContent {
		t.Errorf("text = %q, want %q", text, testContent)
	}
}

func TestReadFileUTF16Encoding(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "utf16.txt")
	testContent := "Hello UTF-16"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`","encoding":"utf-16"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	// Should succeed - utf-16 is a valid encoding
	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	// Content is read as-is (utf-16 encoding doesn't transform the bytes currently)
	if text != testContent {
		t.Errorf("text = %q, want %q", text, testContent)
	}
}

func TestReadFileOffsetExceedsFileSize(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "small.txt")
	testContent := "Short"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Offset 100 exceeds file size of 5 bytes
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`","offset":100}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	// Should return empty content, not an error
	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if text != "" {
		t.Errorf("text = %q, want empty string when offset exceeds file size", text)
	}
}

func TestReadFileLengthExceedsRemaining(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Hello"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Request 100 bytes but file only has 5
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`","length":100}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	// Should return the available content
	if text != testContent {
		t.Errorf("text = %q, want %q", text, testContent)
	}
}

func TestReadFileNullByteInPath(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"/tmp/file\u0000.txt"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for path with null byte")
	}

	content, _ := result["content"].([]interface{})
	if len(content) > 0 {
		item, _ := content[0].(map[string]interface{})
		text, _ := item["text"].(string)
		if !strings.Contains(text, "null byte") {
			t.Errorf("expected error message to mention null byte, got: %s", text)
		}
	}
}

func TestReadFileRelativePath(t *testing.T) {
	// Skip: Relative paths are resolved against the current working directory,
	// not WORKSPACE_ROOT. This security model doesn't support relative paths
	// unless the CWD is within WORKSPACE_ROOT.
	t.Skip("Relative paths resolved against CWD, not WORKSPACE_ROOT")

	// Create a temporary file in a temp directory
	tmpDir := t.TempDir()
	tmpFile, err := os.CreateTemp(tmpDir, "test-read-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	testContent := "Relative path content"
	if _, err := tmpFile.WriteString(testContent); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Use relative path (just the filename) with workspace root set to the temp dir
	relPath := filepath.Base(tmpFile.Name())

	resps := sendAndReceiveWithWorkspace(t, tmpDir,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+relPath+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	// Should succeed - relative paths are allowed in ValidatePathForRead
	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if text != testContent {
		t.Errorf("text = %q, want %q", text, testContent)
	}
}

func TestReadFileSymlink(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a real file
	realFile := filepath.Join(tmpDir, "real.txt")
	testContent := "Content via symlink"
	if err := os.WriteFile(realFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create real file: %v", err)
	}

	// Create a symlink
	symlinkFile := filepath.Join(tmpDir, "link.txt")
	if err := os.Symlink(realFile, symlinkFile); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+symlinkFile+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	// Should succeed - symlinks are allowed in ValidatePathForRead
	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if text != testContent {
		t.Errorf("text = %q, want %q", text, testContent)
	}
}

func TestReadFileInvalidJSONArguments(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":"not_an_object"}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	// Should return JSON-RPC error for invalid params
	if resps[0].Error == nil {
		t.Error("expected error for invalid JSON arguments")
	}
}

func TestReadFileUnicodeFilename(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "файл_测试_🎉.txt")
	testContent := "Unicode filename content"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Need to escape the path properly for JSON
	escapedPath := strings.ReplaceAll(testFile, `\`, `\\`)

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+escapedPath+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if text != testContent {
		t.Errorf("text = %q, want %q", text, testContent)
	}
}

func TestReadFileMultipleConcurrentReads(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple test files
	files := make([]string, 3)
	contents := []string{"File 1 content", "File 2 content", "File 3 content"}
	for i := 0; i < 3; i++ {
		files[i] = filepath.Join(tmpDir, "file"+string(rune('1'+i))+".txt")
		if err := os.WriteFile(files[i], []byte(contents[i]), 0644); err != nil {
			t.Fatalf("failed to create test file %d: %v", i, err)
		}
	}

	// Send multiple read requests in one batch
	requests := []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"` + files[0] + `"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"` + files[1] + `"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"` + files[2] + `"}}}`,
	}

	resps := sendAndReceive(t, requests...)

	if len(resps) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(resps))
	}

	// Verify each response
	for i, resp := range resps {
		if resp.Error != nil {
			t.Errorf("response %d: unexpected error: %v", i, resp.Error)
			continue
		}

		result, _ := resp.Result.(map[string]interface{})
		content, _ := result["content"].([]interface{})
		item, _ := content[0].(map[string]interface{})
		text, _ := item["text"].(string)
		if text != contents[i] {
			t.Errorf("response %d: text = %q, want %q", i, text, contents[i])
		}
	}
}

func TestReadFileBinaryContentWithUTF8(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "mixed.txt")
	// Content with mixed binary and UTF-8 text
	testContent := []byte("Hello\x00World\xFFEnd")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Read with binary encoding - should get base64
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`","encoding":"binary"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)

	// Decode and verify
	decoded, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		t.Fatalf("failed to decode base64: %v", err)
	}
	if string(decoded) != string(testContent) {
		t.Errorf("decoded content = %v, want %v", decoded, testContent)
	}
}

func TestReadFileZeroOffset(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Start from beginning"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`","offset":0}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if text != testContent {
		t.Errorf("text = %q, want %q", text, testContent)
	}
}

func TestReadFileWithNewlines(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "multiline.txt")
	testContent := "Line 1\nLine 2\nLine 3\n"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if text != testContent {
		t.Errorf("text = %q, want %q", text, testContent)
	}
}

func TestReadFilePartialReadAtEnd(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "0123456789"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Read last 3 characters starting from offset 7
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`","offset":7,"length":3}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if text != "789" {
		t.Errorf("text = %q, want %q", text, "789")
	}
}

func TestReadFileDefaultEncoding(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Default encoding test"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// No encoding specified - should default to utf-8
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	// Should return as plain text (utf-8 default), not base64
	if text != testContent {
		t.Errorf("text = %q, want %q", text, testContent)
	}
}

// Tests for write_file tool

func TestWriteFileBasic(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Hello, World!"

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"`+testContent+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	// Check the file was created with correct content
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read created file: %v", err)
	}
	if string(data) != testContent {
		t.Errorf("file content = %q, want %q", string(data), testContent)
	}

	// Check the response message
	result, _ := resps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if !strings.Contains(text, "Successfully wrote") {
		t.Errorf("expected success message, got %q", text)
	}
}

func TestWriteFileOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Create initial file
	initialContent := "Initial content"
	if err := os.WriteFile(testFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to create initial file: %v", err)
	}

	// Overwrite with new content
	newContent := "New content"
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"`+newContent+`","mode":"overwrite"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	// Verify content was overwritten
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != newContent {
		t.Errorf("file content = %q, want %q", string(data), newContent)
	}
}

func TestWriteFileAppend(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Create initial file
	initialContent := "Initial "
	if err := os.WriteFile(testFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to create initial file: %v", err)
	}

	// Append new content
	appendContent := "appended"
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"`+appendContent+`","mode":"append"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	// Verify content was appended
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	expected := initialContent + appendContent
	if string(data) != expected {
		t.Errorf("file content = %q, want %q", string(data), expected)
	}
}

func TestWriteFileBinaryEncoding(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.bin")
	binaryContent := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE}
	base64Content := base64.StdEncoding.EncodeToString(binaryContent)

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"`+base64Content+`","encoding":"binary"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	// Verify binary content was written correctly
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != string(binaryContent) {
		t.Errorf("file content = %v, want %v", data, binaryContent)
	}
}

func TestWriteFileCreateDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "nested", "dirs", "test.txt")
	testContent := "Content in nested directory"

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"`+testContent+`","create_directories":true}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	// Verify file was created in nested directory
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != testContent {
		t.Errorf("file content = %q, want %q", string(data), testContent)
	}
}

func TestWriteFileNoCreateDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "nonexistent", "test.txt")
	testContent := "Content"

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"`+testContent+`","create_directories":false}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	// Should fail because parent directory doesn't exist
	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for non-existent parent directory")
	}
}

func TestWriteFileMissingPath(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"content":"test"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for missing path")
	}
}

func TestWriteFileEmptyContent(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.txt")

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":""}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	// Verify empty file was created
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("file content = %q, want empty", string(data))
	}
}

func TestWriteFilePathTraversal(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"../../../tmp/evil.txt","content":"malicious"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for path traversal attempt")
	}
}

func TestWriteFileNullByteInPath(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"/tmp/file\u0000.txt","content":"test"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for path with null byte")
	}
}

func TestWriteFileInvalidEncoding(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"test","encoding":"invalid"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for invalid encoding")
	}
}

func TestWriteFileInvalidMode(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"test","mode":"invalid"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for invalid mode")
	}
}

func TestWriteFileInvalidBase64(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.bin")

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"not-valid-base64!!!","encoding":"binary"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for invalid base64")
	}
}

func TestWriteFileUTF8WithSpecialChars(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "utf8.txt")
	testContent := "Hello, 世界! こんにちは 🌍"

	// Need to properly escape the content for JSON
	contentJSON, _ := json.Marshal(testContent)
	// Remove surrounding quotes from the JSON string
	escapedContent := string(contentJSON[1 : len(contentJSON)-1])

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"`+escapedContent+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	// Verify UTF-8 content was written correctly
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != testContent {
		t.Errorf("file content = %q, want %q", string(data), testContent)
	}
}

func TestWriteFileAppendToNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "new.txt")
	testContent := "New content"

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"`+testContent+`","mode":"append"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	// Verify file was created with the content
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != testContent {
		t.Errorf("file content = %q, want %q", string(data), testContent)
	}
}

func TestWriteFileLargeContent(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "large.txt")

	// Create 10KB of content
	largeContent := strings.Repeat("abcdefghij", 1024)

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"`+largeContent+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	// Verify large content was written correctly
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != largeContent {
		t.Errorf("file content length = %d, want %d", len(data), len(largeContent))
	}
}

func TestWriteFileWithNewlines(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "multiline.txt")
	testContent := "Line 1\\nLine 2\\nLine 3"

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"`+testContent+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	// Verify content with newlines was written correctly
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	expected := "Line 1\nLine 2\nLine 3"
	if string(data) != expected {
		t.Errorf("file content = %q, want %q", string(data), expected)
	}
}

func TestWriteFileMultipleAppends(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// First write
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"First"}}}`,
	)
	if len(resps) != 1 || resps[0].Error != nil {
		t.Fatalf("first write failed")
	}

	// Second append
	resps = sendAndReceive(t,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":" Second","mode":"append"}}}`,
	)
	if len(resps) != 1 || resps[0].Error != nil {
		t.Fatalf("second write failed")
	}

	// Third append
	resps = sendAndReceive(t,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":" Third","mode":"append"}}}`,
	)
	if len(resps) != 1 || resps[0].Error != nil {
		t.Fatalf("third write failed")
	}

	// Verify final content
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	expected := "First Second Third"
	if string(data) != expected {
		t.Errorf("file content = %q, want %q", string(data), expected)
	}
}

func TestWriteFileRelativePath(t *testing.T) {
	// Skip: Relative paths are resolved against the current working directory,
	// not WORKSPACE_ROOT. This security model doesn't support relative paths
	// unless the CWD is within WORKSPACE_ROOT.
	t.Skip("Relative paths resolved against CWD, not WORKSPACE_ROOT")

	// Create a temporary directory and use relative path within it
	tmpDir := t.TempDir()
	testFile := "test-write-001.txt"
	testContent := "Relative path content"

	resps := sendAndReceiveWithWorkspace(t, tmpDir,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"`+testContent+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	// Verify file was created with correct content (file is in tmpDir)
	fullPath := filepath.Join(tmpDir, testFile)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != testContent {
		t.Errorf("file content = %q, want %q", string(data), testContent)
	}
}

func TestWriteFileInvalidJSONArguments(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":"not_an_object"}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	// Should return JSON-RPC error for invalid params
	if resps[0].Error == nil {
		t.Error("expected error for invalid JSON arguments")
	}
}

func TestWriteFileReadWriteRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "roundtrip.txt")
	testContent := "Round trip content"

	// Write the file
	writeResps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"`+testContent+`"}}}`,
	)
	if len(writeResps) != 1 || writeResps[0].Error != nil {
		t.Fatalf("write failed")
	}

	// Read the file back
	readResps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`"}}}`,
	)
	if len(readResps) != 1 || readResps[0].Error != nil {
		t.Fatalf("read failed")
	}

	result, _ := readResps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if text != testContent {
		t.Errorf("read content = %q, want %q", text, testContent)
	}
}

func TestWriteFileBinaryRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "binary.bin")
	binaryContent := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0x7F, 0x80}
	base64Content := base64.StdEncoding.EncodeToString(binaryContent)

	// Write binary content
	writeResps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"`+base64Content+`","encoding":"binary"}}}`,
	)
	if len(writeResps) != 1 || writeResps[0].Error != nil {
		t.Fatalf("write failed")
	}

	// Read back as binary
	readResps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"read_file","arguments":{"path":"`+testFile+`","encoding":"binary"}}}`,
	)
	if len(readResps) != 1 || readResps[0].Error != nil {
		t.Fatalf("read failed")
	}

	result, _ := readResps[0].Result.(map[string]interface{})
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)

	// Decode and verify
	decoded, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		t.Fatalf("failed to decode base64: %v", err)
	}
	if string(decoded) != string(binaryContent) {
		t.Errorf("decoded content = %v, want %v", decoded, binaryContent)
	}
}

// Tests for run_shell_command tool

func TestRunShellCommandBasic(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo hello"}}}`,
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
	if !strings.Contains(text, "hello") {
		t.Errorf("text = %q, want to contain 'hello'", text)
	}

	// Verify exit code
	exitCode, _ := result["exit_code"].(float64)
	if exitCode != 0 {
		t.Errorf("exit_code = %v, want 0", exitCode)
	}

	// Verify stdout
	stdout, _ := result["stdout"].(string)
	if !strings.Contains(stdout, "hello") {
		t.Errorf("stdout = %q, want to contain 'hello'", stdout)
	}
}

func TestRunShellCommandWithArgs(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo","args":["hello","world"]}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)
	if !strings.Contains(stdout, "hello world") {
		t.Errorf("stdout = %q, want to contain 'hello world'", stdout)
	}

	exitCode, _ := result["exit_code"].(float64)
	if exitCode != 0 {
		t.Errorf("exit_code = %v, want 0", exitCode)
	}
}

func TestRunShellCommandWithWorkingDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"pwd","working_directory":"`+tmpDir+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)
	if !strings.Contains(stdout, tmpDir) {
		t.Errorf("stdout = %q, want to contain %q", stdout, tmpDir)
	}
}

func TestRunShellCommandWithEnv(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo $MY_VAR","env":{"MY_VAR":"test_value"}}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)
	if !strings.Contains(stdout, "test_value") {
		t.Errorf("stdout = %q, want to contain 'test_value'", stdout)
	}
}

func TestRunShellCommandWithStdin(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"cat","stdin":"input from stdin"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)
	if stdout != "input from stdin" {
		t.Errorf("stdout = %q, want 'input from stdin'", stdout)
	}
}

func TestRunShellCommandWithTimeout(t *testing.T) {
	// Use a command that would take longer than the timeout
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"sleep 10","timeout_ms":200}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	// Should not have a JSON-RPC error
	if resps[0].Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})

	// Should have isError set
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for timeout")
	}

	// Check exit code is -1 for timeout
	exitCode, _ := result["exit_code"].(float64)
	if exitCode != -1 {
		t.Errorf("exit_code = %v, want -1 for timeout", exitCode)
	}

	// Check error message mentions timeout
	content, _ := result["content"].([]interface{})
	if len(content) > 0 {
		item, _ := content[0].(map[string]interface{})
		text, _ := item["text"].(string)
		if !strings.Contains(text, "timed out") {
			t.Errorf("text = %q, want to contain 'timed out'", text)
		}
	}
}

func TestRunShellCommandExitCode(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"exit 42"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})

	// Should have isError set
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for non-zero exit")
	}

	// Check exit code
	exitCode, _ := result["exit_code"].(float64)
	if exitCode != 42 {
		t.Errorf("exit_code = %v, want 42", exitCode)
	}
}

func TestRunShellCommandStderr(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo error >&2"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})

	// Verify stderr
	stderr, _ := result["stderr"].(string)
	if !strings.Contains(stderr, "error") {
		t.Errorf("stderr = %q, want to contain 'error'", stderr)
	}

	// stdout should be empty
	stdout, _ := result["stdout"].(string)
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

func TestRunShellCommandStdoutAndStderr(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo stdout; echo stderr >&2"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})

	// Verify both stdout and stderr
	stdout, _ := result["stdout"].(string)
	if !strings.Contains(stdout, "stdout") {
		t.Errorf("stdout = %q, want to contain 'stdout'", stdout)
	}

	stderr, _ := result["stderr"].(string)
	if !strings.Contains(stderr, "stderr") {
		t.Errorf("stderr = %q, want to contain 'stderr'", stderr)
	}

	// Content should include both
	content, _ := result["content"].([]interface{})
	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if !strings.Contains(text, "stdout") || !strings.Contains(text, "stderr") {
		t.Errorf("text = %q, want to contain both 'stdout' and 'stderr'", text)
	}
}

func TestRunShellCommandPipe(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo hello world | tr 'a-z' 'A-Z'"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)
	if !strings.Contains(stdout, "HELLO WORLD") {
		t.Errorf("stdout = %q, want to contain 'HELLO WORLD'", stdout)
	}
}

func TestRunShellCommandEmptyCommand(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":""}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	// Should get a validation error
	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for empty command")
	}
}

func TestRunShellCommandTimeoutTooLow(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo test","timeout_ms":50}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	// Should get a validation error
	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for timeout too low")
	}
}

func TestRunShellCommandTimeoutTooHigh(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo test","timeout_ms":700000}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	// Should get a validation error
	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for timeout too high")
	}
}

func TestRunShellCommandDefaultTimeout(t *testing.T) {
	// This test verifies the default timeout is applied (should be quick enough to not timeout)
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo test"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	exitCode, _ := result["exit_code"].(float64)
	if exitCode != 0 {
		t.Errorf("exit_code = %v, want 0", exitCode)
	}
}

func TestRunShellCommandNonExistentCommand(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"nonexistent_command_xyz123"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	// Should not have a JSON-RPC error
	if resps[0].Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})

	// Should have isError set (command not found)
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for non-existent command")
	}

	// Exit code should be non-zero
	exitCode, _ := result["exit_code"].(float64)
	if exitCode == 0 {
		t.Error("expected non-zero exit code for non-existent command")
	}
}

func TestRunShellCommandInvalidWorkingDirectory(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"ls","working_directory":"/nonexistent/directory/path"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	// Should have isError set
	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for invalid working directory")
	}
}

func TestRunShellCommandMultipleEnvVars(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo $VAR1 $VAR2","env":{"VAR1":"value1","VAR2":"value2"}}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)
	if !strings.Contains(stdout, "value1") || !strings.Contains(stdout, "value2") {
		t.Errorf("stdout = %q, want to contain 'value1' and 'value2'", stdout)
	}
}

func TestRunShellCommandMultilineStdin(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"cat -n","stdin":"line1\nline2\nline3"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)
	// cat -n should output numbered lines
	if !strings.Contains(stdout, "line1") || !strings.Contains(stdout, "line2") || !strings.Contains(stdout, "line3") {
		t.Errorf("stdout = %q, want to contain all three lines", stdout)
	}
}

func TestRunShellCommandChainedCommands(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo first && echo second && echo third"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)
	if !strings.Contains(stdout, "first") || !strings.Contains(stdout, "second") || !strings.Contains(stdout, "third") {
		t.Errorf("stdout = %q, want to contain 'first', 'second', and 'third'", stdout)
	}
}

func TestRunShellCommandArgsWithSpaces(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo","args":["hello world","foo bar"]}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)
	// With args mode, spaces in arguments should be preserved
	if !strings.Contains(stdout, "hello world") || !strings.Contains(stdout, "foo bar") {
		t.Errorf("stdout = %q, want to contain 'hello world' and 'foo bar'", stdout)
	}
}

func TestRunShellCommandInvalidJSONArguments(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":"not_an_object"}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	// Should return JSON-RPC error for invalid params
	if resps[0].Error == nil {
		t.Error("expected error for invalid JSON arguments")
	}
}

func TestRunShellCommandFileListing(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some test files
	for _, name := range []string{"file1.txt", "file2.txt", "file3.txt"} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("content"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"ls -1","working_directory":"`+tmpDir+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)

	for _, name := range []string{"file1.txt", "file2.txt", "file3.txt"} {
		if !strings.Contains(stdout, name) {
			t.Errorf("stdout = %q, want to contain %q", stdout, name)
		}
	}
}

func TestRunShellCommandEmptyOutput(t *testing.T) {
	// Test command that produces no output
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"true"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})

	// Verify exit code is 0
	exitCode, _ := result["exit_code"].(float64)
	if exitCode != 0 {
		t.Errorf("exit_code = %v, want 0", exitCode)
	}

	// Stdout should be empty
	stdout, _ := result["stdout"].(string)
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}

	// Stderr should be empty
	stderr, _ := result["stderr"].(string)
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestRunShellCommandBinaryOutput(t *testing.T) {
	// Test command that outputs binary-like data
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"printf '\\x00\\x01\\x02'"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})

	exitCode, _ := result["exit_code"].(float64)
	if exitCode != 0 {
		t.Errorf("exit_code = %v, want 0", exitCode)
	}

	// Should have some output (binary bytes)
	stdout, _ := result["stdout"].(string)
	if len(stdout) != 3 {
		t.Errorf("stdout length = %d, want 3", len(stdout))
	}
}

func TestRunShellCommandLargeOutput(t *testing.T) {
	// Test command that produces a moderate amount of output
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"seq 1 1000"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})

	exitCode, _ := result["exit_code"].(float64)
	if exitCode != 0 {
		t.Errorf("exit_code = %v, want 0", exitCode)
	}

	stdout, _ := result["stdout"].(string)
	// Should contain 1 through 1000
	if !strings.Contains(stdout, "1\n") || !strings.Contains(stdout, "1000") {
		t.Errorf("stdout should contain numbers 1 through 1000")
	}
}

func TestRunShellCommandQuotesInArgs(t *testing.T) {
	// Test that quotes in arguments are handled correctly in args mode
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo","args":["it's a test","say \"hello\""]}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)

	// The quotes should be preserved in the output
	if !strings.Contains(stdout, "it's a test") {
		t.Errorf("stdout = %q, want to contain \"it's a test\"", stdout)
	}
}

func TestRunShellCommandEmptyArgs(t *testing.T) {
	// Test with empty args array - should use shell mode
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo hello","args":[]}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)

	if !strings.Contains(stdout, "hello") {
		t.Errorf("stdout = %q, want to contain 'hello'", stdout)
	}
}

func TestRunShellCommandEnvironmentInheritance(t *testing.T) {
	// Test that the command inherits the current environment
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo $HOME"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)

	// HOME should be set from the environment
	if strings.TrimSpace(stdout) == "" || strings.TrimSpace(stdout) == "$HOME" {
		t.Errorf("stdout = %q, expected HOME environment variable to be expanded", stdout)
	}
}

func TestRunShellCommandEnvOverrideExisting(t *testing.T) {
	// Test that custom env can override existing environment variables
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo $PATH","env":{"PATH":"/custom/path"}}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)

	if !strings.Contains(stdout, "/custom/path") {
		t.Errorf("stdout = %q, want to contain '/custom/path'", stdout)
	}
}

func TestRunShellCommandSpecialCharactersInEnv(t *testing.T) {
	// Test environment variables with special characters
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo $SPECIAL","env":{"SPECIAL":"hello world!@#$%"}}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)

	if !strings.Contains(stdout, "hello world") {
		t.Errorf("stdout = %q, want to contain 'hello world'", stdout)
	}
}

func TestRunShellCommandExitCodeRange(t *testing.T) {
	// Test various exit codes
	testCases := []struct {
		command  string
		exitCode int
	}{
		{"exit 0", 0},
		{"exit 1", 1},
		{"exit 127", 127},
		{"exit 255", 255},
	}

	for _, tc := range testCases {
		t.Run(tc.command, func(t *testing.T) {
			resps := sendAndReceive(t,
				`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"`+tc.command+`"}}}`,
			)

			if len(resps) != 1 {
				t.Fatalf("expected 1 response, got %d", len(resps))
			}

			result, _ := resps[0].Result.(map[string]interface{})
			exitCode, _ := result["exit_code"].(float64)

			if int(exitCode) != tc.exitCode {
				t.Errorf("exit_code = %v, want %d", exitCode, tc.exitCode)
			}
		})
	}
}

func TestRunShellCommandStdinWithNewlines(t *testing.T) {
	// Test stdin with various newline patterns
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"wc -l","stdin":"line1\nline2\nline3\n"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)

	// Should count 3 lines
	if !strings.Contains(stdout, "3") {
		t.Errorf("stdout = %q, want to contain '3' (line count)", stdout)
	}
}

func TestRunShellCommandConcurrentExecution(t *testing.T) {
	// Test running multiple commands in sequence
	requests := []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo first"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo second"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo third"}}}`,
	}

	resps := sendAndReceive(t, requests...)

	if len(resps) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(resps))
	}

	expectedOutputs := []string{"first", "second", "third"}
	for i, resp := range resps {
		if resp.Error != nil {
			t.Errorf("response %d: unexpected error: %v", i, resp.Error)
			continue
		}

		result, _ := resp.Result.(map[string]interface{})
		stdout, _ := result["stdout"].(string)
		if !strings.Contains(stdout, expectedOutputs[i]) {
			t.Errorf("response %d: stdout = %q, want to contain %q", i, stdout, expectedOutputs[i])
		}
	}
}

func TestRunShellCommandUnicodeOutput(t *testing.T) {
	// Test command with Unicode output
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"printf 'Hello, 世界! 🌍'"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)

	if !strings.Contains(stdout, "世界") || !strings.Contains(stdout, "🌍") {
		t.Errorf("stdout = %q, want to contain Unicode characters", stdout)
	}
}

func TestRunShellCommandMinimumTimeout(t *testing.T) {
	// Test with the minimum allowed timeout (100ms)
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo quick","timeout_ms":100}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)

	if !strings.Contains(stdout, "quick") {
		t.Errorf("stdout = %q, want to contain 'quick'", stdout)
	}
}

func TestRunShellCommandRedirectOutput(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output.txt")

	// Test shell redirect
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo redirect test > `+outputFile+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	exitCode, _ := result["exit_code"].(float64)
	if exitCode != 0 {
		t.Errorf("exit_code = %v, want 0", exitCode)
	}

	// Verify file was created with content
	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if !strings.Contains(string(content), "redirect test") {
		t.Errorf("file content = %q, want to contain 'redirect test'", string(content))
	}
}

func TestRunShellCommandSubshell(t *testing.T) {
	// Test subshell execution
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"(echo subshell; echo test)"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)

	if !strings.Contains(stdout, "subshell") || !strings.Contains(stdout, "test") {
		t.Errorf("stdout = %q, want to contain 'subshell' and 'test'", stdout)
	}
}

func TestRunShellCommandBacktickSubstitution(t *testing.T) {
	// Test command substitution
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"echo $(echo nested)"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	stdout, _ := result["stdout"].(string)

	if !strings.Contains(stdout, "nested") {
		t.Errorf("stdout = %q, want to contain 'nested'", stdout)
	}
}

// Tests for git_apply_patch tool

// setupGitRepo creates a temporary git repository for testing
func setupGitRepo(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to init git repo: %v, output: %s", err, out)
	}

	// Configure git user (required for commits)
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to set git email: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to set git name: %v", err)
	}

	return tmpDir
}

func TestGitApplyPatchBasic(t *testing.T) {
	tmpDir := setupGitRepo(t)

	// Create a file to patch
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Stage and commit the file
	cmd := exec.Command("git", "add", "test.txt")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}

	// Create a valid patch
	patch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,4 @@
 line1
+new line
 line2
 line3
`

	// Escape the patch for JSON (newlines become \n)
	escapedPatch := strings.ReplaceAll(patch, "\n", "\\n")

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git_apply_patch","arguments":{"patch":"`+escapedPatch+`","working_directory":"`+tmpDir+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	if resps[0].Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resps[0].Error)
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if isErr {
		content, _ := result["content"].([]interface{})
		if len(content) > 0 {
			item, _ := content[0].(map[string]interface{})
			text, _ := item["text"].(string)
			t.Fatalf("unexpected tool error: %s", text)
		}
		t.Fatalf("unexpected tool error (no message)")
	}

	content, _ := result["content"].([]interface{})
	if len(content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(content))
	}

	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if !strings.Contains(text, "Patch applied successfully") {
		t.Errorf("text = %q, want to contain 'Patch applied successfully'", text)
	}

	// Verify the file was patched
	patchedContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read patched file: %v", err)
	}
	if !strings.Contains(string(patchedContent), "new line") {
		t.Errorf("patched content = %q, want to contain 'new line'", string(patchedContent))
	}
}

func TestGitApplyPatchCheckOnly(t *testing.T) {
	tmpDir := setupGitRepo(t)

	// Create a file to patch
	testFile := filepath.Join(tmpDir, "test.txt")
	originalContent := "line1\nline2\nline3\n"
	if err := os.WriteFile(testFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Stage and commit the file
	cmd := exec.Command("git", "add", "test.txt")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}

	// Create a valid patch
	patch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,4 @@
 line1
+new line
 line2
 line3
`
	escapedPatch := strings.ReplaceAll(patch, "\n", "\\n")

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git_apply_patch","arguments":{"patch":"`+escapedPatch+`","working_directory":"`+tmpDir+`","check_only":true}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if isErr {
		content, _ := result["content"].([]interface{})
		if len(content) > 0 {
			item, _ := content[0].(map[string]interface{})
			text, _ := item["text"].(string)
			t.Fatalf("unexpected tool error: %s", text)
		}
	}

	content, _ := result["content"].([]interface{})
	if len(content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(content))
	}

	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if !strings.Contains(text, "can be applied cleanly") {
		t.Errorf("text = %q, want to contain 'can be applied cleanly'", text)
	}

	// Verify the file was NOT modified
	currentContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(currentContent) != originalContent {
		t.Errorf("file was modified despite check_only: got %q, want %q", string(currentContent), originalContent)
	}
}

func TestGitApplyPatchReverse(t *testing.T) {
	tmpDir := setupGitRepo(t)

	// Create a file with the "after patch" content
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("line1\nnew line\nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Stage and commit
	cmd := exec.Command("git", "add", "test.txt")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}

	// Apply the same patch in reverse to remove the line
	patch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,4 @@
 line1
+new line
 line2
 line3
`
	escapedPatch := strings.ReplaceAll(patch, "\n", "\\n")

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git_apply_patch","arguments":{"patch":"`+escapedPatch+`","working_directory":"`+tmpDir+`","reverse":true}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if isErr {
		content, _ := result["content"].([]interface{})
		if len(content) > 0 {
			item, _ := content[0].(map[string]interface{})
			text, _ := item["text"].(string)
			t.Fatalf("unexpected tool error: %s", text)
		}
	}

	content, _ := result["content"].([]interface{})
	if len(content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(content))
	}

	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if !strings.Contains(text, "unapplied successfully") {
		t.Errorf("text = %q, want to contain 'unapplied successfully'", text)
	}

	// Verify the line was removed
	patchedContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if strings.Contains(string(patchedContent), "new line") {
		t.Errorf("patched content = %q, should not contain 'new line'", string(patchedContent))
	}
}

func TestGitApplyPatchMissingPatch(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git_apply_patch","arguments":{}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for missing patch")
	}
}

func TestGitApplyPatchInvalidPatch(t *testing.T) {
	tmpDir := setupGitRepo(t)

	// Invalid patch (missing headers)
	invalidPatch := `this is not a valid patch`

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git_apply_patch","arguments":{"patch":"`+invalidPatch+`","working_directory":"`+tmpDir+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for invalid patch")
	}
}

func TestGitApplyPatchNotGitRepo(t *testing.T) {
	tmpDir := t.TempDir() // Not initialized as git repo

	patch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,4 @@
 line1
+new line
 line2
 line3
`
	escapedPatch := strings.ReplaceAll(patch, "\n", "\\n")

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git_apply_patch","arguments":{"patch":"`+escapedPatch+`","working_directory":"`+tmpDir+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for non-git directory")
	}

	content, _ := result["content"].([]interface{})
	if len(content) > 0 {
		item, _ := content[0].(map[string]interface{})
		text, _ := item["text"].(string)
		if !strings.Contains(text, "not a git repository") {
			t.Errorf("text = %q, want to contain 'not a git repository'", text)
		}
	}
}

func TestGitApplyPatchStats(t *testing.T) {
	tmpDir := setupGitRepo(t)

	// Create a file to patch
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Stage and commit
	cmd := exec.Command("git", "add", "test.txt")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}

	// Patch that adds 2 lines and removes 1
	patch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,4 @@
 line1
-line2
+new line2
+extra line
 line3
`
	escapedPatch := strings.ReplaceAll(patch, "\n", "\\n")

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git_apply_patch","arguments":{"patch":"`+escapedPatch+`","working_directory":"`+tmpDir+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if isErr {
		content, _ := result["content"].([]interface{})
		if len(content) > 0 {
			item, _ := content[0].(map[string]interface{})
			text, _ := item["text"].(string)
			t.Fatalf("unexpected tool error: %s", text)
		}
	}

	// Check stats
	stats, ok := result["stats"].(map[string]interface{})
	if !ok {
		t.Fatal("expected stats in result")
	}

	filesChanged, _ := stats["files_changed"].(float64)
	if filesChanged != 1 {
		t.Errorf("files_changed = %v, want 1", filesChanged)
	}

	additions, _ := stats["additions"].(float64)
	if additions != 2 {
		t.Errorf("additions = %v, want 2", additions)
	}

	deletions, _ := stats["deletions"].(float64)
	if deletions != 1 {
		t.Errorf("deletions = %v, want 1", deletions)
	}

	// Check affected_files
	affectedFiles, ok := result["affected_files"].([]interface{})
	if !ok || len(affectedFiles) == 0 {
		t.Error("expected affected_files in result")
	}
}

func TestGitApplyPatchFailedApply(t *testing.T) {
	tmpDir := setupGitRepo(t)

	// Create a dummy file and commit to have a valid git repo with history
	dummyFile := filepath.Join(tmpDir, "dummy.txt")
	if err := os.WriteFile(dummyFile, []byte("dummy\n"), 0644); err != nil {
		t.Fatalf("failed to create dummy file: %v", err)
	}

	cmd := exec.Command("git", "add", "dummy.txt")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}

	// Patch that tries to modify a file that doesn't exist
	// This should always fail
	patch := `--- a/nonexistent.txt
+++ b/nonexistent.txt
@@ -1,3 +1,4 @@
 line1
+new line
 line2
 line3
`
	escapedPatch := strings.ReplaceAll(patch, "\n", "\\n")

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git_apply_patch","arguments":{"patch":"`+escapedPatch+`","working_directory":"`+tmpDir+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for failed patch apply")
	}

	content, _ := result["content"].([]interface{})
	if len(content) > 0 {
		item, _ := content[0].(map[string]interface{})
		text, _ := item["text"].(string)
		if !strings.Contains(text, "Failed to apply patch") {
			t.Errorf("text = %q, want to contain 'Failed to apply patch'", text)
		}
	}
}

func TestGitApplyPatchNewFile(t *testing.T) {
	tmpDir := setupGitRepo(t)

	// Create initial commit with some file
	dummyFile := filepath.Join(tmpDir, "dummy.txt")
	if err := os.WriteFile(dummyFile, []byte("dummy\n"), 0644); err != nil {
		t.Fatalf("failed to create dummy file: %v", err)
	}

	cmd := exec.Command("git", "add", "dummy.txt")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}

	// Patch that creates a new file
	patch := `--- /dev/null
+++ b/newfile.txt
@@ -0,0 +1,3 @@
+line1
+line2
+line3
`
	escapedPatch := strings.ReplaceAll(patch, "\n", "\\n")

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git_apply_patch","arguments":{"patch":"`+escapedPatch+`","working_directory":"`+tmpDir+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if isErr {
		content, _ := result["content"].([]interface{})
		if len(content) > 0 {
			item, _ := content[0].(map[string]interface{})
			text, _ := item["text"].(string)
			t.Fatalf("unexpected tool error: %s", text)
		}
	}

	// Verify the new file was created
	newFile := filepath.Join(tmpDir, "newfile.txt")
	content, err := os.ReadFile(newFile)
	if err != nil {
		t.Fatalf("failed to read new file: %v", err)
	}
	if !strings.Contains(string(content), "line1") {
		t.Errorf("new file content = %q, want to contain 'line1'", string(content))
	}
}

func TestGitApplyPatchToolDefinition(t *testing.T) {
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	tools, _ := result["tools"].([]interface{})

	// Find git_apply_patch tool
	var gapTool map[string]interface{}
	for _, tool := range tools {
		t, _ := tool.(map[string]interface{})
		if t["name"] == "git_apply_patch" {
			gapTool = t
			break
		}
	}

	if gapTool == nil {
		t.Fatal("git_apply_patch tool not found in tools list")
	}

	if gapTool["description"] == nil {
		t.Error("git_apply_patch tool missing description")
	}

	schema, _ := gapTool["inputSchema"].(map[string]interface{})
	props, _ := schema["properties"].(map[string]interface{})

	// Check required properties
	if props["patch"] == nil {
		t.Error("missing 'patch' in git_apply_patch input schema properties")
	}
	if props["working_directory"] == nil {
		t.Error("missing 'working_directory' in git_apply_patch input schema properties")
	}
	if props["check_only"] == nil {
		t.Error("missing 'check_only' in git_apply_patch input schema properties")
	}
	if props["reverse"] == nil {
		t.Error("missing 'reverse' in git_apply_patch input schema properties")
	}
	if props["three_way"] == nil {
		t.Error("missing 'three_way' in git_apply_patch input schema properties")
	}
	if props["ignore_whitespace"] == nil {
		t.Error("missing 'ignore_whitespace' in git_apply_patch input schema properties")
	}
	if props["context_lines"] == nil {
		t.Error("missing 'context_lines' in git_apply_patch input schema properties")
	}

	required, _ := schema["required"].([]interface{})
	foundPatch := false
	for _, r := range required {
		if r == "patch" {
			foundPatch = true
			break
		}
	}
	if !foundPatch {
		t.Error("'patch' should be in required fields")
	}
}

func TestGitApplyPatchThreeWay(t *testing.T) {
	tmpDir := setupGitRepo(t)

	// Create a file to patch
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Stage and commit the file
	cmd := exec.Command("git", "add", "test.txt")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}

	// Create a valid patch with three_way option
	patch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,4 @@
 line1
+new line
 line2
 line3
`
	escapedPatch := strings.ReplaceAll(patch, "\n", "\\n")

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git_apply_patch","arguments":{"patch":"`+escapedPatch+`","working_directory":"`+tmpDir+`","three_way":true}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if isErr {
		content, _ := result["content"].([]interface{})
		if len(content) > 0 {
			item, _ := content[0].(map[string]interface{})
			text, _ := item["text"].(string)
			t.Fatalf("unexpected tool error: %s", text)
		}
	}

	content, _ := result["content"].([]interface{})
	if len(content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(content))
	}

	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if !strings.Contains(text, "Patch applied successfully") {
		t.Errorf("text = %q, want to contain 'Patch applied successfully'", text)
	}
}

func TestGitApplyPatchIgnoreWhitespace(t *testing.T) {
	tmpDir := setupGitRepo(t)

	// Create a file with extra whitespace to patch
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("line1 \nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Stage and commit the file
	cmd := exec.Command("git", "add", "test.txt")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}

	// Patch without trailing space in context line
	patch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,4 @@
 line1
+new line
 line2
 line3
`
	escapedPatch := strings.ReplaceAll(patch, "\n", "\\n")

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git_apply_patch","arguments":{"patch":"`+escapedPatch+`","working_directory":"`+tmpDir+`","ignore_whitespace":true}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if isErr {
		content, _ := result["content"].([]interface{})
		if len(content) > 0 {
			item, _ := content[0].(map[string]interface{})
			text, _ := item["text"].(string)
			t.Fatalf("unexpected tool error: %s", text)
		}
	}

	content, _ := result["content"].([]interface{})
	if len(content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(content))
	}
}

func TestGitApplyPatchContextLines(t *testing.T) {
	tmpDir := setupGitRepo(t)

	// Create a file to patch
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Stage and commit the file
	cmd := exec.Command("git", "add", "test.txt")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}

	// Create a valid patch with context_lines option set to 1 (non-default)
	patch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,4 @@
 line1
+new line
 line2
 line3
`
	escapedPatch := strings.ReplaceAll(patch, "\n", "\\n")

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git_apply_patch","arguments":{"patch":"`+escapedPatch+`","working_directory":"`+tmpDir+`","context_lines":1}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if isErr {
		content, _ := result["content"].([]interface{})
		if len(content) > 0 {
			item, _ := content[0].(map[string]interface{})
			text, _ := item["text"].(string)
			t.Fatalf("unexpected tool error: %s", text)
		}
	}

	content, _ := result["content"].([]interface{})
	if len(content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(content))
	}

	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if !strings.Contains(text, "Patch applied successfully") {
		t.Errorf("text = %q, want to contain 'Patch applied successfully'", text)
	}
}

func TestGitApplyPatchCheckOnlyFailure(t *testing.T) {
	tmpDir := setupGitRepo(t)

	// Create initial file
	dummyFile := filepath.Join(tmpDir, "dummy.txt")
	if err := os.WriteFile(dummyFile, []byte("dummy\n"), 0644); err != nil {
		t.Fatalf("failed to create dummy file: %v", err)
	}

	cmd := exec.Command("git", "add", "dummy.txt")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}

	// Patch that references a file that doesn't exist (should fail check)
	patch := `--- a/nonexistent.txt
+++ b/nonexistent.txt
@@ -1,3 +1,4 @@
 line1
+new line
 line2
 line3
`
	escapedPatch := strings.ReplaceAll(patch, "\n", "\\n")

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git_apply_patch","arguments":{"patch":"`+escapedPatch+`","working_directory":"`+tmpDir+`","check_only":true}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Error("expected isError: true for check_only failure")
	}

	content, _ := result["content"].([]interface{})
	if len(content) > 0 {
		item, _ := content[0].(map[string]interface{})
		text, _ := item["text"].(string)
		if !strings.Contains(text, "cannot be applied cleanly") {
			t.Errorf("text = %q, want to contain 'cannot be applied cleanly'", text)
		}
	}
}

func TestGitApplyPatchInvalidJSON(t *testing.T) {
	// Test with malformed JSON arguments
	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git_apply_patch","arguments":"not valid json"}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	// Should return a JSON-RPC error for invalid params
	if resps[0].Error == nil {
		t.Error("expected JSON-RPC error for invalid JSON arguments")
	}
}

func TestGitApplyPatchDeleteFile(t *testing.T) {
	tmpDir := setupGitRepo(t)

	// Create a file to delete
	testFile := filepath.Join(tmpDir, "todelete.txt")
	if err := os.WriteFile(testFile, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Stage and commit the file
	cmd := exec.Command("git", "add", "todelete.txt")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}

	// Create a delete patch
	patch := `--- a/todelete.txt
+++ /dev/null
@@ -1,3 +0,0 @@
-line1
-line2
-line3
`
	escapedPatch := strings.ReplaceAll(patch, "\n", "\\n")

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git_apply_patch","arguments":{"patch":"`+escapedPatch+`","working_directory":"`+tmpDir+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if isErr {
		content, _ := result["content"].([]interface{})
		if len(content) > 0 {
			item, _ := content[0].(map[string]interface{})
			text, _ := item["text"].(string)
			t.Fatalf("unexpected tool error: %s", text)
		}
	}

	// Verify the file was deleted
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("expected file to be deleted")
	}

	content, _ := result["content"].([]interface{})
	if len(content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(content))
	}

	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if !strings.Contains(text, "Patch applied successfully") {
		t.Errorf("text = %q, want to contain 'Patch applied successfully'", text)
	}
}

func TestGitApplyPatchMultipleFiles(t *testing.T) {
	tmpDir := setupGitRepo(t)

	// Create files to patch
	testFile1 := filepath.Join(tmpDir, "file1.txt")
	if err := os.WriteFile(testFile1, []byte("file1 line1\nfile1 line2\n"), 0644); err != nil {
		t.Fatalf("failed to create test file1: %v", err)
	}

	testFile2 := filepath.Join(tmpDir, "file2.txt")
	if err := os.WriteFile(testFile2, []byte("file2 line1\nfile2 line2\n"), 0644); err != nil {
		t.Fatalf("failed to create test file2: %v", err)
	}

	// Stage and commit the files
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}

	// Create a multi-file patch
	patch := `--- a/file1.txt
+++ b/file1.txt
@@ -1,2 +1,3 @@
 file1 line1
+file1 new line
 file1 line2
--- a/file2.txt
+++ b/file2.txt
@@ -1,2 +1,3 @@
 file2 line1
+file2 new line
 file2 line2
`
	escapedPatch := strings.ReplaceAll(patch, "\n", "\\n")

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git_apply_patch","arguments":{"patch":"`+escapedPatch+`","working_directory":"`+tmpDir+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if isErr {
		content, _ := result["content"].([]interface{})
		if len(content) > 0 {
			item, _ := content[0].(map[string]interface{})
			text, _ := item["text"].(string)
			t.Fatalf("unexpected tool error: %s", text)
		}
	}

	// Verify stats
	stats, ok := result["stats"].(map[string]interface{})
	if !ok {
		t.Fatal("expected stats in result")
	}

	filesChanged, _ := stats["files_changed"].(float64)
	if filesChanged != 2 {
		t.Errorf("files_changed = %v, want 2", filesChanged)
	}

	additions, _ := stats["additions"].(float64)
	if additions != 2 {
		t.Errorf("additions = %v, want 2", additions)
	}

	// Verify affected_files
	affectedFiles, ok := result["affected_files"].([]interface{})
	if !ok || len(affectedFiles) != 2 {
		t.Errorf("expected 2 affected_files, got %d", len(affectedFiles))
	}

	// Verify file contents were patched
	content1, _ := os.ReadFile(testFile1)
	if !strings.Contains(string(content1), "file1 new line") {
		t.Error("file1.txt was not patched correctly")
	}

	content2, _ := os.ReadFile(testFile2)
	if !strings.Contains(string(content2), "file2 new line") {
		t.Error("file2.txt was not patched correctly")
	}
}

// Tests for Python syntax validation in write_file

func skipIfNoPythonServer(t *testing.T) {
	t.Helper()
	_, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python 3 not available, skipping test")
	}
}

func TestWriteFilePythonValidSyntax(t *testing.T) {
	skipIfNoPythonServer(t)
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "valid.py")
	testContent := "def hello():\\n    print('Hello, World!')\\n"

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"`+testContent+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if isErr {
		content, _ := result["content"].([]interface{})
		if len(content) > 0 {
			item, _ := content[0].(map[string]interface{})
			text, _ := item["text"].(string)
			t.Fatalf("unexpected error: %s", text)
		}
	}

	// Verify file was created
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("Python file was not created")
	}

	// Verify content was written
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if !strings.Contains(string(content), "def hello()") {
		t.Error("file content does not contain expected Python code")
	}
}

func TestWriteFilePythonInvalidSyntax(t *testing.T) {
	skipIfNoPythonServer(t)
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid.py")
	// Invalid Python: missing colon after function definition
	testContent := "def hello()\\n    print('Hello, World!')\\n"

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"`+testContent+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Fatal("expected error for invalid Python syntax")
	}

	content, _ := result["content"].([]interface{})
	if len(content) == 0 {
		t.Fatal("expected error content")
	}

	item, _ := content[0].(map[string]interface{})
	text, _ := item["text"].(string)
	if !strings.Contains(text, "Python syntax error") {
		t.Errorf("expected 'Python syntax error' in response, got: %s", text)
	}

	// Verify file was NOT created
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("Python file should not have been created with invalid syntax")
	}
}

func TestWriteFilePythonSyntaxNotValidatedForNonPythonFiles(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "script.sh")
	// Content that would be invalid Python but valid shell script
	testContent := "echo hello()"

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"`+testContent+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if isErr {
		content, _ := result["content"].([]interface{})
		if len(content) > 0 {
			item, _ := content[0].(map[string]interface{})
			text, _ := item["text"].(string)
			t.Fatalf("unexpected error for non-Python file: %s", text)
		}
	}

	// Verify file was created
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("Shell script file was not created")
	}
}

func TestWriteFilePythonBinarySkipsValidation(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "data.py")
	// Binary content that would be invalid as Python but should be written as-is
	binaryContent := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	base64Content := base64.StdEncoding.EncodeToString(binaryContent)

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"`+base64Content+`","encoding":"binary"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if isErr {
		content, _ := result["content"].([]interface{})
		if len(content) > 0 {
			item, _ := content[0].(map[string]interface{})
			text, _ := item["text"].(string)
			t.Fatalf("unexpected error for binary Python file: %s", text)
		}
	}

	// Verify file was created
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("binary Python file was not created")
	}
}

func TestWriteFilePythonEmptyFile(t *testing.T) {
	skipIfNoPythonServer(t)
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.py")

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":""}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if isErr {
		content, _ := result["content"].([]interface{})
		if len(content) > 0 {
			item, _ := content[0].(map[string]interface{})
			text, _ := item["text"].(string)
			t.Fatalf("unexpected error for empty Python file: %s", text)
		}
	}

	// Verify empty file was created (empty files are valid Python)
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("empty Python file was not created")
	}
}

func TestWriteFilePythonWithVenvPath(t *testing.T) {
	// Files in venv directories should skip validation
	tmpDir := t.TempDir()
	venvDir := filepath.Join(tmpDir, "venv", "lib", "python3.9", "site-packages")
	if err := os.MkdirAll(venvDir, 0755); err != nil {
		t.Fatalf("failed to create venv directory: %v", err)
	}
	testFile := filepath.Join(venvDir, "package.py")
	// Content that would be invalid Python
	testContent := "this is not valid python code(("

	resps := sendAndReceive(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_file","arguments":{"path":"`+testFile+`","content":"`+testContent+`"}}}`,
	)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}

	result, _ := resps[0].Result.(map[string]interface{})
	isErr, _ := result["isError"].(bool)
	if isErr {
		content, _ := result["content"].([]interface{})
		if len(content) > 0 {
			item, _ := content[0].(map[string]interface{})
			text, _ := item["text"].(string)
			// venv paths should skip validation, so this should not be a Python syntax error
			if strings.Contains(text, "Python syntax error") {
				t.Errorf("venv path should skip Python syntax validation, got: %s", text)
			}
		}
	}
}
