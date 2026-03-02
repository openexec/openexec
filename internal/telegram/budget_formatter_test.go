package telegram

import (
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/budget"
)

func TestNewBudgetFormatter(t *testing.T) {
	f := NewBudgetFormatter()

	if f == nil {
		t.Fatal("NewBudgetFormatter() returned nil")
	}

	if !f.includeEmojis {
		t.Error("includeEmojis should be true by default")
	}

	if f.progressBarWidth != 20 {
		t.Errorf("progressBarWidth = %v, want 20", f.progressBarWidth)
	}
}

func TestBudgetFormatterFormatAlert(t *testing.T) {
	f := NewBudgetFormatter()

	tests := []struct {
		name            string
		alert           *budget.Alert
		wantTitle       string
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:      "nil alert",
			alert:     nil,
			wantTitle: "Budget Alert",
		},
		{
			name: "warning alert",
			alert: &budget.Alert{
				ID:           "test-1",
				Type:         budget.AlertTypeTotal,
				Threshold:    budget.ThresholdWarning,
				SessionID:    "session-123",
				CurrentSpend: 85,
				BudgetLimit:  100,
				PercentUsed:  85,
				Message:      "Total budget warning",
				CreatedAt:    time.Now(),
			},
			wantTitle:    EmojiBudgetWarning + " Budget Warning",
			wantContains: []string{"$85.00", "$100.00", "85.0%", "total"},
		},
		{
			name: "critical alert",
			alert: &budget.Alert{
				ID:           "test-2",
				Type:         budget.AlertTypeDaily,
				Threshold:    budget.ThresholdCritical,
				CurrentSpend: 23.5,
				BudgetLimit:  25,
				PercentUsed:  94,
				Message:      "Daily budget critical",
				CreatedAt:    time.Now(),
			},
			wantTitle:    EmojiBudgetCritical + " Budget Critical",
			wantContains: []string{"$23.50", "$25.00", "94.0%", "daily"},
		},
		{
			name: "exceeded alert",
			alert: &budget.Alert{
				ID:           "test-3",
				Type:         budget.AlertTypeSession,
				Threshold:    budget.ThresholdExceeded,
				SessionID:    "session-456",
				CurrentSpend: 12,
				BudgetLimit:  10,
				PercentUsed:  120,
				Message:      "Session budget exceeded",
				CreatedAt:    time.Now(),
			},
			wantTitle:    EmojiBudgetExceeded + " Budget Exceeded",
			wantContains: []string{"$12.00", "$10.00", "120.0%", "session"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := f.FormatAlert(tt.alert)

			if result == nil {
				t.Fatal("FormatAlert() returned nil")
			}

			if result.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", result.Title, tt.wantTitle)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(strings.ToLower(result.Body), strings.ToLower(want)) {
					t.Errorf("Body does not contain %q: %s", want, result.Body)
				}
			}

			for _, notWant := range tt.wantNotContains {
				if strings.Contains(result.Body, notWant) {
					t.Errorf("Body should not contain %q: %s", notWant, result.Body)
				}
			}
		})
	}
}

func TestBudgetFormatterFormatAlertNoEmojis(t *testing.T) {
	f := NewBudgetFormatter()
	f.SetIncludeEmojis(false)

	alert := &budget.Alert{
		ID:           "test-1",
		Type:         budget.AlertTypeTotal,
		Threshold:    budget.ThresholdWarning,
		CurrentSpend: 85,
		BudgetLimit:  100,
		PercentUsed:  85,
		Message:      "Total budget warning",
		CreatedAt:    time.Now(),
	}

	result := f.FormatAlert(alert)

	if strings.Contains(result.Title, EmojiBudgetWarning) {
		t.Error("Title should not contain emoji when disabled")
	}

	if strings.Contains(result.Body, EmojiProgress) {
		t.Error("Body should not contain emoji progress bar when disabled")
	}
}

