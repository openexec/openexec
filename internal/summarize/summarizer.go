// Package summarize provides session history summarization for context window management.
package summarize

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/openexec/openexec/internal/agent"
	"github.com/openexec/openexec/pkg/db/session"
)

// SessionSummarizer implements the Summarizer interface, providing session history
// summarization to manage context window limits effectively.
type SessionSummarizer struct {
	config        *Config
	strategy      Strategy
	promptBuilder PromptBuilder
	provider      agent.ProviderAdapter
	repository    session.Repository

	// metrics tracks summarization statistics per session
	metrics map[string]*SummaryMetrics
}

// NewSessionSummarizer creates a new SessionSummarizer with the given configuration.
// It validates the configuration and returns an error if invalid.
func NewSessionSummarizer(config *Config, provider agent.ProviderAdapter, repository session.Repository) (*SessionSummarizer, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}

	if provider == nil {
		return nil, ErrProviderRequired
	}

	return &SessionSummarizer{
		config:        config,
		strategy:      DefaultStrategy(),
		promptBuilder: NewPromptBuilder(),
		provider:      provider,
		repository:    repository,
		metrics:       make(map[string]*SummaryMetrics),
	}, nil
}

// WithStrategy sets a custom selection strategy.
func (s *SessionSummarizer) WithStrategy(strategy Strategy) *SessionSummarizer {
	if strategy != nil {
		s.strategy = strategy
	}
	return s
}

// WithPromptBuilder sets a custom prompt builder.
func (s *SessionSummarizer) WithPromptBuilder(builder PromptBuilder) *SessionSummarizer {
	if builder != nil {
		s.promptBuilder = builder
	}
	return s
}

// ShouldSummarize checks if summarization is needed given the current state.
// It evaluates token usage percentage, message count, and urgency level.
func (s *SessionSummarizer) ShouldSummarize(messages []agent.Message, contextLimit int) *TriggerCheckResult {
	result := &TriggerCheckResult{
		ShouldSummarize: false,
		MessageCount:    len(messages),
		ContextLimit:    contextLimit,
	}

	if !s.config.Enabled {
		result.Reason = ""
		return result
	}

	if len(messages) < s.config.MinMessagesToSummarize {
		// Not enough messages to meaningfully summarize
		return result
	}

	// Calculate current token usage
	result.CurrentTokens = EstimateMessagesTokenCount(messages)
	if contextLimit > 0 {
		result.UsagePercent = float64(result.CurrentTokens) / float64(contextLimit)
	}

	// Check hard threshold (urgent)
	if contextLimit > 0 && result.UsagePercent >= s.config.HardThreshold {
		result.ShouldSummarize = true
		result.Reason = TriggerReasonTokenThreshold
		result.IsUrgent = true
		result.EstimatedSavings = s.estimateSavings(messages)
		return result
	}

	// Check soft threshold
	if contextLimit > 0 && result.UsagePercent >= s.config.SoftThreshold {
		result.ShouldSummarize = true
		result.Reason = TriggerReasonTokenThreshold
		result.EstimatedSavings = s.estimateSavings(messages)
		return result
	}

	// Check message count threshold
	if len(messages) >= s.config.MaxMessagesBeforeSummary {
		result.ShouldSummarize = true
		result.Reason = TriggerReasonMessageCount
		result.EstimatedSavings = s.estimateSavings(messages)
		return result
	}

	return result
}

// SelectMessages determines which messages to summarize and which to preserve.
// It delegates to the configured strategy for the actual selection logic.
func (s *SessionSummarizer) SelectMessages(messages []agent.Message) (*MessageSelection, error) {
	if len(messages) < s.config.MinMessagesToSummarize {
		return nil, ErrNoMessagesToSummarize
	}

	selection := s.strategy.SelectForSummarization(messages, s.config)
	if selection == nil || len(selection.ToSummarize) == 0 {
		return nil, ErrNoMessagesToSummarize
	}

	return selection, nil
}

