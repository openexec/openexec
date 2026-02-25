package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/openexec/openexec/internal/logging"
)

// ConfirmationAction represents the type of action being confirmed.
type ConfirmationAction string

const (
	// ConfirmationActionDeploy is used for deploy confirmations.
	ConfirmationActionDeploy ConfirmationAction = "deploy"
	// ConfirmationActionCancel is used for cancel confirmations.
	ConfirmationActionCancel ConfirmationAction = "cancel"
	// ConfirmationActionDelete is used for delete confirmations.
	ConfirmationActionDelete ConfirmationAction = "delete"
)

// ConfirmationResponse represents the user's response to a confirmation.
type ConfirmationResponse string

const (
	// ConfirmationYes indicates the user confirmed the action.
	ConfirmationYes ConfirmationResponse = "yes"
	// ConfirmationNo indicates the user declined the action.
	ConfirmationNo ConfirmationResponse = "no"
)

// CallbackData holds the data encoded in callback button presses.
type CallbackData struct {
	Action   ConfirmationAction   `json:"a"`
	Response ConfirmationResponse `json:"r"`
	ID       string               `json:"id"`
}

// PendingConfirmation holds details about a pending confirmation.
type PendingConfirmation struct {
	ID        string
	Action    ConfirmationAction
	ChatID    int64
	MessageID int
	UserID    int64
	Data      map[string]string
	ExpiresAt time.Time
	OnConfirm func(ctx context.Context) error
	OnDecline func(ctx context.Context) error
}

// ConfirmationHandler manages interactive confirmation flows.
type ConfirmationHandler struct {
	sender    MessageSender
	pending   map[string]*PendingConfirmation
	mu        sync.RWMutex
	expiryTTL time.Duration
}

// NewConfirmationHandler creates a new confirmation handler.
func NewConfirmationHandler(sender MessageSender) *ConfirmationHandler {
	h := &ConfirmationHandler{
		sender:    sender,
		pending:   make(map[string]*PendingConfirmation),
		expiryTTL: 5 * time.Minute,
	}
	// Start background cleanup goroutine
	go h.cleanupExpired()
	return h
}

// SetExpiryTTL sets the TTL for pending confirmations.
func (h *ConfirmationHandler) SetExpiryTTL(ttl time.Duration) {
	h.expiryTTL = ttl
}

// SendConfirmation sends a confirmation message with Yes/No buttons.
// Returns the pending confirmation ID for tracking.
func (h *ConfirmationHandler) SendConfirmation(
	chatID int64,
	userID int64,
	action ConfirmationAction,
	message string,
	data map[string]string,
	onConfirm func(ctx context.Context) error,
	onDecline func(ctx context.Context) error,
) (string, error) {
	// Generate unique confirmation ID
	confirmID := generateConfirmationID()

	// Create callback data for Yes button
	yesData := CallbackData{
		Action:   action,
		Response: ConfirmationYes,
		ID:       confirmID,
	}
	yesJSON, err := json.Marshal(yesData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal yes callback data: %w", err)
	}

	// Create callback data for No button
	noData := CallbackData{
		Action:   action,
		Response: ConfirmationNo,
		ID:       confirmID,
	}
	noJSON, err := json.Marshal(noData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal no callback data: %w", err)
	}

	// Create inline keyboard with Yes/No buttons
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Yes", string(yesJSON)),
			tgbotapi.NewInlineKeyboardButtonData("No", string(noJSON)),
		),
	)

	// Create message with inline keyboard
	msg := tgbotapi.NewMessage(chatID, message)
	msg.ReplyMarkup = keyboard

	// Send the message
	sentMsg, err := h.sender.Send(msg)
	if err != nil {
		return "", fmt.Errorf("failed to send confirmation message: %w", err)
	}

	// Store pending confirmation
	h.mu.Lock()
	h.pending[confirmID] = &PendingConfirmation{
		ID:        confirmID,
		Action:    action,
		ChatID:    chatID,
		MessageID: sentMsg.MessageID,
		UserID:    userID,
		Data:      data,
		ExpiresAt: time.Now().Add(h.expiryTTL),
		OnConfirm: onConfirm,
		OnDecline: onDecline,
	}
	h.mu.Unlock()

	return confirmID, nil
}

// HandleCallbackQuery processes a callback query from an inline button press.
// Returns true if the callback was handled, false otherwise.
func (h *ConfirmationHandler) HandleCallbackQuery(ctx context.Context, query *tgbotapi.CallbackQuery) (bool, error) {
	if query == nil || query.Data == "" {
		return false, nil
	}

	// Try to parse callback data
	var data CallbackData
	if err := json.Unmarshal([]byte(query.Data), &data); err != nil {
		// Not our callback data format
		return false, nil
	}

	// Look up pending confirmation
	h.mu.RLock()
	pending, exists := h.pending[data.ID]
	h.mu.RUnlock()

	if !exists {
		// Confirmation expired or not found
		h.answerCallback(query.ID, "This confirmation has expired.")
		return true, nil
	}

	// Verify user ID matches (only the original user can respond)
	if query.From != nil && query.From.ID != pending.UserID {
		h.answerCallback(query.ID, "Only the original user can respond to this confirmation.")
		return true, nil
	}

	// Remove from pending
	h.mu.Lock()
	delete(h.pending, data.ID)
	h.mu.Unlock()

	// Execute appropriate callback
	var err error
	var responseText string
	switch data.Response {
	case ConfirmationYes:
		responseText = "Confirmed"
		if pending.OnConfirm != nil {
			err = pending.OnConfirm(ctx)
		}
	case ConfirmationNo:
		responseText = "Cancelled"
		if pending.OnDecline != nil {
			err = pending.OnDecline(ctx)
		}
	}

	// Answer the callback query
	h.answerCallback(query.ID, responseText)

	// Update the original message to remove buttons and show result
	h.updateConfirmationMessage(pending, data.Response, err)

	return true, err
}

