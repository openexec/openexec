package telegram

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/openexec/openexec/internal/conversation"
	"github.com/openexec/openexec/internal/protocol"
)

// mockCreateTaskSender is a mock implementation of CreateTaskCommandSender for testing.
type mockCreateTaskSender struct {
	lastTitle       string
	lastDescription string
	lastProjectID   string
	returnResult    *CreateTaskCommandResult
	returnError     error
}

func (m *mockCreateTaskSender) SendCreateTaskCommand(ctx context.Context, title, description, projectID string) (*CreateTaskCommandResult, error) {
	m.lastTitle = title
	m.lastDescription = description
	m.lastProjectID = projectID
	if m.returnError != nil {
		return nil, m.returnError
	}
	if m.returnResult != nil {
		return m.returnResult, nil
	}
	return &CreateTaskCommandResult{
		TaskID:    "task-123",
		Status:    protocol.CreateTaskStatusCreated,
		Message:   "Task created successfully",
		ProjectID: projectID,
	}, nil
}

// wizardMockSender tracks all messages sent during wizard tests.
type wizardMockSender struct {
	sentMessages []tgbotapi.Chattable
}

func newWizardMockSender() *wizardMockSender {
	return &wizardMockSender{
		sentMessages: make([]tgbotapi.Chattable, 0),
	}
}

func (m *wizardMockSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.sentMessages = append(m.sentMessages, c)
	return tgbotapi.Message{MessageID: len(m.sentMessages)}, nil
}

func (m *wizardMockSender) lastMessage() *tgbotapi.MessageConfig {
	if len(m.sentMessages) == 0 {
		return nil
	}
	msg, ok := m.sentMessages[len(m.sentMessages)-1].(tgbotapi.MessageConfig)
	if !ok {
		return nil
	}
	return &msg
}

func (m *wizardMockSender) messageCount() int {
	return len(m.sentMessages)
}

func TestNewCreateTaskWizard(t *testing.T) {
	sender := newWizardMockSender()
	convManager := conversation.NewManager()
	defer convManager.Stop()
	taskSender := &mockCreateTaskSender{}
	projectsProvider := &mockProjectsProvider{
		projects: []ProjectInfo{
			{ProjectID: "proj-1", ClientCount: 1},
		},
	}

	wizard := NewCreateTaskWizard(sender, convManager, taskSender, projectsProvider)

	if wizard == nil {
		t.Fatal("Expected non-nil wizard")
	}

	// Verify the flow was registered
	flow, ok := convManager.GetFlow(FlowIDCreateTask)
	if !ok {
		t.Fatal("Expected create-task flow to be registered")
	}
	if flow.StartState != StateTitle {
		t.Errorf("Expected start state %s, got %s", StateTitle, flow.StartState)
	}
}

func TestCreateTaskWizard_Start(t *testing.T) {
	sender := newWizardMockSender()
	convManager := conversation.NewManager()
	defer convManager.Stop()
	taskSender := &mockCreateTaskSender{}
	projectsProvider := &mockProjectsProvider{}

	wizard := NewCreateTaskWizard(sender, convManager, taskSender, projectsProvider)

	ctx := context.Background()
	err := wizard.Start(ctx, 12345, 67890)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify the welcome message was sent
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "Create New Task") {
		t.Errorf("Expected welcome message to contain 'Create New Task', got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "title") {
		t.Errorf("Expected welcome message to mention title, got: %s", msg.Text)
	}

	// Verify wizard is active
	if !wizard.HasActiveWizard(12345) {
		t.Error("Expected wizard to be active for user")
	}
}

