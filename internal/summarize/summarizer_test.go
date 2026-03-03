package summarize

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/agent"
	"github.com/openexec/openexec/pkg/db/session"
)

// mockProvider implements agent.ProviderAdapter for testing.
type mockProvider struct {
	name           string
	models         []string
	completeFunc   func(ctx context.Context, req agent.Request) (*agent.Response, error)
	completeResp   *agent.Response
	completeErr    error
	completeCalled bool
}

func newMockProvider() *mockProvider {
	return &mockProvider{
		name:   "mock",
		models: []string{"mock-model"},
		completeResp: &agent.Response{
			ID:    "test-response",
			Model: "mock-model",
			Content: []agent.ContentBlock{
				{Type: agent.ContentTypeText, Text: "This is a test summary of the conversation."},
			},
			StopReason: agent.StopReasonEnd,
			Usage: agent.Usage{
				PromptTokens:     1000,
				CompletionTokens: 200,
				TotalTokens:      1200,
			},
		},
	}
}

func (m *mockProvider) GetName() string {
	return m.name
}

func (m *mockProvider) GetModels() []string {
	return m.models
}

func (m *mockProvider) GetModelInfo(modelID string) (*agent.ModelInfo, error) {
	return &agent.ModelInfo{
		ID:       modelID,
		Name:     modelID,
		Provider: m.name,
	}, nil
}

func (m *mockProvider) GetCapabilities(modelID string) (*agent.ProviderCapabilities, error) {
	return &agent.ProviderCapabilities{
		Streaming:        true,
		ToolUse:          true,
		SystemPrompt:     true,
		MultiTurn:        true,
		MaxContextTokens: 128000,
		MaxOutputTokens:  4096,
	}, nil
}

func (m *mockProvider) Complete(ctx context.Context, req agent.Request) (*agent.Response, error) {
	m.completeCalled = true
	if m.completeFunc != nil {
		return m.completeFunc(ctx, req)
	}
	return m.completeResp, m.completeErr
}

func (m *mockProvider) Stream(ctx context.Context, req agent.Request) (<-chan agent.StreamEvent, error) {
	return nil, errors.New("streaming not implemented for mock")
}

func (m *mockProvider) ValidateRequest(req agent.Request) error {
	return nil
}

func (m *mockProvider) EstimateTokens(content string) int {
	return len(content) / 4
}

// mockRepository implements a simple in-memory session.Repository for testing.
type mockRepository struct {
	summaries map[string][]*session.SessionSummary
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		summaries: make(map[string][]*session.SessionSummary),
	}
}

func (r *mockRepository) CreateSession(ctx context.Context, s *session.Session) error {
	return nil
}

func (r *mockRepository) GetSession(ctx context.Context, id string) (*session.Session, error) {
	return nil, session.ErrSessionNotFound
}

func (r *mockRepository) UpdateSession(ctx context.Context, s *session.Session) error {
	return nil
}

func (r *mockRepository) DeleteSession(ctx context.Context, id string) error {
	return nil
}

func (r *mockRepository) ListSessions(ctx context.Context, opts *session.ListSessionsOptions) ([]*session.Session, error) {
	return nil, nil
}

func (r *mockRepository) ListSessionsByProject(ctx context.Context, projectPath string) ([]*session.Session, error) {
	return nil, nil
}

func (r *mockRepository) GetSessionForks(ctx context.Context, sessionID string) ([]*session.Session, error) {
	return nil, nil
}

func (r *mockRepository) CreateMessage(ctx context.Context, msg *session.Message) error {
	return nil
}

func (r *mockRepository) GetMessage(ctx context.Context, id string) (*session.Message, error) {
	return nil, session.ErrMessageNotFound
}

func (r *mockRepository) UpdateMessage(ctx context.Context, msg *session.Message) error {
	return nil
}

func (r *mockRepository) DeleteMessage(ctx context.Context, id string) error {
	return nil
}

func (r *mockRepository) ListMessages(ctx context.Context, sessionID string) ([]*session.Message, error) {
	return nil, nil
}

func (r *mockRepository) ListMessagesByRole(ctx context.Context, sessionID string, role session.Role) ([]*session.Message, error) {
	return nil, nil
}

func (r *mockRepository) GetMessageCount(ctx context.Context, sessionID string) (int, error) {
	return 0, nil
}