// Summarize generates a summary from the selected messages.
// It uses the configured provider and prompt builder to generate the summary.
func (s *SessionSummarizer) Summarize(ctx context.Context, sessionID string, messages []agent.Message, reason TriggerReason) (*SummaryResult, error) {
	startTime := time.Now()

	// Select messages for summarization
	selection, err := s.SelectMessages(messages)
	if err != nil {
		return nil, err
	}

	// Build the summarization prompt
	promptData := &SummaryPromptData{
		Messages:            selection.ToSummarize,
		TargetTokens:        s.config.SummaryTargetTokens,
		IncludeToolCalls:    s.config.IncludeToolCallSummaries,
		PreserveCodeRefs:    s.config.PreserveCodeReferences,
		PreservePreferences: s.config.PreserveUserPreferences,
	}

	// Check for existing summary to build upon
	if s.config.EnableIncrementalSummary && s.repository != nil {
		existingSummary, err := s.repository.GetLatestSummary(ctx, sessionID)
		if err == nil && existingSummary != nil {
			promptData.PreviousSummary = existingSummary.SummaryText
		}
	}

	prompt := s.promptBuilder.BuildSummarizationPrompt(promptData)

	// Generate the summary using the provider
	req := agent.Request{
		Model: s.config.SummarizationModel,
		Messages: []agent.Message{
			agent.NewTextMessage(agent.RoleUser, prompt),
		},
		MaxTokens: s.config.SummaryMaxTokens,
	}

	resp, err := s.provider.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSummarizationFailed, err)
	}

	summaryText := resp.GetText()
	if summaryText == "" {
		return nil, fmt.Errorf("%w: empty summary generated", ErrSummarizationFailed)
	}

	// Calculate metrics
	originalTokens := selection.EstimatedTokensToSummarize
	summaryTokens := EstimateTokenCount(summaryText)
	tokensSaved := originalTokens - summaryTokens
	if tokensSaved < 0 {
		tokensSaved = 0
	}

	compressionRatio := float64(summaryTokens) / float64(originalTokens)
	if originalTokens == 0 {
		compressionRatio = 1.0
	}

	// Calculate cost based on provider usage
	generationCost := s.calculateCost(resp.Usage)

	// Build the result
	result := &SummaryResult{
		ID:                 uuid.New().String(),
		SessionID:          sessionID,
		Text:               summaryText,
		MessagesSummarized: len(selection.ToSummarize),
		OriginalTokens:     originalTokens,
		SummaryTokens:      summaryTokens,
		TokensSaved:        tokensSaved,
		CompressionRatio:   compressionRatio,
		TriggerReason:      reason,
		GenerationCostUSD:  generationCost,
		GenerationDuration: time.Since(startTime),
		CreatedAt:          time.Now().UTC(),
	}

	// Set incremental flag if building on previous summary
	if promptData.PreviousSummary != "" {
		result.IsIncremental = true
		// Get the previous summary ID if available
		if s.repository != nil {
			if prevSummary, err := s.repository.GetLatestSummary(ctx, sessionID); err == nil && prevSummary != nil {
				result.PreviousSummaryID = prevSummary.ID
			}
		}
	}

	// Persist the summary if repository is available
	if s.repository != nil {
		dbSummary, err := session.NewSessionSummary(sessionID, summaryText, len(selection.ToSummarize), tokensSaved)
		if err != nil {
			return nil, fmt.Errorf("failed to create session summary: %w", err)
		}
		dbSummary.ID = result.ID
		if err := s.repository.CreateSummary(ctx, dbSummary); err != nil {
			return nil, fmt.Errorf("failed to persist summary: %w", err)
		}
	}

	// Update metrics
	s.updateMetrics(sessionID, result)

	return result, nil
}

