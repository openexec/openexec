package telegram

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/openexec/openexec/internal/protocol"
)

// mockAlertSender is a mock implementation of MessageSender for alert testing.
type mockAlertSender struct {
	mu           sync.Mutex
	sentMessages []tgbotapi.Chattable
	callbacks    []tgbotapi.CallbackConfig
	lastMsgID    int
}

func newMockAlertSender() *mockAlertSender {
	return &mockAlertSender{
		sentMessages: make([]tgbotapi.Chattable, 0),
		callbacks:    make([]tgbotapi.CallbackConfig, 0),
		lastMsgID:    0,
	}
}

func (m *mockAlertSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sentMessages = append(m.sentMessages, c)

	// Check if it's a callback config
	if cb, ok := c.(tgbotapi.CallbackConfig); ok {
		m.callbacks = append(m.callbacks, cb)
		return tgbotapi.Message{}, nil
	}

	// For regular messages, increment message ID
	m.lastMsgID++
	return tgbotapi.Message{MessageID: m.lastMsgID}, nil
}

func (m *mockAlertSender) getLastMessage() *tgbotapi.MessageConfig {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := len(m.sentMessages) - 1; i >= 0; i-- {
		if msg, ok := m.sentMessages[i].(tgbotapi.MessageConfig); ok {
			return &msg
		}
	}
	return nil
}

func (m *mockAlertSender) getLastCallback() *tgbotapi.CallbackConfig {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.callbacks) == 0 {
		return nil
	}
	return &m.callbacks[len(m.callbacks)-1]
}

func (m *mockAlertSender) getMessageCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sentMessages)
}

func TestNewAlertNotificationSender(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)

	if alertSender == nil {
		t.Fatal("Expected alert notification sender to be created")
	}

	if alertSender.GetPendingCount() != 0 {
		t.Errorf("Expected 0 pending actions, got %d", alertSender.GetPendingCount())
	}
}

func TestSendAlert(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)

	event := protocol.NewLongRunningAlertEvent(
		"alert-123",
		"task-456",
		"my-project",
		300.0,
		180.0,
	)

	alertID, err := alertSender.SendAlert(
		12345, // chatID
		67890, // userID
		event,
	)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if alertID == "" {
		t.Fatal("Expected non-empty alert ID")
	}

	if alertSender.GetPendingCount() != 1 {
		t.Errorf("Expected 1 pending action, got %d", alertSender.GetPendingCount())
	}

	// Verify message was sent
	msg := sender.getLastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if msg.ChatID != 12345 {
		t.Errorf("Expected chat ID 12345, got %d", msg.ChatID)
	}

	// Verify message contains alert info
	if !strings.Contains(msg.Text, "task-456") {
		t.Errorf("Expected task ID in message, got: %s", msg.Text)
	}

	if !strings.Contains(msg.Text, "my-project") {
		t.Errorf("Expected project ID in message, got: %s", msg.Text)
	}

	// Verify inline keyboard is present
	if msg.ReplyMarkup == nil {
		t.Fatal("Expected reply markup to be set")
	}

	keyboard, ok := msg.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup)
	if !ok {
		t.Fatal("Expected InlineKeyboardMarkup")
	}

	// Should have 2 rows: Cancel+Logs on first, Dismiss on second
	if len(keyboard.InlineKeyboard) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(keyboard.InlineKeyboard))
	}

	// Verify first row has Cancel and Logs buttons
	if len(keyboard.InlineKeyboard[0]) != 2 {
		t.Fatalf("Expected 2 buttons in first row, got %d", len(keyboard.InlineKeyboard[0]))
	}

	cancelBtn := keyboard.InlineKeyboard[0][0]
	logsBtn := keyboard.InlineKeyboard[0][1]

	if !strings.Contains(cancelBtn.Text, "Cancel") {
		t.Errorf("Expected Cancel button, got: %s", cancelBtn.Text)
	}

	if !strings.Contains(logsBtn.Text, "Logs") {
		t.Errorf("Expected Logs button, got: %s", logsBtn.Text)
	}

	// Verify second row has Dismiss button
	if len(keyboard.InlineKeyboard[1]) != 1 {
		t.Fatalf("Expected 1 button in second row, got %d", len(keyboard.InlineKeyboard[1]))
	}

	dismissBtn := keyboard.InlineKeyboard[1][0]
	if !strings.Contains(dismissBtn.Text, "Dismiss") {
		t.Errorf("Expected Dismiss button, got: %s", dismissBtn.Text)
	}

	// Verify callback data
	var cancelData AlertCallbackData
	if err := json.Unmarshal([]byte(*cancelBtn.CallbackData), &cancelData); err != nil {
		t.Fatalf("Failed to parse cancel callback data: %v", err)
	}

	if cancelData.Action != AlertCallbackCancel {
		t.Errorf("Expected cancel action, got: %s", cancelData.Action)
	}
	if cancelData.TaskID != "task-456" {
		t.Errorf("Expected task ID task-456, got: %s", cancelData.TaskID)
	}
	if cancelData.ProjectID != "my-project" {
		t.Errorf("Expected project ID my-project, got: %s", cancelData.ProjectID)
	}
}

