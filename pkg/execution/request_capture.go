package execution

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/openexec/openexec/pkg/agent"
)

// RequestCaptureEnv names a file the caller wants the last request written to
// when an API run fails or ends inconclusive, so the failure can be replayed
// and minimized against the same endpoint instead of guessed at from the
// error text. Unset, nothing is written. The file is private (0600), its
// message contents are truncated to a bounded size, and token-like strings
// are replaced; it is meant for the host that ran the turn, never published.
const RequestCaptureEnv = "OPENEXEC_REQUEST_CAPTURE"

// Capture bounds: each message text keeps its head and tail up to
// captureTextLimit characters, shrunk until the whole file fits captureLimit.
const (
	captureTextLimit = 2000
	captureLimit     = 512 << 10
	captureErrLimit  = 4000
)

// lastRequester is an adapter that remembers the body it last sent
// (agent.OpenAIProvider).
type lastRequester interface {
	LastRequest() ([]byte, time.Time, string)
}

func lastRequestOf(adapter agent.ProviderAdapter) lastRequester {
	for adapter != nil {
		if last, ok := adapter.(lastRequester); ok {
			return last
		}
		bounded, ok := adapter.(*boundedAPIAdapter)
		if !ok {
			return nil
		}
		adapter = bounded.ProviderAdapter
	}
	return nil
}

// captureRequest writes the request the run last sent, when the caller asked
// for captures and the request was sent by this run. Best effort: a capture
// that cannot be written changes nothing about the run.
func captureRequest(adapter agent.ProviderAdapter, started time.Time, result Result, runErr error) {
	path := strings.TrimSpace(os.Getenv(RequestCaptureEnv))
	if path == "" {
		return
	}
	last := lastRequestOf(adapter)
	if last == nil {
		return
	}
	body, sentAt, baseURL := last.LastRequest()
	if len(body) == 0 || sentAt.Before(started) {
		return
	}
	var request map[string]any
	if json.Unmarshal(body, &request) != nil {
		return
	}
	message := ""
	if runErr != nil {
		message = runErr.Error()
	} else if result.Outcome == OutcomeInconclusive {
		message = "inconclusive: " + result.Reason
	}
	capture := map[string]any{
		"version":    1,
		"capturedAt": time.Now().UTC().Format(time.RFC3339),
		"sentAt":     sentAt.UTC().Format(time.RFC3339),
		"provider":   adapter.GetName(),
		"baseURL":    redactSecrets(baseURL),
		"outcome":    result.Outcome,
		"reason":     result.Reason,
		"error":      truncateText(redactSecrets(message), captureErrLimit),
	}
	var data []byte
	for limit := captureTextLimit; ; limit /= 4 {
		sanitized, truncated := sanitizeCapture(request, limit)
		capture["request"], capture["truncated"], capture["textLimit"] = sanitized, truncated, limit
		var err error
		if data, err = json.MarshalIndent(capture, "", " "); err != nil {
			return
		}
		if len(data) <= captureLimit || limit < 50 {
			break
		}
	}
	_ = writePrivate(path, data)
}

func writePrivate(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".capture-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// sanitizeCapture copies a request body with every string redacted and each
// message's text, tool-call arguments and reasoning truncated to limit.
// Images are not kept. Structure (roles, fields, tools, parameters) is kept
// as sent, so a replay keeps the request's shape.
func sanitizeCapture(request map[string]any, limit int) (map[string]any, bool) {
	truncated := false
	var walk func(value any, bounded bool) any
	walk = func(value any, bounded bool) any {
		switch v := value.(type) {
		case string:
			text := redactSecrets(v)
			if bounded && len([]rune(text)) > limit {
				truncated = true
				return truncateText(text, limit)
			}
			return text
		case []any:
			out := make([]any, len(v))
			for i, item := range v {
				out[i] = walk(item, bounded)
			}
			return out
		case map[string]any:
			out := make(map[string]any, len(v))
			for key, item := range v {
				switch key {
				case "image_url", "input_audio":
					out[key] = "<omitted from capture>"
					truncated = true
				default:
					out[key] = walk(item, bounded || key == "messages")
				}
			}
			return out
		default:
			return v
		}
	}
	return walk(request, false).(map[string]any), truncated
}

func truncateText(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	head, tail := limit*3/4, limit/4
	return string(runes[:head]) + fmt.Sprintf(" …[%d characters cut]… ", len(runes)-head-tail) + string(runes[len(runes)-tail:])
}

var secretPatterns = []struct {
	pattern *regexp.Regexp
	with    string
}{
	{regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`), "<redacted private key>"},
	{regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{8,}`), "Bearer <redacted>"},
	{regexp.MustCompile(`\b(?:sk|pk|rk)-[A-Za-z0-9_-]{16,}`), "<redacted>"},
	{regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}|\bgithub_pat_[A-Za-z0-9_]{20,}`), "<redacted>"},
	{regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}`), "<redacted>"},
	{regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), "<redacted>"},
	{regexp.MustCompile(`(?i)\b([A-Za-z0-9_]*(?:api[_-]?key|token|secret|password|passwd)[A-Za-z0-9_]*)(["']?\s*[:=]\s*["']?)[^\s"',;]{6,}`), "${1}${2}<redacted>"},
	{regexp.MustCompile(`(?i)(://[^/\s:@]+:)[^@\s/]+@`), "${1}<redacted>@"},
}

func redactSecrets(text string) string {
	for _, secret := range secretPatterns {
		text = secret.pattern.ReplaceAllString(text, secret.with)
	}
	return text
}