// SummarizeIncremental adds to an existing summary instead of replacing it.
// This is more efficient when only a few new messages have been added.
func (s *SessionSummarizer) SummarizeIncremental(ctx context.Context, sessionID string, existingSummary string, newMessages []agent.Message) (*SummaryResult, error) {
	startTime := time.Now()

	if existingSummary == "" {
		return nil, errors.New("existing summary is required for incremental summarization")
	}

	if len(newMessages) == 0 {
		return nil, ErrNoMessagesToSummarize
	}

	// Build the incremental prompt
	promptData := &SummaryPromptData{
		Messages:            newMessages,
		PreviousSummary:     existingSummary,
		TargetTokens:        s.config.SummaryTargetTokens,
		IncludeToolCalls:    s.config.IncludeToolCallSummaries,
		PreserveCodeRefs:    s.config.PreserveCodeReferences,
		PreservePreferences: s.config.PreserveUserPreferences,
	}

	prompt := s.promptBuilder.BuildIncrementalPrompt(promptData)

	// Generate the updated summary
	req := agent.Request{
		Model: s.config.SummarizationModel,
		Messages: []agent.Message{
			agent.NewTextMessage(agent.RoleUser, prompt),
		},
		MaxTokens: s.config.SummaryMaxTokens,
	}

	resp, err := s.provider.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSummarizationFailed, err)
	}

	summaryText := resp.GetText()
	if summaryText == "" {
		return nil, fmt.Errorf("%w: empty summary generated", ErrSummarizationFailed)
	}

	// Calculate metrics
	originalTokens := EstimateMessagesTokenCount(newMessages) + EstimateTokenCount(existingSummary)
	summaryTokens := EstimateTokenCount(summaryText)
	tokensSaved := EstimateMessagesTokenCount(newMessages) // Tokens saved from new messages
	if tokensSaved < 0 {
		tokensSaved = 0
	}

	compressionRatio := float64(summaryTokens) / float64(originalTokens)
	if originalTokens == 0 {
		compressionRatio = 1.0
	}

	generationCost := s.calculateCost(resp.Usage)

	result := &SummaryResult{
		ID:                 uuid.New().String(),
		SessionID:          sessionID,
		Text:               summaryText,
		MessagesSummarized: len(newMessages),
		OriginalTokens:     originalTokens,
		SummaryTokens:      summaryTokens,
		TokensSaved:        tokensSaved,
		CompressionRatio:   compressionRatio,
		TriggerReason:      TriggerReasonManual,
		GenerationCostUSD:  generationCost,
		GenerationDuration: time.Since(startTime),
		CreatedAt:          time.Now().UTC(),
		IsIncremental:      true,
	}

	// Persist the summary if repository is available
	if s.repository != nil {
		dbSummary, err := session.NewSessionSummary(sessionID, summaryText, len(newMessages), tokensSaved)
		if err != nil {
			return nil, fmt.Errorf("failed to create session summary: %w", err)
		}
		dbSummary.ID = result.ID
		if err := s.repository.CreateSummary(ctx, dbSummary); err != nil {
			return nil, fmt.Errorf("failed to persist summary: %w", err)
		}
	}

	// Update metrics
	s.updateMetrics(sessionID, result)

	return result, nil
}

