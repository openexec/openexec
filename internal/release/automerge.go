package release

import (
	"fmt"
	"time"

	"github.com/openexec/openexec/internal/git"
)

// TaskCompletionResult contains information about what happened when a task completed.
type TaskCompletionResult struct {
	TaskID              string `json:"task_id"`
	TaskCompleted       bool   `json:"task_completed"`
	StoryID             string `json:"story_id,omitempty"`
	StoryComplete       bool   `json:"story_complete"`
	StoryMerged         bool   `json:"story_merged"`
	AwaitingApproval    bool   `json:"awaiting_approval"`
	ReleaseComplete     bool   `json:"release_complete"`
	ReleaseTagged       bool   `json:"release_tagged"`
	ReleaseMergedToMain bool   `json:"release_merged_to_main"`
	Message             string `json:"message,omitempty"`
	Error               string `json:"error,omitempty"`
}

// PendingApprovals contains items awaiting approval.
type PendingApprovals struct {
	Tasks   []*Task  `json:"tasks"`
	Stories []*Story `json:"stories"`
	Release *Release `json:"release,omitempty"`
}

// ProcessResult contains results of processing approved items.
type ProcessResult struct {
	StoriesMerged       []string `json:"stories_merged"`
	ReleaseComplete     bool     `json:"release_complete"`
	ReleaseTagged       bool     `json:"release_tagged"`
	ReleaseMergedToMain bool     `json:"release_merged_to_main"`
	Errors              []string `json:"errors,omitempty"`
	// Dry-run fields
	DryRun          bool     `json:"dry_run,omitempty"`
	WouldMerge      []string `json:"would_merge,omitempty"`
	WouldTag        bool     `json:"would_tag,omitempty"`
	WouldMergeToMain bool    `json:"would_merge_to_main,omitempty"`
}

// CompleteTask marks a task as done and triggers auto-merge if configured.
// Returns information about what actions were taken.
func (m *Manager) CompleteTask(taskID string) (*TaskCompletionResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task %s not found", taskID)
	}

	result := &TaskCompletionResult{
		TaskID: taskID,
	}

	// Mark task as done
	now := time.Now()
	task.Status = TaskStatusDone
	task.CompletedAt = &now
	result.TaskCompleted = true

	// Check if we should trigger auto-merge for the story
	if m.config.AutoMergeStories {
		storyComplete, storyID := m.isStoryCompleteUnlocked(task.StoryID)
		if storyComplete {
			result.StoryID = storyID
			result.StoryComplete = true

			// Check approval requirements
			if m.config.ApprovalEnabled {
				story := m.stories[storyID]
				if story.Approval == nil || story.Approval.Status != ApprovalApproved {
					result.AwaitingApproval = true
					result.Message = fmt.Sprintf("Story %s complete but awaiting approval", storyID)
					if err := m.saveUnlocked(); err != nil {
						return nil, err
					}
					return result, nil
				}
			}

			// Auto-merge story to release
			if err := m.mergeStoryUnlocked(storyID); err != nil {
				result.Error = err.Error()
			} else {
				result.StoryMerged = true
				result.Message = fmt.Sprintf("Story %s auto-merged to release", storyID)

				// Check if release is now complete
				if m.isReleaseCompleteUnlocked() {
					result.ReleaseComplete = true

					// Check release approval if required
					releaseApproved := !m.config.ApprovalEnabled ||
						(m.release.Approval != nil && m.release.Approval.Status == ApprovalApproved)

					if !releaseApproved {
						result.Message = fmt.Sprintf("Release %s complete but awaiting approval", m.release.Version)
					} else {
						// Auto-tag if configured
						if m.config.AutoTagRelease && m.config.GitEnabled {
							msg := fmt.Sprintf("Release %s", m.release.Version)
							if err := m.tagReleaseUnlocked(msg); err != nil {
								result.Error = err.Error()
							} else {
								result.ReleaseTagged = true
							}
						}

						// Auto-merge to main if configured
						if m.config.AutoMergeToMain && m.config.GitEnabled {
							if err := m.mergeReleaseToMainUnlocked(); err != nil {
								result.Error = err.Error()
							} else {
								result.ReleaseMergedToMain = true
							}
						}

						result.Message = fmt.Sprintf("Release %s complete", m.release.Version)
					}
				}
			}
		}
	}

	if err := m.saveUnlocked(); err != nil {
		return nil, err
	}

	return result, nil
}

// IsStoryComplete checks if all tasks for a story are done.
func (m *Manager) IsStoryComplete(storyID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	complete, _ := m.isStoryCompleteUnlocked(storyID)
	return complete
}

