// Package agent provides the Provider Registry for managing AI provider adapters.
// The registry provides a central location for registering, discovering, and
// instantiating AI providers (OpenAI, Anthropic, Gemini) in a unified manner.
package agent

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/openexec/openexec/pkg/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ProviderName represents a provider identifier.
type ProviderName string

const (
	ProviderOpenAI    ProviderName = "openai"
	ProviderAnthropic ProviderName = "anthropic"
	ProviderGemini    ProviderName = "gemini"
)

// ProviderFactory is a function that creates a new provider instance.
type ProviderFactory func() (ProviderAdapter, error)

// ProviderStatus represents the availability status of a provider.
type ProviderStatus struct {
	// Name is the provider identifier.
	Name ProviderName `json:"name"`

	// Available indicates whether the provider can be used.
	Available bool `json:"available"`

	// Reason explains why the provider is or isn't available.
	Reason string `json:"reason,omitempty"`

	// Models lists the available models for this provider.
	Models []string `json:"models,omitempty"`
}

// ProviderRegistry manages the registration and lookup of AI providers.
// It provides a centralized way to access all configured providers and their models.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[ProviderName]ProviderAdapter
	factories map[ProviderName]ProviderFactory
	// modelIndex maps model IDs to their provider for quick lookup
	modelIndex map[string]ProviderName
}

// NewProviderRegistry creates a new provider registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers:  make(map[ProviderName]ProviderAdapter),
		factories:  make(map[ProviderName]ProviderFactory),
		modelIndex: make(map[string]ProviderName),
	}
}

// RegisterFactory registers a factory function for creating a provider.
// The factory will be called lazily when the provider is first needed.
func (r *ProviderRegistry) RegisterFactory(name ProviderName, factory ProviderFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
}

// Register registers an already-instantiated provider.
func (r *ProviderRegistry) Register(provider ProviderAdapter) error {
	if provider == nil {
		return fmt.Errorf("provider cannot be nil")
	}

	name := ProviderName(provider.GetName())
	if name == "" {
		return fmt.Errorf("provider name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.providers[name] = provider

	// Build model index
	for _, modelID := range provider.GetModels() {
		r.modelIndex[modelID] = name
	}

	return nil
}

// Get retrieves a provider by name. If the provider hasn't been instantiated
// yet but a factory exists, the factory will be called.
func (r *ProviderRegistry) Get(name ProviderName) (ProviderAdapter, error) {
	r.mu.RLock()
	provider, exists := r.providers[name]
	factory, hasFactory := r.factories[name]
	r.mu.RUnlock()

	if exists && provider != nil {
		return provider, nil
	}

	if !hasFactory {
		return nil, &ProviderError{
			Code:    ErrCodeNotFound,
			Message: fmt.Sprintf("provider %q not registered", name),
		}
	}

	// Try to create the provider using the factory
	provider, err := factory()
	if err != nil {
		return nil, err
	}

	// Register the newly created provider
	if err := r.Register(provider); err != nil {
		return nil, err
	}

	return provider, nil
}

// MustGet retrieves a provider by name, panicking if not found.
// Use this only during initialization when provider availability is guaranteed.
func (r *ProviderRegistry) MustGet(name ProviderName) ProviderAdapter {
	provider, err := r.Get(name)
	if err != nil {
		panic(fmt.Sprintf("failed to get provider %q: %v", name, err))
	}
	return provider
}

// GetForModel retrieves the provider that supports a given model ID.
func (r *ProviderRegistry) GetForModel(modelID string) (ProviderAdapter, error) {
	r.mu.RLock()
	providerName, exists := r.modelIndex[modelID]
	r.mu.RUnlock()

	if !exists {
		// Model not in index - try to find it by checking all providers
		providers := r.ListProviders()
		for _, name := range providers {
			provider, err := r.Get(name)
			if err != nil {
				continue
			}
			for _, m := range provider.GetModels() {
				if m == modelID {
					return provider, nil
				}
			}
		}

		return nil, &ProviderError{
			Code:    ErrCodeNotFound,
			Message: fmt.Sprintf("no provider found for model %q", modelID),
		}
	}

	return r.Get(providerName)
}

// ListProviders returns all registered provider names.
func (r *ProviderRegistry) ListProviders() []ProviderName {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Combine instantiated providers and registered factories
	seen := make(map[ProviderName]bool)
	for name := range r.providers {
		seen[name] = true
	}
	for name := range r.factories {
		seen[name] = true
	}

	names := make([]ProviderName, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}

	// Sort for consistent ordering
	sort.Slice(names, func(i, j int) bool {
		return names[i] < names[j]
	})

	return names
}

// ListModels returns all available models across all registered providers.
func (r *ProviderRegistry) ListModels() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Collect models from all instantiated providers
	models := make([]string, 0)
	for _, provider := range r.providers {
		models = append(models, provider.GetModels()...)
	}

	// Sort for consistent ordering
	sort.Strings(models)

	return models
}

// GetModelInfo returns detailed information about a specific model.
func (r *ProviderRegistry) GetModelInfo(modelID string) (*ModelInfo, error) {
	provider, err := r.GetForModel(modelID)
	if err != nil {
		return nil, err
	}

	return provider.GetModelInfo(modelID)
}

// GetAllModelInfo returns model information for all available models.
func (r *ProviderRegistry) GetAllModelInfo() []*ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var infos []*ModelInfo
	for _, provider := range r.providers {
		for _, modelID := range provider.GetModels() {
			info, err := provider.GetModelInfo(modelID)
			if err == nil {
				infos = append(infos, info)
			}
		}
	}

	// Sort by provider then model ID
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].Provider != infos[j].Provider {
			return infos[i].Provider < infos[j].Provider
		}
		return infos[i].ID < infos[j].ID
	})

	return infos
}

