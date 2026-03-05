package util

import (
	"testing"
)

func TestSanitizeInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic trim",
			input:    "  hello  ",
			expected: "hello",
		},
		{
			name:     "non-printable characters",
			input:    "hello\x00world",
			expected: "helloworld",
		},
		{
			name:     "newlines and tabs",
			input:    "line1\nline2\ttab",
			expected: "line1\nline2\ttab",
		},
		{
			name:     "empty input",
			input:    "   \x01\x02   ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeInput(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeInput() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSanitizeOutput(t *testing.T) {
	input := "  text  "
	expected := "text"
	if got := SanitizeOutput(input); got != expected {
		t.Errorf("SanitizeOutput() = %q, want %q", got, expected)
	}
}
