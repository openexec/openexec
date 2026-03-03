package summarize

import (
	"fmt"
	"strings"

	"github.com/openexec/openexec/pkg/agent"
)

// defaultPromptBuilder implements PromptBuilder with carefully crafted prompts.
type defaultPromptBuilder struct{}

// NewPromptBuilder creates a new default prompt builder.
func NewPromptBuilder() PromptBuilder {
	return &defaultPromptBuilder{}
}

// BuildSummarizationPrompt creates the prompt for generating a summary of conversation history.
func (b *defaultPromptBuilder) BuildSummarizationPrompt(data *SummaryPromptData) string {
	var sb strings.Builder

	sb.WriteString("You are a conversation summarizer for an AI coding assistant. Your task is to create a concise summary of the conversation history that preserves essential context for continuing the work.\n\n")

	sb.WriteString("## Guidelines\n\n")

	sb.WriteString("1. **Focus on Outcomes**: Emphasize what was accomplished, decided, or discovered.\n")
	sb.WriteString("2. **Preserve Key References**: Retain file paths, function names, and specific code locations mentioned.\n")
	sb.WriteString("3. **Capture Decisions**: Note any architectural decisions, trade-offs, or constraints established.\n")
	sb.WriteString("4. **Summarize Tool Actions**: Briefly note which files were read, written, or commands executed.\n")
	sb.WriteString("5. **Retain Error Context**: Summarize any errors encountered and how they were resolved.\n")
	sb.WriteString("6. **Keep User Preferences**: Preserve any explicit user preferences or requirements stated.\n")
	sb.WriteString("7. **Be Concise**: Aim for maximum information density. Avoid redundancy.\n\n")

	sb.WriteString(fmt.Sprintf("## Target Length\n\nAim for approximately %d tokens (roughly %d words).\n\n",
		data.TargetTokens, data.TargetTokens*3/4))

	// Include specific sections based on configuration
	sb.WriteString("## Summary Structure\n\n")
	sb.WriteString("Structure your summary with these sections:\n\n")
	sb.WriteString("1. **Task Context**: What is the overall goal/task being worked on?\n")
	sb.WriteString("2. **Progress**: What has been accomplished so far?\n")
	sb.WriteString("3. **Current State**: What is the current state of the work?\n")

	if data.IncludeToolCalls {
		sb.WriteString("4. **Key Actions**: Important files modified, commands run, or tools used.\n")
	}

	if data.PreserveCodeRefs {
		sb.WriteString("5. **Code References**: Important file paths, functions, or code patterns discussed.\n")
	}

	if data.PreservePreferences {
		sb.WriteString("6. **Constraints**: Any user preferences, constraints, or requirements to remember.\n")
	}

	sb.WriteString("7. **Next Steps**: What needs to be done next (if mentioned).\n\n")

	// Include previous summary if incremental
	if data.PreviousSummary != "" {
		sb.WriteString("## Previous Summary Context\n\n")
		sb.WriteString("Build upon this existing summary (don't repeat it, extend it):\n\n")
		sb.WriteString("```\n")
		sb.WriteString(data.PreviousSummary)
		sb.WriteString("\n```\n\n")
	}

	sb.WriteString("## Conversation to Summarize\n\n")
	sb.WriteString(formatMessagesForSummary(data.Messages))

	sb.WriteString("\n\n## Your Summary\n\nProvide a well-structured summary following the guidelines above:")

	return sb.String()
}

