package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/openexec/openexec/internal/logging"
	"github.com/openexec/openexec/internal/protocol"
)

// AlertCallbackAction represents the type of action for alert callbacks.
type AlertCallbackAction string

const (
	// AlertCallbackCancel is the cancel action for alert callbacks.
	AlertCallbackCancel AlertCallbackAction = "alert_cancel"

	// AlertCallbackLogs is the logs action for alert callbacks.
	AlertCallbackLogs AlertCallbackAction = "alert_logs"

	// AlertCallbackDismiss is the dismiss action for alert callbacks.
	AlertCallbackDismiss AlertCallbackAction = "alert_dismiss"
)

// AlertCallbackData holds the data encoded in alert action button presses.
type AlertCallbackData struct {
	Action    AlertCallbackAction `json:"a"`
	TaskID    string              `json:"t"`
	ProjectID string              `json:"p,omitempty"`
	AlertID   string              `json:"id"`
}

// PendingAlertAction holds details about a pending alert action.
type PendingAlertAction struct {
	AlertID   string
	TaskID    string
	ProjectID string
	ChatID    int64
	MessageID int
	UserID    int64
	ExpiresAt time.Time
}

// AlertActionHandler is a function that handles alert actions.
type AlertActionHandler func(ctx context.Context, action AlertCallbackAction, taskID, projectID string) error

// AlertNotificationSender sends alert notifications with action buttons.
type AlertNotificationSender struct {
	sender         MessageSender
	formatter      *AlertFormatter
	cancelHandler  AlertActionHandler
	logsHandler    AlertActionHandler
	pendingActions map[string]*PendingAlertAction
	mu             sync.RWMutex
	expiryTTL      time.Duration
	debug          bool
}

// NewAlertNotificationSender creates a new AlertNotificationSender.
func NewAlertNotificationSender(sender MessageSender) *AlertNotificationSender {
	s := &AlertNotificationSender{
		sender:         sender,
		formatter:      NewAlertFormatter(),
		pendingActions: make(map[string]*PendingAlertAction),
		expiryTTL:      30 * time.Minute, // Alerts expire after 30 minutes
	}
	// Start background cleanup goroutine
	go s.cleanupExpired()
	return s
}

// SetFormatter sets a custom alert formatter.
func (s *AlertNotificationSender) SetFormatter(formatter *AlertFormatter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if formatter != nil {
		s.formatter = formatter
	}
}

// SetCancelHandler sets the handler for cancel button presses.
func (s *AlertNotificationSender) SetCancelHandler(handler AlertActionHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelHandler = handler
}

// SetLogsHandler sets the handler for logs button presses.
func (s *AlertNotificationSender) SetLogsHandler(handler AlertActionHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logsHandler = handler
}

// SetExpiryTTL sets the TTL for pending alert actions.
func (s *AlertNotificationSender) SetExpiryTTL(ttl time.Duration) {
	s.expiryTTL = ttl
}

// SetDebug enables or disables debug logging.
func (s *AlertNotificationSender) SetDebug(debug bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.debug = debug
}

// SendAlert sends an alert notification with action buttons.
// Returns the alert ID for tracking.
func (s *AlertNotificationSender) SendAlert(
	chatID int64,
	userID int64,
	event *protocol.AlertEvent,
) (string, error) {
	if event == nil {
		return "", fmt.Errorf("alert event is required")
	}

	// Get formatter under lock
	s.mu.RLock()
	formatter := s.formatter
	debug := s.debug
	s.mu.RUnlock()

	// Format the alert
	formatted := formatter.Format(event)

	// Create inline keyboard with action buttons
	keyboard := s.createAlertKeyboard(event.AlertID, event.TaskID, event.ProjectID)

	// Create message with inline keyboard
	msg := tgbotapi.NewMessage(chatID, formatted.Body)
	msg.ReplyMarkup = keyboard

	// Send the message
	sentMsg, err := s.sender.Send(msg)
	if err != nil {
		return "", fmt.Errorf("failed to send alert message: %w", err)
	}

	// Store pending action for callback handling
	s.mu.Lock()
	s.pendingActions[event.AlertID] = &PendingAlertAction{
		AlertID:   event.AlertID,
		TaskID:    event.TaskID,
		ProjectID: event.ProjectID,
		ChatID:    chatID,
		MessageID: sentMsg.MessageID,
		UserID:    userID,
		ExpiresAt: time.Now().Add(s.expiryTTL),
	}
	s.mu.Unlock()

	if debug {
		logging.Debug("Alert sent", "alert_id", event.AlertID, "task_id", event.TaskID, "project_id", event.ProjectID)
	}

	return event.AlertID, nil
}

