// Package mcp provides MCP (Model Context Protocol) server functionality.
// This file implements the RestartManager for orchestrator restart operations
// as part of the meta self-fix capability.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"
)

// RestartStatus represents the lifecycle status of a restart request.
type RestartStatus string

const (
	// RestartStatusPending indicates the restart request is awaiting approval.
	RestartStatusPending RestartStatus = "pending"
	// RestartStatusApproved indicates the restart request was approved.
	RestartStatusApproved RestartStatus = "approved"
	// RestartStatusRejected indicates the restart request was rejected.
	RestartStatusRejected RestartStatus = "rejected"
	// RestartStatusInProgress indicates the restart is currently executing.
	RestartStatusInProgress RestartStatus = "in_progress"
	// RestartStatusComplete indicates the restart completed successfully.
	RestartStatusComplete RestartStatus = "complete"
	// RestartStatusFailed indicates the restart failed.
	RestartStatusFailed RestartStatus = "failed"
	// RestartStatusCancelled indicates the restart was cancelled.
	RestartStatusCancelled RestartStatus = "cancelled"
)

// ValidRestartStatuses contains all valid restart status values.
var ValidRestartStatuses = []RestartStatus{
	RestartStatusPending,
	RestartStatusApproved,
	RestartStatusRejected,
	RestartStatusInProgress,
	RestartStatusComplete,
	RestartStatusFailed,
	RestartStatusCancelled,
}

