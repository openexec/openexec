package agent

import (
	"testing"
)

func TestCostCalculator_CalculateCost(t *testing.T) {
	registry := NewProviderRegistry()

	// Create a mock provider with known pricing
	provider := NewMockProviderWithInfo("test", []string{"test-model"})
	provider.modelInfo["test-model"].PricePerMInputTokens = 2.0
	provider.modelInfo["test-model"].PricePerMOutputTokens = 10.0
	registry.Register(provider)

	calc := NewCostCalculator(registry)

	// Test basic cost calculation
	usage := Usage{
		PromptTokens:     1000,
		CompletionTokens: 500,
		TotalTokens:      1500,
	}

	estimate, err := calc.CalculateCost("test-model", usage)
	if err != nil {
		t.Errorf("CalculateCost failed: %v", err)
	}

	// Input cost: 1000 tokens * $2.0/M = $0.002
	expectedInputCost := 0.002
	if estimate.InputCost != expectedInputCost {
		t.Errorf("expected input cost %f, got %f", expectedInputCost, estimate.InputCost)
	}

	// Output cost: 500 tokens * $10.0/M = $0.005
	expectedOutputCost := 0.005
	if estimate.OutputCost != expectedOutputCost {
		t.Errorf("expected output cost %f, got %f", expectedOutputCost, estimate.OutputCost)
	}

	// Total cost: $0.002 + $0.005 = $0.007
	expectedTotalCost := 0.007
	if estimate.TotalCost != expectedTotalCost {
		t.Errorf("expected total cost %f, got %f", expectedTotalCost, estimate.TotalCost)
	}
}

func TestCostCalculator_CalculateCostWithCache(t *testing.T) {
	registry := NewProviderRegistry()

	provider := NewMockProviderWithInfo("test", []string{"test-model"})
	provider.modelInfo["test-model"].PricePerMInputTokens = 10.0
	provider.modelInfo["test-model"].PricePerMOutputTokens = 50.0
	registry.Register(provider)

	calc := NewCostCalculator(registry)

	// Test with cache tokens
	usage := Usage{
		PromptTokens:     1000,
		CompletionTokens: 500,
		TotalTokens:      1500,
		CacheReadTokens:  2000,
		CacheWriteTokens: 1000,
	}

	estimate, err := calc.CalculateCost("test-model", usage)
	if err != nil {
		t.Errorf("CalculateCost failed: %v", err)
	}

	// Cache read at 10% of input price: 2000 * $10.0/M * 0.1 = $0.002
	expectedCacheReadCost := 0.002
	if estimate.CacheReadCost != expectedCacheReadCost {
		t.Errorf("expected cache read cost %f, got %f", expectedCacheReadCost, estimate.CacheReadCost)
	}

	// Cache write at 125% of input price: 1000 * $10.0/M * 1.25 = $0.0125
	expectedCacheWriteCost := 0.0125
	if estimate.CacheWriteCost != expectedCacheWriteCost {
		t.Errorf("expected cache write cost %f, got %f", expectedCacheWriteCost, estimate.CacheWriteCost)
	}

	// Total should include cache costs
	if estimate.TotalCost < estimate.InputCost+estimate.OutputCost+estimate.CacheReadCost+estimate.CacheWriteCost-0.0001 {
		t.Error("total cost should include cache costs")
	}
}

func TestCostCalculator_CalculateCostFromTokens(t *testing.T) {
	registry := NewProviderRegistry()

	provider := NewMockProviderWithInfo("test", []string{"model"})
	provider.modelInfo["model"].PricePerMInputTokens = 1.0
	provider.modelInfo["model"].PricePerMOutputTokens = 2.0
	registry.Register(provider)

	calc := NewCostCalculator(registry)

	estimate, err := calc.CalculateCostFromTokens("model", 1000000, 500000)
	if err != nil {
		t.Errorf("CalculateCostFromTokens failed: %v", err)
	}

	// 1M input at $1/M = $1.00
	if estimate.InputCost != 1.0 {
		t.Errorf("expected input cost $1.00, got $%f", estimate.InputCost)
	}

	// 500K output at $2/M = $1.00
	if estimate.OutputCost != 1.0 {
		t.Errorf("expected output cost $1.00, got $%f", estimate.OutputCost)
	}
}

