package twilio

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewWebhookHandler(t *testing.T) {
	cfg := WebhookConfig{
		AuthToken:         "token123",
		ValidateSignature: true,
	}
	handler := NewWebhookHandler(cfg)

	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
	if handler.authToken != cfg.AuthToken {
		t.Errorf("expected authToken %q, got %q", cfg.AuthToken, handler.authToken)
	}
	if handler.validateSig != cfg.ValidateSignature {
		t.Errorf("expected validateSig %v, got %v", cfg.ValidateSignature, handler.validateSig)
	}
}

func TestWebhookHandler_SetMessageHandler(t *testing.T) {
	handler := NewWebhookHandler(WebhookConfig{})

	var receivedSMS *IncomingSMS
	handler.SetMessageHandler(func(sms *IncomingSMS) {
		receivedSMS = sms
	})

	if handler.messageHandler == nil {
		t.Error("expected messageHandler to be set")
	}

	// Test that handler is called
	testSMS := &IncomingSMS{MessageSID: "SM123"}
	handler.messageHandler(testSMS)
	if receivedSMS != testSMS {
		t.Error("messageHandler was not called correctly")
	}
}

func TestWebhookHandler_HandleRequest(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		body        string
		validateSig bool
		wantErr     bool
		errContains string
	}{
		{
			name:        "valid POST request",
			method:      http.MethodPost,
			body:        "MessageSid=SM123&From=%2B15551234567&To=%2B15559876543&Body=Hello",
			validateSig: false,
			wantErr:     false,
		},
		{
			name:        "GET method not allowed",
			method:      http.MethodGet,
			body:        "",
			validateSig: false,
			wantErr:     true,
			errContains: "method not allowed",
		},
		{
			name:        "PUT method not allowed",
			method:      http.MethodPut,
			body:        "MessageSid=SM123",
			validateSig: false,
			wantErr:     true,
			errContains: "method not allowed",
		},
		{
			name:        "invalid signature when validation enabled",
			method:      http.MethodPost,
			body:        "MessageSid=SM123&From=%2B15551234567&To=%2B15559876543&Body=Hello",
			validateSig: true,
			wantErr:     true,
			errContains: "invalid signature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewWebhookHandler(WebhookConfig{
				AuthToken:         "testtoken",
				ValidateSignature: tt.validateSig,
			})

			req := httptest.NewRequest(tt.method, "https://example.com/webhook", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			sms, err := handler.HandleRequest(req, "https://example.com/webhook")

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if sms == nil {
				t.Error("expected non-nil SMS")
				return
			}

			if sms.MessageSID != "SM123" {
				t.Errorf("expected MessageSID %q, got %q", "SM123", sms.MessageSID)
			}
		})
	}
}

func TestWebhookHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		body           string
		validateSig    bool
		expectedStatus int
		expectedType   string
	}{
		{
			name:           "valid request returns TwiML",
			method:         http.MethodPost,
			body:           "MessageSid=SM123&From=%2B15551234567&To=%2B15559876543&Body=Hello",
			validateSig:    false,
			expectedStatus: http.StatusOK,
			expectedType:   "application/xml",
		},
		{
			name:           "invalid method returns error",
			method:         http.MethodGet,
			body:           "",
			validateSig:    false,
			expectedStatus: http.StatusBadRequest,
			expectedType:   "text/plain; charset=utf-8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewWebhookHandler(WebhookConfig{
				AuthToken:         "testtoken",
				ValidateSignature: tt.validateSig,
			})

			var handlerCalled bool
			handler.SetMessageHandler(func(sms *IncomingSMS) {
				handlerCalled = true
			})

			req := httptest.NewRequest(tt.method, "http://example.com/webhook", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			contentType := rr.Header().Get("Content-Type")
			if contentType != tt.expectedType {
				t.Errorf("expected Content-Type %q, got %q", tt.expectedType, contentType)
			}

			if tt.expectedStatus == http.StatusOK {
				if !handlerCalled {
					t.Error("expected message handler to be called")
				}
				if !strings.Contains(rr.Body.String(), "<Response>") {
					t.Error("expected TwiML response")
				}
			}
		})
	}
}

func TestWebhookHandler_ServeHTTP_NoHandler(t *testing.T) {
	handler := NewWebhookHandler(WebhookConfig{
		ValidateSignature: false,
	})
	// Don't set message handler

	req := httptest.NewRequest(http.MethodPost, "http://example.com/webhook",
		strings.NewReader("MessageSid=SM123&From=%2B15551234567&Body=Test"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should still return 200 OK even without handler
	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestWebhookHandler_HandleRequest_ParsesSMS(t *testing.T) {
	handler := NewWebhookHandler(WebhookConfig{
		ValidateSignature: false,
	})

	body := "MessageSid=SM789&From=%2B15551111111&To=%2B15552222222&Body=Test%20message&NumMedia=1&MediaUrl0=https%3A%2F%2Fexample.com%2Fimage.jpg"
	req := httptest.NewRequest(http.MethodPost, "https://example.com/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	sms, err := handler.HandleRequest(req, "https://example.com/webhook")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sms.MessageSID != "SM789" {
		t.Errorf("expected MessageSID %q, got %q", "SM789", sms.MessageSID)
	}
	if sms.From != "+15551111111" {
		t.Errorf("expected From %q, got %q", "+15551111111", sms.From)
	}
	if sms.To != "+15552222222" {
		t.Errorf("expected To %q, got %q", "+15552222222", sms.To)
	}
	if sms.Body != "Test message" {
		t.Errorf("expected Body %q, got %q", "Test message", sms.Body)
	}
	if sms.NumMedia != "1" {
		t.Errorf("expected NumMedia %q, got %q", "1", sms.NumMedia)
	}
	if len(sms.MediaURLs) != 1 || sms.MediaURLs[0] != "https://example.com/image.jpg" {
		t.Errorf("expected MediaURLs [%q], got %v", "https://example.com/image.jpg", sms.MediaURLs)
	}
}
