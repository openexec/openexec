// Package telegram provides Telegram bot integration for the message gateway.
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/openexec/openexec/internal/conversation"
	"github.com/openexec/openexec/internal/protocol"
)

// Create task wizard flow and state identifiers.
const (
	// FlowIDCreateTask is the flow identifier for the create task wizard.
	FlowIDCreateTask conversation.FlowID = "create-task"

	// StateTitle is the state where the user enters the task title.
	StateTitle conversation.StateID = "title"
	// StateProject is the state where the user selects the project.
	StateProject conversation.StateID = "project"
	// StatePriority is the state where the user selects the priority.
	StatePriority conversation.StateID = "priority"
	// StateConfirm is the state where the user confirms the task creation.
	StateConfirm conversation.StateID = "confirm"
)

// Wizard data keys for storing collected information.
const (
	DataKeyTitle       = "title"
	DataKeyDescription = "description"
	DataKeyProject     = "project"
	DataKeyPriority    = "priority"
)

// WizardCallbackAction represents the action type for wizard callbacks.
type WizardCallbackAction string

const (
	// WizardActionProject is used for project selection callbacks.
	WizardActionProject WizardCallbackAction = "wiz_proj"
	// WizardActionPriority is used for priority selection callbacks.
	WizardActionPriority WizardCallbackAction = "wiz_prio"
	// WizardActionConfirm is used for confirmation callbacks.
	WizardActionConfirm WizardCallbackAction = "wiz_conf"
	// WizardActionCancel is used for cancel callbacks.
	WizardActionCancel WizardCallbackAction = "wiz_cancel"
)

// WizardCallbackData holds the data encoded in wizard callback button presses.
type WizardCallbackData struct {
	Action WizardCallbackAction `json:"a"`
	Value  string               `json:"v"`
}

// CreateTaskWizard manages the interactive create task flow.
type CreateTaskWizard struct {
	sender                  MessageSender
	conversationManager     *conversation.Manager
	createTaskCommandSender CreateTaskCommandSender
	projectsProvider        ProjectsProvider
}

// NewCreateTaskWizard creates a new create task wizard.
func NewCreateTaskWizard(
	sender MessageSender,
	convManager *conversation.Manager,
	createTaskSender CreateTaskCommandSender,
	projectsProvider ProjectsProvider,
) *CreateTaskWizard {
	w := &CreateTaskWizard{
		sender:                  sender,
		conversationManager:     convManager,
		createTaskCommandSender: createTaskSender,
		projectsProvider:        projectsProvider,
	}

	// Register the flow with the conversation manager
	flow := w.buildFlow()
	if err := convManager.RegisterFlow(flow); err != nil {
		// This should not happen if the flow is correctly defined
		panic(fmt.Sprintf("failed to register create task flow: %v", err))
	}

	return w
}

// buildFlow constructs the create task wizard flow configuration.
func (w *CreateTaskWizard) buildFlow() *conversation.FlowConfig {
	return conversation.NewFlow(FlowIDCreateTask).
		StartState(StateTitle).
		DefaultTimeout(300). // 5 minutes
		State(StateTitle).
		Handler(w.handleTitleState).
		AllowTransitions(StateProject).
		End().
		State(StateProject).
		Handler(w.handleProjectState).
		AllowTransitions(StatePriority, StateTitle).
		End().
		State(StatePriority).
		Handler(w.handlePriorityState).
		AllowTransitions(StateConfirm, StateProject).
		End().
		State(StateConfirm).
		Handler(w.handleConfirmState).
		AllowTransitions(StateTitle).
		End().
		OnCancel(func(ctx context.Context, conv *conversation.Conversation) error {
			w.sendMessage(conv.ChatID, "❌ Task creation cancelled.")
			return nil
		}).
		OnTimeout(func(ctx context.Context, conv *conversation.Conversation) error {
			w.sendMessage(conv.ChatID, "⏰ Task creation timed out. Please start again with /create_task")
			return nil
		}).
		MustBuild()
}

// Start begins a new create task wizard conversation.
func (w *CreateTaskWizard) Start(ctx context.Context, userID, chatID int64) error {
	conv, err := w.conversationManager.StartConversation(ctx, FlowIDCreateTask, userID, chatID)
	if err != nil {
		return fmt.Errorf("failed to start create task wizard: %w", err)
	}

	// Send the initial prompt
	w.sendMessage(chatID, "📝 *Create New Task*\n\nPlease enter the task title:")

	// Store conversation ID for tracking
	conv.SetData("started", true)
	return nil
}

