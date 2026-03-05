package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/openexec/openexec/internal/knowledge"
)

// DeployTool executes deployments using deterministic environment records.
type DeployTool struct {
	store *knowledge.Store
}

func NewDeployTool(store *knowledge.Store) *DeployTool {
	return &DeployTool{store: store}
}

func (t *DeployTool) Name() string {
	return "deploy"
}

func (t *DeployTool) Description() string {
	return "Deploys the application to a specified environment using verified knowledge records."
}

func (t *DeployTool) InputSchema() string {
	return `{
		"type": "object",
		"properties": {
			"env": {
				"type": "string",
				"enum": ["dev", "staging", "prod"],
				"description": "Target environment"
			},
			"action": {
				"type": "string",
				"description": "Specific action (e.g. 'push', 'restart')"
			}
		},
		"required": ["env"]
	}`
}

func (t *DeployTool) Execute(ctx context.Context, args map[string]interface{}) (any, error) {
	env, _ := args["env"].(string)
	
	// Fetch deterministic instructions for this environment
	record, err := t.store.GetEnvironment(env)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch deployment: %w", err)
	}

	if record == nil {
		// Return a specific prompt asking for information to populate the knowledge base
		return fmt.Sprintf("KNOWLEDGE_MISSING: Deployment instructions for %q are not yet in the Deterministic Control Plane.\n\nPlease provide the topology (IPs and services), the runtime type (e.g. k8s, docker), and any required auth commands so I can record them for future surgical operations.", env), nil
	}

	// Logic to actually run the command would go here.
	result := fmt.Sprintf("🚀 Executing deployment to %s [%s runtime]\nAuth Steps: %s\nDeploy Steps: %s\nTopology: %s", 
		env, record.RuntimeType, record.AuthSteps, record.DeploySteps, record.Topology)
	
	if action, ok := args["action"].(string); ok && strings.Contains(action, "force") {
		result += "\n⚠️ Warning: Force operation requested."
	}

	return result, nil
}
