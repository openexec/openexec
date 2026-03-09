package gates

import (
	"strings"
	"testing"
)

func TestRunPreflightChecks(t *testing.T) {
	// Mock the check functions
	oldDocker := dockerCheckFn
	oldNode := nodeCheckCheckFn
	oldPython := pythonCheckFn

	defer func() {
		dockerCheckFn = oldDocker
		nodeCheckCheckFn = oldNode
		pythonCheckFn = oldPython
	}()

	dockerCheckFn = func() PreflightCheck { return PreflightCheck{Name: "docker", Passed: true} }
	nodeCheckCheckFn = func() PreflightCheck { return PreflightCheck{Name: "node", Passed: true} }
	pythonCheckFn = func() PreflightCheck { return PreflightCheck{Name: "python", Passed: true} }

	tests := []struct {
		name      string
		taskTitle string
		gateNames []string
		wantNames []string
	}{
		{
			name:      "no requirements",
			taskTitle: "clean up readme",
			gateNames: []string{},
			wantNames: []string{},
		},
		{
			name:      "needs docker via title",
			taskTitle: "setup docker container",
			gateNames: []string{},
			wantNames: []string{"docker"},
		},
		{
			name:      "needs node via gate",
			taskTitle: "fix bug",
			gateNames: []string{"frontend_renders"},
			wantNames: []string{"node"},
		},
		{
			name:      "needs python via title",
			taskTitle: "build fastapi backend",
			gateNames: []string{},
			wantNames: []string{"python"},
		},
		{
			name:      "multiple requirements",
			taskTitle: "deploy dockerized react app",
			gateNames: []string{},
			wantNames: []string{"docker", "node"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := RunPreflightChecks(tt.taskTitle, tt.gateNames)

			// We check if the right check types were attempted
			// We can't guarantee they pass/fail in every CI env
			gotNames := []string{}
			for _, check := range report.Checks {
				gotNames = append(gotNames, check.Name)
			}

			for _, want := range tt.wantNames {
				found := false
				for _, got := range gotNames {
					if got == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected check %q not found in %v", want, gotNames)
				}
			}
		})
	}
}

func TestFormatPreflightReport(t *testing.T) {
	report := &PreflightReport{
		Passed:  false,
		Summary: "✗ Preflight failed: docker",
		Checks: []PreflightCheck{
			{
				Name:       "docker",
				Passed:     false,
				Error:      "Not installed",
				FixCommand: "brew install docker",
			},
		},
	}

	formatted := FormatPreflightReport(report)
	if !strings.Contains(formatted, "Preflight Checks FAILED") {
		t.Error("formatted report missing failure header")
	}
	if !strings.Contains(formatted, "brew install docker") {
		t.Error("formatted report missing fix command")
	}

	report.Passed = true
	report.Summary = "✓ OK"
	formatted = FormatPreflightReport(report)
	if formatted != "✓ OK" {
		t.Errorf("expected summary, got %q", formatted)
	}
}
