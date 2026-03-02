// Package mcp provides MCP (Model Context Protocol) server functionality.
// This file implements secret redaction utilities for shell command output.
package mcp

import (
	"regexp"
	"strings"
)

// RedactedPlaceholder is the string used to replace detected secrets.
const RedactedPlaceholder = "[REDACTED]"

// SecretPattern represents a named pattern for detecting secrets.
type SecretPattern struct {
	Name    string
	Pattern *regexp.Regexp
}

// defaultSecretPatterns contains common patterns for detecting secrets in output.
// These patterns are designed to catch common secret formats while minimizing false positives.
var defaultSecretPatterns = []SecretPattern{
	// AWS Access Key ID (starts with AKIA, ASIA, AROA, or AIDA)
	{Name: "aws_access_key", Pattern: regexp.MustCompile(`(?i)\b(A[SK]IA|AIDA|AROA)[A-Z0-9]{16}\b`)},

	// AWS Secret Access Key (40 character base64-like string)
	{Name: "aws_secret_key", Pattern: regexp.MustCompile(`(?i)(?:aws[_-]?secret[_-]?(?:access[_-]?)?key|secret[_-]?access[_-]?key)['":\s=]+([A-Za-z0-9/+=]{40})`)},

	// Generic API Key patterns (common prefixes)
	{Name: "api_key_prefixed", Pattern: regexp.MustCompile(`(?i)\b(sk-[a-zA-Z0-9]{32,})\b`)},                                     // OpenAI-style
	{Name: "api_key_prefixed2", Pattern: regexp.MustCompile(`(?i)\b(pk_live_[a-zA-Z0-9]{24,})\b`)},                               // Stripe public key
	{Name: "api_key_prefixed3", Pattern: regexp.MustCompile(`(?i)\b(sk_live_[a-zA-Z0-9]{24,})\b`)},                               // Stripe secret key
	{Name: "api_key_prefixed4", Pattern: regexp.MustCompile(`(?i)\b(rk_live_[a-zA-Z0-9]{24,})\b`)},                               // Stripe restricted key
	{Name: "api_key_prefixed5", Pattern: regexp.MustCompile(`(?i)\b(ghp_[a-zA-Z0-9]{36})\b`)},                                    // GitHub personal access token
	{Name: "api_key_prefixed6", Pattern: regexp.MustCompile(`(?i)\b(gho_[a-zA-Z0-9]{36})\b`)},                                    // GitHub OAuth access token
	{Name: "api_key_prefixed7", Pattern: regexp.MustCompile(`(?i)\b(github_pat_[a-zA-Z0-9]{22}_[a-zA-Z0-9]{59})\b`)},             // GitHub fine-grained PAT
	{Name: "api_key_prefixed8", Pattern: regexp.MustCompile(`(?i)\b(xox[baprs]-[a-zA-Z0-9-]{10,})\b`)},                           // Slack tokens
	{Name: "api_key_prefixed9", Pattern: regexp.MustCompile(`(?i)\b(SG\.[a-zA-Z0-9_-]{22}\.[a-zA-Z0-9_-]{43})\b`)},               // SendGrid
	{Name: "api_key_prefixed10", Pattern: regexp.MustCompile(`(?i)\b(sq0[a-z]{3}-[a-zA-Z0-9_-]{22,})\b`)},                        // Square
	{Name: "api_key_prefixed11", Pattern: regexp.MustCompile(`(?i)\b(EZAK[a-zA-Z0-9]{54})\b`)},                                   // EasyPost
	{Name: "api_key_prefixed12", Pattern: regexp.MustCompile(`(?i)\b(AC[a-f0-9]{32})\b`)},                                        // Twilio Account SID
	{Name: "api_key_prefixed13", Pattern: regexp.MustCompile(`(?i)\b(key-[a-zA-Z0-9]{32})\b`)},                                   // Mailgun
	{Name: "api_key_prefixed14", Pattern: regexp.MustCompile(`(?i)\b(glpat-[a-zA-Z0-9_-]{20,})\b`)},                              // GitLab personal access token
	{Name: "api_key_prefixed15", Pattern: regexp.MustCompile(`(?i)\b(npm_[a-zA-Z0-9]{36})\b`)},                                   // NPM token
	{Name: "api_key_prefixed16", Pattern: regexp.MustCompile(`(?i)\b(pypi-AgEIcHlwaS5vcmc[a-zA-Z0-9_-]{50,})\b`)},                // PyPI token
	{Name: "api_key_prefixed17", Pattern: regexp.MustCompile(`(?i)\b(CLOJARS_[a-f0-9]{60})\b`)},                                  // Clojars
	{Name: "api_key_prefixed18", Pattern: regexp.MustCompile(`(?i)\b(dop_v1_[a-f0-9]{64})\b`)},                                   // DigitalOcean personal access token
	{Name: "api_key_prefixed19", Pattern: regexp.MustCompile(`(?i)\b(doo_v1_[a-f0-9]{64})\b`)},                                   // DigitalOcean OAuth token
	{Name: "api_key_prefixed20", Pattern: regexp.MustCompile(`(?i)\b(anthropic-[a-zA-Z0-9]{32,})\b`)},                            // Anthropic API key pattern
	{Name: "api_key_prefixed21", Pattern: regexp.MustCompile(`(?i)\b(sk-ant-[a-zA-Z0-9]{32,})\b`)},                               // Anthropic API key
	{Name: "api_key_prefixed22", Pattern: regexp.MustCompile(`(?i)\b(AIza[a-zA-Z0-9_-]{35})\b`)},                                 // Google API Key
	{Name: "api_key_prefixed23", Pattern: regexp.MustCompile(`(?i)\b(ya29\.[a-zA-Z0-9_-]+)\b`)},                                  // Google OAuth token
	{Name: "api_key_prefixed24", Pattern: regexp.MustCompile(`(?i)\b(AGC[a-zA-Z0-9_-]{40,})\b`)},                                 // Firebase Cloud Messaging
	{Name: "api_key_prefixed25", Pattern: regexp.MustCompile(`(?i)\b(r8_[a-zA-Z0-9]{30,})\b`)},                                   // Replicate API token
	{Name: "api_key_prefixed26", Pattern: regexp.MustCompile(`(?i)\b(hf_[a-zA-Z0-9]{34})\b`)},                                    // Hugging Face token
	{Name: "api_key_prefixed27", Pattern: regexp.MustCompile(`(?i)\b(whsec_[a-zA-Z0-9]{32,})\b`)},                                // Webhook secret
	{Name: "api_key_prefixed28", Pattern: regexp.MustCompile(`(?i)\b(shpss_[a-fA-F0-9]{32}|shpat_[a-fA-F0-9]{32})\b`)},           // Shopify
	{Name: "api_key_prefixed29", Pattern: regexp.MustCompile(`(?i)\b(EAA[A-Za-z0-9]+)\b`)},                                       // Facebook access token
	{Name: "api_key_prefixed30", Pattern: regexp.MustCompile(`(?i)\b(heroku[_-]?api[_-]?key)['":\s=]+([a-f0-9-]{36,})\b`)},       // Heroku API Key
	{Name: "api_key_prefixed31", Pattern: regexp.MustCompile(`(?i)\b(sentry[_-]?dsn)['":\s=]+https://[^@]+@[^\s'"]+`)},           // Sentry DSN
	{Name: "api_key_prefixed32", Pattern: regexp.MustCompile(`(?i)\b(database[_-]?url)['":\s=]+[a-z]+://[^\s'"]+@[^\s'"]+\b`)},   // Database URL with credentials
	{Name: "api_key_prefixed33", Pattern: regexp.MustCompile(`(?i)\b(redis[_-]?url)['":\s=]+redis://[^\s'"]+@[^\s'"]+\b`)},       // Redis URL with credentials
	{Name: "api_key_prefixed34", Pattern: regexp.MustCompile(`(?i)\b(mongodb[_-]?uri)['":\s=]+mongodb(\+srv)?://[^\s'"]+\b`)},    // MongoDB URI

	// Generic secret assignment patterns (key=value, key: value, key="value")
	{Name: "generic_secret", Pattern: regexp.MustCompile(`(?i)(?:password|passwd|pwd|secret|token|api[_-]?key|apikey|auth[_-]?token|access[_-]?token|bearer|credentials?)['":\s=]+['"]?([^\s'"]{8,})['"]?`)},

	// Private keys
	{Name: "private_key_header", Pattern: regexp.MustCompile(`-----BEGIN (?:RSA |DSA |EC |OPENSSH |PGP )?PRIVATE KEY(?:\s+BLOCK)?-----`)},

	// JWT tokens
	{Name: "jwt", Pattern: regexp.MustCompile(`\beyJ[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9_-]{10,}\b`)},

	// Bearer tokens in headers
	{Name: "bearer_token", Pattern: regexp.MustCompile(`(?i)(?:authorization|bearer)['":\s=]+bearer\s+([a-zA-Z0-9_.-]{20,})`)},

	// Basic auth headers (base64 encoded credentials)
	{Name: "basic_auth", Pattern: regexp.MustCompile(`(?i)(?:authorization)['":\s=]+basic\s+([a-zA-Z0-9+/=]{20,})`)},

	// URLs with embedded credentials
	{Name: "url_credentials", Pattern: regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://[^:]+:[^@]+@[^\s'"]+)`)},
}

// Redactor handles secret redaction in text output.
type Redactor struct {
	patterns        []SecretPattern
	envVars         map[string]string
	customPatterns  []*regexp.Regexp
	minSecretLength int
}

// RedactorOption is a functional option for configuring the Redactor.
type RedactorOption func(*Redactor)

// WithEnvVars adds environment variable values to be redacted.
// Any occurrence of these values in the output will be replaced with [REDACTED].
func WithEnvVars(envVars map[string]string) RedactorOption {
	return func(r *Redactor) {
		r.envVars = envVars
	}
}

// WithCustomPatterns adds custom regex patterns for redaction.
func WithCustomPatterns(patterns ...*regexp.Regexp) RedactorOption {
	return func(r *Redactor) {
		r.customPatterns = append(r.customPatterns, patterns...)
	}
}

// WithMinSecretLength sets the minimum length for environment variable values
// to be considered for redaction. Default is 8 characters.
// This prevents redacting very short values that might cause false positives.
func WithMinSecretLength(length int) RedactorOption {
	return func(r *Redactor) {
		r.minSecretLength = length
	}
}

// WithPatterns allows replacing the default patterns with custom ones.
func WithPatterns(patterns []SecretPattern) RedactorOption {
	return func(r *Redactor) {
		r.patterns = patterns
	}
}

// NewRedactor creates a new Redactor with the given options.
func NewRedactor(opts ...RedactorOption) *Redactor {
	r := &Redactor{
		patterns:        defaultSecretPatterns,
		envVars:         make(map[string]string),
		customPatterns:  nil,
		minSecretLength: 8,
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Redact applies all redaction patterns to the input text and returns the redacted output.
// It redacts in the following order:
// 1. Environment variable values (if any are configured)
// 2. Default secret patterns
// 3. Custom patterns (if any are configured)
func (r *Redactor) Redact(input string) string {
	if input == "" {
		return input
	}

	result := input

	// First, redact environment variable values
	// We process longer values first to avoid partial matches
	sortedEnvValues := r.getSortedEnvValues()
	for _, value := range sortedEnvValues {
		if len(value) >= r.minSecretLength && value != "" {
			result = strings.ReplaceAll(result, value, RedactedPlaceholder)
		}
	}

	// Apply default patterns
	for _, sp := range r.patterns {
		result = sp.Pattern.ReplaceAllStringFunc(result, func(match string) string {
			// For patterns that capture groups, we want to redact just the secret part
			// For simpler patterns, we redact the entire match
			return RedactedPlaceholder
		})
	}

	// Apply custom patterns
	for _, pattern := range r.customPatterns {
		result = pattern.ReplaceAllString(result, RedactedPlaceholder)
	}

	return result
}

// getSortedEnvValues returns environment variable values sorted by length (descending).
// This ensures longer values are replaced first to avoid partial matches.
func (r *Redactor) getSortedEnvValues() []string {
	values := make([]string, 0, len(r.envVars))
	for _, v := range r.envVars {
		if v != "" {
			values = append(values, v)
		}
	}

	// Sort by length descending (simple bubble sort for small slices)
	for i := 0; i < len(values)-1; i++ {
		for j := 0; j < len(values)-i-1; j++ {
			if len(values[j]) < len(values[j+1]) {
				values[j], values[j+1] = values[j+1], values[j]
			}
		}
	}

	return values
}

// RedactOutput is a convenience function that creates a default Redactor
// and applies it to the input text.
func RedactOutput(input string) string {
	return NewRedactor().Redact(input)
}

// RedactWithEnv creates a Redactor with the given environment variables
// and applies it to the input text.
func RedactWithEnv(input string, envVars map[string]string) string {
	return NewRedactor(WithEnvVars(envVars)).Redact(input)
}

// SensitiveEnvVarNames contains common environment variable names that typically hold secrets.
// These can be used to filter which env vars to include for redaction.
var SensitiveEnvVarNames = []string{
	"API_KEY",
	"APIKEY",
	"API_SECRET",
	"API_TOKEN",
	"ACCESS_KEY",
	"ACCESS_TOKEN",
	"AUTH_TOKEN",
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",
	"BEARER_TOKEN",
	"CLIENT_SECRET",
	"DATABASE_PASSWORD",
	"DATABASE_URL",
	"DB_PASSWORD",
	"DB_URL",
	"GITHUB_TOKEN",
	"GITLAB_TOKEN",
	"JWT_SECRET",
	"OPENAI_API_KEY",
	"ANTHROPIC_API_KEY",
	"GOOGLE_API_KEY",
	"PASSWORD",
	"PASSWD",
	"PRIVATE_KEY",
	"REDIS_PASSWORD",
	"REDIS_URL",
	"SECRET",
	"SECRET_KEY",
	"SENDGRID_API_KEY",
	"SLACK_TOKEN",
	"STRIPE_SECRET_KEY",
	"TELEGRAM_BOT_TOKEN",
	"TELEGRAM_SECRET_TOKEN",
	"TELEGRAM_WEBHOOK_SECRET",
	"TOKEN",
	"TWILIO_AUTH_TOKEN",
}

// FilterSensitiveEnvVars filters a map of environment variables to include only
// those with names that match known sensitive patterns.
func FilterSensitiveEnvVars(envVars map[string]string) map[string]string {
	sensitive := make(map[string]string)

	for key, value := range envVars {
		upperKey := strings.ToUpper(key)
		for _, sensitiveKey := range SensitiveEnvVarNames {
			if strings.Contains(upperKey, sensitiveKey) || upperKey == sensitiveKey {
				sensitive[key] = value
				break
			}
		}

		// Also check for common patterns
		if containsSensitivePattern(upperKey) {
			sensitive[key] = value
		}
	}

	return sensitive
}

// containsSensitivePattern checks if a key name matches common secret patterns.
func containsSensitivePattern(key string) bool {
	sensitivePatterns := []string{
		"_KEY", "_SECRET", "_TOKEN", "_PASSWORD", "_PASSWD", "_PWD",
		"_CREDENTIAL", "_AUTH", "_API", "KEY_", "SECRET_", "TOKEN_",
		"PASSWORD_", "PASSWD_", "PWD_", "CREDENTIAL_", "AUTH_",
	}

	for _, pattern := range sensitivePatterns {
		if strings.Contains(key, pattern) {
			return true
		}
	}

	return false
}
