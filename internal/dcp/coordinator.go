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

// Coordinator orchestrates the Deterministic Control Plane
type Coordinator struct {
	router  router.Router
	store   *knowledge.Store
	indexer *knowledge.Indexer
	tools   map[string]tools.Tool
	log     *logging.Logger
}

// CoordinatorOption allows optional configuration of the Coordinator
type CoordinatorOption func(*Coordinator)

// WithLogger sets a custom logger for the Coordinator
func WithLogger(l *logging.Logger) CoordinatorOption {
	return func(c *Coordinator) {
		c.log = l.WithComponent("dcp")
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

// ProcessQuery uses BitNet to parse intent and execute the correct surgical tool
func (c *Coordinator) ProcessQuery(ctx context.Context, query string) (any, error) {
	qHash := queryHash(query)

	// 1. Local Intent Routing (BitNet)
	intent, err := c.router.ParseIntent(ctx, query)
	if err != nil {
		if result, ok := c.fallbackToChatWithReason(ctx, query, qHash, FallbackReasonRouterError, "", 0, err); ok {
			return result, nil
		}
		return nil, fmt.Errorf("intent routing failed: %w", err)
	}

	// 2. Threshold check: if confidence is too low, fallback to general chat.
	// Below the router's low-confidence threshold, the local model is likely guessing.
	if intent.Confidence < router.LowConfidenceThreshold {
		if result, ok := c.fallbackToChatWithReason(ctx, query, qHash, FallbackReasonLowConfidence, intent.ToolName, intent.Confidence, nil); ok {
			return result, nil
		}
		// Fallback failed (general_chat not registered or errored), proceed with original tool execution
	}

	// 3. Sanitize model outputs
	sanitizeArgs(intent.Args)

	// 4. Fetch Tool
	tool, ok := c.tools[intent.ToolName]
	if !ok {
		if result, ok := c.fallbackToChatWithReason(ctx, query, qHash, FallbackReasonMissingTool, intent.ToolName, intent.Confidence, nil); ok {
			return result, nil
		}
		return nil, fmt.Errorf("tool %q selected by router but not registered in DCP", intent.ToolName)
	}

	// 5. Deterministic Execution
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
