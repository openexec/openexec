package summarize

import (
	"github.com/openexec/openexec/internal/agent"
)

// Strategy defines the algorithm for selecting which messages to summarize.
type Strategy interface {
	// Name returns the strategy name for logging/debugging.
	Name() string

	// SelectForSummarization determines which messages to summarize and which to preserve.
	SelectForSummarization(messages []agent.Message, config *Config) *MessageSelection
}

// SlidingWindowStrategy implements the "sliding window with summary prefix" approach.
// This is the recommended default strategy.
type SlidingWindowStrategy struct{}

// Name returns the strategy name.
func (s *SlidingWindowStrategy) Name() string {
	return "sliding_window"
}

// SelectForSummarization implements the sliding window selection algorithm.
//
// Algorithm:
// 1. Always preserve the most recent N messages (PreserveRecentCount)
// 2. Preserve any incomplete tool call sequences
// 3. Preserve critical system messages
// 4. Mark remaining older messages for summarization
//
// Visual representation:
//
//	Messages: [M1, M2, M3, M4, M5, M6, M7, M8, M9, M10, M11, M12]
//	                                              ^----- preserve_recent_count=3
//	          [-------- TO SUMMARIZE ----------][-- PRESERVE --]
func (s *SlidingWindowStrategy) SelectForSummarization(messages []agent.Message, config *Config) *MessageSelection {
	if len(messages) < config.MinMessagesToSummarize {
		// Not enough messages to summarize meaningfully
		return &MessageSelection{
			ToSummarize:        nil,
			ToPreserve:         messages,
			ToPreserveIndices:  makeRange(0, len(messages)),
			ToSummarizeIndices: nil,
		}
	}

	// Determine the split point
	preserveCount := config.PreserveRecentCount
	if preserveCount >= len(messages) {
		// Can't summarize if we need to preserve everything
		return &MessageSelection{
			ToSummarize:        nil,
			ToPreserve:         messages,
			ToPreserveIndices:  makeRange(0, len(messages)),
			ToSummarizeIndices: nil,
		}
	}

	splitIdx := len(messages) - preserveCount

	// Find incomplete tool call sequences in the "to summarize" portion
	// and extend preservation to include them
	splitIdx = s.adjustForToolCalls(messages, splitIdx, preserveCount)

	// Find critical system messages that should be preserved
	criticalIndices := s.findCriticalSystemMessages(messages[:splitIdx])

	// Build the selection result
	selection := &MessageSelection{
		ToSummarize:        make([]agent.Message, 0, splitIdx),
		ToPreserve:         make([]agent.Message, 0, len(messages)-splitIdx),
		ToSummarizeIndices: make([]int, 0, splitIdx),
		ToPreserveIndices:  make([]int, 0, len(messages)-splitIdx),
	}

	// Categorize messages
	for i, msg := range messages {
		if i >= splitIdx {
			// In the "preserve" window
			selection.ToPreserve = append(selection.ToPreserve, msg)
			selection.ToPreserveIndices = append(selection.ToPreserveIndices, i)
		} else if contains(criticalIndices, i) {
			// Critical message - preserve even though it's old
			selection.ToPreserve = append(selection.ToPreserve, msg)
			selection.ToPreserveIndices = append(selection.ToPreserveIndices, i)
		} else {
			// To be summarized
			selection.ToSummarize = append(selection.ToSummarize, msg)
			selection.ToSummarizeIndices = append(selection.ToSummarizeIndices, i)
		}
	}

	// Calculate token estimates
	selection.EstimatedTokensToSummarize = EstimateMessagesTokenCount(selection.ToSummarize)
	selection.EstimatedTokensToPreserve = EstimateMessagesTokenCount(selection.ToPreserve)

	return selection
}

// adjustForToolCalls ensures we don't split in the middle of a tool call sequence.
// A tool call sequence is: assistant (with tool_use) -> user (with tool_result)
func (s *SlidingWindowStrategy) adjustForToolCalls(messages []agent.Message, splitIdx, maxPreserve int) int {
	// Look backwards from splitIdx to find complete boundaries
	for i := splitIdx; i > 0; i-- {
		msg := messages[i]

		// Check if this message contains tool results
		hasToolResult := false
		for _, block := range msg.Content {
			if block.Type == agent.ContentTypeToolResult {
				hasToolResult = true
				break
			}
		}

		if hasToolResult {
			// This is a tool result - check if there's a corresponding tool_use before
			if i > 0 {
				prevMsg := messages[i-1]
				hasToolUse := false
				for _, block := range prevMsg.Content {
					if block.Type == agent.ContentTypeToolUse {
						hasToolUse = true
						break
					}
				}
				if hasToolUse {
					// Don't split between tool_use and tool_result
					// Move split point to before the tool_use
					if len(messages)-i+1 <= maxPreserve*2 {
						return i - 1
					}
				}
			}
		}
	}

	return splitIdx
}

