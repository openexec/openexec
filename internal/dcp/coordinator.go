package dcp

import (
	"context"
	"fmt"
	"log"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/router"
	"github.com/openexec/openexec/internal/tools"
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

// ProcessQuery uses BitNet to parse intent and execute the correct surgical tool
func (c *Coordinator) ProcessQuery(ctx context.Context, query string) (any, error) {
	// 1. Local Intent Routing (BitNet)
	intent, err := c.router.ParseIntent(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("intent routing failed: %w", err)
	}

	// 2. Fetch Tool
	tool, ok := c.tools[intent.ToolName]
	if !ok {
		return nil, fmt.Errorf("tool %q selected by router but not registered in DCP", intent.ToolName)
	}

	// 3. Deterministic Execution
	log.Printf("[DCP] Executing tool %q with confidence %.2f", intent.ToolName, intent.Confidence)
	return tool.Execute(ctx, intent.Args)
}

// SyncKnowledge triggers the automatic indexing of source code
func (c *Coordinator) SyncKnowledge(projectDir string) error {
	log.Printf("[DCP] Synchronizing knowledge for %s...", projectDir)
	return c.indexer.IndexProject(projectDir)
}
