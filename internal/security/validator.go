// Package security provides OWASP LLM Top 10 security controls for OpenExec.
// It implements validation, sanitization, and monitoring for LLM-specific risks.
package security

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// LLMRisk represents an OWASP LLM Top 10 risk category.
type LLMRisk string

const (
	// LLM01: Prompt Injection
	RiskPromptInjection LLMRisk = "LLM01_PROMPT_INJECTION"
	// LLM02: Insecure Output Handling
	RiskInsecureOutput LLMRisk = "LLM02_INSECURE_OUTPUT"
	// LLM03: Training Data Poisoning (not applicable to inference)
	RiskTrainingPoison LLMRisk = "LLM03_TRAINING_POISON"
	// LLM04: Model Denial of Service
	RiskModelDoS LLMRisk = "LLM04_MODEL_DOS"
	// LLM05: Supply Chain Vulnerabilities
	RiskSupplyChain LLMRisk = "LLM05_SUPPLY_CHAIN"
	// LLM06: Sensitive Information Disclosure
	RiskInfoDisclosure LLMRisk = "LLM06_INFO_DISCLOSURE"
	// LLM07: Insecure Plugin Design
	RiskInsecurePlugin LLMRisk = "LLM07_INSECURE_PLUGIN"
	// LLM08: Excessive Agency
	RiskExcessiveAgency LLMRisk = "LLM08_EXCESSIVE_AGENCY"
	// LLM09: Overreliance
	RiskOverreliance LLMRisk = "LLM09_OVERRELIANCE"
	// LLM10: Model Theft (not applicable)
	RiskModelTheft LLMRisk = "LLM10_MODEL_THEFT"
)

// Severity levels for security events.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

// ValidationResult represents the outcome of a security validation.
type ValidationResult struct {
	Valid    bool
	Risk     LLMRisk
	Severity Severity
	Message  string
	Details  map[string]interface{}
}

// Validator provides security validation for LLM inputs and outputs.
type Validator struct {
	// Prompt injection patterns
	injectionPatterns []*regexp.Regexp

	// Sensitive data patterns
	sensitivePatterns []*regexp.Regexp

	// Rate limiting state
	rateLimitMu sync.Mutex
	rateLimits  map[string]*rateLimitEntry

	// Configuration
	config ValidatorConfig
}

// ValidatorConfig configures the security validator.
type ValidatorConfig struct {
	// MaxPromptLength limits prompt size to prevent DoS (default: 100KB)
	MaxPromptLength int
	// MaxTokensPerMinute limits token usage (default: 100000)
	MaxTokensPerMinute int
	// MaxRequestsPerMinute limits request rate (default: 60)
	MaxRequestsPerMinute int
	// EnableInjectionDetection enables prompt injection scanning
	EnableInjectionDetection bool
	// EnableSensitiveDataRedaction enables PII/secret detection
	EnableSensitiveDataRedaction bool
}

// DefaultConfig returns secure default configuration.
func DefaultConfig() ValidatorConfig {
	return ValidatorConfig{
		MaxPromptLength:              100 * 1024, // 100KB
		MaxTokensPerMinute:           100000,
		MaxRequestsPerMinute:         60,
		EnableInjectionDetection:     true,
		EnableSensitiveDataRedaction: true,
	}
}

type rateLimitEntry struct {
	tokens    int
	requests  int
	resetTime time.Time
}

// NewValidator creates a new security validator.
func NewValidator(config ValidatorConfig) *Validator {
	v := &Validator{
		config:     config,
		rateLimits: make(map[string]*rateLimitEntry),
	}
	v.initPatterns()
	return v
}

