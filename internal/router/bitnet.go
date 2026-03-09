package router

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// FallbackConfidence is used when the router cannot determine intent.
	// This value MUST be >= the coordinator's low-confidence threshold (0.2)
	// to prevent double-fallback behavior.
	FallbackConfidence = 0.5

	// LowConfidenceThreshold is the minimum confidence for trusting model output.
	// Below this threshold, we fall back to general_chat.
	LowConfidenceThreshold = 0.2

	// GeneralChatTool is the fallback tool name used when intent cannot be determined.
	GeneralChatTool = "general_chat"

	// Prompt markers for query extraction
	promptQueryMarker      = "QUERY: "
	promptQueryMarkerLower = "query: "

	// Known error phrases that trigger fallback
	errorPhraseNoIntent = "could not determine intent"
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
	fallback := &Intent{
		ToolName:   GeneralChatTool,
		Args:       map[string]interface{}{"query": query},
		Confidence: FallbackConfidence,
	}

	// Guard: check if we can even run the model
	if err := r.CheckAvailability(); err != nil {
		// Fallback to general chat if model environment is missing
		return fallback, nil
	}

	// 1. Build the local prompt for the 1-bit model
	prompt := r.buildPrompt(query)

	// 2. Invoke local inference engine
	output, err := r.runLocalInference(ctx, prompt)
	if err != nil {
		// CRITICAL: ALWAYS fallback to general chat if inference fails (OOM, binary missing, model error)
		return fallback, nil
	}

	// 3. Parse the model output into an Intent struct
	intent, err := r.parseModelOutput(output)
	if err != nil {
		// Also check if the raw output itself is just an error message string
		if strings.Contains(strings.ToLower(output), errorPhraseNoIntent) {
			return fallback, nil
		}
		// Fallback to general chat if model output is malformed
		return fallback, nil
	}

	// 4. Threshold check: if confidence is extremely low, prefer chat fallback
	if intent.Confidence < LowConfidenceThreshold {
		return fallback, nil
	}

	return intent, nil
}

func (r *BitNetRouter) buildPrompt(query string) string {
	var sb strings.Builder
	sb.WriteString("SYSTEM: You are a surgical tool selector. Select the correct tool and arguments based on the query.\n\n")
	sb.WriteString("AVAILABLE TOOLS:\n")
	for name, info := range r.tools {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", name, info))
	}
	sb.WriteString(fmt.Sprintf("\n%s%s\n", promptQueryMarker, query))
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

// simulatedToolMatch defines a keyword-to-tool mapping for test inference simulation.
type simulatedToolMatch struct {
	keywords   []string
	toolName   string
	args       string // JSON-encoded args
	confidence float64
}

// simulatedToolMatches defines the keyword matching rules for test mode.
// Order matters: first match wins.
var simulatedToolMatches = []simulatedToolMatch{
	{[]string{"symbol", "function"}, "read_symbol", `{"name": "Execute"}`, 0.95},
	{[]string{"deploy", "prod"}, "deploy", `{"env": "prod", "action": "push"}`, 0.98},
	{[]string{"commit", "save", "push"}, "safe_commit", `{"message": "Update from OpenExec", "push": true}`, 0.99},
	{[]string{"wizard"}, "general_chat", `{"query": "wizard"}`, 0.99},
	{[]string{"init"}, "general_chat", `{"query": "init"}`, 0.99},
}

// matchesAnyKeyword returns true if query contains any of the given keywords.
func matchesAnyKeyword(query string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(query, kw) {
			return true
		}
	}
	return false
}

func (r *BitNetRouter) simulateInference(prompt string) (string, error) {
	lowerQuery := ""
	if idx := strings.LastIndex(prompt, promptQueryMarker); idx != -1 {
		lowerQuery = strings.ToLower(prompt[idx:])
	}

	// Check each tool match rule in order
	for _, match := range simulatedToolMatches {
		if matchesAnyKeyword(lowerQuery, match.keywords) {
			return fmt.Sprintf(`{"tool_name": %q, "args": %s, "confidence": %.2f}`,
				match.toolName, match.args, match.confidence), nil
		}
	}

	// Default to general chat if no surgical tool matches
	cleanQuery := strings.TrimPrefix(lowerQuery, promptQueryMarkerLower)
	// Strip trailing "json_intent:" or other prompt markers
	if idx := strings.Index(cleanQuery, "\n"); idx != -1 {
		cleanQuery = cleanQuery[:idx]
	}
	cleanQuery = strings.TrimSpace(cleanQuery)

	// Use json.Marshal to properly escape the query for JSON.
	// Go's %q uses Go escape syntax (\x00, \a) which is NOT valid JSON.
	// JSON requires \uXXXX for control characters.
	queryJSON, err := json.Marshal(cleanQuery)
	if err != nil {
		// Should never happen for a string, but be defensive
		queryJSON = []byte(`""`)
	}

	return fmt.Sprintf(`{"tool_name": %q, "args": {"query": %s}, "confidence": %.2f}`, GeneralChatTool, queryJSON, FallbackConfidence), nil
}

func (r *BitNetRouter) parseModelOutput(output string) (*Intent, error) {
	intent := &Intent{}
	err := json.Unmarshal([]byte(output), intent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse model output: %w", err)
	}
	return intent, nil
}