func TestCreateTaskWizard_TitleState(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectNext    bool
		expectMessage string
	}{
		{
			name:       "valid title",
			input:      "Test Task Title",
			expectNext: true,
		},
		{
			name:          "empty title",
			input:         "",
			expectNext:    false,
			expectMessage: "cannot be empty",
		},
		{
			name:          "whitespace only",
			input:         "   ",
			expectNext:    false,
			expectMessage: "cannot be empty",
		},
		{
			name:          "title too long",
			input:         strings.Repeat("a", 201),
			expectNext:    false,
			expectMessage: "too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := newWizardMockSender()
			convManager := conversation.NewManager()
			defer convManager.Stop()
			taskSender := &mockCreateTaskSender{}
			projectsProvider := &mockProjectsProvider{
				projects: []ProjectInfo{{ProjectID: "proj-1"}},
			}

			wizard := NewCreateTaskWizard(sender, convManager, taskSender, projectsProvider)

			ctx := context.Background()
			_ = wizard.Start(ctx, 12345, 67890)

			// Send title input
			input := conversation.NewTextInput(tt.input)
			result, err := wizard.HandleInput(ctx, 12345, input)
			if err != nil && tt.expectNext {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tt.expectNext {
				// Should transition to project state
				conv, _ := convManager.GetUserConversation(12345)
				if conv.CurrentState != StateProject {
					t.Errorf("Expected state %s, got %s", StateProject, conv.CurrentState)
				}

				// Verify title was stored
				title := conv.GetString(DataKeyTitle)
				if title != tt.input {
					t.Errorf("Expected title '%s', got '%s'", tt.input, title)
				}
			} else {
				// Should stay in title state
				if result != nil && result.NextState != "" && result.NextState != StateTitle {
					t.Errorf("Expected to stay in title state, got transition to %s", result.NextState)
				}

				// Check error message
				msg := sender.lastMessage()
				if msg != nil && !strings.Contains(msg.Text, tt.expectMessage) {
					t.Errorf("Expected message containing '%s', got: %s", tt.expectMessage, msg.Text)
				}
			}
		})
	}
}

func TestCreateTaskWizard_ProjectState(t *testing.T) {
	sender := newWizardMockSender()
	convManager := conversation.NewManager()
	defer convManager.Stop()
	taskSender := &mockCreateTaskSender{}
	projectsProvider := &mockProjectsProvider{
		projects: []ProjectInfo{
			{ProjectID: "proj-1", ClientCount: 2},
			{ProjectID: "proj-2", ClientCount: 1},
		},
	}

	wizard := NewCreateTaskWizard(sender, convManager, taskSender, projectsProvider)

	ctx := context.Background()
	_ = wizard.Start(ctx, 12345, 67890)

	// Enter title
	titleInput := conversation.NewTextInput("Test Task")
	_, _ = wizard.HandleInput(ctx, 12345, titleInput)

	// Now select project via callback
	callbackData := WizardCallbackData{
		Action: WizardActionProject,
		Value:  "proj-1",
	}
	dataJSON, _ := json.Marshal(callbackData)

	result, err := wizard.HandleCallback(ctx, 12345, string(dataJSON))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should transition to priority state
	conv, _ := convManager.GetUserConversation(12345)
	if conv.CurrentState != StatePriority {
		t.Errorf("Expected state %s, got %s", StatePriority, conv.CurrentState)
	}

	// Verify project was stored
	project := conv.GetString(DataKeyProject)
	if project != "proj-1" {
		t.Errorf("Expected project 'proj-1', got '%s'", project)
	}

	// Verify no error in result
	if result != nil && result.Error != nil {
		t.Errorf("Unexpected error in result: %v", result.Error)
	}
}

func TestCreateTaskWizard_PriorityState(t *testing.T) {
	priorities := []struct {
		value    string
		expected protocol.TaskPriority
	}{
		{"low", protocol.TaskPriorityLow},
		{"normal", protocol.TaskPriorityNormal},
		{"high", protocol.TaskPriorityHigh},
		{"critical", protocol.TaskPriorityCritical},
	}

	for _, p := range priorities {
		t.Run(p.value, func(t *testing.T) {
			sender := newWizardMockSender()
			convManager := conversation.NewManager()
			defer convManager.Stop()
			taskSender := &mockCreateTaskSender{}
			projectsProvider := &mockProjectsProvider{
				projects: []ProjectInfo{{ProjectID: "proj-1"}},
			}

			wizard := NewCreateTaskWizard(sender, convManager, taskSender, projectsProvider)

			ctx := context.Background()
			_ = wizard.Start(ctx, 12345, 67890)

			// Enter title
			_, _ = wizard.HandleInput(ctx, 12345, conversation.NewTextInput("Test Task"))

			// Select project
			projData := WizardCallbackData{Action: WizardActionProject, Value: "proj-1"}
			projJSON, _ := json.Marshal(projData)
			_, _ = wizard.HandleCallback(ctx, 12345, string(projJSON))

			// Select priority via callback
			prioData := WizardCallbackData{Action: WizardActionPriority, Value: p.value}
			prioJSON, _ := json.Marshal(prioData)
			_, err := wizard.HandleCallback(ctx, 12345, string(prioJSON))
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Should transition to confirm state
			conv, _ := convManager.GetUserConversation(12345)
			if conv.CurrentState != StateConfirm {
				t.Errorf("Expected state %s, got %s", StateConfirm, conv.CurrentState)
			}

			// Verify priority was stored
			priority := conv.GetString(DataKeyPriority)
			if priority != p.value {
				t.Errorf("Expected priority '%s', got '%s'", p.value, priority)
			}
		})
	}
}

