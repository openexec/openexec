package mcp

import (
	"reflect"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/infra"
)

// These tests are the durable form of the tool-schema audit: every tool
// definition advertised over MCP must (a) document itself and every
// parameter, and (b) exactly match the Go struct its handler unmarshals
// into. Drift between schema property names and struct json tags produces
// silently-ignored arguments — the worst failure mode for an LLM caller.

// allToolDefs returns every tool definition the server can advertise.
func allToolDefs() []map[string]interface{} {
	return []map[string]interface{}{
		axonSignalToolDef(),
		ReadFileToolDef(),
		WriteFileToolDef(),
		RunShellCommandToolDef(),
		GitApplyPatchToolDef(),
		ForkSessionToolDef(),
		GetForkInfoToolDef(),
		ListSessionForksToolDef(),
		OpenExecResultToolDef(),
		OpenExecActionToolDef(),
		BacklogListStoriesToolDef(),
		BacklogGetStoryToolDef(),
		BacklogClaimStoryToolDef(),
		BacklogCompleteTaskToolDef(),
		BacklogCompleteStoryToolDef(),
		BacklogAddTaskToolDef(),
		MemoryReadToolDef(),
		SkillProposeToolDef(),
		// Infra tool schemas are compiled from the operator allowlist;
		// audit them against a representative config.
		AnsibleRunPlaybookToolDef(auditInfraRegistry()),
		SaltApplyStateToolDef(auditInfraRegistry()),
		SSHRunQueryToolDef(auditInfraRegistry()),
		TerraformPlanToolDef(auditInfraRegistry()),
		TerraformApplyToolDef(auditInfraRegistry()),
		ApprovalListToolDef(),
		ApprovalDecideToolDef(),
	}
}

