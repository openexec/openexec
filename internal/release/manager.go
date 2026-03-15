package release

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/openexec/openexec/internal/git"
	"github.com/openexec/openexec/internal/tract"

	_ "github.com/mattn/go-sqlite3"
)

// Manager handles release, story, and task management with git integration.
type Manager struct {
	baseDir    string
	gitClient  *git.Client
	gitTracker *git.Tracker
	config     *Config
	store      Store // SQLite-backed state store
	mu         sync.RWMutex

	// Cached data (populated from SQLite, not JSON)
	release *Release
	goals   map[string]*Goal
	stories map[string]*Story
	tasks   map[string]*Task
}

// Config holds release management configuration.
type Config struct {
	// Git integration settings
	GitEnabled bool   `json:"git_enabled"`
	BaseBranch string `json:"base_branch"` // Default: "main"

	// Approval workflow settings
	ApprovalEnabled bool `json:"approval_enabled"` // Default: false

	// Auto-merge settings
	AutoMergeStories bool `json:"auto_merge_stories"` // Auto-merge story when all tasks done
	AutoMergeToMain  bool `json:"auto_merge_to_main"` // Auto-merge release to main when complete
	AutoTagRelease   bool `json:"auto_tag_release"`   // Auto-create tag when release complete

	// Auto-link commits to tasks based on commit message patterns
	AutoLinkCommits bool `json:"auto_link_commits"` // Default: true when git enabled

	// Branch naming patterns
	ReleaseBranchPrefix string `json:"release_branch_prefix"` // Default: "release/"
	FeatureBranchPrefix string `json:"feature_branch_prefix"` // Default: "feature/"
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		GitEnabled:          false,
		BaseBranch:          "main",
		ApprovalEnabled:     false,
		AutoLinkCommits:     true,
		ReleaseBranchPrefix: "release/",
		FeatureBranchPrefix: "feature/",
	}
}

// NewManager creates a new release manager.
func NewManager(baseDir string, cfg *Config) (*Manager, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	dbPath := filepath.Join(baseDir, ".openexec", "openexec.db")
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open state database: %w", err)
	}

	return NewManagerWithDB(baseDir, cfg, db)
}

// NewManagerWithDB creates a Manager using an existing database connection.
func NewManagerWithDB(baseDir string, cfg *Config, db *sql.DB) (*Manager, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	m := &Manager{
		baseDir: baseDir,
		config:  cfg,
		goals:   make(map[string]*Goal),
		stories: make(map[string]*Story),
		tasks:   make(map[string]*Task),
	}

	store, err := NewSQLiteStore(db)
	if err != nil {
		return nil, fmt.Errorf("failed to create state store: %w", err)
	}
	m.store = store

	// Initialize git client if enabled
	if cfg.GitEnabled {
		gitCfg := git.Config{
			Enabled:  true,
			RepoPath: baseDir,
		}
		m.gitClient = git.NewClient(gitCfg)

		trackerPath := filepath.Join(baseDir, ".openexec", "git-tracker.json")
		tracker, err := git.NewTracker(m.gitClient, trackerPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create git tracker: %w", err)
		}
		m.gitTracker = tracker
	} else {
		m.gitClient = git.NewClient(git.Config{Enabled: false})
	}

	// Load existing data
	if err := m.Load(); err != nil {
		return nil, err
	}

	return m, nil
}

// Close closes the manager and its underlying store.
func (m *Manager) Close() error {
	if m.store != nil {
		return m.store.Close()
	}
	return nil
}

// ResetStatuses resets all stories and tasks to pending status.
func (m *Manager) ResetStatuses() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, s := range m.stories {
		s.Status = StoryStatusPending
	}
	for _, t := range m.tasks {
		t.Status = TaskStatusPending
	}

	return m.saveUnlocked()
}

// GetConfig returns the current configuration.
func (m *Manager) GetConfig() *Config {
	return m.config
}

// BaseDir returns the base project directory.
func (m *Manager) BaseDir() string {
	return m.baseDir
}

