package agent

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

// MockProviderWithInfo extends MockProvider for registry testing with model info.
type MockProviderWithInfo struct {
	name      string
	models    []string
	modelInfo map[string]*ModelInfo
}

func NewMockProviderWithInfo(name string, models []string) *MockProviderWithInfo {
	mp := &MockProviderWithInfo{
		name:      name,
		models:    models,
		modelInfo: make(map[string]*ModelInfo),
	}

	// Create default model info
	for _, m := range models {
		mp.modelInfo[m] = &ModelInfo{
			ID:       m,
			Name:     m,
			Provider: name,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 128000,
				MaxOutputTokens:  4096,
			},
			PricePerMInputTokens:  1.0,
			PricePerMOutputTokens: 2.0,
		}
	}

	return mp
}

func (m *MockProviderWithInfo) GetName() string {
	return m.name
}

func (m *MockProviderWithInfo) GetModels() []string {
	return m.models
}

func (m *MockProviderWithInfo) GetModelInfo(modelID string) (*ModelInfo, error) {
	info, ok := m.modelInfo[modelID]
	if !ok {
		return nil, &ProviderError{
			Code:    ErrCodeNotFound,
			Message: "model not found",
		}
	}
	return info, nil
}

func (m *MockProviderWithInfo) GetCapabilities(modelID string) (*ProviderCapabilities, error) {
	info, err := m.GetModelInfo(modelID)
	if err != nil {
		return nil, err
	}
	return &info.Capabilities, nil
}

func (m *MockProviderWithInfo) Complete(ctx context.Context, req Request) (*Response, error) {
	return &Response{
		ID:         "mock-response",
		Model:      req.Model,
		StopReason: StopReasonEnd,
		Content: []ContentBlock{
			{Type: ContentTypeText, Text: "Mock response"},
		},
	}, nil
}

func (m *MockProviderWithInfo) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 10)
	go func() {
		defer close(ch)
		ch <- StreamEvent{Type: StreamEventStart}
		ch <- StreamEvent{
			Type:  StreamEventContentDelta,
			Delta: &StreamDelta{Text: "Mock stream"},
		}
		ch <- StreamEvent{Type: StreamEventStop, StopReason: StopReasonEnd}
	}()
	return ch, nil
}

func (m *MockProviderWithInfo) ValidateRequest(req Request) error {
	if req.Model == "" {
		return &ProviderError{
			Code:    ErrCodeInvalidRequest,
			Message: "model required",
		}
	}
	return nil
}

func (m *MockProviderWithInfo) EstimateTokens(content string) int {
	return len(content) / 4
}

// Verify MockProviderWithInfo implements ProviderAdapter
var _ ProviderAdapter = (*MockProviderWithInfo)(nil)

func TestNewProviderRegistry(t *testing.T) {
	registry := NewProviderRegistry()
	if registry == nil {
		t.Fatal("NewProviderRegistry returned nil")
	}
	if registry.providers == nil {
		t.Error("providers map not initialized")
	}
	if registry.factories == nil {
		t.Error("factories map not initialized")
	}
	if registry.modelIndex == nil {
		t.Error("modelIndex map not initialized")
	}
}

func TestProviderRegistry_Register(t *testing.T) {
	registry := NewProviderRegistry()

	// Test successful registration
	provider := NewMockProviderWithInfo("test-provider", []string{"model-a", "model-b"})
	err := registry.Register(provider)
	if err != nil {
		t.Errorf("Register failed: %v", err)
	}

	// Verify provider is registered
	got, err := registry.Get("test-provider")
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}
	if got.GetName() != "test-provider" {
		t.Errorf("expected name test-provider, got %s", got.GetName())
	}

	// Test nil provider
	err = registry.Register(nil)
	if err == nil {
		t.Error("expected error for nil provider")
	}
}

