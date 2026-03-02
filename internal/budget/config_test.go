package budget

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	// Check defaults
	if cfg.Enabled {
		t.Error("Enabled should be false by default")
	}
	if cfg.TotalBudgetUSD != 100.0 {
		t.Errorf("TotalBudgetUSD = %v, want 100.0", cfg.TotalBudgetUSD)
	}
	if cfg.SessionBudgetUSD != 10.0 {
		t.Errorf("SessionBudgetUSD = %v, want 10.0", cfg.SessionBudgetUSD)
	}
	if cfg.DailyBudgetUSD != 25.0 {
		t.Errorf("DailyBudgetUSD = %v, want 25.0", cfg.DailyBudgetUSD)
	}
	if cfg.WarningThreshold != 0.8 {
		t.Errorf("WarningThreshold = %v, want 0.8", cfg.WarningThreshold)
	}
	if cfg.CriticalThreshold != 0.95 {
		t.Errorf("CriticalThreshold = %v, want 0.95", cfg.CriticalThreshold)
	}
	if cfg.BlockOnExceed {
		t.Error("BlockOnExceed should be false by default")
	}
	if cfg.AlertCooldown != 5*time.Minute {
		t.Errorf("AlertCooldown = %v, want 5m", cfg.AlertCooldown)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "disabled config - valid",
			config:  &Config{Enabled: false},
			wantErr: false,
		},
		{
			name: "enabled with all budgets - valid",
			config: &Config{
				Enabled:           true,
				TotalBudgetUSD:    100,
				SessionBudgetUSD:  10,
				DailyBudgetUSD:    25,
				WarningThreshold:  0.8,
				CriticalThreshold: 0.95,
			},
			wantErr: false,
		},
		{
			name: "enabled with total budget only - valid",
			config: &Config{
				Enabled:           true,
				TotalBudgetUSD:    100,
				WarningThreshold:  0.8,
				CriticalThreshold: 0.95,
			},
			wantErr: false,
		},
		{
			name: "enabled with no budgets - invalid",
			config: &Config{
				Enabled:           true,
				WarningThreshold:  0.8,
				CriticalThreshold: 0.95,
			},
			wantErr: true,
		},
		{
			name: "invalid warning threshold (negative)",
			config: &Config{
				Enabled:           true,
				TotalBudgetUSD:    100,
				WarningThreshold:  -0.1,
				CriticalThreshold: 0.95,
			},
			wantErr: true,
		},
		{
			name: "invalid warning threshold (> 1)",
			config: &Config{
				Enabled:           true,
				TotalBudgetUSD:    100,
				WarningThreshold:  1.1,
				CriticalThreshold: 0.95,
			},
			wantErr: true,
		},
		{
			name: "warning >= critical - invalid",
			config: &Config{
				Enabled:           true,
				TotalBudgetUSD:    100,
				WarningThreshold:  0.95,
				CriticalThreshold: 0.95,
			},
			wantErr: true,
		},
		{
			name: "warning > critical - invalid",
			config: &Config{
				Enabled:           true,
				TotalBudgetUSD:    100,
				WarningThreshold:  0.98,
				CriticalThreshold: 0.95,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckThreshold(t *testing.T) {
	cfg := &Config{
		WarningThreshold:  0.8,
		CriticalThreshold: 0.95,
	}

	tests := []struct {
		name         string
		currentSpend float64
		budgetLimit  float64
		want         AlertThreshold
	}{
		{
			name:         "zero budget limit",
			currentSpend: 50,
			budgetLimit:  0,
			want:         ThresholdNone,
		},
		{
			name:         "below warning",
			currentSpend: 70,
			budgetLimit:  100,
			want:         ThresholdNone,
		},
		{
			name:         "at warning",
			currentSpend: 80,
			budgetLimit:  100,
			want:         ThresholdWarning,
		},
		{
			name:         "between warning and critical",
			currentSpend: 90,
			budgetLimit:  100,
			want:         ThresholdWarning,
		},
		{
			name:         "at critical",
			currentSpend: 95,
			budgetLimit:  100,
			want:         ThresholdCritical,
		},
		{
			name:         "between critical and exceeded",
			currentSpend: 99,
			budgetLimit:  100,
			want:         ThresholdCritical,
		},
		{
			name:         "at exceeded",
			currentSpend: 100,
			budgetLimit:  100,
			want:         ThresholdExceeded,
		},
		{
			name:         "over exceeded",
			currentSpend: 150,
			budgetLimit:  100,
			want:         ThresholdExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cfg.CheckThreshold(tt.currentSpend, tt.budgetLimit)
			if got != tt.want {
				t.Errorf("CheckThreshold(%v, %v) = %v, want %v", tt.currentSpend, tt.budgetLimit, got, tt.want)
			}
		})
	}
}

func TestGetWarningBudget(t *testing.T) {
	cfg := &Config{
		WarningThreshold: 0.8,
	}

	got := cfg.GetWarningBudget(100)
	want := 80.0

	if got != want {
		t.Errorf("GetWarningBudget(100) = %v, want %v", got, want)
	}
}

func TestGetCriticalBudget(t *testing.T) {
	cfg := &Config{
		CriticalThreshold: 0.95,
	}

	got := cfg.GetCriticalBudget(100)
	want := 95.0

	if got != want {
		t.Errorf("GetCriticalBudget(100) = %v, want %v", got, want)
	}
}

func TestNewStatus(t *testing.T) {
	cfg := &Config{
		Enabled:           true,
		TotalBudgetUSD:    100,
		DailyBudgetUSD:    25,
		SessionBudgetUSD:  10,
		WarningThreshold:  0.8,
		CriticalThreshold: 0.95,
		BlockOnExceed:     true,
	}

	tests := []struct {
		name         string
		totalSpent   float64
		dailySpent   float64
		sessionSpent float64
		wantBlocked  bool
		wantReason   string
	}{
		{
			name:         "all under budget",
			totalSpent:   50,
			dailySpent:   10,
			sessionSpent: 5,
			wantBlocked:  false,
		},
		{
			name:         "total exceeded",
			totalSpent:   100,
			dailySpent:   10,
			sessionSpent: 5,
			wantBlocked:  true,
			wantReason:   "Total budget limit exceeded",
		},
		{
			name:         "daily exceeded",
			totalSpent:   50,
			dailySpent:   25,
			sessionSpent: 5,
			wantBlocked:  true,
			wantReason:   "Daily budget limit exceeded",
		},
		{
			name:         "session exceeded",
			totalSpent:   50,
			dailySpent:   10,
			sessionSpent: 10,
			wantBlocked:  true,
			wantReason:   "Session budget limit exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := NewStatus(cfg, tt.totalSpent, tt.dailySpent, tt.sessionSpent)

			if status.IsBlocked != tt.wantBlocked {
				t.Errorf("IsBlocked = %v, want %v", status.IsBlocked, tt.wantBlocked)
			}
			if status.BlockReason != tt.wantReason {
				t.Errorf("BlockReason = %q, want %q", status.BlockReason, tt.wantReason)
			}
		})
	}
}