func TestSendAlertNilEvent(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)

	_, err := alertSender.SendAlert(12345, 67890, nil)

	if err == nil {
		t.Error("Expected error for nil event")
	}
}

func TestSendAlertWithoutTaskID(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)

	event := protocol.NewAlertEvent(
		"alert-no-task",
		protocol.AlertTypeResourceLimit,
		protocol.AlertSeverityWarning,
		"Resource Warning",
	)
	// No TaskID - should still work but only have Dismiss button

	alertID, err := alertSender.SendAlert(12345, 67890, event)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if alertID == "" {
		t.Fatal("Expected non-empty alert ID")
	}

	// Verify keyboard only has Dismiss button
	msg := sender.getLastMessage()
	keyboard, ok := msg.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup)
	if !ok {
		t.Fatal("Expected InlineKeyboardMarkup")
	}

	// Should have 1 row with just Dismiss
	if len(keyboard.InlineKeyboard) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(keyboard.InlineKeyboard))
	}

	if len(keyboard.InlineKeyboard[0]) != 1 {
		t.Fatalf("Expected 1 button, got %d", len(keyboard.InlineKeyboard[0]))
	}

	if !strings.Contains(keyboard.InlineKeyboard[0][0].Text, "Dismiss") {
		t.Errorf("Expected Dismiss button, got: %s", keyboard.InlineKeyboard[0][0].Text)
	}
}

func TestAlertHandleCallbackQueryCancel(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)

	cancelCalled := false
	alertSender.SetCancelHandler(func(ctx context.Context, action AlertCallbackAction, taskID, projectID string) error {
		cancelCalled = true
		if taskID != "task-123" {
			t.Errorf("Expected task ID task-123, got: %s", taskID)
		}
		if projectID != "my-project" {
			t.Errorf("Expected project ID my-project, got: %s", projectID)
		}
		return nil
	})

	event := protocol.NewLongRunningAlertEvent(
		"alert-cancel-test",
		"task-123",
		"my-project",
		300.0,
		180.0,
	)

	alertID, err := alertSender.SendAlert(12345, 67890, event)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Create callback query for Cancel button
	cancelData := AlertCallbackData{
		Action:    AlertCallbackCancel,
		TaskID:    "task-123",
		ProjectID: "my-project",
		AlertID:   alertID,
	}
	cancelJSON, _ := json.Marshal(cancelData)

	query := &tgbotapi.CallbackQuery{
		ID:   "callback123",
		From: &tgbotapi.User{ID: 67890},
		Data: string(cancelJSON),
	}

	handled, err := alertSender.HandleCallbackQuery(context.Background(), query)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !handled {
		t.Error("Expected callback to be handled")
	}

	if !cancelCalled {
		t.Error("Expected cancel handler to be called")
	}
}

func TestAlertHandleCallbackQueryLogs(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)

	logsCalled := false
	alertSender.SetLogsHandler(func(ctx context.Context, action AlertCallbackAction, taskID, projectID string) error {
		logsCalled = true
		if taskID != "task-456" {
			t.Errorf("Expected task ID task-456, got: %s", taskID)
		}
		return nil
	})

	event := protocol.NewLongRunningAlertEvent(
		"alert-logs-test",
		"task-456",
		"project-x",
		300.0,
		180.0,
	)

	alertID, err := alertSender.SendAlert(12345, 67890, event)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Create callback query for Logs button
	logsData := AlertCallbackData{
		Action:    AlertCallbackLogs,
		TaskID:    "task-456",
		ProjectID: "project-x",
		AlertID:   alertID,
	}
	logsJSON, _ := json.Marshal(logsData)

	query := &tgbotapi.CallbackQuery{
		ID:   "callback456",
		From: &tgbotapi.User{ID: 67890},
		Data: string(logsJSON),
	}

	handled, err := alertSender.HandleCallbackQuery(context.Background(), query)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !handled {
		t.Error("Expected callback to be handled")
	}

	if !logsCalled {
		t.Error("Expected logs handler to be called")
	}
}