// Load loads all data from SQLite store, with JSON bootstrap fallback.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ctx := context.Background()

	// Check if SQLite has data
	storyCount, err := m.store.CountStories(ctx)
	if err != nil {
		return fmt.Errorf("failed to count stories: %w", err)
	}

	// If SQLite is empty, bootstrap from JSON files (one-time migration)
	if storyCount == 0 {
		if err := m.bootstrapFromJSONUnlocked(ctx); err != nil {
			log.Printf("[ReleaseManager] bootstrap from JSON failed: %v", err)
		}
	}

	// Load from SQLite into cache
	return m.refreshCacheUnlocked(ctx)
}

// bootstrapFromJSONUnlocked imports data from JSON files into SQLite (one-time migration).
// Caller must hold the lock.
func (m *Manager) bootstrapFromJSONUnlocked(ctx context.Context) error {
	openexecDir := filepath.Join(m.baseDir, ".openexec")

	// Load and migrate release
	releasePath := filepath.Join(openexecDir, "release.json")
	if data, err := os.ReadFile(releasePath); err == nil {
		var release Release
		if err := json.Unmarshal(data, &release); err == nil {
			if err := m.store.CreateRelease(ctx, &release); err != nil && err != ErrReleaseAlreadyExist {
				return fmt.Errorf("failed to bootstrap release: %w", err)
			}
		}
	}

	// Load and migrate stories, goals, and embedded tasks from stories.json.
	storiesPath := filepath.Join(openexecDir, "stories.json")
	if data, err := os.ReadFile(storiesPath); err == nil {
		var rawFile struct {
			SchemaVersion string `json:"schema_version"`
			Goals         []Goal `json:"goals"`
			Stories       []struct {
				Story
				RawTasks json.RawMessage `json:"tasks"`
			} `json:"stories"`
		}

		if err := json.Unmarshal(data, &rawFile); err != nil {
			log.Printf("[Bootstrap] failed to unmarshal stories.json: %v", err)
		} else {
			log.Printf("[Bootstrap] parsed stories.json: %d goals, %d stories", len(rawFile.Goals), len(rawFile.Stories))

			goals := make([]*Goal, len(rawFile.Goals))
			for i := range rawFile.Goals {
				goals[i] = &rawFile.Goals[i]
			}
			if err := m.store.BulkCreateGoals(ctx, goals); err != nil {
				return fmt.Errorf("failed to bootstrap goals: %w", err)
			}

			var allTasks []*Task
			stories := make([]*Story, len(rawFile.Stories))
			for i := range rawFile.Stories {
				s := &rawFile.Stories[i].Story
				if s.Status == "" {
					s.Status = "pending"
				}

				var embeddedTasks []Task
				if err := json.Unmarshal(rawFile.Stories[i].RawTasks, &embeddedTasks); err == nil && len(embeddedTasks) > 0 && embeddedTasks[0].ID != "" {
					taskIDs := make([]string, len(embeddedTasks))
					for j := range embeddedTasks {
						taskIDs[j] = embeddedTasks[j].ID
						if embeddedTasks[j].StoryID == "" {
							embeddedTasks[j].StoryID = s.ID
						}
						if embeddedTasks[j].Status == "" {
							embeddedTasks[j].Status = "pending"
						}
						if embeddedTasks[j].MaxAttempts == 0 {
							embeddedTasks[j].MaxAttempts = 3
						}
						allTasks = append(allTasks, &embeddedTasks[j])
					}
					s.Tasks = taskIDs
				} else {
					var taskIDs []string
					_ = json.Unmarshal(rawFile.Stories[i].RawTasks, &taskIDs)
					s.Tasks = taskIDs
				}

				stories[i] = s
			}

			log.Printf("[Bootstrap] creating %d stories and %d tasks", len(stories), len(allTasks))
			if err := m.store.BulkCreateStories(ctx, stories); err != nil {
				return fmt.Errorf("failed to bootstrap stories: %w", err)
			}
			if len(allTasks) > 0 {
				if err := m.store.BulkCreateTasks(ctx, allTasks); err != nil {
					return fmt.Errorf("failed to bootstrap embedded tasks: %w", err)
				}
			}
		}
	}

	// Load tasks from separate tasks.json (if exists)
	tasksPath := filepath.Join(openexecDir, "tasks.json")
	if data, err := os.ReadFile(tasksPath); err == nil {
		var tasksData struct {
			Tasks []Task `json:"tasks"`
		}
		if err := json.Unmarshal(data, &tasksData); err == nil {
			tasks := make([]*Task, len(tasksData.Tasks))
			for i := range tasksData.Tasks {
				tasks[i] = &tasksData.Tasks[i]
			}
			if err := m.store.BulkCreateTasks(ctx, tasks); err != nil {
				return fmt.Errorf("failed to bootstrap tasks: %w", err)
			}
		}
	}

	return nil
}