// IsValid checks if the status is a valid restart status value.
func (s RestartStatus) IsValid() bool {
	for _, valid := range ValidRestartStatuses {
		if s == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the restart status.
func (s RestartStatus) String() string {
	return string(s)
}

// IsFinal returns true if the status is terminal (no further changes expected).
func (s RestartStatus) IsFinal() bool {
	return s == RestartStatusComplete || s == RestartStatusFailed ||
		s == RestartStatusRejected || s == RestartStatusCancelled
}

// RestartReason categorizes why a restart is being requested.
type RestartReason string

const (
	// RestartReasonCodeChange indicates code modifications require restart.
	RestartReasonCodeChange RestartReason = "code_change"
	// RestartReasonConfigChange indicates configuration changes require restart.
	RestartReasonConfigChange RestartReason = "config_change"
	// RestartReasonUserRequested indicates manual user request.
	RestartReasonUserRequested RestartReason = "user_requested"
	// RestartReasonRecovery indicates recovery from an error state.
	RestartReasonRecovery RestartReason = "recovery"
	// RestartReasonUpgrade indicates an upgrade or update operation.
	RestartReasonUpgrade RestartReason = "upgrade"
)

// ValidRestartReasons contains all valid restart reason values.
var ValidRestartReasons = []RestartReason{
	RestartReasonCodeChange,
	RestartReasonConfigChange,
	RestartReasonUserRequested,
	RestartReasonRecovery,
	RestartReasonUpgrade,
}

// IsValid checks if the reason is a valid restart reason value.
func (r RestartReason) IsValid() bool {
	for _, valid := range ValidRestartReasons {
		if r == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the restart reason.
func (r RestartReason) String() string {
	return string(r)
}

// Common errors for restart operations.
var (
	ErrRestartNotFound        = errors.New("restart request not found")
	ErrRestartAlreadyDecided  = errors.New("restart request already has a decision")
	ErrRestartInProgress      = errors.New("a restart is already in progress")
	ErrRestartNotApproved     = errors.New("restart request not approved")
	ErrRestartPreflightFailed = errors.New("pre-flight checks failed")
	ErrBuildRequired          = errors.New("build required before restart")
	ErrInvalidRestartRequest  = errors.New("invalid restart request")
)

// RestartRequest represents a request to restart the orchestrator.
type RestartRequest struct {
	// ID is the unique identifier for the request (UUID).
	ID string `json:"id"`
	// Reason categorizes why the restart is requested.
	Reason RestartReason `json:"reason"`
	// Description provides additional context for the restart.
	Description string `json:"description,omitempty"`
	// RequestedBy identifies who/what triggered the request (agent ID, user ID).
	RequestedBy string `json:"requested_by"`
	// SessionID is the session that originated this request.
	SessionID string `json:"session_id,omitempty"`
	// ApprovalID links to the approval system request if applicable.
	ApprovalID string `json:"approval_id,omitempty"`
	// Status is the current status of the restart request.
	Status RestartStatus `json:"status"`
	// BuildRequired indicates if a build should run before restart.
	BuildRequired bool `json:"build_required"`
	// BuildResult stores the result if a build was performed.
	BuildResult *BuildResult `json:"build_result,omitempty"`
	// Port is the server port to restart on.
	Port int `json:"port"`
	// CreatedAt is when the request was created.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when the request was last modified.
	UpdatedAt time.Time `json:"updated_at"`
	// ApprovedAt is when the request was approved.
	ApprovedAt *time.Time `json:"approved_at,omitempty"`
	// CompletedAt is when the restart completed (success or failure).
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Session Resume Fields

	// ResumeEnabled indicates whether session resume is requested for this restart.
	ResumeEnabled bool `json:"resume_enabled"`
	// ResumeStateID is the ID of the SessionResumeState created for this restart.
	ResumeStateID string `json:"resume_state_id,omitempty"`
	// ResumeOnStartup indicates the session should auto-resume on next startup.
	ResumeOnStartup bool `json:"resume_on_startup"`
}

// NewRestartRequest creates a new RestartRequest with a generated UUID.
func NewRestartRequest(reason RestartReason, description, requestedBy string) (*RestartRequest, error) {
	if reason == "" {
		return nil, fmt.Errorf("%w: reason is required", ErrInvalidRestartRequest)
	}
	if requestedBy == "" {
		return nil, fmt.Errorf("%w: requested_by is required", ErrInvalidRestartRequest)
	}

	now := time.Now().UTC()
	return &RestartRequest{
		ID:          uuid.New().String(),
		Reason:      reason,
		Description: description,
		RequestedBy: requestedBy,
		Status:      RestartStatusPending,
		Port:        8080, // default port
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// Validate checks if the restart request has valid field values.
func (r *RestartRequest) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidRestartRequest)
	}
	if r.Reason == "" {
		return fmt.Errorf("%w: reason is required", ErrInvalidRestartRequest)
	}
	if !r.Status.IsValid() {
		return fmt.Errorf("%w: invalid status: %s", ErrInvalidRestartRequest, r.Status)
	}
	if r.RequestedBy == "" {
		return fmt.Errorf("%w: requested_by is required", ErrInvalidRestartRequest)
	}
	return nil
}

// IsPending returns true if the request is still awaiting a decision.
func (r *RestartRequest) IsPending() bool {
	return r.Status == RestartStatusPending
}

// IsApproved returns true if the request was approved.
func (r *RestartRequest) IsApproved() bool {
	return r.Status == RestartStatusApproved
}

// CanExecute returns true if the restart can be executed.
func (r *RestartRequest) CanExecute() bool {
	return r.Status == RestartStatusApproved
}

// Approve marks the request as approved.
func (r *RestartRequest) Approve() error {
	if r.Status.IsFinal() {
		return ErrRestartAlreadyDecided
	}
	if r.Status == RestartStatusInProgress {
		return ErrRestartInProgress
	}
	r.Status = RestartStatusApproved
	now := time.Now().UTC()
	r.ApprovedAt = &now
	r.UpdatedAt = now
	return nil
}

// Reject marks the request as rejected.
func (r *RestartRequest) Reject() error {
	if r.Status.IsFinal() {
		return ErrRestartAlreadyDecided
	}
	if r.Status == RestartStatusInProgress {
		return ErrRestartInProgress
	}
	r.Status = RestartStatusRejected
	now := time.Now().UTC()
	r.CompletedAt = &now
	r.UpdatedAt = now
	return nil
}

// Cancel marks the request as cancelled.
func (r *RestartRequest) Cancel() error {
	if r.Status.IsFinal() {
		return ErrRestartAlreadyDecided
	}
	if r.Status == RestartStatusInProgress {
		return ErrRestartInProgress
	}
	r.Status = RestartStatusCancelled
	now := time.Now().UTC()
	r.CompletedAt = &now
	r.UpdatedAt = now
	return nil
}

// SetPort sets the server port for the restart.
func (r *RestartRequest) SetPort(port int) {
	r.Port = port
	r.UpdatedAt = time.Now().UTC()
}

// SetSessionID sets the session ID for the request.
func (r *RestartRequest) SetSessionID(sessionID string) {
	r.SessionID = sessionID
	r.UpdatedAt = time.Now().UTC()
}

// SetBuildRequired marks that a build is required before restart.
func (r *RestartRequest) SetBuildRequired(required bool) {
	r.BuildRequired = required
	r.UpdatedAt = time.Now().UTC()
}

// EnableResume enables session resume for this restart.
func (r *RestartRequest) EnableResume(resumeOnStartup bool) {
	r.ResumeEnabled = true
	r.ResumeOnStartup = resumeOnStartup
	r.UpdatedAt = time.Now().UTC()
}

// SetResumeStateID sets the session resume state ID for this restart.
func (r *RestartRequest) SetResumeStateID(stateID string) {
	r.ResumeStateID = stateID
	r.UpdatedAt = time.Now().UTC()
}

// HasResume returns true if this restart has session resume enabled.
func (r *RestartRequest) HasResume() bool {
	return r.ResumeEnabled && r.ResumeStateID != ""
}

// RestartResult represents the result of a restart operation.
type RestartResult struct {
	// Success indicates whether the restart completed successfully.
	Success bool `json:"success"`
	// RequestID is the ID of the restart request.
	RequestID string `json:"request_id"`
	// Duration is how long the restart took.
	Duration time.Duration `json:"duration"`
	// BuildResult contains the build result if a build was performed.
	BuildResult *BuildResult `json:"build_result,omitempty"`
	// ErrorMessage contains the error message if the restart failed.
	ErrorMessage string `json:"error_message,omitempty"`
	// Output is any output from the restart process.
	Output string `json:"output,omitempty"`
	// NewPID is the PID of the newly started process.
	NewPID int `json:"new_pid,omitempty"`
	// ResumeStateID is the ID of the session resume state if resume was enabled.
	ResumeStateID string `json:"resume_state_id,omitempty"`
	// SessionPersisted indicates whether the session state was successfully persisted.
	SessionPersisted bool `json:"session_persisted"`
}

// PreflightCheck represents a single pre-flight validation check.
type PreflightCheck struct {
	// Name is the check identifier.
	Name string `json:"name"`
	// Description explains what the check validates.
	Description string `json:"description"`
	// Passed indicates if the check passed.
	Passed bool `json:"passed"`
	// Message provides additional context.
	Message string `json:"message,omitempty"`
	// Critical indicates if failure should block restart.
	Critical bool `json:"critical"`
}

// PreflightResult contains the results of all pre-flight checks.
type PreflightResult struct {
	// AllPassed indicates if all critical checks passed.
	AllPassed bool `json:"all_passed"`
	// Checks contains individual check results.
	Checks []*PreflightCheck `json:"checks"`
	// Errors contains any error messages.
	Errors []string `json:"errors,omitempty"`
}

// RestartManagerConfig holds configuration for the RestartManager.
type RestartManagerConfig struct {
	// Timeout is the maximum time for restart operations. Default is 2 minutes.
	Timeout time.Duration
	// BuildTimeout is the maximum time for build operations. Default is 5 minutes.
	BuildTimeout time.Duration
	// RequireApproval indicates if restarts require approval. Default is true.
	RequireApproval bool
	// AutoBuild indicates if builds should run automatically. Default is true.
	AutoBuild bool
	// DefaultPort is the default server port. Default is 8080.
	DefaultPort int
	// EnableSessionResume enables session resume functionality. Default is true.
	EnableSessionResume bool
	// SessionResumeConfig is the configuration for session resume. If nil, defaults are used.
	SessionResumeConfig *SessionResumeManagerConfig
}

// DefaultRestartManagerConfig returns a configuration with sensible defaults.
func DefaultRestartManagerConfig() *RestartManagerConfig {
	return &RestartManagerConfig{
		Timeout:             2 * time.Minute,
		BuildTimeout:        5 * time.Minute,
		RequireApproval:     true,
		AutoBuild:           true,
		DefaultPort:         8080,
		EnableSessionResume: true,
	}
}

// RestartManager manages orchestrator restart operations.
// It integrates with the build system and approval workflow to provide
// a safe, controlled restart mechanism for meta self-fix operations.
type RestartManager struct {
	// locator is used to find orchestrator files.
	locator *OrchestratorLocator
	// builder is used to rebuild the orchestrator if needed.
	builder *OrchestratorBuilder
	// config holds the manager configuration.
	config *RestartManagerConfig
	// resumeManager handles session resume state persistence.
	resumeManager *SessionResumeManager
	// mu protects concurrent access to state.
	mu sync.RWMutex
	// activeRequests tracks pending and in-progress restart requests.
	activeRequests map[string]*RestartRequest
	// currentRestart tracks the currently executing restart.
	currentRestart *RestartRequest
}

// NewRestartManager creates a new RestartManager with the given locator.
func NewRestartManager(locator *OrchestratorLocator) (*RestartManager, error) {
	return NewRestartManagerWithConfig(locator, nil)
}

// NewRestartManagerWithConfig creates a new RestartManager with custom configuration.
func NewRestartManagerWithConfig(locator *OrchestratorLocator, config *RestartManagerConfig) (*RestartManager, error) {
	if locator == nil {
		return nil, errors.New("orchestrator locator is required")
	}
	if config == nil {
		config = DefaultRestartManagerConfig()
	}

	builder := NewOrchestratorBuilder(locator)

	m := &RestartManager{
		locator:        locator,
		builder:        builder,
		config:         config,
		activeRequests: make(map[string]*RestartRequest),
	}

	// Initialize session resume manager if enabled
	if config.EnableSessionResume {
		resumeConfig := config.SessionResumeConfig
		if resumeConfig == nil {
			resumeConfig = DefaultSessionResumeManagerConfig()
			// Set storage path relative to orchestrator root
			resumeConfig.StoragePath = locator.Root() + "/.openexec/resume"
		}
		resumeManager, err := NewSessionResumeManager(resumeConfig)
		if err != nil {
			// Log but don't fail - session resume is not critical
			_ = err
		} else {
			m.resumeManager = resumeManager
		}
	}

	return m, nil
}

// RequestRestart creates a new restart request.
func (m *RestartManager) RequestRestart(ctx context.Context, reason RestartReason, description, requestedBy string) (*RestartRequest, error) {
	// Check if there's already an active restart
	m.mu.Lock()
	if m.currentRestart != nil && !m.currentRestart.Status.IsFinal() {
		m.mu.Unlock()
		return nil, ErrRestartInProgress
	}
	m.mu.Unlock()

	request, err := NewRestartRequest(reason, description, requestedBy)
	if err != nil {
		return nil, err
	}

	request.SetPort(m.config.DefaultPort)

	// If auto-build is enabled and reason is code change, mark build as required
	if m.config.AutoBuild && reason == RestartReasonCodeChange {
		request.SetBuildRequired(true)
	}

	// If approval is not required, auto-approve
	if !m.config.RequireApproval {
		if err := request.Approve(); err != nil {
			return nil, err
		}
	}

	// Store the request
	m.mu.Lock()
	m.activeRequests[request.ID] = request
	m.mu.Unlock()

	return request, nil
}

// GetRequest retrieves a restart request by ID.
func (m *RestartManager) GetRequest(ctx context.Context, id string) (*RestartRequest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	request, ok := m.activeRequests[id]
	if !ok {
		return nil, ErrRestartNotFound
	}

	return request, nil
}

// Approve approves a pending restart request.
func (m *RestartManager) Approve(ctx context.Context, requestID, decidedBy, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	request, ok := m.activeRequests[requestID]
	if !ok {
		return ErrRestartNotFound
	}

	return request.Approve()
}

// Reject rejects a pending restart request.
func (m *RestartManager) Reject(ctx context.Context, requestID, decidedBy, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	request, ok := m.activeRequests[requestID]
	if !ok {
		return ErrRestartNotFound
	}

	return request.Reject()
}

// Cancel cancels a pending restart request.
func (m *RestartManager) Cancel(ctx context.Context, requestID, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	request, ok := m.activeRequests[requestID]
	if !ok {
		return ErrRestartNotFound
	}

	return request.Cancel()
}

// CanRestart performs pre-flight checks to determine if restart is possible.
func (m *RestartManager) CanRestart(ctx context.Context) (*PreflightResult, error) {
	result := &PreflightResult{
		AllPassed: true,
		Checks:    []*PreflightCheck{},
	}

	// Check 1: Orchestrator root accessible
	rootCheck := &PreflightCheck{
		Name:        "orchestrator_root",
		Description: "Verify orchestrator root directory is accessible",
		Critical:    true,
	}
	if _, err := os.Stat(m.locator.Root()); err != nil {
		rootCheck.Passed = false
		rootCheck.Message = fmt.Sprintf("Cannot access orchestrator root: %v", err)
		result.AllPassed = false
	} else {
		rootCheck.Passed = true
		rootCheck.Message = fmt.Sprintf("Root accessible: %s", m.locator.Root())
	}
	result.Checks = append(result.Checks, rootCheck)

	// Check 2: Go binary available
	goCheck := &PreflightCheck{
		Name:        "go_binary",
		Description: "Verify Go compiler is available",
		Critical:    true,
	}
	if err := exec.Command("go", "version").Run(); err != nil {
		goCheck.Passed = false
		goCheck.Message = fmt.Sprintf("Go binary not available: %v", err)
		result.AllPassed = false
	} else {
		goCheck.Passed = true
		goCheck.Message = "Go compiler available"
	}
	result.Checks = append(result.Checks, goCheck)

	// Check 3: go.mod exists
	goModCheck := &PreflightCheck{
		Name:        "go_mod",
		Description: "Verify go.mod exists in orchestrator root",
		Critical:    true,
	}
	goModPath := m.locator.Root() + "/go.mod"
	if _, err := os.Stat(goModPath); err != nil {
		goModCheck.Passed = false
		goModCheck.Message = "go.mod not found in orchestrator root"
		result.AllPassed = false
	} else {
		goModCheck.Passed = true
		goModCheck.Message = "go.mod exists"
	}
	result.Checks = append(result.Checks, goModCheck)

	// Check 4: No restart already in progress
	inProgressCheck := &PreflightCheck{
		Name:        "no_restart_in_progress",
		Description: "Verify no restart is currently in progress",
		Critical:    true,
	}
	m.mu.RLock()
	if m.currentRestart != nil && !m.currentRestart.Status.IsFinal() {
		inProgressCheck.Passed = false
		inProgressCheck.Message = fmt.Sprintf("Restart already in progress: %s", m.currentRestart.ID)
		result.AllPassed = false
	} else {
		inProgressCheck.Passed = true
		inProgressCheck.Message = "No restart in progress"
	}
	m.mu.RUnlock()
	result.Checks = append(result.Checks, inProgressCheck)

	// Check 5: Executable path accessible
	execCheck := &PreflightCheck{
		Name:        "executable",
		Description: "Verify current executable is accessible",
		Critical:    false,
	}
	execPath, err := os.Executable()
	if err != nil {
		execCheck.Passed = false
		execCheck.Message = fmt.Sprintf("Cannot determine executable path: %v", err)
	} else {
		execCheck.Passed = true
		execCheck.Message = fmt.Sprintf("Executable: %s", execPath)
	}
	result.Checks = append(result.Checks, execCheck)

	// Collect errors
	for _, check := range result.Checks {
		if !check.Passed && check.Critical {
			result.Errors = append(result.Errors, check.Message)
		}
	}

	return result, nil
}

// ExecuteRestart performs the restart operation.
func (m *RestartManager) ExecuteRestart(ctx context.Context, requestID string) (*RestartResult, error) {
	start := time.Now()

	// Get and validate the request
	m.mu.Lock()
	request, ok := m.activeRequests[requestID]
	if !ok {
		m.mu.Unlock()
		return nil, ErrRestartNotFound
	}

	if !request.CanExecute() {
		m.mu.Unlock()
		return nil, ErrRestartNotApproved
	}

	// Check for concurrent restart
	if m.currentRestart != nil && !m.currentRestart.Status.IsFinal() {
		m.mu.Unlock()
		return nil, ErrRestartInProgress
	}

	// Mark as in progress
	request.Status = RestartStatusInProgress
	request.UpdatedAt = time.Now().UTC()
	m.currentRestart = request
	m.mu.Unlock()

	result := &RestartResult{
		RequestID: requestID,
	}

	// Apply timeout
	if m.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.config.Timeout)
		defer cancel()
	}

	// Run pre-flight checks
	preflight, err := m.CanRestart(ctx)
	if err != nil {
		m.completeRestart(request, false)
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("Pre-flight checks failed: %v", err)
		result.Duration = time.Since(start)
		return result, ErrRestartPreflightFailed
	}

	if !preflight.AllPassed {
		m.completeRestart(request, false)
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("Pre-flight checks failed: %v", preflight.Errors)
		result.Duration = time.Since(start)
		return result, ErrRestartPreflightFailed
	}

	// Build if required
	if request.BuildRequired {
		buildCtx := ctx
		if m.config.BuildTimeout > 0 {
			var cancel context.CancelFunc
			buildCtx, cancel = context.WithTimeout(ctx, m.config.BuildTimeout)
			defer cancel()
		}

		buildResult, err := m.builder.Build(buildCtx)
		request.BuildResult = buildResult
		result.BuildResult = buildResult

		if err != nil || !buildResult.Success {
			m.completeRestart(request, false)
			result.Success = false
			if err != nil {
				result.ErrorMessage = fmt.Sprintf("Build failed: %v", err)
			} else {
				result.ErrorMessage = fmt.Sprintf("Build failed with %d errors", len(buildResult.Errors))
			}
			result.Duration = time.Since(start)
			return result, ErrBuildRequired
		}
	}

	// Execute the restart
	restartErr := m.doRestart(ctx, request.Port)
	if restartErr != nil {
		m.completeRestart(request, false)
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("Restart failed: %v", restartErr)
		result.Duration = time.Since(start)
		return result, restartErr
	}

	// Success
	m.completeRestart(request, true)
	result.Success = true
	result.Duration = time.Since(start)
	result.Output = "Orchestrator restart initiated successfully"

	return result, nil
}

// doRestart performs the actual restart operation.
func (m *RestartManager) doRestart(ctx context.Context, port int) error {
	// Get the executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}

	// Stop the current process if running
	stopCmd := exec.CommandContext(ctx, "pkill", "-f", "openexec-execution")
	_ = stopCmd.Run() // Ignore error - process may not be running

	// Wait for process to stop
	time.Sleep(500 * time.Millisecond)

	// Start the new process
	startArgs := []string{"start", "--daemon", "--port", fmt.Sprintf("%d", port)}
	startCmd := exec.CommandContext(ctx, execPath, startArgs...)
	startCmd.Stdout = nil
	startCmd.Stderr = nil

	if err := startCmd.Start(); err != nil {
		return fmt.Errorf("failed to start new process: %w", err)
	}

	// Wait briefly and verify process started
	time.Sleep(1 * time.Second)

	return nil
}

// completeRestart marks a restart as completed with the given success status.
func (m *RestartManager) completeRestart(request *RestartRequest, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	request.CompletedAt = &now
	request.UpdatedAt = now

	if success {
		request.Status = RestartStatusComplete
	} else {
		request.Status = RestartStatusFailed
	}

	if m.currentRestart == request {
		m.currentRestart = nil
	}
}

// GetCurrentRestart returns the currently executing restart, if any.
func (m *RestartManager) GetCurrentRestart() *RestartRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentRestart
}

