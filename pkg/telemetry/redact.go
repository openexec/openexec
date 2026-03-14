package telemetry

import (
    "crypto/sha256"
    "encoding/hex"
    "strings"
    "unicode/utf8"

    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/trace"
)

// Redactor provides best-effort redaction for span attributes.
// It is intentionally conservative: it removes or hashes values that might
// contain secrets, credentials, or excessive content.
type Redactor struct {
    // Keywords that signal a key/value may be sensitive
    SensitiveKeys []string
    // Max length for a value before hashing
    MaxValueLen int
}

// DefaultRedactor returns a Redactor with sensible defaults.
func DefaultRedactor() *Redactor {
    return &Redactor{
        SensitiveKeys: []string{
            "token", "secret", "password", "passwd", "api_key", "apikey",
            "authorization", "auth", "cookie", "set-cookie", "private_key",
        },
        MaxValueLen: 256,
    }
}

// AddSafeAttrs applies redaction and sets attributes on the span.
func (r *Redactor) AddSafeAttrs(span trace.Span, kv map[string]string) {
    if span == nil || kv == nil {
        return
    }
    attrs := make([]attribute.KeyValue, 0, len(kv))
    for k, v := range kv {
        safeV := r.redactValue(k, v)
        if safeV == "" {
            // empty after redaction — skip
            continue
        }
        attrs = append(attrs, attribute.String(k, safeV))
    }
    if len(attrs) > 0 {
        span.SetAttributes(attrs...)
    }
}

func (r *Redactor) redactValue(key, value string) string {
    if key == "" || value == "" {
        return value
    }
    lk := strings.ToLower(key)
    for _, sk := range r.SensitiveKeys {
        if strings.Contains(lk, sk) {
            return hash(value)
        }
    }
    // If looks like a secret (heuristic), hash it
    if looksSensitive(value) {
        return hash(value)
    }
    // Truncate overly long values to avoid payload bloat
    if utf8.RuneCountInString(value) > r.MaxValueLen {
        return hash(value)
    }
    return value
}

func looksSensitive(v string) bool {
    lv := strings.ToLower(v)
    if strings.Contains(lv, "-----begin ") { // PEM blocks
        return true
    }
    if strings.HasPrefix(lv, "sk-") || strings.HasPrefix(lv, "rk-") { // common key prefixes
        return true
    }
    if strings.Contains(lv, "eyj") && strings.Contains(lv, ".") { // jwt-like
        return true
    }
    return false
}

func hash(s string) string {
    sum := sha256.Sum256([]byte(s))
    return "sha256:" + hex.EncodeToString(sum[:])
}