func TestAlertHandleCallbackQueryDismiss(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)

	event := protocol.NewLongRunningAlertEvent(
		"alert-dismiss-test",
		"task-789",
		"project-y",
		300.0,
		180.0,
	)

	alertID, err := alertSender.SendAlert(12345, 67890, event)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if alertSender.GetPendingCount() != 1 {
		t.Errorf("Expected 1 pending action before dismiss, got %d", alertSender.GetPendingCount())
	}

	// Create callback query for Dismiss button
	dismissData := AlertCallbackData{
		Action:  AlertCallbackDismiss,
		AlertID: alertID,
	}
	dismissJSON, _ := json.Marshal(dismissData)

	query := &tgbotapi.CallbackQuery{
		ID:   "callback789",
		From: &tgbotapi.User{ID: 67890},
		Data: string(dismissJSON),
	}

	handled, err := alertSender.HandleCallbackQuery(context.Background(), query)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !handled {
		t.Error("Expected callback to be handled")
	}

	// Pending should be removed after dismiss
	if alertSender.GetPendingCount() != 0 {
		t.Errorf("Expected 0 pending actions after dismiss, got %d", alertSender.GetPendingCount())
	}
}

func TestAlertHandleCallbackQueryExpired(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)
	alertSender.SetExpiryTTL(1 * time.Millisecond) // Very short TTL

	event := protocol.NewLongRunningAlertEvent(
		"alert-expired",
		"task-expired",
		"project-expired",
		300.0,
		180.0,
	)

	alertID, err := alertSender.SendAlert(12345, 67890, event)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Wait for expiry
	time.Sleep(10 * time.Millisecond)

	// Manually clear (since cleanup runs on interval)
	alertSender.ClearPending()

	// Create callback for expired alert
	cancelData := AlertCallbackData{
		Action:  AlertCallbackCancel,
		TaskID:  "task-expired",
		AlertID: alertID,
	}
	cancelJSON, _ := json.Marshal(cancelData)

	query := &tgbotapi.CallbackQuery{
		ID:   "callback_expired",
		From: &tgbotapi.User{ID: 67890},
		Data: string(cancelJSON),
	}

	handled, err := alertSender.HandleCallbackQuery(context.Background(), query)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !handled {
		t.Error("Expected callback to be handled (even if expired)")
	}
}

func TestAlertHandleCallbackQueryInvalidData(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)

	// Create callback with invalid JSON
	query := &tgbotapi.CallbackQuery{
		ID:   "callback_invalid",
		From: &tgbotapi.User{ID: 67890},
		Data: "not valid json",
	}

	handled, err := alertSender.HandleCallbackQuery(context.Background(), query)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if handled {
		t.Error("Expected callback NOT to be handled (invalid data)")
	}
}

func TestAlertHandleCallbackQueryNil(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)

	handled, err := alertSender.HandleCallbackQuery(context.Background(), nil)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if handled {
		t.Error("Expected nil callback NOT to be handled")
	}
}

func TestAlertHandleCallbackQueryEmptyData(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)

	query := &tgbotapi.CallbackQuery{
		ID:   "callback_empty",
		From: &tgbotapi.User{ID: 67890},
		Data: "",
	}

	handled, err := alertSender.HandleCallbackQuery(context.Background(), query)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if handled {
		t.Error("Expected empty data callback NOT to be handled")
	}
}

