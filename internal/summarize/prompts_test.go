package summarize

import (
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

func TestNewPromptBuilder(t *testing.T) {
	builder := NewPromptBuilder()
	if builder == nil {
		t.Fatal("expected prompt builder, got nil")
	}
}

func TestBuildSummarizationPrompt_BasicContent(t *testing.T) {
	builder := NewPromptBuilder()

	data := &SummaryPromptData{
		Messages: []agent.Message{
			agent.NewTextMessage(agent.RoleUser, "Help me implement a feature"),
			agent.NewTextMessage(agent.RoleAssistant, "I'll help you with that feature"),
		},
		TargetTokens:        2000,
		IncludeToolCalls:    true,
		PreserveCodeRefs:    true,
		PreservePreferences: true,
	}

	prompt := builder.BuildSummarizationPrompt(data)

	// Check for required sections
	requiredSections := []string{
		"You are a conversation summarizer",
		"## Guidelines",
		"## Target Length",
		"## Summary Structure",
		"## Conversation to Summarize",
		"## Your Summary",
	}

	for _, section := range requiredSections {
		if !strings.Contains(prompt, section) {
			t.Errorf("expected prompt to contain %q", section)
		}
	}

	// Check target tokens is mentioned
	if !strings.Contains(prompt, "2000") {
		t.Error("expected prompt to mention target tokens")
	}

	// Check that message content is included
	if !strings.Contains(prompt, "[USER 1]") {
		t.Error("expected prompt to contain user message marker")
	}
	if !strings.Contains(prompt, "[ASSISTANT 2]") {
		t.Error("expected prompt to contain assistant message marker")
	}
}

func TestBuildSummarizationPrompt_WithPreviousSummary(t *testing.T) {
	builder := NewPromptBuilder()

	data := &SummaryPromptData{
		Messages: []agent.Message{
			agent.NewTextMessage(agent.RoleUser, "Continue the work"),
		},
		PreviousSummary:     "Previous summary about implementing auth feature.",
		TargetTokens:        2000,
		IncludeToolCalls:    true,
		PreserveCodeRefs:    true,
		PreservePreferences: true,
	}

	prompt := builder.BuildSummarizationPrompt(data)

	// Check for previous summary section
	if !strings.Contains(prompt, "## Previous Summary Context") {
		t.Error("expected prompt to contain previous summary section")
	}
	if !strings.Contains(prompt, "Previous summary about implementing auth feature.") {
		t.Error("expected prompt to contain the previous summary text")
	}
	if !strings.Contains(prompt, "Build upon this existing summary") {
		t.Error("expected prompt to instruct building upon existing summary")
	}
}

func TestBuildSummarizationPrompt_OptionFlags(t *testing.T) {
	builder := NewPromptBuilder()

	t.Run("with all options enabled", func(t *testing.T) {
		data := &SummaryPromptData{
			Messages: []agent.Message{
				agent.NewTextMessage(agent.RoleUser, "Test message"),
			},
			TargetTokens:        2000,
			IncludeToolCalls:    true,
			PreserveCodeRefs:    true,
			PreservePreferences: true,
		}

		prompt := builder.BuildSummarizationPrompt(data)

		if !strings.Contains(prompt, "Key Actions") {
			t.Error("expected prompt to contain Key Actions section when IncludeToolCalls is true")
		}
		if !strings.Contains(prompt, "Code References") {
			t.Error("expected prompt to contain Code References section when PreserveCodeRefs is true")
		}
		if !strings.Contains(prompt, "Constraints") {
			t.Error("expected prompt to contain Constraints section when PreservePreferences is true")
		}
	})

	t.Run("with options disabled", func(t *testing.T) {
		data := &SummaryPromptData{
			Messages: []agent.Message{
				agent.NewTextMessage(agent.RoleUser, "Test message"),
			},
			TargetTokens:        2000,
			IncludeToolCalls:    false,
			PreserveCodeRefs:    false,
			PreservePreferences: false,
		}

		prompt := builder.BuildSummarizationPrompt(data)

		// Note: The prompt still contains "Next Steps" which is always included
		// But these specific conditional sections should not appear
		if strings.Contains(prompt, "4. **Key Actions**") {
			t.Error("expected prompt to NOT contain Key Actions when IncludeToolCalls is false")
		}
		if strings.Contains(prompt, "5. **Code References**") {
			t.Error("expected prompt to NOT contain Code References when PreserveCodeRefs is false")
		}
		if strings.Contains(prompt, "6. **Constraints**") {
			t.Error("expected prompt to NOT contain Constraints when PreservePreferences is false")
		}
	})
}

func TestBuildIncrementalPrompt(t *testing.T) {
	builder := NewPromptBuilder()

	data := &SummaryPromptData{
		Messages: []agent.Message{
			agent.NewTextMessage(agent.RoleUser, "New user message"),
			agent.NewTextMessage(agent.RoleAssistant, "New assistant response"),
		},
		PreviousSummary:     "This is the existing summary of earlier conversation.",
		TargetTokens:        2000,
		IncludeToolCalls:    true,
		PreserveCodeRefs:    true,
		PreservePreferences: true,
	}

	prompt := builder.BuildIncrementalPrompt(data)

	// Check for required sections
	requiredSections := []string{
		"You are updating a conversation summary",
		"## Guidelines",
		"## Target Length",
		"## Existing Summary",
		"## New Conversation Content",
		"## Updated Summary",
	}

	for _, section := range requiredSections {
		if !strings.Contains(prompt, section) {
			t.Errorf("expected incremental prompt to contain %q", section)
		}
	}

	// Check that existing summary is included
	if !strings.Contains(prompt, "This is the existing summary of earlier conversation.") {
		t.Error("expected prompt to contain the existing summary")
	}

	// Check for incremental-specific guidelines
	if !strings.Contains(prompt, "Integrate, Don't Append") {
		t.Error("expected prompt to contain integration guideline")
	}
	if !strings.Contains(prompt, "Update Progress") {
		t.Error("expected prompt to contain update progress guideline")
	}
}

func TestFormatMessagesForSummary_TextMessages(t *testing.T) {
	messages := []agent.Message{
		agent.NewTextMessage(agent.RoleUser, "Hello, I need help"),
		agent.NewTextMessage(agent.RoleAssistant, "Sure, I can help you"),
		agent.NewTextMessage(agent.RoleSystem, "System instructions"),
	}

	result := formatMessagesForSummary(messages)

	// Check message headers
	if !strings.Contains(result, "[USER 1]") {
		t.Error("expected result to contain user header")
	}
	if !strings.Contains(result, "[ASSISTANT 2]") {
		t.Error("expected result to contain assistant header")
	}
	if !strings.Contains(result, "[SYSTEM 3]") {
		t.Error("expected result to contain system header")
	}

	// Check message content
	if !strings.Contains(result, "Hello, I need help") {
		t.Error("expected result to contain user message")
	}
	if !strings.Contains(result, "Sure, I can help you") {
		t.Error("expected result to contain assistant message")
	}
}

func TestFormatMessagesForSummary_ToolUseMessages(t *testing.T) {
	messages := []agent.Message{
		{
			Role: agent.RoleAssistant,
			Content: []agent.ContentBlock{
				{
					Type:      agent.ContentTypeToolUse,
					ToolUseID: "tool-123",
					ToolName:  "read_file",
					ToolInput: []byte(`{"path": "/some/file.go"}`),
				},
			},
		},
	}

	result := formatMessagesForSummary(messages)

	if !strings.Contains(result, "<tool_call name=\"read_file\">") {
		t.Error("expected result to contain tool call marker")
	}
	if !strings.Contains(result, "</tool_call>") {
		t.Error("expected result to contain tool call closing tag")
	}
}

func TestFormatMessagesForSummary_ToolResultMessages(t *testing.T) {
	messages := []agent.Message{
		{
			Role: agent.RoleUser,
			Content: []agent.ContentBlock{
				{
					Type:       agent.ContentTypeToolResult,
					ToolOutput: "File content here",
				},
			},
		},
	}

	result := formatMessagesForSummary(messages)

	if !strings.Contains(result, "<tool_result>") {
		t.Error("expected result to contain tool result opening tag")
	}
	if !strings.Contains(result, "File content here") {
		t.Error("expected result to contain tool output")
	}
	if !strings.Contains(result, "</tool_result>") {
		t.Error("expected result to contain tool result closing tag")
	}
}

func TestFormatMessagesForSummary_TruncateLongToolInput(t *testing.T) {
	// Create a long tool input
	longInput := strings.Repeat("x", 1000)
	messages := []agent.Message{
		{
			Role: agent.RoleAssistant,
			Content: []agent.ContentBlock{
				{
					Type:      agent.ContentTypeToolUse,
					ToolUseID: "tool-456",
					ToolName:  "write_file",
					ToolInput: []byte(longInput),
				},
			},
		},
	}

	result := formatMessagesForSummary(messages)

	// Check that truncation occurred
	if !strings.Contains(result, "...[truncated]") {
		t.Error("expected long tool input to be truncated")
	}
}

func TestFormatMessagesForSummary_TruncateLongToolOutput(t *testing.T) {
	// Create a long tool output
	longOutput := strings.Repeat("y", 1000)
	messages := []agent.Message{
		{
			Role: agent.RoleUser,
			Content: []agent.ContentBlock{
				{
					Type:       agent.ContentTypeToolResult,
					ToolOutput: longOutput,
				},
			},
		},
	}

	result := formatMessagesForSummary(messages)

	// Check that truncation occurred
	if !strings.Contains(result, "...[truncated]") {
		t.Error("expected long tool output to be truncated")
	}

	// The result should be much shorter than the original
	if len(result) > 600 {
		t.Errorf("expected truncated result to be shorter, got length %d", len(result))
	}
}

func TestFormatMessagesForSummary_UnknownRole(t *testing.T) {
	messages := []agent.Message{
		{
			Role: agent.Role("custom_role"),
			Content: []agent.ContentBlock{
				{Type: agent.ContentTypeText, Text: "Custom message"},
			},
		},
	}

	result := formatMessagesForSummary(messages)

	// Should handle unknown roles gracefully with uppercase formatting
	if !strings.Contains(result, "[CUSTOM_ROLE 1]") {
		t.Errorf("expected result to contain uppercase role header, got: %s", result)
	}
}

func TestFormatMessagesForSummary_EmptyMessages(t *testing.T) {
	messages := []agent.Message{}

	result := formatMessagesForSummary(messages)

	if result != "" {
		t.Errorf("expected empty result for empty messages, got: %s", result)
	}
}

func TestExtractKeyReferences(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int // number of expected references
	}{
		{
			name:     "go file path",
			text:     "/path/to/file.go",
			expected: 1,
		},
		{
			name:     "python file path",
			text:     "/home/user/script.py",
			expected: 1,
		},
		{
			name:     "javascript file path",
			text:     "/src/components/Button.js",
			expected: 1,
		},
		{
			name:     "typescript file path",
			text:     "/app/services/api.ts",
			expected: 1,
		},
		{
			name:     "tsx file path",
			text:     "/components/Header.tsx",
			expected: 1,
		},
		{
			name:     "markdown file path",
			text:     "/docs/README.md",
			expected: 1,
		},
		{
			name:     "multiple paths",
			text:     "/path/to/file1.go\n/another/file2.py",
			expected: 2,
		},
		{
			name:     "no paths",
			text:     "This is just regular text without file paths.",
			expected: 0,
		},
		{
			name:     "path without extension",
			text:     "/some/path/without/extension",
			expected: 0,
		},
		{
			name:     "unsupported extension",
			text:     "/some/path/file.xyz",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := ExtractKeyReferences(tt.text)
			if len(refs) != tt.expected {
				t.Errorf("expected %d references, got %d: %v", tt.expected, len(refs), refs)
			}
		})
	}
}

