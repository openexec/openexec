package mcp

import (
	"regexp"
	"strings"
	"testing"
)

func TestRedactOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string // Strings that should NOT be in output
		expected []string // Strings that should be in output
	}{
		{
			name:     "empty input",
			input:    "",
			contains: nil,
			expected: []string{""},
		},
		{
			name:     "no secrets",
			input:    "Hello, World! This is a normal message.",
			contains: nil,
			expected: []string{"Hello, World! This is a normal message."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactOutput(tt.input)

			for _, shouldNotContain := range tt.contains {
				if strings.Contains(result, shouldNotContain) {
					t.Errorf("result should not contain %q, got %q", shouldNotContain, result)
				}
			}

			for _, shouldContain := range tt.expected {
				if !strings.Contains(result, shouldContain) {
					t.Errorf("result should contain %q, got %q", shouldContain, result)
				}
			}
		})
	}
}

func TestRedactAWSKeys(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "AWS access key AKIA",
			input: "export AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE",
		},
		{
			name:  "AWS access key ASIA",
			input: "aws_access_key_id = ASIAIOSFODNN7EXAMPLE",
		},
		{
			name:  "AWS secret key",
			input: "aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactOutput(tt.input)

			if !strings.Contains(result, RedactedPlaceholder) {
				t.Errorf("expected redaction in %q, got %q", tt.input, result)
			}
		})
	}
}

func TestRedactOpenAIKey(t *testing.T) {
	input := "OPENAI_API_KEY=sk-1234567890abcdefghijklmnopqrstuvwxyzABCD"
	result := RedactOutput(input)

	if strings.Contains(result, "sk-1234567890") {
		t.Errorf("OpenAI API key should be redacted, got %q", result)
	}

	if !strings.Contains(result, RedactedPlaceholder) {
		t.Errorf("expected redaction placeholder in output, got %q", result)
	}
}

func TestRedactGitHubTokens(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "GitHub PAT ghp_",
			input: "GITHUB_TOKEN=ghp_1234567890abcdefghijklmnopqrstuvwxyz",
		},
		{
			name:  "GitHub OAuth gho_",
			input: "token: gho_1234567890abcdefghijklmnopqrstuvwxyz",
		},
		{
			name:  "GitHub fine-grained PAT",
			input: "GITHUB_TOKEN=github_pat_11ABCDEFGHIJKLMNOPQRST_0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVW",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactOutput(tt.input)

			if !strings.Contains(result, RedactedPlaceholder) {
				t.Errorf("expected redaction in %q, got %q", tt.input, result)
			}
		})
	}
}

func TestRedactSlackTokens(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Slack bot token",
			input: "SLACK_TOKEN=xoxb-123456789012-1234567890123-abcdefghijklmnopqrstuvwx",
		},
		{
			name:  "Slack app token",
			input: "SLACK_APP_TOKEN=xoxp-123456789012-1234567890123-abcdefghijklmnopqrstuvwx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactOutput(tt.input)

			if !strings.Contains(result, RedactedPlaceholder) {
				t.Errorf("expected redaction in %q, got %q", tt.input, result)
			}
		})
	}
}

func TestRedactStripeKeys(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Stripe secret key",
			input: "STRIPE_SECRET_KEY=sk_live_1234567890abcdefghijklmnopqrstuvwxyz",
		},
		{
			name:  "Stripe publishable key",
			input: "STRIPE_KEY=pk_live_1234567890abcdefghijklmnopqrstuvwxyz",
		},
		{
			name:  "Stripe restricted key",
			input: "STRIPE_RESTRICTED=rk_live_1234567890abcdefghijklmnopqrstuvwxyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactOutput(tt.input)

			if !strings.Contains(result, RedactedPlaceholder) {
				t.Errorf("expected redaction in %q, got %q", tt.input, result)
			}
		})
	}
}

func TestRedactJWT(t *testing.T) {
	// Sample JWT (not a real one, just the correct format)
	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIn0.Sfl1234567890abcdef"
	input := "Authorization: Bearer " + jwt

	result := RedactOutput(input)

	if strings.Contains(result, jwt) {
		t.Errorf("JWT should be redacted, got %q", result)
	}
}

func TestRedactPrivateKey(t *testing.T) {
	input := `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA0Z3VS5JJcds3xfn/ygWyF8pWz5iBiCmN
-----END RSA PRIVATE KEY-----`

	result := RedactOutput(input)

	if !strings.Contains(result, RedactedPlaceholder) {
		t.Errorf("private key header should be redacted, got %q", result)
	}
}

