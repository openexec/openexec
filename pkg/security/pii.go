// Package security provides security-related utilities including PII detection and redaction.
package security

import (
	"regexp"
	"strings"

	"github.com/openexec/openexec/pkg/agent"
)

// PIIType represents the type of PII detected.
type PIIType string

const (
	PIITypeEmail      PIIType = "EMAIL"
	PIITypePhone      PIIType = "PHONE"
	PIITypeSSN        PIIType = "SSN"
	PIITypeCreditCard PIIType = "CREDIT_CARD"
	PIITypeAPIKey     PIIType = "API_KEY"
	PIITypeIPAddress  PIIType = "IP_ADDRESS"
	PIITypeName       PIIType = "NAME"
)

// PIIMatch represents a detected PII pattern with location information.
type PIIMatch struct {
	Type    PIIType
	Value   string
	Start   int
	End     int
	Pattern string // The pattern that matched
}

// PIIScrubber handles detection and redaction of personally identifiable information.
type PIIScrubber struct {
	// Level controls sensitivity: "low", "medium", "high"
	// - low: Only detect high-confidence patterns (email, SSN, credit card)
	// - medium: Add phone numbers, API keys, IP addresses
	// - high: Add name detection (more false positives)
	Level string

	patterns map[PIIType]*regexp.Regexp
}

// NewPIIScrubber creates a scrubber with the given sensitivity level.
// Valid levels: "low", "medium", "high". Defaults to "medium" if invalid.
func NewPIIScrubber(level string) *PIIScrubber {
	if level != "low" && level != "medium" && level != "high" {
		level = "medium"
	}

	s := &PIIScrubber{
		Level:    level,
		patterns: make(map[PIIType]*regexp.Regexp),
	}
	s.initPatterns()
	return s
}