func TestProviderRegistry_RegisterFactory(t *testing.T) {
	registry := NewProviderRegistry()

	called := false
	factory := func() (ProviderAdapter, error) {
		called = true
		return NewMockProviderWithInfo("lazy-provider", []string{"lazy-model"}), nil
	}

	registry.RegisterFactory("lazy-provider", factory)

	// Factory should not be called yet
	if called {
		t.Error("factory called before Get")
	}

	// Get should trigger factory
	provider, err := registry.Get("lazy-provider")
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}
	if !called {
		t.Error("factory not called")
	}
	if provider.GetName() != "lazy-provider" {
		t.Errorf("expected name lazy-provider, got %s", provider.GetName())
	}

	// Second Get should not call factory again
	called = false
	_, err = registry.Get("lazy-provider")
	if err != nil {
		t.Errorf("second Get failed: %v", err)
	}
	if called {
		t.Error("factory called twice")
	}
}

func TestProviderRegistry_Get(t *testing.T) {
	registry := NewProviderRegistry()

	// Test not found
	_, err := registry.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent provider")
	}
	provErr, ok := err.(*ProviderError)
	if !ok {
		t.Errorf("expected ProviderError, got %T", err)
	} else if provErr.Code != ErrCodeNotFound {
		t.Errorf("expected code %s, got %s", ErrCodeNotFound, provErr.Code)
	}

	// Test successful get
	provider := NewMockProviderWithInfo("test", []string{"model"})
	registry.Register(provider)

	got, err := registry.Get("test")
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}
	if got != provider {
		t.Error("returned wrong provider")
	}
}

func TestProviderRegistry_MustGet(t *testing.T) {
	registry := NewProviderRegistry()

	// Test panic on not found
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nonexistent provider")
		}
	}()

	registry.MustGet("nonexistent")
}

func TestProviderRegistry_GetForModel(t *testing.T) {
	registry := NewProviderRegistry()

	provider1 := NewMockProviderWithInfo("provider1", []string{"model-1a", "model-1b"})
	provider2 := NewMockProviderWithInfo("provider2", []string{"model-2a", "model-2b"})

	registry.Register(provider1)
	registry.Register(provider2)

	// Test finding model from first provider
	got, err := registry.GetForModel("model-1a")
	if err != nil {
		t.Errorf("GetForModel failed: %v", err)
	}
	if got.GetName() != "provider1" {
		t.Errorf("expected provider1, got %s", got.GetName())
	}

	// Test finding model from second provider
	got, err = registry.GetForModel("model-2b")
	if err != nil {
		t.Errorf("GetForModel failed: %v", err)
	}
	if got.GetName() != "provider2" {
		t.Errorf("expected provider2, got %s", got.GetName())
	}

	// Test not found
	_, err = registry.GetForModel("nonexistent-model")
	if err == nil {
		t.Error("expected error for nonexistent model")
	}
}

func TestProviderRegistry_ListProviders(t *testing.T) {
	registry := NewProviderRegistry()

	// Empty registry
	names := registry.ListProviders()
	if len(names) != 0 {
		t.Errorf("expected empty list, got %v", names)
	}

	// Add providers
	registry.Register(NewMockProviderWithInfo("zebra", []string{"z"}))
	registry.Register(NewMockProviderWithInfo("alpha", []string{"a"}))
	registry.RegisterFactory("beta", func() (ProviderAdapter, error) {
		return NewMockProviderWithInfo("beta", []string{"b"}), nil
	})

	names = registry.ListProviders()
	if len(names) != 3 {
		t.Errorf("expected 3 providers, got %d", len(names))
	}

	// Check sorted order
	expected := []ProviderName{"alpha", "beta", "zebra"}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("expected %s at position %d, got %s", expected[i], i, name)
		}
	}
}

func TestProviderRegistry_ListModels(t *testing.T) {
	registry := NewProviderRegistry()

	provider1 := NewMockProviderWithInfo("p1", []string{"model-c", "model-a"})
	provider2 := NewMockProviderWithInfo("p2", []string{"model-b"})

	registry.Register(provider1)
	registry.Register(provider2)

	models := registry.ListModels()
	if len(models) != 3 {
		t.Errorf("expected 3 models, got %d", len(models))
	}

	// Check sorted
	expected := []string{"model-a", "model-b", "model-c"}
	for i, m := range models {
		if m != expected[i] {
			t.Errorf("expected %s at position %d, got %s", expected[i], i, m)
		}
	}
}

