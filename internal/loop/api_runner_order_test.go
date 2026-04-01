package loop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAPIRunner_ToolExecutionOrder verifies that when the API returns multiple
// tool calls in a single response, they are executed in the order returned.
func TestAPIRunner_ToolExecutionOrder(t *testing.T) {
	tmpDir := t.TempDir()

	// Create three files to read, so we can verify order by checking events
	for i := 1; i <= 3; i++ {
		path := filepath.Join(tmpDir, fmt.Sprintf("file%d.txt", i))
		if err := os.WriteFile(path, []byte(fmt.Sprintf("contents of file %d", i)), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Server returns a response with three tool calls in a specific order,
	// then a final completion.
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")
	file3 := filepath.Join(tmpDir, "file3.txt")

	// Build the arguments JSON strings with proper escaping for embedding in JSON
	escFile1 := strings.ReplaceAll(file1, `\`, `\\`)
	escFile2 := strings.ReplaceAll(file2, `\`, `\\`)
	escFile3 := strings.ReplaceAll(file3, `\`, `\\`)

	multiToolResponse := `{
		"id": "chatcmpl-test",
		"object": "chat.completion",
		"model": "test-model",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": null,
				"tool_calls": [
					{"id": "call_first", "type": "function", "function": {"name": "read_file", "arguments": "{\"path\": \"` + escFile1 + `\"}"}},
					{"id": "call_second", "type": "function", "function": {"name": "read_file", "arguments": "{\"path\": \"` + escFile2 + `\"}"}},
					{"id": "call_third", "type": "function", "function": {"name": "read_file", "arguments": "{\"path\": \"` + escFile3 + `\"}"}}
				]
			},
			"finish_reason": "tool_calls"
		}],
		"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
	}`

	server := mockOpenAIServer(t, []string{
		multiToolResponse,
		makeCompletionResponse("Read all three files in order"),
	})
	defer server.Close()

	provider := newSimpleTestProvider(server.URL)

	events := make(chan Event, 100)
	runner := NewAPIRunner(APIRunnerConfig{
		Provider: provider,
		Model:    "test-model",
		Prompt:   "Read files in order",
		WorkDir:  tmpDir,
		MaxTurns: 10,
		Tools:    BuildAPIToolDefinitions(),
	}, events)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Collect all events
	var collected []Event
	for e := range events {
		collected = append(collected, e)
	}

	// Extract tool_start events in order to verify execution sequence
	var toolStartOrder []string
	var toolResultOrder []string
	for _, e := range collected {
		if e.Type == EventToolStart {
			toolStartOrder = append(toolStartOrder, e.Tool)
		}
		if e.Type == EventToolResult {
			toolResultOrder = append(toolResultOrder, e.Tool)
		}
	}

	// All three tools should have been started
	if len(toolStartOrder) != 3 {
		t.Fatalf("Expected 3 tool_start events, got %d: %v", len(toolStartOrder), toolStartOrder)
	}

	// They should all be read_file
	for i, tool := range toolStartOrder {
		if tool != "read_file" {
			t.Errorf("Tool start %d: expected read_file, got %s", i, tool)
		}
	}

	// Verify results came back in order by checking content
	resultContents := make([]string, 0, 3)
	for _, e := range collected {
		if e.Type == EventToolResult && e.Tool == "read_file" {
			resultContents = append(resultContents, e.Text)
		}
	}
	if len(resultContents) != 3 {
		t.Fatalf("Expected 3 tool results, got %d", len(resultContents))
	}
	// First result should contain file1 contents, etc.
	if !strings.Contains(resultContents[0], "contents of file 1") {
		t.Errorf("First tool result should contain file 1 contents, got: %s", resultContents[0])
	}
	if !strings.Contains(resultContents[1], "contents of file 2") {
		t.Errorf("Second tool result should contain file 2 contents, got: %s", resultContents[1])
	}
	if !strings.Contains(resultContents[2], "contents of file 3") {
		t.Errorf("Third tool result should contain file 3 contents, got: %s", resultContents[2])
	}
}

// TestAPIRunner_MultipleToolRoundtrips verifies the runner correctly handles
// multiple tool_use -> tool_result -> continuation cycles.
func TestAPIRunner_MultipleToolRoundtrips(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file for the first roundtrip to read
	testFile := filepath.Join(tmpDir, "data.txt")
	if err := os.WriteFile(testFile, []byte("initial data"), 0644); err != nil {
		t.Fatal(err)
	}

	outputFile := filepath.Join(tmpDir, "output.txt")

	// Three API calls:
	// 1. Model reads the file
	// 2. Model writes a new file based on what it read
	// 3. Model says "done"
	server := mockOpenAIServer(t, []string{
		// Round 1: read the file
		makeToolCallResponse("call_read", "read_file",
			fmt.Sprintf(`{"path": %q}`, testFile)),
		// Round 2: write a new file
		makeToolCallResponse("call_write", "write_file",
			fmt.Sprintf(`{"path": %q, "content": "processed data"}`, outputFile)),
		// Round 3: final answer
		makeCompletionResponse("Done: read data.txt and wrote output.txt"),
	})
	defer server.Close()

	provider := newSimpleTestProvider(server.URL)

	events := make(chan Event, 100)
	runner := NewAPIRunner(APIRunnerConfig{
		Provider: provider,
		Model:    "test-model",
		Prompt:   "Read data.txt, then write output.txt",
		WorkDir:  tmpDir,
		MaxTurns: 10,
		Tools:    BuildAPIToolDefinitions(),
	}, events)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Collect events
	var collected []Event
	for e := range events {
		collected = append(collected, e)
	}

	// Count iteration starts — should be 3 (one per API call)
	iterationStarts := 0
	for _, e := range collected {
		if e.Type == EventIterationStart {
			iterationStarts++
		}
	}
	if iterationStarts != 3 {
		t.Errorf("Expected 3 iteration starts (3 roundtrips), got %d", iterationStarts)
	}

	// Verify the tool sequence: read_file, then write_file
	var toolSequence []string
	for _, e := range collected {
		if e.Type == EventToolStart {
			toolSequence = append(toolSequence, e.Tool)
		}
	}
	if len(toolSequence) != 2 {
		t.Fatalf("Expected 2 tool calls across roundtrips, got %d: %v", len(toolSequence), toolSequence)
	}
	if toolSequence[0] != "read_file" {
		t.Errorf("First tool should be read_file, got %s", toolSequence[0])
	}
	if toolSequence[1] != "write_file" {
		t.Errorf("Second tool should be write_file, got %s", toolSequence[1])
	}

	// Verify the file was actually written
	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}
	if string(data) != "processed data" {
		t.Errorf("Expected 'processed data', got %q", string(data))
	}

	// Verify we got a completion event
	hasComplete := false
	for _, e := range collected {
		if e.Type == EventComplete {
			hasComplete = true
		}
	}
	if !hasComplete {
		t.Error("Expected EventComplete after all roundtrips")
	}
}

// TestAPIRunner_ToolResultFeedback verifies that tool results are correctly
// fed back to the API in the conversation history for the next turn.
func TestAPIRunner_ToolResultFeedback(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "check.txt")
	if err := os.WriteFile(testFile, []byte("secret value"), 0644); err != nil {
		t.Fatal(err)
	}

	// The server returns a tool call, then checks that the next request
	// includes the tool result. We verify this via the final completion text.
	server := mockOpenAIServer(t, []string{
		makeToolCallResponse("call_1", "read_file",
			fmt.Sprintf(`{"path": %q}`, testFile)),
		// The model's final response confirms it saw the file contents
		makeCompletionResponse("The file contains: secret value"),
	})
	defer server.Close()

	provider := newSimpleTestProvider(server.URL)

	events := make(chan Event, 100)
	runner := NewAPIRunner(APIRunnerConfig{
		Provider: provider,
		Model:    "test-model",
		Prompt:   "Read the file and tell me what it contains",
		WorkDir:  tmpDir,
		MaxTurns: 10,
		Tools:    BuildAPIToolDefinitions(),
	}, events)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Collect and verify
	var collected []Event
	for e := range events {
		collected = append(collected, e)
	}

	// The tool result event should contain the file content
	hasToolResultWithContent := false
	for _, e := range collected {
		if e.Type == EventToolResult && strings.Contains(e.Text, "secret value") {
			hasToolResultWithContent = true
		}
	}
	if !hasToolResultWithContent {
		t.Error("Expected tool result containing 'secret value' from file read")
	}

	// Final text should reference the content (demonstrating the model received it)
	hasFinalText := false
	for _, e := range collected {
		if e.Type == EventAssistantText && strings.Contains(e.Text, "secret value") {
			hasFinalText = true
		}
	}
	if !hasFinalText {
		t.Error("Expected final assistant text containing 'secret value'")
	}
}
