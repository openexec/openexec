package workflow

import (
	"fmt"
	"regexp"
	"strings"
)

var placeholderRe = regexp.MustCompile(`\{\{(\w+)\}\}`)

// Validate checks that all declared params have provided values.
// Returns an error listing missing params. Returns nil if params is nil (no declared params).
func Validate(declared map[string]string, provided map[string]string) error {
	if len(declared) == 0 {
		return nil
	}

	var missing []string
	for name := range declared {
		if _, ok := provided[name]; !ok {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing params: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Expand replaces {{name}} placeholders in text with provided values.
// Returns an error if any placeholder in the text has no matching value.
// If provided is nil or empty and text has no placeholders, returns text unchanged.
func Expand(text string, params map[string]string) (string, error) {
	if len(params) == 0 {
		// Check for unreplaced placeholders.
		if m := placeholderRe.FindString(text); m != "" {
			return "", fmt.Errorf("unreplaced placeholder %s (no params provided)", m)
		}
		return text, nil
	}

	result := text
	for name, value := range params {
		result = strings.ReplaceAll(result, "{{"+name+"}}", value)
	}

	// Check for any remaining unreplaced placeholders.
	if m := placeholderRe.FindString(result); m != "" {
		return "", fmt.Errorf("unreplaced placeholder %s", m)
	}

	return result, nil
}