func TestRedactURLWithCredentials(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "PostgreSQL URL",
			input: "DATABASE_URL=postgresql://user:password123@localhost:5432/dbname",
		},
		{
			name:  "MySQL URL",
			input: "MYSQL_URL=mysql://admin:secretpass@db.example.com:3306/mydb",
		},
		{
			name:  "Redis URL",
			input: "REDIS_URL=redis://:mypassword@redis.example.com:6379",
		},
		{
			name:  "MongoDB URL",
			input: "MONGO_URI=mongodb://user:pass@mongo.example.com:27017/mydb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactOutput(tt.input)

			if !strings.Contains(result, RedactedPlaceholder) {
				t.Errorf("expected redaction in %q, got %q", tt.input, result)
			}
		})
	}
}

func TestRedactGenericSecrets(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "password assignment",
			input: "password=mysecretpassword123",
		},
		{
			name:  "password in JSON",
			input: `{"password": "mysecretpassword123"}`,
		},
		{
			name:  "api_key assignment",
			input: "api_key=abc123def456ghi789jkl012",
		},
		{
			name:  "token assignment",
			input: "token: verysecrettokenvalue1234",
		},
		{
			name:  "secret assignment",
			input: "secret = my_super_secret_value",
		},
		{
			name:  "auth_token",
			input: "auth_token='authentication_token_value'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactOutput(tt.input)

			if !strings.Contains(result, RedactedPlaceholder) {
				t.Errorf("expected redaction in %q, got %q", tt.input, result)
			}
		})
	}
}

func TestRedactorWithEnvVars(t *testing.T) {
	envVars := map[string]string{
		"MY_SECRET":    "supersecretvalue123",
		"API_KEY":      "apikey-12345-abcdef",
		"SHORT_VAL":    "short",        // Too short, should not be redacted
		"EMPTY_VAL":    "",             // Empty, should not be redacted
		"DB_PASSWORD":  "dbpass9876!@", // Should be redacted
	}

	input := `
Config loaded:
- MY_SECRET: supersecretvalue123
- API_KEY: apikey-12345-abcdef
- SHORT_VAL: short
- DB_PASSWORD: dbpass9876!@
- Other value: not a secret
`

	result := RedactWithEnv(input, envVars)

	// Long secrets should be redacted
	if strings.Contains(result, "supersecretvalue123") {
		t.Error("MY_SECRET value should be redacted")
	}
	if strings.Contains(result, "apikey-12345-abcdef") {
		t.Error("API_KEY value should be redacted")
	}
	if strings.Contains(result, "dbpass9876!@") {
		t.Error("DB_PASSWORD value should be redacted")
	}

	// Short value should NOT be redacted (below minimum length)
	if !strings.Contains(result, "short") {
		t.Error("SHORT_VAL should not be redacted (too short)")
	}

	// Non-secret value should remain
	if !strings.Contains(result, "not a secret") {
		t.Error("non-secret value should remain")
	}
}

func TestRedactorWithCustomPatterns(t *testing.T) {
	customPattern := regexp.MustCompile(`CUSTOM-[A-Z0-9]{8}`)

	r := NewRedactor(WithCustomPatterns(customPattern))
	input := "My custom token is CUSTOM-AB12CD34 and normal text here."

	result := r.Redact(input)

	if strings.Contains(result, "CUSTOM-AB12CD34") {
		t.Error("custom pattern should be redacted")
	}
	if !strings.Contains(result, RedactedPlaceholder) {
		t.Error("expected redaction placeholder")
	}
	if !strings.Contains(result, "normal text here") {
		t.Error("non-matching text should remain")
	}
}

func TestRedactorWithMinSecretLength(t *testing.T) {
	envVars := map[string]string{
		"SHORT":  "abc",           // 3 chars
		"MEDIUM": "abcdefgh",      // 8 chars (default threshold)
		"LONG":   "abcdefghijklm", // 13 chars
	}

	// Test with default min length (8)
	r := NewRedactor(WithEnvVars(envVars))
	input := "Values: abc, abcdefgh, abcdefghijklm"
	result := r.Redact(input)

	if !strings.Contains(result, "abc") {
		t.Error("short value (3 chars) should not be redacted with default min length")
	}

	// Test with custom min length (5)
	r2 := NewRedactor(WithEnvVars(envVars), WithMinSecretLength(5))
	result2 := r2.Redact(input)

	if !strings.Contains(result2, "abc") {
		t.Error("short value (3 chars) should not be redacted with min length 5")
	}
}

