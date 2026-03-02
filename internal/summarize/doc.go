// Package summarize provides session history summarization for context window management.
//
// # Overview
//
// Session History Summarization enables the agent loop to maintain effective conversations
// even when the message history exceeds the LLM's context window limits. The system
// automatically detects when summarization is needed, generates concise summaries of
// older messages, and injects those summaries to preserve context continuity.
//
// # Architecture
//
// The summarization system consists of three main components:
//
// 1. **SummarizationConfig** - Configuration for when and how to summarize:
//   - Token thresholds that trigger summarization
//   - Preservation rules for recent messages
//   - Model selection for summary generation
//   - Quality vs compression trade-offs
//
// 2. **Summarizer** - The core summarization engine:
//   - Detects when context is approaching limits
//   - Selects which messages to summarize
//   - Uses LLM to generate concise summaries
//   - Persists summaries for session continuity
//
// 3. **Integration Points** - How summarization connects to the system:
//   - Integrates with AgentLoop for automatic triggering
//   - Uses existing session_summaries database table
//   - Emits ContextSummarized events for observability
//   - Injects summaries via ContextTypeSessionSummary
//
// # Summarization Strategy
//
// The system employs a "sliding window with summary prefix" strategy:
//
//	┌───────────────────────────────────────────────────────────┐
//	│                    CONTEXT WINDOW                          │
//	├─────────────────┬─────────────────────────────────────────┤
//	│   SUMMARY       │         RECENT MESSAGES                  │
//	│   (compressed   │   (preserved verbatim for               │
//	│    history)     │    full context)                        │
//	├─────────────────┼─────────────────────────────────────────┤
//	│   ~2-8K tokens  │        Variable based on budget          │
//	└─────────────────┴─────────────────────────────────────────┘
//
// # Trigger Conditions
//
// Summarization triggers when ANY of these conditions are met:
//   - Token usage exceeds SoftThreshold (default: 60% of context window)
//   - Message count exceeds MaxMessagesBeforeSummary (default: 50)
//   - A phase/task boundary is reached (natural summarization point)
//
// # Message Selection
//
// When selecting messages to summarize, the system:
//   - ALWAYS preserves the most recent N messages (PreserveRecentCount)
//   - Preserves messages with tool calls that haven't completed
//   - Preserves system messages with critical context
//   - Groups related messages together for coherent summarization
//
// # Summary Generation
//
// The summarizer uses a fast, cost-effective model (e.g., Claude Haiku) with
// a specialized prompt that:
//   - Extracts key decisions and outcomes
//   - Preserves file paths and code references mentioned
//   - Maintains tool call history in condensed form
//   - Retains any user preferences or constraints established
//   - Captures error resolutions for future reference
//
// # Cost Efficiency
//
// The system is designed for cost efficiency:
//   - Uses cheaper models for summarization (not the main task model)
//   - Caches summaries in database for session resumption
//   - Only re-summarizes when significant new content is added
//   - Tracks tokens_saved to demonstrate value
//
// # Database Schema
//
// Summaries are persisted using the existing session_summaries table:
//
//	session_summaries (
//	    id TEXT PRIMARY KEY,
//	    session_id TEXT NOT NULL,
//	    summary_text TEXT NOT NULL,
//	    messages_summarized INTEGER NOT NULL,
//	    tokens_saved INTEGER NOT NULL,
//	    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
//	)
//
// # Usage Example
//
//	// Configure summarization
//	config := summarize.DefaultConfig()
//	config.SoftThreshold = 0.7 // Trigger at 70% context usage
//
//	// Create summarizer
//	summarizer, err := summarize.NewSummarizer(config, provider, repository)
//
//	// Check if summarization is needed
//	if summarizer.ShouldSummarize(messages, contextBudget) {
//	    summary, err := summarizer.Summarize(ctx, sessionID, messages)
//	    // summary.Text contains the compressed history
//	    // summary.TokensSaved reports efficiency gain
//	}
//
// # Integration with Agent Loop
//
// The AgentLoop automatically manages summarization when enabled:
//
//	loopConfig := loop.AgentLoopConfig{
//	    // ... other config ...
//	    SummarizationConfig: summarize.DefaultConfig(),
//	}
//
// The loop checks before each iteration and summarizes as needed, emitting
// ContextSummarized events for observability.
package summarize
