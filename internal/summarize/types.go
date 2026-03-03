// Package summarize provides session history summarization for context window management.
package summarize

import (
	"context"
	"errors"
	"time"

	"github.com/openexec/openexec/pkg/agent"
)

// Common errors returned by summarization operations.
var (
	ErrNoMessagesToSummarize = errors.New("no messages to summarize")
	ErrSummarizationFailed   = errors.New("summarization failed")
	ErrInvalidConfig         = errors.New("invalid summarization config")
	ErrContextTooSmall       = errors.New("context window too small for summarization")
	ErrProviderRequired      = errors.New("provider required for summarization")
)

// TriggerReason describes why summarization was triggered.
type TriggerReason string

const (
	// TriggerReasonTokenThreshold indicates tokens exceeded the soft threshold.
	TriggerReasonTokenThreshold TriggerReason = "token_threshold"
	// TriggerReasonMessageCount indicates message count exceeded limit.
	TriggerReasonMessageCount TriggerReason = "message_count"
	// TriggerReasonPhaseBoundary indicates a natural phase/task boundary.
	TriggerReasonPhaseBoundary TriggerReason = "phase_boundary"
	// TriggerReasonManual indicates manual summarization request.
	TriggerReasonManual TriggerReason = "manual"
	// TriggerReasonSessionResume indicates summarization for session resumption.
	TriggerReasonSessionResume TriggerReason = "session_resume"
)

// Config configures the summarization behavior.
type Config struct {
	// Enabled determines if automatic summarization is active.
	Enabled bool `json:"enabled"`

	// SoftThreshold is the context usage percentage (0.0-1.0) that triggers summarization.
	// Default: 0.6 (60% of context window).
	SoftThreshold float64 `json:"soft_threshold"`

	// HardThreshold is the context usage percentage that forces immediate summarization.
	// Default: 0.85 (85% of context window).
	HardThreshold float64 `json:"hard_threshold"`

	// MaxMessagesBeforeSummary triggers summarization when message count exceeds this.
	// Default: 50 messages.
	MaxMessagesBeforeSummary int `json:"max_messages_before_summary"`

	// PreserveRecentCount is the number of most recent messages to always keep verbatim.
	// These messages are never summarized. Default: 10.
	PreserveRecentCount int `json:"preserve_recent_count"`

	// MinMessagesToSummarize is the minimum messages needed before summarization makes sense.
	// Default: 6 (need enough context to summarize meaningfully).
	MinMessagesToSummarize int `json:"min_messages_to_summarize"`

	// SummaryTargetTokens is the target size for generated summaries.
	// Default: 2000 tokens. Range: 500-8000.
	SummaryTargetTokens int `json:"summary_target_tokens"`

	// SummaryMaxTokens is the absolute maximum size for summaries.
	// Default: 4000 tokens.
	SummaryMaxTokens int `json:"summary_max_tokens"`

	// SummarizationModel is the model to use for generating summaries.
	// Uses a fast, cost-effective model. Default: "claude-3-haiku-20240307".
	SummarizationModel string `json:"summarization_model"`

	// IncludeToolCallSummaries includes condensed tool execution history.
	// Default: true.
	IncludeToolCallSummaries bool `json:"include_tool_call_summaries"`

	// PreserveCodeReferences ensures file paths and code snippets are retained.
	// Default: true.
	PreserveCodeReferences bool `json:"preserve_code_references"`

	// PreserveUserPreferences retains explicit user preferences and constraints.
	// Default: true.
	PreserveUserPreferences bool `json:"preserve_user_preferences"`

	// EnableIncrementalSummary allows adding to existing summaries instead of regenerating.
	// Default: true.
	EnableIncrementalSummary bool `json:"enable_incremental_summary"`

	// SummarizeOnPhaseBoundary triggers summarization at task/phase completion.
	// Default: true.
	SummarizeOnPhaseBoundary bool `json:"summarize_on_phase_boundary"`

	// CostBudgetUSD is the maximum cost allowed for summarization operations per session.
	// Default: 0.10 USD.
	CostBudgetUSD float64 `json:"cost_budget_usd"`
}

