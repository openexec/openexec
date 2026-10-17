package validation

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// Keep UI tests in the ordinary CI build job, where failure blocks the build.
func TestCIRequiresUIUnitTests(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatal(err)
	}
	job := workflow.Jobs["build"]
	if job.If != "" || job.ContinueOnError {
		t.Fatal("CI build must be unconditional and required")
	}
	installed := false
	for _, step := range job.Steps {
		if step.Run == "npm ci && npm run build" && step.Directory == "ui" && step.If == "" && !step.ContinueOnError {
			installed = true
		}
		if step.Run == "npx vitest run --fileParallelism=false" && step.Directory == "ui" {
			if !installed || step.If != "" || step.ContinueOnError {
				t.Fatal("UI tests must follow dependency installation and propagate failures unconditionally")
			}
			return
		}
	}
	t.Fatal("CI build is missing required UI vitest step")
}
