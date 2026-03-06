package util

import (
	"regexp"
	"strings"
)

var (
	// Common PII patterns
	emailRegex = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	// Simplified Finnish HETU pattern (DDMMYY-XXXX)
	hetuRegex = regexp.MustCompile(`\b\d{6}[-+A]\d{3}[0-9A-Y]\b`)
	// IPv4 pattern
	ipv4Regex = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	
	// Sensitive Key patterns
	apiKeyRegex = regexp.MustCompile(`(?i)(?:key|password|secret|token|auth|credential|pwd)["']?\s*[:=]\s*["']?([a-zA-Z0-9\-_]{8,})["']?`)
)

// ScrubPII replaces sensitive information with placeholders.
func ScrubPII(input string) string {
	if input == "" {
		return input
	}

	result := input
	
	// 1. Scrub Emails
	result = emailRegex.ReplaceAllString(result, "[EMAIL_REDACTED]")
	
	// 2. Scrub Finnish HETU
	result = hetuRegex.ReplaceAllString(result, "[HETU_REDACTED]")
	
	// 3. Scrub API Keys / Passwords in "key=value" format
	// We want to keep the key but redact the value
	result = scrubKeyValues(result)

	return result
}

// MaskInfrastructure redacts IP addresses from text.
func MaskInfrastructure(input string) string {
	if input == "" {
		return input
	}
	return ipv4Regex.ReplaceAllString(input, "[IP_REDACTED]")
}

func scrubKeyValues(input string) string {
	matches := apiKeyRegex.FindAllStringSubmatch(input, -1)
	result := input
	for _, match := range matches {
		if len(match) > 1 {
			val := match[1]
			// Replace only the value part
			result = strings.ReplaceAll(result, val, "[SECRET_REDACTED]")
		}
	}
	return result
}
