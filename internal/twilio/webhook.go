package twilio

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// MessageHandler is a function that handles incoming SMS messages.
type MessageHandler func(sms *IncomingSMS)

// WebhookHandler handles incoming Twilio webhook requests.
type WebhookHandler struct {
	authToken      string
	validateSig    bool
	messageHandler MessageHandler
	externalURL    string // Optional external URL for signature validation behind proxies
}

// WebhookConfig holds webhook handler configuration.
type WebhookConfig struct {
	AuthToken         string // #nosec G117 - AuthToken is a required configuration parameter
	ValidateSignature bool
	ExternalURL       string // Optional: Public URL for signature validation (behind proxy)
}

// NewWebhookHandler creates a new webhook handler with the given configuration.
func NewWebhookHandler(cfg WebhookConfig) *WebhookHandler {
	return &WebhookHandler{
		authToken:   cfg.AuthToken,
		validateSig: cfg.ValidateSignature,
		externalURL: cfg.ExternalURL,
	}
}

// SetMessageHandler sets the handler for incoming SMS messages.
func (h *WebhookHandler) SetMessageHandler(handler MessageHandler) {
	h.messageHandler = handler
}

// HandleRequest processes an incoming webhook request and returns the parsed SMS.
// It validates the request and returns an error if validation fails.
func (h *WebhookHandler) HandleRequest(r *http.Request, requestURL string) (*IncomingSMS, error) {
	// Validate request method
	if r.Method != http.MethodPost {
		return nil, fmt.Errorf("method not allowed: %s", r.Method)
	}

	// Read and parse form data
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	defer func() { _ = r.Body.Close() }()

	form, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, fmt.Errorf("failed to parse form data: %w", err)
	}

	// Validate signature if enabled
	if h.validateSig && h.authToken != "" {
		signature := r.Header.Get("X-Twilio-Signature")
		params := make(map[string]string)
		for key, values := range form {
			if len(values) > 0 {
				params[key] = values[0]
			}
		}

		if !ValidateSignature(h.authToken, signature, requestURL, params) {
			return nil, fmt.Errorf("invalid signature")
		}
	}

	// Parse the incoming SMS
	sms := ParseIncomingSMS(form)
	return sms, nil
}

// ServeHTTP implements the http.Handler interface.
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestURL := h.buildRequestURL(r)

	sms, err := h.HandleRequest(r, requestURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Call the message handler if set
	if h.messageHandler != nil {
		h.messageHandler(sms)
	}

	// Return an empty TwiML response
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(NewTwiML().String()))
}

// buildRequestURL constructs the URL for signature validation.
// Supports proxy headers (X-Forwarded-*) and configurable external URL.
func (h *WebhookHandler) buildRequestURL(r *http.Request) string {
	// If external URL is configured, use it as the base
	if h.externalURL != "" {
		return h.externalURL + r.URL.Path
	}

	// Check for proxy headers
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}

	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}

	// Build the full URL
	return fmt.Sprintf("%s://%s%s", scheme, host, r.URL.Path)
}
