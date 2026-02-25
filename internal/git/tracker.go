package git

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// Tracker tracks relationships between git objects and project entities.
type Tracker struct {
	client    *Client
	storePath string
	state     *TrackerState
	mu        sync.RWMutex
}

// TrackerState holds the tracked relationships.
type TrackerState struct {
	// Task ID -> list of commit hashes
	TaskCommits map[string][]string `json:"task_commits"`

	// Story ID -> branch name
	StoryBranches map[string]string `json:"story_branches"`

	// Story ID -> merge info (when merged to release)
	StoryMerges map[string]*MergeInfo `json:"story_merges"`

	// Release version -> branch name
	ReleaseBranches map[string]string `json:"release_branches"`

	// Release version -> tag name
	ReleaseTags map[string]string `json:"release_tags"`

	// Commit hash -> task ID (reverse lookup)
	CommitTasks map[string]string `json:"commit_tasks"`

	LastUpdated time.Time `json:"last_updated"`
}

// MergeInfo holds information about a branch merge.
type MergeInfo struct {
	SourceBranch string    `json:"source_branch"`
	TargetBranch string    `json:"target_branch"`
	MergeCommit  string    `json:"merge_commit"`
	MergedAt     time.Time `json:"merged_at"`
	MergedBy     string    `json:"merged_by,omitempty"`
}

// NewTracker creates a new git tracker.
func NewTracker(client *Client, storePath string) (*Tracker, error) {
	t := &Tracker{
		client:    client,
		storePath: storePath,
		state:     newTrackerState(),
	}

	// Load existing state if present
	if err := t.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return t, nil
}

func newTrackerState() *TrackerState {
	return &TrackerState{
		TaskCommits:     make(map[string][]string),
		StoryBranches:   make(map[string]string),
		StoryMerges:     make(map[string]*MergeInfo),
		ReleaseBranches: make(map[string]string),
		ReleaseTags:     make(map[string]string),
		CommitTasks:     make(map[string]string),
	}
}

// IsEnabled returns whether tracking is enabled.
func (t *Tracker) IsEnabled() bool {
	return t.client.IsEnabled()
}

// LinkCommitToTask links a commit to a task.
func (t *Tracker) LinkCommitToTask(commitHash, taskID string) error {
	if !t.IsEnabled() {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Add to task commits
	commits := t.state.TaskCommits[taskID]
	for _, c := range commits {
		if c == commitHash {
			return nil // Already linked
		}
	}
	t.state.TaskCommits[taskID] = append(commits, commitHash)

	// Add reverse lookup
	t.state.CommitTasks[commitHash] = taskID

	t.state.LastUpdated = time.Now()
	return t.save()
}

// GetTaskCommits returns all commits linked to a task.
func (t *Tracker) GetTaskCommits(taskID string) []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state.TaskCommits[taskID]
}

// GetCommitTask returns the task ID for a commit.
func (t *Tracker) GetCommitTask(commitHash string) string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state.CommitTasks[commitHash]
}

// SetStoryBranch sets the branch for a story.
func (t *Tracker) SetStoryBranch(storyID, branch string) error {
	if !t.IsEnabled() {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.state.StoryBranches[storyID] = branch
	t.state.LastUpdated = time.Now()
	return t.save()
}

// GetStoryBranch returns the branch for a story.
func (t *Tracker) GetStoryBranch(storyID string) string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state.StoryBranches[storyID]
}

// RecordStoryMerge records when a story branch is merged.
func (t *Tracker) RecordStoryMerge(storyID string, info *MergeInfo) error {
	if !t.IsEnabled() {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.state.StoryMerges[storyID] = info
	t.state.LastUpdated = time.Now()
	return t.save()
}

// GetStoryMerge returns merge info for a story.
func (t *Tracker) GetStoryMerge(storyID string) *MergeInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state.StoryMerges[storyID]
}

// SetReleaseBranch sets the branch for a release.
func (t *Tracker) SetReleaseBranch(version, branch string) error {
	if !t.IsEnabled() {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.state.ReleaseBranches[version] = branch
	t.state.LastUpdated = time.Now()
	return t.save()
}

// GetReleaseBranch returns the branch for a release.
func (t *Tracker) GetReleaseBranch(version string) string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state.ReleaseBranches[version]
}

// SetReleaseTag sets the tag for a release.
func (t *Tracker) SetReleaseTag(version, tag string) error {
	if !t.IsEnabled() {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.state.ReleaseTags[version] = tag
	t.state.LastUpdated = time.Now()
	return t.save()
}

// GetReleaseTag returns the tag for a release.
func (t *Tracker) GetReleaseTag(version string) string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state.ReleaseTags[version]
}

// AutoLinkCommitsFromMessages scans recent commits and links them to tasks
// based on task ID patterns in commit messages (e.g., "T-001", "fixes T-002").
func (t *Tracker) AutoLinkCommitsFromMessages(since string) (int, error) {
	if !t.IsEnabled() {
		return 0, nil
	}

	commits, err := t.client.CommitsSince(since)
	if err != nil {
		return 0, err
	}

	// Pattern to match task IDs: T-001, T-123, etc.
	taskPattern := regexp.MustCompile(`\b(T-\d+)\b`)

	linked := 0
	for _, commit := range commits {
		matches := taskPattern.FindAllString(commit.Subject, -1)
		for _, taskID := range matches {
			if err := t.LinkCommitToTask(commit.Hash, taskID); err != nil {
				return linked, err
			}
			linked++
		}
	}

	return linked, nil
}