func TestCostCalculator_EstimateCostFromContent(t *testing.T) {
	registry := NewProviderRegistry()

	provider := NewMockProviderWithInfo("test", []string{"model"})
	provider.modelInfo["model"].PricePerMInputTokens = 4.0
	provider.modelInfo["model"].PricePerMOutputTokens = 8.0
	registry.Register(provider)

	calc := NewCostCalculator(registry)

	// Content of 4000 chars should estimate to ~1000 tokens (4 chars/token)
	content := make([]byte, 4000)
	for i := range content {
		content[i] = 'a'
	}

	estimate, err := calc.EstimateCostFromContent("model", string(content), 500)
	if err != nil {
		t.Errorf("EstimateCostFromContent failed: %v", err)
	}

	// Check that we got a reasonable estimate
	if estimate.InputTokens == 0 {
		t.Error("expected non-zero input tokens")
	}
	if estimate.OutputTokens != 500 {
		t.Errorf("expected 500 output tokens, got %d", estimate.OutputTokens)
	}
}

func TestCostCalculator_CompareCosts(t *testing.T) {
	registry := NewProviderRegistry()

	// Create providers with different pricing
	provider1 := NewMockProviderWithInfo("p1", []string{"cheap-model"})
	provider1.modelInfo["cheap-model"].PricePerMInputTokens = 0.5
	provider1.modelInfo["cheap-model"].PricePerMOutputTokens = 1.0

	provider2 := NewMockProviderWithInfo("p2", []string{"expensive-model"})
	provider2.modelInfo["expensive-model"].PricePerMInputTokens = 10.0
	provider2.modelInfo["expensive-model"].PricePerMOutputTokens = 30.0

	registry.Register(provider1)
	registry.Register(provider2)

	calc := NewCostCalculator(registry)

	usage := Usage{
		PromptTokens:     10000,
		CompletionTokens: 5000,
		TotalTokens:      15000,
	}

	results, err := calc.CompareCosts(usage, "cheap-model", "expensive-model", "nonexistent")
	if err != nil {
		t.Errorf("CompareCosts failed: %v", err)
	}

	// Should have 2 results (nonexistent skipped)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	cheapEstimate := results["cheap-model"]
	expensiveEstimate := results["expensive-model"]

	if cheapEstimate == nil || expensiveEstimate == nil {
		t.Fatal("expected both estimates to be present")
	}

	// Cheap model should cost less
	if cheapEstimate.TotalCost >= expensiveEstimate.TotalCost {
		t.Error("cheap model should cost less than expensive model")
	}
}

