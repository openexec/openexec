// Package telegram provides Telegram bot API client and webhook handling.
package telegram

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/openexec/openexec/internal/logging"
)

// Update is a type alias for tgbotapi.Update.
type Update = tgbotapi.Update

// Message is a type alias for tgbotapi.Message.
type Message = tgbotapi.Message

// User is a type alias for tgbotapi.User.
type User = tgbotapi.User

// Bot represents a Telegram bot client.
type Bot struct {
	api     *tgbotapi.BotAPI
	token   string
	debug   bool
	mu      sync.RWMutex
	handler UpdateHandler
}

// NewMock creates a mock Bot instance for testing purposes.
// This bot is not connected to Telegram and cannot send/receive messages.
func NewMock() *Bot {
	return &Bot{}
}

// UpdateHandler is a function that handles incoming Telegram updates.
type UpdateHandler func(update tgbotapi.Update)

// Config holds bot configuration options.
type Config struct {
	Token string
	Debug bool
}

// New creates a new Telegram bot client with the given configuration.
func New(cfg Config) (*Bot, error) {
	if cfg.Token == "" || strings.TrimSpace(cfg.Token) == "" {
		return nil, fmt.Errorf("telegram bot token is required")
	}

	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot API: %w", err)
	}

	api.Debug = cfg.Debug

	if cfg.Debug {
		logging.Debug("Authorized on Telegram bot account", "username", api.Self.UserName)
	}

	return &Bot{
		api:   api,
		token: cfg.Token,
		debug: cfg.Debug,
	}, nil
}

// SetUpdateHandler sets the handler function for incoming updates.
func (b *Bot) SetUpdateHandler(handler UpdateHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handler = handler
}

// GetUpdateHandler returns the current update handler.
func (b *Bot) GetUpdateHandler() UpdateHandler {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.handler
}

// ProcessUpdate processes an incoming update from a webhook.
func (b *Bot) ProcessUpdate(update tgbotapi.Update) {
	handler := b.GetUpdateHandler()
	if handler != nil {
		handler(update)
	}
}

// API returns the underlying Telegram Bot API client.
func (b *Bot) API() *tgbotapi.BotAPI {
	return b.api
}

// Token returns the bot token.
func (b *Bot) Token() string {
	return b.token
}

// SendMessage sends a text message to a chat.
func (b *Bot) SendMessage(chatID int64, text string) (tgbotapi.Message, error) {
	msg := tgbotapi.NewMessage(chatID, text)
	return b.api.Send(msg)
}

// SetWebhook configures the webhook URL for receiving updates.
func (b *Bot) SetWebhook(webhookURL string) error {
	wh, err := tgbotapi.NewWebhook(webhookURL)
	if err != nil {
		return fmt.Errorf("failed to create webhook config: %w", err)
	}

	_, err = b.api.Request(wh)
	if err != nil {
		return fmt.Errorf("failed to set webhook: %w", err)
	}

	return nil
}

// DeleteWebhook removes the current webhook.
func (b *Bot) DeleteWebhook() error {
	_, err := b.api.Request(tgbotapi.DeleteWebhookConfig{})
	if err != nil {
		return fmt.Errorf("failed to delete webhook: %w", err)
	}
	return nil
}

// GetWebhookInfo returns information about the current webhook.
func (b *Bot) GetWebhookInfo() (tgbotapi.WebhookInfo, error) {
	return b.api.GetWebhookInfo()
}

// BotInfo returns information about the bot.
func (b *Bot) BotInfo() tgbotapi.User {
	return b.api.Self
}

// SendDocument sends a document/file to a chat with an optional caption.
// The file is sent as an in-memory byte slice with the specified filename.
func (b *Bot) SendDocument(chatID int64, fileName string, fileData []byte, caption string) (tgbotapi.Message, error) {
	if b.api == nil {
		return tgbotapi.Message{}, fmt.Errorf("bot API not initialized")
	}

	// Create a file from bytes
	fileReader := tgbotapi.FileBytes{
		Name:  fileName,
		Bytes: fileData,
	}

	doc := tgbotapi.NewDocument(chatID, fileReader)
	if caption != "" {
		doc.Caption = caption
	}

	return b.api.Send(doc)
}

// SendDocumentReader sends a document from an io.Reader to a chat.
// This is useful when you have the file content as a stream.
func (b *Bot) SendDocumentReader(chatID int64, fileName string, reader *bytes.Reader, caption string) (tgbotapi.Message, error) {
	if b.api == nil {
		return tgbotapi.Message{}, fmt.Errorf("bot API not initialized")
	}

	fileReader := tgbotapi.FileReader{
		Name:   fileName,
		Reader: reader,
	}

	doc := tgbotapi.NewDocument(chatID, fileReader)
	if caption != "" {
		doc.Caption = caption
	}

	return b.api.Send(doc)
}