// initPatterns initializes regex patterns based on sensitivity level.
func (s *PIIScrubber) initPatterns() {
	// Always active (low, medium, high)
	// Email: standard email pattern
	s.patterns[PIITypeEmail] = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)

	// SSN: XXX-XX-XXXX pattern
	s.patterns[PIITypeSSN] = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)

	// Credit card: 13-19 digits, optionally separated by spaces or dashes
	// Matches formats like: 4111111111111111, 4111-1111-1111-1111, 4111 1111 1111 1111
	s.patterns[PIITypeCreditCard] = regexp.MustCompile(`\b(?:\d{4}[\s\-]?){3,4}\d{1,4}\b`)

	if s.Level == "low" {
		return
	}

	// Medium level additions
	// Phone: various formats including international
	s.patterns[PIITypePhone] = regexp.MustCompile(`(?:\+\d{1,3}[\s\-]?)?\(?\d{3}\)?[\s\-]?\d{3}[\s\-]?\d{4}\b`)

	// API keys: common patterns
	// Matches: sk-..., api_key_..., apikey-..., key_..., secret_..., token_...
	s.patterns[PIITypeAPIKey] = regexp.MustCompile(`\b(?:sk-[a-zA-Z0-9]{20,}|api[_\-]?key[_\-]?[a-zA-Z0-9]{16,}|secret[_\-][a-zA-Z0-9]{16,}|token[_\-][a-zA-Z0-9]{16,}|key[_\-][a-zA-Z0-9]{20,}|ghp_[a-zA-Z0-9]{36,}|gho_[a-zA-Z0-9]{36,}|github_pat_[a-zA-Z0-9_]{22,}|xox[baprs]-[a-zA-Z0-9\-]+)\b`)

	// IP addresses: IPv4
	s.patterns[PIITypeIPAddress] = regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`)

	if s.Level != "high" {
		return
	}

	// High level additions
	// Names: Basic pattern for capitalized words that look like names
	// This has higher false positive rate but catches more names
	// Matches: "John Smith", "Mary Jane Watson"
	s.patterns[PIITypeName] = regexp.MustCompile(`\b[A-Z][a-z]+(?:\s+[A-Z][a-z]+)+\b`)
}

// ScrubText redacts PII from plain text, returning sanitized text.
func (s *PIIScrubber) ScrubText(text string) string {
	result := text
	for piiType, pattern := range s.patterns {
		replacement := "[REDACTED:" + string(piiType) + "]"
		result = pattern.ReplaceAllString(result, replacement)
	}
	return result
}

// ScrubMap redacts PII from a map of strings (for artifacts/metadata).
func (s *PIIScrubber) ScrubMap(data map[string]string) map[string]string {
	result := make(map[string]string, len(data))
	for key, value := range data {
		result[key] = s.ScrubText(value)
	}
	return result
}

// ScrubMessages redacts PII from LLM message content.
func (s *PIIScrubber) ScrubMessages(messages []agent.Message) []agent.Message {
	result := make([]agent.Message, len(messages))
	for i, msg := range messages {
		result[i] = s.scrubMessage(msg)
	}
	return result
}

// scrubMessage redacts PII from a single message.
func (s *PIIScrubber) scrubMessage(msg agent.Message) agent.Message {
	scrubbed := agent.Message{
		Role: msg.Role,
		Text: s.ScrubText(msg.Text),
	}

	if len(msg.Content) > 0 {
		scrubbed.Content = make([]agent.ContentBlock, len(msg.Content))
		for i, block := range msg.Content {
			scrubbed.Content[i] = s.scrubContentBlock(block)
		}
	}

	return scrubbed
}

// scrubContentBlock redacts PII from a content block.
func (s *PIIScrubber) scrubContentBlock(block agent.ContentBlock) agent.ContentBlock {
	scrubbed := block // Copy all fields

	switch block.Type {
	case agent.ContentTypeText:
		scrubbed.Text = s.ScrubText(block.Text)
	case agent.ContentTypeToolResult:
		scrubbed.ToolOutput = s.ScrubText(block.ToolOutput)
		scrubbed.ToolError = s.ScrubText(block.ToolError)
	}

	return scrubbed
}

// DetectPII returns a list of detected PII patterns without redacting.
func (s *PIIScrubber) DetectPII(text string) []PIIMatch {
	var matches []PIIMatch

	for piiType, pattern := range s.patterns {
		locs := pattern.FindAllStringIndex(text, -1)
		for _, loc := range locs {
			match := PIIMatch{
				Type:    piiType,
				Value:   text[loc[0]:loc[1]],
				Start:   loc[0],
				End:     loc[1],
				Pattern: pattern.String(),
			}
			matches = append(matches, match)
		}
	}

	return matches
}

// ContainsPII returns true if the text contains any PII.
func (s *PIIScrubber) ContainsPII(text string) bool {
	for _, pattern := range s.patterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

// ScrubMapInterface redacts PII from a map with interface{} values.
// Only string values are scrubbed; other types are passed through unchanged.
func (s *PIIScrubber) ScrubMapInterface(data map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(data))
	for key, value := range data {
		switch v := value.(type) {
		case string:
			result[key] = s.ScrubText(v)
		case map[string]interface{}:
			result[key] = s.ScrubMapInterface(v)
		case map[string]string:
			result[key] = s.ScrubMap(v)
		case []string:
			scrubbed := make([]string, len(v))
			for i, str := range v {
				scrubbed[i] = s.ScrubText(str)
			}
			result[key] = scrubbed
		case []interface{}:
			scrubbed := make([]interface{}, len(v))
			for i, item := range v {
				if str, ok := item.(string); ok {
					scrubbed[i] = s.ScrubText(str)
				} else if m, ok := item.(map[string]interface{}); ok {
					scrubbed[i] = s.ScrubMapInterface(m)
				} else {
					scrubbed[i] = item
				}
			}
			result[key] = scrubbed
		default:
			result[key] = value
		}
	}
	return result
}

// MaskValue partially masks a PII value, showing only partial content.
// Useful for logging where you need some visibility but not full exposure.
func (s *PIIScrubber) MaskValue(value string, piiType PIIType) string {
	if len(value) < 4 {
		return strings.Repeat("*", len(value))
	}

	switch piiType {
	case PIITypeEmail:
		// Show first char and domain: j***@example.com
		atIdx := strings.Index(value, "@")
		if atIdx > 0 {
			return value[:1] + strings.Repeat("*", atIdx-1) + value[atIdx:]
		}
		return strings.Repeat("*", len(value))

	case PIITypeCreditCard:
		// Show last 4 digits: ****-****-****-1234
		clean := regexp.MustCompile(`[\s\-]`).ReplaceAllString(value, "")
		if len(clean) >= 4 {
			return strings.Repeat("*", len(clean)-4) + clean[len(clean)-4:]
		}
		return strings.Repeat("*", len(value))

	case PIITypeSSN:
		// Show last 4 digits: ***-**-1234
		if len(value) >= 4 {
			return "***-**-" + value[len(value)-4:]
		}
		return strings.Repeat("*", len(value))

	case PIITypePhone:
		// Show last 4 digits
		digits := regexp.MustCompile(`\d`).FindAllString(value, -1)
		if len(digits) >= 4 {
			return strings.Repeat("*", len(digits)-4) + strings.Join(digits[len(digits)-4:], "")
		}
		return strings.Repeat("*", len(value))

	default:
		// Default: show first 2 and last 2 chars
		if len(value) > 4 {
			return value[:2] + strings.Repeat("*", len(value)-4) + value[len(value)-2:]
		}
		return strings.Repeat("*", len(value))
	}
}
