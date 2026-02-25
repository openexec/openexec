// Package twilio provides Twilio SMS integration for the message gateway.
package twilio

import (
	"crypto/hmac"
	"crypto/sha1" // #nosec G505 - SHA1 is required for Twilio HMAC signature validation
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// Client represents a Twilio API client.
type Client struct {
	accountSID string
	authToken  string
	fromNumber string
	httpClient *http.Client
}

// Config holds Twilio client configuration.
type Config struct {
	AccountSID string
	AuthToken  string // #nosec G117 - AuthToken is a required configuration parameter
	FromNumber string
}

// NewClient creates a new Twilio client with the given configuration.
func NewClient(cfg Config) (*Client, error) {
	if cfg.AccountSID == "" {
		return nil, errors.New("account SID is required")
	}
	if cfg.AuthToken == "" {
		return nil, errors.New("auth token is required")
	}
	if cfg.FromNumber == "" {
		return nil, errors.New("from number is required")
	}

	return &Client{
		accountSID: cfg.AccountSID,
		authToken:  cfg.AuthToken,
		fromNumber: cfg.FromNumber,
		httpClient: &http.Client{},
	}, nil
}

// AccountSID returns the configured account SID.
func (c *Client) AccountSID() string {
	return c.accountSID
}

// FromNumber returns the configured sender phone number.
func (c *Client) FromNumber() string {
	return c.fromNumber
}

// ValidateSignature validates the X-Twilio-Signature header to verify
// that a request came from Twilio.
func (c *Client) ValidateSignature(signature, requestURL string, params map[string]string) bool {
	return ValidateSignature(c.authToken, signature, requestURL, params)
}

// ValidateSignature validates a Twilio webhook signature.
// This is a standalone function for use without a client instance.
func ValidateSignature(authToken, signature, requestURL string, params map[string]string) bool {
	if signature == "" || authToken == "" {
		return false
	}

	// Build the data string: URL + sorted parameters
	data := requestURL

	// Sort parameter keys
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Append key-value pairs
	for _, k := range keys {
		data += k + params[k]
	}

	// Calculate HMAC-SHA1
	mac := hmac.New(sha1.New, []byte(authToken))
	mac.Write([]byte(data))
	expectedSig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expectedSig))
}

// IncomingSMS represents an incoming SMS message from Twilio.
type IncomingSMS struct {
	MessageSID string
	From       string
	To         string
	Body       string
	NumMedia   string
	MediaURLs  []string
}

// ParseIncomingSMS parses form values from a Twilio webhook request into an IncomingSMS.
func ParseIncomingSMS(form url.Values) *IncomingSMS {
	sms := &IncomingSMS{
		MessageSID: form.Get("MessageSid"),
		From:       form.Get("From"),
		To:         form.Get("To"),
		Body:       form.Get("Body"),
		NumMedia:   form.Get("NumMedia"),
	}

	// Parse media URLs if present
	numMedia := 0
	_, _ = fmt.Sscanf(sms.NumMedia, "%d", &numMedia)
	for i := 0; i < numMedia; i++ {
		mediaURL := form.Get(fmt.Sprintf("MediaUrl%d", i))
		if mediaURL != "" {
			sms.MediaURLs = append(sms.MediaURLs, mediaURL)
		}
	}

	return sms
}

// TwiML represents a TwiML response builder.
type TwiML struct {
	content strings.Builder
}

// NewTwiML creates a new TwiML response builder.
func NewTwiML() *TwiML {
	t := &TwiML{}
	t.content.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<Response>")
	return t
}

// Message adds a Message element to the TwiML response.
func (t *TwiML) Message(body string) *TwiML {
	t.content.WriteString("<Message>")
	t.content.WriteString(escapeXML(body))
	t.content.WriteString("</Message>")
	return t
}

// String returns the complete TwiML response as a string.
func (t *TwiML) String() string {
	return t.content.String() + "</Response>"
}

// escapeXML escapes special XML characters.
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// WhatsApp-specific constants based on Twilio API constraints.
const (
	// MaxWhatsAppMessageLength is the maximum length for a WhatsApp message body.
	// Twilio enforces a 1600 character limit for WhatsApp messages.
	MaxWhatsAppMessageLength = 1600

	// WhatsAppPrefix is the prefix required for WhatsApp phone numbers.
	WhatsAppPrefix = "whatsapp:"

	// TwilioAPIBaseURL is the base URL for Twilio's REST API.
	TwilioAPIBaseURL = "https://api.twilio.com/2010-04-01"
)

// MessageResponse represents the response from Twilio after sending a message.
type MessageResponse struct {
	SID          string `json:"sid"`
	Status       string `json:"status"`
	To           string `json:"to,omitempty"`
	From         string `json:"from,omitempty"`
	Body         string `json:"body,omitempty"`
	DateCreated  string `json:"date_created,omitempty"`
	ErrorCode    int    `json:"error_code,omitempty"`
	ErrorMessage string `json:"message,omitempty"`
}

