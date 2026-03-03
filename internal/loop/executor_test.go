package loop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/agent"
	"github.com/openexec/openexec/internal/mcp"
)

func TestNewExecutor(t *testing.T) {
	cfg := ExecutorConfig{
		WorkDir:   "/tmp",
		SessionID: "test-session",
	}

	e := NewExecutor(cfg)

	if e == nil {
		t.Fatal("NewExecutor returned nil")
	}

	// Check defaults were set
	if e.cfg.ApprovalTimeout != 5*time.Minute {
		t.Errorf("expected default ApprovalTimeout of 5m, got %v", e.cfg.ApprovalTimeout)
	}
	if e.cfg.ApprovalCheckInterval != 500*time.Millisecond {
		t.Errorf("expected default ApprovalCheckInterval of 500ms, got %v", e.cfg.ApprovalCheckInterval)
	}
	if e.cfg.MaxShellTimeout != 10*time.Minute {
		t.Errorf("expected default MaxShellTimeout of 10m, got %v", e.cfg.MaxShellTimeout)
	}

	// Check built-in tools are registered
	tools := e.ListTools()
	expectedTools := []string{"read_file", "write_file", "run_shell_command", "git_apply_patch", "axon_signal"}

	toolSet := make(map[string]bool)
	for _, t := range tools {
		toolSet[t] = true
	}

	for _, expected := range expectedTools {
		if !toolSet[expected] {
			t.Errorf("expected built-in tool %s to be registered", expected)
		}
	}
}

func TestExecutorRegisterTool(t *testing.T) {
	e := NewExecutor(ExecutorConfig{})

	// Register a custom tool
	handler := func(ctx context.Context, input json.RawMessage) (*ToolResult, error) {
		return &ToolResult{Output: "custom output"}, nil
	}

	err := e.RegisterTool("custom_tool", handler)
	if err != nil {
		t.Fatalf("failed to register tool: %v", err)
	}

	// Check it's registered
	tools := e.ListTools()
	found := false
	for _, t := range tools {
		if t == "custom_tool" {
			found = true
			break
		}
	}
	if !found {
		t.Error("custom_tool not found in registered tools")
	}

	// Try to register again - should fail
	err = e.RegisterTool("custom_tool", handler)
	if err == nil {
		t.Error("expected error when registering duplicate tool")
	}
}

func TestExecutorUnregisterTool(t *testing.T) {
	e := NewExecutor(ExecutorConfig{})

	// Register a custom tool
	handler := func(ctx context.Context, input json.RawMessage) (*ToolResult, error) {
		return &ToolResult{Output: "custom output"}, nil
	}
	e.RegisterTool("custom_tool", handler)

	// Unregister it
	err := e.UnregisterTool("custom_tool")
	if err != nil {
		t.Fatalf("failed to unregister tool: %v", err)
	}

	// Check it's removed
	tools := e.ListTools()
	for _, tool := range tools {
		if tool == "custom_tool" {
			t.Error("custom_tool should have been unregistered")
		}
	}

	// Try to unregister a built-in tool - should fail
	err = e.UnregisterTool("read_file")
	if err == nil {
		t.Error("expected error when unregistering built-in tool")
	}
}

func TestExecutorGetToolDefinitions(t *testing.T) {
	e := NewExecutor(ExecutorConfig{})

	defs := e.GetToolDefinitions()

	if len(defs) == 0 {
		t.Fatal("expected at least one tool definition")
	}

	// Check that definitions have required fields
	for _, def := range defs {
		name, ok := def["name"].(string)
		if !ok || name == "" {
			t.Error("tool definition missing name")
		}
		desc, ok := def["description"].(string)
		if !ok || desc == "" {
			t.Error("tool definition missing description")
		}
		schema, ok := def["inputSchema"].(map[string]interface{})
		if !ok || schema == nil {
			t.Error("tool definition missing inputSchema")
		}
	}
}

func TestExecutorExecute_UnknownTool(t *testing.T) {
	e := NewExecutor(ExecutorConfig{SessionID: "test-session"})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-1",
		ToolName:  "nonexistent_tool",
		ToolInput: json.RawMessage(`{}`),
	}

	result, err := e.Execute(context.Background(), toolCall)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error result for unknown tool")
	}
	if result.Output == "" {
		t.Error("expected error output for unknown tool")
	}
}

