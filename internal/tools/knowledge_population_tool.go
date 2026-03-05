package tools

import (
	"context"
	"fmt"

	"github.com/openexec/openexec/internal/knowledge"
)

// KnowledgePopulationTool allows the LLM to record new deterministic knowledge
type KnowledgePopulationTool struct {
	store *knowledge.Store
}

func NewKnowledgePopulationTool(store *knowledge.Store) *KnowledgePopulationTool {
	return &KnowledgePopulationTool{store: store}
}

func (t *KnowledgePopulationTool) Name() string {
	return "populate_knowledge"
}

func (t *KnowledgePopulationTool) Description() string {
	return "Records new deterministic knowledge (deployment details, API contracts) discovered during the session."
}

func (t *KnowledgePopulationTool) InputSchema() string {
	return `{
		"type": "object",
		"properties": {
			"type": {
				"type": "string",
				"enum": ["environment", "api_doc"]
			},
			"env": { "type": "string", "description": "Target environment (e.g. prod, dev, local)" },
			"runtime_type": { "type": "string", "description": "e.g. k8s, docker-compose, vm" },
			"auth_steps": { "type": "string", "description": "JSON stringified array of pre-flight commands" },
			"deploy_steps": { "type": "string", "description": "JSON stringified array of deployment commands" },
			"topology": { "type": "string", "description": "JSON stringified array of {ip, services[], role} objects" },
			"path": { "type": "string", "description": "For api_doc type" },
			"method": { "type": "string" },
			"description": { "type": "string" }
		},
		"required": ["type"]
	}`
}

func (t *KnowledgePopulationTool) Execute(ctx context.Context, args map[string]interface{}) (any, error) {
	kType, _ := args["type"].(string)

	switch kType {
	case "environment":
		env, _ := args["env"].(string)
		if env == "" {
			return nil, fmt.Errorf("missing environment name (env)")
		}
		
		runtimeType, _ := args["runtime_type"].(string)
		authSteps, _ := args["auth_steps"].(string)
		deploySteps, _ := args["deploy_steps"].(string)
		topology, _ := args["topology"].(string)

		err := t.store.SetEnvironment(&knowledge.EnvironmentRecord{
			Env:         env,
			RuntimeType: runtimeType,
			AuthSteps:   authSteps,
			DeploySteps: deploySteps,
			Topology:    topology,
		})
		if err != nil { return nil, err }
		return fmt.Sprintf("Successfully recorded complex environment topology for %q", env), nil

	case "api_doc":
		path, _ := args["path"].(string)
		method, _ := args["method"].(string)
		desc, _ := args["description"].(string)
		if path == "" || method == "" {
			return nil, fmt.Errorf("missing api_doc fields (path, method)")
		}
		err := t.store.SetAPIDoc(&knowledge.APIDocRecord{
			Path:        path,
			Method:      method,
			Description: desc,
		})
		if err != nil { return nil, err }
		return fmt.Sprintf("Successfully recorded API contract for %s %s", method, path), nil

	default:
		return nil, fmt.Errorf("unsupported knowledge type: %s", kType)
	}
}
