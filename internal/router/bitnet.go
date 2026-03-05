package router

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// BitNetRouter wraps a local 1-bit LLM for intent selection
type BitNetRouter struct {
	modelPath string
	tools     map[string]string // tool name -> description/schema
	skipAvailabilityCheck bool // Used for unit tests
}

func NewBitNetRouter(modelPath string) *BitNetRouter {
	return &BitNetRouter{
		modelPath: modelPath,
		tools:     make(map[string]string),
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
	// 1. Check if model file exists
	if _, err := os.Stat(r.modelPath); os.IsNotExist(err) {
		return fmt.Errorf("BitNet model not found at %s", r.modelPath)
	}

	// 2. Check if inference engine is installed
	if _, err := exec.LookPath("bitnet-cli"); err != nil {
		return fmt.Errorf("inference engine 'bitnet-cli' not found in PATH")
	}

	return nil
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

	// 2. Invoke local inference engine (e.g. bitnet-cli or llama.cpp wrapper)
	// In production, this would use a CGO binding or a persistent worker process.
	// For this scaffold, we simulate the tool selection logic.
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
	// The prompt now includes the query after "QUERY: "
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

	return "", fmt.Errorf("model could not determine intent with high confidence")
}

func (r *BitNetRouter) parseModelOutput(output string) (*Intent, error) {
	intent := &Intent{}
	err := json.Unmarshal([]byte(output), intent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse model output: %w", err)
	}
	return intent, nil
}
