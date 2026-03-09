package router

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// BitNetRouter wraps a local 1-bit LLM for intent selection
type BitNetRouter struct {
	manager               *InferenceManager
	tools                 map[string]string // tool name -> description/schema
	skipAvailabilityCheck bool              // Used for unit tests
}

func NewBitNetRouter(modelPath string) *BitNetRouter {
	return &BitNetRouter{
		manager: NewInferenceManager(modelPath),
		tools:   make(map[string]string),
	}
}

// SetSkipAvailabilityCheck is used for unit testing to bypass environment checks
func (r *BitNetRouter) SetSkipAvailabilityCheck(skip bool) {
	r.skipAvailabilityCheck = skip
}

// CheckAvailability verifies if the model and inference engine are usable.
func (r *BitNetRouter) CheckAvailability() error {
	if r.skipAvailabilityCheck {
		return nil
	}
	return r.manager.EnsureReady()
}

func (r *BitNetRouter) RegisterTool(name, description, schema string) error {
	r.tools[name] = fmt.Sprintf("Description: %s, Schema: %s", description, schema)
	return nil
}

// ParseIntent invokes the local BitNet model to select a tool
func (r *BitNetRouter) ParseIntent(ctx context.Context, query string) (*Intent, error) {
	// Guard: check if we can even run the model
	if err := r.CheckAvailability(); err != nil {
		return nil, fmt.Errorf("local router unavailable: %w", err)
	}

	// 1. Build the local prompt for the 1-bit model
	prompt := r.buildPrompt(query)

	// 2. Invoke local inference engine
	output, err := r.runLocalInference(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("local inference failed: %w", err)
	}

	// 3. Parse the model output into an Intent struct
	return r.parseModelOutput(output)
}

func (r *BitNetRouter) buildPrompt(query string) string {
	var sb strings.Builder
	sb.WriteString("SYSTEM: You are a surgical tool selector. Select the correct tool and arguments based on the query.\n\n")
	sb.WriteString("AVAILABLE TOOLS:\n")
	for name, info := range r.tools {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", name, info))
	}
	sb.WriteString(fmt.Sprintf("\nQUERY: %s\n", query))
	sb.WriteString("JSON_INTENT:")
	return sb.String()
}

func (r *BitNetRouter) runLocalInference(ctx context.Context, prompt string) (string, error) {
	// If we are in skip mode (tests), use the keyword simulator
	if r.skipAvailabilityCheck {
		return r.simulateInference(prompt)
	}

	// Actual local execution via manager
	return r.manager.RunInference(ctx, prompt)
}

func (r *BitNetRouter) simulateInference(prompt string) (string, error) {
	queryPart := ""
	if idx := strings.LastIndex(prompt, "QUERY: "); idx != -1 {
		queryPart = strings.ToLower(prompt[idx:])
	}

	if strings.Contains(queryPart, "symbol") || strings.Contains(queryPart, "function") {
		return `{"tool_name": "read_symbol", "args": {"name": "Execute"}, "confidence": 0.95}`, nil
	}
	
	if strings.Contains(queryPart, "deploy") || strings.Contains(queryPart, "prod") {
		return `{"tool_name": "deploy", "args": {"env": "prod", "action": "push"}, "confidence": 0.98}`, nil
	}

	if strings.Contains(queryPart, "commit") || strings.Contains(queryPart, "save") || strings.Contains(queryPart, "push") {
		return `{"tool_name": "safe_commit", "args": {"message": "Update from OpenExec", "push": true}, "confidence": 0.99}`, nil
	}

	// Default to general chat if no surgical tool matches
	cleanQuery := strings.TrimPrefix(queryPart, "query: ")
	// Strip trailing "json_intent:" or other prompt markers
	if idx := strings.Index(cleanQuery, "\n"); idx != -1 {
		cleanQuery = cleanQuery[:idx]
	}
	cleanQuery = strings.TrimSpace(cleanQuery)
	
	return fmt.Sprintf(`{"tool_name": "general_chat", "args": {"query": %q}, "confidence": 0.50}`, cleanQuery), nil
}

func (r *BitNetRouter) parseModelOutput(output string) (*Intent, error) {
	intent := &Intent{}
	err := json.Unmarshal([]byte(output), intent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse model output: %w", err)
	}
	return intent, nil
}
