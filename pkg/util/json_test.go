package util

import (
	"encoding/json"
	"testing"
)

func TestJSONSanitization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "valid json",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
			wantErr:  false,
		},
		{
			name:     "markdown block",
			input:    "Here is the JSON:\n```json\n{\"key\": \"value\"}\n```\nHope this helps!",
			expected: `{"key": "value"}`,
			wantErr:  false,
		},
		{
			name:     "trailing comma",
			input:    `{"key": "value",}`,
			expected: `{"key": "value"}`,
			wantErr:  false,
		},
		{
			name:     "comments",
			input:    "{\n  \"key\": \"value\" // this is a comment\n}",
			expected: `{"key": "value"}`,
			wantErr:  false,
		},
		{
			name:     "truncated object",
			input:    `{"key": "value"`,
			expected: `{"key": "value"}`,
			wantErr:  false,
		},
		{
			name:     "truncated array",
			input:    `["one", "two"`,
			expected: `["one", "two"]`,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SanitizeJSON(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("SanitizeJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				var m interface{}
				if err := json.Unmarshal(got, &m); err != nil {
					t.Errorf("SanitizeJSON() invalid JSON: %v", err)
				}
			}
		})
	}
}

func TestUnmarshalRobust(t *testing.T) {
	type Data struct {
		Key string `json:"key"`
	}
	
	input := "```json\n{\"key\": \"value\",}\n```"
	var d Data
	err := UnmarshalRobust(input, &d)
	if err != nil {
		t.Fatalf("UnmarshalRobust failed: %v", err)
	}
	if d.Key != "value" {
		t.Errorf("expected value, got %q", d.Key)
	}
}