// initPatterns compiles regex patterns for security checks.
func (v *Validator) initPatterns() {
	// LLM01: Prompt Injection patterns
	injectionStrings := []string{
		// Direct instruction override attempts
		`(?i)ignore\s+(previous|all|above)\s+(instructions|prompts)`,
		`(?i)disregard\s+(everything|all)\s+(above|before)`,
		`(?i)forget\s+(everything|all)\s+(above|before)`,
		`(?i)new\s+instructions?:`,
		`(?i)system\s*:\s*you\s+are`,
		`(?i)\[INST\]`,
		`(?i)<\|im_start\|>`,
		`(?i)###\s*(system|instruction)`,
		// Role hijacking
		`(?i)pretend\s+you\s+are`,
		`(?i)act\s+as\s+(if\s+you|an?)`,
		`(?i)roleplay\s+as`,
		`(?i)you\s+are\s+now`,
		// Jailbreak patterns
		`(?i)DAN\s+mode`,
		`(?i)developer\s+mode`,
		`(?i)jailbreak`,
		`(?i)bypass\s+(filter|restriction|safety)`,
		// Delimiter exploitation
		`(?i)\{\{.*system.*\}\}`,
		`(?i)\[\[.*instruction.*\]\]`,
	}

	for _, pattern := range injectionStrings {
		if re, err := regexp.Compile(pattern); err == nil {
			v.injectionPatterns = append(v.injectionPatterns, re)
		}
	}

	// LLM06: Sensitive Information patterns
	sensitiveStrings := []string{
		// API Keys and Tokens
		`(?i)(api[_-]?key|apikey)\s*[=:]\s*['"]?[a-zA-Z0-9_-]{20,}`,
		`(?i)(secret|token)\s*[=:]\s*['"]?[a-zA-Z0-9_-]{20,}`,
		`sk-[a-zA-Z0-9]{48}`,                      // OpenAI keys
		`anthropic-[a-zA-Z0-9]{32,}`,              // Anthropic keys
		`AIza[a-zA-Z0-9_-]{35}`,                   // Google API keys
		`ghp_[a-zA-Z0-9]{36}`,                     // GitHub PAT
		`glpat-[a-zA-Z0-9_-]{20}`,                 // GitLab PAT
		// AWS
		`AKIA[A-Z0-9]{16}`,                        // AWS Access Key ID
		`(?i)aws_secret_access_key\s*[=:]\s*['"]?[a-zA-Z0-9/+=]{40}`,
		// Private Keys
		`-----BEGIN\s+(RSA\s+)?PRIVATE\s+KEY-----`,
		`-----BEGIN\s+OPENSSH\s+PRIVATE\s+KEY-----`,
		// Passwords in common formats
		`(?i)(password|passwd|pwd)\s*[=:]\s*['"]?[^\s'"]{8,}`,
		// Database connection strings
		`(?i)(mysql|postgres|mongodb)://[^\s]+:[^\s]+@`,
		// JWT tokens
		`eyJ[a-zA-Z0-9_-]+\.eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+`,
		// Credit cards (basic Luhn-checkable patterns)
		`\b[4-6][0-9]{3}[\s-]?[0-9]{4}[\s-]?[0-9]{4}[\s-]?[0-9]{4}\b`,
		// SSN
		`\b[0-9]{3}-[0-9]{2}-[0-9]{4}\b`,
		// Email addresses (for PII tracking)
		`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`,
	}

	for _, pattern := range sensitiveStrings {
		if re, err := regexp.Compile(pattern); err == nil {
			v.sensitivePatterns = append(v.sensitivePatterns, re)
		}
	}
}

// ValidateInput checks input for security issues.
func (v *Validator) ValidateInput(input string, userID string) []ValidationResult {
	var results []ValidationResult

	// LLM04: Check for DoS via large input
	if len(input) > v.config.MaxPromptLength {
		results = append(results, ValidationResult{
			Valid:    false,
			Risk:     RiskModelDoS,
			Severity: SeverityHigh,
			Message:  fmt.Sprintf("Input exceeds maximum length (%d > %d)", len(input), v.config.MaxPromptLength),
		})
	}

	// LLM01: Check for prompt injection
	if v.config.EnableInjectionDetection {
		for _, pattern := range v.injectionPatterns {
			if pattern.MatchString(input) {
				results = append(results, ValidationResult{
					Valid:    false,
					Risk:     RiskPromptInjection,
					Severity: SeverityHigh,
					Message:  "Potential prompt injection detected",
					Details: map[string]interface{}{
						"pattern": pattern.String(),
					},
				})
				break // One match is enough
			}
		}
	}

	// LLM06: Check for sensitive data in input
	if v.config.EnableSensitiveDataRedaction {
		for _, pattern := range v.sensitivePatterns {
			if pattern.MatchString(input) {
				results = append(results, ValidationResult{
					Valid:    false,
					Risk:     RiskInfoDisclosure,
					Severity: SeverityMedium,
					Message:  "Sensitive data detected in input",
					Details: map[string]interface{}{
						"type": "input_contains_secrets",
					},
				})
				break
			}
		}
	}

	// LLM04: Check rate limits
	if !v.checkRateLimit(userID, 0) {
		results = append(results, ValidationResult{
			Valid:    false,
			Risk:     RiskModelDoS,
			Severity: SeverityMedium,
			Message:  "Rate limit exceeded",
		})
	}

	return results
}

