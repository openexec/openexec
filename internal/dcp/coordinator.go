// Package dcp provides the Deterministic Control Plane - a thin tool-routing
// service layer for OpenExec.
//
// Architecture Note: The DCP is NOT an orchestration plane. All state management
// and phase transitions flow through Pipeline + Loop (internal/pipeline, internal/loop).
// The Coordinator's sole responsibilities are:
//   - BitNet intent routing (parsing queries into tool invocations)
//   - Tool registration and dispatch
//   - PII sanitization of inputs
//   - Confidence-based fallback to general_chat
//
// This separation ensures a single source of truth for orchestration state while
// allowing the DCP to focus purely on deterministic tool execution.
package dcp

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/logging"
	"github.com/openexec/openexec/internal/mode"
	"github.com/openexec/openexec/internal/router"
	"github.com/openexec/openexec/internal/tools"
	"github.com/openexec/openexec/internal/toolset"
	"github.com/openexec/openexec/pkg/util"
)

// Fallback reason constants for structured logging
const (
	FallbackReasonRouterError   = "router_error"
	FallbackReasonLowConfidence = "low_confidence"
	FallbackReasonMissingTool   = "missing_tool"
	FallbackReasonChatFailed    = "fallback_failed"
)

// IntentSuggestion represents a parsed intent without execution.
// Used when DCP is in suggest-only mode (default).
type IntentSuggestion struct {
	ToolName    string                 `json:"tool_name"`
	Args        map[string]interface{} `json:"args"`
	Confidence  float64                `json:"confidence"`
	Description string                 `json:"description"`
	IsFallback  bool                   `json:"is_fallback"`
}

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

	// Intent is the underlying intent suggestion if available.
	Intent *IntentSuggestion `json:"intent,omitempty"`
}

// Coordinator is a thin tool-routing layer for the Deterministic Control Plane.
// It routes queries to registered tools via BitNet intent parsing and handles
// fallback logic. It does NOT manage state, phases, or orchestration - those
// responsibilities belong to Pipeline and Loop.
//
// IMPORTANT: By default, DCP operates in suggest-only mode and does NOT execute tools.
// Tool execution is handled exclusively by MCP. Set AllowExecution=true to enable
// execution (requires explicit opt-in for security reasons).
type Coordinator struct {
	router          router.Router
	store           *knowledge.Store
	indexer         *knowledge.Indexer
	tools           map[string]tools.Tool
	log             *logging.Logger
	AllowExecution  bool // If false (default), only returns IntentSuggestion; MCP handles execution
	toolsetRegistry *toolset.Registry
	toolsetSelector *toolset.Selector
}

// CoordinatorOption allows optional configuration of the Coordinator
type CoordinatorOption func(*Coordinator)

// WithLogger sets a custom logger for the Coordinator
func WithLogger(l *logging.Logger) CoordinatorOption {
	return func(c *Coordinator) {
		c.log = l.WithComponent("dcp")
	}
}

// WithExecution enables tool execution mode (default is suggest-only).
// WARNING: Only enable this for trusted, controlled environments.
// In production, MCP should handle all tool execution.
func WithExecution(enabled bool) CoordinatorOption {
	return func(c *Coordinator) {
		c.AllowExecution = enabled
	}
}

// WithToolsetRegistry configures a custom toolset registry.
// If not provided, the default registry with built-in toolsets is used.
func WithToolsetRegistry(r *toolset.Registry) CoordinatorOption {
	return func(c *Coordinator) {
		c.toolsetRegistry = r
		c.toolsetSelector = toolset.NewSelector(r)
	}
}

func NewCoordinator(r router.Router, s *knowledge.Store, opts ...CoordinatorOption) *Coordinator {
	c := &Coordinator{
		router:  r,
		store:   s,
		indexer: knowledge.NewIndexer(s),
		tools:   make(map[string]tools.Tool),
	}
	for _, opt := range opts {
		opt(c)
	}

	// Initialize default logger if not provided via options
	if c.log == nil {
		c.log = logging.Default().WithComponent("dcp")
	}

	// Initialize default toolset registry if not provided
	if c.toolsetRegistry == nil {
		c.toolsetRegistry = toolset.NewRegistry()
		c.toolsetSelector = toolset.NewSelector(c.toolsetRegistry)
	}

	return c
}