// refreshCacheUnlocked populates the in-memory cache from SQLite.
// Caller must hold the lock.
func (m *Manager) refreshCacheUnlocked(ctx context.Context) error {
	// Clear existing cache
	m.goals = make(map[string]*Goal)
	m.stories = make(map[string]*Story)
	m.tasks = make(map[string]*Task)
	m.release = nil

	// Load release
	release, err := m.store.GetRelease(ctx)
	if err != nil {
		return fmt.Errorf("failed to load release: %w", err)
	}
	m.release = release

	// Load goals
	goals, err := m.store.ListGoals(ctx)
	if err != nil {
		return fmt.Errorf("failed to load goals: %w", err)
	}
	for _, g := range goals {
		m.goals[g.ID] = g
	}

	// Load stories
	stories, err := m.store.ListStories(ctx)
	if err != nil {
		return fmt.Errorf("failed to load stories: %w", err)
	}
	for _, s := range stories {
		m.stories[s.ID] = s
	}

	// Load tasks
	tasks, err := m.store.ListTasks(ctx)
	if err != nil {
		return fmt.Errorf("failed to load tasks: %w", err)
	}
	for _, t := range tasks {
		m.tasks[t.ID] = t
	}

	return nil
}

// Save saves all data to disk.
func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveUnlocked()
}

// GetRelease returns the current release.
func (m *Manager) GetRelease() *Release {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.release
}

// CreateRelease creates a new release.
func (m *Manager) CreateRelease(name, version, description string) (*Release, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	release := &Release{
		Name:        name,
		Version:     version,
		Description: description,
		Stories:     []string{},
		Status:      ReleaseStatusDraft,
		CreatedAt:   time.Now(),
	}

	// Create git branch if enabled
	if m.config.GitEnabled && m.gitTracker != nil {
		// Validate base branch exists, fetch if missing
		if err := m.validateBaseBranchUnlocked(m.config.BaseBranch); err != nil {
			return nil, err
		}

		branch, err := m.gitTracker.CreateReleaseBranch(version, m.config.BaseBranch)
		if err != nil {
			return nil, fmt.Errorf("failed to create release branch: %w", err)
		}

		release.Git = &ReleaseGitInfo{
			Branch:     branch,
			BaseBranch: m.config.BaseBranch,
		}
	}

	// Initialize approval if enabled
	if m.config.ApprovalEnabled {
		release.Approval = &ApprovalInfo{
			Status:      ApprovalPending,
			ReviewCycle: 0,
		}
	}

	m.release = release
	return release, m.saveUnlocked()
}

// GetGoal returns a goal by ID.
func (m *Manager) GetGoal(id string) *Goal {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.goals[id]
}

// GetGoals returns all goals.
func (m *Manager) GetGoals() []*Goal {
	m.mu.RLock()
	defer m.mu.RUnlock()

	goals := make([]*Goal, 0, len(m.goals))
	for _, g := range m.goals {
		goals = append(goals, g)
	}
	return goals
}

// CreateGoal creates a new goal.
func (m *Manager) CreateGoal(goal *Goal) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.goals[goal.ID] = goal
	return m.saveUnlocked()
}

