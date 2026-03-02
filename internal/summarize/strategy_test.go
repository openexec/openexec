package summarize

import (
	"testing"

	"github.com/openexec/openexec/internal/agent"
)

func makeTextMessage(role agent.Role, text string) agent.Message {
	return agent.Message{
		Role: role,
		Content: []agent.ContentBlock{
			{Type: agent.ContentTypeText, Text: text},
		},
	}
}

func makeToolUseMessage(role agent.Role, toolName string, input []byte) agent.Message {
	return agent.Message{
		Role: role,
		Content: []agent.ContentBlock{
			{Type: agent.ContentTypeToolUse, ToolUseID: "tool-123", ToolName: toolName, ToolInput: input},
		},
	}
}

func makeToolResultMessage(role agent.Role, output string) agent.Message {
	return agent.Message{
		Role: role,
		Content: []agent.ContentBlock{
			{Type: agent.ContentTypeToolResult, ToolOutput: output},
		},
	}
}

func TestSlidingWindowStrategy_Name(t *testing.T) {
	s := &SlidingWindowStrategy{}
	if s.Name() != "sliding_window" {
		t.Errorf("expected name 'sliding_window', got %s", s.Name())
	}
}

func TestSlidingWindowStrategy_NotEnoughMessages(t *testing.T) {
	s := &SlidingWindowStrategy{}
	config := DefaultConfig()

	// Create fewer messages than MinMessagesToSummarize
	messages := make([]agent.Message, config.MinMessagesToSummarize-1)
	for i := range messages {
		messages[i] = makeTextMessage(agent.RoleUser, "message")
	}

	selection := s.SelectForSummarization(messages, config)

	if len(selection.ToSummarize) != 0 {
		t.Errorf("expected 0 messages to summarize, got %d", len(selection.ToSummarize))
	}
	if len(selection.ToPreserve) != len(messages) {
		t.Errorf("expected all messages preserved, got %d", len(selection.ToPreserve))
	}
}

func TestSlidingWindowStrategy_BasicSplit(t *testing.T) {
	s := &SlidingWindowStrategy{}
	config := DefaultConfig()
	config.PreserveRecentCount = 3
	config.MinMessagesToSummarize = 4

	// Create 10 messages
	messages := make([]agent.Message, 10)
	for i := range messages {
		messages[i] = makeTextMessage(agent.RoleUser, "message")
	}

	selection := s.SelectForSummarization(messages, config)

	// Should preserve last 3, summarize first 7
	if len(selection.ToPreserve) != 3 {
		t.Errorf("expected 3 messages preserved, got %d", len(selection.ToPreserve))
	}
	if len(selection.ToSummarize) != 7 {
		t.Errorf("expected 7 messages to summarize, got %d", len(selection.ToSummarize))
	}

	// Verify indices
	if selection.ToPreserveIndices[0] != 7 {
		t.Errorf("expected first preserved index to be 7, got %d", selection.ToPreserveIndices[0])
	}
	if selection.ToSummarizeIndices[0] != 0 {
		t.Errorf("expected first summarize index to be 0, got %d", selection.ToSummarizeIndices[0])
	}
}

func TestSlidingWindowStrategy_PreserveAllWhenPreserveCountExceedsMessages(t *testing.T) {
	s := &SlidingWindowStrategy{}
	config := DefaultConfig()
	config.PreserveRecentCount = 20
	config.MinMessagesToSummarize = 4

	messages := make([]agent.Message, 10)
	for i := range messages {
		messages[i] = makeTextMessage(agent.RoleUser, "message")
	}

	selection := s.SelectForSummarization(messages, config)

	if len(selection.ToSummarize) != 0 {
		t.Errorf("expected 0 messages to summarize, got %d", len(selection.ToSummarize))
	}
	if len(selection.ToPreserve) != 10 {
		t.Errorf("expected all 10 messages preserved, got %d", len(selection.ToPreserve))
	}
}

