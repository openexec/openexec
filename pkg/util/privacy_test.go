package util

import (
	"testing"
)

func TestScrubPII(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Scrub Email",
			input:    "Contact me at john.doe@example.com for info.",
			expected: "Contact me at [EMAIL_REDACTED] for info.",
		},
		{
			name:     "Scrub Finnish HETU",
			input:    "My ID is 010101-123A and phone is 123.",
			expected: "My ID is [HETU_REDACTED] and phone is 123.",
		},
		{
			name:     "Scrub API Key",
			input:    "Set api_key: 'sk-1234567890abcdef' in config.",
			expected: "Set api_key: '[SECRET_REDACTED]' in config.",
		},
		{
			name:     "Scrub Password",
			input:    "password=SuperSecret123!",
			expected: "password=[SECRET_REDACTED]!",
		},
		{
			name:     "Mixed Content",
			input:    "Email test@test.fi with key=abc12345678",
			expected: "Email [EMAIL_REDACTED] with key=[SECRET_REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ScrubPII(tt.input)
			if got != tt.expected {
				t.Errorf("ScrubPII() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestMaskInfrastructure(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Mask Single IP",
			input:    "Server is at 192.168.1.1",
			expected: "Server is at [IP_REDACTED]",
		},
		{
			name:     "Mask Multiple IPs",
			input:    "Nodes: 10.0.0.1, 10.0.0.2",
			expected: "Nodes: [IP_REDACTED], [IP_REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaskInfrastructure(tt.input)
			if got != tt.expected {
				t.Errorf("MaskInfrastructure() = %q, want %q", got, tt.expected)
			}
		})
	}
}
