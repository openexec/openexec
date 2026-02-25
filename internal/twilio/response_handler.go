// Package twilio provides Twilio SMS/WhatsApp integration for the message gateway.
package twilio

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// WhatsApp session and message constraints based on Twilio API.
const (
	// SessionWindowDuration is the 24-hour window for session messaging.
	// After receiving a message, you can send freeform messages for 24 hours.
	SessionWindowDuration = 24 * time.Hour

	// MaxMediaAttachments is the maximum number of media items per WhatsApp message.
	MaxMediaAttachments = 10

	// MaxMediaSizeBytes is the maximum file size for WhatsApp media (16MB).
	MaxMediaSizeBytes = 16 * 1024 * 1024

	// TemplateMessageMarker indicates a template message prefix.
	TemplateMessageMarker = "template:"
)

// AllowedMediaTypes lists supported media MIME types for WhatsApp.
var AllowedMediaTypes = map[string]bool{
	// Images
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,

	// Audio
	"audio/aac":  true,
	"audio/mp4":  true,
	"audio/mpeg": true,
	"audio/amr":  true,
	"audio/ogg":  true,

	// Video
	"video/mp4":  true,
	"video/3gpp": true,

	// Documents
	"application/pdf":               true,
	"application/vnd.ms-powerpoint": true,
	"application/msword":            true,
	"application/vnd.ms-excel":      true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"text/plain": true,
}

// SessionState represents the state of a WhatsApp conversation session.
type SessionState int

const (
	// SessionStateUnknown means no session information is available.
	SessionStateUnknown SessionState = iota
	// SessionStateActive means we're within the 24-hour messaging window.
	SessionStateActive
	// SessionStateExpired means the 24-hour window has passed.
	SessionStateExpired
)

// String returns the string representation of the session state.
func (s SessionState) String() string {
	switch s {
	case SessionStateActive:
		return "active"
	case SessionStateExpired:
		return "expired"
	default:
		return "unknown"
	}
}

// UserSession tracks the WhatsApp session state for a user.
type UserSession struct {
	PhoneNumber      string
	LastMessageAt    time.Time
	SessionExpiresAt time.Time
	State            SessionState
}

// IsActive returns true if the session is currently active.
func (u *UserSession) IsActive() bool {
	return u.State == SessionStateActive && time.Now().Before(u.SessionExpiresAt)
}