func TestStatusHighestThreshold(t *testing.T) {
	tests := []struct {
		name   string
		status *Status
		want   AlertThreshold
	}{
		{
			name: "all none",
			status: &Status{
				TotalThreshold:   ThresholdNone,
				DailyThreshold:   ThresholdNone,
				SessionThreshold: ThresholdNone,
			},
			want: ThresholdNone,
		},
		{
			name: "total warning",
			status: &Status{
				TotalThreshold:   ThresholdWarning,
				DailyThreshold:   ThresholdNone,
				SessionThreshold: ThresholdNone,
			},
			want: ThresholdWarning,
		},
		{
			name: "daily critical",
			status: &Status{
				TotalThreshold:   ThresholdWarning,
				DailyThreshold:   ThresholdCritical,
				SessionThreshold: ThresholdNone,
			},
			want: ThresholdCritical,
		},
		{
			name: "session exceeded",
			status: &Status{
				TotalThreshold:   ThresholdWarning,
				DailyThreshold:   ThresholdCritical,
				SessionThreshold: ThresholdExceeded,
			},
			want: ThresholdExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.HighestThreshold()
			if got != tt.want {
				t.Errorf("HighestThreshold() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatusPercentages(t *testing.T) {
	cfg := &Config{
		Enabled:           true,
		TotalBudgetUSD:    100,
		DailyBudgetUSD:    50,
		SessionBudgetUSD:  10,
		WarningThreshold:  0.8,
		CriticalThreshold: 0.95,
	}

	status := NewStatus(cfg, 50, 25, 5)

	// Check percentage calculations
	if status.TotalPercentUsed != 50 {
		t.Errorf("TotalPercentUsed = %v, want 50", status.TotalPercentUsed)
	}
	if status.DailyPercentUsed != 50 {
		t.Errorf("DailyPercentUsed = %v, want 50", status.DailyPercentUsed)
	}
	if status.SessionPercentUsed != 50 {
		t.Errorf("SessionPercentUsed = %v, want 50", status.SessionPercentUsed)
	}
}