// HandleInput processes user input for an active wizard conversation.
func (w *CreateTaskWizard) HandleInput(ctx context.Context, userID int64, input *conversation.Input) (*conversation.TransitionResult, error) {
	return w.conversationManager.ProcessInput(ctx, userID, input)
}

// HandleCallback processes callback button presses for the wizard.
func (w *CreateTaskWizard) HandleCallback(ctx context.Context, userID int64, callbackData string) (*conversation.TransitionResult, error) {
	// Parse the callback data
	var data WizardCallbackData
	if err := json.Unmarshal([]byte(callbackData), &data); err != nil {
		return nil, fmt.Errorf("invalid callback data: %w", err)
	}

	// Check for cancel action
	if data.Action == WizardActionCancel {
		if err := w.conversationManager.CancelUserConversation(ctx, userID); err != nil {
			return nil, err
		}
		return conversation.Cancelled(), nil
	}

	// Create callback input and process
	input := conversation.NewCallbackInput(callbackData)
	return w.conversationManager.ProcessInput(ctx, userID, input)
}

// HasActiveWizard checks if the user has an active create task wizard.
func (w *CreateTaskWizard) HasActiveWizard(userID int64) bool {
	conv, err := w.conversationManager.GetUserConversation(userID)
	if err != nil {
		return false
	}
	return conv.FlowID == FlowIDCreateTask
}

// handleTitleState handles input in the title state.
func (w *CreateTaskWizard) handleTitleState(ctx context.Context, conv *conversation.Conversation, input *conversation.Input) *conversation.TransitionResult {
	// Handle cancel command
	if input.Type == conversation.InputTypeCommand && strings.ToLower(input.Text) == "/cancel" {
		return conversation.Cancelled()
	}

	// Validate title
	title := strings.TrimSpace(input.Text)
	if title == "" {
		w.sendMessage(conv.ChatID, "❌ Title cannot be empty. Please enter a task title:")
		return conversation.Stay()
	}

	if len(title) > 200 {
		w.sendMessage(conv.ChatID, "❌ Title is too long (max 200 characters). Please enter a shorter title:")
		return conversation.Stay()
	}

	// Store the title and transition to project selection
	conv.SetData(DataKeyTitle, title)
	conv.SetData(DataKeyDescription, title) // Use title as description for simplicity

	// Send project selection message
	w.sendProjectSelection(conv.ChatID)

	return conversation.GoTo(StateProject)
}

// handleProjectState handles input in the project selection state.
func (w *CreateTaskWizard) handleProjectState(ctx context.Context, conv *conversation.Conversation, input *conversation.Input) *conversation.TransitionResult {
	// Handle cancel command
	if input.Type == conversation.InputTypeCommand && strings.ToLower(input.Text) == "/cancel" {
		return conversation.Cancelled()
	}

	var projectID string

	if input.Type == conversation.InputTypeCallback {
		// Parse callback data
		var data WizardCallbackData
		if err := json.Unmarshal([]byte(input.CallbackData), &data); err != nil {
			w.sendMessage(conv.ChatID, "❌ Invalid selection. Please try again.")
			return conversation.Stay()
		}

		if data.Action != WizardActionProject {
			w.sendMessage(conv.ChatID, "❌ Invalid selection. Please select a project.")
			return conversation.Stay()
		}

		projectID = data.Value
	} else if input.Type == conversation.InputTypeText {
		// Allow manual project ID entry
		projectID = strings.TrimSpace(input.Text)
	}

	// Validate project ID
	if projectID == "" {
		w.sendMessage(conv.ChatID, "❌ Please select a project from the list or enter a project ID.")
		return conversation.Stay()
	}

	// Store the project and transition to priority selection
	conv.SetData(DataKeyProject, projectID)

	// Send priority selection message
	w.sendPrioritySelection(conv.ChatID)

	return conversation.GoTo(StatePriority)
}

