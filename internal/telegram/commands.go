package telegram

import (
	"context"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/openexec/openexec/internal/user"
	"github.com/openexec/openexec/internal/conversation"
	"github.com/openexec/openexec/internal/logging"
	"github.com/openexec/openexec/internal/protocol"
)

// MessageSender is an interface for sending Telegram messages.
// This allows for easier testing by mocking the message sending capability.
type MessageSender interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
}

// FileSender is an interface for sending files via Telegram.
// Implementations can send documents/files as attachments.
type FileSender interface {
	MessageSender
	// SendDocument sends a document to a chat with optional caption.
	SendDocument(chatID int64, fileName string, fileData []byte, caption string) (tgbotapi.Message, error)
}

// StatusProvider provides system status information.
// This interface allows for dependency injection and easier testing.
type StatusProvider interface {
	// GetStatus returns the current system status.
	GetStatus() *protocol.StatusResponse
}

// AggregatedStatusProvider provides aggregated status from connected OpenExec clients.
// This interface allows querying multiple connected clients and aggregating their responses.
type AggregatedStatusProvider interface {
	// BroadcastStatus sends status requests to all connected clients and returns aggregated results.
	// The includeMetrics flag requests additional performance metrics from clients.
	BroadcastStatus(ctx context.Context, includeMetrics bool) (*AggregatedClientStatus, error)
}

// RunCommandSender sends run commands to connected OpenExec clients.
// This interface allows sending task execution requests to specific projects.
type RunCommandSender interface {
	// SendRunCommand sends a run request for a task.
	// If projectID is empty, routes to any available client.
	// If projectID is specified, routes to a client connected for that project.
	// Returns the response from the client or an error if the request fails.
	SendRunCommand(ctx context.Context, taskID, projectID string) (*RunCommandResult, error)
}

// LogsCommandSender sends logs requests to connected OpenExec clients.
// This interface allows retrieving task logs from specific projects.
type LogsCommandSender interface {
	// SendLogsCommand sends a logs request for a task.
	// If projectID is empty, routes to any available client.
	// If projectID is specified, routes to a client connected for that project.
	// Returns the response from the client or an error if the request fails.
	SendLogsCommand(ctx context.Context, taskID, projectID string) (*LogsCommandResult, error)
}

// CancelCommandSender sends cancel requests to connected OpenExec clients.
// This interface allows cancelling running tasks on specific projects.
type CancelCommandSender interface {
	// SendCancelCommand sends a cancel request for a task.
	// If projectID is empty, routes to any available client.
	// If projectID is specified, routes to a client connected for that project.
	// Returns the response from the client or an error if the request fails.
	SendCancelCommand(ctx context.Context, taskID, projectID string) (*CancelCommandResult, error)
}

// CreateTaskCommandSender sends create task requests to connected OpenExec clients.
// This interface allows creating new tasks in the task queue.
type CreateTaskCommandSender interface {
	// SendCreateTaskCommand sends a create task request.
	// If projectID is empty, routes to any available client.
	// If projectID is specified, routes to a client connected for that project.
	// Returns the response from the client or an error if the request fails.
	SendCreateTaskCommand(ctx context.Context, title, description, projectID string) (*CreateTaskCommandResult, error)
}

// DeployCommandSender sends deploy requests to connected OpenExec clients.
// This interface allows deploying projects to target environments.
type DeployCommandSender interface {
	// SendDeployCommand sends a deploy request for a project.
	// If projectID is empty, routes to any available client.
	// If projectID is specified, routes to a client connected for that project.
	// Returns the response from the client or an error if the request fails.
	SendDeployCommand(ctx context.Context, projectID, environment string) (*DeployCommandResult, error)
}

// ProjectInfo holds information about a connected project.
type ProjectInfo struct {
	ProjectID   string
	ClientCount int
	MachineIDs  []string
}

// ProjectsProvider provides information about connected projects.
// This interface allows for dependency injection and easier testing.
type ProjectsProvider interface {
	// GetProjects returns a list of all connected projects with their details.
	GetProjects() []ProjectInfo
}

// RunCommandResult represents the result of a run command.
type RunCommandResult struct {
	TaskID    string
	Status    protocol.RunStatus
	Message   string
	Error     string
	ProjectID string
}

// LogsCommandResult represents the result of a logs command.
type LogsCommandResult struct {
	TaskID    string
	Entries   []protocol.LogEntry
	HasMore   bool
	Error     string
	ProjectID string
}

// CancelCommandResult represents the result of a cancel command.
type CancelCommandResult struct {
	TaskID    string
	Status    protocol.CancelStatus
	Message   string
	Error     string
	ProjectID string
}

// CreateTaskCommandResult represents the result of a create task command.
type CreateTaskCommandResult struct {
	TaskID        string
	Status        protocol.CreateTaskStatus
	Message       string
	Error         string
	ProjectID     string
	QueuePosition int
}

// DeployCommandResult represents the result of a deploy command.
type DeployCommandResult struct {
	ProjectID   string
	Status      protocol.DeployStatus
	Message     string
	Error       string
	Environment string
	Version     string
}

// AggregatedClientStatus represents the combined status of all connected OpenExec clients.
type AggregatedClientStatus struct {
	TotalClients   int
	Responded      int
	TimedOut       int
	OverallStatus  protocol.StatusCode
	FormattedText  string
	ClientStatuses []ClientStatusInfo
}

// ClientStatusInfo holds status information for a single client.
type ClientStatusInfo struct {
	ClientID  string
	ProjectID string
	MachineID string
	Status    protocol.StatusCode
	Message   string
	Error     string
}

// UserStore defines the interface for user persistence operations needed by commands.
// This is a subset of the user.Store interface focused on update operations.
type UserStore interface {
	// Update modifies an existing user.
	Update(ctx context.Context, u *user.User) error
}

