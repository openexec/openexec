package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openexec/openexec/pkg/infracontract"
)

// Infra suggestion tools (Phase 4 of the SRE roadmap): typed routing
// targets that let the DCP intent router (heuristic selector, or a local
// BitNet model when one is configured) classify natural-language SRE
// prompts — "bounce the nginx servers on staging" — into structured infra
// tool calls.
//
// SECURITY BOUNDARY: these adapters are suggestion-only. Execute always
// refuses. Router output is an UNTRUSTED PROPOSAL by construction: the
// only execution path for infra commands is the MCP server, where the
// broker, the deny-by-default allowlist registry, and the approval gate
// validate every parameter. A misrouted or hallucinated suggestion is a
// wrong suggestion, never a wrong command.

// InfraSuggestTool implements Tool for routing purposes only.
type InfraSuggestTool struct {
	name        string
	description string
	schema      string
}

func (t *InfraSuggestTool) Name() string        { return t.name }
func (t *InfraSuggestTool) Description() string { return t.description }
func (t *InfraSuggestTool) InputSchema() string { return t.schema }

// Execute refuses: the DCP plane proposes, the MCP plane disposes.
func (t *InfraSuggestTool) Execute(_ context.Context, _ map[string]interface{}) (any, error) {
	return nil, fmt.Errorf("%s is suggestion-only in the DCP routing plane: infra commands execute exclusively through the MCP server (openexec mcp-serve), which enforces the deny-by-default allowlist and the approval gate", t.name)
}

func suggestSchema(props map[string]interface{}, required []string) string {
	b, _ := json.Marshal(map[string]interface{}{
		"type":       "object",
		"properties": props,
		"required":   required,
	})
	return string(b)
}

func suggestEnum(desc string, values []string) map[string]interface{} {
	p := map[string]interface{}{"type": "string", "description": desc}
	if len(values) > 0 {
		p["enum"] = values
	}
	return p
}

// NewInfraSuggestTools builds one suggestion tool per configured engine.
// Descriptions deliberately carry the operational verbs operators use
// (deploy, restart, bounce, check, provision) — the heuristic selector
// scores word overlap and action verbs against the user's prompt.
func NewInfraSuggestTools(reg infracontract.Registry) []Tool {
	envs := reg.Environments()
	var out []Tool
	if reg.HasEngine("ansible") {
		out = append(out, &InfraSuggestTool{
			name:        "ansible_run_playbook",
			description: "Deploy, restart, bounce, or roll out services by running a pre-approved Ansible playbook against an environment (staging, production). Use for deployment and service-restart requests.",
			schema: suggestSchema(map[string]interface{}{
				"environment": suggestEnum("Target environment.", envs),
				"playbook":    suggestEnum("Allowlisted playbook to run.", reg.Playbooks()),
				"limit":       map[string]interface{}{"type": "string", "description": "Optional host pattern to limit the run."},
			}, []string{"environment", "playbook"}),
		})
	}
	if reg.HasEngine("salt") {
		out = append(out, &InfraSuggestTool{
			name:        "salt_apply_state",
			description: "Configure, install, or deploy software on managed nodes by applying a pre-approved Salt state to an allowlisted target. Use for configuration-management requests.",
			schema: suggestSchema(map[string]interface{}{
				"environment": suggestEnum("Target environment.", envs),
				"state":       suggestEnum("Allowlisted Salt state.", reg.States()),
				"target":      suggestEnum("Allowlisted Salt target.", reg.Targets()),
			}, []string{"environment", "state", "target"}),
		})
	}
	if reg.HasEngine("ssh") {
		out = append(out, &InfraSuggestTool{
			name:        "ssh_run_query",
			description: "Check, inspect, or get the status, health, disk usage, or service state of a host via a read-only diagnostic query over SSH. Use for monitoring and status questions.",
			schema: suggestSchema(map[string]interface{}{
				"environment": suggestEnum("Target environment.", envs),
				"host":        map[string]interface{}{"type": "string", "description": "Host to query (must match the allowlist patterns)."},
				"query":       suggestEnum("Allowlisted diagnostic query.", reg.Queries()),
			}, []string{"environment", "host", "query"}),
		})
	}
	if reg.HasEngine("terraform") {
		out = append(out, &InfraSuggestTool{
			name:        "terraform_plan",
			description: "Preview, show, or plan pending infrastructure changes (provision, scale, resize) with terraform plan for an environment. Use before any infrastructure change.",
			schema: suggestSchema(map[string]interface{}{
				"environment": suggestEnum("Target environment.", envs),
			}, []string{"environment"}),
		}, &InfraSuggestTool{
			name:        "terraform_apply",
			description: "Apply, execute, or roll out the previously saved and reviewed terraform plan for an environment. Requires a saved plan and human approval.",
			schema: suggestSchema(map[string]interface{}{
				"environment": suggestEnum("Target environment.", envs),
			}, []string{"environment"}),
		})
	}
	return out
}
