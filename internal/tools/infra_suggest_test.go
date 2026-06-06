package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/infra"
)

func suggestTestRegistry(t *testing.T) *infra.Registry {
	t.Helper()
	reg, err := infra.NewRegistry(&infra.Config{
		Enabled: true,
		Environments: map[string]*infra.EnvConfig{
			"staging": {
				RiskProfile: "low",
				Ansible: &infra.AnsibleConfig{
					PlaybookDir: "/etc/openexec/playbooks",
					Inventory:   "/etc/openexec/inv.ini",
					Playbooks:   []string{"rolling_restart.yml"},
				},
				SSH: &infra.SSHConfig{
					AllowedHosts: []string{"10.0.1.*"},
					Queries:      map[string][]string{"check_disk": {"df", "-h"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func TestNewInfraSuggestTools_OnlyConfiguredEngines(t *testing.T) {
	got := NewInfraSuggestTools(suggestTestRegistry(t))
	names := map[string]bool{}
	for _, tool := range got {
		names[tool.Name()] = true
	}
	if !names["ansible_run_playbook"] || !names["ssh_run_query"] {
		t.Errorf("configured engines missing: %v", names)
	}
	if names["salt_apply_state"] || names["terraform_plan"] {
		t.Errorf("unconfigured engines must not be suggested: %v", names)
	}
}

func TestInfraSuggestTool_SchemaIsValidJSONWithEnums(t *testing.T) {
	for _, tool := range NewInfraSuggestTools(suggestTestRegistry(t)) {
		var schema map[string]interface{}
		if err := json.Unmarshal([]byte(tool.InputSchema()), &schema); err != nil {
			t.Errorf("%s: schema is not valid JSON: %v", tool.Name(), err)
		}
		if len(tool.Description()) < 30 {
			t.Errorf("%s: description too short for routing word-overlap scoring", tool.Name())
		}
	}
}

func TestInfraSuggestTool_ExecuteAlwaysRefuses(t *testing.T) {
	// The DCP plane proposes, the MCP plane disposes: a routed suggestion
	// must never be executable in place, even if AllowExecution were
	// misconfigured upstream.
	for _, tool := range NewInfraSuggestTools(suggestTestRegistry(t)) {
		if _, err := tool.Execute(context.Background(), map[string]interface{}{"environment": "staging"}); err == nil {
			t.Errorf("%s: Execute must refuse in the routing plane", tool.Name())
		} else if !strings.Contains(err.Error(), "suggestion-only") {
			t.Errorf("%s: refusal should explain the boundary: %v", tool.Name(), err)
		}
	}
}
