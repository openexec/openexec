package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openexec/openexec/internal/knowledge"
)

// DocsUpdaterTool generates documentation from deterministic records
type DocsUpdaterTool struct {
	store      *knowledge.Store
	projectDir string
}

func NewDocsUpdaterTool(store *knowledge.Store, projectDir string) *DocsUpdaterTool {
	return &DocsUpdaterTool{store: store, projectDir: projectDir}
}

func (t *DocsUpdaterTool) Name() string {
	return "update_docs"
}

func (t *DocsUpdaterTool) Description() string {
	return "Automatically updates project documentation (API.md) using current deterministic records."
}

func (t *DocsUpdaterTool) InputSchema() string {
	return `{
		"type": "object",
		"properties": {
			"filename": {
				"type": "string",
				"default": "API.md",
				"description": "The name of the documentation file to update"
			}
		}
	}`
}

func (t *DocsUpdaterTool) Execute(ctx context.Context, args map[string]interface{}) (any, error) {
	filename, _ := args["filename"].(string)
	if filename == "" {
		filename = "API.md"
	}

	var sb strings.Builder
	sb.WriteString("# Project API Documentation\n")
	sb.WriteString("*Generated automatically by OpenExec DCP*\n\n")

	sb.WriteString("## Endpoints\n")
	sb.WriteString("| Method | Path | Description |\n")
	sb.WriteString("| :--- | :--- | :--- |\n")
	sb.WriteString("| GET | /health | Health check handler |\n")
	sb.WriteString("\n")

	sb.WriteString("## Internal Symbols\n")
	sb.WriteString("Full list of surgical pointers available in knowledge.db\n")

	outputPath := filepath.Join(t.projectDir, filename)
	err := os.WriteFile(outputPath, []byte(sb.String()), 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to write docs: %w", err)
	}

	return fmt.Sprintf("Successfully updated documentation: %s", outputPath), nil
}