// handlePriorityState handles input in the priority selection state.
func (w *CreateTaskWizard) handlePriorityState(ctx context.Context, conv *conversation.Conversation, input *conversation.Input) *conversation.TransitionResult {
	// Handle cancel command
	if input.Type == conversation.InputTypeCommand && strings.ToLower(input.Text) == "/cancel" {
		return conversation.Cancelled()
	}

	var priority protocol.TaskPriority

	if input.Type == conversation.InputTypeCallback {
		// Parse callback data
		var data WizardCallbackData
		if err := json.Unmarshal([]byte(input.CallbackData), &data); err != nil {
			w.sendMessage(conv.ChatID, "❌ Invalid selection. Please try again.")
			return conversation.Stay()
		}

		if data.Action != WizardActionPriority {
			w.sendMessage(conv.ChatID, "❌ Invalid selection. Please select a priority.")
			return conversation.Stay()
		}

		priority = protocol.TaskPriority(data.Value)
	} else if input.Type == conversation.InputTypeText {
		// Allow manual priority entry
		p := strings.ToLower(strings.TrimSpace(input.Text))
		switch p {
		case "low":
			priority = protocol.TaskPriorityLow
		case "normal":
			priority = protocol.TaskPriorityNormal
		case "high":
			priority = protocol.TaskPriorityHigh
		case "critical":
			priority = protocol.TaskPriorityCritical
		default:
			w.sendMessage(conv.ChatID, "❌ Invalid priority. Please select: low, normal, high, or critical")
			return conversation.Stay()
		}
	}

	// Validate priority
	if priority == "" {
		w.sendMessage(conv.ChatID, "❌ Please select a priority level.")
		return conversation.Stay()
	}

	// Store the priority and transition to confirmation
	conv.SetData(DataKeyPriority, string(priority))

	// Send confirmation message
	w.sendConfirmation(conv)

	return conversation.GoTo(StateConfirm)
}

// handleConfirmState handles input in the confirmation state.
func (w *CreateTaskWizard) handleConfirmState(ctx context.Context, conv *conversation.Conversation, input *conversation.Input) *conversation.TransitionResult {
	// Handle cancel command
	if input.Type == conversation.InputTypeCommand && strings.ToLower(input.Text) == "/cancel" {
		return conversation.Cancelled()
	}

	var confirmed bool

	if input.Type == conversation.InputTypeCallback {
		// Parse callback data
		var data WizardCallbackData
		if err := json.Unmarshal([]byte(input.CallbackData), &data); err != nil {
			w.sendMessage(conv.ChatID, "❌ Invalid selection. Please try again.")
			return conversation.Stay()
		}

		if data.Action != WizardActionConfirm {
			w.sendMessage(conv.ChatID, "❌ Invalid selection. Please confirm or cancel.")
			return conversation.Stay()
		}

		confirmed = data.Value == "yes"
	} else if input.Type == conversation.InputTypeText {
		// Allow text confirmation
		text := strings.ToLower(strings.TrimSpace(input.Text))
		switch text {
		case "yes", "y", "confirm":
			confirmed = true
		case "no", "n", "cancel":
			confirmed = false
		default:
			w.sendMessage(conv.ChatID, "❌ Please type 'yes' to confirm or 'no' to cancel.")
			return conversation.Stay()
		}
	}

	if !confirmed {
		return conversation.Cancelled()
	}

	// Get collected data
	title := conv.GetString(DataKeyTitle)
	description := conv.GetString(DataKeyDescription)
	projectID := conv.GetString(DataKeyProject)

	// Create the task
	w.sendMessage(conv.ChatID, "⏳ Creating task...")

	result, err := w.createTaskCommandSender.SendCreateTaskCommand(ctx, title, description, projectID)
	if err != nil {
		w.sendMessage(conv.ChatID, fmt.Sprintf("❌ Failed to create task: %v", err))
		return conversation.DoneWithMessage("")
	}

	// Format and send the result
	resultMsg := w.formatCreateTaskResult(result)
	w.sendMessage(conv.ChatID, resultMsg)

	return conversation.Done()
}

