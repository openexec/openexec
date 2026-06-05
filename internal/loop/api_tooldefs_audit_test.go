package loop

import (
	"encoding/json"
	"testing"
)

// The direct-API path keeps its own tool definitions (apiToolDefinitions) —
// a second schema source, separate from internal/mcp's defs. This mirrors
// internal/mcp/schema_audit_test.go for that copy: every tool and parameter
// must document itself. Keep both audits when changing either set.
func TestAPIToolDefinitionsAreDocumented(t *testing.T) {
	defs := BuildAPIToolDefinitions()
	if len(defs) == 0 {
		t.Fatal("no API tool definitions")
	}
	for _, def := range defs {
		if len(def.Description) < 30 {
			t.Errorf("%s: tool description too short: %q", def.Name, def.Description)
		}
		var schema struct {
			Properties map[string]struct {
				Type        string `json:"type"`
				Description string `json:"description"`
			} `json:"properties"`
			Required []string `json:"required"`
		}
		if err := json.Unmarshal(def.InputSchema, &schema); err != nil {
			t.Errorf("%s: invalid inputSchema JSON: %v", def.Name, err)
			continue
		}
		for pname, prop := range schema.Properties {
			if len(prop.Description) < 10 {
				t.Errorf("%s.%s: parameter description missing or too short", def.Name, pname)
			}
			if prop.Type == "" {
				t.Errorf("%s.%s: parameter missing type", def.Name, pname)
			}
		}
		for _, r := range schema.Required {
			if _, ok := schema.Properties[r]; !ok {
				t.Errorf("%s: required field %q not in properties", def.Name, r)
			}
		}
	}
}