// ListActiveRequests returns all active (non-completed) restart requests.
func (m *RestartManager) ListActiveRequests(ctx context.Context) []*RestartRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var requests []*RestartRequest
	for _, request := range m.activeRequests {
		if !request.Status.IsFinal() {
			requests = append(requests, request)
		}
	}
	return requests
}

// ListPendingRequests returns all pending restart requests.
func (m *RestartManager) ListPendingRequests(ctx context.Context) []*RestartRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var requests []*RestartRequest
	for _, request := range m.activeRequests {
		if request.IsPending() {
			requests = append(requests, request)
		}
	}
	return requests
}

// ClearCompletedRequests removes all completed (final status) requests.
func (m *RestartManager) ClearCompletedRequests(ctx context.Context) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for id, request := range m.activeRequests {
		if request.Status.IsFinal() {
			delete(m.activeRequests, id)
			count++
		}
	}
	return count
}

// GetConfig returns the current configuration.
func (m *RestartManager) GetConfig() *RestartManagerConfig {
	return m.config
}

// SetConfig updates the manager configuration.
func (m *RestartManager) SetConfig(config *RestartManagerConfig) {
	if config != nil {
		m.config = config
	}
}

// GetLocator returns the orchestrator locator.
func (m *RestartManager) GetLocator() *OrchestratorLocator {
	return m.locator
}