// CheckAvailability checks if a provider is available and returns its status.
func (r *ProviderRegistry) CheckAvailability(name ProviderName) ProviderStatus {
	status := ProviderStatus{
		Name: name,
	}

	provider, err := r.Get(name)
	if err != nil {
		status.Available = false
		if provErr, ok := err.(*ProviderError); ok {
			status.Reason = provErr.Message
		} else {
			status.Reason = err.Error()
		}
		return status
	}

	status.Available = true
	status.Models = provider.GetModels()
	status.Reason = "configured and ready"

	return status
}

// CheckAllAvailability checks the availability of all registered providers.
func (r *ProviderRegistry) CheckAllAvailability() []ProviderStatus {
	providers := r.ListProviders()
	statuses := make([]ProviderStatus, 0, len(providers))

	for _, name := range providers {
		statuses = append(statuses, r.CheckAvailability(name))
	}

	return statuses
}

// HasProvider checks if a provider is registered (by name or factory).
func (r *ProviderRegistry) HasProvider(name ProviderName) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, hasProvider := r.providers[name]
	_, hasFactory := r.factories[name]

	return hasProvider || hasFactory
}

// HasModel checks if any registered provider supports the given model.
func (r *ProviderRegistry) HasModel(modelID string) bool {
	r.mu.RLock()
	if _, exists := r.modelIndex[modelID]; exists {
		r.mu.RUnlock()
		return true
	}
	r.mu.RUnlock()

	// Check instantiated providers
	providers := r.ListProviders()
	for _, name := range providers {
		provider, err := r.Get(name)
		if err != nil {
			continue
		}
		for _, m := range provider.GetModels() {
			if m == modelID {
				return true
			}
		}
	}

	return false
}

// Unregister removes a provider from the registry.
func (r *ProviderRegistry) Unregister(name ProviderName) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove from model index
	if provider, exists := r.providers[name]; exists {
		for _, modelID := range provider.GetModels() {
			delete(r.modelIndex, modelID)
		}
	}

	delete(r.providers, name)
	delete(r.factories, name)
}

// Clear removes all providers from the registry.
func (r *ProviderRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.providers = make(map[ProviderName]ProviderAdapter)
	r.factories = make(map[ProviderName]ProviderFactory)
	r.modelIndex = make(map[string]ProviderName)
}

// Stream sends a streaming request to the appropriate provider for the model.
func (r *ProviderRegistry) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	provider, err := r.GetForModel(req.Model)
	if err != nil {
		return nil, err
	}

	return provider.Stream(ctx, req)
}

// DefaultRegistry is the global default registry instance.
var DefaultRegistry = NewProviderRegistry()

// RegisterDefaultFactories registers the default provider factories.
// These factories create providers using environment variables for configuration.
func RegisterDefaultFactories(registry *ProviderRegistry) {
	// OpenAI factory
	registry.RegisterFactory(ProviderOpenAI, func() (ProviderAdapter, error) {
		return NewOpenAIProviderFromEnv()
	})

	// Anthropic factory
	registry.RegisterFactory(ProviderAnthropic, func() (ProviderAdapter, error) {
		return NewAnthropicProvider(AnthropicConfig{})
	})

	// Gemini factory
	registry.RegisterFactory(ProviderGemini, func() (ProviderAdapter, error) {
		return NewGeminiProviderFromEnv()
	})
}

// InitializeDefaultRegistry initializes the default registry with all available providers.
// Returns the number of providers successfully initialized.
func InitializeDefaultRegistry() int {
	RegisterDefaultFactories(DefaultRegistry)

	count := 0
	for _, name := range DefaultRegistry.ListProviders() {
		_, err := DefaultRegistry.Get(name)
		if err == nil {
			count++
		}
	}

	return count
}

// GetProvider retrieves a provider from the default registry.
func GetProvider(name ProviderName) (ProviderAdapter, error) {
	return DefaultRegistry.Get(name)
}

// GetProviderForModel retrieves the provider for a model from the default registry.
func GetProviderForModel(modelID string) (ProviderAdapter, error) {
	return DefaultRegistry.GetForModel(modelID)
}