func TestBudgetFormatterFormatStatus(t *testing.T) {
	f := NewBudgetFormatter()

	tests := []struct {
		name         string
		status       *budget.Status
		wantContains []string
	}{
		{
			name:   "nil status",
			status: nil,
		},
		{
			name: "full status",
			status: &budget.Status{
				TotalSpentUSD:      50,
				TotalBudgetUSD:     100,
				TotalPercentUsed:   50,
				TotalThreshold:     budget.ThresholdNone,
				DailySpentUSD:      15,
				DailyBudgetUSD:     25,
				DailyPercentUsed:   60,
				DailyThreshold:     budget.ThresholdNone,
				SessionSpentUSD:    5,
				SessionBudgetUSD:   10,
				SessionPercentUsed: 50,
				SessionThreshold:   budget.ThresholdNone,
				IsBlocked:          false,
				CheckedAt:          time.Now(),
			},
			wantContains: []string{"$50.00", "$100.00", "$15.00", "$25.00", "$5.00", "$10.00"},
		},
		{
			name: "blocked status",
			status: &budget.Status{
				TotalSpentUSD:    100,
				TotalBudgetUSD:   100,
				TotalPercentUsed: 100,
				TotalThreshold:   budget.ThresholdExceeded,
				IsBlocked:        true,
				BlockReason:      "Total budget limit exceeded",
				CheckedAt:        time.Now(),
			},
			wantContains: []string{"BLOCKED", "Total budget limit exceeded"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := f.FormatStatus(tt.status)

			if tt.status == nil {
				if !strings.Contains(result, "No budget status available") {
					t.Error("Expected 'No budget status available' for nil status")
				}
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("Result does not contain %q: %s", want, result)
				}
			}
		})
	}
}

func TestBudgetFormatterProgressBar(t *testing.T) {
	f := NewBudgetFormatter()

	tests := []struct {
		name        string
		percentUsed float64
		wantFilled  int
	}{
		{
			name:        "0%",
			percentUsed: 0,
			wantFilled:  0,
		},
		{
			name:        "50%",
			percentUsed: 50,
			wantFilled:  10,
		},
		{
			name:        "100%",
			percentUsed: 100,
			wantFilled:  20,
		},
		{
			name:        "over 100%",
			percentUsed: 150,
			wantFilled:  20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := f.formatProgressBar(tt.percentUsed)

			filled := strings.Count(result, EmojiProgress)
			if filled != tt.wantFilled {
				t.Errorf("Progress bar has %d filled, want %d: %s", filled, tt.wantFilled, result)
			}

			totalChars := filled + strings.Count(result, EmojiProgressEmpty)
			if totalChars != f.progressBarWidth {
				t.Errorf("Progress bar total width = %d, want %d", totalChars, f.progressBarWidth)
			}
		})
	}
}

func TestBudgetFormatterProgressBarASCII(t *testing.T) {
	f := NewBudgetFormatter()
	f.SetIncludeEmojis(false)

	result := f.formatProgressBar(50)

	if !strings.HasPrefix(result, "[") || !strings.HasSuffix(result, "]") {
		t.Errorf("ASCII progress bar should be wrapped in brackets: %s", result)
	}

	filled := strings.Count(result, "#")
	if filled != 10 {
		t.Errorf("ASCII progress bar has %d filled, want 10: %s", filled, result)
	}
}

func TestFormatBudgetAlertConvenience(t *testing.T) {
	alert := &budget.Alert{
		ID:           "test-1",
		Type:         budget.AlertTypeTotal,
		Threshold:    budget.ThresholdWarning,
		CurrentSpend: 85,
		BudgetLimit:  100,
		PercentUsed:  85,
		Message:      "Total budget warning",
		CreatedAt:    time.Now(),
	}

	result := FormatBudgetAlert(alert)

	if result == "" {
		t.Error("FormatBudgetAlert() returned empty string")
	}

	if !strings.Contains(result, "$85.00") {
		t.Error("Result should contain spend amount")
	}
}

func TestFormatBudgetStatusConvenience(t *testing.T) {
	status := &budget.Status{
		TotalSpentUSD:    50,
		TotalBudgetUSD:   100,
		TotalPercentUsed: 50,
		CheckedAt:        time.Now(),
	}

	result := FormatBudgetStatus(status)

	if result == "" {
		t.Error("FormatBudgetStatus() returned empty string")
	}

	if !strings.Contains(result, "$50.00") {
		t.Error("Result should contain spend amount")
	}
}

func TestFormattedBudgetAlertSeverity(t *testing.T) {
	f := NewBudgetFormatter()

	tests := []struct {
		threshold budget.AlertThreshold
		wantStr   string
	}{
		{budget.ThresholdWarning, "warning"},
		{budget.ThresholdCritical, "critical"},
		{budget.ThresholdExceeded, "critical"},
		{budget.ThresholdNone, "info"},
	}

	for _, tt := range tests {
		alert := &budget.Alert{
			ID:           "test",
			Type:         budget.AlertTypeTotal,
			Threshold:    tt.threshold,
			CurrentSpend: 80,
			BudgetLimit:  100,
			PercentUsed:  80,
			Message:      "test",
			CreatedAt:    time.Now(),
		}

		result := f.FormatAlert(alert)

		if string(result.Severity) != tt.wantStr {
			t.Errorf("Severity for threshold %v = %v, want %v", tt.threshold, result.Severity, tt.wantStr)
		}
	}
}
