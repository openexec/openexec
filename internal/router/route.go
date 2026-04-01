package router

import (
	"context"
	"strings"

	"github.com/openexec/openexec/internal/mode"
	"github.com/openexec/openexec/internal/toolset"
)

// Sensitivity represents the data sensitivity level of a query.
type Sensitivity string

const (
	SensitivityLow    Sensitivity = "low"    // No PII, safe for any model
	SensitivityMedium Sensitivity = "medium" // May contain business data
	SensitivityHigh   Sensitivity = "high"   // Contains credentials, PII, or secrets
)

// RoutingPlan is the structured output from the local routing pipeline.
// It guides the session runtime on mode, toolset, and context assembly.
type RoutingPlan struct {
	// Mode determines the interaction mode: chat (read-only), task (scoped action), or run (blueprint).
	Mode mode.Mode `json:"mode"`

	// Toolset is the name of the selected toolset (e.g., "repo_readonly", "coding_backend").
	Toolset string `json:"toolset"`

	// RepoZones identifies relevant code areas (e.g., "internal/api", "pkg/db").
	RepoZones []string `json:"repo_zones"`

	// KnowledgeSources ranks knowledge sources by relevance.
	KnowledgeSources []string `json:"knowledge_sources"`

	// Sensitivity indicates the data sensitivity level of the query.
	Sensitivity Sensitivity `json:"sensitivity"`

	// NeedsFrontier indicates whether the task requires a frontier model (Claude).
	NeedsFrontier bool `json:"needs_frontier_model"`

	// Confidence is the overall routing confidence (0.0-1.0).
	Confidence float64 `json:"confidence"`

	// Intent is the underlying parsed intent if available.
	Intent *Intent `json:"intent,omitempty"`
}

// Route produces a full routing plan for a query by combining intent parsing
// with deterministic classification of mode, toolset, repo zones, sensitivity,
// and frontier model requirements.
func Route(ctx context.Context, r Router, query string, registry *toolset.Registry) (*RoutingPlan, error) {
	// Step 1: Parse intent via the provided router
	intent, err := r.ParseIntent(ctx, query)
	if err != nil {
		// If intent parsing fails, create a fallback intent
		intent = &Intent{
			ToolName:   GeneralChatTool,
			Args:       map[string]interface{}{"query": query},
			Confidence: 0.0,
		}
	}

	// Step 2: Classify mode based on query and intent
	selectedMode := classifyMode(query, intent)

	// Step 3: Select toolset
	selectedToolset := selectToolset(query, intent, registry)

	// Step 4: Identify relevant repo zones
	repoZones := identifyRepoZones(query)

	// Step 5: Rank knowledge sources
	knowledgeSources := rankKnowledgeSources(query)

	// Step 6: Detect sensitivity
	sensitivity := detectSensitivity(query)

	// Step 7: Decide if frontier model is needed
	needsFrontier := needsFrontierModel(intent, sensitivity, selectedMode)

	// Calculate overall confidence
	confidence := intent.Confidence
	if intent.Confidence == 0.0 {
		confidence = 0.3 // Low confidence for fallback routing
	}

	return &RoutingPlan{
		Mode:             selectedMode,
		Toolset:          selectedToolset,
		RepoZones:        repoZones,
		KnowledgeSources: knowledgeSources,
		Sensitivity:      sensitivity,
		NeedsFrontier:    needsFrontier,
		Confidence:       confidence,
		Intent:           intent,
	}, nil
}

// classifyMode determines the appropriate mode based on query characteristics.
// Chat: questions, explanations, no side effects
// Task: explicit actions, modifications, bounded scope
// Run: complex multi-step operations, automation keywords
func classifyMode(query string, intent *Intent) mode.Mode {
	lower := strings.ToLower(query)

	// Run indicators: automation, complex multi-step tasks
	runKeywords := []string{
		"implement", "refactor", "migrate", "convert",
		"fix all", "update all", "rewrite",
		"run the full", "execute pipeline", "deploy",
	}
	for _, kw := range runKeywords {
		if strings.Contains(lower, kw) {
			return mode.ModeRun
		}
	}

	// Chat indicators: questions, explanations, no action verbs
	chatKeywords := []string{
		"what is", "how does", "explain", "why",
		"describe", "tell me about", "what are",
		"can you explain", "understand",
	}
	for _, kw := range chatKeywords {
		if strings.Contains(lower, kw) {
			return mode.ModeChat
		}
	}

	// Task indicators: single actions, modifications
	taskKeywords := []string{
		"add", "remove", "delete", "create", "update",
		"fix", "change", "rename", "move", "copy",
		"edit", "modify", "set", "configure",
	}
	for _, kw := range taskKeywords {
		if strings.Contains(lower, kw) {
			return mode.ModeTask
		}
	}

	// Default: if read-only tool was suggested, use chat; otherwise task
	if intent.ToolName == "read_file" || intent.ToolName == GeneralChatTool {
		return mode.ModeChat
	}

	return mode.ModeTask
}