// GetBuilder returns the orchestrator builder.
func (m *RestartManager) GetBuilder() *OrchestratorBuilder {
	return m.builder
}

// SetApprovalRequired enables or disables the approval requirement.
func (m *RestartManager) SetApprovalRequired(required bool) {
	m.config.RequireApproval = required
}

// SetAutoBuild enables or disables automatic builds.
func (m *RestartManager) SetAutoBuild(enabled bool) {
	m.config.AutoBuild = enabled
}

// SetDefaultPort sets the default port for restarts.
func (m *RestartManager) SetDefaultPort(port int) {
	m.config.DefaultPort = port
}

// GetResumeManager returns the session resume manager.
func (m *RestartManager) GetResumeManager() *SessionResumeManager {
	return m.resumeManager
}

// HasResumeManager returns true if session resume is enabled and available.
func (m *RestartManager) HasResumeManager() bool {
	return m.resumeManager != nil
}

// SessionState represents the minimal session state required for persistence.
// This is used as input to PersistSessionForRestart.
type SessionState struct {
	// SessionID is the unique session identifier.
	SessionID string
	// Iteration is the current iteration number.
	Iteration int
	// TotalTokens is the total tokens consumed.
	TotalTokens int
	// TotalCostUSD is the total cost in USD.
	TotalCostUSD float64
	// Messages is the conversation history (any JSON-serializable type).
	Messages interface{}
	// MessageCount is the number of messages.
	MessageCount int
	// LastSignal is the last received signal (any JSON-serializable type).
	LastSignal interface{}
	// IterationsSinceProgress tracks iterations without progress.
	IterationsSinceProgress int
	// Model is the LLM model being used.
	Model string
	// SystemPrompt is the system prompt.
	SystemPrompt string
	// ProjectPath is the project context path.
	ProjectPath string
	// WorkDir is the working directory.
	WorkDir string
	// PendingPrompt is any in-flight user prompt.
	PendingPrompt string
	// ContextSummary is the latest context summary.
	ContextSummary string
	// ToolsState is any stateful tool configuration.
	ToolsState interface{}
	// Metadata is arbitrary key-value pairs.
	Metadata map[string]string
}