// ValidateOutput checks model output for security issues.
func (v *Validator) ValidateOutput(output string) []ValidationResult {
	var results []ValidationResult

	// LLM02: Check for sensitive data in output (potential disclosure)
	if v.config.EnableSensitiveDataRedaction {
		for _, pattern := range v.sensitivePatterns {
			if pattern.MatchString(output) {
				results = append(results, ValidationResult{
					Valid:    false,
					Risk:     RiskInsecureOutput,
					Severity: SeverityHigh,
					Message:  "Sensitive data detected in output",
					Details: map[string]interface{}{
						"type": "output_contains_secrets",
					},
				})
				break
			}
		}
	}

	// LLM02: Check for potential code injection in output
	dangerousPatterns := []string{
		`<script[^>]*>`,           // XSS
		`javascript:`,             // XSS
		`on\w+\s*=`,               // Event handlers
		`eval\s*\(`,               // JS eval
		`exec\s*\(`,               // Python exec
		`system\s*\(`,             // Shell execution
		`os\.popen`,               // Python shell
		`subprocess\.`,            // Python subprocess
		`Runtime\.getRuntime\(\)`, // Java runtime
	}

	for _, pattern := range dangerousPatterns {
		if re, err := regexp.Compile(`(?i)` + pattern); err == nil && re.MatchString(output) {
			results = append(results, ValidationResult{
				Valid:    false,
				Risk:     RiskInsecureOutput,
				Severity: SeverityMedium,
				Message:  "Potentially dangerous code pattern in output",
				Details: map[string]interface{}{
					"pattern": pattern,
				},
			})
			break
		}
	}

	return results
}

// ValidateToolCall checks if a tool call is safe to execute.
func (v *Validator) ValidateToolCall(toolName string, args map[string]interface{}) []ValidationResult {
	var results []ValidationResult

	// LLM07: Check for dangerous tool patterns
	dangerousTools := map[string]string{
		"bash":    "Shell command execution",
		"shell":   "Shell command execution",
		"exec":    "Command execution",
		"eval":    "Code evaluation",
		"system":  "System command",
		"popen":   "Process spawning",
	}

	toolLower := strings.ToLower(toolName)
	if reason, dangerous := dangerousTools[toolLower]; dangerous {
		results = append(results, ValidationResult{
			Valid:    true, // Warning only, not blocking
			Risk:     RiskInsecurePlugin,
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("Potentially dangerous tool: %s", reason),
		})
	}

	// Check for path traversal in file arguments
	for key, val := range args {
		if str, ok := val.(string); ok {
			if strings.Contains(str, "..") || strings.HasPrefix(str, "/etc") || strings.HasPrefix(str, "/root") {
				results = append(results, ValidationResult{
					Valid:    false,
					Risk:     RiskInsecurePlugin,
					Severity: SeverityHigh,
					Message:  fmt.Sprintf("Potential path traversal in argument %q", key),
					Details: map[string]interface{}{
						"argument": key,
						"value":    str,
					},
				})
			}
		}
	}

	return results
}

// RedactSensitive removes or masks sensitive data from text.
func (v *Validator) RedactSensitive(text string) string {
	result := text
	for _, pattern := range v.sensitivePatterns {
		result = pattern.ReplaceAllString(result, "[REDACTED]")
	}
	return result
}

// checkRateLimit verifies rate limits haven't been exceeded.
func (v *Validator) checkRateLimit(userID string, tokens int) bool {
	v.rateLimitMu.Lock()
	defer v.rateLimitMu.Unlock()

	now := time.Now()
	entry, exists := v.rateLimits[userID]

	// Create or reset entry if expired
	if !exists || now.After(entry.resetTime) {
		v.rateLimits[userID] = &rateLimitEntry{
			tokens:    tokens,
			requests:  1,
			resetTime: now.Add(time.Minute),
		}
		return true
	}

	// Check limits
	if entry.requests >= v.config.MaxRequestsPerMinute {
		return false
	}
	if entry.tokens+tokens > v.config.MaxTokensPerMinute {
		return false
	}

	// Update counters
	entry.tokens += tokens
	entry.requests++
	return true
}

// RecordTokenUsage updates token usage for rate limiting.
func (v *Validator) RecordTokenUsage(userID string, tokens int) {
	v.rateLimitMu.Lock()
	defer v.rateLimitMu.Unlock()

	if entry, exists := v.rateLimits[userID]; exists {
		entry.tokens += tokens
	}
}