// GetStory returns a story by ID.
func (m *Manager) GetStory(id string) *Story {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stories[id]
}

// GetStories returns all stories.
func (m *Manager) GetStories() []*Story {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stories := make([]*Story, 0, len(m.stories))
	for _, s := range m.stories {
		stories = append(stories, s)
	}
	return stories
}

// CreateStory creates a new story.
func (m *Manager) CreateStory(story *Story) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if story.CreatedAt.IsZero() {
		story.CreatedAt = time.Now()
	}
	if story.Status == "" {
		story.Status = StoryStatusPending
	}

	// Create git branch if enabled and release exists
	if m.config.GitEnabled && m.gitTracker != nil && m.release != nil {
		baseBranch := m.config.BaseBranch
		if m.release.Git != nil && m.release.Git.Branch != "" {
			baseBranch = m.release.Git.Branch
		}

		// Validate base branch exists, fetch if missing
		if err := m.validateBaseBranchUnlocked(baseBranch); err != nil {
			return err
		}

		branch, err := m.gitTracker.CreateStoryBranch(story.ID, baseBranch)
		if err != nil {
			return fmt.Errorf("failed to create story branch: %w", err)
		}

		story.Git = &StoryGitInfo{
			Branch:     branch,
			BaseBranch: baseBranch,
		}
	}

	// Initialize approval if enabled
	if m.config.ApprovalEnabled {
		story.Approval = &ApprovalInfo{
			Status:      ApprovalPending,
			ReviewCycle: 0,
		}
	}

	m.stories[story.ID] = story

	// Add to release if exists
	if m.release != nil {
		m.release.Stories = append(m.release.Stories, story.ID)
	}

	return m.saveUnlocked()
}

// GetTask returns a task by ID.
func (m *Manager) GetTask(id string) *Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tasks[id]
}

// GetTasks returns all tasks.
func (m *Manager) GetTasks() []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tasks := make([]*Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		tasks = append(tasks, t)
	}
	return tasks
}

// Brief returns a Tract-compatible BriefResponse for the given FWU (Task) ID.
// This implements the "built-in" Tract logic directly in the release manager.
func (m *Manager) Brief(fwuID string) (*tract.BriefResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	task, ok := m.tasks[fwuID]
	if !ok {
		return nil, fmt.Errorf("task %s not found", fwuID)
	}

	story, ok := m.stories[task.StoryID]
	if !ok {
		return nil, fmt.Errorf("story %s not found for task %s", task.StoryID, fwuID)
	}

	brief := &tract.BriefResponse{
		FWU: tract.FWU{
			ID:        task.ID,
			Name:      task.Title,
			Status:    string(task.Status),
			Intent:    task.Description,
			FeatureID: story.ID, // StoryID as fallback for FeatureID
		},
		ReasoningChain: &tract.ReasoningChain{},
	}

	// Map goals to reasoning chain
	if goal, ok := m.goals[story.GoalID]; ok {
		brief.ReasoningChain.Goals = []tract.ChainEntity{
			{ID: goal.ID, Name: goal.Title, Description: goal.Description},
		}
	}

	// Add boundaries from task description
	brief.Boundaries = append(brief.Boundaries, tract.Boundary{
		ID:          "scope",
		Scope:       "in_scope",
		Description: task.Description,
	})

	// Add dependencies
	for _, depID := range task.DependsOn {
		depText := "Prerequisite task"
		if dep, ok := m.tasks[depID]; ok {
			depText = dep.Title
		}
		brief.Dependencies = append(brief.Dependencies, tract.Dependency{
			ID:             depID,
			DependencyType: "prerequisite",
			TargetFWUID:    depID,
			Description:    depText,
		})
	}

	// If it's a Chassis task, add acceptance criteria from story
	if strings.Contains(strings.ToLower(task.Title), "chassis") {
		for i, ac := range story.AcceptanceCriteria {
			brief.DesignDecisions = append(brief.DesignDecisions, tract.DesignDecision{
				ID:         fmt.Sprintf("AC-%d", i+1),
				Decision:   "Acceptance Criteria",
				Resolution: ac,
			})
		}
	}

	return brief, nil
}