func TestCostCalculator_FindCheapestModel(t *testing.T) {
	registry := NewProviderRegistry()

	// Create providers with different pricing and capabilities
	provider1 := NewMockProviderWithInfo("p1", []string{"expensive-vision"})
	provider1.modelInfo["expensive-vision"].PricePerMInputTokens = 10.0
	provider1.modelInfo["expensive-vision"].PricePerMOutputTokens = 30.0
	provider1.modelInfo["expensive-vision"].Capabilities.Vision = true

	provider2 := NewMockProviderWithInfo("p2", []string{"cheap-vision"})
	provider2.modelInfo["cheap-vision"].PricePerMInputTokens = 0.5
	provider2.modelInfo["cheap-vision"].PricePerMOutputTokens = 1.0
	provider2.modelInfo["cheap-vision"].Capabilities.Vision = true

	provider3 := NewMockProviderWithInfo("p3", []string{"cheapest-no-vision"})
	provider3.modelInfo["cheapest-no-vision"].PricePerMInputTokens = 0.1
	provider3.modelInfo["cheapest-no-vision"].PricePerMOutputTokens = 0.2
	provider3.modelInfo["cheapest-no-vision"].Capabilities.Vision = false

	registry.Register(provider1)
	registry.Register(provider2)
	registry.Register(provider3)

	calc := NewCostCalculator(registry)

	usage := Usage{
		PromptTokens:     10000,
		CompletionTokens: 5000,
	}

	// Find cheapest with vision requirement
	model, estimate, err := calc.FindCheapestModel(usage, ProviderCapabilities{Vision: true})
	if err != nil {
		t.Errorf("FindCheapestModel failed: %v", err)
	}

	if model.ID != "cheap-vision" {
		t.Errorf("expected cheap-vision, got %s", model.ID)
	}

	if estimate == nil {
		t.Error("expected estimate to be returned")
	}

	// Find cheapest without vision requirement - should get cheapest-no-vision
	model, _, err = calc.FindCheapestModel(usage, ProviderCapabilities{})
	if err != nil {
		t.Errorf("FindCheapestModel failed: %v", err)
	}

	if model.ID != "cheapest-no-vision" {
		t.Errorf("expected cheapest-no-vision, got %s", model.ID)
	}
}

func TestCostCalculator_FindCheapestModel_NoMatch(t *testing.T) {
	registry := NewProviderRegistry()

	// Create provider without tool use
	provider := NewMockProviderWithInfo("p", []string{"model"})
	provider.modelInfo["model"].Capabilities.ToolUse = false
	registry.Register(provider)

	calc := NewCostCalculator(registry)

	usage := Usage{PromptTokens: 1000, CompletionTokens: 500}

	// Require tool use which no model supports
	_, _, err := calc.FindCheapestModel(usage, ProviderCapabilities{ToolUse: true})
	if err == nil {
		t.Error("expected error when no models match")
	}
}

func TestCostCalculator_ModelNotFound(t *testing.T) {
	registry := NewProviderRegistry()
	calc := NewCostCalculator(registry)

	_, err := calc.CalculateCost("nonexistent", Usage{})
	if err == nil {
		t.Error("expected error for nonexistent model")
	}
}

func TestModelCatalog_GetModel(t *testing.T) {
	catalog := NewModelCatalog()

	// Test getting a known model
	info, ok := catalog.GetModel(ModelGPT4o)
	if !ok {
		t.Error("expected to find gpt-4o")
	}
	if info.Name != "GPT-4o" {
		t.Errorf("expected name 'GPT-4o', got '%s'", info.Name)
	}
	if info.Family != FamilyGPT4o {
		t.Errorf("expected family gpt-4o, got %s", info.Family)
	}

	// Test getting unknown model
	_, ok = catalog.GetModel("nonexistent-model")
	if ok {
		t.Error("expected not to find nonexistent model")
	}
}

func TestModelCatalog_GetModelsByFamily(t *testing.T) {
	catalog := NewModelCatalog()

	// Get GPT-4o family models
	models := catalog.GetModelsByFamily(FamilyGPT4o)
	if len(models) < 2 {
		t.Errorf("expected at least 2 gpt-4o family models, got %d", len(models))
	}

	// Verify all returned models are in the family
	for _, m := range models {
		if m.Family != FamilyGPT4o {
			t.Errorf("model %s should be in gpt-4o family", m.ID)
		}
	}
}

func TestModelCatalog_GetModelsByTier(t *testing.T) {
	catalog := NewModelCatalog()

	// Get economy tier models
	models := catalog.GetModelsByTier(TierEconomy)
	if len(models) == 0 {
		t.Error("expected at least one economy tier model")
	}

	// Verify all returned models are economy tier
	for _, m := range models {
		if m.Tier != TierEconomy {
			t.Errorf("model %s should be economy tier", m.ID)
		}
	}
}