// queryHash returns SHA256 prefix (8 hex chars) of query for correlation without PII
func queryHash(query string) string {
	h := sha256.Sum256([]byte(query))
	return fmt.Sprintf("%x", h[:4])
}

func (c *Coordinator) RegisterTool(t tools.Tool) {
	c.tools[t.Name()] = t
	c.router.RegisterTool(t.Name(), t.Description(), t.InputSchema())
}

func (c *Coordinator) GetRouter() router.Router {
	if c == nil {
		return nil
	}
	return c.router
}

// SyncKnowledge performs a full project re-index.
// Error is intentionally ignored: indexing is best-effort and the DCP
// continues to function with stale or empty knowledge when indexing fails.
func (c *Coordinator) SyncKnowledge(projectDir string) {
	_ = c.indexer.IndexProject(projectDir)
}

// SyncFile surgically updates symbols for a single file to handle line drift
func (c *Coordinator) SyncFile(filePath string) error {
	return c.indexer.IndexFile(filePath)
}

// ProcessQuery uses BitNet to parse intent and returns an IntentSuggestion.
// By default (AllowExecution=false), this returns a suggestion for MCP to execute.
// When AllowExecution=true, the tool is directly executed (use with caution).
func (c *Coordinator) ProcessQuery(ctx context.Context, query string) (any, error) {
	qHash := queryHash(query)

	// 1. Local Intent Routing (BitNet)
	intent, err := c.router.ParseIntent(ctx, query)
	if err != nil {
		// In suggest-only mode, return a fallback suggestion
		if !c.AllowExecution {
			return &IntentSuggestion{
				ToolName:    router.GeneralChatTool,
				Args:        map[string]interface{}{"query": query},
				Confidence:  0.0,
				Description: "Intent parsing failed; suggesting general chat",
				IsFallback:  true,
			}, nil
		}
		if result, ok := c.fallbackToChatWithReason(ctx, query, qHash, FallbackReasonRouterError, "", 0, err); ok {
			return result, nil
		}
		return nil, fmt.Errorf("intent routing failed: %w", err)
	}

	// 2. Threshold check: if confidence is too low, suggest general chat
	if intent.Confidence < router.LowConfidenceThreshold {
		if !c.AllowExecution {
			return &IntentSuggestion{
				ToolName:    router.GeneralChatTool,
				Args:        map[string]interface{}{"query": query},
				Confidence:  intent.Confidence,
				Description: fmt.Sprintf("Low confidence (%.2f); suggesting general chat instead of %s", intent.Confidence, intent.ToolName),
				IsFallback:  true,
			}, nil
		}
		if result, ok := c.fallbackToChatWithReason(ctx, query, qHash, FallbackReasonLowConfidence, intent.ToolName, intent.Confidence, nil); ok {
			return result, nil
		}
	}

	// 3. Sanitize model outputs
	sanitizeArgs(intent.Args)

	// 4. Fetch Tool (verify it exists)
	tool, ok := c.tools[intent.ToolName]
	if !ok {
		if !c.AllowExecution {
			return &IntentSuggestion{
				ToolName:    router.GeneralChatTool,
				Args:        map[string]interface{}{"query": query},
				Confidence:  intent.Confidence,
				Description: fmt.Sprintf("Tool %q not registered; suggesting general chat", intent.ToolName),
				IsFallback:  true,
			}, nil
		}
		if result, ok := c.fallbackToChatWithReason(ctx, query, qHash, FallbackReasonMissingTool, intent.ToolName, intent.Confidence, nil); ok {
			return result, nil
		}
		return nil, fmt.Errorf("tool %q selected by router but not registered in DCP", intent.ToolName)
	}

	// 5. Return suggestion or execute based on mode
	if !c.AllowExecution {
		// SUGGEST-ONLY MODE (default): Return intent for MCP to execute
		c.log.Info("Suggesting tool (no execution)",
			"tool", intent.ToolName,
			"confidence", intent.Confidence,
			"query_hash", qHash,
		)
		return &IntentSuggestion{
			ToolName:    intent.ToolName,
			Args:        intent.Args,
			Confidence:  intent.Confidence,
			Description: tool.Description(),
			IsFallback:  false,
		}, nil
	}

	// EXECUTION MODE: Execute directly (requires explicit opt-in)
	c.log.Info("Executing tool",
		"tool", intent.ToolName,
		"confidence", intent.Confidence,
		"query_hash", qHash,
	)
	return tool.Execute(ctx, intent.Args)
}