func TestRedactorWithPatterns(t *testing.T) {
	customPatterns := []SecretPattern{
		{Name: "only_pattern", Pattern: regexp.MustCompile(`SECRET-[0-9]+`)},
	}

	r := NewRedactor(WithPatterns(customPatterns))
	input := "SECRET-12345 and sk-normalopenaiapikey1234567890"

	result := r.Redact(input)

	// Custom pattern should be redacted
	if strings.Contains(result, "SECRET-12345") {
		t.Error("custom pattern should be redacted")
	}

	// Default pattern (OpenAI key) should NOT be redacted because we replaced all patterns
	if !strings.Contains(result, "sk-normalopenaiapikey") {
		t.Error("default patterns should be replaced, so OpenAI key should remain")
	}
}

func TestNewRedactorDefaults(t *testing.T) {
	r := NewRedactor()

	if r.minSecretLength != 8 {
		t.Errorf("default minSecretLength = %d, want 8", r.minSecretLength)
	}

	if len(r.patterns) == 0 {
		t.Error("default patterns should be set")
	}

	if len(r.envVars) != 0 {
		t.Error("default envVars should be empty")
	}

	if len(r.customPatterns) != 0 {
		t.Error("default customPatterns should be empty")
	}
}

func TestFilterSensitiveEnvVars(t *testing.T) {
	envVars := map[string]string{
		"PATH":                    "/usr/bin:/bin",
		"HOME":                    "/home/user",
		"API_KEY":                 "secret-api-key",
		"DATABASE_PASSWORD":       "db-secret",
		"AWS_SECRET_ACCESS_KEY":   "aws-secret",
		"OPENAI_API_KEY":          "sk-openai",
		"GITHUB_TOKEN":            "ghp-token",
		"MY_CUSTOM_SECRET":        "custom-secret",
		"SOME_TOKEN_VALUE":        "some-token",
		"NORMAL_VAR":              "normal-value",
		"LOG_LEVEL":               "debug",
	}

	result := FilterSensitiveEnvVars(envVars)

	// Should include sensitive vars
	sensitiveKeys := []string{
		"API_KEY",
		"DATABASE_PASSWORD",
		"AWS_SECRET_ACCESS_KEY",
		"OPENAI_API_KEY",
		"GITHUB_TOKEN",
		"MY_CUSTOM_SECRET",
		"SOME_TOKEN_VALUE",
	}

	for _, key := range sensitiveKeys {
		if _, ok := result[key]; !ok {
			t.Errorf("expected %q to be included in sensitive vars", key)
		}
	}

	// Should NOT include non-sensitive vars
	nonSensitiveKeys := []string{
		"PATH",
		"HOME",
		"NORMAL_VAR",
		"LOG_LEVEL",
	}

	for _, key := range nonSensitiveKeys {
		if _, ok := result[key]; ok {
			t.Errorf("expected %q to NOT be included in sensitive vars", key)
		}
	}
}

func TestContainsSensitivePattern(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		{"API_KEY", true},
		{"MY_SECRET", true},
		{"AUTH_TOKEN", true},
		{"DB_PASSWORD", true},
		{"PASSWORD_RESET", true},
		{"CREDENTIAL_FILE", true},
		{"KEY_ID", true},
		{"SECRET_VALUE", true},
		{"PATH", false},
		{"HOME", false},
		{"LOG_LEVEL", false},
		{"NORMAL_VAR", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := containsSensitivePattern(tt.key)
			if result != tt.expected {
				t.Errorf("containsSensitivePattern(%q) = %v, want %v", tt.key, result, tt.expected)
			}
		})
	}
}

func TestRedactedPlaceholder(t *testing.T) {
	if RedactedPlaceholder != "[REDACTED]" {
		t.Errorf("RedactedPlaceholder = %q, want [REDACTED]", RedactedPlaceholder)
	}
}

