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

// SyncKnowledge performs a full project re-index
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
	
	// If intent routing fails or confidence is too low, fallback to general chat
	if err != nil || intent == nil || intent.Confidence < 0.2 {
		if chatTool, ok := c.tools["general_chat"]; ok {
			log.Printf("[DCP] Routing failed or low confidence, falling back to general_chat")
			return chatTool.Execute(ctx, map[string]interface{}{"query": query})
		}
		
		if err != nil {
			return nil, fmt.Errorf("intent routing failed: %w", err)
		}
		return nil, fmt.Errorf("intent routing failed: low confidence and no chat fallback")
	}

	// 2. Sanitize model outputs
	c.sanitizeArgs(intent.Args)

	// 3. Fetch Tool
	tool, ok := c.tools[intent.ToolName]
	if !ok {
		// Even if tool is not found, try falling back to chat
		if chatTool, ok := c.tools["general_chat"]; ok {
			return chatTool.Execute(ctx, map[string]interface{}{"query": query})
		}
		return nil, fmt.Errorf("tool %q selected by router but not registered in DCP", intent.ToolName)
	}

	// 4. Deterministic Execution
	log.Printf("[DCP] Executing tool %q with confidence %.2f", intent.ToolName, intent.Confidence)
	return tool.Execute(ctx, intent.Args)
}

// sanitizeArgs recursively cleans all string values in the arguments map,
// scrubbing PII and masking infrastructure details before any tool execution.
func (c *Coordinator) sanitizeArgs(args map[string]interface{}) {
	for k, v := range args {
		switch val := v.(type) {
		case string:
			// 1. Basic sanitization (printable chars)
			sanitized := util.SanitizeInput(val)
			// 2. Scrub PII (GDPR compliance)
			scrubbed := util.ScrubPII(sanitized)
			// 3. Mask Infrastructure (IPs)
			args[k] = util.MaskInfrastructure(scrubbed)
		case map[string]interface{}:
			c.sanitizeArgs(val)
		case []interface{}:
			for i, item := range val {
				if s, ok := item.(string); ok {
					sanitized := util.SanitizeInput(s)
					scrubbed := util.ScrubPII(sanitized)
					val[i] = util.MaskInfrastructure(scrubbed)
				}
			}
		}
	}
}