// CommandHandler handles Telegram bot commands.
type CommandHandler struct {
	sender                   MessageSender
	fileSender               FileSender
	authMiddleware           *AuthMiddleware
	statusProvider           StatusProvider
	aggregatedStatusProvider AggregatedStatusProvider
	runCommandSender         RunCommandSender
	logsCommandSender        LogsCommandSender
	cancelCommandSender      CancelCommandSender
	createTaskCommandSender  CreateTaskCommandSender
	deployCommandSender      DeployCommandSender
	projectsProvider         ProjectsProvider
	logFormatter             *LogFormatter
	userStore                UserStore
	confirmationHandler      *ConfirmationHandler
	createTaskWizard         *CreateTaskWizard
	alertNotificationSender  *AlertNotificationSender
}

// NewCommandHandler creates a new command handler.
func NewCommandHandler(sender MessageSender, auth *AuthMiddleware) *CommandHandler {
	return &CommandHandler{
		sender:         sender,
		authMiddleware: auth,
		logFormatter:   NewLogFormatter(),
	}
}

// SetFileSender sets the file sender for sending document attachments.
func (h *CommandHandler) SetFileSender(sender FileSender) {
	h.fileSender = sender
}

// SetStatusProvider sets the status provider for the /status command.
func (h *CommandHandler) SetStatusProvider(provider StatusProvider) {
	h.statusProvider = provider
}

// SetAggregatedStatusProvider sets the aggregated status provider for the /clients command.
func (h *CommandHandler) SetAggregatedStatusProvider(provider AggregatedStatusProvider) {
	h.aggregatedStatusProvider = provider
}

// SetRunCommandSender sets the run command sender for the /run command.
func (h *CommandHandler) SetRunCommandSender(sender RunCommandSender) {
	h.runCommandSender = sender
}

// SetLogsCommandSender sets the logs command sender for the /logs command.
func (h *CommandHandler) SetLogsCommandSender(sender LogsCommandSender) {
	h.logsCommandSender = sender
}

// SetCancelCommandSender sets the cancel command sender for the /cancel command.
func (h *CommandHandler) SetCancelCommandSender(sender CancelCommandSender) {
	h.cancelCommandSender = sender
}

// SetCreateTaskCommandSender sets the create task command sender for the /create-task command.
func (h *CommandHandler) SetCreateTaskCommandSender(sender CreateTaskCommandSender) {
	h.createTaskCommandSender = sender
}

// SetDeployCommandSender sets the deploy command sender for the /deploy command.
func (h *CommandHandler) SetDeployCommandSender(sender DeployCommandSender) {
	h.deployCommandSender = sender
}

// SetProjectsProvider sets the projects provider for the /projects command.
func (h *CommandHandler) SetProjectsProvider(provider ProjectsProvider) {
	h.projectsProvider = provider
}

// SetUserStore sets the user store for updating user sessions.
func (h *CommandHandler) SetUserStore(store UserStore) {
	h.userStore = store
}

// SetConfirmationHandler sets the confirmation handler for interactive button flows.
func (h *CommandHandler) SetConfirmationHandler(handler *ConfirmationHandler) {
	h.confirmationHandler = handler
}

// GetConfirmationHandler returns the confirmation handler.
func (h *CommandHandler) GetConfirmationHandler() *ConfirmationHandler {
	return h.confirmationHandler
}

// SetCreateTaskWizard sets the create task wizard for interactive task creation.
func (h *CommandHandler) SetCreateTaskWizard(wizard *CreateTaskWizard) {
	h.createTaskWizard = wizard
}

// GetCreateTaskWizard returns the create task wizard.
func (h *CommandHandler) GetCreateTaskWizard() *CreateTaskWizard {
	return h.createTaskWizard
}

// SetAlertNotificationSender sets the alert notification sender for alert action buttons.
func (h *CommandHandler) SetAlertNotificationSender(sender *AlertNotificationSender) {
	h.alertNotificationSender = sender
}

// GetAlertNotificationSender returns the alert notification sender.
func (h *CommandHandler) GetAlertNotificationSender() *AlertNotificationSender {
	return h.alertNotificationSender
}

// HandleUpdate processes an incoming Telegram update and routes commands.
func (h *CommandHandler) HandleUpdate(ctx context.Context, update tgbotapi.Update) {
	// Handle callback queries (inline button presses)
	if update.CallbackQuery != nil {
		h.handleCallbackQuery(ctx, update)
		return
	}

	if update.Message == nil {
		return
	}

	// Handle text input for active wizard conversations
	if !update.Message.IsCommand() {
		h.handleWizardInput(ctx, update)
		return
	}

	switch update.Message.Command() {
	case "start":
		h.handleStart(ctx, update)
	case "status":
		h.handleStatus(ctx, update)
	case "clients":
		h.handleClients(ctx, update)
	case "projects":
		h.handleProjects(ctx, update)
	case "run":
		h.handleRun(ctx, update)
	case "logs":
		h.handleLogs(ctx, update)
	case "cancel":
		h.handleCancel(ctx, update)
	case "create_task":
		h.handleCreateTask(ctx, update)
	case "switch":
		h.handleSwitch(ctx, update)
	case "deploy":
		h.handleDeploy(ctx, update)
	default:
		// Unknown command - ignore
	}
}