func TestCreateTaskWizard_ConfirmState_Yes(t *testing.T) {
	sender := newWizardMockSender()
	convManager := conversation.NewManager()
	defer convManager.Stop()
	taskSender := &mockCreateTaskSender{
		returnResult: &CreateTaskCommandResult{
			TaskID:    "task-456",
			Status:    protocol.CreateTaskStatusCreated,
			Message:   "Task created",
			ProjectID: "proj-1",
		},
	}
	projectsProvider := &mockProjectsProvider{
		projects: []ProjectInfo{{ProjectID: "proj-1"}},
	}

	wizard := NewCreateTaskWizard(sender, convManager, taskSender, projectsProvider)

	ctx := context.Background()
	_ = wizard.Start(ctx, 12345, 67890)

	// Complete wizard steps
	_, _ = wizard.HandleInput(ctx, 12345, conversation.NewTextInput("My Test Task"))

	projData := WizardCallbackData{Action: WizardActionProject, Value: "proj-1"}
	projJSON, _ := json.Marshal(projData)
	_, _ = wizard.HandleCallback(ctx, 12345, string(projJSON))

	prioData := WizardCallbackData{Action: WizardActionPriority, Value: "high"}
	prioJSON, _ := json.Marshal(prioData)
	_, _ = wizard.HandleCallback(ctx, 12345, string(prioJSON))

	// Confirm
	confirmData := WizardCallbackData{Action: WizardActionConfirm, Value: "yes"}
	confirmJSON, _ := json.Marshal(confirmData)
	result, err := wizard.HandleCallback(ctx, 12345, string(confirmJSON))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should complete
	if result != nil && !result.Complete {
		t.Error("Expected conversation to complete")
	}

	// Verify task was created with correct data
	if taskSender.lastTitle != "My Test Task" {
		t.Errorf("Expected title 'My Test Task', got '%s'", taskSender.lastTitle)
	}
	if taskSender.lastProjectID != "proj-1" {
		t.Errorf("Expected project 'proj-1', got '%s'", taskSender.lastProjectID)
	}

	// Wizard should no longer be active
	if wizard.HasActiveWizard(12345) {
		t.Error("Expected wizard to be inactive after completion")
	}
}

func TestCreateTaskWizard_ConfirmState_No(t *testing.T) {
	sender := newWizardMockSender()
	convManager := conversation.NewManager()
	defer convManager.Stop()
	taskSender := &mockCreateTaskSender{}
	projectsProvider := &mockProjectsProvider{
		projects: []ProjectInfo{{ProjectID: "proj-1"}},
	}

	wizard := NewCreateTaskWizard(sender, convManager, taskSender, projectsProvider)

	ctx := context.Background()
	_ = wizard.Start(ctx, 12345, 67890)

	// Complete wizard steps
	_, _ = wizard.HandleInput(ctx, 12345, conversation.NewTextInput("My Test Task"))

	projData := WizardCallbackData{Action: WizardActionProject, Value: "proj-1"}
	projJSON, _ := json.Marshal(projData)
	_, _ = wizard.HandleCallback(ctx, 12345, string(projJSON))

	prioData := WizardCallbackData{Action: WizardActionPriority, Value: "normal"}
	prioJSON, _ := json.Marshal(prioData)
	_, _ = wizard.HandleCallback(ctx, 12345, string(prioJSON))

	// Decline
	confirmData := WizardCallbackData{Action: WizardActionConfirm, Value: "no"}
	confirmJSON, _ := json.Marshal(confirmData)
	result, err := wizard.HandleCallback(ctx, 12345, string(confirmJSON))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should cancel
	if result != nil && !result.Cancel {
		t.Error("Expected conversation to be cancelled")
	}

	// Wizard should no longer be active
	if wizard.HasActiveWizard(12345) {
		t.Error("Expected wizard to be inactive after cancellation")
	}
}

