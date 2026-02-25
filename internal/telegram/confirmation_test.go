package telegram

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// mockConfirmationSender is a mock implementation of MessageSender for confirmation testing.
type mockConfirmationSender struct {
	mu           sync.Mutex
	sentMessages []tgbotapi.Chattable
	callbacks    []tgbotapi.CallbackConfig
	lastMsgID    int
}

func newMockConfirmationSender() *mockConfirmationSender {
	return &mockConfirmationSender{
		sentMessages: make([]tgbotapi.Chattable, 0),
		callbacks:    make([]tgbotapi.CallbackConfig, 0),
		lastMsgID:    0,
	}
}

func (m *mockConfirmationSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
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

func (m *mockConfirmationSender) getLastMessage() *tgbotapi.MessageConfig {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := len(m.sentMessages) - 1; i >= 0; i-- {
		if msg, ok := m.sentMessages[i].(tgbotapi.MessageConfig); ok {
			return &msg
		}
	}
	return nil
}

func (m *mockConfirmationSender) getLastCallback() *tgbotapi.CallbackConfig {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.callbacks) == 0 {
		return nil
	}
	return &m.callbacks[len(m.callbacks)-1]
}

func (m *mockConfirmationSender) getMessageCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sentMessages)
}

func TestNewConfirmationHandler(t *testing.T) {
	sender := newMockConfirmationSender()
	handler := NewConfirmationHandler(sender)

	if handler == nil {
		t.Fatal("Expected confirmation handler to be created")
	}

	if handler.GetPendingCount() != 0 {
		t.Errorf("Expected 0 pending confirmations, got %d", handler.GetPendingCount())
	}
}

func TestSendConfirmation(t *testing.T) {
	sender := newMockConfirmationSender()
	handler := NewConfirmationHandler(sender)

	confirmID, err := handler.SendConfirmation(
		12345, // chatID
		67890, // userID
		ConfirmationActionDeploy,
		"Are you sure you want to deploy?",
		map[string]string{"project_id": "test-project"},
		nil, // onConfirm
		nil, // onDecline
	)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if confirmID == "" {
		t.Fatal("Expected non-empty confirmation ID")
	}

	if handler.GetPendingCount() != 1 {
		t.Errorf("Expected 1 pending confirmation, got %d", handler.GetPendingCount())
	}

	// Verify message was sent
	msg := sender.getLastMessage()
	if msg == nil {
		t.Fatal("Expected a message to be sent")
	}

	if msg.ChatID != 12345 {
		t.Errorf("Expected chat ID 12345, got %d", msg.ChatID)
	}

	if !strings.Contains(msg.Text, "Are you sure you want to deploy?") {
		t.Errorf("Expected confirmation message, got: %s", msg.Text)
	}

	// Verify inline keyboard is present
	if msg.ReplyMarkup == nil {
		t.Fatal("Expected reply markup to be set")
	}

	keyboard, ok := msg.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup)
	if !ok {
		t.Fatal("Expected InlineKeyboardMarkup")
	}

	if len(keyboard.InlineKeyboard) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(keyboard.InlineKeyboard))
	}

	if len(keyboard.InlineKeyboard[0]) != 2 {
		t.Fatalf("Expected 2 buttons, got %d", len(keyboard.InlineKeyboard[0]))
	}

	// Verify button labels
	yesBtn := keyboard.InlineKeyboard[0][0]
	noBtn := keyboard.InlineKeyboard[0][1]

	if yesBtn.Text != "Yes" {
		t.Errorf("Expected Yes button, got: %s", yesBtn.Text)
	}

	if noBtn.Text != "No" {
		t.Errorf("Expected No button, got: %s", noBtn.Text)
	}

	// Verify callback data
	if yesBtn.CallbackData == nil || noBtn.CallbackData == nil {
		t.Fatal("Expected callback data to be set")
	}

	var yesData, noData CallbackData
	if err := json.Unmarshal([]byte(*yesBtn.CallbackData), &yesData); err != nil {
		t.Fatalf("Failed to parse yes callback data: %v", err)
	}
	if err := json.Unmarshal([]byte(*noBtn.CallbackData), &noData); err != nil {
		t.Fatalf("Failed to parse no callback data: %v", err)
	}

	if yesData.Response != ConfirmationYes {
		t.Errorf("Expected Yes response, got: %s", yesData.Response)
	}
	if noData.Response != ConfirmationNo {
		t.Errorf("Expected No response, got: %s", noData.Response)
	}
	if yesData.ID != confirmID || noData.ID != confirmID {
		t.Error("Confirmation IDs don't match")
	}
}