// PersistSessionForRestart saves the session state for resume after restart.
// Returns the resume state ID on success.
func (m *RestartManager) PersistSessionForRestart(ctx context.Context, requestID string, state *SessionState) (string, error) {
	if m.resumeManager == nil {
		return "", errors.New("session resume not enabled")
	}

	if state == nil || state.SessionID == "" {
		return "", errors.New("invalid session state")
	}

	// Get the restart request to link them
	request, err := m.GetRequest(ctx, requestID)
	if err != nil {
		return "", fmt.Errorf("restart request not found: %w", err)
	}

	// Create a new resume state
	resumeState, err := m.resumeManager.CreateResumeState(ctx, state.SessionID)
	if err != nil {
		return "", fmt.Errorf("failed to create resume state: %w", err)
	}

	// Populate the resume state
	resumeState.RestartRequestID = requestID
	resumeState.Iteration = state.Iteration
	resumeState.TotalTokens = state.TotalTokens
	resumeState.TotalCostUSD = state.TotalCostUSD
	resumeState.MessageCount = state.MessageCount
	resumeState.IterationsSinceProgress = state.IterationsSinceProgress
	resumeState.Model = state.Model
	resumeState.SystemPrompt = state.SystemPrompt
	resumeState.ProjectPath = state.ProjectPath
	resumeState.WorkDir = state.WorkDir
	resumeState.PendingPrompt = state.PendingPrompt
	resumeState.ContextSummary = state.ContextSummary
	resumeState.Metadata = state.Metadata

	// Set complex fields
	if state.Messages != nil {
		if err := resumeState.SetMessages(state.Messages); err != nil {
			return "", fmt.Errorf("failed to set messages: %w", err)
		}
	}

	if state.LastSignal != nil {
		if err := resumeState.SetLastSignal(state.LastSignal); err != nil {
			return "", fmt.Errorf("failed to set last signal: %w", err)
		}
	}

	if state.ToolsState != nil {
		if err := resumeState.SetToolsState(state.ToolsState); err != nil {
			return "", fmt.Errorf("failed to set tools state: %w", err)
		}
	}

	// Update the resume state
	if err := m.resumeManager.UpdateResumeState(ctx, resumeState); err != nil {
		return "", fmt.Errorf("failed to update resume state: %w", err)
	}

	// Link the restart request to the resume state
	request.SetResumeStateID(resumeState.ID)

	return resumeState.ID, nil
}

