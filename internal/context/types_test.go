package context

import (
	"database/sql"
	"testing"
	"time"
)

func TestContextType_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		ct       ContextType
		expected bool
	}{
		{"valid project_instructions", ContextTypeProjectInstructions, true},
		{"valid git_status", ContextTypeGitStatus, true},
		{"valid git_diff", ContextTypeGitDiff, true},
		{"valid git_log", ContextTypeGitLog, true},
		{"valid directory_structure", ContextTypeDirectoryStructure, true},
		{"valid recent_files", ContextTypeRecentFiles, true},
		{"valid open_files", ContextTypeOpenFiles, true},
		{"valid package_info", ContextTypePackageInfo, true},
		{"valid environment", ContextTypeEnvironment, true},
		{"valid session_summary", ContextTypeSessionSummary, true},
		{"valid custom", ContextTypeCustom, true},
		{"invalid empty", ContextType(""), false},
		{"invalid unknown", ContextType("unknown_type"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ct.IsValid(); got != tt.expected {
				t.Errorf("ContextType.IsValid() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestContextType_String(t *testing.T) {
	tests := []struct {
		ct       ContextType
		expected string
	}{
		{ContextTypeProjectInstructions, "project_instructions"},
		{ContextTypeGitStatus, "git_status"},
		{ContextTypeCustom, "custom"},
	}

	for _, tt := range tests {
		if got := tt.ct.String(); got != tt.expected {
			t.Errorf("ContextType.String() = %v, want %v", got, tt.expected)
		}
	}
}

func TestDefaultPriorityForType(t *testing.T) {
	tests := []struct {
		ct       ContextType
		expected Priority
	}{
		{ContextTypeProjectInstructions, PriorityCritical},
		{ContextTypeSessionSummary, PriorityCritical},
		{ContextTypeGitStatus, PriorityHigh},
		{ContextTypeEnvironment, PriorityHigh},
		{ContextTypePackageInfo, PriorityMedium},
		{ContextTypeDirectoryStructure, PriorityMedium},
		{ContextTypeRecentFiles, PriorityMedium},
		{ContextTypeGitDiff, PriorityLow},
		{ContextTypeGitLog, PriorityLow},
		{ContextTypeOpenFiles, PriorityLow},
		{ContextTypeCustom, PriorityMedium},
		{ContextType("unknown"), PriorityOptional},
	}

	for _, tt := range tests {
		t.Run(string(tt.ct), func(t *testing.T) {
			if got := DefaultPriorityForType(tt.ct); got != tt.expected {
				t.Errorf("DefaultPriorityForType(%v) = %v, want %v", tt.ct, got, tt.expected)
			}
		})
	}
}

func TestGathererStatus_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		status   GathererStatus
		expected bool
	}{
		{"valid idle", GathererStatusIdle, true},
		{"valid running", GathererStatusRunning, true},
		{"valid completed", GathererStatusCompleted, true},
		{"valid failed", GathererStatusFailed, true},
		{"valid disabled", GathererStatusDisabled, true},
		{"invalid empty", GathererStatus(""), false},
		{"invalid unknown", GathererStatus("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.expected {
				t.Errorf("GathererStatus.IsValid() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNewContextItem(t *testing.T) {
	tests := []struct {
		name        string
		contextType ContextType
		source      string
		content     string
		tokenCount  int
		wantErr     bool
	}{
		{
			name:        "valid context item",
			contextType: ContextTypeGitStatus,
			source:      "git status",
			content:     "On branch main\nnothing to commit",
			tokenCount:  10,
			wantErr:     false,
		},
		{
			name:        "empty content is valid",
			contextType: ContextTypeGitDiff,
			source:      "git diff",
			content:     "",
			tokenCount:  0,
			wantErr:     false,
		},
		{
			name:        "invalid context type",
			contextType: ContextType("invalid"),
			source:      "test",
			content:     "content",
			tokenCount:  5,
			wantErr:     true,
		},
		{
			name:        "empty source",
			contextType: ContextTypeGitStatus,
			source:      "",
			content:     "content",
			tokenCount:  5,
			wantErr:     true,
		},
		{
			name:        "negative token count",
			contextType: ContextTypeGitStatus,
			source:      "test",
			content:     "content",
			tokenCount:  -1,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item, err := NewContextItem(tt.contextType, tt.source, tt.content, tt.tokenCount)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewContextItem() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if item.ID == "" {
					t.Error("NewContextItem() ID should not be empty")
				}
				if item.Type != tt.contextType {
					t.Errorf("NewContextItem() Type = %v, want %v", item.Type, tt.contextType)
				}
				if item.Source != tt.source {
					t.Errorf("NewContextItem() Source = %v, want %v", item.Source, tt.source)
				}
				if item.Content != tt.content {
					t.Errorf("NewContextItem() Content = %v, want %v", item.Content, tt.content)
				}
				if item.TokenCount != tt.tokenCount {
					t.Errorf("NewContextItem() TokenCount = %v, want %v", item.TokenCount, tt.tokenCount)
				}
				if item.Priority != DefaultPriorityForType(tt.contextType) {
					t.Errorf("NewContextItem() Priority = %v, want %v", item.Priority, DefaultPriorityForType(tt.contextType))
				}
				if item.IsStale {
					t.Error("NewContextItem() IsStale should be false")
				}
				if item.ContentHash == "" && tt.content != "" {
					t.Error("NewContextItem() ContentHash should not be empty for non-empty content")
				}
			}
		})
	}
}

func TestContextItem_Validate(t *testing.T) {
	validItem := &ContextItem{
		ID:         "test-id",
		Type:       ContextTypeGitStatus,
		Source:     "git status",
		Content:    "test content",
		TokenCount: 10,
		Priority:   PriorityHigh,
	}

	if err := validItem.Validate(); err != nil {
		t.Errorf("Validate() unexpected error = %v", err)
	}

	tests := []struct {
		name    string
		modify  func(*ContextItem)
		wantErr bool
	}{
		{
			name:    "valid item",
			modify:  func(c *ContextItem) {},
			wantErr: false,
		},
		{
			name:    "empty id",
			modify:  func(c *ContextItem) { c.ID = "" },
			wantErr: true,
		},
		{
			name:    "invalid type",
			modify:  func(c *ContextItem) { c.Type = ContextType("invalid") },
			wantErr: true,
		},
		{
			name:    "empty source",
			modify:  func(c *ContextItem) { c.Source = "" },
			wantErr: true,
		},
		{
			name:    "negative token count",
			modify:  func(c *ContextItem) { c.TokenCount = -1 },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := &ContextItem{
				ID:         "test-id",
				Type:       ContextTypeGitStatus,
				Source:     "git status",
				Content:    "test content",
				TokenCount: 10,
				Priority:   PriorityHigh,
			}
			tt.modify(item)
			err := item.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContextItem_SetSessionID(t *testing.T) {
	item := &ContextItem{ID: "test-id", Type: ContextTypeGitStatus, Source: "test"}

	item.SetSessionID("session-123")
	if !item.SessionID.Valid || item.SessionID.String != "session-123" {
		t.Errorf("SetSessionID() SessionID = %v, want session-123", item.SessionID)
	}

	item.SetSessionID("")
	if item.SessionID.Valid {
		t.Error("SetSessionID('') should set Valid to false")
	}
}

func TestContextItem_SetExpiration(t *testing.T) {
	item := &ContextItem{ID: "test-id", Type: ContextTypeGitStatus, Source: "test"}
	expTime := time.Now().Add(time.Hour)

	item.SetExpiration(expTime)
	if !item.ExpiresAt.Valid {
		t.Error("SetExpiration() ExpiresAt should be valid")
	}
	// Allow 1 second tolerance for test execution time
	if item.ExpiresAt.Time.Sub(expTime.UTC()) > time.Second {
		t.Errorf("SetExpiration() ExpiresAt = %v, want %v", item.ExpiresAt.Time, expTime.UTC())
	}
}

func TestContextItem_MarkStale(t *testing.T) {
	item := &ContextItem{ID: "test-id", Type: ContextTypeGitStatus, Source: "test", IsStale: false}

	item.MarkStale()
	if !item.IsStale {
		t.Error("MarkStale() IsStale should be true")
	}
}

func TestContextItem_Refresh(t *testing.T) {
	item := &ContextItem{
		ID:          "test-id",
		Type:        ContextTypeGitStatus,
		Source:      "test",
		Content:     "old content",
		ContentHash: "old-hash",
		TokenCount:  5,
		IsStale:     true,
	}

	item.Refresh("new content", 10)

	if item.Content != "new content" {
		t.Errorf("Refresh() Content = %v, want 'new content'", item.Content)
	}
	if item.TokenCount != 10 {
		t.Errorf("Refresh() TokenCount = %v, want 10", item.TokenCount)
	}
	if item.IsStale {
		t.Error("Refresh() IsStale should be false")
	}
	if item.ContentHash == "old-hash" {
		t.Error("Refresh() ContentHash should be updated")
	}
}

func TestContextItem_IsExpired(t *testing.T) {
	tests := []struct {
		name     string
		item     ContextItem
		expected bool
	}{
		{
			name:     "no expiration set",
			item:     ContextItem{ID: "test", ExpiresAt: sql.NullTime{Valid: false}},
			expected: false,
		},
		{
			name: "future expiration",
			item: ContextItem{
				ID:        "test",
				ExpiresAt: sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true},
			},
			expected: false,
		},
		{
			name: "past expiration",
			item: ContextItem{
				ID:        "test",
				ExpiresAt: sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.item.IsExpired(); got != tt.expected {
				t.Errorf("IsExpired() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestContextItem_NeedsRefresh(t *testing.T) {
	tests := []struct {
		name     string
		item     ContextItem
		expected bool
	}{
		{
			name:     "stale item",
			item:     ContextItem{ID: "test", IsStale: true},
			expected: true,
		},
		{
			name: "expired item",
			item: ContextItem{
				ID:        "test",
				IsStale:   false,
				ExpiresAt: sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
			},
			expected: true,
		},
		{
			name: "fresh item",
			item: ContextItem{
				ID:        "test",
				IsStale:   false,
				ExpiresAt: sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.item.NeedsRefresh(); got != tt.expected {
				t.Errorf("NeedsRefresh() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNewGathererConfig(t *testing.T) {
	tests := []struct {
		name        string
		contextType ContextType
		configName  string
		maxTokens   int
		wantErr     bool
	}{
		{
			name:        "valid config",
			contextType: ContextTypeGitStatus,
			configName:  "Git Status Gatherer",
			maxTokens:   2000,
			wantErr:     false,
		},
		{
			name:        "zero max tokens",
			contextType: ContextTypeGitStatus,
			configName:  "Test",
			maxTokens:   0,
			wantErr:     false,
		},
		{
			name:        "invalid context type",
			contextType: ContextType("invalid"),
			configName:  "Test",
			maxTokens:   1000,
			wantErr:     true,
		},
		{
			name:        "empty name",
			contextType: ContextTypeGitStatus,
			configName:  "",
			maxTokens:   1000,
			wantErr:     true,
		},
		{
			name:        "negative max tokens",
			contextType: ContextTypeGitStatus,
			configName:  "Test",
			maxTokens:   -1,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := NewGathererConfig(tt.contextType, tt.configName, tt.maxTokens)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewGathererConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if config.ID == "" {
					t.Error("NewGathererConfig() ID should not be empty")
				}
				if config.Type != tt.contextType {
					t.Errorf("NewGathererConfig() Type = %v, want %v", config.Type, tt.contextType)
				}
				if config.Name != tt.configName {
					t.Errorf("NewGathererConfig() Name = %v, want %v", config.Name, tt.configName)
				}
				if config.MaxTokens != tt.maxTokens {
					t.Errorf("NewGathererConfig() MaxTokens = %v, want %v", config.MaxTokens, tt.maxTokens)
				}
				if !config.IsEnabled {
					t.Error("NewGathererConfig() IsEnabled should be true by default")
				}
				if config.RefreshIntervalSeconds != 300 {
					t.Errorf("NewGathererConfig() RefreshIntervalSeconds = %v, want 300", config.RefreshIntervalSeconds)
				}
			}
		})
	}
}

func TestGathererConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*GathererConfig)
		wantErr bool
	}{
		{
			name:    "valid config",
			modify:  func(g *GathererConfig) {},
			wantErr: false,
		},
		{
			name:    "empty id",
			modify:  func(g *GathererConfig) { g.ID = "" },
			wantErr: true,
		},
		{
			name:    "invalid type",
			modify:  func(g *GathererConfig) { g.Type = ContextType("invalid") },
			wantErr: true,
		},
		{
			name:    "empty name",
			modify:  func(g *GathererConfig) { g.Name = "" },
			wantErr: true,
		},
		{
			name:    "negative max tokens",
			modify:  func(g *GathererConfig) { g.MaxTokens = -1 },
			wantErr: true,
		},
		{
			name:    "negative refresh interval",
			modify:  func(g *GathererConfig) { g.RefreshIntervalSeconds = -1 },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &GathererConfig{
				ID:                     "test-id",
				Type:                   ContextTypeGitStatus,
				Name:                   "Test Gatherer",
				MaxTokens:              1000,
				RefreshIntervalSeconds: 300,
			}
			tt.modify(config)
			err := config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGathererConfig_SetProjectPath(t *testing.T) {
	config := &GathererConfig{ID: "test-id", Type: ContextTypeGitStatus, Name: "Test"}

	config.SetProjectPath("/project/path")
	if !config.ProjectPath.Valid || config.ProjectPath.String != "/project/path" {
		t.Errorf("SetProjectPath() ProjectPath = %v, want /project/path", config.ProjectPath)
	}

	config.SetProjectPath("")
	if config.ProjectPath.Valid {
		t.Error("SetProjectPath('') should set Valid to false")
	}
}

func TestGathererConfig_EnableDisable(t *testing.T) {
	config := &GathererConfig{ID: "test-id", Type: ContextTypeGitStatus, Name: "Test", IsEnabled: false}

	config.Enable()
	if !config.IsEnabled {
		t.Error("Enable() IsEnabled should be true")
	}

	config.Disable()
	if config.IsEnabled {
		t.Error("Disable() IsEnabled should be false")
	}
}

func TestGathererConfig_GetRefreshInterval(t *testing.T) {
	config := &GathererConfig{RefreshIntervalSeconds: 300}
	expected := 5 * time.Minute

	if got := config.GetRefreshInterval(); got != expected {
		t.Errorf("GetRefreshInterval() = %v, want %v", got, expected)
	}
}

func TestNewContextBudget(t *testing.T) {
	tests := []struct {
		name             string
		totalTokenBudget int
		wantErr          bool
	}{
		{"valid budget", 128000, false},
		{"zero budget", 0, false},
		{"negative budget", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			budget, err := NewContextBudget(tt.totalTokenBudget)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewContextBudget() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if budget.ID == "" {
					t.Error("NewContextBudget() ID should not be empty")
				}
				if budget.TotalTokenBudget != tt.totalTokenBudget {
					t.Errorf("NewContextBudget() TotalTokenBudget = %v, want %v", budget.TotalTokenBudget, tt.totalTokenBudget)
				}
				if budget.ReservedForSystemPrompt != 1000 {
					t.Errorf("NewContextBudget() ReservedForSystemPrompt = %v, want 1000", budget.ReservedForSystemPrompt)
				}
				if budget.ReservedForConversation != 4000 {
					t.Errorf("NewContextBudget() ReservedForConversation = %v, want 4000", budget.ReservedForConversation)
				}
			}
		})
	}
}

func TestContextBudget_Validate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*ContextBudget)
		wantErr bool
	}{
		{
			name:    "valid budget",
			modify:  func(b *ContextBudget) {},
			wantErr: false,
		},
		{
			name:    "empty id",
			modify:  func(b *ContextBudget) { b.ID = "" },
			wantErr: true,
		},
		{
			name:    "negative total budget",
			modify:  func(b *ContextBudget) { b.TotalTokenBudget = -1 },
			wantErr: true,
		},
		{
			name:    "negative system prompt reservation",
			modify:  func(b *ContextBudget) { b.ReservedForSystemPrompt = -1 },
			wantErr: true,
		},
		{
			name:    "negative conversation reservation",
			modify:  func(b *ContextBudget) { b.ReservedForConversation = -1 },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			budget := &ContextBudget{
				ID:                      "test-id",
				TotalTokenBudget:        128000,
				ReservedForSystemPrompt: 2000,
				ReservedForConversation: 32000,
			}
			tt.modify(budget)
			err := budget.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContextBudget_AvailableForContext(t *testing.T) {
	tests := []struct {
		name     string
		budget   ContextBudget
		expected int
	}{
		{
			name: "normal budget",
			budget: ContextBudget{
				TotalTokenBudget:        128000,
				ReservedForSystemPrompt: 2000,
				ReservedForConversation: 32000,
			},
			expected: 94000,
		},
		{
			name: "reservations exceed total",
			budget: ContextBudget{
				TotalTokenBudget:        10000,
				ReservedForSystemPrompt: 8000,
				ReservedForConversation: 8000,
			},
			expected: 0,
		},
		{
			name: "zero total",
			budget: ContextBudget{
				TotalTokenBudget:        0,
				ReservedForSystemPrompt: 0,
				ReservedForConversation: 0,
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.budget.AvailableForContext(); got != tt.expected {
				t.Errorf("AvailableForContext() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNewGathererExecution(t *testing.T) {
	tests := []struct {
		name       string
		gathererID string
		wantErr    bool
	}{
		{"valid execution", "gatherer-123", false},
		{"empty gatherer id", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec, err := NewGathererExecution(tt.gathererID)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewGathererExecution() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if exec.ID == "" {
					t.Error("NewGathererExecution() ID should not be empty")
				}
				if exec.GathererID != tt.gathererID {
					t.Errorf("NewGathererExecution() GathererID = %v, want %v", exec.GathererID, tt.gathererID)
				}
				if exec.Status != GathererStatusRunning {
					t.Errorf("NewGathererExecution() Status = %v, want running", exec.Status)
				}
			}
		})
	}
}

func TestGathererExecution_Complete(t *testing.T) {
	exec, _ := NewGathererExecution("gatherer-123")
	time.Sleep(10 * time.Millisecond) // Small delay to ensure duration > 0

	exec.Complete("context-item-456", 1500)

	if exec.Status != GathererStatusCompleted {
		t.Errorf("Complete() Status = %v, want completed", exec.Status)
	}
	if !exec.ContextItemID.Valid || exec.ContextItemID.String != "context-item-456" {
		t.Errorf("Complete() ContextItemID = %v, want context-item-456", exec.ContextItemID)
	}
	if exec.TokensGathered != 1500 {
		t.Errorf("Complete() TokensGathered = %v, want 1500", exec.TokensGathered)
	}
	if !exec.CompletedAt.Valid {
		t.Error("Complete() CompletedAt should be valid")
	}
	if exec.DurationMs < 0 {
		t.Error("Complete() DurationMs should be non-negative")
	}
}

func TestGathererExecution_Fail(t *testing.T) {
	exec, _ := NewGathererExecution("gatherer-123")

	exec.Fail("some error occurred")

	if exec.Status != GathererStatusFailed {
		t.Errorf("Fail() Status = %v, want failed", exec.Status)
	}
	if !exec.Error.Valid || exec.Error.String != "some error occurred" {
		t.Errorf("Fail() Error = %v, want 'some error occurred'", exec.Error)
	}
	if !exec.CompletedAt.Valid {
		t.Error("Fail() CompletedAt should be valid")
	}
}

func TestDefaultGathererConfigs(t *testing.T) {
	configs := DefaultGathererConfigs()

	if len(configs) == 0 {
		t.Error("DefaultGathererConfigs() should return non-empty slice")
	}

	// Check that all configs have valid IDs
	for _, config := range configs {
		if config.ID == "" {
			t.Error("DefaultGathererConfigs() all configs should have IDs")
		}
		if !config.Type.IsValid() {
			t.Errorf("DefaultGathererConfigs() config %s has invalid type %s", config.Name, config.Type)
		}
	}

	// Check that project_instructions gatherer exists
	found := false
	for _, config := range configs {
		if config.Type == ContextTypeProjectInstructions {
			found = true
			if config.Priority != PriorityCritical {
				t.Error("Project instructions gatherer should have critical priority")
			}
			break
		}
	}
	if !found {
		t.Error("DefaultGathererConfigs() should include project_instructions gatherer")
	}
}

func TestDefaultContextBudget(t *testing.T) {
	budget := DefaultContextBudget()

	if budget.ID == "" {
		t.Error("DefaultContextBudget() ID should not be empty")
	}
	if budget.TotalTokenBudget != 128000 {
		t.Errorf("DefaultContextBudget() TotalTokenBudget = %v, want 128000", budget.TotalTokenBudget)
	}
	if !budget.IsDefault {
		t.Error("DefaultContextBudget() IsDefault should be true")
	}
}

func TestComputeContentHash(t *testing.T) {
	// Test that same content produces same hash
	hash1 := computeContentHash("test content")
	hash2 := computeContentHash("test content")
	if hash1 != hash2 {
		t.Error("computeContentHash() should produce same hash for same content")
	}

	// Test that different content produces different hash
	hash3 := computeContentHash("different content")
	if hash1 == hash3 {
		t.Error("computeContentHash() should produce different hash for different content")
	}

	// Test empty content
	emptyHash := computeContentHash("")
	if emptyHash != "" {
		t.Error("computeContentHash() should return empty string for empty content")
	}
}