func TestEstimateTokenCount(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "empty string",
			text:     "",
			expected: 0,
		},
		{
			name:     "short text",
			text:     "Hello",
			expected: 1, // 5 chars / 4 = 1
		},
		{
			name:     "medium text",
			text:     "This is a test message for token counting.",
			expected: 10, // 42 chars / 4 = 10
		},
		{
			name:     "exact multiple",
			text:     "12345678", // 8 characters
			expected: 2,          // 8 / 4 = 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EstimateTokenCount(tt.text)
			if result != tt.expected {
				t.Errorf("expected %d tokens, got %d", tt.expected, result)
			}
		})
	}
}

func TestEstimateMessagesTokenCount(t *testing.T) {
	t.Run("empty messages", func(t *testing.T) {
		messages := []agent.Message{}
		result := EstimateMessagesTokenCount(messages)
		if result != 0 {
			t.Errorf("expected 0 tokens for empty messages, got %d", result)
		}
	})

	t.Run("text messages", func(t *testing.T) {
		messages := []agent.Message{
			agent.NewTextMessage(agent.RoleUser, "Hello world"), // 11 chars = 2 tokens + 4 overhead
		}
		result := EstimateMessagesTokenCount(messages)
		// Expected: 11/4 + 4 = 2 + 4 = 6 (but depends on calculation)
		if result < 4 {
			t.Errorf("expected at least 4 tokens, got %d", result)
		}
	})

	t.Run("tool use messages", func(t *testing.T) {
		messages := []agent.Message{
			{
				Role: agent.RoleAssistant,
				Content: []agent.ContentBlock{
					{
						Type:      agent.ContentTypeToolUse,
						ToolName:  "read_file",
						ToolInput: []byte(`{"path": "/test.go"}`),
					},
				},
			},
		}
		result := EstimateMessagesTokenCount(messages)
		if result <= 0 {
			t.Error("expected positive token count for tool use message")
		}
	})

	t.Run("tool result messages", func(t *testing.T) {
		messages := []agent.Message{
			{
				Role: agent.RoleUser,
				Content: []agent.ContentBlock{
					{
						Type:       agent.ContentTypeToolResult,
						ToolOutput: "File content here",
					},
				},
			},
		}
		result := EstimateMessagesTokenCount(messages)
		if result <= 0 {
			t.Error("expected positive token count for tool result message")
		}
	})

	t.Run("multiple messages", func(t *testing.T) {
		messages := []agent.Message{
			agent.NewTextMessage(agent.RoleUser, "First message"),
			agent.NewTextMessage(agent.RoleAssistant, "Second message"),
			agent.NewTextMessage(agent.RoleUser, "Third message"),
		}
		result := EstimateMessagesTokenCount(messages)
		// Each message should contribute at least overhead (4) tokens
		if result < 12 {
			t.Errorf("expected at least 12 tokens for 3 messages, got %d", result)
		}
	})
}

func TestPromptBuilderInterface(t *testing.T) {
	// Verify that defaultPromptBuilder implements PromptBuilder interface
	var _ PromptBuilder = &defaultPromptBuilder{}
	var _ PromptBuilder = NewPromptBuilder()
}