func TestModelCatalog_GetModelsByProvider(t *testing.T) {
	catalog := NewModelCatalog()

	// Get OpenAI models
	openAIModels := catalog.GetModelsByProvider("openai")
	if len(openAIModels) == 0 {
		t.Error("expected OpenAI models")
	}

	// Verify all returned models are from OpenAI
	for _, m := range openAIModels {
		if m.Provider != "openai" {
			t.Errorf("model %s should be from openai", m.ID)
		}
	}

	// Get Anthropic models
	anthropicModels := catalog.GetModelsByProvider("anthropic")
	if len(anthropicModels) == 0 {
		t.Error("expected Anthropic models")
	}

	// Get Gemini models
	geminiModels := catalog.GetModelsByProvider("gemini")
	if len(geminiModels) == 0 {
		t.Error("expected Gemini models")
	}
}

func TestModelCatalog_GetAllModels(t *testing.T) {
	catalog := NewModelCatalog()

	models := catalog.GetAllModels()
	if len(models) == 0 {
		t.Error("expected models in catalog")
	}

	// Verify sorted order (by provider then ID)
	for i := 1; i < len(models); i++ {
		prev := models[i-1]
		curr := models[i]
		if prev.Provider > curr.Provider {
			t.Errorf("models should be sorted by provider: %s > %s", prev.Provider, curr.Provider)
		} else if prev.Provider == curr.Provider && prev.ID > curr.ID {
			t.Errorf("models should be sorted by ID within provider: %s > %s", prev.ID, curr.ID)
		}
	}
}

func TestModelCatalog_GetNonDeprecatedModels(t *testing.T) {
	catalog := NewModelCatalog()

	nonDeprecated := catalog.GetNonDeprecatedModels()
	allModels := catalog.GetAllModels()

	// Non-deprecated should be less than or equal to all
	if len(nonDeprecated) > len(allModels) {
		t.Error("non-deprecated count exceeds total count")
	}

	// Verify none are deprecated
	for _, m := range nonDeprecated {
		if m.Deprecated {
			t.Errorf("model %s should not be deprecated", m.ID)
		}
	}
}

func TestExtendedModelInfo_Fields(t *testing.T) {
	catalog := NewModelCatalog()

	// Test GPT-4o has expected extended fields
	gpt4o, ok := catalog.GetModel(ModelGPT4o)
	if !ok {
		t.Fatal("expected gpt-4o")
	}

	if gpt4o.Description == "" {
		t.Error("expected description")
	}
	if len(gpt4o.UseCases) == 0 {
		t.Error("expected use cases")
	}
	if gpt4o.BatchPricing == nil {
		t.Error("expected batch pricing for gpt-4o")
	}

	// Test Claude model has cache pricing
	claude, ok := catalog.GetModel("claude-opus-4-5-20251101")
	if !ok {
		t.Fatal("expected claude-opus-4-5-20251101")
	}

	if claude.CachePricing == nil {
		t.Error("expected cache pricing for Claude Opus")
	}
	if claude.CachePricing.PricePerMCacheWriteTokens == 0 {
		t.Error("expected non-zero cache write price")
	}

	// Test Gemini model has tiered pricing
	gemini, ok := catalog.GetModel(ModelGemini15Pro)
	if !ok {
		t.Fatal("expected gemini-1.5-pro")
	}

	if len(gemini.TieredPricing) == 0 {
		t.Error("expected tiered pricing for Gemini 1.5 Pro")
	}
	if gemini.ContextLengthCategories == nil {
		t.Error("expected context length categories")
	}
}

func TestFormatPrice(t *testing.T) {
	tests := []struct {
		price    float64
		expected string
	}{
		{0, "Free"},
		{0.001, "$0.0010/M"},
		{0.075, "$0.075/M"},
		{0.5, "$0.500/M"},
		{1.0, "$1.00/M"},
		{15.0, "$15.00/M"},
		{75.0, "$75.00/M"},
	}

	for _, tt := range tests {
		result := FormatPrice(tt.price)
		if result != tt.expected {
			t.Errorf("FormatPrice(%f) = %s, expected %s", tt.price, result, tt.expected)
		}
	}
}

