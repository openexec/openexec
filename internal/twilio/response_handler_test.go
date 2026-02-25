package twilio

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSessionState_String(t *testing.T) {
	tests := []struct {
		state    SessionState
		expected string
	}{
		{SessionStateUnknown, "unknown"},
		{SessionStateActive, "active"},
		{SessionStateExpired, "expired"},
		{SessionState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("SessionState.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestResponseType_String(t *testing.T) {
	tests := []struct {
		rtype    ResponseType
		expected string
	}{
		{ResponseTypeUnspecified, "unspecified"},
		{ResponseTypeTwiML, "twiml"},
		{ResponseTypeAPI, "api"},
		{ResponseTypeTemplate, "template"},
		{ResponseType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.rtype.String(); got != tt.expected {
				t.Errorf("ResponseType.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestUserSession_IsActive(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		session  UserSession
		expected bool
	}{
		{
			name: "active session",
			session: UserSession{
				State:            SessionStateActive,
				SessionExpiresAt: now.Add(1 * time.Hour),
			},
			expected: true,
		},
		{
			name: "expired session by time",
			session: UserSession{
				State:            SessionStateActive,
				SessionExpiresAt: now.Add(-1 * time.Hour),
			},
			expected: false,
		},
		{
			name: "expired session by state",
			session: UserSession{
				State:            SessionStateExpired,
				SessionExpiresAt: now.Add(1 * time.Hour),
			},
			expected: false,
		},
		{
			name: "unknown session",
			session: UserSession{
				State:            SessionStateUnknown,
				SessionExpiresAt: now.Add(1 * time.Hour),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.session.IsActive(); got != tt.expected {
				t.Errorf("UserSession.IsActive() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestUserSession_TimeRemaining(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		session        UserSession
		expectPositive bool
	}{
		{
			name: "active session with time remaining",
			session: UserSession{
				State:            SessionStateActive,
				SessionExpiresAt: now.Add(1 * time.Hour),
			},
			expectPositive: true,
		},
		{
			name: "expired session",
			session: UserSession{
				State:            SessionStateActive,
				SessionExpiresAt: now.Add(-1 * time.Hour),
			},
			expectPositive: false,
		},
		{
			name: "inactive session",
			session: UserSession{
				State:            SessionStateExpired,
				SessionExpiresAt: now.Add(1 * time.Hour),
			},
			expectPositive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remaining := tt.session.TimeRemaining()
			if tt.expectPositive && remaining <= 0 {
				t.Errorf("expected positive time remaining, got %v", remaining)
			}
			if !tt.expectPositive && remaining > 0 {
				t.Errorf("expected zero time remaining, got %v", remaining)
			}
		})
	}
}

func TestResponse_Validate(t *testing.T) {
	tests := []struct {
		name        string
		response    Response
		wantErr     bool
		errContains string
	}{
		{
			name: "valid text response",
			response: Response{
				Body: "Hello, World!",
			},
			wantErr: false,
		},
		{
			name: "valid media response",
			response: Response{
				MediaURLs: []string{"https://example.com/image.jpg"},
			},
			wantErr: false,
		},
		{
			name: "valid text and media response",
			response: Response{
				Body:      "Check this out!",
				MediaURLs: []string{"https://example.com/image.jpg"},
			},
			wantErr: false,
		},
		{
			name:        "empty response",
			response:    Response{},
			wantErr:     true,
			errContains: "must have body or media",
		},
		{
			name: "body exceeds limit",
			response: Response{
				Body: strings.Repeat("a", MaxWhatsAppMessageLength+1),
			},
			wantErr:     true,
			errContains: "exceeds",
		},
		{
			name: "body at exact limit",
			response: Response{
				Body: strings.Repeat("a", MaxWhatsAppMessageLength),
			},
			wantErr: false,
		},
		{
			name: "too many media attachments",
			response: Response{
				Body: "text",
				MediaURLs: []string{
					"url1", "url2", "url3", "url4", "url5",
					"url6", "url7", "url8", "url9", "url10", "url11",
				},
			},
			wantErr:     true,
			errContains: "too many media",
		},
		{
			name: "max media attachments",
			response: Response{
				Body: "text",
				MediaURLs: []string{
					"url1", "url2", "url3", "url4", "url5",
					"url6", "url7", "url8", "url9", "url10",
				},
			},
			wantErr: false,
		},
		{
			name: "template without ID",
			response: Response{
				Body: "text",
				Type: ResponseTypeTemplate,
			},
			wantErr:     true,
			errContains: "template ID required",
		},
		{
			name: "valid template response",
			response: Response{
				Body:       "text",
				Type:       ResponseTypeTemplate,
				TemplateID: "template123",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.response.Validate()
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestNewResponseHandler(t *testing.T) {
	// Without client
	h := NewResponseHandler(nil)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.sessions == nil {
		t.Error("expected sessions map to be initialized")
	}

	// With debug option
	h = NewResponseHandler(nil, WithDebug(true))
	if !h.debug {
		t.Error("expected debug to be true")
	}
}

func TestResponseHandler_RecordIncomingMessage(t *testing.T) {
	h := NewResponseHandler(nil)

	// Record a message
	h.RecordIncomingMessage("+15551234567")

	if h.SessionCount() != 1 {
		t.Errorf("expected 1 session, got %d", h.SessionCount())
	}

	// Verify session state
	state := h.GetSessionState("+15551234567")
	if state != SessionStateActive {
		t.Errorf("expected active state, got %s", state)
	}

	// Verify session details
	session := h.GetSession("+15551234567")
	if session == nil {
		t.Fatal("expected session to exist")
	}
	if !session.IsActive() {
		t.Error("expected session to be active")
	}

	// Test with whatsapp: prefix
	h.RecordIncomingMessage("whatsapp:+15559876543")
	if h.SessionCount() != 2 {
		t.Errorf("expected 2 sessions, got %d", h.SessionCount())
	}

	// Should be stored without prefix
	session = h.GetSession("+15559876543")
	if session == nil {
		t.Error("expected session for normalized phone number")
	}
}

func TestResponseHandler_GetSessionState(t *testing.T) {
	h := NewResponseHandler(nil)

	// Unknown number
	state := h.GetSessionState("+15551234567")
	if state != SessionStateUnknown {
		t.Errorf("expected unknown state for new number, got %s", state)
	}

	// Record a message
	h.RecordIncomingMessage("+15551234567")

	// Should be active
	state = h.GetSessionState("+15551234567")
	if state != SessionStateActive {
		t.Errorf("expected active state, got %s", state)
	}
}

func TestResponseHandler_CanSendFreeformMessage(t *testing.T) {
	h := NewResponseHandler(nil)

	// Unknown number - cannot send freeform
	if h.CanSendFreeformMessage("+15551234567") {
		t.Error("should not be able to send freeform to unknown number")
	}

	// Record a message
	h.RecordIncomingMessage("+15551234567")

	// Should be able to send freeform
	if !h.CanSendFreeformMessage("+15551234567") {
		t.Error("should be able to send freeform to active session")
	}
}

func TestResponseHandler_DetermineResponseType(t *testing.T) {
	h := NewResponseHandler(nil)

	// Immediate response should always be TwiML
	if rt := h.DetermineResponseType("+15551234567", true); rt != ResponseTypeTwiML {
		t.Errorf("expected TwiML for immediate response, got %s", rt)
	}

	// Unknown session should use template
	if rt := h.DetermineResponseType("+15551234567", false); rt != ResponseTypeTemplate {
		t.Errorf("expected template for unknown session, got %s", rt)
	}

	// Active session should use API
	h.RecordIncomingMessage("+15551234567")
	if rt := h.DetermineResponseType("+15551234567", false); rt != ResponseTypeAPI {
		t.Errorf("expected API for active session, got %s", rt)
	}
}

func TestResponseHandler_BuildTwiMLResponse(t *testing.T) {
	h := NewResponseHandler(nil)

	tests := []struct {
		name      string
		responses []*Response
		wantErr   bool
		contains  []string
	}{
		{
			name:      "empty responses",
			responses: nil,
			contains:  []string{"<Response>", "</Response>"},
		},
		{
			name:      "nil response",
			responses: []*Response{nil},
			contains:  []string{"<Response>", "</Response>"},
		},
		{
			name: "single message",
			responses: []*Response{
				{Body: "Hello!"},
			},
			contains: []string{"<Message>Hello!</Message>"},
		},
		{
			name: "multiple messages",
			responses: []*Response{
				{Body: "First"},
				{Body: "Second"},
			},
			contains: []string{"<Message>First</Message>", "<Message>Second</Message>"},
		},
		{
			name: "message with special characters",
			responses: []*Response{
				{Body: "<script>alert('xss')</script>"},
			},
			contains: []string{"&lt;script&gt;"},
		},
		{
			name: "invalid response",
			responses: []*Response{
				{Body: strings.Repeat("a", MaxWhatsAppMessageLength+1)},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := h.BuildTwiMLResponse(tt.responses...)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			for _, s := range tt.contains {
				if !strings.Contains(result, s) {
					t.Errorf("result %q does not contain %q", result, s)
				}
			}
		})
	}
}

func TestResponseHandler_SendResponse(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(MessageResponse{
			SID:    "SM123456",
			Status: "queued",
		})
	}))
	defer server.Close()

	tests := []struct {
		name        string
		response    *Response
		client      *Client
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil response",
			response:    nil,
			wantErr:     true,
			errContains: "response is required",
		},
		{
			name:        "invalid response",
			response:    &Response{},
			wantErr:     true,
			errContains: "invalid response",
		},
		{
			name: "TwiML response type",
			response: &Response{
				Body: "Hello",
				Type: ResponseTypeTwiML,
			},
			wantErr:     true,
			errContains: "TwiML responses should be returned",
		},
		{
			name: "API response without client",
			response: &Response{
				Body: "Hello",
				Type: ResponseTypeAPI,
			},
			wantErr:     true,
			errContains: "client not configured",
		},
		{
			name: "template response without client",
			response: &Response{
				Body:       "Hello",
				Type:       ResponseTypeTemplate,
				TemplateID: "template123",
			},
			wantErr:     true,
			errContains: "client not configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewResponseHandler(tt.client)
			h.RecordIncomingMessage("+15551234567")

			_, err := h.SendResponse("+15551234567", tt.response)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestSplitLongMessage(t *testing.T) {
	tests := []struct {
		name           string
		msg            string
		maxLen         int
		expectedParts  int
		firstPartLen   int
		lastPartNotice string
	}{
		{
			name:          "short message",
			msg:           "Hello",
			maxLen:        100,
			expectedParts: 1,
		},
		{
			name:          "message at limit",
			msg:           strings.Repeat("a", 100),
			maxLen:        100,
			expectedParts: 1,
		},
		{
			name:          "message exceeds limit",
			msg:           strings.Repeat("a", 150),
			maxLen:        100,
			expectedParts: 2,
		},
		{
			name:          "message with spaces",
			msg:           "This is a test message that should be split at a space boundary",
			maxLen:        30,
			expectedParts: 3,
		},
		{
			name:          "message with newlines",
			msg:           "Line one\nLine two\nLine three",
			maxLen:        15,
			expectedParts: 3,
		},
		{
			name:          "default max length",
			msg:           strings.Repeat("a", MaxWhatsAppMessageLength+100),
			maxLen:        0,
			expectedParts: 2,
		},
		{
			name:          "sentence boundary split",
			msg:           "First sentence. Second sentence. Third sentence.",
			maxLen:        25,
			expectedParts: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := SplitLongMessage(tt.msg, tt.maxLen)
			if len(parts) != tt.expectedParts {
				t.Errorf("expected %d parts, got %d", tt.expectedParts, len(parts))
				for i, p := range parts {
					t.Logf("Part %d (len %d): %q", i, len(p), p)
				}
			}

			// Verify all parts are within limit
			maxLen := tt.maxLen
			if maxLen <= 0 {
				maxLen = MaxWhatsAppMessageLength
			}
			for i, p := range parts {
				if len(p) > maxLen {
					t.Errorf("part %d exceeds max length: %d > %d", i, len(p), maxLen)
				}
			}

			// Verify combined length
			combined := strings.Join(parts, "")
			// Allow for trimmed spaces
			if len(combined) < len(strings.TrimSpace(tt.msg))-10 {
				t.Errorf("combined parts too short: got %d, want ~%d", len(combined), len(tt.msg))
			}
		})
	}
}

func TestFindSplitPoint(t *testing.T) {
	tests := []struct {
		name   string
		msg    string
		maxLen int
		minIdx int
		maxIdx int
	}{
		{
			name:   "short message",
			msg:    "Hello",
			maxLen: 100,
			minIdx: 5,
			maxIdx: 5,
		},
		{
			name:   "split at space",
			msg:    "Hello World Test",
			maxLen: 12,
			minIdx: 6,
			maxIdx: 12,
		},
		{
			name:   "split at newline",
			msg:    "Line one\nLine two",
			maxLen: 12,
			minIdx: 9,
			maxIdx: 12,
		},
		{
			name:   "no good split point",
			msg:    strings.Repeat("a", 100),
			maxLen: 50,
			minIdx: 50,
			maxIdx: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := findSplitPoint(tt.msg, tt.maxLen)
			if idx < tt.minIdx || idx > tt.maxIdx {
				t.Errorf("split point %d not in range [%d, %d]", idx, tt.minIdx, tt.maxIdx)
			}
		})
	}
}

func TestIsValidMediaType(t *testing.T) {
	tests := []struct {
		mimeType string
		expected bool
	}{
		// Valid types
		{"image/jpeg", true},
		{"image/png", true},
		{"image/gif", true},
		{"audio/mpeg", true},
		{"video/mp4", true},
		{"application/pdf", true},
		{"text/plain", true},

		// Case insensitive
		{"IMAGE/JPEG", true},
		{"Audio/MPEG", true},

		// Invalid types
		{"application/javascript", false},
		{"text/html", false},
		{"image/svg+xml", false},
		{"", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			if got := IsValidMediaType(tt.mimeType); got != tt.expected {
				t.Errorf("IsValidMediaType(%q) = %v, want %v", tt.mimeType, got, tt.expected)
			}
		})
	}
}

func TestResponseHandler_CleanupExpiredSessions(t *testing.T) {
	h := NewResponseHandler(nil)

	// Add some sessions
	h.RecordIncomingMessage("+15551234567")
	h.RecordIncomingMessage("+15559876543")

	// Manually expire one session
	h.mu.Lock()
	session := h.sessions["+15551234567"]
	session.SessionExpiresAt = time.Now().Add(-1 * time.Hour)
	h.mu.Unlock()

	// Cleanup
	count := h.CleanupExpiredSessions()
	if count != 1 {
		t.Errorf("expected 1 expired session cleaned, got %d", count)
	}

	if h.SessionCount() != 1 {
		t.Errorf("expected 1 session remaining, got %d", h.SessionCount())
	}

	// Active session should still exist
	if h.GetSession("+15559876543") == nil {
		t.Error("expected active session to remain")
	}

	// Expired session should be gone
	if h.GetSession("+15551234567") != nil {
		t.Error("expected expired session to be removed")
	}
}

func TestNormalizePhoneNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"+15551234567", "+15551234567"},
		{"whatsapp:+15551234567", "+15551234567"},
		{"", ""},
		{"whatsapp:", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizePhoneNumber(tt.input); got != tt.expected {
				t.Errorf("normalizePhoneNumber(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestConstants(t *testing.T) {
	// Verify important constants are set correctly
	if SessionWindowDuration != 24*time.Hour {
		t.Errorf("SessionWindowDuration = %v, want %v", SessionWindowDuration, 24*time.Hour)
	}

	if MaxMediaAttachments != 10 {
		t.Errorf("MaxMediaAttachments = %d, want 10", MaxMediaAttachments)
	}

	if MaxMediaSizeBytes != 16*1024*1024 {
		t.Errorf("MaxMediaSizeBytes = %d, want %d", MaxMediaSizeBytes, 16*1024*1024)
	}

	if TemplateMessageMarker != "template:" {
		t.Errorf("TemplateMessageMarker = %q, want %q", TemplateMessageMarker, "template:")
	}
}