func TestCreateTaskWizard_Cancel(t *testing.T) {
	sender := newWizardMockSender()
	convManager := conversation.NewManager()
	defer convManager.Stop()
	taskSender := &mockCreateTaskSender{}
	projectsProvider := &mockProjectsProvider{
		projects: []ProjectInfo{{ProjectID: "proj-1"}},
	}

	wizard := NewCreateTaskWizard(sender, convManager, taskSender, projectsProvider)

	ctx := context.Background()
	_ = wizard.Start(ctx, 12345, 67890)

	// Send cancel callback
	cancelData := WizardCallbackData{Action: WizardActionCancel}
	cancelJSON, _ := json.Marshal(cancelData)
	result, err := wizard.HandleCallback(ctx, 12345, string(cancelJSON))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should cancel
	if result != nil && !result.Cancel {
		t.Error("Expected conversation to be cancelled")
	}

	// Verify cancellation message was sent
	found := false
	for _, m := range sender.sentMessages {
		if msg, ok := m.(tgbotapi.MessageConfig); ok {
			if strings.Contains(msg.Text, "cancelled") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("Expected cancellation message to be sent")
	}
}

func TestIsWizardCallback(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected bool
	}{
		{
			name:     "project action",
			data:     `{"a":"wiz_proj","v":"proj-1"}`,
			expected: true,
		},
		{
			name:     "priority action",
			data:     `{"a":"wiz_prio","v":"high"}`,
			expected: true,
		},
		{
			name:     "confirm action",
			data:     `{"a":"wiz_conf","v":"yes"}`,
			expected: true,
		},
		{
			name:     "cancel action",
			data:     `{"a":"wiz_cancel","v":""}`,
			expected: true,
		},
		{
			name:     "confirmation handler action",
			data:     `{"a":"deploy","r":"yes","id":"123"}`,
			expected: false,
		},
		{
			name:     "invalid json",
			data:     "not json",
			expected: false,
		},
		{
			name:     "unknown action",
			data:     `{"a":"unknown","v":"test"}`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsWizardCallback(tt.data)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for data: %s", tt.expected, result, tt.data)
			}
		})
	}
}

func TestCreateTaskWizard_TextPriorityInput(t *testing.T) {
	sender := newWizardMockSender()
	convManager := conversation.NewManager()
	defer convManager.Stop()
	taskSender := &mockCreateTaskSender{}
	projectsProvider := &mockProjectsProvider{
		projects: []ProjectInfo{{ProjectID: "proj-1"}},
	}

	wizard := NewCreateTaskWizard(sender, convManager, taskSender, projectsProvider)

	ctx := context.Background()
	_ = wizard.Start(ctx, 12345, 67890)

	// Enter title
	_, _ = wizard.HandleInput(ctx, 12345, conversation.NewTextInput("Test Task"))

	// Select project
	projData := WizardCallbackData{Action: WizardActionProject, Value: "proj-1"}
	projJSON, _ := json.Marshal(projData)
	_, _ = wizard.HandleCallback(ctx, 12345, string(projJSON))

	// Enter priority via text (instead of callback)
	_, err := wizard.HandleInput(ctx, 12345, conversation.NewTextInput("high"))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should transition to confirm state
	conv, _ := convManager.GetUserConversation(12345)
	if conv.CurrentState != StateConfirm {
		t.Errorf("Expected state %s, got %s", StateConfirm, conv.CurrentState)
	}

	// Verify priority was stored
	priority := conv.GetString(DataKeyPriority)
	if priority != "high" {
		t.Errorf("Expected priority 'high', got '%s'", priority)
	}
}

func TestCreateTaskWizard_InvalidPriorityText(t *testing.T) {
	sender := newWizardMockSender()
	convManager := conversation.NewManager()
	defer convManager.Stop()
	taskSender := &mockCreateTaskSender{}
	projectsProvider := &mockProjectsProvider{
		projects: []ProjectInfo{{ProjectID: "proj-1"}},
	}

	wizard := NewCreateTaskWizard(sender, convManager, taskSender, projectsProvider)

	ctx := context.Background()
	_ = wizard.Start(ctx, 12345, 67890)

	// Enter title and project
	_, _ = wizard.HandleInput(ctx, 12345, conversation.NewTextInput("Test Task"))
	projData := WizardCallbackData{Action: WizardActionProject, Value: "proj-1"}
	projJSON, _ := json.Marshal(projData)
	_, _ = wizard.HandleCallback(ctx, 12345, string(projJSON))

	// Enter invalid priority
	_, err := wizard.HandleInput(ctx, 12345, conversation.NewTextInput("invalid"))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should stay in priority state
	conv, _ := convManager.GetUserConversation(12345)
	if conv.CurrentState != StatePriority {
		t.Errorf("Expected to stay in state %s, got %s", StatePriority, conv.CurrentState)
	}

	// Check error message was sent
	msg := sender.lastMessage()
	if msg == nil || !strings.Contains(msg.Text, "Invalid priority") {
		t.Error("Expected error message about invalid priority")
	}
}