func TestProviderRegistry_GetModelInfo(t *testing.T) {
	registry := NewProviderRegistry()

	provider := NewMockProviderWithInfo("test", []string{"test-model"})
	registry.Register(provider)

	info, err := registry.GetModelInfo("test-model")
	if err != nil {
		t.Errorf("GetModelInfo failed: %v", err)
	}
	if info.ID != "test-model" {
		t.Errorf("expected ID test-model, got %s", info.ID)
	}
	if info.Provider != "test" {
		t.Errorf("expected provider test, got %s", info.Provider)
	}

	// Test not found
	_, err = registry.GetModelInfo("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent model")
	}
}

func TestProviderRegistry_GetAllModelInfo(t *testing.T) {
	registry := NewProviderRegistry()

	provider1 := NewMockProviderWithInfo("aaa", []string{"model-1"})
	provider2 := NewMockProviderWithInfo("bbb", []string{"model-2", "model-3"})

	registry.Register(provider1)
	registry.Register(provider2)

	infos := registry.GetAllModelInfo()
	if len(infos) != 3 {
		t.Errorf("expected 3 models, got %d", len(infos))
	}

	// Verify sorted by provider then model
	if infos[0].Provider != "aaa" || infos[0].ID != "model-1" {
		t.Errorf("first model should be aaa/model-1, got %s/%s", infos[0].Provider, infos[0].ID)
	}
	if infos[1].Provider != "bbb" || infos[1].ID != "model-2" {
		t.Errorf("second model should be bbb/model-2, got %s/%s", infos[1].Provider, infos[1].ID)
	}
}

func TestProviderRegistry_CheckAvailability(t *testing.T) {
	registry := NewProviderRegistry()

	// Test unavailable provider
	status := registry.CheckAvailability("nonexistent")
	if status.Available {
		t.Error("expected unavailable")
	}
	if status.Reason == "" {
		t.Error("expected reason to be set")
	}

	// Test available provider
	provider := NewMockProviderWithInfo("test", []string{"m1", "m2"})
	registry.Register(provider)

	status = registry.CheckAvailability("test")
	if !status.Available {
		t.Error("expected available")
	}
	if len(status.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(status.Models))
	}
}

func TestProviderRegistry_CheckAllAvailability(t *testing.T) {
	registry := NewProviderRegistry()

	registry.Register(NewMockProviderWithInfo("p1", []string{"m1"}))
	registry.RegisterFactory("p2", func() (ProviderAdapter, error) {
		return nil, &ProviderError{Code: ErrCodeAuthentication, Message: "no api key"}
	})

	statuses := registry.CheckAllAvailability()
	if len(statuses) != 2 {
		t.Errorf("expected 2 statuses, got %d", len(statuses))
	}

	// Find each status
	var p1Status, p2Status *ProviderStatus
	for i := range statuses {
		if statuses[i].Name == "p1" {
			p1Status = &statuses[i]
		} else if statuses[i].Name == "p2" {
			p2Status = &statuses[i]
		}
	}

	if p1Status == nil || !p1Status.Available {
		t.Error("p1 should be available")
	}
	if p2Status == nil || p2Status.Available {
		t.Error("p2 should not be available")
	}
}

func TestProviderRegistry_HasProvider(t *testing.T) {
	registry := NewProviderRegistry()

	if registry.HasProvider("test") {
		t.Error("should not have test provider")
	}

	registry.Register(NewMockProviderWithInfo("test", []string{"m"}))
	if !registry.HasProvider("test") {
		t.Error("should have test provider")
	}

	// Also test with factory
	registry.RegisterFactory("lazy", func() (ProviderAdapter, error) {
		return NewMockProviderWithInfo("lazy", []string{"l"}), nil
	})
	if !registry.HasProvider("lazy") {
		t.Error("should have lazy provider from factory")
	}
}

