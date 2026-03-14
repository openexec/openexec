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

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/logging"
	"github.com/openexec/openexec/internal/router"
	"github.com/openexec/openexec/internal/tools"
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

// Coordinator is a thin tool-routing layer for the Deterministic Control Plane.
// It routes queries to registered tools via BitNet intent parsing and handles
// fallback logic. It does NOT manage state, phases, or orchestration - those
// responsibilities belong to Pipeline and Loop.
//
// IMPORTANT: By default, DCP operates in suggest-only mode and does NOT execute tools.
// Tool execution is handled exclusively by MCP. Set AllowExecution=true to enable
// execution (requires explicit opt-in for security reasons).
type Coordinator struct {
	router         router.Router
	store          *knowledge.Store
	indexer        *knowledge.Indexer
	tools          map[string]tools.Tool
	log            *logging.Logger
	AllowExecution bool // If false (default), only returns IntentSuggestion; MCP handles execution
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

func NewCoordinator(r router.Router, s *knowledge.Store, opts ...CoordinatorOption) *Coordinator {
	c := &Coordinator{
		router:  r,
		store:   s,
		indexer: knowledge.NewIndexer(s),
		tools:   make(map[string]tools.Tool),
		log:     logging.Default().WithComponent("dcp"),
	}
	for _, opt := range opts {
		opt(c)
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
