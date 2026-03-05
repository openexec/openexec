package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/policy"
)

// SafeCommitTool acts as a mandatory gate before git operations
type SafeCommitTool struct {
	policy *policy.Engine
	syncer knowledge.Syncer
}

func NewSafeCommitTool(p *policy.Engine, s knowledge.Syncer) *SafeCommitTool {
	return &SafeCommitTool{policy: p, syncer: s}
}

func (t *SafeCommitTool) Name() string {
	return "safe_commit"
}

func (t *SafeCommitTool) Description() string {
	return "Surgically validates code quality (lint, type-check) and commits changes to git only if all gates pass."
}

func (t *SafeCommitTool) InputSchema() string {
	return `{
		"type": "object",
		"properties": {
			"message": { "type": "string", "description": "Git commit message" },
			"push": { "type": "boolean", "default": false, "description": "Whether to push after commit" }
		},
		"required": ["message"]
	}`
}

func (t *SafeCommitTool) Execute(ctx context.Context, args map[string]interface{}) (any, error) {
	message, _ := args["message"].(string)
	push, _ := args["push"].(bool)

	// 1. Run Mandatory Quality Gates (DCP Enforcement)
	passed, reason := t.policy.ValidateCompliance(ctx, ".")
	if !passed {
		return nil, fmt.Errorf("ABORTING COMMIT: Compliance gates failed.\n%s\n\nPlease fix these issues before trying to save again.", reason)
	}

	// 2. Execute Commit
	cmd := exec.CommandContext(ctx, "git", "add", ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git add failed: %w\n%s", err, string(out))
	}

	cmd = exec.CommandContext(ctx, "git", "commit", "-m", message)
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(out), "nothing to commit") {
			return "Nothing to commit, working tree clean.", nil
		}
		return nil, fmt.Errorf("git commit failed: %w\n%s", err, string(out))
	}

	result := fmt.Sprintf("✓ Successfully validated and committed changes: %q", message)

	// 4. Autonomous Sync: Re-index modified files to handle Line Drift
	if t.syncer != nil {
		// Get list of files in the commit
		cmd = exec.CommandContext(ctx, "git", "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
		if out, err := cmd.Output(); err == nil {
			files := strings.Split(strings.TrimSpace(string(out)), "\n")
			for _, f := range files {
				if strings.HasSuffix(f, ".go") {
					_ = t.syncer.SyncFile(f)
				}
			}
			result += fmt.Sprintf("\n✓ Knowledge Base synchronized for %d files.", len(files))
		}
	}

	// 5. Optional Push
	if push {
		cmd = exec.CommandContext(ctx, "git", "push")
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("git push failed: %w\n%s", err, string(out))
		}
		result += "\n✓ Changes pushed to remote."
	}

	return result, nil
}