func TestIsAlertCallback(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected bool
	}{
		{
			name:     "cancel callback",
			data:     `{"a":"alert_cancel","t":"task-1","id":"alert-1"}`,
			expected: true,
		},
		{
			name:     "logs callback",
			data:     `{"a":"alert_logs","t":"task-1","id":"alert-1"}`,
			expected: true,
		},
		{
			name:     "dismiss callback",
			data:     `{"a":"alert_dismiss","id":"alert-1"}`,
			expected: true,
		},
		{
			name:     "confirmation callback",
			data:     `{"a":"deploy","r":"yes","id":"confirm-1"}`,
			expected: false,
		},
		{
			name:     "invalid JSON",
			data:     "not json",
			expected: false,
		},
		{
			name:     "empty string",
			data:     "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAlertCallback(tt.data)
			if result != tt.expected {
				t.Errorf("IsAlertCallback(%q) = %v, want %v", tt.data, result, tt.expected)
			}
		})
	}
}

func TestParseAlertCallbackData(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
		expected  *AlertCallbackData
	}{
		{
			name:      "valid cancel data",
			input:     `{"a":"alert_cancel","t":"task-123","p":"proj-456","id":"alert-789"}`,
			wantError: false,
			expected: &AlertCallbackData{
				Action:    AlertCallbackCancel,
				TaskID:    "task-123",
				ProjectID: "proj-456",
				AlertID:   "alert-789",
			},
		},
		{
			name:      "valid logs data",
			input:     `{"a":"alert_logs","t":"task-abc","id":"alert-xyz"}`,
			wantError: false,
			expected: &AlertCallbackData{
				Action:  AlertCallbackLogs,
				TaskID:  "task-abc",
				AlertID: "alert-xyz",
			},
		},
		{
			name:      "valid dismiss data",
			input:     `{"a":"alert_dismiss","id":"alert-123"}`,
			wantError: false,
			expected: &AlertCallbackData{
				Action:  AlertCallbackDismiss,
				AlertID: "alert-123",
			},
		},
		{
			name:      "empty string",
			input:     "",
			wantError: true,
			expected:  nil,
		},
		{
			name:      "invalid JSON",
			input:     "not json",
			wantError: true,
			expected:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseAlertCallbackData(tt.input)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.Action != tt.expected.Action {
				t.Errorf("Expected action %s, got %s", tt.expected.Action, result.Action)
			}
			if result.TaskID != tt.expected.TaskID {
				t.Errorf("Expected task ID %s, got %s", tt.expected.TaskID, result.TaskID)
			}
			if result.ProjectID != tt.expected.ProjectID {
				t.Errorf("Expected project ID %s, got %s", tt.expected.ProjectID, result.ProjectID)
			}
			if result.AlertID != tt.expected.AlertID {
				t.Errorf("Expected alert ID %s, got %s", tt.expected.AlertID, result.AlertID)
			}
		})
	}
}

func TestAlertCallbackConstants(t *testing.T) {
	// Verify alert callback actions
	if AlertCallbackCancel != "alert_cancel" {
		t.Errorf("Expected 'alert_cancel', got '%s'", AlertCallbackCancel)
	}
	if AlertCallbackLogs != "alert_logs" {
		t.Errorf("Expected 'alert_logs', got '%s'", AlertCallbackLogs)
	}
	if AlertCallbackDismiss != "alert_dismiss" {
		t.Errorf("Expected 'alert_dismiss', got '%s'", AlertCallbackDismiss)
	}
}

func TestAlertSetExpiryTTL(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)

	alertSender.SetExpiryTTL(10 * time.Minute)

	event := protocol.NewLongRunningAlertEvent(
		"alert-ttl-test",
		"task-ttl",
		"project-ttl",
		300.0,
		180.0,
	)

	alertID, _ := alertSender.SendAlert(12345, 67890, event)

	pending := alertSender.GetPendingAction(alertID)
	if pending == nil {
		t.Fatal("Expected pending action")
	}

	// Expiry should be approximately 10 minutes from now
	expectedExpiry := time.Now().Add(10 * time.Minute)
	if pending.ExpiresAt.Before(expectedExpiry.Add(-1*time.Second)) ||
		pending.ExpiresAt.After(expectedExpiry.Add(1*time.Second)) {
		t.Errorf("Expiry time mismatch: expected around %v, got %v", expectedExpiry, pending.ExpiresAt)
	}
}

func TestSetDebug(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)

	// Should not panic
	alertSender.SetDebug(true)
	alertSender.SetDebug(false)
}