func TestCreateTaskWizard_MultipleProjects(t *testing.T) {
	sender := newWizardMockSender()
	convManager := conversation.NewManager()
	defer convManager.Stop()
	taskSender := &mockCreateTaskSender{}
	projectsProvider := &mockProjectsProvider{
		projects: []ProjectInfo{
			{ProjectID: "proj-1", ClientCount: 2},
			{ProjectID: "proj-2", ClientCount: 1},
			{ProjectID: "proj-3", ClientCount: 3},
		},
	}

	wizard := NewCreateTaskWizard(sender, convManager, taskSender, projectsProvider)

	ctx := context.Background()
	_ = wizard.Start(ctx, 12345, 67890)

	// Enter title to trigger project selection
	_, _ = wizard.HandleInput(ctx, 12345, conversation.NewTextInput("Test Task"))

	// The last message should have the project selection keyboard
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected project selection message")
	}

	if !strings.Contains(msg.Text, "Select Project") {
		t.Errorf("Expected 'Select Project' in message, got: %s", msg.Text)
	}

	// Verify inline keyboard was attached
	if msg.ReplyMarkup == nil {
		t.Error("Expected inline keyboard to be attached")
	}
}

func TestCreateTaskWizard_FullFlow(t *testing.T) {
	sender := newWizardMockSender()
	convManager := conversation.NewManager()
	defer convManager.Stop()
	taskSender := &mockCreateTaskSender{
		returnResult: &CreateTaskCommandResult{
			TaskID:        "task-789",
			Status:        protocol.CreateTaskStatusQueued,
			Message:       "Task queued for execution",
			ProjectID:     "my-project",
			QueuePosition: 3,
		},
	}
	projectsProvider := &mockProjectsProvider{
		projects: []ProjectInfo{{ProjectID: "my-project", ClientCount: 1}},
	}

	wizard := NewCreateTaskWizard(sender, convManager, taskSender, projectsProvider)

	ctx := context.Background()

	// Start wizard
	err := wizard.Start(ctx, 12345, 67890)
	if err != nil {
		t.Fatalf("Failed to start wizard: %v", err)
	}

	// Step 1: Enter title
	_, _ = wizard.HandleInput(ctx, 12345, conversation.NewTextInput("Deploy new feature"))

	// Step 2: Select project
	projData := WizardCallbackData{Action: WizardActionProject, Value: "my-project"}
	projJSON, _ := json.Marshal(projData)
	_, _ = wizard.HandleCallback(ctx, 12345, string(projJSON))

	// Step 3: Select priority
	prioData := WizardCallbackData{Action: WizardActionPriority, Value: "critical"}
	prioJSON, _ := json.Marshal(prioData)
	_, _ = wizard.HandleCallback(ctx, 12345, string(prioJSON))

	// Verify confirmation message shows collected data
	msg := sender.lastMessage()
	if msg == nil {
		t.Fatal("Expected confirmation message")
	}
	if !strings.Contains(msg.Text, "Deploy new feature") {
		t.Errorf("Expected title in confirmation, got: %s", msg.Text)
	}
	if !strings.Contains(msg.Text, "my-project") {
		t.Errorf("Expected project in confirmation, got: %s", msg.Text)
	}
	if !strings.Contains(strings.ToLower(msg.Text), "critical") {
		t.Errorf("Expected priority in confirmation, got: %s", msg.Text)
	}

	// Step 4: Confirm
	confirmData := WizardCallbackData{Action: WizardActionConfirm, Value: "yes"}
	confirmJSON, _ := json.Marshal(confirmData)
	_, err = wizard.HandleCallback(ctx, 12345, string(confirmJSON))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify task creation was called with correct parameters
	if taskSender.lastTitle != "Deploy new feature" {
		t.Errorf("Expected title 'Deploy new feature', got '%s'", taskSender.lastTitle)
	}
	if taskSender.lastProjectID != "my-project" {
		t.Errorf("Expected project 'my-project', got '%s'", taskSender.lastProjectID)
	}

	// Verify result message was sent
	found := false
	for _, m := range sender.sentMessages {
		if msg, ok := m.(tgbotapi.MessageConfig); ok {
			if strings.Contains(msg.Text, "task-789") && strings.Contains(msg.Text, "QUEUED") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("Expected success message with task ID and status")
	}
}
