package security

import (
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

func TestNewPIIScrubber(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		expected string
	}{
		{"low level", "low", "low"},
		{"medium level", "medium", "medium"},
		{"high level", "high", "high"},
		{"invalid defaults to medium", "invalid", "medium"},
		{"empty defaults to medium", "", "medium"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewPIIScrubber(tt.level)
			if s.Level != tt.expected {
				t.Errorf("expected level %q, got %q", tt.expected, s.Level)
			}
		})
	}
}

func TestScrubText_Email(t *testing.T) {
	s := NewPIIScrubber("low")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple email",
			input:    "Contact me at john.doe@example.com for details",
			expected: "Contact me at [REDACTED:EMAIL] for details",
		},
		{
			name:     "multiple emails",
			input:    "From: alice@test.org To: bob@company.co.uk",
			expected: "From: [REDACTED:EMAIL] To: [REDACTED:EMAIL]",
		},
		{
			name:     "email with plus",
			input:    "Send to user+tag@gmail.com",
			expected: "Send to [REDACTED:EMAIL]",
		},
		{
			name:     "no email",
			input:    "No email here",
			expected: "No email here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.ScrubText(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestScrubText_SSN(t *testing.T) {
	s := NewPIIScrubber("low")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard SSN",
			input:    "SSN: 123-45-6789",
			expected: "SSN: [REDACTED:SSN]",
		},
		{
			name:     "SSN in text",
			input:    "My social is 987-65-4321 please update",
			expected: "My social is [REDACTED:SSN] please update",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.ScrubText(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestScrubText_CreditCard(t *testing.T) {
	s := NewPIIScrubber("low")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "visa format",
			input:    "Card: 4111111111111111",
			expected: "Card: [REDACTED:CREDIT_CARD]",
		},
		{
			name:     "with dashes",
			input:    "Card: 4111-1111-1111-1111",
			expected: "Card: [REDACTED:CREDIT_CARD]",
		},
		{
			name:     "with spaces",
			input:    "Card: 4111 1111 1111 1111",
			expected: "Card: [REDACTED:CREDIT_CARD]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.ScrubText(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestScrubText_Phone(t *testing.T) {
	s := NewPIIScrubber("medium")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "US format",
			input:    "Call me at 555-123-4567",
			expected: "Call me at [REDACTED:PHONE]",
		},
		{
			name:     "with parentheses",
			input:    "Phone: (555) 123-4567",
			expected: "Phone: [REDACTED:PHONE]",
		},
		{
			name:     "international",
			input:    "Intl: +1-555-123-4567",
			expected: "Intl: [REDACTED:PHONE]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.ScrubText(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestScrubText_APIKey(t *testing.T) {
	s := NewPIIScrubber("medium")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "OpenAI key",
			input:    "API key: sk-abcdefghijklmnopqrstuvwxyz123456",
			expected: "API key: [REDACTED:API_KEY]",
		},
		{
			name:     "generic api_key",
			input:    "Use api_key_abcdefghijklmnop",
			expected: "Use [REDACTED:API_KEY]",
		},
		{
			name:     "GitHub token",
			input:    "Token: ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
			expected: "Token: [REDACTED:API_KEY]",
		},
		{
			name:     "secret underscore",
			input:    "secret_abcdefghijklmnop",
			expected: "[REDACTED:API_KEY]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.ScrubText(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestScrubText_IPAddress(t *testing.T) {
	s := NewPIIScrubber("medium")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard IP",
			input:    "Server at 192.168.1.100",
			expected: "Server at [REDACTED:IP_ADDRESS]",
		},
		{
			name:     "multiple IPs",
			input:    "From 10.0.0.1 to 172.16.0.1",
			expected: "From [REDACTED:IP_ADDRESS] to [REDACTED:IP_ADDRESS]",
		},
		{
			name:     "localhost",
			input:    "Localhost: 127.0.0.1",
			expected: "Localhost: [REDACTED:IP_ADDRESS]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.ScrubText(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestScrubText_Name(t *testing.T) {
	s := NewPIIScrubber("high")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "two word name standalone",
			input:    "the name is John Smith here",
			expected: "the name is [REDACTED:NAME] here",
		},
		{
			name:     "three word name",
			input:    "Mary Jane Watson is here",
			expected: "[REDACTED:NAME] is here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.ScrubText(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestScrubText_LowLevelDoesNotDetectMedium(t *testing.T) {
	s := NewPIIScrubber("low")

	input := "Phone: 555-123-4567, IP: 192.168.1.1"
	result := s.ScrubText(input)

	// Low level should NOT scrub phone or IP
	if result != input {
		t.Errorf("low level should not scrub phone/IP, got %q", result)
	}
}

func TestScrubText_MediumLevelDoesNotDetectNames(t *testing.T) {
	s := NewPIIScrubber("medium")

	input := "Contact John Smith for help"
	result := s.ScrubText(input)

	// Medium level should NOT scrub names
	if result != input {
		t.Errorf("medium level should not scrub names, got %q", result)
	}
}

func TestScrubMap(t *testing.T) {
	s := NewPIIScrubber("medium")

	input := map[string]string{
		"email":   "user@example.com",
		"message": "Call 555-123-4567",
		"safe":    "no pii here",
	}

	result := s.ScrubMap(input)

	if result["email"] != "[REDACTED:EMAIL]" {
		t.Errorf("expected email scrubbed, got %q", result["email"])
	}
	if result["message"] != "Call [REDACTED:PHONE]" {
		t.Errorf("expected phone scrubbed, got %q", result["message"])
	}
	if result["safe"] != "no pii here" {
		t.Errorf("expected safe unchanged, got %q", result["safe"])
	}
}

func TestScrubMessages(t *testing.T) {
	s := NewPIIScrubber("medium")

	messages := []agent.Message{
		{
			Role: agent.RoleUser,
			Content: []agent.ContentBlock{
				{Type: agent.ContentTypeText, Text: "My email is test@example.com"},
			},
		},
		{
			Role: agent.RoleAssistant,
			Content: []agent.ContentBlock{
				{Type: agent.ContentTypeText, Text: "I see your email is test@example.com"},
			},
		},
		{
			Role: agent.RoleTool,
			Content: []agent.ContentBlock{
				{Type: agent.ContentTypeToolResult, ToolOutput: "Found: 192.168.1.1"},
			},
		},
	}

	result := s.ScrubMessages(messages)

	// Check user message
	if result[0].Content[0].Text != "My email is [REDACTED:EMAIL]" {
		t.Errorf("expected user email scrubbed, got %q", result[0].Content[0].Text)
	}

	// Check assistant message
	if result[1].Content[0].Text != "I see your email is [REDACTED:EMAIL]" {
		t.Errorf("expected assistant email scrubbed, got %q", result[1].Content[0].Text)
	}

	// Check tool result
	if result[2].Content[0].ToolOutput != "Found: [REDACTED:IP_ADDRESS]" {
		t.Errorf("expected tool IP scrubbed, got %q", result[2].Content[0].ToolOutput)
	}
}

func TestScrubMessages_LegacyText(t *testing.T) {
	s := NewPIIScrubber("medium")

	messages := []agent.Message{
		{
			Role: agent.RoleUser,
			Text: "My email is test@example.com",
		},
	}

	result := s.ScrubMessages(messages)

	if result[0].Text != "My email is [REDACTED:EMAIL]" {
		t.Errorf("expected legacy text scrubbed, got %q", result[0].Text)
	}
}

func TestDetectPII(t *testing.T) {
	s := NewPIIScrubber("medium")

	text := "Email: john@test.com, Phone: 555-123-4567, IP: 10.0.0.1"
	matches := s.DetectPII(text)

	if len(matches) != 3 {
		t.Errorf("expected 3 matches, got %d", len(matches))
	}

	// Check that we have one of each type
	types := make(map[PIIType]bool)
	for _, m := range matches {
		types[m.Type] = true
	}

	if !types[PIITypeEmail] {
		t.Error("expected EMAIL type")
	}
	if !types[PIITypePhone] {
		t.Error("expected PHONE type")
	}
	if !types[PIITypeIPAddress] {
		t.Error("expected IP_ADDRESS type")
	}
}

func TestContainsPII(t *testing.T) {
	s := NewPIIScrubber("medium")

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"has email", "contact user@example.com", true},
		{"has phone", "call 555-123-4567", true},
		{"no pii", "just regular text", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.ContainsPII(tt.input)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestScrubMapInterface(t *testing.T) {
	s := NewPIIScrubber("medium")

	input := map[string]interface{}{
		"email": "user@example.com",
		"count": 42,
		"nested": map[string]interface{}{
			"phone": "555-123-4567",
		},
		"list": []string{"test@example.com", "safe"},
	}

	result := s.ScrubMapInterface(input)

	if result["email"] != "[REDACTED:EMAIL]" {
		t.Errorf("expected email scrubbed, got %v", result["email"])
	}
	if result["count"] != 42 {
		t.Errorf("expected count unchanged, got %v", result["count"])
	}

	nested := result["nested"].(map[string]interface{})
	if nested["phone"] != "Call [REDACTED:PHONE]" && nested["phone"] != "[REDACTED:PHONE]" {
		t.Errorf("expected nested phone scrubbed, got %v", nested["phone"])
	}

	list := result["list"].([]string)
	if list[0] != "[REDACTED:EMAIL]" {
		t.Errorf("expected list email scrubbed, got %v", list[0])
	}
	if list[1] != "safe" {
		t.Errorf("expected list safe unchanged, got %v", list[1])
	}
}

func TestMaskValue(t *testing.T) {
	s := NewPIIScrubber("medium")

	tests := []struct {
		name     string
		value    string
		piiType  PIIType
		expected string
	}{
		{
			name:     "mask email",
			value:    "john@example.com",
			piiType:  PIITypeEmail,
			expected: "j***@example.com",
		},
		{
			name:     "mask credit card",
			value:    "4111111111111111",
			piiType:  PIITypeCreditCard,
			expected: "************1111",
		},
		{
			name:     "mask SSN",
			value:    "123-45-6789",
			piiType:  PIITypeSSN,
			expected: "***-**-6789",
		},
		{
			name:     "mask phone",
			value:    "555-123-4567",
			piiType:  PIITypePhone,
			expected: "******4567",
		},
		{
			name:     "mask short value",
			value:    "abc",
			piiType:  PIITypeAPIKey,
			expected: "***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.MaskValue(tt.value, tt.piiType)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestScrubText_MultiplePIITypes(t *testing.T) {
	s := NewPIIScrubber("high")

	input := "Contact John Smith at john@example.com or 555-123-4567. His SSN is 123-45-6789 and card 4111-1111-1111-1111. Server: 192.168.1.1"
	result := s.ScrubText(input)

	// Should scrub all PII types
	if !contains(result, "[REDACTED:EMAIL]") {
		t.Error("expected email redacted")
	}
	if !contains(result, "[REDACTED:PHONE]") {
		t.Error("expected phone redacted")
	}
	if !contains(result, "[REDACTED:SSN]") {
		t.Error("expected SSN redacted")
	}
	if !contains(result, "[REDACTED:CREDIT_CARD]") {
		t.Error("expected credit card redacted")
	}
	if !contains(result, "[REDACTED:IP_ADDRESS]") {
		t.Error("expected IP address redacted")
	}
	if !contains(result, "[REDACTED:NAME]") {
		t.Error("expected name redacted")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
