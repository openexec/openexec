package dcp

import (
	"context"
	"fmt"
	"log"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/router"
	"github.com/openexec/openexec/internal/tools"
	"github.com/openexec/openexec/pkg/util"
)

// Coordinator orchestrates the Deterministic Control Plane
type Coordinator struct {
	router  router.Router
	store   *knowledge.Store
	indexer *knowledge.Indexer
	tools   map[string]tools.Tool
}

func NewCoordinator(r router.Router, s *knowledge.Store) *Coordinator {
	c := &Coordinator{
		router:  r,
		store:   s,
		indexer: knowledge.NewIndexer(s),
		tools:   make(map[string]tools.Tool),
	}
	return c
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
	// 1. Local Intent Routing (BitNet)
	intent, err := c.router.ParseIntent(ctx, query)
	if err != nil {
		if result, ok := c.fallbackToChat(ctx, query, "Routing failed: %v", err); ok {
			return result, nil
		}
		return nil, fmt.Errorf("intent routing failed: %w", err)
	}

	// 2. Threshold check: if confidence is too low, fallback to general chat.
	// Below the router's low-confidence threshold, the local model is likely guessing.
	if intent.Confidence < router.LowConfidenceThreshold {
		if result, ok := c.fallbackToChat(ctx, query, "Low confidence (%.2f)", intent.Confidence); ok {
			return result, nil
		}
		// Fallback failed (general_chat not registered or errored), proceed with original tool execution
	}

	// 3. Sanitize model outputs
	c.sanitizeArgs(intent.Args)

	// 4. Fetch Tool
	tool, ok := c.tools[intent.ToolName]
	if !ok {
		if result, ok := c.fallbackToChat(ctx, query, "Tool %q not found", intent.ToolName); ok {
			return result, nil
		}
		return nil, fmt.Errorf("tool %q selected by router but not registered in DCP", intent.ToolName)
	}

	// 5. Deterministic Execution
	log.Printf("[DCP] Executing tool %q with confidence %.2f", intent.ToolName, intent.Confidence)
	return tool.Execute(ctx, intent.Args)
}

// fallbackToChat attempts to execute the general_chat tool as a fallback.
// Returns (result, true) if fallback succeeded, (nil, false) if no fallback available.
func (c *Coordinator) fallbackToChat(ctx context.Context, query, reason string, args ...interface{}) (any, bool) {
	chatTool, ok := c.tools["general_chat"]
	if !ok {
		return nil, false
	}
	log.Printf("[DCP] %s, falling back to general_chat", fmt.Sprintf(reason, args...))
	result, err := chatTool.Execute(ctx, map[string]interface{}{"query": query})
	if err != nil {
		log.Printf("[DCP] Fallback to general_chat failed: %v", err)
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
func (c *Coordinator) sanitizeArgs(args map[string]interface{}) {
	for k, v := range args {
		switch val := v.(type) {
		case string:
			args[k] = sanitizeString(val)
		case map[string]interface{}:
			c.sanitizeArgs(val)
		case []interface{}:
			for i, item := range val {
				if s, ok := item.(string); ok {
					val[i] = sanitizeString(s)
				}
			}
		}
	}
}