func TestSlidingWindowStrategy_TokenEstimates(t *testing.T) {
	s := &SlidingWindowStrategy{}
	config := DefaultConfig()
	config.PreserveRecentCount = 2
	config.MinMessagesToSummarize = 4

	messages := []agent.Message{
		makeTextMessage(agent.RoleUser, "Short message"),
		makeTextMessage(agent.RoleAssistant, "This is a longer response that contains more content"),
		makeTextMessage(agent.RoleUser, "Another short one"),
		makeTextMessage(agent.RoleAssistant, "Reply"),
		makeTextMessage(agent.RoleUser, "Final message"),
	}

	selection := s.SelectForSummarization(messages, config)

	if selection.EstimatedTokensToSummarize <= 0 {
		t.Error("expected positive token estimate for messages to summarize")
	}
	if selection.EstimatedTokensToPreserve <= 0 {
		t.Error("expected positive token estimate for preserved messages")
	}
}

func TestPhaseAwareStrategy_Name(t *testing.T) {
	s := &PhaseAwareSummaryStrategy{}
	if s.Name() != "phase_aware" {
		t.Errorf("expected name 'phase_aware', got %s", s.Name())
	}
}

func TestPhaseAwareStrategy_FallbackWithoutPhaseBoundary(t *testing.T) {
	s := &PhaseAwareSummaryStrategy{}
	config := DefaultConfig()
	config.PreserveRecentCount = 3
	config.MinMessagesToSummarize = 4
	config.SummarizeOnPhaseBoundary = false

	messages := make([]agent.Message, 10)
	for i := range messages {
		messages[i] = makeTextMessage(agent.RoleUser, "message")
	}

	selection := s.SelectForSummarization(messages, config)

	// Should behave like sliding window
	if len(selection.ToPreserve) != 3 {
		t.Errorf("expected 3 messages preserved, got %d", len(selection.ToPreserve))
	}
}

func TestPhaseAwareStrategy_SplitAtPhaseBoundary(t *testing.T) {
	s := &PhaseAwareSummaryStrategy{}
	config := DefaultConfig()
	config.PreserveRecentCount = 2
	config.MinMessagesToSummarize = 4
	config.SummarizeOnPhaseBoundary = true

	messages := []agent.Message{
		makeTextMessage(agent.RoleUser, "Start task"),
		makeTextMessage(agent.RoleAssistant, "Working on it"),
		makeTextMessage(agent.RoleUser, "Continue"),
		makeTextMessage(agent.RoleAssistant, "Task completed successfully. ## Summary - Done"), // Phase boundary
		makeTextMessage(agent.RoleUser, "New task"),
		makeTextMessage(agent.RoleAssistant, "Starting new task"),
		makeTextMessage(agent.RoleUser, "More work"),
		makeTextMessage(agent.RoleAssistant, "Continuing"),
	}

	selection := s.SelectForSummarization(messages, config)

	// Should prefer to split at the phase boundary (after message 3)
	// rather than just preserving last 2
	if len(selection.ToSummarize) < 3 {
		t.Errorf("expected at least 3 messages to summarize (up to phase boundary), got %d", len(selection.ToSummarize))
	}
}

func TestDefaultStrategy(t *testing.T) {
	strategy := DefaultStrategy()
	if strategy == nil {
		t.Fatal("DefaultStrategy returned nil")
	}
	if strategy.Name() != "phase_aware" {
		t.Errorf("expected default strategy to be phase_aware, got %s", strategy.Name())
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		text     string
		patterns []string
		expected bool
	}{
		{"This is a test", []string{"test"}, true},
		{"This is a test", []string{"foo", "bar"}, false},
		{"Task completed successfully", []string{"completed", "failed"}, true},
		{"## Summary of work", []string{"## Summary"}, true},
		{"", []string{"anything"}, false},
		{"text", []string{""}, true},
	}

	for _, tt := range tests {
		result := containsAny(tt.text, tt.patterns)
		if result != tt.expected {
			t.Errorf("containsAny(%q, %v) = %v, expected %v", tt.text, tt.patterns, result, tt.expected)
		}
	}
}