// SendAlertEvent sends a protocol AlertEvent as a notification with buttons.
func (s *AlertNotificationSender) SendAlertEvent(chatID int64, userID int64, event *protocol.AlertEvent) (string, error) {
	return s.SendAlert(chatID, userID, event)
}

// SendFormattedAlert sends a pre-formatted alert with action buttons.
func (s *AlertNotificationSender) SendFormattedAlert(
	chatID int64,
	userID int64,
	alert *FormattedAlert,
	alertID string,
) (string, error) {
	if alert == nil {
		return "", fmt.Errorf("formatted alert is required")
	}

	// Generate alert ID if not provided
	if alertID == "" {
		alertID = fmt.Sprintf("alert_%d", time.Now().UnixNano())
	}

	// Create inline keyboard with action buttons
	keyboard := s.createAlertKeyboard(alertID, alert.TaskID, alert.ProjectID)

	// Create message with inline keyboard
	msg := tgbotapi.NewMessage(chatID, alert.Body)
	msg.ReplyMarkup = keyboard

	// Send the message
	sentMsg, err := s.sender.Send(msg)
	if err != nil {
		return "", fmt.Errorf("failed to send alert message: %w", err)
	}

	// Store pending action for callback handling
	s.mu.Lock()
	s.pendingActions[alertID] = &PendingAlertAction{
		AlertID:   alertID,
		TaskID:    alert.TaskID,
		ProjectID: alert.ProjectID,
		ChatID:    chatID,
		MessageID: sentMsg.MessageID,
		UserID:    userID,
		ExpiresAt: time.Now().Add(s.expiryTTL),
	}
	s.mu.Unlock()

	return alertID, nil
}

// createAlertKeyboard creates an inline keyboard with alert action buttons.
func (s *AlertNotificationSender) createAlertKeyboard(alertID, taskID, projectID string) tgbotapi.InlineKeyboardMarkup {
	// Build callback data for each button
	var buttons []tgbotapi.InlineKeyboardButton

	// Only add action buttons if there's a task ID to act on
	if taskID != "" {
		// Cancel button
		cancelData := AlertCallbackData{
			Action:    AlertCallbackCancel,
			TaskID:    taskID,
			ProjectID: projectID,
			AlertID:   alertID,
		}
		cancelJSON, err := json.Marshal(cancelData)
		if err == nil {
			buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("🛑 Cancel", string(cancelJSON)))
		}

		// Logs button
		logsData := AlertCallbackData{
			Action:    AlertCallbackLogs,
			TaskID:    taskID,
			ProjectID: projectID,
			AlertID:   alertID,
		}
		logsJSON, err := json.Marshal(logsData)
		if err == nil {
			buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("📋 Logs", string(logsJSON)))
		}
	}

	// Dismiss button (always available)
	dismissData := AlertCallbackData{
		Action:  AlertCallbackDismiss,
		AlertID: alertID,
	}
	dismissJSON, err := json.Marshal(dismissData)
	if err == nil {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("✓ Dismiss", string(dismissJSON)))
	}

	// Create keyboard layout
	if len(buttons) == 0 {
		return tgbotapi.InlineKeyboardMarkup{}
	}

	// Arrange buttons: Cancel and Logs on first row, Dismiss on second row
	var rows [][]tgbotapi.InlineKeyboardButton
	if len(buttons) == 3 {
		// Cancel + Logs on first row, Dismiss on second
		rows = append(rows, buttons[:2])
		rows = append(rows, buttons[2:])
	} else {
		// All buttons on one row
		rows = append(rows, buttons)
	}

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// HandleCallbackQuery processes a callback query from an alert action button press.
// Returns true if the callback was handled, false otherwise.
func (s *AlertNotificationSender) HandleCallbackQuery(ctx context.Context, query *tgbotapi.CallbackQuery) (bool, error) {
	if query == nil || query.Data == "" {
		return false, nil
	}

	// Try to parse callback data
	var data AlertCallbackData
	if err := json.Unmarshal([]byte(query.Data), &data); err != nil {
		// Not our callback data format
		return false, nil
	}

	// Check if this is an alert callback
	if !isAlertCallback(data.Action) {
		return false, nil
	}

	// Look up pending action
	s.mu.RLock()
	pending, exists := s.pendingActions[data.AlertID]
	debug := s.debug
	s.mu.RUnlock()

	if !exists {
		// Alert expired or not found
		s.answerCallback(query.ID, "This alert has expired.")
		return true, nil
	}

	// Process the action
	var err error
	var responseText string

	switch data.Action {
	case AlertCallbackCancel:
		s.mu.RLock()
		handler := s.cancelHandler
		s.mu.RUnlock()
		if handler != nil {
			err = handler(ctx, data.Action, data.TaskID, data.ProjectID)
			if err != nil {
				responseText = fmt.Sprintf("Cancel failed: %v", err)
			} else {
				responseText = "Cancel request sent"
			}
		} else {
			responseText = "Cancel handler not configured"
		}

	case AlertCallbackLogs:
		s.mu.RLock()
		handler := s.logsHandler
		s.mu.RUnlock()
		if handler != nil {
			err = handler(ctx, data.Action, data.TaskID, data.ProjectID)
			if err != nil {
				responseText = fmt.Sprintf("Failed to fetch logs: %v", err)
			} else {
				responseText = "Logs request sent"
			}
		} else {
			responseText = "Logs handler not configured"
		}

	case AlertCallbackDismiss:
		responseText = "Alert dismissed"
		// Remove from pending
		s.mu.Lock()
		delete(s.pendingActions, data.AlertID)
		s.mu.Unlock()
	}

	// Answer the callback query
	s.answerCallback(query.ID, responseText)

	// Update the message based on action
	if data.Action == AlertCallbackDismiss {
		// Remove keyboard and update message for dismiss
		s.updateAlertMessage(pending, "Alert dismissed", true)
	} else if err == nil && data.Action == AlertCallbackCancel {
		// Update message to show cancel was requested
		s.updateAlertMessage(pending, "Cancel request sent for task: "+data.TaskID, false)
	}

	if debug {
		logging.Debug("Alert callback handled", "action", data.Action, "task_id", data.TaskID, "alert_id", data.AlertID)
	}

	return true, err
}