// auditInfraRegistry builds a minimal valid infra registry so the
// config-compiled tool schemas can be audited like static ones.
func auditInfraRegistry() *infra.Registry {
	reg, err := infra.NewRegistry(&infra.Config{
		Enabled: true,
		Environments: map[string]*infra.EnvConfig{
			"staging": {
				RiskProfile: "low",
				Ansible: &infra.AnsibleConfig{
					PlaybookDir: "/etc/openexec/playbooks",
					Inventory:   "/etc/openexec/inventory.ini",
					Playbooks:   []string{"deploy.yml"},
				},
				Salt: &infra.SaltConfig{
					Targets: []string{"web*"},
					States:  []string{"app.deploy"},
				},
				SSH: &infra.SSHConfig{
					AllowedHosts: []string{"10.0.1.*"},
					Queries:      map[string][]string{"check_disk": {"df", "-h"}},
				},
				Terraform: &infra.TerraformConfig{
					WorkingDir:       "./infra",
					AllowedVariables: []string{"instance_count"},
				},
			},
			// production is risk_profile=high: apply-class commands here
			// exercise the approval-gate paths in tests (staging is low →
			// autonomous under Phase-3 environment tiering).
			"production": {
				RiskProfile: "high",
				Ansible: &infra.AnsibleConfig{
					PlaybookDir: "/etc/openexec/playbooks",
					Inventory:   "/etc/openexec/prod.ini",
					Playbooks:   []string{"deploy.yml"},
				},
				Salt: &infra.SaltConfig{
					Targets: []string{"web*"},
					States:  []string{"app.deploy"},
				},
				Terraform: &infra.TerraformConfig{
					WorkingDir:       "./infra/prod",
					AllowedVariables: []string{"instance_count"},
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return reg
}

func schemaProperties(t *testing.T, def map[string]interface{}) map[string]interface{} {
	t.Helper()
	schema, ok := def["inputSchema"].(map[string]interface{})
	if !ok {
		t.Fatalf("tool %v has no inputSchema", def["name"])
	}
	props, _ := schema["properties"].(map[string]interface{})
	return props
}

// TestToolDefsAreDocumented: every tool and every parameter carries a
// non-trivial description, and required fields exist in properties.
func TestToolDefsAreDocumented(t *testing.T) {
	for _, def := range allToolDefs() {
		name, _ := def["name"].(string)
		if name == "" {
			t.Fatal("tool definition missing name")
		}
		desc, _ := def["description"].(string)
		if len(desc) < 30 {
			t.Errorf("%s: tool description too short to be useful: %q", name, desc)
		}

		props := schemaProperties(t, def)
		for pname, raw := range props {
			prop, _ := raw.(map[string]interface{})
			pdesc, _ := prop["description"].(string)
			if len(pdesc) < 10 {
				t.Errorf("%s.%s: parameter description missing or too short: %q", name, pname, pdesc)
			}
			if ptype, _ := prop["type"].(string); ptype == "" {
				t.Errorf("%s.%s: parameter missing type", name, pname)
			}
			if pname != strings.ToLower(pname) {
				t.Errorf("%s.%s: parameter names must be snake_case/lowercase", name, pname)
			}
		}

		schema, _ := def["inputSchema"].(map[string]interface{})
		if required, ok := schema["required"].([]string); ok {
			for _, r := range required {
				if _, exists := props[r]; !exists {
					t.Errorf("%s: required field %q not present in properties", name, r)
				}
			}
		}
	}
}

// jsonTagSet extracts the json tag names of a struct's exported fields,
// excluding fields tagged "-" (internal, not client-settable).
func jsonTagSet(v interface{}) map[string]bool {
	out := map[string]bool{}
	typ := reflect.TypeOf(v)
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if name == "" || name == "-" {
			continue
		}
		out[name] = true
	}
	return out
}

// TestToolSchemasMatchRequestStructs: schema property names and handler
// struct json tags must be identical sets.
func TestToolSchemasMatchRequestStructs(t *testing.T) {
	cases := []struct {
		def map[string]interface{}
		req interface{}
	}{
		{ReadFileToolDef(), ReadFileRequest{}},
		{WriteFileToolDef(), WriteFileRequest{}},
		{RunShellCommandToolDef(), RunShellCommandRequest{}},
		{GitApplyPatchToolDef(), GitApplyPatchRequest{}},
		{ForkSessionToolDef(), ForkSessionRequest{}},
		{GetForkInfoToolDef(), GetForkInfoRequest{}},
		{ListSessionForksToolDef(), ListSessionForksRequest{}},
		{OpenExecResultToolDef(), OpenExecResultRequest{}},
		{SkillProposeToolDef(), skillProposeArgs{}},
		{BacklogAddTaskToolDef(), backlogAddTaskArgs{}},
		{AnsibleRunPlaybookToolDef(auditInfraRegistry()), AnsibleRunPlaybookRequest{}},
		{SaltApplyStateToolDef(auditInfraRegistry()), SaltApplyStateRequest{}},
		{SSHRunQueryToolDef(auditInfraRegistry()), SSHRunQueryRequest{}},
		{TerraformPlanToolDef(auditInfraRegistry()), TerraformPlanRequest{}},
		{TerraformApplyToolDef(auditInfraRegistry()), TerraformApplyRequest{}},
		{ApprovalDecideToolDef(), ApprovalDecideRequest{}},
		// openexec_signal / openexec_action / backlog list are handled via
		// dynamic maps or inline structs; covered by TestToolDefsAreDocumented.
	}

	for _, tc := range cases {
		name, _ := tc.def["name"].(string)
		props := schemaProperties(t, tc.def)
		tags := jsonTagSet(tc.req)

		for pname := range props {
			if !tags[pname] {
				t.Errorf("%s: schema property %q has no matching json tag on %T — the argument would be silently ignored", name, pname, tc.req)
			}
		}
		for tag := range tags {
			if _, ok := props[tag]; !ok {
				t.Errorf("%s: struct field %q (%T) is not advertised in the schema — the model cannot discover it", name, tag, tc.req)
			}
		}
	}
}