// CreateStoryBranch creates a branch for a story.
func (t *Tracker) CreateStoryBranch(storyID, baseBranch string) (string, error) {
	if !t.IsEnabled() {
		return "", nil
	}

	if err := t.client.Lock(); err != nil {
		return "", fmt.Errorf("failed to acquire git lock: %w", err)
	}
	defer t.client.Unlock()

	branch := fmt.Sprintf("feature/%s", storyID)

	if t.client.BranchExists(branch) {
		return branch, nil // Already exists
	}

	if err := t.client.CreateBranchFrom(branch, baseBranch); err != nil {
		return "", err
	}

	if err := t.SetStoryBranch(storyID, branch); err != nil {
		return "", err
	}

	return branch, nil
}

// CreateReleaseBranch creates a branch for a release.
func (t *Tracker) CreateReleaseBranch(version, baseBranch string) (string, error) {
	if !t.IsEnabled() {
		return "", nil
	}

	if err := t.client.Lock(); err != nil {
		return "", fmt.Errorf("failed to acquire git lock: %w", err)
	}
	defer t.client.Unlock()

	branch := fmt.Sprintf("release/%s", version)

	if t.client.BranchExists(branch) {
		return branch, nil // Already exists
	}

	if err := t.client.CreateBranchFrom(branch, baseBranch); err != nil {
		return "", err
	}

	if err := t.SetReleaseBranch(version, branch); err != nil {
		return "", err
	}

	return branch, nil
}

// MergeStoryToRelease merges a story branch into the release branch.
func (t *Tracker) MergeStoryToRelease(storyID, releaseVersion string) (*MergeInfo, error) {
	if !t.IsEnabled() {
		return nil, nil
	}

	if err := t.client.Lock(); err != nil {
		return nil, fmt.Errorf("failed to acquire git lock: %w", err)
	}
	defer t.client.Unlock()

	storyBranch := t.GetStoryBranch(storyID)
	if storyBranch == "" {
		return nil, fmt.Errorf("no branch found for story %s; ensure story was created with git enabled", storyID)
	}

	releaseBranch := t.GetReleaseBranch(releaseVersion)
	if releaseBranch == "" {
		return nil, fmt.Errorf("no branch found for release %s; ensure release was created with git enabled", releaseVersion)
	}

	// Verify branches exist before attempting merge
	if !t.client.BranchExists(storyBranch) {
		return nil, fmt.Errorf("story branch '%s' not found; it may have been deleted or not pushed", storyBranch)
	}
	if !t.client.BranchExists(releaseBranch) {
		return nil, fmt.Errorf("release branch '%s' not found; it may have been deleted or not pushed", releaseBranch)
	}

	// Record current branch for recovery
	currentBranch, _ := t.client.CurrentBranch()

	// Switch to release branch
	if err := t.client.Checkout(releaseBranch); err != nil {
		return nil, fmt.Errorf("failed to checkout release branch '%s': %w", releaseBranch, err)
	}

	// Merge story branch with no-ff to preserve history
	if err := t.client.MergeBranch(storyBranch, true); err != nil {
		// Attempt to recover by aborting merge and returning to original branch
		_, _ = t.client.run("merge", "--abort")
		if currentBranch != "" && currentBranch != releaseBranch {
			_ = t.client.Checkout(currentBranch)
		}
		return nil, &MergeConflictError{
			StoryID:      storyID,
			SourceBranch: storyBranch,
			TargetBranch: releaseBranch,
			RawError:     err,
		}
	}

	// Get merge commit
	mergeCommit, err := t.client.LatestCommit()
	if err != nil {
		return nil, fmt.Errorf("merge succeeded but failed to get commit hash: %w", err)
	}

	info := &MergeInfo{
		SourceBranch: storyBranch,
		TargetBranch: releaseBranch,
		MergeCommit:  mergeCommit,
		MergedAt:     time.Now(),
	}

	if err := t.RecordStoryMerge(storyID, info); err != nil {
		return nil, err
	}

	return info, nil
}

// MergeConflictError represents a git merge conflict.
type MergeConflictError struct {
	StoryID       string
	SourceBranch  string
	TargetBranch  string
	RawError      error
}

func (e *MergeConflictError) Error() string {
	return fmt.Sprintf("merge conflict in story %s: failed to merge %s into %s: %v", 
		e.StoryID, e.SourceBranch, e.TargetBranch, e.RawError)
}

// IsMergeConflict checks if an error is a merge conflict.
func IsMergeConflict(err error) bool {
	_, ok := err.(*MergeConflictError)
	return ok
}

// TagRelease creates a tag for a release.
func (t *Tracker) TagRelease(version, message string) error {
	if !t.IsEnabled() {
		return nil
	}

	if err := t.client.Lock(); err != nil {
		return fmt.Errorf("failed to acquire git lock: %w", err)
	}
	defer t.client.Unlock()

	tag := fmt.Sprintf("v%s", version)

	if t.client.TagExists(tag) {
		return nil // Already exists
	}

	if err := t.client.CreateTag(tag, message); err != nil {
		return err
	}

	return t.SetReleaseTag(version, tag)
}

// GetState returns a copy of the tracker state.
func (t *Tracker) GetState() *TrackerState {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Deep copy
	data, _ := json.Marshal(t.state)
	var copy TrackerState
	_ = json.Unmarshal(data, &copy)
	return &copy
}

func (t *Tracker) load() error {
	data, err := os.ReadFile(t.storePath)
	if err != nil {
		return err
	}

	state := newTrackerState()
	if err := json.Unmarshal(data, state); err != nil {
		return err
	}

	t.state = state
	return nil
}

func (t *Tracker) save() error {
	// Ensure directory exists
	dir := filepath.Dir(t.storePath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	data, err := json.MarshalIndent(t.state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(t.storePath, data, 0o600)
}
