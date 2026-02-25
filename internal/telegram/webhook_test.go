package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/openexec/openexec/internal/user"
)

func TestWebhookHandlerValidatesMethod(t *testing.T) {
	bot := &Bot{}
	handler := NewWebhookHandler(bot, "")

	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	_, err := handler.HandleUpdate(req)

	if err == nil {
		t.Error("Expected error for GET request")
	}
}

func TestWebhookHandlerValidatesSecretToken(t *testing.T) {
	bot := &Bot{}
	handler := NewWebhookHandler(bot, "secret-token")

	// Test with missing token
	body := bytes.NewBufferString(`{"update_id": 123}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook", body)

	_, err := handler.HandleUpdate(req)
	if err == nil {
		t.Error("Expected error for missing secret token")
	}

	// Test with wrong token
	body = bytes.NewBufferString(`{"update_id": 123}`)
	req = httptest.NewRequest(http.MethodPost, "/webhook", body)
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "wrong-token")

	_, err = handler.HandleUpdate(req)
	if err == nil {
		t.Error("Expected error for wrong secret token")
	}
}

func TestWebhookHandlerAcceptsValidRequest(t *testing.T) {
	bot := &Bot{}
	handler := NewWebhookHandler(bot, "secret-token")

	update := Update{UpdateID: 12345}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBuffer(body))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "secret-token")

	result, err := handler.HandleUpdate(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.UpdateID != 12345 {
		t.Errorf("Expected UpdateID 12345, got %d", result.UpdateID)
	}
}

func TestWebhookHandlerWithoutSecretToken(t *testing.T) {
	bot := &Bot{}
	handler := NewWebhookHandler(bot, "") // No secret token configured

	update := Update{UpdateID: 67890}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBuffer(body))

	result, err := handler.HandleUpdate(req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.UpdateID != 67890 {
		t.Errorf("Expected UpdateID 67890, got %d", result.UpdateID)
	}
}

func TestWebhookHandlerRejectsInvalidJSON(t *testing.T) {
	bot := &Bot{}
	handler := NewWebhookHandler(bot, "")

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString("invalid json"))

	_, err := handler.HandleUpdate(req)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestWebhookHandlerServeHTTP(t *testing.T) {
	bot := &Bot{}

	var receivedUpdate *Update
	bot.SetUpdateHandler(func(update Update) {
		receivedUpdate = &update
	})

	handler := NewWebhookHandler(bot, "")

	update := Update{UpdateID: 99999}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	if receivedUpdate == nil {
		t.Fatal("Expected update to be processed")
	}

	if receivedUpdate.UpdateID != 99999 {
		t.Errorf("Expected UpdateID 99999, got %d", receivedUpdate.UpdateID)
	}
}

func TestWebhookHandlerServeHTTPRejectsInvalidRequest(t *testing.T) {
	bot := &Bot{}
	handler := NewWebhookHandler(bot, "")

	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}
}

func TestWebhookHandlerBotAccessor(t *testing.T) {
	bot := &Bot{}
	handler := NewWebhookHandler(bot, "")

	if handler.Bot() != bot {
		t.Error("Expected Bot() to return the bot instance")
	}
}

func TestWebhookHandlerWithAuth(t *testing.T) {
	store := user.NewMockStore()
	auth := NewAuthMiddleware(store)
	bot := &Bot{}

	handler := NewWebhookHandler(bot, "").WithAuth(auth)

	if handler.authMiddleware != auth {
		t.Error("Expected auth middleware to be set")
	}
}

func TestWebhookHandlerServeHTTPWithAuthAllowed(t *testing.T) {
	store := user.NewMockStore()
	ctx := context.Background()

	// Create allowlisted user
	allowedUser, _ := user.NewUser(12345, user.RoleCustomer)
	store.Create(ctx, allowedUser)

	auth := NewAuthMiddleware(store)
	bot := &Bot{}

	var receivedUpdate *Update
	bot.SetUpdateHandler(func(update Update) {
		receivedUpdate = &update
	})

	handler := NewWebhookHandler(bot, "").WithAuth(auth)

	update := Update{
		UpdateID: 11111,
		Message: &tgbotapi.Message{
			From: &tgbotapi.User{ID: 12345},
		},
	}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	if receivedUpdate == nil {
		t.Fatal("Expected update to be processed")
	}
}

func TestWebhookHandlerServeHTTPWithAuthDenied(t *testing.T) {
	store := user.NewMockStore()
	auth := NewAuthMiddleware(store)
	bot := &Bot{}

	var receivedUpdate *Update
	bot.SetUpdateHandler(func(update Update) {
		receivedUpdate = &update
	})

	handler := NewWebhookHandler(bot, "").WithAuth(auth)

	// User not in allowlist
	update := Update{
		UpdateID: 22222,
		Message: &tgbotapi.Message{
			From: &tgbotapi.User{ID: 99999},
		},
	}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rec.Code)
	}

	if receivedUpdate != nil {
		t.Error("Expected update to NOT be processed")
	}
}

func TestWebhookHandlerServeHTTPWithAuthNoUser(t *testing.T) {
	store := user.NewMockStore()
	auth := NewAuthMiddleware(store)
	bot := &Bot{}

	handler := NewWebhookHandler(bot, "").WithAuth(auth)

	// Update with no user
	update := Update{UpdateID: 33333}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rec.Code)
	}
}

func TestWebhookHandlerServeHTTPWithoutAuthMiddleware(t *testing.T) {
	bot := &Bot{}

	var receivedUpdate *Update
	bot.SetUpdateHandler(func(update Update) {
		receivedUpdate = &update
	})

	// No auth middleware - should allow all
	handler := NewWebhookHandler(bot, "")

	update := Update{
		UpdateID: 44444,
		Message: &tgbotapi.Message{
			From: &tgbotapi.User{ID: 99999},
		},
	}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	if receivedUpdate == nil {
		t.Fatal("Expected update to be processed")
	}
}