func TestMakeRange(t *testing.T) {
	tests := []struct {
		start    int
		end      int
		expected []int
	}{
		{0, 5, []int{0, 1, 2, 3, 4}},
		{3, 6, []int{3, 4, 5}},
		{0, 0, []int{}},
		{5, 5, []int{}},
	}

	for _, tt := range tests {
		result := makeRange(tt.start, tt.end)
		if len(result) != len(tt.expected) {
			t.Errorf("makeRange(%d, %d) length = %d, expected %d", tt.start, tt.end, len(result), len(tt.expected))
			continue
		}
		for i, v := range result {
			if v != tt.expected[i] {
				t.Errorf("makeRange(%d, %d)[%d] = %d, expected %d", tt.start, tt.end, i, v, tt.expected[i])
			}
		}
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		slice    []int
		val      int
		expected bool
	}{
		{[]int{1, 2, 3}, 2, true},
		{[]int{1, 2, 3}, 4, false},
		{[]int{}, 1, false},
		{[]int{5}, 5, true},
	}

	for _, tt := range tests {
		result := contains(tt.slice, tt.val)
		if result != tt.expected {
			t.Errorf("contains(%v, %d) = %v, expected %v", tt.slice, tt.val, result, tt.expected)
		}
	}
}

// Additional edge case tests

func TestSlidingWindowStrategy_ToolCallPreservation(t *testing.T) {
	s := &SlidingWindowStrategy{}
	config := DefaultConfig()
	config.PreserveRecentCount = 2
	config.MinMessagesToSummarize = 4

	// Create a sequence with tool call/result that spans the split boundary
	messages := []agent.Message{
		makeTextMessage(agent.RoleUser, "First message"),
		makeTextMessage(agent.RoleAssistant, "Second message"),
		makeTextMessage(agent.RoleUser, "Third message"),
		// Tool call sequence that could be split
		makeToolUseMessage(agent.RoleAssistant, "read_file", []byte(`{"path": "/test.go"}`)),
		makeToolResultMessage(agent.RoleUser, "File content"),
		makeTextMessage(agent.RoleAssistant, "Based on the file..."),
		makeTextMessage(agent.RoleUser, "Final message"),
	}

	selection := s.SelectForSummarization(messages, config)

	// The strategy should not split between tool_use and tool_result
	// Verify selection is valid
	if selection == nil {
		t.Fatal("expected selection, got nil")
	}
	if len(selection.ToSummarize)+len(selection.ToPreserve) != len(messages) {
		t.Errorf("expected total messages %d, got %d", len(messages), len(selection.ToSummarize)+len(selection.ToPreserve))
	}
}

func TestSlidingWindowStrategy_CriticalSystemMessages(t *testing.T) {
	s := &SlidingWindowStrategy{}
	config := DefaultConfig()
	config.PreserveRecentCount = 2
	config.MinMessagesToSummarize = 4

	// Create messages with a critical system message early in the conversation
	messages := []agent.Message{
		{
			Role: agent.RoleSystem,
			Content: []agent.ContentBlock{
				{Type: agent.ContentTypeText, Text: "## Task\nYou are implementing a new feature. IMPORTANT: Use TypeScript."},
			},
		},
		makeTextMessage(agent.RoleUser, "Start working"),
		makeTextMessage(agent.RoleAssistant, "Working on it"),
		makeTextMessage(agent.RoleUser, "Continue"),
		makeTextMessage(agent.RoleAssistant, "Progress made"),
		makeTextMessage(agent.RoleUser, "More work"),
		makeTextMessage(agent.RoleAssistant, "Almost done"),
		makeTextMessage(agent.RoleUser, "Final"),
	}

	selection := s.SelectForSummarization(messages, config)

	// The critical system message should be preserved
	found := false
	for _, msg := range selection.ToPreserve {
		if msg.Role == agent.RoleSystem {
			for _, block := range msg.Content {
				if block.Type == agent.ContentTypeText && containsAny(block.Text, []string{"## Task", "IMPORTANT:"}) {
					found = true
					break
				}
			}
		}
	}
	if !found {
		t.Error("expected critical system message to be preserved")
	}
}

func TestSlidingWindowStrategy_EmptyMessages(t *testing.T) {
	s := &SlidingWindowStrategy{}
	config := DefaultConfig()

	messages := []agent.Message{}
	selection := s.SelectForSummarization(messages, config)

	if len(selection.ToSummarize) != 0 {
		t.Errorf("expected 0 messages to summarize for empty input, got %d", len(selection.ToSummarize))
	}
	if len(selection.ToPreserve) != 0 {
		t.Errorf("expected 0 messages to preserve for empty input, got %d", len(selection.ToPreserve))
	}
}