// identifyRepoZones extracts relevant code zones from the query.
// These are used for focused context assembly.
func identifyRepoZones(query string) []string {
	zones := []string{}
	lower := strings.ToLower(query)

	// Common code patterns that indicate specific areas
	zonePatterns := map[string][]string{
		"internal/api":     {"api", "endpoint", "handler", "route"},
		"internal/db":      {"database", "migration", "schema", "model"},
		"internal/auth":    {"auth", "login", "token", "permission"},
		"pkg/":             {"package", "library", "util", "helper"},
		"cmd/":             {"main", "cli", "command"},
		"internal/mcp":     {"mcp", "tool", "server"},
		"internal/loop":    {"loop", "execution", "process"},
		"internal/context": {"context", "gather"},
		"tests/":           {"test", "spec", "fixture"},
	}

	for zone, keywords := range zonePatterns {
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				zones = append(zones, zone)
				break
			}
		}
	}

	// Also check for explicit path references
	pathPrefixes := []string{"internal/", "pkg/", "cmd/", "api/", "src/"}
	for _, prefix := range pathPrefixes {
		if idx := strings.Index(lower, prefix); idx != -1 {
			end := idx
			for end < len(lower) && lower[end] != ' ' && lower[end] != '"' {
				end++
			}
			if end > idx {
				zones = append(zones, lower[idx:end])
			}
		}
	}

	// Deduplicate
	seen := make(map[string]bool)
	unique := []string{}
	for _, z := range zones {
		if !seen[z] {
			seen[z] = true
			unique = append(unique, z)
		}
	}

	return unique
}

// rankKnowledgeSources ranks knowledge sources by relevance to the query.
func rankKnowledgeSources(query string) []string {
	sources := []string{}
	lower := strings.ToLower(query)

	// Priority 1: Local documentation
	if strings.Contains(lower, "readme") || strings.Contains(lower, "doc") {
		sources = append(sources, "local_docs")
	}

	// Priority 2: Code symbols (always relevant for code queries)
	codeKeywords := []string{"function", "class", "method", "variable", "type", "struct", "interface"}
	for _, kw := range codeKeywords {
		if strings.Contains(lower, kw) {
			sources = append(sources, "code_symbols")
			break
		}
	}

	// Priority 3: Git history
	if strings.Contains(lower, "change") || strings.Contains(lower, "recent") ||
		strings.Contains(lower, "commit") || strings.Contains(lower, "history") {
		sources = append(sources, "git_history")
	}

	// Priority 4: Tests
	if strings.Contains(lower, "test") || strings.Contains(lower, "spec") {
		sources = append(sources, "test_files")
	}

	// Priority 5: Dependencies
	if strings.Contains(lower, "import") || strings.Contains(lower, "package") ||
		strings.Contains(lower, "dependency") || strings.Contains(lower, "module") {
		sources = append(sources, "dependencies")
	}

	// Default sources if none matched
	if len(sources) == 0 {
		sources = []string{"code_symbols", "local_docs"}
	}

	return sources
}

// detectSensitivity analyzes the query for potential sensitive data.
func detectSensitivity(query string) Sensitivity {
	lower := strings.ToLower(query)

	// High sensitivity keywords
	highKeywords := []string{
		"password", "secret", "key", "token", "credential",
		"api_key", "apikey", "private", "ssh", "gpg",
		"encrypt", "decrypt", "auth_token",
	}
	for _, kw := range highKeywords {
		if strings.Contains(lower, kw) {
			return SensitivityHigh
		}
	}

	// Medium sensitivity keywords
	mediumKeywords := []string{
		"email", "user", "customer", "account", "profile",
		"config", "setting", "environment", "env",
	}
	for _, kw := range mediumKeywords {
		if strings.Contains(lower, kw) {
			return SensitivityMedium
		}
	}

	return SensitivityLow
}

// needsFrontierModel determines if the task requires a frontier model vs local model.
// Local model handles: routing, classification, summarization
// Frontier model handles: implementation, complex reasoning, code generation
func needsFrontierModel(intent *Intent, sensitivity Sensitivity, m mode.Mode) bool {
	// High sensitivity always requires frontier (for proper handling)
	if sensitivity == SensitivityHigh {
		return true
	}

	// Run mode always requires frontier (complex multi-step tasks)
	if m == mode.ModeRun {
		return true
	}

	// Task mode with code generation requires frontier
	if m == mode.ModeTask {
		codeTools := []string{"write_file", "git_apply_patch", "run_shell_command"}
		for _, ct := range codeTools {
			if intent.ToolName == ct {
				return true
			}
		}
	}

	// Low confidence routing suggests complexity that needs frontier
	if intent.Confidence < 0.5 {
		return true
	}

	// Chat mode with simple read operations can stay local
	if m == mode.ModeChat && (intent.ToolName == "read_file" || intent.ToolName == GeneralChatTool) {
		return false
	}

	// Default: use frontier for safety
	return true
}

// selectToolset chooses the appropriate toolset based on query and intent.
func selectToolset(query string, intent *Intent, registry *toolset.Registry) string {
	if registry == nil {
		return "repo_readonly"
	}

	// Use the toolset selector to analyze the query
	selector := toolset.NewSelector(registry)
	toolsets := selector.SelectForTask(query)
	if len(toolsets) > 0 {
		return toolsets[0].Name
	}

	// If intent suggests a specific tool, find its toolset
	if intent.ToolName != "" && intent.ToolName != GeneralChatTool {
		ts := registry.GetToolsetForTool(intent.ToolName)
		if ts != nil {
			return ts.Name
		}
	}

	// Default to repo_readonly (safest option)
	return "repo_readonly"
}
