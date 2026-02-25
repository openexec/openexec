package release

import (
	"fmt"
)

// AutoLinkCommits scans commits and links them to tasks based on commit messages.
// Returns the number of links created.
func (m *Manager) AutoLinkCommits(since string) (int, error) {
	if !m.config.GitEnabled || m.gitTracker == nil {
		return 0, fmt.Errorf("git integration is not enabled")
	}

	linked, err := m.gitTracker.AutoLinkCommitsFromMessages(since)
	if err != nil {
		return 0, err
	}

	// Sync links from tracker to tasks
	state := m.gitTracker.GetState()
	for taskID, commits := range state.TaskCommits {
		m.mu.Lock()
		task, ok := m.tasks[taskID]
		if ok {
			if task.Git == nil {
				task.Git = &TaskGitInfo{Commits: []string{}}
			}
			// Add any commits not already in task
			for _, hash := range commits {
				found := false
				for _, existing := range task.Git.Commits {
					if existing == hash {
						found = true
						break
					}
				}
				if !found {
					task.Git.Commits = append(task.Git.Commits, hash)
				}
			}
		}
		m.mu.Unlock()
	}

	if err := m.Save(); err != nil {
		return linked, err
	}

	return linked, nil
}

// SetTaskPR sets pull request metadata for a task.
func (m *Manager) SetTaskPR(taskID string, prNumber int, prURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}

	if task.Git == nil {
		task.Git = &TaskGitInfo{Commits: []string{}}
	}

	if prNumber > 0 {
		task.Git.PRNumber = &prNumber
	}
	if prURL != "" {
		task.Git.PRUrl = prURL
	}

	return m.saveUnlocked()
}

// PushTag pushes a tag to the remote repository.
func (m *Manager) PushTag(tag string) error {
	if !m.config.GitEnabled || m.gitClient == nil {
		return fmt.Errorf("git integration is not enabled")
	}

	return m.gitClient.PushTag(tag)
}

// PushBranch pushes a branch to the remote repository.
func (m *Manager) PushBranch(branch string) error {
	if !m.config.GitEnabled || m.gitClient == nil {
		return fmt.Errorf("git integration is not enabled")
	}

	return m.gitClient.PushBranch(branch)
}

// GetGitIdentity returns the configured git user name and email.
func (m *Manager) GetGitIdentity() (name, email string) {
	if m.gitClient == nil || !m.gitClient.IsEnabled() {
		return "", ""
	}
	return m.gitClient.GetUserIdentity()
}

// ValidateBaseBranch checks if the base branch exists locally,
// optionally fetching from origin if not present.
func (m *Manager) ValidateBaseBranch(baseBranch string, fetchIfMissing bool) error {
	if !m.config.GitEnabled || m.gitClient == nil {
		return nil // No validation needed if git disabled
	}

	if m.gitClient.BranchExists(baseBranch) {
		return nil
	}

	// Try remote branch
	remoteBranch := "origin/" + baseBranch
	if m.gitClient.BranchExists(remoteBranch) {
		return nil
	}

	if fetchIfMissing {
		if err := m.gitClient.FetchBranch(baseBranch); err != nil {
			return fmt.Errorf("base branch '%s' not found locally or on origin: %w", baseBranch, err)
		}
		return nil
	}

	return fmt.Errorf("base branch '%s' not found; run 'git fetch origin %s' or create it first", baseBranch, baseBranch)
}