func TestFormatCost(t *testing.T) {
	tests := []struct {
		cost     float64
		expected string
	}{
		{0, "$0.00"},
		{0.000001, "$0.000001"}, // < 0.01, uses 6 decimal places
		{0.001, "$0.001000"},    // < 0.01, uses 6 decimal places
		{0.0123, "$0.0123"},     // >= 0.01 and < 1, uses 4 decimal places
		{0.5, "$0.5000"},        // >= 0.01 and < 1, uses 4 decimal places
		{1.0, "$1.00"},          // >= 1, uses 2 decimal places
		{10.5, "$10.50"},
		{100.0, "$100.00"},
	}

	for _, tt := range tests {
		result := FormatCost(tt.cost)
		if result != tt.expected {
			t.Errorf("FormatCost(%f) = %s, expected %s", tt.cost, result, tt.expected)
		}
	}
}

func TestModelFamilyConstants(t *testing.T) {
	// Verify family constants are defined
	families := []ModelFamily{
		FamilyGPT4o,
		FamilyGPT4,
		FamilyGPT35,
		FamilyO1,
		FamilyO3,
		FamilyClaudeOpus,
		FamilyClaudeSonnet,
		FamilyClaudeHaiku,
		FamilyGemini20,
		FamilyGemini15,
		FamilyGemini1,
	}

	for _, f := range families {
		if f == "" {
			t.Error("family constant should not be empty")
		}
	}
}

func TestModelTierConstants(t *testing.T) {
	tiers := []ModelTier{
		TierFlagship,
		TierStandard,
		TierEconomy,
		TierReasoning,
	}

	for _, tier := range tiers {
		if tier == "" {
			t.Error("tier constant should not be empty")
		}
	}
}

func TestPricingTier_Structure(t *testing.T) {
	tier := PricingTier{
		UpToTokens:            128000,
		PricePerMInputTokens:  1.25,
		PricePerMOutputTokens: 5.00,
	}

	if tier.UpToTokens != 128000 {
		t.Error("UpToTokens mismatch")
	}
	if tier.PricePerMInputTokens != 1.25 {
		t.Error("PricePerMInputTokens mismatch")
	}
	if tier.PricePerMOutputTokens != 5.00 {
		t.Error("PricePerMOutputTokens mismatch")
	}
}

func TestCachePricing_Structure(t *testing.T) {
	cache := CachePricing{
		PricePerMCacheWriteTokens: 3.75,
		PricePerMCacheReadTokens:  0.30,
		CacheTTLMinutes:           5,
		MinCacheableTokens:        1024,
	}

	if cache.PricePerMCacheWriteTokens != 3.75 {
		t.Error("PricePerMCacheWriteTokens mismatch")
	}
	if cache.PricePerMCacheReadTokens != 0.30 {
		t.Error("PricePerMCacheReadTokens mismatch")
	}
	if cache.CacheTTLMinutes != 5 {
		t.Error("CacheTTLMinutes mismatch")
	}
	if cache.MinCacheableTokens != 1024 {
		t.Error("MinCacheableTokens mismatch")
	}
}

func TestBatchPricing_Structure(t *testing.T) {
	batch := BatchPricing{
		PricePerMInputTokens:  1.25,
		PricePerMOutputTokens: 5.00,
		MaxTurnaroundHours:    24,
	}

	if batch.PricePerMInputTokens != 1.25 {
		t.Error("PricePerMInputTokens mismatch")
	}
	if batch.PricePerMOutputTokens != 5.00 {
		t.Error("PricePerMOutputTokens mismatch")
	}
	if batch.MaxTurnaroundHours != 24 {
		t.Error("MaxTurnaroundHours mismatch")
	}
}