func TestExecutorExecute_InvalidContentType(t *testing.T) {
	e := NewExecutor(ExecutorConfig{})

	toolCall := agent.ContentBlock{
		Type: agent.ContentTypeText,
		Text: "not a tool call",
	}

	_, err := e.Execute(context.Background(), toolCall)
	if err == nil {
		t.Error("expected error for non-tool_use content block")
	}
}

func TestExecutorExecute_ReadFile(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Hello, World!"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	e := NewExecutor(ExecutorConfig{
		WorkDir:   tmpDir,
		SessionID: "test-session",
	})

	input, _ := json.Marshal(map[string]interface{}{
		"path": testFile,
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-1",
		ToolName:  "read_file",
		ToolInput: input,
	}

	result, err := e.Execute(context.Background(), toolCall)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if result.Output != testContent {
		t.Errorf("expected %q, got %q", testContent, result.Output)
	}
}

func TestExecutorExecute_ReadFile_WithOffset(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Hello, World!"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	e := NewExecutor(ExecutorConfig{
		WorkDir:   tmpDir,
		SessionID: "test-session",
	})

	input, _ := json.Marshal(map[string]interface{}{
		"path":   testFile,
		"offset": 7,
		"length": 5,
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-1",
		ToolName:  "read_file",
		ToolInput: input,
	}

	result, err := e.Execute(context.Background(), toolCall)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if result.Output != "World" {
		t.Errorf("expected %q, got %q", "World", result.Output)
	}
}

func TestExecutorExecute_WriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "output.txt")
	testContent := "Test content"

	e := NewExecutor(ExecutorConfig{
		WorkDir:   tmpDir,
		SessionID: "test-session",
	})

	input, _ := json.Marshal(map[string]interface{}{
		"path":    testFile,
		"content": testContent,
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-1",
		ToolName:  "write_file",
		ToolInput: input,
	}

	result, err := e.Execute(context.Background(), toolCall)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}

	// Verify the file was written
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(content) != testContent {
		t.Errorf("expected %q, got %q", testContent, string(content))
	}
}

func TestExecutorExecute_WriteFile_Append(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "append.txt")

	// Create initial file
	if err := os.WriteFile(testFile, []byte("Initial "), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	e := NewExecutor(ExecutorConfig{
		WorkDir:   tmpDir,
		SessionID: "test-session",
	})

	input, _ := json.Marshal(map[string]interface{}{
		"path":    testFile,
		"content": "Appended",
		"mode":    "append",
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-1",
		ToolName:  "write_file",
		ToolInput: input,
	}

	result, err := e.Execute(context.Background(), toolCall)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}

	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(content) != "Initial Appended" {
		t.Errorf("expected %q, got %q", "Initial Appended", string(content))
	}
}

func TestExecutorExecute_WriteFile_CreateDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "nested", "dir", "output.txt")
	testContent := "Nested content"

	e := NewExecutor(ExecutorConfig{
		WorkDir:   tmpDir,
		SessionID: "test-session",
	})

	input, _ := json.Marshal(map[string]interface{}{
		"path":               testFile,
		"content":            testContent,
		"create_directories": true,
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-1",
		ToolName:  "write_file",
		ToolInput: input,
	}

	result, err := e.Execute(context.Background(), toolCall)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}

	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(content) != testContent {
		t.Errorf("expected %q, got %q", testContent, string(content))
	}
}