// handleStatus processes the /status command.
// It returns the current system status for authorized users.
func (h *CommandHandler) handleStatus(ctx context.Context, update tgbotapi.Update) {
	chatID := update.Message.Chat.ID
	telegramUser := update.Message.From

	if telegramUser == nil {
		h.sendMessage(chatID, "Error: Unable to identify user.")
		return
	}

	// Check authorization
	result := h.authMiddleware.CheckUserID(ctx, telegramUser.ID)

	if !result.Allowed {
		h.sendMessage(chatID, "Access denied. You are not authorized to use this command.")
		return
	}

	// Check if status provider is configured
	if h.statusProvider == nil {
		h.sendMessage(chatID, "Status service is not available.")
		return
	}

	// Get status from provider
	status := h.statusProvider.GetStatus()
	if status == nil {
		h.sendMessage(chatID, "Unable to retrieve status information.")
		return
	}

	// Format status message
	statusMsg := formatStatusMessage(status)
	h.sendMessage(chatID, statusMsg)
}

// formatStatusMessage formats a StatusResponse into a user-friendly message.
func formatStatusMessage(status *protocol.StatusResponse) string {
	var sb strings.Builder

	// Status icon based on status code
	var icon string
	switch status.Status {
	case protocol.StatusOK:
		icon = "✅"
	case protocol.StatusDegraded:
		icon = "⚠️"
	case protocol.StatusError:
		icon = "❌"
	default:
		icon = "❓"
	}

	sb.WriteString(fmt.Sprintf("%s Gateway Status: %s\n", icon, strings.ToUpper(string(status.Status))))

	if status.Message != "" {
		sb.WriteString(fmt.Sprintf("\n%s\n", status.Message))
	}

	if status.Version != "" {
		sb.WriteString(fmt.Sprintf("\nVersion: %s", status.Version))
	}

	if status.Connections != nil {
		sb.WriteString(fmt.Sprintf("\n\nConnections: %d", status.Connections.TotalConnections))
		if status.Connections.AuthenticatedConnections > 0 {
			sb.WriteString(fmt.Sprintf(" (authenticated: %d)", status.Connections.AuthenticatedConnections))
		}
	}

	if status.Metrics != nil {
		sb.WriteString(fmt.Sprintf("\n\nUptime: %ds", status.Metrics.UptimeSeconds))
		if status.Metrics.MessagesReceived > 0 || status.Metrics.MessagesSent > 0 {
			sb.WriteString(fmt.Sprintf("\nMessages: %d received, %d sent",
				status.Metrics.MessagesReceived, status.Metrics.MessagesSent))
		}
	}

	return sb.String()
}

// handleClients processes the /clients command.
// It broadcasts a status request to all connected OpenExec clients and returns aggregated results.
func (h *CommandHandler) handleClients(ctx context.Context, update tgbotapi.Update) {
	chatID := update.Message.Chat.ID
	telegramUser := update.Message.From

	if telegramUser == nil {
		h.sendMessage(chatID, "Error: Unable to identify user.")
		return
	}

	// Check authorization
	result := h.authMiddleware.CheckUserID(ctx, telegramUser.ID)

	if !result.Allowed {
		h.sendMessage(chatID, "Access denied. You are not authorized to use this command.")
		return
	}

	// Check if aggregated status provider is configured
	if h.aggregatedStatusProvider == nil {
		h.sendMessage(chatID, "Client status service is not available.")
		return
	}

	// Send a "typing" indicator while waiting for responses
	h.sendMessage(chatID, "Querying connected clients...")

	// Broadcast status request and aggregate responses
	aggregated, err := h.aggregatedStatusProvider.BroadcastStatus(ctx, true)
	if err != nil {
		h.sendMessage(chatID, fmt.Sprintf("Error querying clients: %v", err))
		return
	}

	// Format and send the aggregated status
	statusMsg := formatAggregatedStatus(aggregated)
	h.sendMessage(chatID, statusMsg)
}

// handleProjects processes the /projects command.
// It returns a list of all projects with active connections.
func (h *CommandHandler) handleProjects(ctx context.Context, update tgbotapi.Update) {
	chatID := update.Message.Chat.ID
	telegramUser := update.Message.From

	if telegramUser == nil {
		h.sendMessage(chatID, "Error: Unable to identify user.")
		return
	}

	// Check authorization
	result := h.authMiddleware.CheckUserID(ctx, telegramUser.ID)

	if !result.Allowed {
		h.sendMessage(chatID, "Access denied. You are not authorized to use this command.")
		return
	}

	// Check if projects provider is configured
	if h.projectsProvider == nil {
		h.sendMessage(chatID, "Projects service is not available.")
		return
	}

	// Get projects from provider
	projects := h.projectsProvider.GetProjects()

	// Format and send the projects list
	projectsMsg := formatProjectsList(projects)
	h.sendMessage(chatID, projectsMsg)
}

