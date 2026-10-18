package validation

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Keep UI tests in the ordinary CI build job, where failure blocks the build.
func TestCIRequiresUIUnitTests(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCIUIUnitTests(data); err != nil {
		t.Fatal(err)
	}
}

func validateCIUIUnitTests(data []byte) error {
	var workflow struct {
		Jobs map[string]struct {
			If              string `yaml:"if"`
			ContinueOnError bool   `yaml:"continue-on-error"`
			Steps           []struct {
				Run             string `yaml:"run"`
				Directory       string `yaml:"working-directory"`
				If              string `yaml:"if"`
				ContinueOnError bool   `yaml:"continue-on-error"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return err
	}
	job := workflow.Jobs["build"]
	if job.If != "" || job.ContinueOnError {
		return fmt.Errorf("CI build must be unconditional and required")
	}
	installed := false
	for _, step := range job.Steps {
		if step.Run == "npm ci && npm run build" && step.Directory == "ui" && step.If == "" && !step.ContinueOnError {
			installed = true
		}
		if step.Run == "npx vitest run --fileParallelism=false" && step.Directory == "ui" {
			if !installed || step.If != "" || step.ContinueOnError {
				return fmt.Errorf("UI tests must follow dependency installation and propagate failures unconditionally")
			}
			return nil
		}
	}
	return fmt.Errorf("CI build is missing required UI vitest step")
}

// Exercise the same validator as the repository workflow test. These mutations
// are in memory so the negative controls run in every ordinary Go test gate.
func TestCIRequiresUIUnitTestsNegativeControls(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	original := string(data)
	step := "    - name: Test UI\n      run: npx vitest run --fileParallelism=false\n      working-directory: ui\n"
	cases := []struct {
		name, old, replacement, want string
	}{
		{"removed", step, "", "CI build is missing required UI vitest step"},
		{"conditional", step, step + "      if: github.event_name == 'push'\n", "UI tests must follow dependency installation and propagate failures unconditionally"},
		{"ignored failure", step, step + "      continue-on-error: true\n", "UI tests must follow dependency installation and propagate failures unconditionally"},
		{"wrong directory", step, strings.Replace(step, "working-directory: ui", "working-directory: .", 1), "CI build is missing required UI vitest step"},
		{"missing installation", "run: npm ci && npm run build", "run: npm run build", "UI tests must follow dependency installation and propagate failures unconditionally"},
		{"conditional job", "  build:\n", "  build:\n    if: github.event_name == 'push'\n", "CI build must be unconditional and required"},
		{"ignored job failure", "  build:\n", "  build:\n    continue-on-error: true\n", "CI build must be unconditional and required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if strings.Count(original, tc.old) != 1 {
				t.Fatal("mutation must match exactly once")
			}
			mutated := strings.Replace(original, tc.old, tc.replacement, 1)
			err := validateCIUIUnitTests([]byte(mutated))
			if err == nil || err.Error() != tc.want {
				t.Fatalf("got %v; want %q", err, tc.want)
			}
		})
	}
}