func (r *mockRepository) CreateToolCall(ctx context.Context, tc *session.ToolCall) error {
	return nil
}

func (r *mockRepository) GetToolCall(ctx context.Context, id string) (*session.ToolCall, error) {
	return nil, session.ErrToolCallNotFound
}

func (r *mockRepository) UpdateToolCall(ctx context.Context, tc *session.ToolCall) error {
	return nil
}

func (r *mockRepository) DeleteToolCall(ctx context.Context, id string) error {
	return nil
}

func (r *mockRepository) ListToolCalls(ctx context.Context, sessionID string) ([]*session.ToolCall, error) {
	return nil, nil
}

func (r *mockRepository) ListToolCallsByMessage(ctx context.Context, messageID string) ([]*session.ToolCall, error) {
	return nil, nil
}

func (r *mockRepository) ListToolCallsByStatus(ctx context.Context, sessionID string, status session.ToolCallStatus) ([]*session.ToolCall, error) {
	return nil, nil
}

func (r *mockRepository) ListPendingApprovals(ctx context.Context, sessionID string) ([]*session.ToolCall, error) {
	return nil, nil
}

func (r *mockRepository) CreateSummary(ctx context.Context, summary *session.SessionSummary) error {
	if r.summaries[summary.SessionID] == nil {
		r.summaries[summary.SessionID] = []*session.SessionSummary{}
	}
	r.summaries[summary.SessionID] = append(r.summaries[summary.SessionID], summary)
	return nil
}

func (r *mockRepository) GetSummary(ctx context.Context, id string) (*session.SessionSummary, error) {
	for _, summaries := range r.summaries {
		for _, s := range summaries {
			if s.ID == id {
				return s, nil
			}
		}
	}
	return nil, errors.New("summary not found")
}

func (r *mockRepository) ListSummaries(ctx context.Context, sessionID string) ([]*session.SessionSummary, error) {
	return r.summaries[sessionID], nil
}

func (r *mockRepository) GetLatestSummary(ctx context.Context, sessionID string) (*session.SessionSummary, error) {
	summaries := r.summaries[sessionID]
	if len(summaries) == 0 {
		return nil, nil
	}
	return summaries[len(summaries)-1], nil
}

func (r *mockRepository) DeleteSummary(ctx context.Context, id string) error {
	return nil
}

func (r *mockRepository) GetSessionStats(ctx context.Context, sessionID string) (*session.SessionStats, error) {
	return nil, nil
}

func (r *mockRepository) GetUsageByProvider(ctx context.Context) ([]*session.ProviderUsage, error) {
	return nil, nil
}

func (r *mockRepository) Close() error {
	return nil
}

// Test helper to create messages
func createTestMessages(count int) []agent.Message {
	messages := make([]agent.Message, count)
	for i := 0; i < count; i++ {
		role := agent.RoleUser
		if i%2 == 1 {
			role = agent.RoleAssistant
		}
		messages[i] = agent.NewTextMessage(role, "Test message content for testing purposes. This is message number "+string(rune('0'+i%10)))
	}
	return messages
}