// formatProjectsList formats a list of ProjectInfo into a user-friendly message.
func formatProjectsList(projects []ProjectInfo) string {
	if len(projects) == 0 {
		return "📋 No projects connected."
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📋 Connected Projects: %d\n\n", len(projects)))

	for _, project := range projects {
		sb.WriteString(fmt.Sprintf("📁 %s\n", project.ProjectID))
		sb.WriteString(fmt.Sprintf("   Connections: %d\n", project.ClientCount))

		if len(project.MachineIDs) > 0 {
			sb.WriteString("   Machines: ")
			for i, machineID := range project.MachineIDs {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(machineID)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	return strings.TrimSuffix(sb.String(), "\n")
}

// handleSwitch processes the /switch command.
// It updates the user's current project context.
// Usage: /switch <project-id>
// Use /switch without arguments to clear the current project.
func (h *CommandHandler) handleSwitch(ctx context.Context, update tgbotapi.Update) {
	chatID := update.Message.Chat.ID
	telegramUser := update.Message.From

	if telegramUser == nil {
		h.sendMessage(chatID, "Error: Unable to identify user.")
		return
	}

	// Check authorization
	result := h.authMiddleware.CheckUserID(ctx, telegramUser.ID)

	if !result.Allowed {
		h.sendMessage(chatID, "Access denied. You are not authorized to use this command.")
		return
	}

	// Check if user store is configured
	if h.userStore == nil {
		h.sendMessage(chatID, "Switch command is not available.")
		return
	}

	// Parse arguments: /switch [project-id]
	args := strings.TrimSpace(update.Message.CommandArguments())

	if args == "" {
		// Clear current project
		result.User.ClearCurrentProject()
		if err := h.userStore.Update(ctx, result.User); err != nil {
			h.sendMessage(chatID, fmt.Sprintf("❌ Error clearing project: %v", err))
			return
		}
		h.sendMessage(chatID, "🔄 Project context cleared.\n\nCommands will now target the default or first available project.")
		return
	}

	projectID := args

	// Validate that the project exists (if projects provider is available)
	if h.projectsProvider != nil {
		projects := h.projectsProvider.GetProjects()
		found := false
		for _, p := range projects {
			if p.ProjectID == projectID {
				found = true
				break
			}
		}
		if !found {
			h.sendMessage(chatID, fmt.Sprintf("❌ Project '%s' not found.\n\nUse /projects to see available projects.", projectID))
			return
		}
	}

	// Update the user's current project
	result.User.SetCurrentProject(projectID)
	if err := h.userStore.Update(ctx, result.User); err != nil {
		h.sendMessage(chatID, fmt.Sprintf("❌ Error switching project: %v", err))
		return
	}

	h.sendMessage(chatID, fmt.Sprintf("✅ Switched to project: %s\n\nCommands will now target this project by default.", projectID))
}

// handleRun processes the /run command.
// It sends a run request to execute a task on a connected OpenExec client.
// Usage: /run <task-id> [project-id]
// Requires 'executor' or 'admin' role.
func (h *CommandHandler) handleRun(ctx context.Context, update tgbotapi.Update) {
	chatID := update.Message.Chat.ID
	telegramUser := update.Message.From

	if telegramUser == nil {
		h.sendMessage(chatID, "Error: Unable to identify user.")
		return
	}

	// Check authorization
	result := h.authMiddleware.CheckUserID(ctx, telegramUser.ID)

	if !result.Allowed {
		h.sendMessage(chatID, "Access denied. You are not authorized to use this command.")
		return
	}

	// Check if user has permission to execute run commands (requires executor or admin role)
	if !result.User.CanExecute() {
		h.sendMessage(chatID, "Permission denied. The /run command requires 'executor' or 'admin' role.")
		return
	}

	// Check if run command sender is configured
	if h.runCommandSender == nil {
		h.sendMessage(chatID, "Run command service is not available.")
		return
	}

	// Parse arguments: /run <task-id> [project-id]
	args := strings.Fields(strings.TrimSpace(update.Message.CommandArguments()))
	if len(args) == 0 {
		h.sendMessage(chatID, "Usage: /run <task-id> [project-id]\n\nPlease provide a task ID to execute.\nOptionally specify a project ID to route to a specific project.")
		return
	}

	taskID := args[0]
	var projectID string
	if len(args) > 1 {
		projectID = args[1]
	}

	// Send a "processing" indicator
	if projectID != "" {
		h.sendMessage(chatID, fmt.Sprintf("🚀 Sending run request for task: %s to project: %s...", taskID, projectID))
	} else {
		h.sendMessage(chatID, fmt.Sprintf("🚀 Sending run request for task: %s...", taskID))
	}

	// Send the run command with optional project routing
	runResult, err := h.runCommandSender.SendRunCommand(ctx, taskID, projectID)
	if err != nil {
		h.sendMessage(chatID, fmt.Sprintf("❌ Error sending run command: %v", err))
		return
	}

	// Format and send the result
	resultMsg := formatRunResult(runResult)
	h.sendMessage(chatID, resultMsg)
}

// formatRunResult formats a RunCommandResult into a user-friendly message.
func formatRunResult(result *RunCommandResult) string {
	var sb strings.Builder

	// Status icon based on run status
	var icon string
	switch result.Status {
	case protocol.RunStatusAccepted:
		icon = "✅"
	case protocol.RunStatusRunning:
		icon = "⏳"
	case protocol.RunStatusCompleted:
		icon = "🎉"
	case protocol.RunStatusFailed:
		icon = "❌"
	case protocol.RunStatusRejected:
		icon = "🚫"
	default:
		icon = "❓"
	}

	sb.WriteString(fmt.Sprintf("%s Task: %s\n", icon, result.TaskID))
	sb.WriteString(fmt.Sprintf("Status: %s\n", strings.ToUpper(string(result.Status))))

	if result.ProjectID != "" {
		sb.WriteString(fmt.Sprintf("Project: %s\n", result.ProjectID))
	}

	if result.Message != "" {
		sb.WriteString(fmt.Sprintf("\n%s", result.Message))
	}

	if result.Error != "" {
		sb.WriteString(fmt.Sprintf("\nError: %s", result.Error))
	}

	return sb.String()
}

// handleLogs processes the /logs command.
// It retrieves logs for a task from a connected OpenExec client.
// Usage: /logs <task-id> [project-id]
// Requires 'executor' or 'admin' role.
func (h *CommandHandler) handleLogs(ctx context.Context, update tgbotapi.Update) {
	chatID := update.Message.Chat.ID
	telegramUser := update.Message.From

	if telegramUser == nil {
		h.sendMessage(chatID, "Error: Unable to identify user.")
		return
	}

	// Check authorization
	result := h.authMiddleware.CheckUserID(ctx, telegramUser.ID)

	if !result.Allowed {
		h.sendMessage(chatID, "Access denied. You are not authorized to use this command.")
		return
	}

	// Check if user has permission to view logs (requires executor or admin role)
	if !result.User.CanExecute() {
		h.sendMessage(chatID, "Permission denied. The /logs command requires 'executor' or 'admin' role.")
		return
	}

	// Check if logs command sender is configured
	if h.logsCommandSender == nil {
		h.sendMessage(chatID, "Logs command service is not available.")
		return
	}

	// Parse arguments: /logs <task-id> [project-id]
	args := strings.Fields(strings.TrimSpace(update.Message.CommandArguments()))
	if len(args) == 0 {
		h.sendMessage(chatID, "Usage: /logs <task-id> [project-id]\n\nPlease provide a task ID to retrieve logs.\nOptionally specify a project ID to route to a specific project.")
		return
	}

	taskID := args[0]
	var projectID string
	if len(args) > 1 {
		projectID = args[1]
	}

	// Send a "processing" indicator
	if projectID != "" {
		h.sendMessage(chatID, fmt.Sprintf("📋 Fetching logs for task: %s from project: %s...", taskID, projectID))
	} else {
		h.sendMessage(chatID, fmt.Sprintf("📋 Fetching logs for task: %s...", taskID))
	}

	// Send the logs command with optional project routing
	logsResult, err := h.logsCommandSender.SendLogsCommand(ctx, taskID, projectID)
	if err != nil {
		h.sendMessage(chatID, fmt.Sprintf("❌ Error fetching logs: %v", err))
		return
	}

	// Format the logs using the log formatter
	formatted := h.logFormatter.Format(logsResult)

	// Send based on output mode
	if formatted.Mode == LogOutputFile && h.fileSender != nil {
		// Send as file attachment
		_, err := h.fileSender.SendDocument(chatID, formatted.FileName, formatted.FileContent, formatted.TextContent)
		if err != nil {
			// Fallback to text-only summary if file send fails
			h.sendMessage(chatID, fmt.Sprintf("%s\n\n⚠️ (Failed to send log file: %v)", formatted.TextContent, err))
		}
	} else if formatted.Mode == LogOutputFile {
		// No file sender available, send text with notice
		h.sendMessage(chatID, fmt.Sprintf("%s\n\n⚠️ (File attachment not available, logs truncated)", formatted.TextContent))
	} else {
		// Send as text message
		h.sendMessage(chatID, formatted.TextContent)
	}
}

// handleCancel processes the /cancel command.
// It sends a cancel request to stop a running task on a connected OpenExec client.
// Usage: /cancel <task-id> [project-id]
// Requires 'executor' or 'admin' role.
func (h *CommandHandler) handleCancel(ctx context.Context, update tgbotapi.Update) {
	chatID := update.Message.Chat.ID
	telegramUser := update.Message.From

	if telegramUser == nil {
		h.sendMessage(chatID, "Error: Unable to identify user.")
		return
	}

	// Check authorization
	result := h.authMiddleware.CheckUserID(ctx, telegramUser.ID)

	if !result.Allowed {
		h.sendMessage(chatID, "Access denied. You are not authorized to use this command.")
		return
	}

	// Check if user has permission to cancel tasks (requires executor or admin role)
	if !result.User.CanExecute() {
		h.sendMessage(chatID, "Permission denied. The /cancel command requires 'executor' or 'admin' role.")
		return
	}

	// Check if cancel command sender is configured
	if h.cancelCommandSender == nil {
		h.sendMessage(chatID, "Cancel command service is not available.")
		return
	}

	// Parse arguments: /cancel <task-id> [project-id]
	args := strings.Fields(strings.TrimSpace(update.Message.CommandArguments()))
	if len(args) == 0 {
		h.sendMessage(chatID, "Usage: /cancel <task-id> [project-id]\n\nPlease provide a task ID to cancel.\nOptionally specify a project ID to route to a specific project.")
		return
	}

	taskID := args[0]
	var projectID string
	if len(args) > 1 {
		projectID = args[1]
	}

	// Send a "processing" indicator
	if projectID != "" {
		h.sendMessage(chatID, fmt.Sprintf("🛑 Sending cancel request for task: %s to project: %s...", taskID, projectID))
	} else {
		h.sendMessage(chatID, fmt.Sprintf("🛑 Sending cancel request for task: %s...", taskID))
	}

	// Send the cancel command with optional project routing
	cancelResult, err := h.cancelCommandSender.SendCancelCommand(ctx, taskID, projectID)
	if err != nil {
		h.sendMessage(chatID, fmt.Sprintf("❌ Error sending cancel command: %v", err))
		return
	}

	// Format and send the result
	resultMsg := formatCancelResult(cancelResult)
	h.sendMessage(chatID, resultMsg)
}

// formatCancelResult formats a CancelCommandResult into a user-friendly message.
func formatCancelResult(result *CancelCommandResult) string {
	var sb strings.Builder

	// Status icon based on cancel status
	var icon string
	switch result.Status {
	case protocol.CancelStatusAccepted:
		icon = "⏳"
	case protocol.CancelStatusCompleted:
		icon = "✅"
	case protocol.CancelStatusFailed:
		icon = "❌"
	case protocol.CancelStatusRejected:
		icon = "🚫"
	case protocol.CancelStatusNotFound:
		icon = "❓"
	default:
		icon = "❓"
	}

	sb.WriteString(fmt.Sprintf("%s Task: %s\n", icon, result.TaskID))
	sb.WriteString(fmt.Sprintf("Status: %s\n", strings.ToUpper(string(result.Status))))

	if result.ProjectID != "" {
		sb.WriteString(fmt.Sprintf("Project: %s\n", result.ProjectID))
	}

	if result.Message != "" {
		sb.WriteString(fmt.Sprintf("\n%s", result.Message))
	}

	if result.Error != "" {
		sb.WriteString(fmt.Sprintf("\nError: %s", result.Error))
	}

	return sb.String()
}

// handleCreateTask processes the /create_task command.
// It sends a create task request to queue a new task on a connected OpenExec client.
// Usage: /create_task <title> [project-id]
// The title is required; description is set to the title for simplicity.
// Requires 'executor' or 'admin' role.
func (h *CommandHandler) handleCreateTask(ctx context.Context, update tgbotapi.Update) {
	chatID := update.Message.Chat.ID
	telegramUser := update.Message.From

	if telegramUser == nil {
		h.sendMessage(chatID, "Error: Unable to identify user.")
		return
	}

	// Check authorization
	result := h.authMiddleware.CheckUserID(ctx, telegramUser.ID)

	if !result.Allowed {
		h.sendMessage(chatID, "Access denied. You are not authorized to use this command.")
		return
	}

	// Check if user has permission to create tasks (requires executor or admin role)
	if !result.User.CanExecute() {
		h.sendMessage(chatID, "Permission denied. The /create_task command requires 'executor' or 'admin' role.")
		return
	}

	// Check if create task command sender is configured
	if h.createTaskCommandSender == nil {
		h.sendMessage(chatID, "Create task service is not available.")
		return
	}

	// Parse arguments: /create_task <title> [project-id]
	args := strings.Fields(strings.TrimSpace(update.Message.CommandArguments()))
	if len(args) == 0 {
		// No arguments provided - start the interactive wizard
		if h.createTaskWizard != nil {
			if err := h.createTaskWizard.Start(ctx, telegramUser.ID, chatID); err != nil {
				h.sendMessage(chatID, fmt.Sprintf("❌ Failed to start task wizard: %v", err))
			}
			return
		}
		// Fall back to usage message if wizard is not available
		h.sendMessage(chatID, "Usage: /create_task <title> [project-id]\n\nPlease provide a task title.\nOptionally specify a project ID to route to a specific project.")
		return
	}

	// First argument is the title; if there are more, the last might be project-id
	// For simplicity, we'll treat all but the last as the title if multiple words
	title := args[0]
	var projectID string

	if len(args) > 1 {
		// Check if the last argument looks like a project ID (no spaces, alphanumeric with dashes)
		// For now, we'll treat the last argument as project ID if there are 2+ args
		title = strings.Join(args[:len(args)-1], " ")
		projectID = args[len(args)-1]
	}

	// Use title as description (minimal input for Telegram command)
	description := title

	// Send a "processing" indicator
	if projectID != "" {
		h.sendMessage(chatID, fmt.Sprintf("📝 Creating task: %s for project: %s...", title, projectID))
	} else {
		h.sendMessage(chatID, fmt.Sprintf("📝 Creating task: %s...", title))
	}

	// Send the create task command with optional project routing
	createResult, err := h.createTaskCommandSender.SendCreateTaskCommand(ctx, title, description, projectID)
	if err != nil {
		h.sendMessage(chatID, fmt.Sprintf("❌ Error creating task: %v", err))
		return
	}

	// Format and send the result
	resultMsg := formatCreateTaskResult(createResult)
	h.sendMessage(chatID, resultMsg)
}

// formatCreateTaskResult formats a CreateTaskCommandResult into a user-friendly message.
func formatCreateTaskResult(result *CreateTaskCommandResult) string {
	var sb strings.Builder

	// Status icon based on create task status
	var icon string
	switch result.Status {
	case protocol.CreateTaskStatusCreated:
		icon = "✅"
	case protocol.CreateTaskStatusQueued:
		icon = "📋"
	case protocol.CreateTaskStatusRejected:
		icon = "🚫"
	case protocol.CreateTaskStatusFailed:
		icon = "❌"
	default:
		icon = "❓"
	}

	sb.WriteString(fmt.Sprintf("%s Task Creation: %s\n", icon, strings.ToUpper(string(result.Status))))

	if result.TaskID != "" {
		sb.WriteString(fmt.Sprintf("Task ID: %s\n", result.TaskID))
	}

	if result.ProjectID != "" {
		sb.WriteString(fmt.Sprintf("Project: %s\n", result.ProjectID))
	}

	if result.QueuePosition > 0 {
		sb.WriteString(fmt.Sprintf("Queue Position: %d\n", result.QueuePosition))
	}

	if result.Message != "" {
		sb.WriteString(fmt.Sprintf("\n%s", result.Message))
	}

	if result.Error != "" {
		sb.WriteString(fmt.Sprintf("\nError: %s", result.Error))
	}

	return sb.String()
}

// handleDeploy processes the /deploy command.
// It sends a deploy request to deploy a project on a connected OpenExec client.
// Usage: /deploy [project-id] [environment]
// Requires 'executor' or 'admin' role.
func (h *CommandHandler) handleDeploy(ctx context.Context, update tgbotapi.Update) {
	chatID := update.Message.Chat.ID
	telegramUser := update.Message.From

	if telegramUser == nil {
		h.sendMessage(chatID, "Error: Unable to identify user.")
		return
	}

	// Check authorization
	result := h.authMiddleware.CheckUserID(ctx, telegramUser.ID)

	if !result.Allowed {
		h.sendMessage(chatID, "Access denied. You are not authorized to use this command.")
		return
	}

	// Check if user has permission to execute deploy commands (requires admin role only)
	if !result.User.CanDeploy() {
		h.sendMessage(chatID, "Permission denied. The /deploy command requires 'admin' role.")
		return
	}

	// Check if deploy command sender is configured
	if h.deployCommandSender == nil {
		h.sendMessage(chatID, "Deploy command service is not available.")
		return
	}

	// Parse arguments: /deploy [project-id] [environment]
	args := strings.Fields(strings.TrimSpace(update.Message.CommandArguments()))

	var projectID, environment string

	// Handle argument parsing based on count
	switch len(args) {
	case 0:
		// No arguments - use user's current project if set, or auto-resolve
		if result.User.CurrentProjectID != "" {
			projectID = result.User.CurrentProjectID
		}
	case 1:
		// One argument - could be project ID or environment
		// If it looks like an environment (staging, production, etc.), treat as environment
		// Otherwise, treat as project ID
		arg := args[0]
		if isEnvironmentName(arg) {
			environment = arg
			if result.User.CurrentProjectID != "" {
				projectID = result.User.CurrentProjectID
			}
		} else {
			projectID = arg
		}
	default:
		// Two or more arguments - first is project ID, second is environment
		projectID = args[0]
		environment = args[1]
	}

	// Send a "processing" indicator
	var processingMsg string
	if projectID != "" && environment != "" {
		processingMsg = fmt.Sprintf("🚀 Deploying project: %s to %s...", projectID, environment)
	} else if projectID != "" {
		processingMsg = fmt.Sprintf("🚀 Deploying project: %s...", projectID)
	} else if environment != "" {
		processingMsg = fmt.Sprintf("🚀 Deploying to %s...", environment)
	} else {
		processingMsg = "🚀 Starting deployment..."
	}
	h.sendMessage(chatID, processingMsg)

	// Send the deploy command with optional project routing
	deployResult, err := h.deployCommandSender.SendDeployCommand(ctx, projectID, environment)
	if err != nil {
		h.sendMessage(chatID, fmt.Sprintf("❌ Error sending deploy command: %v", err))
		return
	}

	// Format and send the result
	resultMsg := formatDeployResult(deployResult)
	h.sendMessage(chatID, resultMsg)
}

// isEnvironmentName checks if the given string looks like an environment name.
func isEnvironmentName(s string) bool {
	s = strings.ToLower(s)
	environments := []string{"staging", "production", "prod", "dev", "development", "test", "qa", "uat"}
	for _, env := range environments {
		if s == env {
			return true
		}
	}
	return false
}

// formatDeployResult formats a DeployCommandResult into a user-friendly message.
func formatDeployResult(result *DeployCommandResult) string {
	var sb strings.Builder

	// Status icon based on deploy status
	var icon string
	switch result.Status {
	case protocol.DeployStatusAccepted:
		icon = "✅"
	case protocol.DeployStatusRunning:
		icon = "⏳"
	case protocol.DeployStatusCompleted:
		icon = "🎉"
	case protocol.DeployStatusFailed:
		icon = "❌"
	case protocol.DeployStatusRejected:
		icon = "🚫"
	default:
		icon = "❓"
	}

	sb.WriteString(fmt.Sprintf("%s Deployment: %s\n", icon, strings.ToUpper(string(result.Status))))

	if result.ProjectID != "" {
		sb.WriteString(fmt.Sprintf("Project: %s\n", result.ProjectID))
	}

	if result.Environment != "" {
		sb.WriteString(fmt.Sprintf("Environment: %s\n", result.Environment))
	}

	if result.Version != "" {
		sb.WriteString(fmt.Sprintf("Version: %s\n", result.Version))
	}

	if result.Message != "" {
		sb.WriteString(fmt.Sprintf("\n%s", result.Message))
	}

	if result.Error != "" {
		sb.WriteString(fmt.Sprintf("\nError: %s", result.Error))
	}

	return sb.String()
}

// formatLogsResult formats a LogsCommandResult into a user-friendly message.
func formatLogsResult(result *LogsCommandResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📋 Logs for task: %s\n", result.TaskID))

	if result.ProjectID != "" {
		sb.WriteString(fmt.Sprintf("Project: %s\n", result.ProjectID))
	}

	if result.Error != "" {
		sb.WriteString(fmt.Sprintf("\n❌ Error: %s", result.Error))
		return sb.String()
	}

	if len(result.Entries) == 0 {
		sb.WriteString("\nNo log entries found.")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("\n📝 %d log entries:\n\n", len(result.Entries)))

	for _, entry := range result.Entries {
		// Log level icon
		var levelIcon string
		switch entry.Level {
		case protocol.LogLevelDebug:
			levelIcon = "🔍"
		case protocol.LogLevelInfo:
			levelIcon = "ℹ️"
		case protocol.LogLevelWarn:
			levelIcon = "⚠️"
		case protocol.LogLevelError:
			levelIcon = "❌"
		default:
			levelIcon = "📝"
		}

		// Format timestamp (just time portion for readability)
		timestamp := entry.Timestamp
		if len(timestamp) > 19 {
			timestamp = timestamp[11:19] // Extract HH:MM:SS
		}

		sb.WriteString(fmt.Sprintf("%s [%s] %s\n", levelIcon, timestamp, entry.Message))
	}

	if result.HasMore {
		sb.WriteString("\n... (more entries available)")
	}

	return sb.String()
}

// formatAggregatedStatus formats an AggregatedClientStatus into a user-friendly message.
func formatAggregatedStatus(status *AggregatedClientStatus) string {
	if status.TotalClients == 0 {
		return "No OpenExec clients connected."
	}

	var sb strings.Builder

	// Overall status icon
	var icon string
	switch status.OverallStatus {
	case protocol.StatusOK:
		icon = "✅"
	case protocol.StatusDegraded:
		icon = "⚠️"
	case protocol.StatusError:
		icon = "❌"
	default:
		icon = "❓"
	}

	sb.WriteString(fmt.Sprintf("%s Overall Status: %s\n\n", icon, strings.ToUpper(string(status.OverallStatus))))

	// Summary
	sb.WriteString(fmt.Sprintf("📊 Clients: %d total | %d responded | %d timed out\n\n",
		status.TotalClients, status.Responded, status.TimedOut))

	// Per-client details
	sb.WriteString("📋 Client Details:\n")
	for _, client := range status.ClientStatuses {
		var clientIcon string
		switch client.Status {
		case protocol.StatusOK:
			clientIcon = "✅"
		case protocol.StatusDegraded:
			clientIcon = "⚠️"
		case protocol.StatusError:
			clientIcon = "❌"
		default:
			clientIcon = "❓"
		}

		// Build client line
		clientLine := fmt.Sprintf("  %s ", clientIcon)

		if client.ProjectID != "" {
			clientLine += fmt.Sprintf("%s", client.ProjectID)
		} else {
			clientLine += fmt.Sprintf("%s", truncateID(client.ClientID))
		}

		if client.MachineID != "" {
			clientLine += fmt.Sprintf(" [%s]", client.MachineID)
		}

		if client.Error != "" {
			clientLine += fmt.Sprintf(": %s", client.Error)
		} else if client.Message != "" {
			clientLine += fmt.Sprintf(": %s", client.Message)
		} else {
			clientLine += fmt.Sprintf(": %s", client.Status)
		}

		sb.WriteString(clientLine + "\n")
	}

	return sb.String()
}

// truncateID shortens a UUID-style ID for display.
func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8] + "..."
	}
	return id
}

// handleStart processes the /start command.
// It welcomes authorized users or rejects unauthorized access.
func (h *CommandHandler) handleStart(ctx context.Context, update tgbotapi.Update) {
	chatID := update.Message.Chat.ID
	telegramUser := update.Message.From

	if telegramUser == nil {
		h.sendMessage(chatID, "Error: Unable to identify user.")
		return
	}

	// Check authorization
	result := h.authMiddleware.CheckUserID(ctx, telegramUser.ID)

	if result.Allowed {
		// User is authorized - send welcome message
		userName := getUserDisplayName(telegramUser)
		welcomeMsg := fmt.Sprintf("Welcome, %s! You are authorized to use this bot.\n\nYour role: %s", userName, result.User.Role)
		h.sendMessage(chatID, welcomeMsg)
	} else {
		// User is not authorized - reject access
		rejectMsg := "Access denied. You are not authorized to use this bot.\n\nPlease contact an administrator if you believe this is an error."
		h.sendMessage(chatID, rejectMsg)
	}
}

// sendMessage sends a text message to the given chat.
func (h *CommandHandler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.sender.Send(msg); err != nil {
		logging.Warn("Failed to send message", "chat_id", chatID, "error", err)
	}
}

// getUserDisplayName returns a display name for the Telegram user.
// It prefers FirstName + LastName, falls back to Username, then FirstName only.
func getUserDisplayName(user *tgbotapi.User) string {
	if user == nil {
		return "User"
	}

	// Try full name first
	fullName := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if fullName != "" {
		return fullName
	}

	// Fall back to username
	if user.UserName != "" {
		return "@" + user.UserName
	}

	// Last resort
	return "User"
}

// handleCallbackQuery processes callback queries from inline button presses.
func (h *CommandHandler) handleCallbackQuery(ctx context.Context, update tgbotapi.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}

	// Check authorization
	if query.From == nil {
		h.answerCallbackQuery(query.ID, "Error: Unable to identify user.")
		return
	}

	result := h.authMiddleware.CheckUserID(ctx, query.From.ID)
	if !result.Allowed {
		h.answerCallbackQuery(query.ID, "Access denied.")
		return
	}

	// Try to handle with create task wizard
	if h.createTaskWizard != nil && IsWizardCallback(query.Data) {
		_, err := h.createTaskWizard.HandleCallback(ctx, query.From.ID, query.Data)
		if err != nil {
			logging.Warn("Wizard callback handler error", "error", err)
		}
		h.answerCallbackQuery(query.ID, "")
		return
	}

	// Try to handle with alert notification sender
	if h.alertNotificationSender != nil && IsAlertCallback(query.Data) {
		handled, err := h.alertNotificationSender.HandleCallbackQuery(ctx, query)
		if handled {
			if err != nil {
				logging.Warn("Alert callback handler error", "error", err)
			}
			return
		}
	}

	// Try to handle with confirmation handler
	if h.confirmationHandler != nil {
		handled, err := h.confirmationHandler.HandleCallbackQuery(ctx, query)
		if handled {
			if err != nil {
				logging.Warn("Confirmation handler error", "error", err)
			}
			return
		}
	}

	// Unknown callback query - acknowledge without message
	h.answerCallbackQuery(query.ID, "")
}