// GetTasksForStory returns all tasks for a story.
func (m *Manager) GetTasksForStory(storyID string) []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var tasks []*Task
	for _, t := range m.tasks {
		if t.StoryID == storyID {
			tasks = append(tasks, t)
		}
	}
	return tasks
}

// CreateTask creates a new task.
func (m *Manager) CreateTask(task *Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}
	if task.Status == "" {
		task.Status = TaskStatusPending
	}

	// Initialize git info if enabled
	if m.config.GitEnabled {
		task.Git = &TaskGitInfo{
			Commits: []string{},
		}

		// Set branch from story if available
		if story, ok := m.stories[task.StoryID]; ok && story.Git != nil {
			task.Git.Branch = story.Git.Branch
		}
	}

	// Initialize approval if enabled
	if m.config.ApprovalEnabled {
		task.Approval = &ApprovalInfo{
			Status:      ApprovalPending,
			ReviewCycle: 0,
		}
	}

	m.tasks[task.ID] = task

	// Add to story if exists
	if story, ok := m.stories[task.StoryID]; ok {
		story.Tasks = append(story.Tasks, task.ID)
	}

	return m.saveUnlocked()
}

// UpdateTask updates an existing task in place and persists changes.
// If the StoryID changes, it will remove the task from the old story and
// append it to the new story's task list.
func (m *Manager) UpdateTask(updated *Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.tasks[updated.ID]
	if !ok {
		return fmt.Errorf("task %s not found", updated.ID)
	}

	// Track old story to repair story.Tasks list if StoryID changes
	oldStoryID := existing.StoryID

	// Replace fields
	m.tasks[updated.ID] = updated

	// Move task between story task lists if needed
	if oldStoryID != updated.StoryID {
		if old, ok := m.stories[oldStoryID]; ok {
			// Filter out the task from old story
			filtered := make([]string, 0, len(old.Tasks))
			for _, id := range old.Tasks {
				if id != updated.ID {
					filtered = append(filtered, id)
				}
			}
			old.Tasks = filtered
		}
		if ns, ok := m.stories[updated.StoryID]; ok {
			// Avoid duplicates
			present := false
			for _, id := range ns.Tasks {
				if id == updated.ID {
					present = true
					break
				}
			}
			if !present {
				ns.Tasks = append(ns.Tasks, updated.ID)
			}
		}
	}

	return m.saveUnlocked()
}

// ReassignTask changes a task's StoryID and persists the update.
func (m *Manager) ReassignTask(taskID, newStoryID string) error {
	m.mu.RLock()
	t, ok := m.tasks[taskID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}
	// Copy to avoid mutating shared pointer inadvertently; keep existing as base
	updated := *t
	updated.StoryID = newStoryID
	return m.UpdateTask(&updated)
}

// SetTaskStatus updates the lifecycle status of a task and persists it.
func (m *Manager) SetTaskStatus(taskID string, status string) error {
	m.mu.RLock()
	t, ok := m.tasks[taskID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}
	updated := *t
	updated.Status = status
	return m.UpdateTask(&updated)
}

// DeleteTask removes a task from the tracking system and its parent story.
func (m *Manager) DeleteTask(taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return nil // already gone
	}

	// Remove from parent story
	if story, ok := m.stories[task.StoryID]; ok {
		filtered := make([]string, 0, len(story.Tasks))
		for _, id := range story.Tasks {
			if id != taskID {
				filtered = append(filtered, id)
			}
		}
		story.Tasks = filtered
	}

	// Remove from main tasks map
	delete(m.tasks, taskID)

	return m.saveUnlocked()
}