func TestNewSessionSummarizer(t *testing.T) {
	provider := newMockProvider()

	t.Run("with default config", func(t *testing.T) {
		summarizer, err := NewSessionSummarizer(nil, provider, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if summarizer == nil {
			t.Fatal("expected summarizer, got nil")
		}
		if !summarizer.Config().Enabled {
			t.Error("expected config.Enabled to be true")
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		config := DefaultConfig()
		config.SoftThreshold = 0.5
		config.SummaryTargetTokens = 1000

		summarizer, err := NewSessionSummarizer(config, provider, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if summarizer.Config().SoftThreshold != 0.5 {
			t.Errorf("expected SoftThreshold 0.5, got %f", summarizer.Config().SoftThreshold)
		}
	})

	t.Run("with invalid config", func(t *testing.T) {
		config := DefaultConfig()
		config.SoftThreshold = 2.0 // Invalid

		_, err := NewSessionSummarizer(config, provider, nil)
		if err == nil {
			t.Error("expected error for invalid config")
		}
		if !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("expected ErrInvalidConfig, got %v", err)
		}
	})

	t.Run("without provider", func(t *testing.T) {
		_, err := NewSessionSummarizer(nil, nil, nil)
		if err == nil {
			t.Error("expected error without provider")
		}
		if !errors.Is(err, ErrProviderRequired) {
			t.Errorf("expected ErrProviderRequired, got %v", err)
		}
	})
}

func TestSessionSummarizer_ShouldSummarize(t *testing.T) {
	provider := newMockProvider()
	config := DefaultConfig()
	config.MinMessagesToSummarize = 4
	config.MaxMessagesBeforeSummary = 10
	config.SoftThreshold = 0.6
	config.HardThreshold = 0.85

	summarizer, _ := NewSessionSummarizer(config, provider, nil)

	t.Run("not enough messages", func(t *testing.T) {
		messages := createTestMessages(3) // Less than MinMessagesToSummarize
		result := summarizer.ShouldSummarize(messages, 128000)

		if result.ShouldSummarize {
			t.Error("should not summarize with too few messages")
		}
	})

	t.Run("message count threshold", func(t *testing.T) {
		messages := createTestMessages(12) // More than MaxMessagesBeforeSummary
		result := summarizer.ShouldSummarize(messages, 128000)

		if !result.ShouldSummarize {
			t.Error("should summarize when message count exceeds threshold")
		}
		if result.Reason != TriggerReasonMessageCount {
			t.Errorf("expected reason %s, got %s", TriggerReasonMessageCount, result.Reason)
		}
	})

	t.Run("disabled summarization", func(t *testing.T) {
		disabledConfig := DefaultConfig()
		disabledConfig.Enabled = false
		summarizer, _ := NewSessionSummarizer(disabledConfig, provider, nil)

		messages := createTestMessages(100)
		result := summarizer.ShouldSummarize(messages, 128000)

		if result.ShouldSummarize {
			t.Error("should not summarize when disabled")
		}
	})

	t.Run("token threshold soft", func(t *testing.T) {
		// Create enough messages to trigger soft threshold
		// We need to estimate token count to know context limit
		messages := createTestMessages(8)
		tokenCount := EstimateMessagesTokenCount(messages)

		// Set context limit so current usage is > 60%
		contextLimit := int(float64(tokenCount) / 0.65) // This puts us at ~65% usage

		result := summarizer.ShouldSummarize(messages, contextLimit)

		if !result.ShouldSummarize {
			t.Errorf("should summarize at soft threshold, usage: %f", result.UsagePercent)
		}
		if result.IsUrgent {
			t.Error("soft threshold should not be urgent")
		}
	})

	t.Run("token threshold hard (urgent)", func(t *testing.T) {
		messages := createTestMessages(8)
		tokenCount := EstimateMessagesTokenCount(messages)

		// Set context limit so current usage is > 85%
		contextLimit := int(float64(tokenCount) / 0.90) // This puts us at ~90% usage

		result := summarizer.ShouldSummarize(messages, contextLimit)

		if !result.ShouldSummarize {
			t.Errorf("should summarize at hard threshold, usage: %f", result.UsagePercent)
		}
		if !result.IsUrgent {
			t.Error("hard threshold should be urgent")
		}
	})
}

func TestSessionSummarizer_SelectMessages(t *testing.T) {
	provider := newMockProvider()
	config := DefaultConfig()
	config.MinMessagesToSummarize = 4
	config.PreserveRecentCount = 3

	summarizer, _ := NewSessionSummarizer(config, provider, nil)

	t.Run("select with enough messages", func(t *testing.T) {
		messages := createTestMessages(10)
		selection, err := summarizer.SelectMessages(messages)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if selection == nil {
			t.Fatal("expected selection, got nil")
		}
		if len(selection.ToPreserve) < config.PreserveRecentCount {
			t.Errorf("expected at least %d preserved messages, got %d", config.PreserveRecentCount, len(selection.ToPreserve))
		}
		if len(selection.ToSummarize) == 0 {
			t.Error("expected some messages to summarize")
		}
	})

	t.Run("not enough messages", func(t *testing.T) {
		messages := createTestMessages(2)
		_, err := summarizer.SelectMessages(messages)

		if err == nil {
			t.Error("expected error for too few messages")
		}
		if !errors.Is(err, ErrNoMessagesToSummarize) {
			t.Errorf("expected ErrNoMessagesToSummarize, got %v", err)
		}
	})
}

func TestSessionSummarizer_Summarize(t *testing.T) {
	provider := newMockProvider()
	repo := newMockRepository()
	config := DefaultConfig()
	config.MinMessagesToSummarize = 4
	config.PreserveRecentCount = 2

	summarizer, _ := NewSessionSummarizer(config, provider, repo)

	t.Run("successful summarization", func(t *testing.T) {
		ctx := context.Background()
		messages := createTestMessages(10)

		result, err := summarizer.Summarize(ctx, "session-123", messages, TriggerReasonTokenThreshold)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected result, got nil")
		}
		if result.SessionID != "session-123" {
			t.Errorf("expected session ID 'session-123', got %s", result.SessionID)
		}
		if result.Text == "" {
			t.Error("expected non-empty summary text")
		}
		if result.MessagesSummarized == 0 {
			t.Error("expected some messages to be summarized")
		}
		if result.TriggerReason != TriggerReasonTokenThreshold {
			t.Errorf("expected reason %s, got %s", TriggerReasonTokenThreshold, result.TriggerReason)
		}

		// Check that provider was called
		if !provider.completeCalled {
			t.Error("expected provider.Complete to be called")
		}

		// Check that summary was persisted
		latestSummary, _ := repo.GetLatestSummary(ctx, "session-123")
		if latestSummary == nil {
			t.Error("expected summary to be persisted in repository")
		}
	})

	t.Run("not enough messages", func(t *testing.T) {
		ctx := context.Background()
		messages := createTestMessages(2)

		_, err := summarizer.Summarize(ctx, "session-456", messages, TriggerReasonManual)

		if err == nil {
			t.Error("expected error for too few messages")
		}
		if !errors.Is(err, ErrNoMessagesToSummarize) {
			t.Errorf("expected ErrNoMessagesToSummarize, got %v", err)
		}
	})

	t.Run("provider error", func(t *testing.T) {
		errorProvider := newMockProvider()
		errorProvider.completeErr = errors.New("API error")

		summarizer, _ := NewSessionSummarizer(config, errorProvider, repo)
		ctx := context.Background()
		messages := createTestMessages(10)

		_, err := summarizer.Summarize(ctx, "session-789", messages, TriggerReasonManual)

		if err == nil {
			t.Error("expected error from provider")
		}
		if !errors.Is(err, ErrSummarizationFailed) {
			t.Errorf("expected ErrSummarizationFailed, got %v", err)
		}
	})

	t.Run("empty response", func(t *testing.T) {
		emptyProvider := newMockProvider()
		emptyProvider.completeResp = &agent.Response{
			ID:      "empty",
			Content: []agent.ContentBlock{},
		}

		summarizer, _ := NewSessionSummarizer(config, emptyProvider, repo)
		ctx := context.Background()
		messages := createTestMessages(10)

		_, err := summarizer.Summarize(ctx, "session-empty", messages, TriggerReasonManual)

		if err == nil {
			t.Error("expected error for empty response")
		}
	})
}

func TestSessionSummarizer_SummarizeIncremental(t *testing.T) {
	provider := newMockProvider()
	repo := newMockRepository()
	config := DefaultConfig()

	summarizer, _ := NewSessionSummarizer(config, provider, repo)

	t.Run("successful incremental summarization", func(t *testing.T) {
		ctx := context.Background()
		existingSummary := "Previous conversation summary about implementing a feature."
		newMessages := createTestMessages(5)

		result, err := summarizer.SummarizeIncremental(ctx, "session-inc-1", existingSummary, newMessages)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected result, got nil")
		}
		if !result.IsIncremental {
			t.Error("expected IsIncremental to be true")
		}
		if result.MessagesSummarized != len(newMessages) {
			t.Errorf("expected %d messages summarized, got %d", len(newMessages), result.MessagesSummarized)
		}
	})

	t.Run("empty existing summary", func(t *testing.T) {
		ctx := context.Background()
		newMessages := createTestMessages(5)

		_, err := summarizer.SummarizeIncremental(ctx, "session-inc-2", "", newMessages)

		if err == nil {
			t.Error("expected error for empty existing summary")
		}
	})

	t.Run("no new messages", func(t *testing.T) {
		ctx := context.Background()
		existingSummary := "Previous summary."

		_, err := summarizer.SummarizeIncremental(ctx, "session-inc-3", existingSummary, []agent.Message{})

		if err == nil {
			t.Error("expected error for no new messages")
		}
	})
}