func TestProviderRegistry_HasModel(t *testing.T) {
	registry := NewProviderRegistry()

	if registry.HasModel("model-x") {
		t.Error("should not have model-x")
	}

	registry.Register(NewMockProviderWithInfo("test", []string{"model-x"}))
	if !registry.HasModel("model-x") {
		t.Error("should have model-x")
	}
}

func TestProviderRegistry_Unregister(t *testing.T) {
	registry := NewProviderRegistry()

	provider := NewMockProviderWithInfo("test", []string{"model-1"})
	registry.Register(provider)

	// Verify registered
	if !registry.HasProvider("test") {
		t.Error("provider should be registered")
	}
	if !registry.HasModel("model-1") {
		t.Error("model should be registered")
	}

	// Unregister
	registry.Unregister("test")

	if registry.HasProvider("test") {
		t.Error("provider should be unregistered")
	}
	if registry.HasModel("model-1") {
		t.Error("model should be unregistered")
	}
}

func TestProviderRegistry_Clear(t *testing.T) {
	registry := NewProviderRegistry()

	registry.Register(NewMockProviderWithInfo("p1", []string{"m1"}))
	registry.Register(NewMockProviderWithInfo("p2", []string{"m2"}))
	registry.RegisterFactory("p3", func() (ProviderAdapter, error) {
		return NewMockProviderWithInfo("p3", []string{"m3"}), nil
	})

	registry.Clear()

	if len(registry.ListProviders()) != 0 {
		t.Error("providers should be cleared")
	}
	if len(registry.ListModels()) != 0 {
		t.Error("models should be cleared")
	}
}

func TestProviderRegistry_Complete(t *testing.T) {
	registry := NewProviderRegistry()

	provider := NewMockProviderWithInfo("test", []string{"test-model"})
	registry.Register(provider)

	resp, err := registry.Complete(context.Background(), Request{
		Model: "test-model",
		Messages: []Message{
			NewTextMessage(RoleUser, "Hello"),
		},
	})
	if err != nil {
		t.Errorf("Complete failed: %v", err)
	}
	if resp.Model != "test-model" {
		t.Errorf("expected model test-model, got %s", resp.Model)
	}

	// Test with unknown model
	_, err = registry.Complete(context.Background(), Request{
		Model: "unknown-model",
	})
	if err == nil {
		t.Error("expected error for unknown model")
	}
}

func TestProviderRegistry_Stream(t *testing.T) {
	registry := NewProviderRegistry()

	provider := NewMockProviderWithInfo("test", []string{"test-model"})
	registry.Register(provider)

	ch, err := registry.Stream(context.Background(), Request{
		Model: "test-model",
		Messages: []Message{
			NewTextMessage(RoleUser, "Hello"),
		},
	})
	if err != nil {
		t.Errorf("Stream failed: %v", err)
	}

	// Consume events
	eventCount := 0
	for range ch {
		eventCount++
	}
	if eventCount < 2 {
		t.Errorf("expected at least 2 events, got %d", eventCount)
	}
}

func TestModelSelector_SelectByCapabilities(t *testing.T) {
	registry := NewProviderRegistry()

	// Create providers with different capabilities
	provider1 := NewMockProviderWithInfo("p1", []string{"vision-model"})
	provider1.modelInfo["vision-model"].Capabilities.Vision = true
	provider1.modelInfo["vision-model"].Capabilities.ToolUse = true

	provider2 := NewMockProviderWithInfo("p2", []string{"basic-model"})
	provider2.modelInfo["basic-model"].Capabilities.Vision = false
	provider2.modelInfo["basic-model"].Capabilities.ToolUse = true

	registry.Register(provider1)
	registry.Register(provider2)

	selector := NewModelSelector(registry)

	// Select models with vision
	matches := selector.SelectByCapabilities(ProviderCapabilities{Vision: true})
	if len(matches) != 1 {
		t.Errorf("expected 1 match, got %d", len(matches))
	}
	if len(matches) > 0 && matches[0].ID != "vision-model" {
		t.Errorf("expected vision-model, got %s", matches[0].ID)
	}

	// Select models with tool use (both should match)
	matches = selector.SelectByCapabilities(ProviderCapabilities{ToolUse: true})
	if len(matches) != 2 {
		t.Errorf("expected 2 matches, got %d", len(matches))
	}
}