// GetSessionResumeState retrieves a session resume state by ID.
func (m *RestartManager) GetSessionResumeState(ctx context.Context, stateID string) (*SessionResumeState, error) {
	if m.resumeManager == nil {
		return nil, errors.New("session resume not enabled")
	}
	return m.resumeManager.GetResumeState(ctx, stateID)
}

// GetLatestSessionResumeState retrieves the latest pending resume state for a session.
func (m *RestartManager) GetLatestSessionResumeState(ctx context.Context, sessionID string) (*SessionResumeState, error) {
	if m.resumeManager == nil {
		return nil, errors.New("session resume not enabled")
	}
	return m.resumeManager.GetLatestResumeState(ctx, sessionID)
}

// ListPendingSessionResumes returns all sessions that can be resumed.
func (m *RestartManager) ListPendingSessionResumes(ctx context.Context) []*SessionResumeState {
	if m.resumeManager == nil {
		return nil
	}
	return m.resumeManager.ListPendingStates(ctx)
}

// ResumeSession begins the session resume process for a given state ID.
// Returns the resume state containing all the data needed to restore the session.
func (m *RestartManager) ResumeSession(ctx context.Context, stateID string) (*SessionResumeState, error) {
	if m.resumeManager == nil {
		return nil, errors.New("session resume not enabled")
	}
	return m.resumeManager.ResumeSession(ctx, stateID)
}