// fallbackToChatWithReason attempts to execute the general_chat tool as a fallback.
// Returns (result, true) if fallback succeeded, (nil, false) if no fallback available.
func (c *Coordinator) fallbackToChatWithReason(ctx context.Context, query, qHash, reason, originalTool string, originalConfidence float64, routerErr error) (any, bool) {
	chatTool, ok := c.tools[router.GeneralChatTool]
	if !ok {
		return nil, false
	}

	// Build structured log attributes
	attrs := []any{
		"reason", reason,
		"fallback_tool", router.GeneralChatTool,
		"query_hash", qHash,
	}

	// Add context-specific attributes
	switch reason {
	case FallbackReasonRouterError:
		if routerErr != nil {
			attrs = append(attrs, "error", routerErr.Error())
		}
	case FallbackReasonLowConfidence:
		attrs = append(attrs, "original_confidence", originalConfidence)
	case FallbackReasonMissingTool:
		attrs = append(attrs, "original_tool", originalTool)
	}

	c.log.Warn("Fallback triggered", attrs...)

	result, err := chatTool.Execute(ctx, map[string]interface{}{"query": query})
	if err != nil {
		c.log.Error("Fallback failed",
			"reason", FallbackReasonChatFailed,
			"error", err.Error(),
			"query_hash", qHash,
		)
		return nil, false
	}
	return result, true
}

// sanitizeString applies the full sanitization pipeline to a single string:
// 1. Basic sanitization (printable chars)
// 2. Scrub PII (GDPR compliance)
// 3. Mask Infrastructure (IPs)
func sanitizeString(s string) string {
	sanitized := util.SanitizeInput(s)
	scrubbed := util.ScrubPII(sanitized)
	return util.MaskInfrastructure(scrubbed)
}

// sanitizeArgs recursively cleans all string values in the arguments map,
// scrubbing PII and masking infrastructure details before any tool execution.
func sanitizeArgs(args map[string]interface{}) {
	for k, v := range args {
		switch val := v.(type) {
		case string:
			args[k] = sanitizeString(val)
		case map[string]interface{}:
			sanitizeArgs(val)
		case []interface{}:
			for i, item := range val {
				if s, ok := item.(string); ok {
					val[i] = sanitizeString(s)
				}
			}
		}
	}
}