// findCriticalSystemMessages identifies system messages that should be preserved.
// Critical messages include: initial context, user preferences, task definitions.
func (s *SlidingWindowStrategy) findCriticalSystemMessages(messages []agent.Message) []int {
	var critical []int

	for i, msg := range messages {
		if msg.Role != agent.RoleSystem {
			continue
		}

		// Check for critical content markers
		for _, block := range msg.Content {
			if block.Type != agent.ContentTypeText {
				continue
			}

			text := block.Text
			// Preserve messages containing key structural markers
			// These indicate important context that shouldn't be summarized
			if containsAny(text, []string{
				"## Task",
				"## Instructions",
				"## Context",
				"IMPORTANT:",
				"CONSTRAINT:",
				"REQUIREMENT:",
			}) {
				critical = append(critical, i)
				break
			}
		}
	}

	return critical
}

// PhaseAwareSummaryStrategy extends SlidingWindowStrategy with phase awareness.
// It prefers to summarize at natural phase boundaries (task completion, etc.)
type PhaseAwareSummaryStrategy struct {
	SlidingWindowStrategy
}

// Name returns the strategy name.
func (s *PhaseAwareSummaryStrategy) Name() string {
	return "phase_aware"
}

// SelectForSummarization extends the sliding window with phase boundary detection.
func (s *PhaseAwareSummaryStrategy) SelectForSummarization(messages []agent.Message, config *Config) *MessageSelection {
	if !config.SummarizeOnPhaseBoundary {
		// Fall back to basic sliding window
		return s.SlidingWindowStrategy.SelectForSummarization(messages, config)
	}

	// First get the basic selection
	selection := s.SlidingWindowStrategy.SelectForSummarization(messages, config)

	// Then try to find a better split point at a phase boundary
	// A phase boundary is indicated by signal messages or completion messages
	for i := len(selection.ToSummarizeIndices) - 1; i >= 0; i-- {
		idx := selection.ToSummarizeIndices[i]
		if s.isPhaseEnd(messages[idx]) {
			// Found a good split point - adjust selection
			return s.resplitAtBoundary(messages, idx+1, config)
		}
	}

	return selection
}

// isPhaseEnd checks if a message marks the end of a phase/task.
func (s *PhaseAwareSummaryStrategy) isPhaseEnd(msg agent.Message) bool {
	for _, block := range msg.Content {
		if block.Type != agent.ContentTypeText {
			continue
		}

		text := block.Text
		// Look for phase completion markers
		if containsAny(text, []string{
			"phase-complete",
			"task completed",
			"## Summary",
			"completed successfully",
		}) {
			return true
		}
	}
	return false
}

// resplitAtBoundary creates a new selection with the split at the given boundary.
func (s *PhaseAwareSummaryStrategy) resplitAtBoundary(messages []agent.Message, boundaryIdx int, config *Config) *MessageSelection {
	// Ensure we still preserve minimum recent messages
	minPreserveIdx := len(messages) - config.PreserveRecentCount
	if boundaryIdx > minPreserveIdx {
		boundaryIdx = minPreserveIdx
	}

	selection := &MessageSelection{
		ToSummarize:        make([]agent.Message, 0, boundaryIdx),
		ToPreserve:         make([]agent.Message, 0, len(messages)-boundaryIdx),
		ToSummarizeIndices: make([]int, 0, boundaryIdx),
		ToPreserveIndices:  make([]int, 0, len(messages)-boundaryIdx),
	}

	for i, msg := range messages {
		if i < boundaryIdx {
			selection.ToSummarize = append(selection.ToSummarize, msg)
			selection.ToSummarizeIndices = append(selection.ToSummarizeIndices, i)
		} else {
			selection.ToPreserve = append(selection.ToPreserve, msg)
			selection.ToPreserveIndices = append(selection.ToPreserveIndices, i)
		}
	}

	selection.EstimatedTokensToSummarize = EstimateMessagesTokenCount(selection.ToSummarize)
	selection.EstimatedTokensToPreserve = EstimateMessagesTokenCount(selection.ToPreserve)

	return selection
}

// Helper functions

func makeRange(start, end int) []int {
	result := make([]int, end-start)
	for i := range result {
		result[i] = start + i
	}
	return result
}

func contains(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

func containsAny(text string, patterns []string) bool {
	for _, p := range patterns {
		if len(text) >= len(p) {
			for i := 0; i <= len(text)-len(p); i++ {
				if text[i:i+len(p)] == p {
					return true
				}
			}
		}
	}
	return false
}

// DefaultStrategy returns the recommended default strategy.
func DefaultStrategy() Strategy {
	return &PhaseAwareSummaryStrategy{}
}
