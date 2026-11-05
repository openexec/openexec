package channeldecl

import (
	"os"
	"strings"
	"testing"
)

const valid = `channels:
  - id: openexec-ci
    type: ci
    repository: owner/repo
    branch: main
    bearer_token_env: TOKEN
    schedule: 10m
    timeout: 10s
`

func TestRepositoryDeclaration(t *testing.T) {
	data, err := os.ReadFile("../../.agent-console/channels.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(data); err != nil {
		t.Fatal(err)
	}
}

func TestValidate(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{"valid", valid, ""},
		{"quoted ID", strings.Replace(valid, "id: openexec-ci", `id: "openexec-ci"`, 1), ""},
		{"whitespace before ID", strings.Replace(valid, "id: openexec-ci", "id:   openexec-ci", 1), ""},
		{"missing channel", "channels: []", "channel missing"},
		{"other channel", strings.Replace(valid, "openexec-ci", "other", 1), "channel missing"},
		{"wrong type", strings.Replace(valid, "type: ci", "type: other", 1), "type must be ci"},
		{"missing repository", strings.Replace(valid, "    repository: owner/repo\n", "", 1), "missing repository"},
		{"missing branch", strings.Replace(valid, "    branch: main\n", "", 1), "missing branch"},
		{"missing token", strings.Replace(valid, "    bearer_token_env: TOKEN\n", "", 1), "missing bearer_token_env"},
		{"missing schedule", strings.Replace(valid, "    schedule: 10m\n", "", 1), "missing schedule"},
		{"missing timeout", strings.Replace(valid, "    timeout: 10s\n", "", 1), "missing timeout"},
		{"blank value", strings.Replace(valid, "branch: main", "branch: '  '", 1), "missing branch"},
		{"invalid yaml", "channels: [", "parse channels"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate([]byte(tc.input))
			if tc.want == "" && err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
		})
	}
}