func (m *Manager) isStoryCompleteUnlocked(storyID string) (bool, string) {
	story, ok := m.stories[storyID]
	if !ok {
		return false, ""
	}

	// If story has no tasks, it's not complete
	if len(story.Tasks) == 0 {
		return false, storyID
	}

	// Check all tasks
	for _, taskID := range story.Tasks {
		task, ok := m.tasks[taskID]
		if !ok {
			return false, storyID
		}
		if task.Status != TaskStatusDone && task.Status != TaskStatusApproved {
			return false, storyID
		}
	}

	return true, storyID
}

// IsReleaseComplete checks if all stories for the release are done.
func (m *Manager) IsReleaseComplete() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isReleaseCompleteUnlocked()
}

func (m *Manager) isReleaseCompleteUnlocked() bool {
	if m.release == nil {
		return false
	}

	if len(m.release.Stories) == 0 {
		return false
	}

	for _, storyID := range m.release.Stories {
		story, ok := m.stories[storyID]
		if !ok {
			return false
		}
		if story.Status != StoryStatusDone {
			return false
		}
	}

	return true
}

// mergeStoryUnlocked merges a story to release (caller must hold lock).
func (m *Manager) mergeStoryUnlocked(storyID string) error {
	story, ok := m.stories[storyID]
	if !ok {
		return fmt.Errorf("story %s not found", storyID)
	}

	if m.release == nil {
		return fmt.Errorf("no release defined")
	}

	// When approval is enabled, verify all tasks are approved/done
	if m.config.ApprovalEnabled {
		for _, taskID := range story.Tasks {
			task, ok := m.tasks[taskID]
			if !ok {
				return fmt.Errorf("task %s not found", taskID)
			}
			// Task must be done or approved
			if task.Status != TaskStatusDone && task.Status != TaskStatusApproved {
				return fmt.Errorf("task %s is not complete (status: %s)", taskID, task.Status)
			}
			// If task needs review, it must be approved
			if task.NeedsReview && (task.Approval == nil || task.Approval.Status != ApprovalApproved) {
				return fmt.Errorf("task %s requires approval", taskID)
			}
		}
	}

	// Merge git branch if enabled
	if m.config.GitEnabled && m.gitTracker != nil && story.Git != nil {
		info, err := m.gitTracker.MergeStoryToRelease(storyID, m.release.Version)
		if err != nil {
			if git.IsMergeConflict(err) {
				story.Status = "conflict" // Custom status for manual intervention
				return err
			}
			return fmt.Errorf("failed to merge story branch: %w", err)
		}

		if info != nil {
			story.Git.MergedTo = info.TargetBranch
			story.Git.MergeCommit = info.MergeCommit
			story.Git.MergedAt = &info.MergedAt
		}
	}

	story.Status = StoryStatusDone
	now := time.Now()
	story.CompletedAt = &now

	return nil
}

// tagReleaseUnlocked creates a tag (caller must hold lock).
func (m *Manager) tagReleaseUnlocked(message string) error {
	if m.release == nil {
		return fmt.Errorf("no release defined")
	}

	if m.gitTracker != nil {
		if err := m.gitTracker.TagRelease(m.release.Version, message); err != nil {
			return err
		}

		if m.release.Git != nil {
			m.release.Git.Tag = "v" + m.release.Version
		}
	}

	return nil
}

// mergeReleaseToMainUnlocked merges the release branch to main (caller must hold lock).
func (m *Manager) mergeReleaseToMainUnlocked() error {
	if m.release == nil || m.release.Git == nil {
		return fmt.Errorf("no release branch defined")
	}

	if m.gitClient == nil || !m.gitClient.IsEnabled() {
		return nil
	}

	if err := m.gitClient.Lock(); err != nil {
		return fmt.Errorf("failed to acquire git lock: %w", err)
	}
	defer m.gitClient.Unlock()

	// Checkout main branch
	if err := m.gitClient.Checkout(m.config.BaseBranch); err != nil {
		return fmt.Errorf("failed to checkout %s: %w", m.config.BaseBranch, err)
	}

	// Merge release branch with no-ff
	if err := m.gitClient.MergeBranch(m.release.Git.Branch, true); err != nil {
		return fmt.Errorf("failed to merge release to %s: %w", m.config.BaseBranch, err)
	}

	// Get merge commit
	mergeCommit, err := m.gitClient.LatestCommit()
	if err != nil {
		return err
	}

	m.release.Git.MergeCommit = mergeCommit
	m.release.Status = ReleaseStatusDeployed
	now := time.Now()
	m.release.CompletedAt = &now

	return nil
}

