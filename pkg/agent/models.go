// Package agent provides comprehensive model information and pricing data
// for all supported AI providers. This file centralizes model metadata,
// capabilities, and cost calculation utilities.
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ... (rest of imports and types unchanged)

// LoadFromConfig loads model definitions and enabled status from a JSON file.
func (c *ModelCatalog) LoadFromConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var config struct {
		Models []*ExtendedModelInfo `json:"models"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	for _, model := range config.Models {
		// If it's an update to an existing model, merge it
		if existing, ok := c.models[model.ID]; ok {
			// Update mutable fields
			existing.Enabled = model.Enabled
			if model.Name != "" {
				existing.Name = model.Name
			}
			if model.PricePerMInputTokens > 0 {
				existing.PricePerMInputTokens = model.PricePerMInputTokens
			}
			if model.PricePerMOutputTokens > 0 {
				existing.PricePerMOutputTokens = model.PricePerMOutputTokens
			}
			// Update capabilities if provided
			if model.Capabilities.MaxContextTokens > 0 {
				existing.Capabilities = model.Capabilities
			}
		} else {
			// New model
			c.models[model.ID] = model
		}
	}

	return nil
}

// SaveToConfig saves the current catalog to a JSON file.
func (c *ModelCatalog) SaveToConfig(path string) error {
	config := struct {
		Models []*ExtendedModelInfo `json:"models"`
	}{
		Models: c.GetAllModels(),
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// GetEnabledModels returns all enabled models.
func (c *ModelCatalog) GetEnabledModels() []*ExtendedModelInfo {
	var results []*ExtendedModelInfo
	for _, model := range c.models {
		if model.Enabled {
			results = append(results, model)
		}
	}

	// Sort by provider then ID
	sort.Slice(results, func(i, j int) bool {
		if results[i].Provider != results[j].Provider {
			return results[i].Provider < results[j].Provider
		}
		return results[i].ID < results[j].ID
	})

	return results
}

// ModelFamily represents a grouping of related models.
type ModelFamily string

const (
	// OpenAI model families
	FamilyGPT4o      ModelFamily = "gpt-4o"
	FamilyGPT4       ModelFamily = "gpt-4"
	FamilyGPT35      ModelFamily = "gpt-3.5"
	FamilyO1         ModelFamily = "o1"
	FamilyO3         ModelFamily = "o3"

	// Anthropic model families
	FamilyClaudeOpus   ModelFamily = "claude-opus"
	FamilyClaudeSonnet ModelFamily = "claude-sonnet"
	FamilyClaudeHaiku  ModelFamily = "claude-haiku"

	// Google model families
	FamilyGemini20 ModelFamily = "gemini-2.0"
	FamilyGemini15 ModelFamily = "gemini-1.5"
	FamilyGemini1  ModelFamily = "gemini-1.0"
)

// ModelTier represents the quality/capability tier of a model.
type ModelTier string

const (
	TierFlagship  ModelTier = "flagship"  // Most capable models
	TierStandard  ModelTier = "standard"  // Good balance of capability and cost
	TierEconomy   ModelTier = "economy"   // Cost-optimized models
	TierReasoning ModelTier = "reasoning" // Specialized reasoning models
)

// PricingTier represents token count thresholds for tiered pricing.
type PricingTier struct {
	// UpToTokens is the upper bound of tokens for this tier (0 means unlimited)
	UpToTokens int64 `json:"up_to_tokens"`

	// PricePerMInputTokens is the input price per million tokens for this tier
	PricePerMInputTokens float64 `json:"price_per_m_input_tokens"`

	// PricePerMOutputTokens is the output price per million tokens for this tier
	PricePerMOutputTokens float64 `json:"price_per_m_output_tokens"`
}

// ExtendedModelInfo provides comprehensive model information beyond the base ModelInfo.
type ExtendedModelInfo struct {
	ModelInfo

	// Family groups related models together
	Family ModelFamily `json:"family,omitempty"`

	// Tier indicates the model's capability/cost tier
	Tier ModelTier `json:"tier,omitempty"`

	// ReleaseDate is when the model was released
	ReleaseDate *time.Time `json:"release_date,omitempty"`

	// Description provides a brief description of the model
	Description string `json:"description,omitempty"`

	// UseCases lists recommended use cases for this model
	UseCases []string `json:"use_cases,omitempty"`

	// CachePricing contains cache-related pricing (Anthropic-specific)
	CachePricing *CachePricing `json:"cache_pricing,omitempty"`

	// BatchPricing contains batch API pricing if available
	BatchPricing *BatchPricing `json:"batch_pricing,omitempty"`

	// TieredPricing contains token-volume based pricing tiers
	TieredPricing []PricingTier `json:"tiered_pricing,omitempty"`

	// ContextLengthCategories maps named context lengths to token counts
	// e.g., "standard": 128000, "extended": 1000000
	ContextLengthCategories map[string]int `json:"context_length_categories,omitempty"`
}

// CachePricing contains pricing information for cached prompts.
type CachePricing struct {
	// PricePerMCacheWriteTokens is the price per million tokens for cache writes
	PricePerMCacheWriteTokens float64 `json:"price_per_m_cache_write_tokens"`

	// PricePerMCacheReadTokens is the price per million tokens for cache reads
	PricePerMCacheReadTokens float64 `json:"price_per_m_cache_read_tokens"`

	// CacheTTLMinutes is the cache time-to-live in minutes
	CacheTTLMinutes int `json:"cache_ttl_minutes,omitempty"`

	// MinCacheableTokens is the minimum prompt size to enable caching
	MinCacheableTokens int `json:"min_cacheable_tokens,omitempty"`
}

// BatchPricing contains pricing for batch API requests.
type BatchPricing struct {
	// PricePerMInputTokens for batch requests (usually 50% of standard)
	PricePerMInputTokens float64 `json:"price_per_m_input_tokens"`

	// PricePerMOutputTokens for batch requests
	PricePerMOutputTokens float64 `json:"price_per_m_output_tokens"`

	// MaxTurnaroundHours is the maximum batch completion time
	MaxTurnaroundHours int `json:"max_turnaround_hours,omitempty"`
}

// UsageEstimate represents an estimate of token usage and associated costs.
type UsageEstimate struct {
	// InputTokens is the estimated number of input tokens
	InputTokens int `json:"input_tokens"`

	// OutputTokens is the estimated number of output tokens
	OutputTokens int `json:"output_tokens"`

	// InputCost is the cost for input tokens in USD
	InputCost float64 `json:"input_cost"`

	// OutputCost is the cost for output tokens in USD
	OutputCost float64 `json:"output_cost"`

	// TotalCost is the total estimated cost in USD
	TotalCost float64 `json:"total_cost"`

	// CacheReadCost is the cost for cache reads (if applicable)
	CacheReadCost float64 `json:"cache_read_cost,omitempty"`

	// CacheWriteCost is the cost for cache writes (if applicable)
	CacheWriteCost float64 `json:"cache_write_cost,omitempty"`
}

// CostCalculator provides utilities for calculating AI API costs.
type CostCalculator struct {
	registry *ProviderRegistry
}

// NewCostCalculator creates a new CostCalculator using the given registry.
func NewCostCalculator(registry *ProviderRegistry) *CostCalculator {
	return &CostCalculator{registry: registry}
}

// CalculateCost calculates the cost for the given usage on a specific model.
func (c *CostCalculator) CalculateCost(modelID string, usage Usage) (*UsageEstimate, error) {
	info, err := c.registry.GetModelInfo(modelID)
	if err != nil {
		return nil, err
	}

	inputCost := float64(usage.PromptTokens) * info.PricePerMInputTokens / 1_000_000
	outputCost := float64(usage.CompletionTokens) * info.PricePerMOutputTokens / 1_000_000

	estimate := &UsageEstimate{
		InputTokens:  usage.PromptTokens,
		OutputTokens: usage.CompletionTokens,
		InputCost:    inputCost,
		OutputCost:   outputCost,
		TotalCost:    inputCost + outputCost,
	}

	// Handle cache costs if available (Anthropic-specific)
	if usage.CacheReadTokens > 0 || usage.CacheWriteTokens > 0 {
		// Anthropic cache pricing: read is 10% of input, write is 25% more than input
		cacheReadPrice := info.PricePerMInputTokens * 0.1
		cacheWritePrice := info.PricePerMInputTokens * 1.25

		estimate.CacheReadCost = float64(usage.CacheReadTokens) * cacheReadPrice / 1_000_000
		estimate.CacheWriteCost = float64(usage.CacheWriteTokens) * cacheWritePrice / 1_000_000
		estimate.TotalCost += estimate.CacheReadCost + estimate.CacheWriteCost
	}

	return estimate, nil
}

// CalculateCostFromTokens calculates the cost for given token counts on a specific model.
func (c *CostCalculator) CalculateCostFromTokens(modelID string, inputTokens, outputTokens int) (*UsageEstimate, error) {
	return c.CalculateCost(modelID, Usage{
		PromptTokens:     inputTokens,
		CompletionTokens: outputTokens,
		TotalTokens:      inputTokens + outputTokens,
	})
}

// EstimateCostFromContent estimates cost based on content string and expected output.
func (c *CostCalculator) EstimateCostFromContent(modelID string, content string, estimatedOutputTokens int) (*UsageEstimate, error) {
	provider, err := c.registry.GetForModel(modelID)
	if err != nil {
		return nil, err
	}

	inputTokens := provider.EstimateTokens(content)
	return c.CalculateCostFromTokens(modelID, inputTokens, estimatedOutputTokens)
}

// CompareCosts compares costs across multiple models for the same usage.
func (c *CostCalculator) CompareCosts(usage Usage, modelIDs ...string) (map[string]*UsageEstimate, error) {
	results := make(map[string]*UsageEstimate)

	for _, modelID := range modelIDs {
		estimate, err := c.CalculateCost(modelID, usage)
		if err != nil {
			// Skip models that can't be found
			continue
		}
		results[modelID] = estimate
	}

	return results, nil
}

// FindCheapestModel finds the cheapest model that meets capability requirements.
func (c *CostCalculator) FindCheapestModel(usage Usage, caps ProviderCapabilities) (*ModelInfo, *UsageEstimate, error) {
	selector := NewModelSelector(c.registry)
	matches := selector.SelectByCapabilities(caps)

	if len(matches) == 0 {
		return nil, nil, &ProviderError{
			Code:    ErrCodeNotFound,
			Message: "no models match the required capabilities",
		}
	}

	var cheapestModel *ModelInfo
	var cheapestEstimate *UsageEstimate
	var minCost float64 = -1

	for _, model := range matches {
		estimate, err := c.CalculateCost(model.ID, usage)
		if err != nil {
			continue
		}

		if minCost < 0 || estimate.TotalCost < minCost {
			minCost = estimate.TotalCost
			cheapestModel = model
			cheapestEstimate = estimate
		}
	}

	if cheapestModel == nil {
		return nil, nil, &ProviderError{
			Code:    ErrCodeNotFound,
			Message: "unable to calculate costs for any matching model",
		}
	}

	return cheapestModel, cheapestEstimate, nil
}

// ModelCatalog provides a centralized catalog of all known models with their
// extended information.
type ModelCatalog struct {
	models map[string]*ExtendedModelInfo
}

// NewModelCatalog creates a new ModelCatalog with default model data.
func NewModelCatalog() *ModelCatalog {
	catalog := &ModelCatalog{
		models: make(map[string]*ExtendedModelInfo),
	}
	catalog.initializeDefaultModels()
	
	// Attempt to load user overrides/custom models
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, ".openexec", "models.json")
	if _, err := os.Stat(configPath); err == nil {
		_ = catalog.LoadFromConfig(configPath)
	}
	
	return catalog
}

// GetModel returns extended information for a specific model.
func (c *ModelCatalog) GetModel(modelID string) (*ExtendedModelInfo, bool) {
	info, ok := c.models[modelID]
	return info, ok
}

// GetModelsByFamily returns all models in a specific family.
func (c *ModelCatalog) GetModelsByFamily(family ModelFamily) []*ExtendedModelInfo {
	var results []*ExtendedModelInfo
	for _, model := range c.models {
		if model.Family == family {
			results = append(results, model)
		}
	}
	return results
}

// GetModelsByTier returns all models in a specific tier.
func (c *ModelCatalog) GetModelsByTier(tier ModelTier) []*ExtendedModelInfo {
	var results []*ExtendedModelInfo
	for _, model := range c.models {
		if model.Tier == tier {
			results = append(results, model)
		}
	}
	return results
}

// GetModelsByProvider returns all models from a specific provider.
func (c *ModelCatalog) GetModelsByProvider(provider string) []*ExtendedModelInfo {
	var results []*ExtendedModelInfo
	for _, model := range c.models {
		if model.Provider == provider {
			results = append(results, model)
		}
	}

	// Sort by ID for consistent ordering
	sort.Slice(results, func(i, j int) bool {
		return results[i].ID < results[j].ID
	})

	return results
}

// GetAllModels returns all models in the catalog.
func (c *ModelCatalog) GetAllModels() []*ExtendedModelInfo {
	results := make([]*ExtendedModelInfo, 0, len(c.models))
	for _, model := range c.models {
		results = append(results, model)
	}

	// Sort by provider then ID
	sort.Slice(results, func(i, j int) bool {
		if results[i].Provider != results[j].Provider {
			return results[i].Provider < results[j].Provider
		}
		return results[i].ID < results[j].ID
	})

	return results
}

// GetNonDeprecatedModels returns all non-deprecated models.
func (c *ModelCatalog) GetNonDeprecatedModels() []*ExtendedModelInfo {
	var results []*ExtendedModelInfo
	for _, model := range c.models {
		if !model.Deprecated {
			results = append(results, model)
		}
	}
	return results
}

// initializeDefaultModels populates the catalog with known model information.
func (c *ModelCatalog) initializeDefaultModels() {
	// ============================================
	// OpenAI Models
	// ============================================

	// GPT-4o family
	c.models[ModelGPT4o] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       ModelGPT4o,
			Name:     "GPT-4o",
			Provider: "openai",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 128000,
				MaxOutputTokens:  16384,
			},
			PricePerMInputTokens:  2.50,
			PricePerMOutputTokens: 10.00,
		},
		Family:      FamilyGPT4o,
		Tier:        TierStandard,
		Description: "Most advanced multimodal model with vision, text, and audio capabilities",
		UseCases:    []string{"complex reasoning", "vision tasks", "code generation", "creative writing"},
		BatchPricing: &BatchPricing{
			PricePerMInputTokens:  1.25,
			PricePerMOutputTokens: 5.00,
			MaxTurnaroundHours:    24,
		},
	}

	c.models[ModelGPT4oMini] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       ModelGPT4oMini,
			Name:     "GPT-4o Mini",
			Provider: "openai",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 128000,
				MaxOutputTokens:  16384,
			},
			PricePerMInputTokens:  0.15,
			PricePerMOutputTokens: 0.60,
		},
		Family:      FamilyGPT4o,
		Tier:        TierEconomy,
		Description: "Small, fast, and affordable multimodal model",
		UseCases:    []string{"high-volume tasks", "real-time applications", "simple reasoning"},
		BatchPricing: &BatchPricing{
			PricePerMInputTokens:  0.075,
			PricePerMOutputTokens: 0.30,
			MaxTurnaroundHours:    24,
		},
	}

	// GPT-4 family
	c.models[ModelGPT4Turbo] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       ModelGPT4Turbo,
			Name:     "GPT-4 Turbo",
			Provider: "openai",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 128000,
				MaxOutputTokens:  4096,
			},
			PricePerMInputTokens:  10.00,
			PricePerMOutputTokens: 30.00,
		},
		Family:      FamilyGPT4,
		Tier:        TierFlagship,
		Description: "Enhanced GPT-4 with vision and improved performance",
		UseCases:    []string{"complex analysis", "document processing", "vision tasks"},
	}

	c.models[ModelGPT4] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       ModelGPT4,
			Name:     "GPT-4",
			Provider: "openai",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           false,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 8192,
				MaxOutputTokens:  4096,
			},
			PricePerMInputTokens:  30.00,
			PricePerMOutputTokens: 60.00,
		},
		Family:      FamilyGPT4,
		Tier:        TierFlagship,
		Description: "Original GPT-4 model with 8K context",
		UseCases:    []string{"complex reasoning", "code review", "analysis"},
	}

	// GPT-3.5 family
	c.models[ModelGPT35Turbo] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       ModelGPT35Turbo,
			Name:     "GPT-3.5 Turbo",
			Provider: "openai",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           false,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 16385,
				MaxOutputTokens:  4096,
			},
			PricePerMInputTokens:  0.50,
			PricePerMOutputTokens: 1.50,
		},
		Family:      FamilyGPT35,
		Tier:        TierEconomy,
		Description: "Fast and cost-effective for simpler tasks",
		UseCases:    []string{"chatbots", "simple queries", "text classification"},
	}

	// O1 reasoning family
	c.models[ModelO1] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       ModelO1,
			Name:     "O1",
			Provider: "openai",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          false,
				SystemPrompt:     false,
				MultiTurn:        true,
				MaxContextTokens: 200000,
				MaxOutputTokens:  100000,
			},
			PricePerMInputTokens:  15.00,
			PricePerMOutputTokens: 60.00,
		},
		Family:      FamilyO1,
		Tier:        TierReasoning,
		Description: "Advanced reasoning model for complex problem-solving",
		UseCases:    []string{"mathematical reasoning", "scientific analysis", "complex problem-solving"},
	}

	c.models[ModelO1Mini] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       ModelO1Mini,
			Name:     "O1 Mini",
			Provider: "openai",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           false,
				ToolUse:          false,
				SystemPrompt:     false,
				MultiTurn:        true,
				MaxContextTokens: 128000,
				MaxOutputTokens:  65536,
			},
			PricePerMInputTokens:  3.00,
			PricePerMOutputTokens: 12.00,
		},
		Family:      FamilyO1,
		Tier:        TierReasoning,
		Description: "Smaller, faster reasoning model",
		UseCases:    []string{"coding tasks", "math problems", "logical reasoning"},
	}

	c.models[ModelO1Preview] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:         ModelO1Preview,
			Name:       "O1 Preview",
			Provider:   "openai",
			Deprecated: true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           false,
				ToolUse:          false,
				SystemPrompt:     false,
				MultiTurn:        true,
				MaxContextTokens: 128000,
				MaxOutputTokens:  32768,
			},
			PricePerMInputTokens:  15.00,
			PricePerMOutputTokens: 60.00,
		},
		Family:      FamilyO1,
		Tier:        TierReasoning,
		Description: "Preview version of O1 (deprecated)",
	}

	c.models[ModelO3Mini] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       ModelO3Mini,
			Name:     "O3 Mini",
			Provider: "openai",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           false,
				ToolUse:          true,
				SystemPrompt:     false,
				MultiTurn:        true,
				MaxContextTokens: 200000,
				MaxOutputTokens:  100000,
			},
			PricePerMInputTokens:  1.10,
			PricePerMOutputTokens: 4.40,
		},
		Family:      FamilyO3,
		Tier:        TierReasoning,
		Description: "Efficient reasoning model with tool use support",
		UseCases:    []string{"code generation", "agentic tasks", "reasoning with tools"},
	}

	// ============================================
	// Anthropic Models
	// ============================================

	// Claude Opus family
	c.models["claude-opus-4-5-20251101"] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       "claude-opus-4-5-20251101",
			Name:     "Claude Opus 4.5",
			Provider: "anthropic",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 200000,
				MaxOutputTokens:  32000,
			},
			PricePerMInputTokens:  15.0,
			PricePerMOutputTokens: 75.0,
		},
		Family:      FamilyClaudeOpus,
		Tier:        TierFlagship,
		Description: "Most capable Claude model for complex tasks",
		UseCases:    []string{"complex analysis", "research", "creative writing", "code architecture"},
		CachePricing: &CachePricing{
			PricePerMCacheWriteTokens: 18.75,
			PricePerMCacheReadTokens:  1.50,
			CacheTTLMinutes:           5,
			MinCacheableTokens:        1024,
		},
		BatchPricing: &BatchPricing{
			PricePerMInputTokens:  7.50,
			PricePerMOutputTokens: 37.50,
			MaxTurnaroundHours:    24,
		},
	}

	c.models["claude-3-opus-20240229"] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       "claude-3-opus-20240229",
			Name:     "Claude 3 Opus",
			Provider: "anthropic",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 200000,
				MaxOutputTokens:  4096,
			},
			PricePerMInputTokens:  15.0,
			PricePerMOutputTokens: 75.0,
		},
		Family:      FamilyClaudeOpus,
		Tier:        TierFlagship,
		Description: "Claude 3 flagship model for complex tasks",
		UseCases:    []string{"complex analysis", "research", "creative writing"},
	}

	// Claude Sonnet family
	c.models["claude-sonnet-4-20250514"] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       "claude-sonnet-4-20250514",
			Name:     "Claude Sonnet 4",
			Provider: "anthropic",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 200000,
				MaxOutputTokens:  64000,
			},
			PricePerMInputTokens:  3.0,
			PricePerMOutputTokens: 15.0,
		},
		Family:      FamilyClaudeSonnet,
		Tier:        TierStandard,
		Description: "Balanced Claude model for most tasks",
		UseCases:    []string{"code generation", "analysis", "writing", "general tasks"},
		CachePricing: &CachePricing{
			PricePerMCacheWriteTokens: 3.75,
			PricePerMCacheReadTokens:  0.30,
			CacheTTLMinutes:           5,
			MinCacheableTokens:        1024,
		},
		BatchPricing: &BatchPricing{
			PricePerMInputTokens:  1.50,
			PricePerMOutputTokens: 7.50,
			MaxTurnaroundHours:    24,
		},
	}

	c.models["claude-3-5-sonnet-20241022"] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       "claude-3-5-sonnet-20241022",
			Name:     "Claude 3.5 Sonnet",
			Provider: "anthropic",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 200000,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  3.0,
			PricePerMOutputTokens: 15.0,
		},
		Family:      FamilyClaudeSonnet,
		Tier:        TierStandard,
		Description: "Claude 3.5 balanced model",
		UseCases:    []string{"code generation", "analysis", "writing"},
	}

	c.models["claude-3-sonnet-20240229"] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       "claude-3-sonnet-20240229",
			Name:     "Claude 3 Sonnet",
			Provider: "anthropic",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 200000,
				MaxOutputTokens:  4096,
			},
			PricePerMInputTokens:  3.0,
			PricePerMOutputTokens: 15.0,
		},
		Family:      FamilyClaudeSonnet,
		Tier:        TierStandard,
		Description: "Claude 3 balanced model",
	}

	// Claude Haiku family
	c.models["claude-3-5-haiku-20241022"] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       "claude-3-5-haiku-20241022",
			Name:     "Claude 3.5 Haiku",
			Provider: "anthropic",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 200000,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  1.0,
			PricePerMOutputTokens: 5.0,
		},
		Family:      FamilyClaudeHaiku,
		Tier:        TierEconomy,
		Description: "Fast and efficient Claude model",
		UseCases:    []string{"high-volume tasks", "simple queries", "real-time applications"},
		BatchPricing: &BatchPricing{
			PricePerMInputTokens:  0.50,
			PricePerMOutputTokens: 2.50,
			MaxTurnaroundHours:    24,
		},
	}

	c.models["claude-3-haiku-20240307"] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       "claude-3-haiku-20240307",
			Name:     "Claude 3 Haiku",
			Provider: "anthropic",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 200000,
				MaxOutputTokens:  4096,
			},
			PricePerMInputTokens:  0.25,
			PricePerMOutputTokens: 1.25,
		},
		Family:      FamilyClaudeHaiku,
		Tier:        TierEconomy,
		Description: "Most cost-effective Claude model",
		UseCases:    []string{"high-volume tasks", "simple queries", "classification"},
	}

	// ============================================
	// Google Gemini Models
	// ============================================

	// Gemini 2.0 family
	c.models[ModelGemini20FlashExp] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       ModelGemini20FlashExp,
			Name:     "Gemini 2.0 Flash Experimental",
			Provider: "gemini",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 1048576,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  0.0,
			PricePerMOutputTokens: 0.0,
		},
		Family:      FamilyGemini20,
		Tier:        TierStandard,
		Description: "Experimental next-gen Gemini model (free during preview)",
		UseCases:    []string{"experimentation", "testing new features"},
	}

	c.models[ModelGemini20Flash] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       ModelGemini20Flash,
			Name:     "Gemini 2.0 Flash",
			Provider: "gemini",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 1048576,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  0.10,
			PricePerMOutputTokens: 0.40,
		},
		Family:      FamilyGemini20,
		Tier:        TierEconomy,
		Description: "Fast and efficient Gemini 2.0 model",
		UseCases:    []string{"high-volume tasks", "real-time applications", "multimodal tasks"},
		TieredPricing: []PricingTier{
			{UpToTokens: 128000, PricePerMInputTokens: 0.10, PricePerMOutputTokens: 0.40},
			{UpToTokens: 0, PricePerMInputTokens: 0.10, PricePerMOutputTokens: 0.40}, // Same for long context
		},
	}

	// Gemini 1.5 family
	c.models[ModelGemini15Pro] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       ModelGemini15Pro,
			Name:     "Gemini 1.5 Pro",
			Provider: "gemini",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 2097152,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  1.25,
			PricePerMOutputTokens: 5.00,
		},
		Family:      FamilyGemini15,
		Tier:        TierStandard,
		Description: "Advanced Gemini model with 2M context window",
		UseCases:    []string{"long document analysis", "code review", "research"},
		TieredPricing: []PricingTier{
			{UpToTokens: 128000, PricePerMInputTokens: 1.25, PricePerMOutputTokens: 5.00},
			{UpToTokens: 0, PricePerMInputTokens: 2.50, PricePerMOutputTokens: 10.00}, // Long context pricing
		},
		ContextLengthCategories: map[string]int{
			"standard": 128000,
			"extended": 2097152,
		},
	}

	c.models[ModelGemini15ProLatest] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       ModelGemini15ProLatest,
			Name:     "Gemini 1.5 Pro Latest",
			Provider: "gemini",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 2097152,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  1.25,
			PricePerMOutputTokens: 5.00,
		},
		Family:      FamilyGemini15,
		Tier:        TierStandard,
		Description: "Latest version of Gemini 1.5 Pro",
	}

	c.models[ModelGemini15Flash] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       ModelGemini15Flash,
			Name:     "Gemini 1.5 Flash",
			Provider: "gemini",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 1048576,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  0.075,
			PricePerMOutputTokens: 0.30,
		},
		Family:      FamilyGemini15,
		Tier:        TierEconomy,
		Description: "Fast Gemini model with 1M context",
		UseCases:    []string{"real-time tasks", "high-volume processing", "cost-effective analysis"},
		TieredPricing: []PricingTier{
			{UpToTokens: 128000, PricePerMInputTokens: 0.075, PricePerMOutputTokens: 0.30},
			{UpToTokens: 0, PricePerMInputTokens: 0.15, PricePerMOutputTokens: 0.60}, // Long context pricing
		},
	}

	c.models[ModelGemini15FlashLatest] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       ModelGemini15FlashLatest,
			Name:     "Gemini 1.5 Flash Latest",
			Provider: "gemini",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 1048576,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  0.075,
			PricePerMOutputTokens: 0.30,
		},
		Family:      FamilyGemini15,
		Tier:        TierEconomy,
		Description: "Latest version of Gemini 1.5 Flash",
	}

	c.models[ModelGemini15Flash8B] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:       ModelGemini15Flash8B,
			Name:     "Gemini 1.5 Flash 8B",
			Provider: "gemini",
			Enabled:  true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 1048576,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  0.0375,
			PricePerMOutputTokens: 0.15,
		},
		Family:      FamilyGemini15,
		Tier:        TierEconomy,
		Description: "Most efficient Gemini model",
		UseCases:    []string{"high-volume processing", "simple tasks", "cost optimization"},
		TieredPricing: []PricingTier{
			{UpToTokens: 128000, PricePerMInputTokens: 0.0375, PricePerMOutputTokens: 0.15},
			{UpToTokens: 0, PricePerMInputTokens: 0.075, PricePerMOutputTokens: 0.30}, // Long context pricing
		},
	}

	// Legacy Gemini models
	c.models[ModelGeminiPro] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:         ModelGeminiPro,
			Name:       "Gemini Pro",
			Provider:   "gemini",
			Deprecated: true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           false,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 32760,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  0.50,
			PricePerMOutputTokens: 1.50,
		},
		Family:      FamilyGemini1,
		Tier:        TierStandard,
		Description: "Legacy Gemini Pro model (deprecated)",
	}

	c.models[ModelGeminiProVision] = &ExtendedModelInfo{
		ModelInfo: ModelInfo{
			ID:         ModelGeminiProVision,
			Name:       "Gemini Pro Vision",
			Provider:   "gemini",
			Deprecated: true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          false,
				SystemPrompt:     false,
				MultiTurn:        false,
				MaxContextTokens: 16384,
				MaxOutputTokens:  2048,
			},
			PricePerMInputTokens:  0.50,
			PricePerMOutputTokens: 1.50,
		},
		Family:      FamilyGemini1,
		Tier:        TierStandard,
		Description: "Legacy Gemini Pro Vision model (deprecated)",
	}
}

// FormatPrice formats a price per million tokens as a human-readable string.
func FormatPrice(pricePerMillion float64) string {
	if pricePerMillion == 0 {
		return "Free"
	}
	if pricePerMillion < 0.01 {
		return fmt.Sprintf("$%.4f/M", pricePerMillion)
	}
	if pricePerMillion < 1 {
		return fmt.Sprintf("$%.3f/M", pricePerMillion)
	}
	return fmt.Sprintf("$%.2f/M", pricePerMillion)
}

// FormatCost formats a cost value in USD as a human-readable string.
func FormatCost(cost float64) string {
	if cost == 0 {
		return "$0.00"
	}
	if cost < 0.01 {
		return fmt.Sprintf("$%.6f", cost)
	}
	if cost < 1 {
		return fmt.Sprintf("$%.4f", cost)
	}
	return fmt.Sprintf("$%.2f", cost)
}

// DefaultModelCatalog is the global default catalog instance.
var DefaultModelCatalog = NewModelCatalog()

// DefaultCostCalculator uses the default registry.
var DefaultCostCalculator = NewCostCalculator(DefaultRegistry)