// CompleteSessionResume marks a session resume as completed.
func (m *RestartManager) CompleteSessionResume(ctx context.Context, stateID string) error {
	if m.resumeManager == nil {
		return errors.New("session resume not enabled")
	}
	return m.resumeManager.CompleteResume(ctx, stateID)
}

// FailSessionResume marks a session resume as failed.
func (m *RestartManager) FailSessionResume(ctx context.Context, stateID, errMsg string) error {
	if m.resumeManager == nil {
		return errors.New("session resume not enabled")
	}
	return m.resumeManager.FailResume(ctx, stateID, errMsg)
}

// RequestRestartWithResume creates a restart request with session resume enabled.
func (m *RestartManager) RequestRestartWithResume(ctx context.Context, reason RestartReason, description, requestedBy string, sessionState *SessionState, resumeOnStartup bool) (*RestartRequest, error) {
	// Create the base restart request
	request, err := m.RequestRestart(ctx, reason, description, requestedBy)
	if err != nil {
		return nil, err
	}

	// Enable session resume
	request.EnableResume(resumeOnStartup)

	// Set the session ID from the state
	if sessionState != nil && sessionState.SessionID != "" {
		request.SetSessionID(sessionState.SessionID)

		// Persist the session state
		if m.resumeManager != nil {
			stateID, err := m.PersistSessionForRestart(ctx, request.ID, sessionState)
			if err != nil {
				// Log but don't fail - session resume is optional
				_ = err
			} else {
				request.SetResumeStateID(stateID)
			}
		}
	}

	return request, nil
}

// Close cleans up resources used by the RestartManager.
func (m *RestartManager) Close() error {
	if m.resumeManager != nil {
		return m.resumeManager.Close()
	}
	return nil
}