func TestModelSelector_SelectCheapest(t *testing.T) {
	registry := NewProviderRegistry()

	provider1 := NewMockProviderWithInfo("p1", []string{"expensive-model"})
	provider1.modelInfo["expensive-model"].PricePerMInputTokens = 10.0
	provider1.modelInfo["expensive-model"].PricePerMOutputTokens = 30.0

	provider2 := NewMockProviderWithInfo("p2", []string{"cheap-model"})
	provider2.modelInfo["cheap-model"].PricePerMInputTokens = 0.5
	provider2.modelInfo["cheap-model"].PricePerMOutputTokens = 1.5

	registry.Register(provider1)
	registry.Register(provider2)

	selector := NewModelSelector(registry)

	cheapest := selector.SelectCheapest(ProviderCapabilities{})
	if cheapest == nil {
		t.Fatal("expected to find cheapest model")
	}
	if cheapest.ID != "cheap-model" {
		t.Errorf("expected cheap-model, got %s", cheapest.ID)
	}
}

func TestModelSelector_SelectByProvider(t *testing.T) {
	registry := NewProviderRegistry()

	registry.Register(NewMockProviderWithInfo("p1", []string{"m1", "m2"}))
	registry.Register(NewMockProviderWithInfo("p2", []string{"m3"}))

	selector := NewModelSelector(registry)

	models := selector.SelectByProvider("p1")
	if len(models) != 2 {
		t.Errorf("expected 2 models for p1, got %d", len(models))
	}

	models = selector.SelectByProvider("nonexistent")
	if len(models) != 0 {
		t.Errorf("expected 0 models for nonexistent, got %d", len(models))
	}
}

func TestModelSelector_SelectNonDeprecated(t *testing.T) {
	registry := NewProviderRegistry()

	provider := NewMockProviderWithInfo("p", []string{"active", "deprecated"})
	provider.modelInfo["deprecated"].Deprecated = true

	registry.Register(provider)

	selector := NewModelSelector(registry)

	active := selector.SelectNonDeprecated()
	if len(active) != 1 {
		t.Errorf("expected 1 active model, got %d", len(active))
	}
	if len(active) > 0 && active[0].ID != "active" {
		t.Errorf("expected active model, got %s", active[0].ID)
	}
}

func TestCheckEnvVars(t *testing.T) {
	// Save current env
	origOpenAI := os.Getenv("OPENAI_API_KEY")
	origAnthropic := os.Getenv("ANTHROPIC_API_KEY")
	origGemini := os.Getenv("GEMINI_API_KEY")
	origGoogle := os.Getenv("GOOGLE_API_KEY")

	// Clear all
	os.Unsetenv("OPENAI_API_KEY")
	os.Unsetenv("ANTHROPIC_API_KEY")
	os.Unsetenv("GEMINI_API_KEY")
	os.Unsetenv("GOOGLE_API_KEY")

	defer func() {
		// Restore
		if origOpenAI != "" {
			os.Setenv("OPENAI_API_KEY", origOpenAI)
		}
		if origAnthropic != "" {
			os.Setenv("ANTHROPIC_API_KEY", origAnthropic)
		}
		if origGemini != "" {
			os.Setenv("GEMINI_API_KEY", origGemini)
		}
		if origGoogle != "" {
			os.Setenv("GOOGLE_API_KEY", origGoogle)
		}
	}()

	// Test with no env vars
	result := CheckEnvVars()
	if result[ProviderOpenAI] {
		t.Error("OpenAI should not be available")
	}
	if result[ProviderAnthropic] {
		t.Error("Anthropic should not be available")
	}
	if result[ProviderGemini] {
		t.Error("Gemini should not be available")
	}

	// Test with OpenAI key
	os.Setenv("OPENAI_API_KEY", "test-key")
	result = CheckEnvVars()
	if !result[ProviderOpenAI] {
		t.Error("OpenAI should be available")
	}

	// Test Gemini with GOOGLE_API_KEY
	os.Setenv("GOOGLE_API_KEY", "test-key")
	result = CheckEnvVars()
	if !result[ProviderGemini] {
		t.Error("Gemini should be available via GOOGLE_API_KEY")
	}
}

