// Package worktree provides git worktree management for parallel runs.
// It enables multiple independent runs to execute concurrently without
// conflicts by using git worktrees for isolation.
package worktree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Worktree represents an active git worktree.
type Worktree struct {
	Path       string    // Absolute path to the worktree
	Branch     string    // Branch name (can be same as run ID)
	RunID      string    // Associated run ID
	CreatedAt  time.Time
	SourcePath string    // Original repository path
}

// Manager handles git worktree lifecycle for parallel runs.
type Manager struct {
	baseDir string // Base directory for worktrees (e.g., .openexec/worktrees)
	repoDir string // Main repository directory

	worktrees map[string]*Worktree // runID -> Worktree
	mu        sync.RWMutex
}

// NewManager creates a new worktree manager.
func NewManager(repoDir, baseDir string) (*Manager, error) {
	if repoDir == "" {
		return nil, fmt.Errorf("repository directory is required")
	}

	// Default worktree base directory
	if baseDir == "" {
		baseDir = filepath.Join(repoDir, ".openexec", "worktrees")
	}

	// Ensure base directory exists
	if err := os.MkdirAll(baseDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create worktree base directory: %w", err)
	}

	return &Manager{
		baseDir:   baseDir,
		repoDir:   repoDir,
		worktrees: make(map[string]*Worktree),
	}, nil
}

// Create creates a new worktree for a run.
// The worktree is created with a branch named after the run ID.
func (m *Manager) Create(ctx context.Context, runID string) (*Worktree, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if worktree already exists for this run
	if wt, exists := m.worktrees[runID]; exists {
		return wt, nil
	}

	// Generate worktree path
	worktreePath := filepath.Join(m.baseDir, runID)
	branchName := fmt.Sprintf("openexec/%s", runID)

	// Create the worktree with a new branch
	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "-b", branchName, worktreePath)
	cmd.Dir = m.repoDir
	_, err := cmd.CombinedOutput()
	if err != nil {
		// Try without creating a new branch (branch might already exist)
		cmd = exec.CommandContext(ctx, "git", "worktree", "add", worktreePath, branchName)
		cmd.Dir = m.repoDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("failed to create worktree: %s: %w", string(output), err)
		}
	}

	wt := &Worktree{
		Path:       worktreePath,
		Branch:     branchName,
		RunID:      runID,
		CreatedAt:  time.Now(),
		SourcePath: m.repoDir,
	}

	m.worktrees[runID] = wt
	return wt, nil
}

// Get returns the worktree for a run, or nil if not found.
func (m *Manager) Get(runID string) *Worktree {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.worktrees[runID]
}

// Remove removes a worktree and optionally its branch.
func (m *Manager) Remove(ctx context.Context, runID string, deleteBranch bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	wt, exists := m.worktrees[runID]
	if !exists {
		return nil // Already removed
	}

	// Remove the worktree
	cmd := exec.CommandContext(ctx, "git", "worktree", "remove", wt.Path, "--force")
	cmd.Dir = m.repoDir
	if output, err := cmd.CombinedOutput(); err != nil {
		// Try manual removal if git command fails
		if rmErr := os.RemoveAll(wt.Path); rmErr != nil {
			return fmt.Errorf("failed to remove worktree: %s: %w", string(output), err)
		}
	}

	// Optionally delete the branch
	if deleteBranch {
		cmd = exec.CommandContext(ctx, "git", "branch", "-D", wt.Branch)
		cmd.Dir = m.repoDir
		_ = cmd.Run() // Ignore errors - branch might not exist
	}

	delete(m.worktrees, runID)
	return nil
}

// List returns all active worktrees.
func (m *Manager) List() []*Worktree {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Worktree, 0, len(m.worktrees))
	for _, wt := range m.worktrees {
		result = append(result, wt)
	}
	return result
}

// Prune removes worktrees older than maxAge and those without associated runs.
func (m *Manager) Prune(ctx context.Context, maxAge time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	var toRemove []string

	for runID, wt := range m.worktrees {
		if now.Sub(wt.CreatedAt) > maxAge {
			toRemove = append(toRemove, runID)
		}
	}

	for _, runID := range toRemove {
		wt := m.worktrees[runID]
		cmd := exec.CommandContext(ctx, "git", "worktree", "remove", wt.Path, "--force")
		cmd.Dir = m.repoDir
		_ = cmd.Run() // Best effort

		// Also try direct removal
		_ = os.RemoveAll(wt.Path)

		delete(m.worktrees, runID)
	}

	// Run git worktree prune to clean up stale entries
	cmd := exec.CommandContext(ctx, "git", "worktree", "prune")
	cmd.Dir = m.repoDir
	_ = cmd.Run()

	return nil
}

// MergeChanges merges changes from a worktree branch back to the source branch.
func (m *Manager) MergeChanges(ctx context.Context, runID, targetBranch string) error {
	wt := m.Get(runID)
	if wt == nil {
		return fmt.Errorf("worktree not found for run %s", runID)
	}

	// Switch to target branch in main repo
	cmd := exec.CommandContext(ctx, "git", "checkout", targetBranch)
	cmd.Dir = m.repoDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to checkout target branch: %s: %w", string(output), err)
	}

	// Merge the worktree branch
	cmd = exec.CommandContext(ctx, "git", "merge", wt.Branch, "--no-edit")
	cmd.Dir = m.repoDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to merge worktree branch: %s: %w", string(output), err)
	}

	return nil
}

// DiscoverExisting scans the worktree directory and populates the manager
// with existing worktrees. Useful for recovery after restart.
func (m *Manager) DiscoverExisting(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get list from git
	cmd := exec.CommandContext(ctx, "git", "worktree", "list", "--porcelain")
	cmd.Dir = m.repoDir
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	// Parse output
	lines := strings.Split(string(output), "\n")
	var currentPath, currentBranch string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if currentPath != "" && strings.HasPrefix(currentPath, m.baseDir) {
				// Extract run ID from path
				runID := filepath.Base(currentPath)
				if _, exists := m.worktrees[runID]; !exists {
					m.worktrees[runID] = &Worktree{
						Path:       currentPath,
						Branch:     currentBranch,
						RunID:      runID,
						CreatedAt:  time.Now(), // Unknown, use now
						SourcePath: m.repoDir,
					}
				}
			}
			currentPath = ""
			currentBranch = ""
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			currentPath = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") {
			currentBranch = strings.TrimPrefix(line, "branch refs/heads/")
		}
	}

	return nil
}

// Status returns the git status of a worktree.
func (m *Manager) Status(ctx context.Context, runID string) (string, error) {
	wt := m.Get(runID)
	if wt == nil {
		return "", fmt.Errorf("worktree not found for run %s", runID)
	}

	cmd := exec.CommandContext(ctx, "git", "status", "--short")
	cmd.Dir = wt.Path
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get status: %w", err)
	}

	return string(output), nil
}