func TestExecutorExecute_RunShellCommand(t *testing.T) {
	e := NewExecutor(ExecutorConfig{SessionID: "test-session"})

	input, _ := json.Marshal(map[string]interface{}{
		"command": "echo 'Hello World'",
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-1",
		ToolName:  "run_shell_command",
		ToolInput: input,
	}

	result, err := e.Execute(context.Background(), toolCall)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if result.Output != "Hello World\n" {
		t.Errorf("expected %q, got %q", "Hello World\n", result.Output)
	}
}

func TestExecutorExecute_RunShellCommand_WithWorkDir(t *testing.T) {
	tmpDir := t.TempDir()

	e := NewExecutor(ExecutorConfig{
		WorkDir:   tmpDir,
		SessionID: "test-session",
	})

	input, _ := json.Marshal(map[string]interface{}{
		"command": "pwd",
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-1",
		ToolName:  "run_shell_command",
		ToolInput: input,
	}

	result, err := e.Execute(context.Background(), toolCall)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}

	// Resolve symlinks for comparison (macOS /tmp is a symlink)
	expectedDir, _ := filepath.EvalSymlinks(tmpDir)
	actualDirRaw := filepath.Clean(result.Output[:len(result.Output)-1]) // Remove trailing newline
	actualDir, _ := filepath.EvalSymlinks(actualDirRaw)

	if actualDir != expectedDir {
		t.Errorf("expected working dir %q, got %q", expectedDir, actualDir)
	}
}

func TestExecutorExecute_RunShellCommand_WithEnv(t *testing.T) {
	e := NewExecutor(ExecutorConfig{SessionID: "test-session"})

	input, _ := json.Marshal(map[string]interface{}{
		"command": "echo $MY_VAR",
		"env": map[string]string{
			"MY_VAR": "test_value",
		},
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-1",
		ToolName:  "run_shell_command",
		ToolInput: input,
	}

	result, err := e.Execute(context.Background(), toolCall)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if result.Output != "test_value\n" {
		t.Errorf("expected %q, got %q", "test_value\n", result.Output)
	}
}

func TestExecutorExecute_RunShellCommand_NonZeroExit(t *testing.T) {
	e := NewExecutor(ExecutorConfig{SessionID: "test-session"})

	input, _ := json.Marshal(map[string]interface{}{
		"command": "exit 1",
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-1",
		ToolName:  "run_shell_command",
		ToolInput: input,
	}

	result, err := e.Execute(context.Background(), toolCall)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error for non-zero exit")
	}

	exitCode, ok := result.Metadata["exit_code"].(int)
	if !ok || exitCode != 1 {
		t.Errorf("expected exit code 1, got %v", result.Metadata["exit_code"])
	}
}

func TestExecutorExecute_AxonSignal(t *testing.T) {
	e := NewExecutor(ExecutorConfig{SessionID: "test-session"})

	input, _ := json.Marshal(map[string]interface{}{
		"type":   "progress",
		"reason": "Making progress on the task",
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-1",
		ToolName:  "axon_signal",
		ToolInput: input,
	}

	result, err := e.Execute(context.Background(), toolCall)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if result.Output != "Signal received: progress" {
		t.Errorf("unexpected output: %s", result.Output)
	}

	signalType, ok := result.Metadata["signal_type"].(string)
	if !ok || signalType != "progress" {
		t.Errorf("expected signal_type 'progress', got %v", result.Metadata["signal_type"])
	}
}

func TestExecutorExecute_AxonSignal_Invalid(t *testing.T) {
	e := NewExecutor(ExecutorConfig{SessionID: "test-session"})

	input, _ := json.Marshal(map[string]interface{}{
		"type": "invalid_signal_type",
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-1",
		ToolName:  "axon_signal",
		ToolInput: input,
	}

	result, err := e.Execute(context.Background(), toolCall)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error for invalid signal type")
	}
}

func TestExecutorExecute_EventCallback(t *testing.T) {
	var events []*LoopEvent
	var mu sync.Mutex

	e := NewExecutor(ExecutorConfig{
		SessionID: "test-session",
		EventCallback: func(event *LoopEvent) {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		},
	})

	input, _ := json.Marshal(map[string]interface{}{
		"command": "echo 'test'",
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-1",
		ToolName:  "run_shell_command",
		ToolInput: input,
	}

	_, err := e.Execute(context.Background(), toolCall)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// Should have: ToolCallRequested, ToolAutoApproved (no approval manager), ToolCallStart, ToolCallComplete
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}

	// First event should be ToolCallRequested
	if events[0].Type != ToolCallRequested {
		t.Errorf("expected first event to be ToolCallRequested, got %s", events[0].Type)
	}

	// Last event should be ToolCallComplete
	lastEvent := events[len(events)-1]
	if lastEvent.Type != ToolCallComplete {
		t.Errorf("expected last event to be ToolCallComplete, got %s", lastEvent.Type)
	}

	// All events should have the session ID
	for _, event := range events {
		if event.SessionID != "test-session" {
			t.Errorf("expected session ID 'test-session', got %s", event.SessionID)
		}
		if event.ToolCall == nil {
			t.Error("expected tool call info in event")
		}
	}
}

func TestExecutorExecuteBatch(t *testing.T) {
	e := NewExecutor(ExecutorConfig{SessionID: "test-session"})

	toolCalls := []agent.ContentBlock{
		{
			Type:      agent.ContentTypeToolUse,
			ToolUseID: "call-1",
			ToolName:  "run_shell_command",
			ToolInput: json.RawMessage(`{"command": "echo 'first'"}`),
		},
		{
			Type:      agent.ContentTypeToolUse,
			ToolUseID: "call-2",
			ToolName:  "run_shell_command",
			ToolInput: json.RawMessage(`{"command": "echo 'second'"}`),
		},
	}

	results, err := e.ExecuteBatch(context.Background(), toolCalls)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].Output != "first\n" {
		t.Errorf("expected first output 'first\\n', got %q", results[0].Output)
	}
	if results[1].Output != "second\n" {
		t.Errorf("expected second output 'second\\n', got %q", results[1].Output)
	}
}

func TestExecutorToContentBlock(t *testing.T) {
	e := NewExecutor(ExecutorConfig{})

	result := &ToolResult{
		Output: "Success!",
	}

	block := e.ToContentBlock("call-1", result)

	if block.Type != agent.ContentTypeToolResult {
		t.Errorf("expected type tool_result, got %s", block.Type)
	}
	if block.ToolResultID != "call-1" {
		t.Errorf("expected tool result ID 'call-1', got %s", block.ToolResultID)
	}
	if block.ToolOutput != "Success!" {
		t.Errorf("expected output 'Success!', got %s", block.ToolOutput)
	}
	if block.ToolError != "" {
		t.Error("expected no error")
	}
}

func TestExecutorToContentBlock_WithError(t *testing.T) {
	e := NewExecutor(ExecutorConfig{})

	result := &ToolResult{
		Output:  "Error: something went wrong",
		IsError: true,
	}

	block := e.ToContentBlock("call-1", result)

	if block.ToolError != "Error: something went wrong" {
		t.Errorf("expected error message, got %s", block.ToolError)
	}
}

func TestIsAxonSignal(t *testing.T) {
	tests := []struct {
		name     string
		toolCall agent.ContentBlock
		expected bool
	}{
		{
			name: "axon_signal tool",
			toolCall: agent.ContentBlock{
				Type:     agent.ContentTypeToolUse,
				ToolName: "axon_signal",
			},
			expected: true,
		},
		{
			name: "other tool",
			toolCall: agent.ContentBlock{
				Type:     agent.ContentTypeToolUse,
				ToolName: "read_file",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAxonSignal(tt.toolCall)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetSignalFromToolCall(t *testing.T) {
	input, _ := json.Marshal(map[string]interface{}{
		"type":   "phase-complete",
		"reason": "Task finished",
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolName:  "axon_signal",
		ToolInput: input,
	}

	sig, err := GetSignalFromToolCall(toolCall)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sig.Type != mcp.SignalPhaseComplete {
		t.Errorf("expected signal type phase-complete, got %s", sig.Type)
	}
	if sig.Reason != "Task finished" {
		t.Errorf("expected reason 'Task finished', got %s", sig.Reason)
	}
}

func TestGetSignalFromToolCall_NotAxonSignal(t *testing.T) {
	toolCall := agent.ContentBlock{
		Type:     agent.ContentTypeToolUse,
		ToolName: "read_file",
	}

	_, err := GetSignalFromToolCall(toolCall)
	if err == nil {
		t.Error("expected error for non-axon_signal tool")
	}
}

func TestExecutorConcurrency(t *testing.T) {
	e := NewExecutor(ExecutorConfig{SessionID: "test-session"})

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			input, _ := json.Marshal(map[string]interface{}{
				"command": "echo 'test'",
			})

			toolCall := agent.ContentBlock{
				Type:      agent.ContentTypeToolUse,
				ToolUseID: "call-" + string(rune('0'+i)),
				ToolName:  "run_shell_command",
				ToolInput: input,
			}

			_, err := e.Execute(context.Background(), toolCall)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent execution error: %v", err)
	}
}
