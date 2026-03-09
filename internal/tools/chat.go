package tools

import (
	"context"
	"fmt"
)

// GeneralChatTool handles basic conversational queries that don't match surgical tools
type GeneralChatTool struct{}

func NewGeneralChatTool() *GeneralChatTool {
	return &GeneralChatTool{}
}

func (t *GeneralChatTool) Name() string {
	return "general_chat"
}

func (t *GeneralChatTool) Description() string {
	return "Handles general questions, help requests, and conversational interaction."
}

func (t *GeneralChatTool) InputSchema() string {
	return `{"type": "object", "properties": {"query": {"type": "string"}}}`
}

func (t *GeneralChatTool) Execute(ctx context.Context, args map[string]interface{}) (any, error) {
	query, _ := args["query"].(string)
	
	switch query {
	case "help":
		return "I am OpenExec, your deterministic orchestration agent. You can ask me to:\n- list project files\n- show symbols in a file\n- deploy to production\n- safe commit your changes\n- run the initialization wizard", nil
	case "list", "list project files":
		return "To list files, you can use the standard terminal commands or ask me to 'index the project' to see a symbol-level map.", nil
	case "wizard":
		return "To start the requirement gathering wizard, run 'openexec wizard' in your terminal.", nil
	case "init":
		return "To initialize a new project, run 'openexec init' in your terminal.", nil
	default:
		return fmt.Sprintf("I received your query: %q. I'm currently in a limited local-only mode. For complex task execution, use the 'openexec run' command.", query), nil
	}
}
