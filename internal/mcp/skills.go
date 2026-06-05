package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/openexec/openexec/internal/skills"
)

// skill_propose lets an agent capture a durable, project-specific lesson as a
// CANDIDATE skill. Candidates are written to .openexec/skills/_candidates/
// and are never loaded into the skill registry until a human runs
// `openexec skills approve <name>` — the propose-then-approve loop that
// prevents an agent from poisoning future runs with a wrong lesson.

// SkillProposeToolDef returns the MCP tool definition for skill_propose.
func SkillProposeToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name": "skill_propose",
		"description": "Propose a project-specific skill capturing a durable lesson learned during this session (a framework quirk, a recurring fix, a project convention). The proposal is written as a CANDIDATE for human review — it does NOT become active until a human approves it with `openexec skills approve <name>`. Propose a skill when the lesson would apply to future sessions, not for one-time fixes.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Kebab-case skill name, e.g. \"vitest-jsdom-quirks\". Lowercase letters, digits, and hyphens only.",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "One-line summary of what the skill teaches, used for routing. Be specific: \"Workarounds for JSDOM layout-event gaps in this repo's Vitest suite\", not \"testing tips\".",
				},
				"when_to_use": map[string]interface{}{
					"type":        "string",
					"description": "Natural-language trigger describing when future sessions should load this skill, e.g. \"When writing or debugging Vitest tests that involve mouse events or layout\".",
				},
				"tags": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Technology/topic tags for discovery, e.g. [\"vitest\", \"jsdom\", \"react\"].",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "The skill body in markdown: the rule, why it exists (root cause), and a concrete example. Write for a future agent with no memory of this session.",
				},
			},
			"required": []string{"name", "description", "content"},
		},
	}
}

type skillProposeArgs struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	WhenToUse   string   `json:"when_to_use"`
	Tags        []string `json:"tags"`
	Content     string   `json:"content"`
}

func (s *Server) handleSkillPropose(req Request, params toolsCallParams) {
	var args skillProposeArgs
	if err := json.Unmarshal(params.Arguments, &args); err != nil {
		s.writeError(req.ID, -32602, fmt.Sprintf("invalid skill_propose arguments: %v", err))
		return
	}
	if len(s.workspaceRoots) == 0 || s.workspaceRoots[0] == "" {
		s.writeToolError(req.ID, "no workspace root configured")
		return
	}

	path, err := skills.ProposeCandidate(s.workspaceRoots[0], args.Name, args.Description, args.WhenToUse, args.Tags, args.Content)
	if err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("skill proposal rejected: %v", err))
		return
	}

	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": fmt.Sprintf("Skill candidate %q written to %s. It is NOT active: a human must review and run `openexec skills approve %s` to activate it.", args.Name, path, args.Name),
			},
		},
		"skill_name":     args.Name,
		"candidate_path": path,
		"active":         false,
	})
}
