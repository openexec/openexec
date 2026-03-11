package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/policy"
	"github.com/openexec/openexec/internal/project"
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
	return "Surgically validates code quality and commits changes to a LOCAL story branch (branching off a local release branch). Note: This tool NEVER pushes to remote."
}

func (t *SafeCommitTool) InputSchema() string {
	return `{
		"type": "object",
		"properties": {
			"message": { "type": "string", "description": "Git commit message" },
			"story_id": { "type": "string", "description": "The current story ID (e.g. US-001)" },
			"task_id": { "type": "string", "description": "The current task ID (e.g. T-US-001-001)" }
		},
		"required": ["message", "story_id"]
	}`
}

func (t *SafeCommitTool) Execute(ctx context.Context, args map[string]interface{}) (any, error) {
	// 0. Initial validation
	message, _ := args["message"].(string)
	storyID, _ := args["story_id"].(string)
	
	projCfg, err := project.LoadProjectConfig(".")
	if err != nil {
		return nil, fmt.Errorf("failed to load project config: %w", err)
	}

	if !projCfg.GitCommitEnabled {
		return nil, fmt.Errorf("ABORTING COMMIT: Autonomous git committing is disabled. Set 'git_commit_enabled: true' in openexec.yaml to enable.")
	}

	// 1. Resolve Branch Names
	relPrefix := projCfg.ReleaseBranchPrefix
	if relPrefix == "" { relPrefix = "release/" }
	featPrefix := projCfg.FeatureBranchPrefix
	if featPrefix == "" { featPrefix = "feature/" }
	
	baseBranch := projCfg.BaseBranch
	if baseBranch == "" { baseBranch = "main" }

	// Resolve release branch name (e.g. release/v0.2.8)
	releaseBranch := relPrefix + "current" // Default to 'current' or we could use project version
	fromVersion, _ := exec.Command("openexec", "version", "--short").Output()
	if v := strings.TrimSpace(string(fromVersion)); v != "" {
		releaseBranch = relPrefix + v
	}

	storyBranch := featPrefix + storyID

	// 2. Run Mandatory Quality Gates (DCP Enforcement)
	passed, reason := t.policy.ValidateCompliance(ctx, ".")
	if !passed {
		return nil, fmt.Errorf("ABORTING COMMIT: Compliance gates failed.\n%s", reason)
	}

	// 3. Prepare Branches (Local Only)
	
	// Ensure Release Branch exists (branching off baseBranch)
	// We don't error if it exists, just ensure it's there.
	_ = exec.CommandContext(ctx, "git", "branch", releaseBranch, baseBranch).Run()

	// Ensure Story Branch exists (branching off releaseBranch)
	// If it doesn't exist, create it from the release branch.
	if err := exec.CommandContext(ctx, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+storyBranch).Run(); err != nil {
		if err := exec.CommandContext(ctx, "git", "checkout", "-b", storyBranch, releaseBranch).Run(); err != nil {
			return nil, fmt.Errorf("failed to create story branch %s from %s: %w", storyBranch, releaseBranch, err)
		}
	} else {
		// Just switch to it
		if err := exec.CommandContext(ctx, "git", "checkout", storyBranch).Run(); err != nil {
			return nil, fmt.Errorf("failed to checkout story branch %s: %w", storyBranch, err)
		}
	}

	// 4. Execute Commit
	exec.CommandContext(ctx, "git", "add", ".").Run()
	cmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
	if out, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(out), "nothing to commit") {
			return "Nothing to commit, working tree clean.", nil
		}
		return nil, fmt.Errorf("git commit failed: %w\n%s", err, string(out))
	}

	result := fmt.Sprintf("✓ Committed to LOCAL branch %s (parent: %s): %q", storyBranch, releaseBranch, message)

	// 5. Autonomous Sync: Re-index modified files
	if t.syncer != nil {
		cmd = exec.CommandContext(ctx, "git", "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
		if out, err := cmd.Output(); err == nil {
			files := strings.Split(strings.TrimSpace(string(out)), "\n")
			for _, f := range files {
				if strings.HasSuffix(f, ".go") || strings.HasSuffix(f, ".ts") || strings.HasSuffix(f, ".tsx") {
					_ = t.syncer.SyncFile(f)
				}
			}
			result += fmt.Sprintf("\n✓ Knowledge Base synchronized for %d files.", len(files))
		}
	}

	return result, nil
}
