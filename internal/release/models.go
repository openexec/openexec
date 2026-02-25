// Package release provides release management with git integration and optional approval workflows.
package release

import (
	"time"
)

// Release represents a software release with git and approval tracking.
type Release struct {
	// Core fields
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`

	// Stories included in this release
	Stories []string `json:"stories"` // Story IDs

	// Git integration (populated when git tracking enabled)
	Git *ReleaseGitInfo `json:"git,omitempty"`

	// Approval workflow (populated when approval required)
	Approval *ApprovalInfo `json:"approval,omitempty"`

	// Lifecycle timestamps
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	DeployedAt  *time.Time `json:"deployed_at,omitempty"`

	// Status: draft, in_progress, ready_for_approval, approved, deployed
	Status string `json:"status"`

	// Deployment targets (e.g., ["staging", "production"])
	DeployedTo []string `json:"deployed_to,omitempty"`

	// Generated changelog (markdown)
	Changelog string `json:"changelog,omitempty"`
}

// ReleaseGitInfo holds git-related information for a release.
type ReleaseGitInfo struct {
	Branch      string `json:"branch"`                  // e.g., "release/1.0.0"
	Tag         string `json:"tag,omitempty"`           // e.g., "v1.0.0"
	BaseBranch  string `json:"base_branch"`             // e.g., "main"
	MergeCommit string `json:"merge_commit,omitempty"`  // Final merge to main
	HeadCommit  string `json:"head_commit,omitempty"`   // Latest commit on release branch
}

// Story represents a user story with git and approval tracking.
type Story struct {
	// Core fields
	ID          string   `json:"id"`                      // e.g., "US-001"
	EpicID      *string  `json:"epic_id,omitempty"`       // Parent epic if any
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`   // Short description
	Role        string   `json:"role,omitempty"`          // As a <role>
	Want        string   `json:"want,omitempty"`          // I want <feature>
	Benefit     string   `json:"benefit,omitempty"`       // So that <benefit>

	// Acceptance criteria
	AcceptanceCriteria []string `json:"acceptance_criteria"`

	// Task references
	Tasks []string `json:"tasks"` // Task IDs

	// Dependencies
	DependsOn []string `json:"depends_on,omitempty"` // Story IDs this depends on

	// Classification
	StoryType string `json:"story_type"` // feature, bugfix, chore, refactor
	Priority  int    `json:"priority"`   // 0 = highest

	// Git integration (populated when git tracking enabled)
	Git *StoryGitInfo `json:"git,omitempty"`

	// Approval workflow (populated when approval required)
	Approval *ApprovalInfo `json:"approval,omitempty"`

	// Lifecycle
	Status      string     `json:"status"` // pending, in_progress, ready_for_review, approved, done
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// StoryGitInfo holds git-related information for a story.
type StoryGitInfo struct {
	Branch       string     `json:"branch"`                  // e.g., "feature/US-001"
	BaseBranch   string     `json:"base_branch"`             // e.g., "release/1.0.0"
	MergedTo     string     `json:"merged_to,omitempty"`     // Branch it was merged into
	MergeCommit  string     `json:"merge_commit,omitempty"`  // Merge commit hash
	MergedAt     *time.Time `json:"merged_at,omitempty"`
	CommitCount  int        `json:"commit_count,omitempty"`  // Total commits on branch
}

// Task represents a work unit with git and approval tracking.
type Task struct {
	// Core fields
	ID          string  `json:"id"`           // e.g., "T-001"
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	StoryID     string  `json:"story_id"`     // Parent story

	// Dependencies
	DependsOn []string `json:"depends_on,omitempty"` // Task IDs this depends on

	// Classification
	TaskType string `json:"task_type,omitempty"` // implementation, test, docs, fix
	Priority int    `json:"priority"`

	// Execution tracking
	AssignedAgent string `json:"assigned_agent,omitempty"`
	AttemptCount  int    `json:"attempt_count"`
	MaxAttempts   int    `json:"max_attempts"`

	// Git integration (populated when git tracking enabled)
	Git *TaskGitInfo `json:"git,omitempty"`

	// Approval workflow (populated when approval required)
	Approval *ApprovalInfo `json:"approval,omitempty"`

	// Review flags
	NeedsReview bool   `json:"needs_review"`
	ReviewNotes string `json:"review_notes,omitempty"`

	// Lifecycle
	Status       string     `json:"status"` // pending, in_progress, needs_review, approved, done, failed
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`

	// Metadata for fix tracking
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// TaskGitInfo holds git-related information for a task.
type TaskGitInfo struct {
	Commits     []string `json:"commits"`               // Commit hashes implementing this task
	Branch      string   `json:"branch,omitempty"`      // Branch where work was done
	PRNumber    *int     `json:"pr_number,omitempty"`   // Pull request number if applicable
	PRUrl       string   `json:"pr_url,omitempty"`      // Pull request URL
}

// ApprovalInfo holds approval workflow information.
type ApprovalInfo struct {
	// Approval state: pending, approved, rejected
	Status string `json:"status"`

	// Who approved/rejected
	ApprovedBy string `json:"approved_by,omitempty"`

	// When approved/rejected
	ApprovedAt *time.Time `json:"approved_at,omitempty"`

	// Approval comments/rationale
	Comments string `json:"comments,omitempty"`

	// For rejections: what needs to be fixed
	RejectionReason string `json:"rejection_reason,omitempty"`

	// Review cycle number (increments on each review request)
	ReviewCycle int `json:"review_cycle"`

	// Review requested timestamp
	ReviewRequestedAt *time.Time `json:"review_requested_at,omitempty"`
}

// ApprovalStatus constants
const (
	ApprovalPending  = "pending"
	ApprovalApproved = "approved"
	ApprovalRejected = "rejected"
)

// ReleaseStatus constants
const (
	ReleaseStatusDraft            = "draft"
	ReleaseStatusInProgress       = "in_progress"
	ReleaseStatusReadyForApproval = "ready_for_approval"
	ReleaseStatusApproved         = "approved"
	ReleaseStatusDeployed         = "deployed"
)

// StoryStatus constants
const (
	StoryStatusPending        = "pending"
	StoryStatusInProgress     = "in_progress"
	StoryStatusReadyForReview = "ready_for_review"
	StoryStatusApproved       = "approved"
	StoryStatusDone           = "done"
)

// TaskStatus constants
const (
	TaskStatusPending     = "pending"
	TaskStatusInProgress  = "in_progress"
	TaskStatusNeedsReview = "needs_review"
	TaskStatusApproved    = "approved"
	TaskStatusDone        = "done"
	TaskStatusFailed      = "failed"
)

// StoryType constants
const (
	StoryTypeFeature  = "feature"
	StoryTypeBugfix   = "bugfix"
	StoryTypeChore    = "chore"
	StoryTypeRefactor = "refactor"
)