func TestHandleCallbackQueryYes(t *testing.T) {
	sender := newMockConfirmationSender()
	handler := NewConfirmationHandler(sender)

	confirmCalled := false
	declineCalled := false

	confirmID, err := handler.SendConfirmation(
		12345, 67890,
		ConfirmationActionDeploy,
		"Are you sure?",
		nil,
		func(ctx context.Context) error {
			confirmCalled = true
			return nil
		},
		func(ctx context.Context) error {
			declineCalled = true
			return nil
		},
	)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Create callback query for Yes button
	yesData := CallbackData{
		Action:   ConfirmationActionDeploy,
		Response: ConfirmationYes,
		ID:       confirmID,
	}
	yesJSON, _ := json.Marshal(yesData)

	query := &tgbotapi.CallbackQuery{
		ID:   "callback123",
		From: &tgbotapi.User{ID: 67890},
		Data: string(yesJSON),
	}

	handled, err := handler.HandleCallbackQuery(context.Background(), query)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !handled {
		t.Error("Expected callback to be handled")
	}

	if !confirmCalled {
		t.Error("Expected confirm callback to be called")
	}

	if declineCalled {
		t.Error("Expected decline callback NOT to be called")
	}

	// Pending should be removed
	if handler.GetPendingCount() != 0 {
		t.Errorf("Expected 0 pending confirmations after handling, got %d", handler.GetPendingCount())
	}
}

func TestHandleCallbackQueryNo(t *testing.T) {
	sender := newMockConfirmationSender()
	handler := NewConfirmationHandler(sender)

	confirmCalled := false
	declineCalled := false

	confirmID, err := handler.SendConfirmation(
		12345, 67890,
		ConfirmationActionCancel,
		"Are you sure?",
		nil,
		func(ctx context.Context) error {
			confirmCalled = true
			return nil
		},
		func(ctx context.Context) error {
			declineCalled = true
			return nil
		},
	)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Create callback query for No button
	noData := CallbackData{
		Action:   ConfirmationActionCancel,
		Response: ConfirmationNo,
		ID:       confirmID,
	}
	noJSON, _ := json.Marshal(noData)

	query := &tgbotapi.CallbackQuery{
		ID:   "callback456",
		From: &tgbotapi.User{ID: 67890},
		Data: string(noJSON),
	}

	handled, err := handler.HandleCallbackQuery(context.Background(), query)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !handled {
		t.Error("Expected callback to be handled")
	}

	if confirmCalled {
		t.Error("Expected confirm callback NOT to be called")
	}

	if !declineCalled {
		t.Error("Expected decline callback to be called")
	}
}