func TestSetFormatter(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)

	customFormatter := NewAlertFormatter()
	customFormatter.SetIncludeEmojis(false)

	alertSender.SetFormatter(customFormatter)

	// Send an alert and verify it uses the custom formatter
	event := protocol.NewLongRunningAlertEvent(
		"alert-formatter-test",
		"task-fmt",
		"project-fmt",
		300.0,
		180.0,
	)

	_, err := alertSender.SendAlert(12345, 67890, event)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	msg := sender.getLastMessage()
	// Without emojis, should not contain emoji characters
	if strings.Contains(msg.Text, EmojiLongRunning) {
		t.Error("Expected no emojis with custom formatter")
	}
}

func TestSetFormatterNil(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)

	// Setting nil formatter should be ignored
	alertSender.SetFormatter(nil)

	// Should still work
	event := protocol.NewLongRunningAlertEvent(
		"alert-nil-fmt",
		"task-nil",
		"project-nil",
		300.0,
		180.0,
	)

	_, err := alertSender.SendAlert(12345, 67890, event)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestClearPending(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)

	// Send multiple alerts
	for i := 0; i < 5; i++ {
		event := protocol.NewLongRunningAlertEvent(
			"alert-clear-"+string(rune('0'+i)),
			"task-"+string(rune('0'+i)),
			"project-clear",
			300.0,
			180.0,
		)
		_, _ = alertSender.SendAlert(12345, 67890, event)
	}

	if alertSender.GetPendingCount() != 5 {
		t.Errorf("Expected 5 pending actions, got %d", alertSender.GetPendingCount())
	}

	alertSender.ClearPending()

	if alertSender.GetPendingCount() != 0 {
		t.Errorf("Expected 0 pending actions after clear, got %d", alertSender.GetPendingCount())
	}
}

func TestSendFormattedAlert(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)

	formattedAlert := &FormattedAlert{
		Title:     "Test Alert",
		Body:      "This is a test alert body",
		Severity:  protocol.AlertSeverityWarning,
		Summary:   "Test summary",
		TaskID:    "task-formatted",
		ProjectID: "proj-formatted",
	}

	alertID, err := alertSender.SendFormattedAlert(12345, 67890, formattedAlert, "")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if alertID == "" {
		t.Fatal("Expected non-empty alert ID")
	}

	// Verify message was sent
	msg := sender.getLastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if !strings.Contains(msg.Text, "This is a test alert body") {
		t.Errorf("Expected formatted body in message, got: %s", msg.Text)
	}
}

func TestSendFormattedAlertNil(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)

	_, err := alertSender.SendFormattedAlert(12345, 67890, nil, "")

	if err == nil {
		t.Error("Expected error for nil formatted alert")
	}
}

func TestSendFormattedAlertWithCustomID(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)

	formattedAlert := &FormattedAlert{
		Title:     "Test Alert",
		Body:      "Test body",
		Severity:  protocol.AlertSeverityInfo,
		Summary:   "Test",
		TaskID:    "task-custom",
		ProjectID: "proj-custom",
	}

	customID := "my-custom-alert-id"
	alertID, err := alertSender.SendFormattedAlert(12345, 67890, formattedAlert, customID)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if alertID != customID {
		t.Errorf("Expected custom alert ID %s, got %s", customID, alertID)
	}

	// Verify pending action uses custom ID
	pending := alertSender.GetPendingAction(customID)
	if pending == nil {
		t.Error("Expected pending action with custom ID")
	}
}

func TestHandlerNotConfigured(t *testing.T) {
	sender := newMockAlertSender()
	alertSender := NewAlertNotificationSender(sender)
	// Don't set cancel or logs handlers

	event := protocol.NewLongRunningAlertEvent(
		"alert-no-handler",
		"task-no-handler",
		"project-no-handler",
		300.0,
		180.0,
	)

	alertID, _ := alertSender.SendAlert(12345, 67890, event)

	// Create callback for cancel (handler not configured)
	cancelData := AlertCallbackData{
		Action:  AlertCallbackCancel,
		TaskID:  "task-no-handler",
		AlertID: alertID,
	}
	cancelJSON, _ := json.Marshal(cancelData)

	query := &tgbotapi.CallbackQuery{
		ID:   "callback_no_handler",
		From: &tgbotapi.User{ID: 67890},
		Data: string(cancelJSON),
	}

	// Should still handle the callback without error
	handled, err := alertSender.HandleCallbackQuery(context.Background(), query)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !handled {
		t.Error("Expected callback to be handled")
	}
}