// TimeRemaining returns the time remaining in the session window.
func (u *UserSession) TimeRemaining() time.Duration {
	if !u.IsActive() {
		return 0
	}
	remaining := time.Until(u.SessionExpiresAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ResponseType indicates how the response should be delivered.
type ResponseType int

const (
	// ResponseTypeUnspecified is the zero value indicating type not set.
	ResponseTypeUnspecified ResponseType = iota
	// ResponseTypeTwiML uses synchronous TwiML response (immediate).
	ResponseTypeTwiML
	// ResponseTypeAPI uses async API call (for delayed responses).
	ResponseTypeAPI
	// ResponseTypeTemplate uses pre-approved template message.
	ResponseTypeTemplate
)

// String returns the string representation of the response type.
func (r ResponseType) String() string {
	switch r {
	case ResponseTypeUnspecified:
		return "unspecified"
	case ResponseTypeTwiML:
		return "twiml"
	case ResponseTypeAPI:
		return "api"
	case ResponseTypeTemplate:
		return "template"
	default:
		return "unknown"
	}
}

// Response represents a WhatsApp message response to send.
type Response struct {
	Body       string       // Message body (max 1600 chars)
	MediaURLs  []string     // Optional media URLs
	Type       ResponseType // How to deliver the response
	TemplateID string       // Template ID if using template response
}

// Validate checks if the response is valid for WhatsApp.
func (r *Response) Validate() error {
	if r.Body == "" && len(r.MediaURLs) == 0 {
		return errors.New("response must have body or media")
	}

	if len(r.Body) > MaxWhatsAppMessageLength {
		return fmt.Errorf("body exceeds %d character limit (got %d)",
			MaxWhatsAppMessageLength, len(r.Body))
	}

	if len(r.MediaURLs) > MaxMediaAttachments {
		return fmt.Errorf("too many media attachments: max %d, got %d",
			MaxMediaAttachments, len(r.MediaURLs))
	}

	if r.Type == ResponseTypeTemplate && r.TemplateID == "" {
		return errors.New("template ID required for template response")
	}

	return nil
}

// ResponseHandler handles WhatsApp message responses with Twilio API constraints.
type ResponseHandler struct {
	client   *Client
	sessions map[string]*UserSession
	mu       sync.RWMutex
	debug    bool
}

// ResponseHandlerOption is a functional option for ResponseHandler.
type ResponseHandlerOption func(*ResponseHandler)

// WithDebug enables debug logging.
func WithDebug(debug bool) ResponseHandlerOption {
	return func(h *ResponseHandler) {
		h.debug = debug
	}
}

// NewResponseHandler creates a new WhatsApp response handler.
func NewResponseHandler(client *Client, opts ...ResponseHandlerOption) *ResponseHandler {
	h := &ResponseHandler{
		client:   client,
		sessions: make(map[string]*UserSession),
	}

	for _, opt := range opts {
		opt(h)
	}

	return h
}

// RecordIncomingMessage records an incoming message to update session state.
// This should be called when a message is received from a user.
func (h *ResponseHandler) RecordIncomingMessage(phoneNumber string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	normalized := normalizePhoneNumber(phoneNumber)
	now := time.Now()

	h.sessions[normalized] = &UserSession{
		PhoneNumber:      normalized,
		LastMessageAt:    now,
		SessionExpiresAt: now.Add(SessionWindowDuration),
		State:            SessionStateActive,
	}
}

// GetSessionState returns the current session state for a phone number.
func (h *ResponseHandler) GetSessionState(phoneNumber string) SessionState {
	h.mu.RLock()
	defer h.mu.RUnlock()

	normalized := normalizePhoneNumber(phoneNumber)
	session, exists := h.sessions[normalized]
	if !exists {
		return SessionStateUnknown
	}

	// Check if session has expired
	if time.Now().After(session.SessionExpiresAt) {
		return SessionStateExpired
	}

	return session.State
}

// GetSession returns the session info for a phone number.
func (h *ResponseHandler) GetSession(phoneNumber string) *UserSession {
	h.mu.RLock()
	defer h.mu.RUnlock()

	normalized := normalizePhoneNumber(phoneNumber)
	session, exists := h.sessions[normalized]
	if !exists {
		return nil
	}

	// Return a copy to avoid race conditions
	sessionCopy := *session
	return &sessionCopy
}

// CanSendFreeformMessage checks if we can send a freeform (non-template) message.
func (h *ResponseHandler) CanSendFreeformMessage(phoneNumber string) bool {
	state := h.GetSessionState(phoneNumber)
	return state == SessionStateActive
}

// DetermineResponseType determines the best response type for a message.
func (h *ResponseHandler) DetermineResponseType(phoneNumber string, isImmediate bool) ResponseType {
	// If we're responding immediately to a webhook, use TwiML
	if isImmediate {
		return ResponseTypeTwiML
	}

	// Check session state for async responses
	state := h.GetSessionState(phoneNumber)
	switch state {
	case SessionStateActive:
		return ResponseTypeAPI
	case SessionStateExpired, SessionStateUnknown:
		return ResponseTypeTemplate
	default:
		return ResponseTypeTemplate
	}
}

// BuildTwiMLResponse creates a TwiML response for immediate webhook response.
func (h *ResponseHandler) BuildTwiMLResponse(responses ...*Response) (string, error) {
	twiml := NewTwiML()

	for _, resp := range responses {
		if resp == nil {
			continue
		}

		if err := resp.Validate(); err != nil {
			return "", fmt.Errorf("invalid response: %w", err)
		}

		// TwiML Message element
		if resp.Body != "" {
			twiml.Message(resp.Body)
		}
	}

	return twiml.String(), nil
}

// SendResponse sends a response via the appropriate channel.
func (h *ResponseHandler) SendResponse(phoneNumber string, resp *Response) (*MessageResponse, error) {
	if resp == nil {
		return nil, errors.New("response is required")
	}

	if err := resp.Validate(); err != nil {
		return nil, fmt.Errorf("invalid response: %w", err)
	}

	// Determine response type if not specified
	if resp.Type == ResponseTypeUnspecified {
		resp.Type = h.DetermineResponseType(phoneNumber, false)
	}

	switch resp.Type {
	case ResponseTypeTwiML:
		// TwiML responses are returned synchronously, not sent via API
		return nil, errors.New("TwiML responses should be returned via BuildTwiMLResponse")

	case ResponseTypeAPI:
		return h.sendAPIResponse(phoneNumber, resp)

	case ResponseTypeTemplate:
		return h.sendTemplateResponse(phoneNumber, resp)

	default:
		return nil, fmt.Errorf("unknown response type: %v", resp.Type)
	}
}

// sendAPIResponse sends a response via the Twilio API.
func (h *ResponseHandler) sendAPIResponse(phoneNumber string, resp *Response) (*MessageResponse, error) {
	if h.client == nil {
		return nil, errors.New("client not configured")
	}

	req := SendMessageRequest{
		To:       phoneNumber,
		Body:     resp.Body,
		MediaURL: resp.MediaURLs,
	}

	return h.client.SendWhatsAppMessage(req)
}

// sendTemplateResponse sends a template-based response.
// Note: Full template support requires additional Twilio Content API integration.
func (h *ResponseHandler) sendTemplateResponse(phoneNumber string, resp *Response) (*MessageResponse, error) {
	if h.client == nil {
		return nil, errors.New("client not configured")
	}

	// For basic template support, we send the template ID as the body
	// Full implementation would use Twilio's Content API
	body := resp.Body
	if resp.TemplateID != "" {
		body = TemplateMessageMarker + resp.TemplateID
	}

	req := SendMessageRequest{
		To:       phoneNumber,
		Body:     body,
		MediaURL: resp.MediaURLs,
	}

	return h.client.SendWhatsAppMessage(req)
}

// SplitLongMessage splits a message that exceeds the character limit into multiple parts.
func SplitLongMessage(msg string, maxLen int) []string {
	if maxLen <= 0 {
		maxLen = MaxWhatsAppMessageLength
	}

	if len(msg) <= maxLen {
		return []string{msg}
	}

	var parts []string
	remaining := msg

	for len(remaining) > 0 {
		if len(remaining) <= maxLen {
			parts = append(parts, remaining)
			break
		}

		// Try to split at a natural break point (space, newline)
		splitPoint := findSplitPoint(remaining, maxLen)
		parts = append(parts, strings.TrimSpace(remaining[:splitPoint]))
		remaining = strings.TrimSpace(remaining[splitPoint:])
	}

	return parts
}

// findSplitPoint finds the best point to split a message.
func findSplitPoint(msg string, maxLen int) int {
	if len(msg) <= maxLen {
		return len(msg)
	}

	// Look for natural break points near the end of the allowed length
	// Check for newlines first
	if idx := strings.LastIndex(msg[:maxLen], "\n"); idx > maxLen/2 {
		return idx + 1
	}

	// Check for sentence endings
	for i := maxLen - 1; i > maxLen/2; i-- {
		if msg[i] == '.' || msg[i] == '!' || msg[i] == '?' {
			if i+1 < len(msg) && msg[i+1] == ' ' {
				return i + 1
			}
		}
	}

	// Check for spaces
	if idx := strings.LastIndex(msg[:maxLen], " "); idx > maxLen/2 {
		return idx + 1
	}

	// Hard split at maxLen
	return maxLen
}

// normalizePhoneNumber normalizes a phone number by stripping the whatsapp: prefix.
func normalizePhoneNumber(phone string) string {
	return StripWhatsAppPrefix(phone)
}

// IsValidMediaType checks if a MIME type is supported for WhatsApp media.
func IsValidMediaType(mimeType string) bool {
	return AllowedMediaTypes[strings.ToLower(mimeType)]
}

// CleanupExpiredSessions removes expired sessions from the handler.
// This should be called periodically to prevent memory leaks.
func (h *ResponseHandler) CleanupExpiredSessions() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	count := 0

	for phone, session := range h.sessions {
		if now.After(session.SessionExpiresAt) {
			delete(h.sessions, phone)
			count++
		}
	}

	return count
}

// SessionCount returns the number of tracked sessions (for testing/monitoring).
func (h *ResponseHandler) SessionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.sessions)
}