// BuildIncrementalPrompt creates the prompt for adding to an existing summary.
func (b *defaultPromptBuilder) BuildIncrementalPrompt(data *SummaryPromptData) string {
	var sb strings.Builder

	sb.WriteString("You are updating a conversation summary for an AI coding assistant. Your task is to integrate new conversation content into an existing summary.\n\n")

	sb.WriteString("## Guidelines\n\n")

	sb.WriteString("1. **Integrate, Don't Append**: Merge new information into the appropriate sections.\n")
	sb.WriteString("2. **Update Progress**: Reflect any new accomplishments or changes in state.\n")
	sb.WriteString("3. **Maintain Structure**: Keep the same organizational structure as the original.\n")
	sb.WriteString("4. **Remove Obsolete Info**: If new content supersedes old information, update accordingly.\n")
	sb.WriteString("5. **Preserve Important Context**: Don't lose critical context from the original summary.\n")
	sb.WriteString("6. **Stay Concise**: The updated summary should not be significantly longer than the original.\n\n")

	sb.WriteString(fmt.Sprintf("## Target Length\n\nAim for approximately %d tokens (roughly %d words).\n\n",
		data.TargetTokens, data.TargetTokens*3/4))

	sb.WriteString("## Existing Summary\n\n")
	sb.WriteString("```\n")
	sb.WriteString(data.PreviousSummary)
	sb.WriteString("\n```\n\n")

	sb.WriteString("## New Conversation Content\n\n")
	sb.WriteString(formatMessagesForSummary(data.Messages))

	sb.WriteString("\n\n## Updated Summary\n\nProvide the updated summary that incorporates the new content:")

	return sb.String()
}

// formatMessagesForSummary formats messages for inclusion in a summarization prompt.
func formatMessagesForSummary(messages []agent.Message) string {
	var sb strings.Builder

	for i, msg := range messages {
		// Add message header
		switch msg.Role {
		case agent.RoleUser:
			sb.WriteString(fmt.Sprintf("[USER %d]\n", i+1))
		case agent.RoleAssistant:
			sb.WriteString(fmt.Sprintf("[ASSISTANT %d]\n", i+1))
		case agent.RoleSystem:
			sb.WriteString(fmt.Sprintf("[SYSTEM %d]\n", i+1))
		default:
			sb.WriteString(fmt.Sprintf("[%s %d]\n", strings.ToUpper(string(msg.Role)), i+1))
		}

		// Extract text content from message
		for _, block := range msg.Content {
			switch block.Type {
			case agent.ContentTypeText:
				sb.WriteString(block.Text)
				sb.WriteString("\n")
			case agent.ContentTypeToolUse:
				sb.WriteString(fmt.Sprintf("<tool_call name=\"%s\">\n", block.ToolName))
				// Truncate long tool inputs
				input := fmt.Sprintf("%v", block.ToolInput)
				if len(input) > 500 {
					input = input[:500] + "...[truncated]"
				}
				sb.WriteString(input)
				sb.WriteString("\n</tool_call>\n")
			case agent.ContentTypeToolResult:
				sb.WriteString("<tool_result>\n")
				// Truncate long tool outputs
				output := block.ToolOutput
				if len(output) > 500 {
					output = output[:500] + "...[truncated]"
				}
				sb.WriteString(output)
				sb.WriteString("\n</tool_result>\n")
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// ExtractKeyReferences extracts file paths, function names, and other references from text.
func ExtractKeyReferences(text string) []string {
	var refs []string

	// Common patterns for file paths (simplified - production would use regex)
	// This is a placeholder for the actual implementation
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for patterns that look like file paths
		if strings.Contains(line, "/") && (strings.HasSuffix(line, ".go") ||
			strings.HasSuffix(line, ".py") ||
			strings.HasSuffix(line, ".js") ||
			strings.HasSuffix(line, ".ts") ||
			strings.HasSuffix(line, ".tsx") ||
			strings.HasSuffix(line, ".md")) {
			// Extract the path portion
			refs = append(refs, line)
		}
	}

	return refs
}

// EstimateTokenCount provides a rough token count estimate.
// This is a simplified estimation - production would use a proper tokenizer.
func EstimateTokenCount(text string) int {
	// Rough estimate: ~4 characters per token for English text
	// This is a common approximation used by many systems
	return len(text) / 4
}

// EstimateMessagesTokenCount estimates total tokens in a slice of messages.
func EstimateMessagesTokenCount(messages []agent.Message) int {
	total := 0
	for _, msg := range messages {
		for _, block := range msg.Content {
			switch block.Type {
			case agent.ContentTypeText:
				total += EstimateTokenCount(block.Text)
			case agent.ContentTypeToolUse:
				total += EstimateTokenCount(block.ToolName)
				total += EstimateTokenCount(fmt.Sprintf("%v", block.ToolInput))
			case agent.ContentTypeToolResult:
				total += EstimateTokenCount(block.ToolOutput)
			}
		}
		// Add overhead for message structure
		total += 4 // Role and formatting tokens
	}
	return total
}
