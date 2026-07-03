package mcpgov

import (
	"reflect"
	"strings"
	"testing"
)

// The durable schema audit for the governance MCP tools (moved here from
// internal/mcp when the adapter was extracted). Every tool definition must (a)
// document itself and every parameter, and (b) exactly match the Go struct its
// handler unmarshals into — drift produces silently-ignored arguments, the
// worst failure mode for an LLM caller.

// govToolDefs returns every governance tool definition the provider advertises.
func govToolDefs() []map[string]interface{} {
	return []map[string]interface{}{
		ListReleasesToolDef(),
		ListApprovedWorkToolDef(),
		GetWorkBriefToolDef(),
		ClaimWorkToolDef(),
		RecordPlanToolDef(),
		RequestRevisionToolDef(),
		RecordPRToolDef(),
		RecordTestEvidenceToolDef(),
		GenerateHandoffToolDef(),
		RequestDoneToolDef(),
	}
}

// TestGovProviderCoversAllTools asserts the audited set matches the provider's
// registered tools, so a new tool cannot be added without being audited.
func TestGovProviderCoversAllTools(t *testing.T) {
	registered := New().Tools()
	if len(registered) != len(govToolDefs()) {
		t.Fatalf("provider registers %d tools but %d are audited — keep govToolDefs in sync", len(registered), len(govToolDefs()))
	}
	for _, def := range govToolDefs() {
		name, _ := def["name"].(string)
		if _, ok := registered[name]; !ok {
			t.Errorf("audited tool %q is not registered by the provider", name)
		}
	}
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

// TestGovToolDefsAreDocumented: every tool and parameter carries a non-trivial
// description and a type; parameter names are snake_case; required fields exist.
func TestGovToolDefsAreDocumented(t *testing.T) {
	for _, def := range govToolDefs() {
		name, _ := def["name"].(string)
		if name == "" {
			t.Fatal("tool definition missing name")
		}
		if desc, _ := def["description"].(string); len(desc) < 30 {
			t.Errorf("%s: tool description too short to be useful: %q", name, desc)
		}
		props := schemaProperties(t, def)
		for pname, raw := range props {
			prop, _ := raw.(map[string]interface{})
			if pdesc, _ := prop["description"].(string); len(pdesc) < 10 {
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
// excluding fields tagged "-".
func jsonTagSet(v interface{}) map[string]bool {
	out := map[string]bool{}
	typ := reflect.TypeOf(v)
	for i := 0; i < typ.NumField(); i++ {
		name := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		out[name] = true
	}
	return out
}

// TestGovToolSchemasMatchRequestStructs: schema property names and handler
// struct json tags must be identical sets.
func TestGovToolSchemasMatchRequestStructs(t *testing.T) {
	cases := []struct {
		def map[string]interface{}
		req interface{}
	}{
		{ListReleasesToolDef(), ListReleasesRequest{}},
		{ListApprovedWorkToolDef(), ListApprovedWorkRequest{}},
		{GetWorkBriefToolDef(), GetWorkBriefRequest{}},
		{ClaimWorkToolDef(), ClaimWorkRequest{}},
		{RecordPlanToolDef(), RecordPlanRequest{}},
		{RequestRevisionToolDef(), RequestRevisionRequest{}},
		{RecordPRToolDef(), RecordPRRequest{}},
		{RecordTestEvidenceToolDef(), RecordTestEvidenceRequest{}},
		{GenerateHandoffToolDef(), GenerateHandoffRequest{}},
		{RequestDoneToolDef(), RequestDoneRequest{}},
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