// RequiredEnvVars returns the environment variable names required for each provider.
func RequiredEnvVars() map[ProviderName][]string {
	return map[ProviderName][]string{
		ProviderOpenAI:    {"OPENAI_API_KEY"},
		ProviderAnthropic: {"ANTHROPIC_API_KEY"},
		ProviderGemini:    {"GEMINI_API_KEY", "GOOGLE_API_KEY"}, // Either one works
	}
}

// CheckEnvVars checks which provider API keys are configured.
func CheckEnvVars() map[ProviderName]bool {
	result := make(map[ProviderName]bool)

	result[ProviderOpenAI] = os.Getenv("OPENAI_API_KEY") != ""
	result[ProviderAnthropic] = os.Getenv("ANTHROPIC_API_KEY") != ""
	result[ProviderGemini] = os.Getenv("GEMINI_API_KEY") != "" || os.Getenv("GOOGLE_API_KEY") != ""

	return result
}

// AvailableProviders returns a list of providers that have API keys configured.
func AvailableProviders() []ProviderName {
	envVars := CheckEnvVars()
	var available []ProviderName

	for name, configured := range envVars {
		if configured {
			available = append(available, name)
		}
	}

	// Sort for consistent ordering
	sort.Slice(available, func(i, j int) bool {
		return available[i] < available[j]
	})

	return available
}

// ModelSelector provides model selection utilities based on criteria.
type ModelSelector struct {
	registry *ProviderRegistry
}

// NewModelSelector creates a new ModelSelector using the given registry.
func NewModelSelector(registry *ProviderRegistry) *ModelSelector {
	return &ModelSelector{registry: registry}
}

// SelectByCapabilities finds models that match the given capability requirements.
func (s *ModelSelector) SelectByCapabilities(caps ProviderCapabilities) []*ModelInfo {
	var matches []*ModelInfo

	for _, info := range s.registry.GetAllModelInfo() {
		if s.matchesCapabilities(info.Capabilities, caps) {
			matches = append(matches, info)
		}
	}

	return matches
}

// matchesCapabilities checks if a model's capabilities meet the requirements.
func (s *ModelSelector) matchesCapabilities(model, required ProviderCapabilities) bool {
	if required.Streaming && !model.Streaming {
		return false
	}
	if required.Vision && !model.Vision {
		return false
	}
	if required.ToolUse && !model.ToolUse {
		return false
	}
	if required.SystemPrompt && !model.SystemPrompt {
		return false
	}
	if required.MultiTurn && !model.MultiTurn {
		return false
	}
	if required.MaxContextTokens > 0 && model.MaxContextTokens < required.MaxContextTokens {
		return false
	}
	if required.MaxOutputTokens > 0 && model.MaxOutputTokens < required.MaxOutputTokens {
		return false
	}

	return true
}

// SelectCheapest finds the cheapest model that meets the capability requirements.
func (s *ModelSelector) SelectCheapest(caps ProviderCapabilities) *ModelInfo {
	matches := s.SelectByCapabilities(caps)
	if len(matches) == 0 {
		return nil
	}

	// Sort by total price (input + output) and return cheapest
	sort.Slice(matches, func(i, j int) bool {
		priceI := matches[i].PricePerMInputTokens + matches[i].PricePerMOutputTokens
		priceJ := matches[j].PricePerMInputTokens + matches[j].PricePerMOutputTokens
		return priceI < priceJ
	})

	return matches[0]
}

// SelectByProvider finds all models for a specific provider.
func (s *ModelSelector) SelectByProvider(name ProviderName) []*ModelInfo {
	provider, err := s.registry.Get(name)
	if err != nil {
		return nil
	}

	var infos []*ModelInfo
	for _, modelID := range provider.GetModels() {
		info, err := provider.GetModelInfo(modelID)
		if err == nil {
			infos = append(infos, info)
		}
	}

	return infos
}

// SelectNonDeprecated returns only non-deprecated models.
func (s *ModelSelector) SelectNonDeprecated() []*ModelInfo {
	var active []*ModelInfo

	for _, info := range s.registry.GetAllModelInfo() {
		if !info.Deprecated {
			active = append(active, info)
		}
	}

	return active
}

// Complete selects the appropriate provider for the request and calls its Complete method.
// It includes OTel tracing for observability.
func (r *ProviderRegistry) Complete(ctx context.Context, req Request) (*Response, error) {
	ctx, span := telemetry.StartSpan(ctx, "Agent.Complete", trace.WithAttributes(
		attribute.String("gen_ai.request.model", req.Model),
		attribute.Int("gen_ai.request.max_tokens", req.MaxTokens),
	))
	defer span.End()

	provider, err := r.GetForModel(req.Model)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	resp, err := provider.Complete(ctx, req)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(
		attribute.Int("gen_ai.response.input_tokens", resp.Usage.PromptTokens),
		attribute.Int("gen_ai.response.output_tokens", resp.Usage.CompletionTokens),
	)

	return resp, nil
}

// DefaultModelSelector uses the default registry.
var DefaultModelSelector = NewModelSelector(DefaultRegistry)