// LinkCommitToTask links a commit hash to a task.
func (m *Manager) LinkCommitToTask(taskID, commitHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}

	if task.Git == nil {
		task.Git = &TaskGitInfo{Commits: []string{}}
	}

	// Check if already linked
	for _, c := range task.Git.Commits {
		if c == commitHash {
			return nil
		}
	}

	task.Git.Commits = append(task.Git.Commits, commitHash)

	// Also update git tracker
	if m.gitTracker != nil {
		if err := m.gitTracker.LinkCommitToTask(commitHash, taskID); err != nil {
			return err
		}
	}

	return m.saveUnlocked()
}

// MergeStoryToRelease merges a story branch to the release branch.
func (m *Manager) MergeStoryToRelease(storyID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	story, ok := m.stories[storyID]
	if !ok {
		return fmt.Errorf("story %s not found", storyID)
	}

	if m.release == nil {
		return fmt.Errorf("no release defined")
	}

	// Check approval if required
	if m.config.ApprovalEnabled {
		if story.Approval == nil || story.Approval.Status != ApprovalApproved {
			return fmt.Errorf("story %s is not approved", storyID)
		}
	}

	// Merge git branch if enabled
	if m.config.GitEnabled && m.gitTracker != nil {
		info, err := m.gitTracker.MergeStoryToRelease(storyID, m.release.Version)
		if err != nil {
			return fmt.Errorf("failed to merge story branch: %w", err)
		}

		if info != nil && story.Git != nil {
			story.Git.MergedTo = info.TargetBranch
			story.Git.MergeCommit = info.MergeCommit
			story.Git.MergedAt = &info.MergedAt
		}
	}

	story.Status = StoryStatusDone
	now := time.Now()
	story.CompletedAt = &now

	return m.saveUnlocked()
}

// ApproveTask approves a task.
func (m *Manager) ApproveTask(taskID, approverID, comments string) error {
	if !m.config.ApprovalEnabled {
		return nil // Approval not required
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}

	if task.Approval == nil {
		task.Approval = &ApprovalInfo{}
	}

	now := time.Now()
	task.Approval.Status = ApprovalApproved
	task.Approval.ApprovedBy = approverID
	task.Approval.ApprovedAt = &now
	task.Approval.Comments = comments
	task.Status = TaskStatusApproved

	return m.saveUnlocked()
}

// RejectTask rejects a task.
func (m *Manager) RejectTask(taskID, approverID, reason string) error {
	if !m.config.ApprovalEnabled {
		return nil // Approval not required
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}

	if task.Approval == nil {
		task.Approval = &ApprovalInfo{}
	}

	now := time.Now()
	task.Approval.Status = ApprovalRejected
	task.Approval.ApprovedBy = approverID
	task.Approval.ApprovedAt = &now
	task.Approval.RejectionReason = reason
	task.Status = TaskStatusNeedsReview

	return m.saveUnlocked()
}