// SendMessageRequest contains the parameters for sending a message.
type SendMessageRequest struct {
	To       string   // Recipient phone number (with or without whatsapp: prefix)
	Body     string   // Message body (max 1600 chars for WhatsApp)
	MediaURL []string // Optional media URLs for MMS/WhatsApp media messages
}

// SendMessage sends an SMS or WhatsApp message via the Twilio API.
// For WhatsApp messages, the To number should include the "whatsapp:" prefix,
// or call SendWhatsAppMessage instead.
func (c *Client) SendMessage(req SendMessageRequest) (*MessageResponse, error) {
	if req.To == "" {
		return nil, errors.New("recipient (To) is required")
	}
	if req.Body == "" && len(req.MediaURL) == 0 {
		return nil, errors.New("message body or media URL is required")
	}

	// Build form data
	form := url.Values{}
	form.Set("To", req.To)
	form.Set("From", c.fromNumber)
	form.Set("Body", req.Body)

	// Add media URLs if present
	for i, mediaURL := range req.MediaURL {
		form.Set(fmt.Sprintf("MediaUrl%d", i), mediaURL)
	}

	return c.sendMessageRequest(form)
}

// SendWhatsAppMessage sends a WhatsApp message via the Twilio API.
// This method automatically handles the whatsapp: prefix and enforces
// WhatsApp-specific constraints.
func (c *Client) SendWhatsAppMessage(req SendMessageRequest) (*MessageResponse, error) {
	if req.To == "" {
		return nil, errors.New("recipient (To) is required")
	}
	if req.Body == "" && len(req.MediaURL) == 0 {
		return nil, errors.New("message body or media URL is required")
	}

	// Validate message length for WhatsApp
	if len(req.Body) > MaxWhatsAppMessageLength {
		return nil, fmt.Errorf("message body exceeds WhatsApp limit of %d characters (got %d)",
			MaxWhatsAppMessageLength, len(req.Body))
	}

	// Ensure To number has whatsapp: prefix
	to := req.To
	if !strings.HasPrefix(to, WhatsAppPrefix) {
		to = WhatsAppPrefix + to
	}

	// Ensure From number has whatsapp: prefix for WhatsApp
	from := c.fromNumber
	if !strings.HasPrefix(from, WhatsAppPrefix) {
		from = WhatsAppPrefix + from
	}

	// Build form data
	form := url.Values{}
	form.Set("To", to)
	form.Set("From", from)
	form.Set("Body", req.Body)

	// Add media URLs if present
	for i, mediaURL := range req.MediaURL {
		form.Set(fmt.Sprintf("MediaUrl%d", i), mediaURL)
	}

	return c.sendMessageRequest(form)
}

// sendMessageRequest performs the actual HTTP request to send a message.
func (c *Client) sendMessageRequest(form url.Values) (*MessageResponse, error) {
	apiURL := fmt.Sprintf("%s/Accounts/%s/Messages.json", TwilioAPIBaseURL, c.accountSID)

	httpReq, err := http.NewRequest(http.MethodPost, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.SetBasicAuth(c.accountSID, c.authToken)

	resp, err := c.httpClient.Do(httpReq) // #nosec G704 - URL is constructed safely from config
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse JSON response
	var msgResp MessageResponse
	if err := json.Unmarshal(body, &msgResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for API errors
	if resp.StatusCode >= 400 {
		if msgResp.ErrorMessage != "" {
			return nil, fmt.Errorf("twilio API error (code %d): %s", msgResp.ErrorCode, msgResp.ErrorMessage)
		}
		return nil, fmt.Errorf("twilio API returned status %d", resp.StatusCode)
	}

	return &msgResp, nil
}

// FormatWhatsAppNumber ensures a phone number has the whatsapp: prefix.
func FormatWhatsAppNumber(phone string) string {
	if strings.HasPrefix(phone, WhatsAppPrefix) {
		return phone
	}
	return WhatsAppPrefix + phone
}

// StripWhatsAppPrefix removes the whatsapp: prefix from a phone number.
func StripWhatsAppPrefix(phone string) string {
	return strings.TrimPrefix(phone, WhatsAppPrefix)
}

// TruncateMessage truncates a message to fit within WhatsApp's character limit.
// If the message exceeds the limit, it appends "..." at the end.
func TruncateMessage(msg string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = MaxWhatsAppMessageLength
	}
	if len(msg) <= maxLen {
		return msg
	}
	if maxLen <= 3 {
		return msg[:maxLen]
	}
	return msg[:maxLen-3] + "..."
}