func TestSessionSummarizer_GetLatestSummary(t *testing.T) {
	provider := newMockProvider()
	repo := newMockRepository()
	config := DefaultConfig()
	config.MinMessagesToSummarize = 4
	config.PreserveRecentCount = 2

	summarizer, _ := NewSessionSummarizer(config, provider, repo)

	t.Run("get existing summary", func(t *testing.T) {
		ctx := context.Background()

		// First create a summary
		messages := createTestMessages(10)
		_, err := summarizer.Summarize(ctx, "session-get-1", messages, TriggerReasonManual)
		if err != nil {
			t.Fatalf("failed to create summary: %v", err)
		}

		// Now get it
		result, err := summarizer.GetLatestSummary(ctx, "session-get-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected result, got nil")
		}
		if result.SessionID != "session-get-1" {
			t.Errorf("expected session ID 'session-get-1', got %s", result.SessionID)
		}
	})

	t.Run("no summary exists", func(t *testing.T) {
		ctx := context.Background()

		result, err := summarizer.GetLatestSummary(ctx, "session-nonexistent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != nil {
			t.Error("expected nil result for nonexistent session")
		}
	})

	t.Run("no repository", func(t *testing.T) {
		summarizer, _ := NewSessionSummarizer(config, provider, nil)
		ctx := context.Background()

		_, err := summarizer.GetLatestSummary(ctx, "session-any")
		if err == nil {
			t.Error("expected error without repository")
		}
	})
}