// ApproveStory approves a story.
func (m *Manager) ApproveStory(storyID, approverID, comments string) error {
	if !m.config.ApprovalEnabled {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	story, ok := m.stories[storyID]
	if !ok {
		return fmt.Errorf("story %s not found", storyID)
	}

	if story.Approval == nil {
		story.Approval = &ApprovalInfo{}
	}

	now := time.Now()
	story.Approval.Status = ApprovalApproved
	story.Approval.ApprovedBy = approverID
	story.Approval.ApprovedAt = &now
	story.Approval.Comments = comments
	story.Status = StoryStatusApproved

	return m.saveUnlocked()
}

// ApproveRelease approves a release.
func (m *Manager) ApproveRelease(approverID, comments string) error {
	if !m.config.ApprovalEnabled {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.release == nil {
		return fmt.Errorf("no release defined")
	}

	if m.release.Approval == nil {
		m.release.Approval = &ApprovalInfo{}
	}

	now := time.Now()
	m.release.Approval.Status = ApprovalApproved
	m.release.Approval.ApprovedBy = approverID
	m.release.Approval.ApprovedAt = &now
	m.release.Approval.Comments = comments
	m.release.Status = ReleaseStatusApproved

	return m.saveUnlocked()
}

// TagRelease creates a git tag for the release.
func (m *Manager) TagRelease(message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.release == nil {
		return fmt.Errorf("no release defined")
	}

	if m.config.GitEnabled && m.gitTracker != nil {
		if err := m.gitTracker.TagRelease(m.release.Version, message); err != nil {
			return err
		}

		if m.release.Git != nil {
			m.release.Git.Tag = "v" + m.release.Version
		}
	}

	return m.saveUnlocked()
}

// validateBaseBranchUnlocked validates a base branch exists, fetching if missing (caller must hold lock).
func (m *Manager) validateBaseBranchUnlocked(baseBranch string) error {
	if !m.config.GitEnabled || m.gitClient == nil {
		return nil
	}

	if m.gitClient.BranchExists(baseBranch) {
		return nil
	}

	// Try remote branch
	remoteBranch := "origin/" + baseBranch
	if m.gitClient.BranchExists(remoteBranch) {
		return nil
	}

	// Try fetching from origin
	if err := m.gitClient.FetchBranch(baseBranch); err != nil {
		return fmt.Errorf("base branch '%s' not found locally or on origin; run 'git fetch origin %s' or create it first",
			baseBranch, baseBranch)
	}

	return nil
}

// saveUnlocked persists state to SQLite without acquiring the lock (caller must hold lock).
// Note: This method no longer writes JSON files - SQLite is the canonical state store.
// Use ExportJSON() for read-only JSON artifact generation.
func (m *Manager) saveUnlocked() error {
	ctx := context.Background()

	// Sync release to SQLite
	if m.release != nil {
		// Try to get existing release
		existing, err := m.store.GetRelease(ctx)
		if err != nil {
			return fmt.Errorf("failed to check release: %w", err)
		}

		if existing == nil {
			// Create new release
			if err := m.store.CreateRelease(ctx, m.release); err != nil {
				return fmt.Errorf("failed to create release: %w", err)
			}
		} else {
			// Update existing release
			if err := m.store.UpdateRelease(ctx, m.release); err != nil {
				return fmt.Errorf("failed to update release: %w", err)
			}
		}
	}

	// Sync goals to SQLite
	for _, goal := range m.goals {
		existing, err := m.store.GetGoal(ctx, goal.ID)
		if err == ErrGoalNotFound {
			// Create new goal
			if err := m.store.CreateGoal(ctx, goal); err != nil && err != ErrGoalAlreadyExist {
				return fmt.Errorf("failed to create goal %s: %w", goal.ID, err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to check goal %s: %w", goal.ID, err)
		} else if existing != nil {
			// Update existing goal
			if err := m.store.UpdateGoal(ctx, goal); err != nil {
				return fmt.Errorf("failed to update goal %s: %w", goal.ID, err)
			}
		}
	}

	// Sync stories to SQLite
	for _, story := range m.stories {
		existing, err := m.store.GetStory(ctx, story.ID)
		if err == ErrStoryNotFound {
			// Create new story
			if err := m.store.CreateStory(ctx, story); err != nil && err != ErrStoryAlreadyExist {
				return fmt.Errorf("failed to create story %s: %w", story.ID, err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to check story %s: %w", story.ID, err)
		} else if existing != nil {
			// Update existing story
			if err := m.store.UpdateStory(ctx, story); err != nil {
				return fmt.Errorf("failed to update story %s: %w", story.ID, err)
			}
		}
	}

	// Sync tasks to SQLite
	for _, task := range m.tasks {
		existing, err := m.store.GetTask(ctx, task.ID)
		if err == ErrTaskNotFound {
			// Create new task
			if err := m.store.CreateTask(ctx, task); err != nil && err != ErrTaskAlreadyExist {
				return fmt.Errorf("failed to create task %s: %w", task.ID, err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to check task %s: %w", task.ID, err)
		} else if existing != nil {
			// Update existing task
			if err := m.store.UpdateTask(ctx, task); err != nil {
				return fmt.Errorf("failed to update task %s: %w", task.ID, err)
			}
		}
	}

	return nil
}

// ExportJSON exports the current state to JSON files in the specified directory.
// This is for read-only artifact generation only - SQLite remains the canonical state.
func (m *Manager) ExportJSON(exportDir string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := os.MkdirAll(exportDir, 0o750); err != nil {
		return err
	}

	// Export release
	if m.release != nil {
		releasePath := filepath.Join(exportDir, "release.json")
		data, err := json.MarshalIndent(m.release, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(releasePath, data, 0o600); err != nil {
			return err
		}
	}

	// Export stories and goals
	if len(m.stories) > 0 || len(m.goals) > 0 {
		storiesPath := filepath.Join(exportDir, "stories.json")
		storiesList := make([]Story, 0, len(m.stories))
		for _, s := range m.stories {
			storiesList = append(storiesList, *s)
		}
		goalsList := make([]Goal, 0, len(m.goals))
		for _, g := range m.goals {
			goalsList = append(goalsList, *g)
		}

		sf := struct {
			SchemaVersion string  `json:"schema_version"`
			Goals         []Goal  `json:"goals"`
			Stories       []Story `json:"stories"`
		}{
			SchemaVersion: "1.1",
			Goals:         goalsList,
			Stories:       storiesList,
		}

		data, err := json.MarshalIndent(sf, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(storiesPath, data, 0o600); err != nil {
			return err
		}
	}

	// Export tasks
	tasksPath := filepath.Join(exportDir, "tasks.json")
	tasksList := make([]Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		tasksList = append(tasksList, *t)
	}
	tasksData := struct {
		Tasks []Task `json:"tasks"`
	}{Tasks: tasksList}
	data, err := json.MarshalIndent(tasksData, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tasksPath, data, 0o600); err != nil {
		return err
	}

	return nil
}

// LoadConfig loads the release configuration from a project directory.
func LoadConfig(projectDir string) *Config {
	cfg := DefaultConfig()

	// Try to load from config.json in .openexec
	configPath := filepath.Join(projectDir, ".openexec", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return cfg
	}

	var fileConfig struct {
		GitEnabled          *bool   `json:"git_enabled"`
		ApprovalEnabled     *bool   `json:"approval_enabled"`
		BaseBranch          *string `json:"base_branch"`
		AutoMergeStories    *bool   `json:"auto_merge_stories"`
		AutoMergeToMain     *bool   `json:"auto_merge_to_main"`
		AutoTagRelease      *bool   `json:"auto_tag_release"`
		AutoLinkCommits     *bool   `json:"auto_link_commits"`
		ReleaseBranchPrefix *string `json:"release_branch_prefix"`
		FeatureBranchPrefix *string `json:"feature_branch_prefix"`
	}

	if err := json.Unmarshal(data, &fileConfig); err != nil {
		return cfg
	}

	if fileConfig.GitEnabled != nil {
		cfg.GitEnabled = *fileConfig.GitEnabled
	}
	if fileConfig.ApprovalEnabled != nil {
		cfg.ApprovalEnabled = *fileConfig.ApprovalEnabled
	}
	if fileConfig.BaseBranch != nil {
		cfg.BaseBranch = *fileConfig.BaseBranch
	}
	if fileConfig.AutoMergeStories != nil {
		cfg.AutoMergeStories = *fileConfig.AutoMergeStories
	}
	if fileConfig.AutoMergeToMain != nil {
		cfg.AutoMergeToMain = *fileConfig.AutoMergeToMain
	}
	if fileConfig.AutoTagRelease != nil {
		cfg.AutoTagRelease = *fileConfig.AutoTagRelease
	}
	if fileConfig.AutoLinkCommits != nil {
		cfg.AutoLinkCommits = *fileConfig.AutoLinkCommits
	}
	if fileConfig.ReleaseBranchPrefix != nil {
		cfg.ReleaseBranchPrefix = *fileConfig.ReleaseBranchPrefix
	}
	if fileConfig.FeatureBranchPrefix != nil {
		cfg.FeatureBranchPrefix = *fileConfig.FeatureBranchPrefix
	}

	return cfg
}