// handleWizardInput processes text input for active wizard conversations.
func (h *CommandHandler) handleWizardInput(ctx context.Context, update tgbotapi.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}

	// Check if user has an active wizard conversation
	if h.createTaskWizard == nil || !h.createTaskWizard.HasActiveWizard(update.Message.From.ID) {
		return
	}

	// Process the text input through the wizard
	input := conversation.NewTextInput(update.Message.Text)
	_, err := h.createTaskWizard.HandleInput(ctx, update.Message.From.ID, input)
	if err != nil {
		logging.Warn("Wizard input handler error", "error", err)
	}
}

// answerCallbackQuery answers a callback query with optional text.
func (h *CommandHandler) answerCallbackQuery(callbackID, text string) {
	callback := tgbotapi.NewCallback(callbackID, text)
	if _, err := h.sender.Send(callback); err != nil {
		logging.Warn("Failed to answer callback query", "error", err)
	}
}

// SendConfirmation sends a confirmation message with Yes/No buttons.
// This is a convenience method that delegates to the confirmation handler.
func (h *CommandHandler) SendConfirmation(
	chatID int64,
	userID int64,
	action ConfirmationAction,
	message string,
	data map[string]string,
	onConfirm func(ctx context.Context) error,
	onDecline func(ctx context.Context) error,
) (string, error) {
	if h.confirmationHandler == nil {
		return "", fmt.Errorf("confirmation handler not configured")
	}
	return h.confirmationHandler.SendConfirmation(chatID, userID, action, message, data, onConfirm, onDecline)
}