// GetPendingApprovals returns all items awaiting approval.
func (m *Manager) GetPendingApprovals() *PendingApprovals {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := &PendingApprovals{
		Tasks:   make([]*Task, 0),
		Stories: make([]*Story, 0),
	}

	if !m.config.ApprovalEnabled {
		return result
	}

	// Check tasks
	for _, task := range m.tasks {
		if task.Status == TaskStatusNeedsReview ||
			(task.Approval != nil && task.Approval.Status == ApprovalPending && task.Status == TaskStatusDone) {
			result.Tasks = append(result.Tasks, task)
		}
	}

	// Check stories - complete but not approved
	for _, story := range m.stories {
		complete, _ := m.isStoryCompleteUnlocked(story.ID)
		if complete && (story.Approval == nil || story.Approval.Status == ApprovalPending) {
			result.Stories = append(result.Stories, story)
		}
	}

	// Check release
	if m.release != nil && m.isReleaseCompleteUnlocked() {
		if m.release.Approval == nil || m.release.Approval.Status == ApprovalPending {
			result.Release = m.release
		}
	}

	return result
}

// ProcessApprovedStories merges any stories that are approved and complete.
// Call this after approving a story to trigger auto-merge.
func (m *Manager) ProcessApprovedStories() (*ProcessResult, error) {
	return m.ProcessApprovedStoriesWithOptions(false)
}

// ProcessApprovedStoriesWithOptions merges approved stories with optional dry-run mode.
func (m *Manager) ProcessApprovedStoriesWithOptions(dryRun bool) (*ProcessResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := &ProcessResult{
		StoriesMerged: make([]string, 0),
		WouldMerge:    make([]string, 0),
		DryRun:        dryRun,
	}

	for _, story := range m.stories {
		// Skip if already done
		if story.Status == StoryStatusDone {
			continue
		}

		// Check if complete
		complete, _ := m.isStoryCompleteUnlocked(story.ID)
		if !complete {
			continue
		}

		// Check approval
		if m.config.ApprovalEnabled {
			if story.Approval == nil || story.Approval.Status != ApprovalApproved {
				continue
			}
		}

		if dryRun {
			// Just record what would happen
			result.WouldMerge = append(result.WouldMerge, story.ID)
		} else {
			// Merge story
			if err := m.mergeStoryUnlocked(story.ID); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", story.ID, err))
			} else {
				result.StoriesMerged = append(result.StoriesMerged, story.ID)
			}
		}
	}

	// Check if release would be/is complete after merging
	wouldBeComplete := len(result.StoriesMerged) > 0 || len(result.WouldMerge) > 0
	if wouldBeComplete && m.wouldReleaseBeComplete(result.StoriesMerged, result.WouldMerge) {
		result.ReleaseComplete = true

		// Check release approval if required
		releaseApproved := !m.config.ApprovalEnabled ||
			(m.release.Approval != nil && m.release.Approval.Status == ApprovalApproved)

		if releaseApproved {
			if m.config.AutoTagRelease && m.config.GitEnabled {
				if dryRun {
					result.WouldTag = true
				} else {
					msg := fmt.Sprintf("Release %s", m.release.Version)
					if err := m.tagReleaseUnlocked(msg); err == nil {
						result.ReleaseTagged = true
					}
				}
			}

			if m.config.AutoMergeToMain && m.config.GitEnabled {
				if dryRun {
					result.WouldMergeToMain = true
				} else {
					if err := m.mergeReleaseToMainUnlocked(); err == nil {
						result.ReleaseMergedToMain = true
					}
				}
			}
		}
	}

	if !dryRun {
		if err := m.saveUnlocked(); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// wouldReleaseBeComplete checks if the release would be complete after merging given stories.
func (m *Manager) wouldReleaseBeComplete(merged, wouldMerge []string) bool {
	if m.release == nil || len(m.release.Stories) == 0 {
		return false
	}

	// Create a set of stories that are/would be done
	doneStories := make(map[string]bool)
	for _, id := range merged {
		doneStories[id] = true
	}
	for _, id := range wouldMerge {
		doneStories[id] = true
	}

	// Check all release stories
	for _, storyID := range m.release.Stories {
		story, ok := m.stories[storyID]
		if !ok {
			return false
		}
		// Story must be done or in the merge set
		if story.Status != StoryStatusDone && !doneStories[storyID] {
			return false
		}
	}

	return true
}
