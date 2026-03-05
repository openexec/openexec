package util

import (
	"strings"
	"unicode"
)

// SanitizeInput cleans up user input from the UI.
// It removes non-printable characters and normalizes whitespace.
func SanitizeInput(input string) string {
	// Remove non-printable characters (except common ones like newline/tab)
	var b strings.Builder
	for _, r := range input {
		if unicode.IsPrint(r) || r == '\n' || r == '\r' || r == '\t' {
			b.WriteRune(r)
		}
	}
	
	// Trim leading/trailing whitespace
	return strings.TrimSpace(b.String())
}

// SanitizeOutput prepares text for UI display.
func SanitizeOutput(input string) string {
	// Basic trimming
	return strings.TrimSpace(input)
}