// DefaultConfig returns the default summarization configuration.
func DefaultConfig() *Config {
	return &Config{
		Enabled:                  true,
		SoftThreshold:            0.60,
		HardThreshold:            0.85,
		MaxMessagesBeforeSummary: 50,
		PreserveRecentCount:      10,
		MinMessagesToSummarize:   6,
		SummaryTargetTokens:      2000,
		SummaryMaxTokens:         4000,
		SummarizationModel:       "claude-3-haiku-20240307",
		IncludeToolCallSummaries: true,
		PreserveCodeReferences:   true,
		PreserveUserPreferences:  true,
		EnableIncrementalSummary: true,
		SummarizeOnPhaseBoundary: true,
		CostBudgetUSD:            0.10,
	}
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.SoftThreshold <= 0 || c.SoftThreshold >= 1.0 {
		return errors.New("soft_threshold must be between 0 and 1")
	}
	if c.HardThreshold <= 0 || c.HardThreshold >= 1.0 {
		return errors.New("hard_threshold must be between 0 and 1")
	}
	if c.SoftThreshold >= c.HardThreshold {
		return errors.New("soft_threshold must be less than hard_threshold")
	}
	if c.PreserveRecentCount < 2 {
		return errors.New("preserve_recent_count must be at least 2")
	}
	if c.MinMessagesToSummarize < 4 {
		return errors.New("min_messages_to_summarize must be at least 4")
	}
	if c.SummaryTargetTokens < 500 || c.SummaryTargetTokens > 8000 {
		return errors.New("summary_target_tokens must be between 500 and 8000")
	}
	if c.SummaryMaxTokens < c.SummaryTargetTokens {
		return errors.New("summary_max_tokens must be at least summary_target_tokens")
	}
	if c.SummarizationModel == "" {
		return errors.New("summarization_model is required")
	}
	return nil
}

// TriggerCheckResult contains the result of checking if summarization is needed.
type TriggerCheckResult struct {
	// ShouldSummarize indicates if summarization should be performed.
	ShouldSummarize bool `json:"should_summarize"`

	// Reason explains why summarization is or isn't needed.
	Reason TriggerReason `json:"reason,omitempty"`

	// CurrentTokens is the current estimated token count.
	CurrentTokens int `json:"current_tokens"`

	// ContextLimit is the context window size.
	ContextLimit int `json:"context_limit"`

	// UsagePercent is the current context usage as a percentage.
	UsagePercent float64 `json:"usage_percent"`

	// MessageCount is the number of messages in history.
	MessageCount int `json:"message_count"`

	// IsUrgent indicates if this is a hard threshold trigger requiring immediate action.
	IsUrgent bool `json:"is_urgent"`

	// EstimatedSavings is the approximate tokens that could be saved.
	EstimatedSavings int `json:"estimated_savings,omitempty"`
}

// MessageSelection contains the result of selecting messages for summarization.
type MessageSelection struct {
	// ToSummarize contains the messages that should be summarized.
	ToSummarize []agent.Message `json:"to_summarize"`

	// ToPreserve contains the messages that should remain verbatim.
	ToPreserve []agent.Message `json:"to_preserve"`

	// ToSummarizeIndices contains the original indices of messages to summarize.
	ToSummarizeIndices []int `json:"to_summarize_indices"`

	// ToPreserveIndices contains the original indices of messages to preserve.
	ToPreserveIndices []int `json:"to_preserve_indices"`

	// EstimatedTokensToSummarize is the token count of messages to summarize.
	EstimatedTokensToSummarize int `json:"estimated_tokens_to_summarize"`

	// EstimatedTokensToPreserve is the token count of messages to preserve.
	EstimatedTokensToPreserve int `json:"estimated_tokens_to_preserve"`
}