func TestSessionSummarizer_GetMetrics(t *testing.T) {
	provider := newMockProvider()
	repo := newMockRepository()
	config := DefaultConfig()
	config.MinMessagesToSummarize = 4
	config.PreserveRecentCount = 2

	summarizer, _ := NewSessionSummarizer(config, provider, repo)

	t.Run("metrics after summarization", func(t *testing.T) {
		ctx := context.Background()

		// Create some summaries
		messages := createTestMessages(10)
		for i := 0; i < 3; i++ {
			_, err := summarizer.Summarize(ctx, "session-metrics-1", messages, TriggerReasonManual)
			if err != nil {
				t.Fatalf("failed to create summary: %v", err)
			}
		}

		metrics, err := summarizer.GetMetrics(ctx, "session-metrics-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if metrics == nil {
			t.Fatal("expected metrics, got nil")
		}
		if metrics.TotalSummarizations != 3 {
			t.Errorf("expected 3 summarizations, got %d", metrics.TotalSummarizations)
		}
		if metrics.TotalTokensSaved <= 0 {
			t.Error("expected positive tokens saved")
		}
		if metrics.LastSummarizationAt.IsZero() {
			t.Error("expected LastSummarizationAt to be set")
		}
	})

	t.Run("metrics for new session", func(t *testing.T) {
		ctx := context.Background()

		metrics, err := summarizer.GetMetrics(ctx, "session-new")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if metrics == nil {
			t.Fatal("expected metrics, got nil")
		}
		if metrics.TotalSummarizations != 0 {
			t.Errorf("expected 0 summarizations, got %d", metrics.TotalSummarizations)
		}
	})
}

func TestSessionSummarizer_BuildContextWithSummary(t *testing.T) {
	provider := newMockProvider()
	repo := newMockRepository()
	config := DefaultConfig()
	config.MinMessagesToSummarize = 4
	config.PreserveRecentCount = 2

	summarizer, _ := NewSessionSummarizer(config, provider, repo)

	t.Run("with existing summary", func(t *testing.T) {
		ctx := context.Background()

		// Create a summary first
		messages := createTestMessages(10)
		_, err := summarizer.Summarize(ctx, "session-context-1", messages, TriggerReasonManual)
		if err != nil {
			t.Fatalf("failed to create summary: %v", err)
		}

		// Build context with recent messages
		recentMessages := createTestMessages(3)
		result, err := summarizer.BuildContextWithSummary(ctx, "session-context-1", recentMessages)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have summary message + recent messages
		if len(result) != len(recentMessages)+1 {
			t.Errorf("expected %d messages, got %d", len(recentMessages)+1, len(result))
		}

		// First message should be system message with summary
		if result[0].Role != agent.RoleSystem {
			t.Errorf("expected first message to be system role, got %s", result[0].Role)
		}
	})

	t.Run("without existing summary", func(t *testing.T) {
		ctx := context.Background()

		recentMessages := createTestMessages(3)
		result, err := summarizer.BuildContextWithSummary(ctx, "session-no-summary", recentMessages)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have only recent messages
		if len(result) != len(recentMessages) {
			t.Errorf("expected %d messages, got %d", len(recentMessages), len(result))
		}
	})
}