func TestUsageEstimate_Structure(t *testing.T) {
	estimate := UsageEstimate{
		InputTokens:   1000,
		OutputTokens:  500,
		InputCost:     0.002,
		OutputCost:    0.005,
		TotalCost:     0.007,
		CacheReadCost: 0.001,
	}

	if estimate.InputTokens != 1000 {
		t.Error("InputTokens mismatch")
	}
	if estimate.OutputTokens != 500 {
		t.Error("OutputTokens mismatch")
	}
	if estimate.TotalCost != 0.007 {
		t.Error("TotalCost mismatch")
	}
}

func TestDefaultModelCatalog(t *testing.T) {
	if DefaultModelCatalog == nil {
		t.Error("DefaultModelCatalog should be initialized")
	}

	// Should have models
	models := DefaultModelCatalog.GetAllModels()
	if len(models) == 0 {
		t.Error("DefaultModelCatalog should contain models")
	}
}

func TestDefaultCostCalculator(t *testing.T) {
	if DefaultCostCalculator == nil {
		t.Error("DefaultCostCalculator should be initialized")
	}

	// Should use DefaultRegistry
	if DefaultCostCalculator.registry != DefaultRegistry {
		t.Error("DefaultCostCalculator should use DefaultRegistry")
	}
}

func TestModelCatalog_AllProvidersCovered(t *testing.T) {
	catalog := NewModelCatalog()

	providers := map[string]bool{
		"openai":    false,
		"anthropic": false,
		"gemini":    false,
	}

	for _, model := range catalog.GetAllModels() {
		if _, ok := providers[model.Provider]; ok {
			providers[model.Provider] = true
		}
	}

	for provider, found := range providers {
		if !found {
			t.Errorf("expected models from provider %s", provider)
		}
	}
}

func TestModelCatalog_AllTiersCovered(t *testing.T) {
	catalog := NewModelCatalog()

	tiers := map[ModelTier]bool{
		TierFlagship:  false,
		TierStandard:  false,
		TierEconomy:   false,
		TierReasoning: false,
	}

	for _, model := range catalog.GetAllModels() {
		if model.Tier != "" {
			tiers[model.Tier] = true
		}
	}

	for tier, found := range tiers {
		if !found {
			t.Errorf("expected models from tier %s", tier)
		}
	}
}

func TestCostCalculator_ZeroTokenUsage(t *testing.T) {
	registry := NewProviderRegistry()

	provider := NewMockProviderWithInfo("test", []string{"model"})
	registry.Register(provider)

	calc := NewCostCalculator(registry)

	estimate, err := calc.CalculateCost("model", Usage{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if estimate.TotalCost != 0 {
		t.Errorf("expected zero cost for zero usage, got %f", estimate.TotalCost)
	}
}

func TestCostCalculator_LargeTokenUsage(t *testing.T) {
	registry := NewProviderRegistry()

	provider := NewMockProviderWithInfo("test", []string{"model"})
	provider.modelInfo["model"].PricePerMInputTokens = 10.0
	provider.modelInfo["model"].PricePerMOutputTokens = 30.0
	registry.Register(provider)

	calc := NewCostCalculator(registry)

	// 10 million tokens
	usage := Usage{
		PromptTokens:     10_000_000,
		CompletionTokens: 10_000_000,
		TotalTokens:      20_000_000,
	}

	estimate, err := calc.CalculateCost("model", usage)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Input: 10M tokens * $10/M = $100
	if estimate.InputCost != 100.0 {
		t.Errorf("expected input cost $100, got $%f", estimate.InputCost)
	}

	// Output: 10M tokens * $30/M = $300
	if estimate.OutputCost != 300.0 {
		t.Errorf("expected output cost $300, got $%f", estimate.OutputCost)
	}

	// Total: $400
	if estimate.TotalCost != 400.0 {
		t.Errorf("expected total cost $400, got $%f", estimate.TotalCost)
	}
}
