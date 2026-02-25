package telegram

import (
	"testing"
)

func TestNewBotRequiresToken(t *testing.T) {
	_, err := New(Config{Token: ""})
	if err == nil {
		t.Error("Expected error when token is empty")
	}
}

func TestBotUpdateHandler(t *testing.T) {
	// Create a mock bot without actually connecting to Telegram
	// Since we can't create a real bot without a token, we test the handler logic separately
	bot := &Bot{}

	handlerCalled := false
	bot.SetUpdateHandler(func(update Update) {
		handlerCalled = true
	})

	handler := bot.GetUpdateHandler()
	if handler == nil {
		t.Fatal("Expected handler to be set")
	}

	// Call the handler directly
	handler(Update{})
	if !handlerCalled {
		t.Error("Expected handler to be called")
	}
}

func TestBotProcessUpdateWithNoHandler(t *testing.T) {
	bot := &Bot{}

	// Should not panic when no handler is set
	bot.ProcessUpdate(Update{})
}

func TestBotProcessUpdateWithHandler(t *testing.T) {
	bot := &Bot{}

	var receivedUpdate *Update
	bot.SetUpdateHandler(func(update Update) {
		receivedUpdate = &update
	})

	testUpdate := Update{UpdateID: 12345}
	bot.ProcessUpdate(testUpdate)

	if receivedUpdate == nil {
		t.Fatal("Expected update to be received by handler")
	}
	if receivedUpdate.UpdateID != 12345 {
		t.Errorf("Expected UpdateID 12345, got %d", receivedUpdate.UpdateID)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		cfg       Config
		wantError bool
	}{
		{
			name:      "empty token",
			cfg:       Config{Token: ""},
			wantError: true,
		},
		{
			name:      "whitespace token",
			cfg:       Config{Token: "   "},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.cfg)
			if tt.wantError && err == nil {
				t.Error("Expected error but got none")
			}
		})
	}
}

func TestSendDocumentWithNoAPI(t *testing.T) {
	// Test that SendDocument returns an error when bot API is not initialized
	bot := &Bot{}

	_, err := bot.SendDocument(12345, "test.txt", []byte("test content"), "caption")
	if err == nil {
		t.Error("Expected error when API is not initialized")
	}
}

func TestSendDocumentReaderWithNoAPI(t *testing.T) {
	// Test that SendDocumentReader returns an error when bot API is not initialized
	bot := &Bot{}

	_, err := bot.SendDocumentReader(12345, "test.txt", nil, "caption")
	if err == nil {
		t.Error("Expected error when API is not initialized")
	}
}