func TestSessionSummarizer_WithStrategy(t *testing.T) {
	provider := newMockProvider()
	config := DefaultConfig()

	summarizer, _ := NewSessionSummarizer(config, provider, nil)

	// Custom strategy
	customStrategy := &SlidingWindowStrategy{}
	summarizer.WithStrategy(customStrategy)

	if summarizer.Strategy().Name() != "sliding_window" {
		t.Errorf("expected strategy name 'sliding_window', got %s", summarizer.Strategy().Name())
	}
}

func TestSessionSummarizer_WithPromptBuilder(t *testing.T) {
	provider := newMockProvider()
	config := DefaultConfig()

	summarizer, _ := NewSessionSummarizer(config, provider, nil)

	// Custom prompt builder
	customBuilder := NewPromptBuilder()
	summarizer.WithPromptBuilder(customBuilder)

	// Just verify it doesn't panic
	if summarizer == nil {
		t.Error("summarizer should not be nil")
	}
}

func TestSessionSummarizer_CostCalculation(t *testing.T) {
	provider := newMockProvider()
	provider.completeResp = &agent.Response{
		ID:    "test",
		Model: "mock-model",
		Content: []agent.ContentBlock{
			{Type: agent.ContentTypeText, Text: "Summary text"},
		},
		Usage: agent.Usage{
			PromptTokens:     10000,
			CompletionTokens: 2000,
		},
	}

	config := DefaultConfig()
	config.MinMessagesToSummarize = 4
	config.PreserveRecentCount = 2

	summarizer, _ := NewSessionSummarizer(config, provider, nil)

	ctx := context.Background()
	messages := createTestMessages(10)
	result, err := summarizer.Summarize(ctx, "session-cost", messages, TriggerReasonManual)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Cost should be calculated based on token usage
	if result.GenerationCostUSD <= 0 {
		t.Error("expected positive generation cost")
	}

	// Verify cost calculation: (10000 * 0.25 + 2000 * 1.25) / 1_000_000
	expectedCost := (10000.0*0.25 + 2000.0*1.25) / 1_000_000
	if result.GenerationCostUSD != expectedCost {
		t.Errorf("expected cost %f, got %f", expectedCost, result.GenerationCostUSD)
	}
}

func TestSessionSummarizer_CompressionRatio(t *testing.T) {
	provider := newMockProvider()
	config := DefaultConfig()
	config.MinMessagesToSummarize = 4
	config.PreserveRecentCount = 2

	summarizer, _ := NewSessionSummarizer(config, provider, nil)

	ctx := context.Background()
	messages := createTestMessages(10)
	result, err := summarizer.Summarize(ctx, "session-ratio", messages, TriggerReasonManual)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Compression ratio should be between 0 and 1 (ideally)
	if result.CompressionRatio < 0 {
		t.Errorf("compression ratio should not be negative, got %f", result.CompressionRatio)
	}

	// Tokens saved should be positive
	if result.TokensSaved < 0 {
		t.Errorf("tokens saved should not be negative, got %d", result.TokensSaved)
	}
}

func TestSessionSummarizer_Duration(t *testing.T) {
	provider := newMockProvider()
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		time.Sleep(10 * time.Millisecond) // Simulate API latency
		return &agent.Response{
			ID:    "test",
			Model: "mock-model",
			Content: []agent.ContentBlock{
				{Type: agent.ContentTypeText, Text: "Summary"},
			},
		}, nil
	}

	config := DefaultConfig()
	config.MinMessagesToSummarize = 4
	config.PreserveRecentCount = 2

	summarizer, _ := NewSessionSummarizer(config, provider, nil)

	ctx := context.Background()
	messages := createTestMessages(10)
	result, err := summarizer.Summarize(ctx, "session-duration", messages, TriggerReasonManual)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Duration should be at least our simulated delay
	if result.GenerationDuration < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", result.GenerationDuration)
	}
}