func TestSlidingWindowStrategy_ExactlyMinMessages(t *testing.T) {
	s := &SlidingWindowStrategy{}
	config := DefaultConfig()
	config.MinMessagesToSummarize = 6
	config.PreserveRecentCount = 3

	// Exactly the minimum number of messages
	messages := make([]agent.Message, config.MinMessagesToSummarize)
	for i := range messages {
		messages[i] = makeTextMessage(agent.RoleUser, "message")
	}

	selection := s.SelectForSummarization(messages, config)

	// Should summarize some messages since we have exactly the minimum
	if len(selection.ToSummarize) == 0 {
		t.Error("expected some messages to be summarized with exactly min messages")
	}
	if len(selection.ToPreserve) < config.PreserveRecentCount {
		t.Errorf("expected at least %d messages preserved, got %d", config.PreserveRecentCount, len(selection.ToPreserve))
	}
}

func TestPhaseAwareStrategy_MultiplePhaseEndMarkers(t *testing.T) {
	s := &PhaseAwareSummaryStrategy{}
	config := DefaultConfig()
	config.PreserveRecentCount = 2
	config.MinMessagesToSummarize = 4
	config.SummarizeOnPhaseBoundary = true

	messages := []agent.Message{
		makeTextMessage(agent.RoleUser, "Phase 1 task"),
		makeTextMessage(agent.RoleAssistant, "phase-complete for phase 1"), // First boundary
		makeTextMessage(agent.RoleUser, "Phase 2 task"),
		makeTextMessage(agent.RoleAssistant, "Working on phase 2"),
		makeTextMessage(agent.RoleAssistant, "task completed for phase 2"), // Second boundary
		makeTextMessage(agent.RoleUser, "Phase 3 task"),
		makeTextMessage(agent.RoleAssistant, "Starting phase 3"),
		makeTextMessage(agent.RoleUser, "Continue phase 3"),
	}

	selection := s.SelectForSummarization(messages, config)

	// Should use the most recent boundary that's in the summarize range
	if selection == nil {
		t.Fatal("expected selection, got nil")
	}
	if len(selection.ToSummarize) < 1 {
		t.Error("expected at least some messages to summarize")
	}
}

func TestPhaseAwareStrategy_NoBoundaryInRange(t *testing.T) {
	s := &PhaseAwareSummaryStrategy{}
	config := DefaultConfig()
	config.PreserveRecentCount = 2
	config.MinMessagesToSummarize = 4
	config.SummarizeOnPhaseBoundary = true

	// No phase boundary markers
	messages := []agent.Message{
		makeTextMessage(agent.RoleUser, "Message 1"),
		makeTextMessage(agent.RoleAssistant, "Response 1"),
		makeTextMessage(agent.RoleUser, "Message 2"),
		makeTextMessage(agent.RoleAssistant, "Response 2"),
		makeTextMessage(agent.RoleUser, "Message 3"),
		makeTextMessage(agent.RoleAssistant, "Response 3"),
	}

	selection := s.SelectForSummarization(messages, config)

	// Should fall back to sliding window behavior
	if selection == nil {
		t.Fatal("expected selection, got nil")
	}
	// Basic sliding window split: 6 messages - 2 preserve = 4 to summarize
	if len(selection.ToPreserve) != 2 {
		t.Errorf("expected 2 messages preserved (sliding window), got %d", len(selection.ToPreserve))
	}
}

func TestPhaseAwareStrategy_BoundaryAtEnd(t *testing.T) {
	s := &PhaseAwareSummaryStrategy{}
	config := DefaultConfig()
	config.PreserveRecentCount = 2
	config.MinMessagesToSummarize = 4
	config.SummarizeOnPhaseBoundary = true

	// Phase boundary at the very end (in preserve zone)
	messages := []agent.Message{
		makeTextMessage(agent.RoleUser, "Message 1"),
		makeTextMessage(agent.RoleAssistant, "Response 1"),
		makeTextMessage(agent.RoleUser, "Message 2"),
		makeTextMessage(agent.RoleAssistant, "Response 2"),
		makeTextMessage(agent.RoleUser, "Message 3"),
		makeTextMessage(agent.RoleAssistant, "## Summary - task completed"), // Boundary in preserve zone
	}

	selection := s.SelectForSummarization(messages, config)

	// Boundary in preserve zone shouldn't change split point
	if selection == nil {
		t.Fatal("expected selection, got nil")
	}
	if len(selection.ToPreserve) < 2 {
		t.Errorf("expected at least 2 messages preserved, got %d", len(selection.ToPreserve))
	}
}