// =====================================================
// Auto-Resume on Startup
// =====================================================

// AutoResumeResult represents the result of an auto-resume check or operation.
type AutoResumeResult struct {
	// HasPendingResume indicates if there are sessions pending resume.
	HasPendingResume bool `json:"has_pending_resume"`

	// PendingStates contains all sessions that can be auto-resumed.
	PendingStates []*SessionResumeState `json:"pending_states,omitempty"`

	// ResumedStateID is the ID of the state that was resumed (if auto-resume was performed).
	ResumedStateID string `json:"resumed_state_id,omitempty"`

	// ResumedState contains the state data for the resumed session.
	ResumedState *SessionResumeState `json:"resumed_state,omitempty"`

	// Error contains any error message if auto-resume failed.
	Error string `json:"error,omitempty"`
}

// CheckAutoResume checks if there are any sessions that should be auto-resumed on startup.
// This should be called early during server startup to detect pending session resumes.
func (m *RestartManager) CheckAutoResume(ctx context.Context) *AutoResumeResult {
	result := &AutoResumeResult{
		HasPendingResume: false,
	}

	if m.resumeManager == nil {
		return result
	}

	// Get all pending resume states
	pendingStates := m.resumeManager.ListPendingStates(ctx)
	if len(pendingStates) == 0 {
		return result
	}

	// Filter to only include states marked for auto-resume on startup
	// These are states where the restart request had ResumeOnStartup=true
	// For now, we include all pending states as candidates for resume
	result.HasPendingResume = true
	result.PendingStates = pendingStates

	return result
}

// AutoResumeSession attempts to automatically resume a session from a pending state.
// If stateID is empty, it will resume the most recent pending state.
// Returns the resume state that should be used to restore the session.
func (m *RestartManager) AutoResumeSession(ctx context.Context, stateID string) (*AutoResumeResult, error) {
	result := &AutoResumeResult{}

	if m.resumeManager == nil {
		result.Error = "session resume not enabled"
		return result, errors.New(result.Error)
	}

	// If no specific state ID provided, find the most recent pending state
	if stateID == "" {
		pendingStates := m.resumeManager.ListPendingStates(ctx)
		if len(pendingStates) == 0 {
			result.Error = "no sessions pending resume"
			return result, ErrNoSessionToResume
		}

		// Use the most recent pending state (last in list)
		stateID = pendingStates[len(pendingStates)-1].ID
	}

	// Mark the state as resuming and get the data
	state, err := m.resumeManager.ResumeSession(ctx, stateID)
	if err != nil {
		result.Error = fmt.Sprintf("failed to resume session: %v", err)
		return result, err
	}

	result.HasPendingResume = true
	result.ResumedStateID = state.ID
	result.ResumedState = state

	return result, nil
}

// AutoResumeBySessionID attempts to auto-resume a session by its original session ID.
// This is useful when you want to resume a specific session after restart.
func (m *RestartManager) AutoResumeBySessionID(ctx context.Context, sessionID string) (*AutoResumeResult, error) {
	result := &AutoResumeResult{}

	if m.resumeManager == nil {
		result.Error = "session resume not enabled"
		return result, errors.New(result.Error)
	}

	// Get the latest pending state for this session
	state, err := m.resumeManager.GetLatestResumeState(ctx, sessionID)
	if err != nil {
		result.Error = fmt.Sprintf("failed to find resume state for session %s: %v", sessionID, err)
		return result, err
	}

	// Attempt to resume
	return m.AutoResumeSession(ctx, state.ID)
}

// CompleteAutoResume marks an auto-resume as successfully completed.
// This should be called after the session has been fully restored and is running.
func (m *RestartManager) CompleteAutoResume(ctx context.Context, stateID string) error {
	return m.CompleteSessionResume(ctx, stateID)
}

// FailAutoResume marks an auto-resume as failed.
// This should be called if the session failed to restore properly.
func (m *RestartManager) FailAutoResume(ctx context.Context, stateID, errMsg string) error {
	return m.FailSessionResume(ctx, stateID, errMsg)
}

// CancelPendingResumes marks all pending resume states as expired/cancelled.
// This can be used to clear the resume queue if auto-resume is not desired.
func (m *RestartManager) CancelPendingResumes(ctx context.Context) (int, error) {
	if m.resumeManager == nil {
		return 0, errors.New("session resume not enabled")
	}

	pendingStates := m.resumeManager.ListPendingStates(ctx)
	cancelled := 0

	for _, state := range pendingStates {
		state.MarkFailed("cancelled by user")
		if err := m.resumeManager.UpdateResumeState(ctx, state); err == nil {
			cancelled++
		}
	}

	return cancelled, nil
}