// isAlertCallback checks if the action is an alert callback action.
func isAlertCallback(action AlertCallbackAction) bool {
	switch action {
	case AlertCallbackCancel, AlertCallbackLogs, AlertCallbackDismiss:
		return true
	default:
		return false
	}
}

// IsAlertCallback checks if callback data represents an alert action.
// This can be used by the command handler to route callbacks.
func IsAlertCallback(data string) bool {
	var callbackData AlertCallbackData
	if err := json.Unmarshal([]byte(data), &callbackData); err != nil {
		return false
	}
	return isAlertCallback(callbackData.Action)
}

// ParseAlertCallbackData parses callback data from a button press.
func ParseAlertCallbackData(data string) (*AlertCallbackData, error) {
	if data == "" {
		return nil, fmt.Errorf("empty callback data")
	}

	var callbackData AlertCallbackData
	if err := json.Unmarshal([]byte(data), &callbackData); err != nil {
		return nil, fmt.Errorf("failed to parse callback data: %w", err)
	}

	return &callbackData, nil
}

// answerCallback answers a callback query with optional text.
func (s *AlertNotificationSender) answerCallback(callbackID, text string) {
	callback := tgbotapi.NewCallback(callbackID, text)
	if _, err := s.sender.Send(callback); err != nil {
		logging.Warn("Failed to answer callback", "error", err)
	}
}

// updateAlertMessage updates the alert message after an action.
func (s *AlertNotificationSender) updateAlertMessage(pending *PendingAlertAction, statusText string, removeKeyboard bool) {
	if pending == nil {
		return
	}

	edit := tgbotapi.NewEditMessageText(pending.ChatID, pending.MessageID, statusText)
	if removeKeyboard {
		edit.ReplyMarkup = nil // Remove inline keyboard
	}

	if _, err := s.sender.Send(edit); err != nil {
		logging.Warn("Failed to update alert message", "error", err)
	}
}

// cleanupExpired periodically removes expired pending alert actions.
func (s *AlertNotificationSender) cleanupExpired() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for id, pending := range s.pendingActions {
			if now.After(pending.ExpiresAt) {
				delete(s.pendingActions, id)
			}
		}
		s.mu.Unlock()
	}
}

// GetPendingCount returns the number of pending alert actions (for testing).
func (s *AlertNotificationSender) GetPendingCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.pendingActions)
}

// GetPendingAction returns a pending alert action by ID (for testing).
func (s *AlertNotificationSender) GetPendingAction(alertID string) *PendingAlertAction {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pendingActions[alertID]
}

// ClearPending removes all pending alert actions (for testing).
func (s *AlertNotificationSender) ClearPending() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingActions = make(map[string]*PendingAlertAction)
}