func TestSlidingWindowStrategy_AllMessageTypes(t *testing.T) {
	s := &SlidingWindowStrategy{}
	config := DefaultConfig()
	config.PreserveRecentCount = 2
	config.MinMessagesToSummarize = 4

	// Mix of all message types
	messages := []agent.Message{
		{
			Role: agent.RoleSystem,
			Content: []agent.ContentBlock{
				{Type: agent.ContentTypeText, Text: "System message"},
			},
		},
		makeTextMessage(agent.RoleUser, "User text"),
		makeTextMessage(agent.RoleAssistant, "Assistant text"),
		makeToolUseMessage(agent.RoleAssistant, "test_tool", []byte(`{}`)),
		makeToolResultMessage(agent.RoleUser, "Tool result"),
		makeTextMessage(agent.RoleUser, "Final user message"),
	}

	selection := s.SelectForSummarization(messages, config)

	// Should handle all message types without panic
	if selection == nil {
		t.Fatal("expected selection, got nil")
	}

	total := len(selection.ToSummarize) + len(selection.ToPreserve)
	if total != len(messages) {
		t.Errorf("expected total %d messages, got %d", len(messages), total)
	}
}

func TestSlidingWindowStrategy_LargeMessageSet(t *testing.T) {
	s := &SlidingWindowStrategy{}
	config := DefaultConfig()
	config.PreserveRecentCount = 10
	config.MinMessagesToSummarize = 4

	// Create a large number of messages
	messages := make([]agent.Message, 100)
	for i := range messages {
		role := agent.RoleUser
		if i%2 == 1 {
			role = agent.RoleAssistant
		}
		messages[i] = makeTextMessage(role, "Message content for testing large sets")
	}

	selection := s.SelectForSummarization(messages, config)

	if selection == nil {
		t.Fatal("expected selection, got nil")
	}
	if len(selection.ToPreserve) != config.PreserveRecentCount {
		t.Errorf("expected %d messages preserved, got %d", config.PreserveRecentCount, len(selection.ToPreserve))
	}
	if len(selection.ToSummarize) != 90 {
		t.Errorf("expected 90 messages to summarize, got %d", len(selection.ToSummarize))
	}

	// Verify token estimates are calculated
	if selection.EstimatedTokensToSummarize <= 0 {
		t.Error("expected positive token estimate for messages to summarize")
	}
}

func TestPhaseAwareStrategy_IsPhaseEndVariations(t *testing.T) {
	s := &PhaseAwareSummaryStrategy{}

	testCases := []struct {
		text     string
		expected bool
	}{
		{"phase-complete", true},
		{"task completed", true},
		{"## Summary", true},
		{"completed successfully", true},
		{"Working on the task", false},
		{"Starting the implementation", false},
		{"", false},
		{"Some other text", false},
		{"PHASE-COMPLETE", false}, // Case sensitive
	}

	for _, tc := range testCases {
		msg := makeTextMessage(agent.RoleAssistant, tc.text)
		result := s.isPhaseEnd(msg)
		if result != tc.expected {
			t.Errorf("isPhaseEnd(%q) = %v, expected %v", tc.text, result, tc.expected)
		}
	}
}

func TestPhaseAwareStrategy_IsPhaseEndNonTextBlock(t *testing.T) {
	s := &PhaseAwareSummaryStrategy{}

	// Tool use message should not be considered phase end
	msg := makeToolUseMessage(agent.RoleAssistant, "some_tool", []byte(`{}`))
	if s.isPhaseEnd(msg) {
		t.Error("tool use message should not be considered phase end")
	}

	// Tool result message should not be considered phase end
	msg = makeToolResultMessage(agent.RoleUser, "phase-complete in tool result")
	if s.isPhaseEnd(msg) {
		t.Error("tool result message should not be considered phase end")
	}
}