func TestRedactMultipleSecrets(t *testing.T) {
	input := `
Environment:
AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
GITHUB_TOKEN=ghp_1234567890abcdefghijklmnopqrstuvwxyz
DATABASE_URL=postgresql://user:password@localhost:5432/db
API_KEY=sk-1234567890abcdefghijklmnopqrstuvwxyzABCD
`

	result := RedactOutput(input)

	// All secrets should be redacted
	secrets := []string{
		"AKIAIOSFODNN7EXAMPLE",
		"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"ghp_1234567890abcdefghijklmnopqrstuvwxyz",
		"user:password@",
		"sk-1234567890abcdefghijklmnopqrstuvwxyzABCD",
	}

	for _, secret := range secrets {
		if strings.Contains(result, secret) {
			t.Errorf("secret %q should be redacted", secret)
		}
	}

	// Should contain multiple redaction placeholders
	count := strings.Count(result, RedactedPlaceholder)
	if count < 3 {
		t.Errorf("expected at least 3 redactions, got %d", count)
	}
}

func TestRedactPreservesStructure(t *testing.T) {
	input := `{
  "api_key": "sk-secretkey1234567890abcdefghijklm",
  "name": "test",
  "nested": {
    "password": "nestedpassword123"
  }
}`

	result := RedactOutput(input)

	// Structure should be preserved
	if !strings.Contains(result, `"name": "test"`) {
		t.Error("non-secret structure should be preserved")
	}
	if !strings.Contains(result, "nested") {
		t.Error("nested structure should be preserved")
	}
}

func TestRedactAnthropicKeys(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Anthropic API key sk-ant prefix",
			input: "ANTHROPIC_API_KEY=sk-ant-api03-1234567890abcdefghijklmnopqrstuv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactOutput(tt.input)

			if !strings.Contains(result, RedactedPlaceholder) {
				t.Errorf("expected redaction in %q, got %q", tt.input, result)
			}
		})
	}
}

func TestRedactGoogleKeys(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Google API key",
			input: "GOOGLE_API_KEY=AIzaSyB1234567890abcdefghijklmnop_qrst",
		},
		{
			name:  "Google OAuth token",
			input: "Access token: ya29.a0AfH6SMA1234567890abcdefghijklmnopqrstuvwxyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactOutput(tt.input)

			if !strings.Contains(result, RedactedPlaceholder) {
				t.Errorf("expected redaction in %q, got %q", tt.input, result)
			}
		})
	}
}

func TestRedactTwilioSID(t *testing.T) {
	input := "TWILIO_ACCOUNT_SID=AC12345678901234567890123456789012"
	result := RedactOutput(input)

	if !strings.Contains(result, RedactedPlaceholder) {
		t.Errorf("Twilio SID should be redacted, got %q", result)
	}
}

func TestRedactSendGridKey(t *testing.T) {
	input := "SENDGRID_API_KEY=SG.abcdefghijklmnopqrstuv.1234567890abcdefghijklmnopqrstuvwxyzABCDE"
	result := RedactOutput(input)

	if !strings.Contains(result, RedactedPlaceholder) {
		t.Errorf("SendGrid key should be redacted, got %q", result)
	}
}

func TestRedactHuggingFaceToken(t *testing.T) {
	input := "HF_TOKEN=hf_abcdefghijklmnopqrstuvwxyz012345"
	result := RedactOutput(input)

	if !strings.Contains(result, RedactedPlaceholder) {
		t.Errorf("HuggingFace token should be redacted, got %q", result)
	}
}

func TestRedactNPMToken(t *testing.T) {
	input := "NPM_TOKEN=npm_abcdefghijklmnopqrstuvwxyz0123456789"
	result := RedactOutput(input)

	if !strings.Contains(result, RedactedPlaceholder) {
		t.Errorf("NPM token should be redacted, got %q", result)
	}
}

func TestRedactBasicAuth(t *testing.T) {
	input := "Authorization: Basic dXNlcm5hbWU6cGFzc3dvcmQxMjM0NTY3ODk="
	result := RedactOutput(input)

	if !strings.Contains(result, RedactedPlaceholder) {
		t.Errorf("Basic auth should be redacted, got %q", result)
	}
}

func TestRedactBearerToken(t *testing.T) {
	input := "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"
	result := RedactOutput(input)

	if !strings.Contains(result, RedactedPlaceholder) {
		t.Errorf("Bearer token should be redacted, got %q", result)
	}
}

func TestSensitiveEnvVarNames(t *testing.T) {
	// Verify that the list contains expected sensitive names
	expectedNames := []string{
		"API_KEY",
		"AWS_SECRET_ACCESS_KEY",
		"DATABASE_PASSWORD",
		"GITHUB_TOKEN",
		"PASSWORD",
		"SECRET",
		"TOKEN",
	}

	for _, expected := range expectedNames {
		found := false
		for _, name := range SensitiveEnvVarNames {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q to be in SensitiveEnvVarNames", expected)
		}
	}
}