func TestHandleCallbackQueryWrongUser(t *testing.T) {
	sender := newMockConfirmationSender()
	handler := NewConfirmationHandler(sender)

	confirmID, err := handler.SendConfirmation(
		12345, 67890, // userID = 67890
		ConfirmationActionDeploy,
		"Are you sure?",
		nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Create callback from different user
	yesData := CallbackData{
		Action:   ConfirmationActionDeploy,
		Response: ConfirmationYes,
		ID:       confirmID,
	}
	yesJSON, _ := json.Marshal(yesData)

	query := &tgbotapi.CallbackQuery{
		ID:   "callback789",
		From: &tgbotapi.User{ID: 99999}, // Different user
		Data: string(yesJSON),
	}

	handled, err := handler.HandleCallbackQuery(context.Background(), query)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !handled {
		t.Error("Expected callback to be handled (even if rejected)")
	}

	// Confirmation should still be pending (not consumed)
	if handler.GetPendingCount() != 1 {
		t.Errorf("Expected 1 pending confirmation (should not be consumed by wrong user), got %d", handler.GetPendingCount())
	}
}

func TestHandleCallbackQueryExpired(t *testing.T) {
	sender := newMockConfirmationSender()
	handler := NewConfirmationHandler(sender)
	handler.SetExpiryTTL(1 * time.Millisecond) // Very short TTL

	confirmID, err := handler.SendConfirmation(
		12345, 67890,
		ConfirmationActionDeploy,
		"Are you sure?",
		nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Wait for expiry (plus some buffer)
	time.Sleep(10 * time.Millisecond)

	// Manually trigger cleanup (since the ticker is 1 minute)
	pending := handler.GetPending(confirmID)
	if pending != nil && time.Now().After(pending.ExpiresAt) {
		// Simulate cleanup
		handler.mu.Lock()
		delete(handler.pending, confirmID)
		handler.mu.Unlock()
	}

	// Create callback for expired confirmation
	yesData := CallbackData{
		Action:   ConfirmationActionDeploy,
		Response: ConfirmationYes,
		ID:       confirmID,
	}
	yesJSON, _ := json.Marshal(yesData)

	query := &tgbotapi.CallbackQuery{
		ID:   "callback_expired",
		From: &tgbotapi.User{ID: 67890},
		Data: string(yesJSON),
	}

	handled, err := handler.HandleCallbackQuery(context.Background(), query)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !handled {
		t.Error("Expected callback to be handled (even if expired)")
	}
}

func TestHandleCallbackQueryInvalidData(t *testing.T) {
	sender := newMockConfirmationSender()
	handler := NewConfirmationHandler(sender)

	// Create callback with invalid JSON
	query := &tgbotapi.CallbackQuery{
		ID:   "callback_invalid",
		From: &tgbotapi.User{ID: 67890},
		Data: "not valid json",
	}

	handled, err := handler.HandleCallbackQuery(context.Background(), query)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if handled {
		t.Error("Expected callback NOT to be handled (invalid data)")
	}
}

func TestHandleCallbackQueryNil(t *testing.T) {
	sender := newMockConfirmationSender()
	handler := NewConfirmationHandler(sender)

	handled, err := handler.HandleCallbackQuery(context.Background(), nil)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if handled {
		t.Error("Expected nil callback NOT to be handled")
	}
}

func TestHandleCallbackQueryEmptyData(t *testing.T) {
	sender := newMockConfirmationSender()
	handler := NewConfirmationHandler(sender)

	query := &tgbotapi.CallbackQuery{
		ID:   "callback_empty",
		From: &tgbotapi.User{ID: 67890},
		Data: "",
	}

	handled, err := handler.HandleCallbackQuery(context.Background(), query)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if handled {
		t.Error("Expected empty data callback NOT to be handled")
	}
}

func TestCreateConfirmationKeyboard(t *testing.T) {
	keyboard, err := CreateConfirmationKeyboard("test-id", ConfirmationActionDeploy)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(keyboard.InlineKeyboard) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(keyboard.InlineKeyboard))
	}

	if len(keyboard.InlineKeyboard[0]) != 2 {
		t.Fatalf("Expected 2 buttons, got %d", len(keyboard.InlineKeyboard[0]))
	}

	// Verify Yes button
	yesBtn := keyboard.InlineKeyboard[0][0]
	if yesBtn.Text != "Yes" {
		t.Errorf("Expected Yes button text, got: %s", yesBtn.Text)
	}

	var yesData CallbackData
	if err := json.Unmarshal([]byte(*yesBtn.CallbackData), &yesData); err != nil {
		t.Fatalf("Failed to parse yes callback data: %v", err)
	}
	if yesData.ID != "test-id" {
		t.Errorf("Expected test-id, got: %s", yesData.ID)
	}
	if yesData.Action != ConfirmationActionDeploy {
		t.Errorf("Expected deploy action, got: %s", yesData.Action)
	}

	// Verify No button
	noBtn := keyboard.InlineKeyboard[0][1]
	if noBtn.Text != "No" {
		t.Errorf("Expected No button text, got: %s", noBtn.Text)
	}
}

