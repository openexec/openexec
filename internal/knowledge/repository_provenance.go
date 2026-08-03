package knowledge

import (
	"context"
	"os/exec"
	"strings"
)

func worktreeProjection(ctx context.Context, root, stateHash string) WorktreeProjection {
	projection := WorktreeProjection{State: "unversioned", StateHash: stateHash}
	inside := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--is-inside-work-tree")
	if output, err := inside.Output(); err != nil || strings.TrimSpace(string(output)) != "true" {
		return projection
	}
	status := exec.CommandContext(ctx, "git", "-C", root, "status", "--porcelain=v1", "-z", "--untracked-files=normal")
	output, err := status.Output()
	if err != nil {
		projection.State = "unknown"
		return projection
	}
	for _, record := range strings.Split(string(output), "\x00") {
		if len(record) >= 3 && record[2] == ' ' {
			projection.DirtyCount++
		}
	}
	projection.State = "clean"
	if projection.DirtyCount > 0 {
		projection.State = "dirty"
	}
	return projection
}