func TestRedactEmptyInput(t *testing.T) {
	r := NewRedactor()
	result := r.Redact("")
	if result != "" {
		t.Errorf("empty input should return empty output, got %q", result)
	}
}

func TestRedactWhitespaceOnly(t *testing.T) {
	input := "   \n\t  \n   "
	result := RedactOutput(input)

	if result != input {
		t.Errorf("whitespace-only input should remain unchanged, got %q", result)
	}
}

func TestGetSortedEnvValues(t *testing.T) {
	r := NewRedactor(WithEnvVars(map[string]string{
		"SHORT":  "abc",
		"MEDIUM": "abcdef",
		"LONG":   "abcdefghij",
		"EMPTY":  "",
	}))

	values := r.getSortedEnvValues()

	// Should not include empty value
	for _, v := range values {
		if v == "" {
			t.Error("empty values should be excluded")
		}
	}

	// Should be sorted by length descending
	for i := 0; i < len(values)-1; i++ {
		if len(values[i]) < len(values[i+1]) {
			t.Errorf("values should be sorted by length descending: %v", values)
		}
	}
}

func TestRedactorEnvVarOverlap(t *testing.T) {
	// Test case where one env var value is a substring of another
	envVars := map[string]string{
		"PARTIAL":  "secretvalue",
		"FULL":     "mysecretvaluehere",
	}

	r := NewRedactor(WithEnvVars(envVars))
	input := "Found: mysecretvaluehere and secretvalue"

	result := r.Redact(input)

	// Both should be redacted, with longer one first to avoid partial matches
	if strings.Contains(result, "mysecretvaluehere") {
		t.Error("full value should be redacted")
	}
	if strings.Contains(result, "secretvalue") {
		t.Error("partial value should be redacted")
	}
}

func TestRedactGitLabToken(t *testing.T) {
	input := "GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxx"
	result := RedactOutput(input)

	if !strings.Contains(result, RedactedPlaceholder) {
		t.Errorf("GitLab token should be redacted, got %q", result)
	}
}

func TestRedactMailgunKey(t *testing.T) {
	input := "MAILGUN_API_KEY=key-12345678901234567890123456789012"
	result := RedactOutput(input)

	if !strings.Contains(result, RedactedPlaceholder) {
		t.Errorf("Mailgun key should be redacted, got %q", result)
	}
}

func TestRedactDigitalOceanToken(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "DigitalOcean personal access token",
			input: "DO_TOKEN=dop_v1_1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		},
		{
			name:  "DigitalOcean OAuth token",
			input: "DO_OAUTH=doo_v1_1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactOutput(tt.input)

			if !strings.Contains(result, RedactedPlaceholder) {
				t.Errorf("expected redaction in %q, got %q", tt.input, result)
			}
		})
	}
}

func TestRedactSquareToken(t *testing.T) {
	input := "SQUARE_TOKEN=sq0atp-1234567890abcdefghijklmn"
	result := RedactOutput(input)

	if !strings.Contains(result, RedactedPlaceholder) {
		t.Errorf("Square token should be redacted, got %q", result)
	}
}

func TestRedactShopifyToken(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Shopify shared secret",
			input: "SHOPIFY_SECRET=shpss_12345678901234567890123456789012",
		},
		{
			name:  "Shopify access token",
			input: "SHOPIFY_TOKEN=shpat_12345678901234567890123456789012",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactOutput(tt.input)

			if !strings.Contains(result, RedactedPlaceholder) {
				t.Errorf("expected redaction in %q, got %q", tt.input, result)
			}
		})
	}
}

func TestRedactReplicateToken(t *testing.T) {
	input := "REPLICATE_API_TOKEN=r8_123456789012345678901234567890"
	result := RedactOutput(input)

	if !strings.Contains(result, RedactedPlaceholder) {
		t.Errorf("Replicate token should be redacted, got %q", result)
	}
}

func TestRedactWebhookSecret(t *testing.T) {
	input := "WEBHOOK_SECRET=whsec_12345678901234567890123456789012"
	result := RedactOutput(input)

	if !strings.Contains(result, RedactedPlaceholder) {
		t.Errorf("Webhook secret should be redacted, got %q", result)
	}
}