// GetLatestSummary retrieves the most recent summary for a session.
func (s *SessionSummarizer) GetLatestSummary(ctx context.Context, sessionID string) (*SummaryResult, error) {
	if s.repository == nil {
		return nil, errors.New("repository not configured")
	}

	dbSummary, err := s.repository.GetLatestSummary(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if dbSummary == nil {
		return nil, nil
	}

	return &SummaryResult{
		ID:                 dbSummary.ID,
		SessionID:          dbSummary.SessionID,
		Text:               dbSummary.SummaryText,
		MessagesSummarized: dbSummary.MessagesSummarized,
		TokensSaved:        dbSummary.TokensSaved,
		SummaryTokens:      EstimateTokenCount(dbSummary.SummaryText),
		CreatedAt:          dbSummary.CreatedAt,
	}, nil
}

// GetMetrics returns summarization statistics for a session.
func (s *SessionSummarizer) GetMetrics(ctx context.Context, sessionID string) (*SummaryMetrics, error) {
	if metrics, ok := s.metrics[sessionID]; ok {
		return metrics, nil
	}

	// If repository is available, try to build metrics from stored summaries
	if s.repository != nil {
		summaries, err := s.repository.ListSummaries(ctx, sessionID)
		if err != nil {
			return nil, err
		}

		metrics := &SummaryMetrics{}
		for _, summary := range summaries {
			metrics.TotalSummarizations++
			metrics.TotalTokensSaved += summary.TokensSaved
			if metrics.LastSummarizationAt.Before(summary.CreatedAt) {
				metrics.LastSummarizationAt = summary.CreatedAt
			}
		}

		if metrics.TotalSummarizations > 0 {
			s.metrics[sessionID] = metrics
		}

		return metrics, nil
	}

	return &SummaryMetrics{}, nil
}

// BuildContextWithSummary creates a message list with summary prefix.
// This method prepares the context for the next LLM call by:
// 1. Including the latest summary as a system message (if exists)
// 2. Including the recent messages that were preserved
func (s *SessionSummarizer) BuildContextWithSummary(ctx context.Context, sessionID string, recentMessages []agent.Message) ([]agent.Message, error) {
	var result []agent.Message

	// Get the latest summary
	summary, err := s.GetLatestSummary(ctx, sessionID)
	if err != nil && !errors.Is(err, errors.New("repository not configured")) {
		// Log error but continue without summary
		summary = nil
	}

	// If we have a summary, add it as a system message prefix
	if summary != nil && summary.Text != "" {
		summaryMsg := agent.Message{
			Role: agent.RoleSystem,
			Content: []agent.ContentBlock{
				{
					Type: agent.ContentTypeText,
					Text: fmt.Sprintf("[SESSION SUMMARY]\n\nThe following is a summary of the earlier conversation that was condensed to manage context length:\n\n%s\n\n[END OF SUMMARY]\n\nContinue the conversation from here:", summary.Text),
				},
			},
		}
		result = append(result, summaryMsg)
	}

	// Append the recent messages
	result = append(result, recentMessages...)

	return result, nil
}

// estimateSavings calculates the approximate tokens that would be saved by summarization.
func (s *SessionSummarizer) estimateSavings(messages []agent.Message) int {
	// Use the strategy to determine what would be summarized
	selection := s.strategy.SelectForSummarization(messages, s.config)
	if selection == nil {
		return 0
	}

	// Estimate savings: original tokens - target summary tokens
	originalTokens := selection.EstimatedTokensToSummarize
	targetSummaryTokens := s.config.SummaryTargetTokens

	savings := originalTokens - targetSummaryTokens
	if savings < 0 {
		return 0
	}

	return savings
}

// calculateCost estimates the cost of a summarization based on token usage.
func (s *SessionSummarizer) calculateCost(usage agent.Usage) float64 {
	// Use pricing from models.go if available
	// For now, use approximate Claude Haiku pricing
	inputPrice := 0.25  // $ per million tokens
	outputPrice := 1.25 // $ per million tokens

	inputCost := float64(usage.PromptTokens) * inputPrice / 1_000_000
	outputCost := float64(usage.CompletionTokens) * outputPrice / 1_000_000

	return inputCost + outputCost
}

// updateMetrics updates the metrics for a session after summarization.
func (s *SessionSummarizer) updateMetrics(sessionID string, result *SummaryResult) {
	metrics, ok := s.metrics[sessionID]
	if !ok {
		metrics = &SummaryMetrics{}
		s.metrics[sessionID] = metrics
	}

	metrics.TotalSummarizations++
	metrics.TotalTokensSaved += result.TokensSaved
	metrics.TotalCostUSD += result.GenerationCostUSD
	metrics.LastSummarizationAt = result.CreatedAt

	// Update average compression ratio using cumulative average
	if metrics.TotalSummarizations == 1 {
		metrics.AverageCompressionRatio = result.CompressionRatio
	} else {
		// Cumulative moving average
		metrics.AverageCompressionRatio = metrics.AverageCompressionRatio +
			(result.CompressionRatio-metrics.AverageCompressionRatio)/float64(metrics.TotalSummarizations)
	}
}

// Config returns the current configuration.
func (s *SessionSummarizer) Config() *Config {
	return s.config
}

// Strategy returns the current selection strategy.
func (s *SessionSummarizer) Strategy() Strategy {
	return s.strategy
}