func TestStrategyInterface(t *testing.T) {
	// Verify that both strategies implement the Strategy interface
	var _ Strategy = &SlidingWindowStrategy{}
	var _ Strategy = &PhaseAwareSummaryStrategy{}
	var _ Strategy = DefaultStrategy()
}

func TestFindCriticalSystemMessages_Variations(t *testing.T) {
	s := &SlidingWindowStrategy{}

	testCases := []struct {
		name         string
		text         string
		shouldBeCrit bool
	}{
		{"task marker", "## Task\nDo something", true},
		{"instructions marker", "## Instructions\nFollow these", true},
		{"context marker", "## Context\nThe project is...", true},
		{"important marker", "IMPORTANT: Don't forget this", true},
		{"constraint marker", "CONSTRAINT: Must use TypeScript", true},
		{"requirement marker", "REQUIREMENT: Must be async", true},
		{"regular message", "Just a normal system message", false},
		{"empty message", "", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			messages := []agent.Message{
				{
					Role: agent.RoleSystem,
					Content: []agent.ContentBlock{
						{Type: agent.ContentTypeText, Text: tc.text},
					},
				},
			}

			critical := s.findCriticalSystemMessages(messages)
			found := len(critical) > 0

			if found != tc.shouldBeCrit {
				t.Errorf("expected critical=%v for text %q, got %v", tc.shouldBeCrit, tc.text, found)
			}
		})
	}
}

func TestFindCriticalSystemMessages_OnlySystemRole(t *testing.T) {
	s := &SlidingWindowStrategy{}

	// Non-system messages with critical markers should not be preserved
	messages := []agent.Message{
		makeTextMessage(agent.RoleUser, "## Task: Do something"),
		makeTextMessage(agent.RoleAssistant, "IMPORTANT: Key info"),
	}

	critical := s.findCriticalSystemMessages(messages)
	if len(critical) != 0 {
		t.Errorf("expected no critical messages (only system role counts), got %d", len(critical))
	}
}

func TestResplitAtBoundary(t *testing.T) {
	s := &PhaseAwareSummaryStrategy{}
	config := DefaultConfig()
	config.PreserveRecentCount = 2
	config.MinMessagesToSummarize = 4

	messages := []agent.Message{
		makeTextMessage(agent.RoleUser, "Message 1"),
		makeTextMessage(agent.RoleAssistant, "Response 1"),
		makeTextMessage(agent.RoleUser, "Message 2"),
		makeTextMessage(agent.RoleAssistant, "Response 2"),
		makeTextMessage(agent.RoleUser, "Message 3"),
		makeTextMessage(agent.RoleAssistant, "Response 3"),
	}

	// Split at boundary index 3 (after 3 messages)
	selection := s.resplitAtBoundary(messages, 3, config)

	if selection == nil {
		t.Fatal("expected selection, got nil")
	}
	if len(selection.ToSummarize) != 3 {
		t.Errorf("expected 3 messages to summarize, got %d", len(selection.ToSummarize))
	}
	if len(selection.ToPreserve) != 3 {
		t.Errorf("expected 3 messages to preserve, got %d", len(selection.ToPreserve))
	}
}

func TestResplitAtBoundary_BoundaryExceedsMinPreserve(t *testing.T) {
	s := &PhaseAwareSummaryStrategy{}
	config := DefaultConfig()
	config.PreserveRecentCount = 3
	config.MinMessagesToSummarize = 4

	messages := []agent.Message{
		makeTextMessage(agent.RoleUser, "Message 1"),
		makeTextMessage(agent.RoleAssistant, "Response 1"),
		makeTextMessage(agent.RoleUser, "Message 2"),
		makeTextMessage(agent.RoleAssistant, "Response 2"),
		makeTextMessage(agent.RoleUser, "Message 3"),
		makeTextMessage(agent.RoleAssistant, "Response 3"),
	}

	// Try to split at index 5 (preserving only 1), but minPreserve is 3
	selection := s.resplitAtBoundary(messages, 5, config)

	// Should adjust to preserve at least 3 messages
	if len(selection.ToPreserve) < config.PreserveRecentCount {
		t.Errorf("expected at least %d messages preserved, got %d", config.PreserveRecentCount, len(selection.ToPreserve))
	}
}