// sendProjectSelection sends the project selection message with inline buttons.
func (w *CreateTaskWizard) sendProjectSelection(chatID int64) {
	var buttons [][]tgbotapi.InlineKeyboardButton

	// Get available projects
	if w.projectsProvider != nil {
		projects := w.projectsProvider.GetProjects()
		for _, p := range projects {
			data := WizardCallbackData{
				Action: WizardActionProject,
				Value:  p.ProjectID,
			}
			dataJSON, _ := json.Marshal(data)

			label := p.ProjectID
			if p.ClientCount > 1 {
				label = fmt.Sprintf("%s (%d clients)", p.ProjectID, p.ClientCount)
			}

			buttons = append(buttons, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(label, string(dataJSON)),
			))
		}
	}

	// Add cancel button
	cancelData := WizardCallbackData{Action: WizardActionCancel}
	cancelJSON, _ := json.Marshal(cancelData)
	buttons = append(buttons, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Cancel", string(cancelJSON)),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	msg := tgbotapi.NewMessage(chatID, "📁 *Select Project*\n\nChoose the project for this task:")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	_, _ = w.sender.Send(msg)
}

// sendPrioritySelection sends the priority selection message with inline buttons.
func (w *CreateTaskWizard) sendPrioritySelection(chatID int64) {
	priorities := []struct {
		label    string
		priority protocol.TaskPriority
	}{
		{"🟢 Low", protocol.TaskPriorityLow},
		{"🟡 Normal", protocol.TaskPriorityNormal},
		{"🟠 High", protocol.TaskPriorityHigh},
		{"🔴 Critical", protocol.TaskPriorityCritical},
	}

	var buttons [][]tgbotapi.InlineKeyboardButton
	for _, p := range priorities {
		data := WizardCallbackData{
			Action: WizardActionPriority,
			Value:  string(p.priority),
		}
		dataJSON, _ := json.Marshal(data)
		buttons = append(buttons, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(p.label, string(dataJSON)),
		))
	}

	// Add cancel button
	cancelData := WizardCallbackData{Action: WizardActionCancel}
	cancelJSON, _ := json.Marshal(cancelData)
	buttons = append(buttons, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Cancel", string(cancelJSON)),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)

	msg := tgbotapi.NewMessage(chatID, "⚡ *Select Priority*\n\nChoose the priority level for this task:")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	_, _ = w.sender.Send(msg)
}

// sendConfirmation sends the confirmation message with collected data.
func (w *CreateTaskWizard) sendConfirmation(conv *conversation.Conversation) {
	title := conv.GetString(DataKeyTitle)
	project := conv.GetString(DataKeyProject)
	priority := conv.GetString(DataKeyPriority)

	// Format priority for display
	priorityDisplay := strings.Title(priority)

	text := fmt.Sprintf(`✅ *Confirm Task Creation*

*Title:* %s
*Project:* %s
*Priority:* %s

Do you want to create this task?`, title, project, priorityDisplay)

	// Create confirmation buttons
	yesData := WizardCallbackData{Action: WizardActionConfirm, Value: "yes"}
	noData := WizardCallbackData{Action: WizardActionConfirm, Value: "no"}
	yesJSON, _ := json.Marshal(yesData)
	noJSON, _ := json.Marshal(noData)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Yes, Create", string(yesJSON)),
			tgbotapi.NewInlineKeyboardButtonData("❌ No, Cancel", string(noJSON)),
		),
	)

	msg := tgbotapi.NewMessage(conv.ChatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	_, _ = w.sender.Send(msg)
}

// formatCreateTaskResult formats the task creation result for display.
func (w *CreateTaskWizard) formatCreateTaskResult(result *CreateTaskCommandResult) string {
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

	sb.WriteString(fmt.Sprintf("%s *Task Creation: %s*\n", icon, strings.ToUpper(string(result.Status))))

	if result.TaskID != "" {
		sb.WriteString(fmt.Sprintf("Task ID: `%s`\n", result.TaskID))
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
		sb.WriteString(fmt.Sprintf("\n⚠️ Error: %s", result.Error))
	}

	return sb.String()
}

// sendMessage sends a text message to a chat.
func (w *CreateTaskWizard) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	_, _ = w.sender.Send(msg)
}

// IsWizardCallback checks if a callback data string is for the wizard.
func IsWizardCallback(callbackData string) bool {
	var data WizardCallbackData
	if err := json.Unmarshal([]byte(callbackData), &data); err != nil {
		return false
	}
	switch data.Action {
	case WizardActionProject, WizardActionPriority, WizardActionConfirm, WizardActionCancel:
		return true
	default:
		return false
	}
}