// answerCallback answers a callback query with optional text.
func (h *ConfirmationHandler) answerCallback(callbackID, text string) {
	callback := tgbotapi.NewCallback(callbackID, text)
	if _, err := h.sender.Send(callback); err != nil {
		logging.Warn("Failed to answer callback", "error", err)
	}
}

// updateConfirmationMessage updates the confirmation message after a response.
func (h *ConfirmationHandler) updateConfirmationMessage(pending *PendingConfirmation, response ConfirmationResponse, execErr error) {
	var statusText string
	switch response {
	case ConfirmationYes:
		if execErr != nil {
			statusText = fmt.Sprintf("Action failed: %v", execErr)
		} else {
			statusText = "Action confirmed and executed."
		}
	case ConfirmationNo:
		statusText = "Action cancelled."
	}

	// Edit message to remove keyboard and show status
	edit := tgbotapi.NewEditMessageText(pending.ChatID, pending.MessageID, statusText)
	edit.ReplyMarkup = nil // Remove inline keyboard

	if _, err := h.sender.Send(edit); err != nil {
		logging.Warn("Failed to update confirmation message", "error", err)
	}
}

// cleanupExpired periodically removes expired pending confirmations.
func (h *ConfirmationHandler) cleanupExpired() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		h.mu.Lock()
		now := time.Now()
		for id, pending := range h.pending {
			if now.After(pending.ExpiresAt) {
				delete(h.pending, id)
			}
		}
		h.mu.Unlock()
	}
}

// GetPendingCount returns the number of pending confirmations (for testing).
func (h *ConfirmationHandler) GetPendingCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.pending)
}

// GetPending returns a pending confirmation by ID (for testing).
func (h *ConfirmationHandler) GetPending(id string) *PendingConfirmation {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.pending[id]
}

// generateConfirmationID generates a unique confirmation ID.
func generateConfirmationID() string {
	return fmt.Sprintf("c_%d", time.Now().UnixNano())
}

// CreateConfirmationKeyboard creates an inline keyboard with Yes/No buttons.
// This is a helper function for creating custom confirmation keyboards.
func CreateConfirmationKeyboard(confirmID string, action ConfirmationAction) (tgbotapi.InlineKeyboardMarkup, error) {
	// Create callback data for Yes button
	yesData := CallbackData{
		Action:   action,
		Response: ConfirmationYes,
		ID:       confirmID,
	}
	yesJSON, err := json.Marshal(yesData)
	if err != nil {
		return tgbotapi.InlineKeyboardMarkup{}, fmt.Errorf("failed to marshal yes callback data: %w", err)
	}

	// Create callback data for No button
	noData := CallbackData{
		Action:   action,
		Response: ConfirmationNo,
		ID:       confirmID,
	}
	noJSON, err := json.Marshal(noData)
	if err != nil {
		return tgbotapi.InlineKeyboardMarkup{}, fmt.Errorf("failed to marshal no callback data: %w", err)
	}

	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Yes", string(yesJSON)),
			tgbotapi.NewInlineKeyboardButtonData("No", string(noJSON)),
		),
	), nil
}

// ParseCallbackData parses callback data from a button press.
func ParseCallbackData(data string) (*CallbackData, error) {
	if data == "" {
		return nil, fmt.Errorf("empty callback data")
	}

	var callbackData CallbackData
	if err := json.Unmarshal([]byte(data), &callbackData); err != nil {
		return nil, fmt.Errorf("failed to parse callback data: %w", err)
	}

	return &callbackData, nil
}

// FormatConfirmationMessage formats a confirmation message for a given action.
func FormatConfirmationMessage(action ConfirmationAction, details map[string]string) string {
	var sb strings.Builder

	switch action {
	case ConfirmationActionDeploy:
		sb.WriteString("Are you sure you want to deploy?\n\n")
		if projectID, ok := details["project_id"]; ok {
			sb.WriteString(fmt.Sprintf("Project: %s\n", projectID))
		}
		if env, ok := details["environment"]; ok {
			sb.WriteString(fmt.Sprintf("Environment: %s\n", env))
		}
	case ConfirmationActionCancel:
		sb.WriteString("Are you sure you want to cancel this task?\n\n")
		if taskID, ok := details["task_id"]; ok {
			sb.WriteString(fmt.Sprintf("Task: %s\n", taskID))
		}
	case ConfirmationActionDelete:
		sb.WriteString("Are you sure you want to delete?\n\n")
		if name, ok := details["name"]; ok {
			sb.WriteString(fmt.Sprintf("Item: %s\n", name))
		}
	default:
		sb.WriteString("Please confirm this action.\n")
	}

	sb.WriteString("\nSelect Yes or No below.")

	return sb.String()
}