func TestParseCallbackData(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
		expected  *CallbackData
	}{
		{
			name:      "valid data",
			input:     `{"a":"deploy","r":"yes","id":"123"}`,
			wantError: false,
			expected: &CallbackData{
				Action:   ConfirmationActionDeploy,
				Response: ConfirmationYes,
				ID:       "123",
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
			result, err := ParseCallbackData(tt.input)

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
			if result.Response != tt.expected.Response {
				t.Errorf("Expected response %s, got %s", tt.expected.Response, result.Response)
			}
			if result.ID != tt.expected.ID {
				t.Errorf("Expected ID %s, got %s", tt.expected.ID, result.ID)
			}
		})
	}
}

func TestFormatConfirmationMessage(t *testing.T) {
	tests := []struct {
		name     string
		action   ConfirmationAction
		details  map[string]string
		contains []string
	}{
		{
			name:   "deploy with details",
			action: ConfirmationActionDeploy,
			details: map[string]string{
				"project_id":  "my-project",
				"environment": "production",
			},
			contains: []string{"deploy", "Project: my-project", "Environment: production"},
		},
		{
			name:   "cancel with task",
			action: ConfirmationActionCancel,
			details: map[string]string{
				"task_id": "T-001",
			},
			contains: []string{"cancel", "Task: T-001"},
		},
		{
			name:   "delete with name",
			action: ConfirmationActionDelete,
			details: map[string]string{
				"name": "my-item",
			},
			contains: []string{"delete", "Item: my-item"},
		},
		{
			name:     "unknown action",
			action:   ConfirmationAction("unknown"),
			details:  nil,
			contains: []string{"confirm"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatConfirmationMessage(tt.action, tt.details)

			for _, s := range tt.contains {
				if !strings.Contains(result, s) {
					t.Errorf("Expected message to contain '%s', got: %s", s, result)
				}
			}
		})
	}
}

func TestConfirmationConstants(t *testing.T) {
	// Verify confirmation actions
	if ConfirmationActionDeploy != "deploy" {
		t.Errorf("Expected 'deploy', got '%s'", ConfirmationActionDeploy)
	}
	if ConfirmationActionCancel != "cancel" {
		t.Errorf("Expected 'cancel', got '%s'", ConfirmationActionCancel)
	}
	if ConfirmationActionDelete != "delete" {
		t.Errorf("Expected 'delete', got '%s'", ConfirmationActionDelete)
	}

	// Verify confirmation responses
	if ConfirmationYes != "yes" {
		t.Errorf("Expected 'yes', got '%s'", ConfirmationYes)
	}
	if ConfirmationNo != "no" {
		t.Errorf("Expected 'no', got '%s'", ConfirmationNo)
	}
}

func TestGenerateConfirmationID(t *testing.T) {
	id1 := generateConfirmationID()

	if id1 == "" {
		t.Error("Expected non-empty ID")
	}

	if !strings.HasPrefix(id1, "c_") {
		t.Errorf("Expected ID to start with 'c_', got: %s", id1)
	}

	// IDs should be unique (with high probability given nanosecond timestamp)
	// Add small delay to ensure different timestamps
	time.Sleep(1 * time.Millisecond)
	id2 := generateConfirmationID()
	if id1 == id2 {
		t.Error("Expected unique IDs")
	}
}

func TestSetExpiryTTL(t *testing.T) {
	sender := newMockConfirmationSender()
	handler := NewConfirmationHandler(sender)

	handler.SetExpiryTTL(10 * time.Minute)

	// Send a confirmation and check expiry time
	confirmID, _ := handler.SendConfirmation(
		12345, 67890,
		ConfirmationActionDeploy,
		"Test",
		nil, nil, nil,
	)

	pending := handler.GetPending(confirmID)
	if pending == nil {
		t.Fatal("Expected pending confirmation")
	}

	// Expiry should be approximately 10 minutes from now
	expectedExpiry := time.Now().Add(10 * time.Minute)
	if pending.ExpiresAt.Before(expectedExpiry.Add(-1*time.Second)) ||
		pending.ExpiresAt.After(expectedExpiry.Add(1*time.Second)) {
		t.Errorf("Expiry time mismatch: expected around %v, got %v", expectedExpiry, pending.ExpiresAt)
	}
}
