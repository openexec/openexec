package util

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// SanitizeJSON attempts to fix common LLM JSON formatting issues.
// It handles markdown blocks, trailing commas, comments, and truncation.
func SanitizeJSON(input string) ([]byte, error) {
	// Strategy 1: Find JSON in markdown code blocks
	reJSON := regexp.MustCompile("(?s)```(?:json)?\n?(.*?)\n?```")
	matches := reJSON.FindStringSubmatch(input)

	jsonText := input
	if len(matches) > 1 {
		jsonText = matches[1]
	} else {
		// Strategy 2: Extract JSON from surrounding text.
		// Check both { and [ and use whichever appears first,
		// so arrays like [{"key":"val"}] aren't misextracted as {"key":"val"}.
		firstBrace := strings.Index(input, "{")
		lastBrace := strings.LastIndex(input, "}")
		firstBracket := strings.Index(input, "[")
		lastBracket := strings.LastIndex(input, "]")

		first, last := -1, -1
		if firstBrace != -1 && lastBrace > firstBrace {
			first, last = firstBrace, lastBrace
		}
		if firstBracket != -1 && lastBracket > firstBracket {
			// Use array bounds if they appear before object bounds (or no object found)
			if first == -1 || firstBracket < first {
				first, last = firstBracket, lastBracket
			}
		}

		if first != -1 && last != -1 && last > first {
			jsonText = input[first : last+1]
		}
	}

	jsonText = strings.TrimSpace(jsonText)
	if jsonText == "" {
		return nil, fmt.Errorf("no JSON content found")
	}

	// Strategy 3: Basic cleanup (comments and trailing commas)
	// Note: // comment stripping was removed because it corrupts JSON containing
	// file paths (e.g., /tmp/x.patch) and URLs (e.g., https://example.com).
	cleaned := jsonText

	// Remove trailing commas before } or ]
	reCommas := regexp.MustCompile(`,\s*([\}\]])`)
	cleaned = reCommas.ReplaceAllString(cleaned, "$1")

	// Try to parse
	var temp interface{}
	if err := json.Unmarshal([]byte(cleaned), &temp); err == nil {
		return []byte(cleaned), nil
	}

	// Strategy 4: Handle truncation by balancing braces
	if strings.HasPrefix(cleaned, "{") {
		opens := strings.Count(cleaned, "{")
		closes := strings.Count(cleaned, "}")
		if opens > closes {
			balanced := cleaned + strings.Repeat("}", opens-closes)
			if err := json.Unmarshal([]byte(balanced), &temp); err == nil {
				return []byte(balanced), nil
			}
		}
	} else if strings.HasPrefix(cleaned, "[") {
		opens := strings.Count(cleaned, "[")
		closes := strings.Count(cleaned, "]")
		if opens > closes {
			balanced := cleaned + strings.Repeat("]", opens-closes)
			if err := json.Unmarshal([]byte(balanced), &temp); err == nil {
				return []byte(balanced), nil
			}
		}
	}

	return nil, fmt.Errorf("failed to parse JSON after all recovery attempts")
}

// UnmarshalRobust attempts to parse JSON using sanitization strategies.
func UnmarshalRobust(input string, v interface{}) error {
	data, err := SanitizeJSON(input)
	if err != nil {
		// Fallback to standard unmarshal just in case
		return json.Unmarshal([]byte(input), v)
	}
	return json.Unmarshal(data, v)
}
