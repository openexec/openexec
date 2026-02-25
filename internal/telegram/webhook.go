package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// WebhookHandler handles incoming Telegram webhook requests.
type WebhookHandler struct {
	bot            *Bot
	secretToken    string
	authMiddleware *AuthMiddleware
}

// NewWebhookHandler creates a new webhook handler for the given bot.
func NewWebhookHandler(bot *Bot, secretToken string) *WebhookHandler {
	return &WebhookHandler{
		bot:         bot,
		secretToken: secretToken,
	}
}

// WithAuth sets the auth middleware for the webhook handler.
// When set, incoming updates will be checked against the user allowlist.
func (h *WebhookHandler) WithAuth(auth *AuthMiddleware) *WebhookHandler {
	h.authMiddleware = auth
	return h
}

// HandleUpdate processes an incoming webhook request and returns the parsed update.
// It validates the request and returns an error if validation fails.
func (h *WebhookHandler) HandleUpdate(r *http.Request) (*tgbotapi.Update, error) {
	// Validate request method
	if r.Method != http.MethodPost {
		return nil, fmt.Errorf("method not allowed: %s", r.Method)
	}

	// Validate secret token if configured
	if h.secretToken != "" {
		token := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		if token != h.secretToken {
			return nil, fmt.Errorf("invalid secret token")
		}
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	defer func() { _ = r.Body.Close() }()

	// Parse update
	var update tgbotapi.Update
	if err := json.Unmarshal(body, &update); err != nil {
		return nil, fmt.Errorf("failed to parse update: %w", err)
	}

	return &update, nil
}

// ServeHTTP implements the http.Handler interface for use as a standard HTTP handler.
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	update, err := h.HandleUpdate(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check auth if middleware is configured
	if h.authMiddleware != nil {
		result := h.authMiddleware.CheckUpdate(context.Background(), update)
		if !result.Allowed {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Process the update
	h.bot.ProcessUpdate(*update)

	// Respond with 200 OK
	w.WriteHeader(http.StatusOK)
}

// Bot returns the associated bot instance.
func (h *WebhookHandler) Bot() *Bot {
	return h.bot
}
