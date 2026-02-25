package release

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/openexec/openexec/internal/git"
)

// Manager handles release, story, and task management with git integration.
type Manager struct {
	baseDir    string
	gitClient  *git.Client
	gitTracker *git.Tracker
	config     *Config
	mu         sync.RWMutex

	// Cached data
	release *Release
	goals   map[string]*Goal
	stories map[string]*Story
	tasks   map[string]*Task
}

// Config holds release management configuration.
type Config struct {
	// Git integration settings
	GitEnabled  bool   `json:"git_enabled"`
	BaseBranch  string `json:"base_branch"`  // Default: "main"

	// Approval workflow settings
	ApprovalEnabled bool `json:"approval_enabled"` // Default: false

	// Auto-merge settings
	AutoMergeStories  bool `json:"auto_merge_stories"`  // Auto-merge story when all tasks done
	AutoMergeToMain   bool `json:"auto_merge_to_main"`  // Auto-merge release to main when complete
	AutoTagRelease    bool `json:"auto_tag_release"`    // Auto-create tag when release complete

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

	m := &Manager{
		baseDir: baseDir,
		config:  cfg,
		goals:   make(map[string]*Goal),
		stories: make(map[string]*Story),
		tasks:   make(map[string]*Task),
	}

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

// GetConfig returns the current configuration.
func (m *Manager) GetConfig() *Config {
	return m.config
}

// Load loads all data from disk.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	openexecDir := filepath.Join(m.baseDir, ".openexec")

	// Load release
	releasePath := filepath.Join(openexecDir, "release.json")
	if data, err := os.ReadFile(releasePath); err == nil {
		var release Release
		if err := json.Unmarshal(data, &release); err == nil {
			m.release = &release
		}
	}

	// Load stories and goals
	storiesPath := filepath.Join(openexecDir, "stories.json")
	if data, err := os.ReadFile(storiesPath); err == nil {
		var sf struct {
			SchemaVersion string  `json:"schema_version"`
			Goals         []Goal  `json:"goals"`
			Stories       []Story `json:"stories"`
		}

		if err := json.Unmarshal(data, &sf); err == nil {
			for i := range sf.Goals {
				m.goals[sf.Goals[i].ID] = &sf.Goals[i]
			}
			for i := range sf.Stories {
				m.stories[sf.Stories[i].ID] = &sf.Stories[i]
			}
		} else {
			// Fallback for legacy format
			var bareStories []Story
			if errArray := json.Unmarshal(data, &bareStories); errArray == nil {
				for i := range bareStories {
					m.stories[bareStories[i].ID] = &bareStories[i]
				}
			}
		}
	}

	// Load tasks
	tasksPath := filepath.Join(openexecDir, "tasks.json")
	if data, err := os.ReadFile(tasksPath); err == nil {
		var tasksData struct {
			Tasks []Task `json:"tasks"`
		}
		if err := json.Unmarshal(data, &tasksData); err == nil {
			for i := range tasksData.Tasks {
				m.tasks[tasksData.Tasks[i].ID] = &tasksData.Tasks[i]
			}
		}
	}

	return nil
}

// Save saves all data to disk.
func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	openexecDir := filepath.Join(m.baseDir, ".openexec")
	if err := os.MkdirAll(openexecDir, 0o750); err != nil {
		return err
	}

	// Save release
	if m.release != nil {
		releasePath := filepath.Join(openexecDir, "release.json")
		data, err := json.MarshalIndent(m.release, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(releasePath, data, 0o600); err != nil {
			return err
		}
	}

	// Save stories and goals
	storiesPath := filepath.Join(openexecDir, "stories.json")
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

	// Save tasks
	tasksPath := filepath.Join(openexecDir, "tasks.json")
	tasksList := make([]Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		tasksList = append(tasksList, *t)
	}
	tasksData := struct {
		Tasks []Task `json:"tasks"`
	}{Tasks: tasksList}
	data, err = json.MarshalIndent(tasksData, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tasksPath, data, 0o600); err != nil {
		return err
	}

	return nil
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

// saveUnlocked saves data without acquiring the lock (caller must hold lock).
func (m *Manager) saveUnlocked() error {
	openexecDir := filepath.Join(m.baseDir, ".openexec")
	if err := os.MkdirAll(openexecDir, 0o750); err != nil {
		return err
	}

	// Save release
	if m.release != nil {
		releasePath := filepath.Join(openexecDir, "release.json")
		data, err := json.MarshalIndent(m.release, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(releasePath, data, 0o600); err != nil {
			return err
		}
	}

	// Save stories and goals
	storiesPath := filepath.Join(openexecDir, "stories.json")
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

	// Save tasks
	tasksPath := filepath.Join(openexecDir, "tasks.json")
	tasksList := make([]Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		tasksList = append(tasksList, *t)
	}
	tasksData := struct {
		Tasks []Task `json:"tasks"`
	}{Tasks: tasksList}
	data, err = json.MarshalIndent(tasksData, "", "  ")
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