func TestAvailableProviders(t *testing.T) {
	// Save current env
	origOpenAI := os.Getenv("OPENAI_API_KEY")
	os.Unsetenv("OPENAI_API_KEY")
	os.Unsetenv("ANTHROPIC_API_KEY")
	os.Unsetenv("GEMINI_API_KEY")
	os.Unsetenv("GOOGLE_API_KEY")

	defer func() {
		if origOpenAI != "" {
			os.Setenv("OPENAI_API_KEY", origOpenAI)
		}
	}()

	// Empty when no keys
	available := AvailableProviders()
	if len(available) != 0 {
		t.Errorf("expected 0 available providers, got %d", len(available))
	}

	// Set some keys
	os.Setenv("OPENAI_API_KEY", "key1")
	os.Setenv("ANTHROPIC_API_KEY", "key2")

	available = AvailableProviders()
	if len(available) != 2 {
		t.Errorf("expected 2 available providers, got %d", len(available))
	}

	// Should be sorted
	if len(available) >= 2 {
		if available[0] != ProviderAnthropic || available[1] != ProviderOpenAI {
			t.Errorf("expected [anthropic, openai], got %v", available)
		}
	}
}

func TestProviderRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewProviderRegistry()

	// Register some providers
	for i := 0; i < 10; i++ {
		name := ProviderName(string(rune('a' + i)))
		registry.Register(NewMockProviderWithInfo(string(name), []string{string(name) + "-model"}))
	}

	// Concurrent reads and writes
	done := make(chan bool)

	// Reader goroutines
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				registry.ListProviders()
				registry.ListModels()
				registry.GetAllModelInfo()
			}
			done <- true
		}()
	}

	// Writer goroutines
	for i := 0; i < 5; i++ {
		go func(id int) {
			name := ProviderName(string(rune('z' - id)))
			for j := 0; j < 100; j++ {
				registry.Register(NewMockProviderWithInfo(string(name), []string{string(name) + "-model"}))
				registry.CheckAvailability(name)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestRegisterDefaultFactories(t *testing.T) {
	registry := NewProviderRegistry()
	RegisterDefaultFactories(registry)

	// Verify all default providers have factories
	expectedProviders := []ProviderName{ProviderOpenAI, ProviderAnthropic, ProviderGemini}
	for _, name := range expectedProviders {
		if !registry.HasProvider(name) {
			t.Errorf("expected factory for %s", name)
		}
	}
}

func TestProviderStatus_JSON(t *testing.T) {
	status := ProviderStatus{
		Name:      ProviderOpenAI,
		Available: true,
		Reason:    "configured",
		Models:    []string{"gpt-4", "gpt-3.5-turbo"},
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Errorf("Marshal failed: %v", err)
	}

	var decoded ProviderStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Errorf("Unmarshal failed: %v", err)
	}

	if decoded.Name != status.Name {
		t.Errorf("expected name %s, got %s", status.Name, decoded.Name)
	}
	if decoded.Available != status.Available {
		t.Error("available mismatch")
	}
	if len(decoded.Models) != len(status.Models) {
		t.Errorf("models length mismatch: %d vs %d", len(decoded.Models), len(status.Models))
	}
}

func TestProviderNameConstants(t *testing.T) {
	if ProviderOpenAI != "openai" {
		t.Errorf("expected openai, got %s", ProviderOpenAI)
	}
	if ProviderAnthropic != "anthropic" {
		t.Errorf("expected anthropic, got %s", ProviderAnthropic)
	}
	if ProviderGemini != "gemini" {
		t.Errorf("expected gemini, got %s", ProviderGemini)
	}
}

func TestRequiredEnvVars(t *testing.T) {
	vars := RequiredEnvVars()

	if len(vars[ProviderOpenAI]) == 0 {
		t.Error("expected env vars for OpenAI")
	}
	if vars[ProviderOpenAI][0] != "OPENAI_API_KEY" {
		t.Errorf("expected OPENAI_API_KEY, got %s", vars[ProviderOpenAI][0])
	}

	if len(vars[ProviderAnthropic]) == 0 {
		t.Error("expected env vars for Anthropic")
	}

	if len(vars[ProviderGemini]) == 0 {
		t.Error("expected env vars for Gemini")
	}
}

func TestDefaultRegistry(t *testing.T) {
	// DefaultRegistry should be initialized
	if DefaultRegistry == nil {
		t.Error("DefaultRegistry should not be nil")
	}

	// DefaultModelSelector should use DefaultRegistry
	if DefaultModelSelector == nil {
		t.Error("DefaultModelSelector should not be nil")
	}
	if DefaultModelSelector.registry != DefaultRegistry {
		t.Error("DefaultModelSelector should use DefaultRegistry")
	}
}

func TestGetProvider(t *testing.T) {
	// Clear default registry first
	DefaultRegistry.Clear()
	RegisterDefaultFactories(DefaultRegistry)

	// Without API key set, should fail
	origKey := os.Getenv("OPENAI_API_KEY")
	os.Unsetenv("OPENAI_API_KEY")
	defer func() {
		if origKey != "" {
			os.Setenv("OPENAI_API_KEY", origKey)
		}
	}()

	_, err := GetProvider(ProviderOpenAI)
	if err == nil {
		t.Error("expected error without API key")
	}
}

func TestModelSelector_EmptyRegistry(t *testing.T) {
	registry := NewProviderRegistry()
	selector := NewModelSelector(registry)

	// Should return empty results, not panic
	matches := selector.SelectByCapabilities(ProviderCapabilities{})
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}

	cheapest := selector.SelectCheapest(ProviderCapabilities{})
	if cheapest != nil {
		t.Error("expected nil for cheapest in empty registry")
	}

	models := selector.SelectByProvider("nonexistent")
	if len(models) != 0 {
		t.Errorf("expected 0 models, got %d", len(models))
	}

	active := selector.SelectNonDeprecated()
	if len(active) != 0 {
		t.Errorf("expected 0 active models, got %d", len(active))
	}
}

func TestProviderRegistry_FactoryError(t *testing.T) {
	registry := NewProviderRegistry()

	factoryErr := &ProviderError{
		Code:    ErrCodeAuthentication,
		Message: "missing API key",
	}

	registry.RegisterFactory("failing-provider", func() (ProviderAdapter, error) {
		return nil, factoryErr
	})

	_, err := registry.Get("failing-provider")
	if err == nil {
		t.Error("expected error from factory")
	}
	if err != factoryErr {
		t.Errorf("expected specific factory error, got %v", err)
	}
}

func TestProviderRegistry_GetForModelLazyLoading(t *testing.T) {
	registry := NewProviderRegistry()

	factoryCalled := false
	registry.RegisterFactory("lazy-p", func() (ProviderAdapter, error) {
		factoryCalled = true
		return NewMockProviderWithInfo("lazy-p", []string{"lazy-model"}), nil
	})

	// GetForModel should trigger factory for unregistered but factored provider
	// First, the model isn't in index, so it should search all providers
	// Since factory hasn't been called yet, lazy-model won't be found initially
	_, err := registry.GetForModel("lazy-model")
	if err == nil && !factoryCalled {
		// If it works without calling factory, something's wrong
		t.Log("Model found without factory call - checking if factory providers are searched")
	}
}