// SummaryResult contains the result of a summarization operation.
type SummaryResult struct {
	// ID is the unique identifier for this summary.
	ID string `json:"id"`

	// SessionID is the session this summary belongs to.
	SessionID string `json:"session_id"`

	// Text is the generated summary content.
	Text string `json:"text"`

	// MessagesSummarized is the count of messages that were summarized.
	MessagesSummarized int `json:"messages_summarized"`

	// OriginalTokens is the token count before summarization.
	OriginalTokens int `json:"original_tokens"`

	// SummaryTokens is the token count of the generated summary.
	SummaryTokens int `json:"summary_tokens"`

	// TokensSaved is OriginalTokens - SummaryTokens.
	TokensSaved int `json:"tokens_saved"`

	// CompressionRatio is SummaryTokens / OriginalTokens.
	CompressionRatio float64 `json:"compression_ratio"`

	// TriggerReason explains why summarization was performed.
	TriggerReason TriggerReason `json:"trigger_reason"`

	// GenerationCostUSD is the cost of generating this summary.
	GenerationCostUSD float64 `json:"generation_cost_usd"`

	// GenerationDuration is how long summarization took.
	GenerationDuration time.Duration `json:"generation_duration"`

	// CreatedAt is when the summary was created.
	CreatedAt time.Time `json:"created_at"`

	// PreviousSummaryID is the ID of the previous summary if incremental.
	PreviousSummaryID string `json:"previous_summary_id,omitempty"`

	// IsIncremental indicates if this summary builds on a previous one.
	IsIncremental bool `json:"is_incremental"`
}

// SummaryMetrics tracks summarization statistics for a session.
type SummaryMetrics struct {
	// TotalSummarizations is the count of summarization operations performed.
	TotalSummarizations int `json:"total_summarizations"`

	// TotalTokensSaved is the cumulative tokens saved across all summaries.
	TotalTokensSaved int `json:"total_tokens_saved"`

	// TotalCostUSD is the cumulative cost of all summarization operations.
	TotalCostUSD float64 `json:"total_cost_usd"`

	// AverageCompressionRatio is the mean compression ratio achieved.
	AverageCompressionRatio float64 `json:"average_compression_ratio"`

	// LastSummarizationAt is when the last summarization occurred.
	LastSummarizationAt time.Time `json:"last_summarization_at,omitempty"`
}

// Summarizer is the interface for session history summarization.
type Summarizer interface {
	// ShouldSummarize checks if summarization is needed given the current state.
	ShouldSummarize(messages []agent.Message, contextLimit int) *TriggerCheckResult

	// SelectMessages determines which messages to summarize and which to preserve.
	SelectMessages(messages []agent.Message) (*MessageSelection, error)

	// Summarize generates a summary from the selected messages.
	Summarize(ctx context.Context, sessionID string, messages []agent.Message, reason TriggerReason) (*SummaryResult, error)

	// SummarizeIncremental adds to an existing summary instead of replacing it.
	SummarizeIncremental(ctx context.Context, sessionID string, existingSummary string, newMessages []agent.Message) (*SummaryResult, error)

	// GetLatestSummary retrieves the most recent summary for a session.
	GetLatestSummary(ctx context.Context, sessionID string) (*SummaryResult, error)

	// GetMetrics returns summarization statistics for a session.
	GetMetrics(ctx context.Context, sessionID string) (*SummaryMetrics, error)

	// BuildContextWithSummary creates a message list with summary prefix.
	BuildContextWithSummary(ctx context.Context, sessionID string, recentMessages []agent.Message) ([]agent.Message, error)
}

// SummaryPromptData contains data passed to the summarization prompt template.
type SummaryPromptData struct {
	// Messages is the raw messages to summarize.
	Messages []agent.Message

	// PreviousSummary is any existing summary to build upon.
	PreviousSummary string

	// TargetTokens is the desired summary length.
	TargetTokens int

	// IncludeToolCalls specifies whether to include tool execution history.
	IncludeToolCalls bool

	// PreserveCodeRefs specifies whether to preserve code/file references.
	PreserveCodeRefs bool

	// PreservePreferences specifies whether to preserve user preferences.
	PreservePreferences bool
}

// PromptBuilder creates prompts for summarization.
type PromptBuilder interface {
	// BuildSummarizationPrompt creates the prompt for summary generation.
	BuildSummarizationPrompt(data *SummaryPromptData) string

	// BuildIncrementalPrompt creates the prompt for incremental summarization.
	BuildIncrementalPrompt(data *SummaryPromptData) string
}