// Route produces a full routing plan for a query, determining mode, toolset,
// context zones, and whether frontier model access is needed.
// This is the primary entry point for the converged architecture's local routing.
func (c *Coordinator) Route(ctx context.Context, query string) (*RoutingPlan, error) {
	qHash := queryHash(query)

	// Step 1: Parse intent (determines base tool/action)
	intent, err := c.router.ParseIntent(ctx, query)
	var intentSuggestion *IntentSuggestion
	if err != nil {
		// If intent parsing fails, create a fallback suggestion
		intentSuggestion = &IntentSuggestion{
			ToolName:    router.GeneralChatTool,
			Args:        map[string]interface{}{"query": query},
			Confidence:  0.0,
			Description: "Intent parsing failed; defaulting to chat",
			IsFallback:  true,
		}
	} else {
		intentSuggestion = &IntentSuggestion{
			ToolName:    intent.ToolName,
			Args:        intent.Args,
			Confidence:  intent.Confidence,
			Description: "", // Will be filled by tool lookup
			IsFallback:  false,
		}
	}

	// Step 2: Classify mode based on intent and query analysis
	selectedMode := c.classifyMode(query, intentSuggestion)

	// Step 3: Select toolset based on task characteristics
	selectedToolset := c.selectToolset(query, intentSuggestion)

	// Step 4: Identify relevant repo zones
	repoZones := c.identifyRepoZones(query)

	// Step 5: Rank knowledge sources
	knowledgeSources := c.rankKnowledgeSources(query)

	// Step 6: Detect sensitivity
	sensitivity := c.detectSensitivity(query)

	// Step 7: Decide if frontier model is needed
	needsFrontier := c.needsFrontierModel(intentSuggestion, sensitivity, selectedMode)

	// Calculate overall confidence
	confidence := intentSuggestion.Confidence
	if intentSuggestion.IsFallback {
		confidence = 0.3 // Low confidence for fallback routing
	}

	c.log.Info("Route complete",
		"mode", selectedMode,
		"toolset", selectedToolset,
		"needs_frontier", needsFrontier,
		"sensitivity", sensitivity,
		"confidence", confidence,
		"query_hash", qHash,
	)

	return &RoutingPlan{
		Mode:             selectedMode,
		Toolset:          selectedToolset,
		RepoZones:        repoZones,
		KnowledgeSources: knowledgeSources,
		Sensitivity:      sensitivity,
		NeedsFrontier:    needsFrontier,
		Confidence:       confidence,
		Intent:           intentSuggestion,
	}, nil
}

// classifyMode determines the appropriate mode based on query characteristics.
// Chat: questions, explanations, no side effects
// Task: explicit actions, modifications, bounded scope
// Run: complex multi-step operations, automation keywords
func (c *Coordinator) classifyMode(query string, intent *IntentSuggestion) mode.Mode {
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
	if intent.ToolName == "read_file" || intent.ToolName == router.GeneralChatTool {
		return mode.ModeChat
	}

	return mode.ModeTask
}

// selectToolset chooses the appropriate toolset based on query and intent.
func (c *Coordinator) selectToolset(query string, intent *IntentSuggestion) string {
	// Use the toolset selector to analyze the query
	toolsets := c.toolsetSelector.SelectForTask(query)
	if len(toolsets) > 0 {
		return toolsets[0].Name
	}

	// If intent suggests a specific tool, find its toolset
	if !intent.IsFallback && intent.ToolName != "" {
		ts := c.toolsetRegistry.GetToolsetForTool(intent.ToolName)
		if ts != nil {
			return ts.Name
		}
	}

	// Default to repo_readonly (safest option)
	return "repo_readonly"
}

// identifyRepoZones extracts relevant code zones from the query.
// These are used for focused context assembly.
func (c *Coordinator) identifyRepoZones(query string) []string {
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
			// Extract potential path up to space or end
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
func (c *Coordinator) rankKnowledgeSources(query string) []string {
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
func (c *Coordinator) detectSensitivity(query string) Sensitivity {
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

// needsFrontierModel determines if the task requires Claude (frontier) vs local model.
// Local model handles: routing, classification, summarization
// Frontier model handles: implementation, complex reasoning, code generation
func (c *Coordinator) needsFrontierModel(intent *IntentSuggestion, sensitivity Sensitivity, m mode.Mode) bool {
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
	if m == mode.ModeChat && (intent.ToolName == "read_file" || intent.ToolName == router.GeneralChatTool) {
		return false
	}

	// Default: use frontier for safety
	return true
}

// GetToolsetRegistry returns the coordinator's toolset registry.
func (c *Coordinator) GetToolsetRegistry() *toolset.Registry {
	return c.toolsetRegistry
}
