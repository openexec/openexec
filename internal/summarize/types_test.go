package summarize

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if !config.Enabled {
		t.Error("expected Enabled to be true by default")
	}

	if config.SoftThreshold != 0.60 {
		t.Errorf("expected SoftThreshold 0.60, got %f", config.SoftThreshold)
	}

	if config.HardThreshold != 0.85 {
		t.Errorf("expected HardThreshold 0.85, got %f", config.HardThreshold)
	}

	if config.MaxMessagesBeforeSummary != 50 {
		t.Errorf("expected MaxMessagesBeforeSummary 50, got %d", config.MaxMessagesBeforeSummary)
	}

	if config.PreserveRecentCount != 10 {
		t.Errorf("expected PreserveRecentCount 10, got %d", config.PreserveRecentCount)
	}

	if config.MinMessagesToSummarize != 6 {
		t.Errorf("expected MinMessagesToSummarize 6, got %d", config.MinMessagesToSummarize)
	}

	if config.SummaryTargetTokens != 2000 {
		t.Errorf("expected SummaryTargetTokens 2000, got %d", config.SummaryTargetTokens)
	}

	if config.SummaryMaxTokens != 4000 {
		t.Errorf("expected SummaryMaxTokens 4000, got %d", config.SummaryMaxTokens)
	}

	if config.SummarizationModel != "claude-3-haiku-20240307" {
		t.Errorf("expected SummarizationModel claude-3-haiku-20240307, got %s", config.SummarizationModel)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name      string
		modify    func(*Config)
		expectErr bool
	}{
		{
			name:      "valid default config",
			modify:    func(c *Config) {},
			expectErr: false,
		},
		{
			name:      "invalid soft threshold zero",
			modify:    func(c *Config) { c.SoftThreshold = 0 },
			expectErr: true,
		},
		{
			name:      "invalid soft threshold one",
			modify:    func(c *Config) { c.SoftThreshold = 1.0 },
			expectErr: true,
		},
		{
			name:      "invalid hard threshold zero",
			modify:    func(c *Config) { c.HardThreshold = 0 },
			expectErr: true,
		},
		{
			name:      "soft threshold greater than hard",
			modify:    func(c *Config) { c.SoftThreshold = 0.9; c.HardThreshold = 0.5 },
			expectErr: true,
		},
		{
			name:      "preserve recent count too low",
			modify:    func(c *Config) { c.PreserveRecentCount = 1 },
			expectErr: true,
		},
		{
			name:      "min messages too low",
			modify:    func(c *Config) { c.MinMessagesToSummarize = 3 },
			expectErr: true,
		},
		{
			name:      "summary target tokens too low",
			modify:    func(c *Config) { c.SummaryTargetTokens = 100 },
			expectErr: true,
		},
		{
			name:      "summary target tokens too high",
			modify:    func(c *Config) { c.SummaryTargetTokens = 10000 },
			expectErr: true,
		},
		{
			name:      "summary max less than target",
			modify:    func(c *Config) { c.SummaryMaxTokens = 1000 },
			expectErr: true,
		},
		{
			name:      "empty summarization model",
			modify:    func(c *Config) { c.SummarizationModel = "" },
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			tt.modify(config)
			err := config.Validate()
			if tt.expectErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestTriggerReason(t *testing.T) {
	reasons := []TriggerReason{
		TriggerReasonTokenThreshold,
		TriggerReasonMessageCount,
		TriggerReasonPhaseBoundary,
		TriggerReasonManual,
		TriggerReasonSessionResume,
	}

	for _, reason := range reasons {
		if string(reason) == "" {
			t.Errorf("trigger reason should not be empty: %v", reason)
		}
	}
}

func TestTriggerCheckResult(t *testing.T) {
	result := TriggerCheckResult{
		ShouldSummarize:  true,
		Reason:           TriggerReasonTokenThreshold,
		CurrentTokens:    80000,
		ContextLimit:     128000,
		UsagePercent:     0.625,
		MessageCount:     45,
		IsUrgent:         false,
		EstimatedSavings: 40000,
	}

	if !result.ShouldSummarize {
		t.Error("expected ShouldSummarize to be true")
	}

	if result.Reason != TriggerReasonTokenThreshold {
		t.Errorf("expected reason TokenThreshold, got %s", result.Reason)
	}

	if result.UsagePercent != 0.625 {
		t.Errorf("expected usage percent 0.625, got %f", result.UsagePercent)
	}
}

func TestSummaryResult(t *testing.T) {
	result := SummaryResult{
		ID:                 "sum-123",
		SessionID:          "sess-456",
		Text:               "This is a summary of the conversation...",
		MessagesSummarized: 30,
		OriginalTokens:     15000,
		SummaryTokens:      2000,
		TokensSaved:        13000,
		CompressionRatio:   2000.0 / 15000.0,
		TriggerReason:      TriggerReasonTokenThreshold,
	}

	if result.TokensSaved != 13000 {
		t.Errorf("expected tokens saved 13000, got %d", result.TokensSaved)
	}

	expectedRatio := 2000.0 / 15000.0
	if result.CompressionRatio != expectedRatio {
		t.Errorf("expected compression ratio %f, got %f", expectedRatio, result.CompressionRatio)
	}
}
