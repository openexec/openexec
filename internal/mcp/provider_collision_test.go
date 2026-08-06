package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/approval"
	"github.com/openexec/openexec/pkg/mcptool"
)

// fakeProvider is a minimal mcptool.Provider for collision tests.
type fakeProvider struct{ names []string }

func (f fakeProvider) Tools() map[string]mcptool.Tool {
	out := map[string]mcptool.Tool{}
	for _, n := range f.names {
		name := n
		out[name] = mcptool.Tool{
			Def:    map[string]interface{}{"name": name},
			Handle: func(h mcptool.Host, _ json.RawMessage) { h.WriteResult(name) },
		}
	}
	return out
}

func TestRegisterProvider_RejectsReservedCoreName(t *testing.T) {
	s := &Server{}
	// memory_read is a core tool; a module must not be able to shadow it.
	err := s.RegisterProvider(fakeProvider{names: []string{"memory_read"}})
	if err == nil {
		t.Fatal("expected collision error for reserved core name, got nil")
	}
	if len(s.providerTools) != 0 {
		t.Errorf("all-or-nothing violated: %d tools registered after rejection", len(s.providerTools))
	}
}

func TestRegisterProvider_RejectsCrossModuleDuplicate(t *testing.T) {
	s := &Server{}
	if err := s.RegisterProvider(fakeProvider{names: []string{"acme_widget"}}); err != nil {
		t.Fatalf("first registration should succeed: %v", err)
	}
	err := s.RegisterProvider(fakeProvider{names: []string{"acme_widget"}})
	if err == nil {
		t.Fatal("expected duplicate-name error from a second module, got nil")
	}
}

func TestRegisterProvider_AtomicOnPartialCollision(t *testing.T) {
	s := &Server{}
	// One fresh name, one reserved — the whole provider must be rejected.
	err := s.RegisterProvider(fakeProvider{names: []string{"acme_ok", "run_shell_command"}})
	if err == nil {
		t.Fatal("expected rejection when any tool collides")
	}
	if _, ok := s.providerTools["acme_ok"]; ok {
		t.Error("fresh tool was registered despite a sibling collision (not all-or-nothing)")
	}
}

// TestRegisterProvider_RejectsSymbolToolNames covers a bypass rather than a
// mere naming clash. The broker authorizes the symbol tools by name in every
// permission mode, and dispatchProvider runs a module handler before the core
// switch — so a module permitted to take one of these names would execute with
// a read-only session's blessing.
func TestRegisterProvider_RejectsSymbolToolNames(t *testing.T) {
	for _, name := range []string{"symbol_find", "symbol_read", "symbol_relations"} {
		t.Run(name, func(t *testing.T) {
			s := &Server{}
			if err := s.RegisterProvider(fakeProvider{names: []string{name}}); err == nil {
				t.Fatalf("module was allowed to shadow %s", name)
			}
			if len(s.providerTools) != 0 {
				t.Errorf("%s: %d tools registered after rejection", name, len(s.providerTools))
			}
		})
	}
}

// TestReservedNamesCoverEveryAdvertisedTool compares the reserved set with the
// audit list. Necessary but not sufficient — see the test below, which is the
// one that actually catches a forgotten tool.
func TestReservedNamesCoverEveryAdvertisedTool(t *testing.T) {
	reserved := reservedCoreToolNames()
	for _, def := range allToolDefs() {
		name, _ := def["name"].(string)
		if name != "" && !reserved[name] {
			t.Errorf("tool %q is advertised but not reserved — a module could shadow it", name)
		}
	}
}

// TestEveryAdvertisedToolIsReserved drives the real handleToolsList with every
// optional surface switched on, rather than comparing two hand-written lists.
//
// allToolDefs() is itself maintained by hand, so a tool added to
// handleToolsList and forgotten in both lists would leave the older test green
// while the tool stayed shadowable. This one reads what the server actually
// advertises, so forgetting the lists cannot hide it.
func TestEveryAdvertisedToolIsReserved(t *testing.T) {
	out := &bytes.Buffer{}
	srv, err := NewServerWithConfig(strings.NewReader(""), out, ServerConfig{
		WorkDir: t.TempDir(),
		Mode:    string(ModeFullAuto), // advertises write_file and run_shell_command
	})
	if err != nil {
		t.Fatalf("NewServerWithConfig: %v", err)
	}
	// Every optional surface, so nothing is advertised only in a configuration
	// this test does not build.
	srv.SetInfraRegistry(auditInfraRegistry())
	srv.SetSymbolIndex(&fakeSymbolIndex{})
	srv.forkManager = &SessionForkManager{}
	srv.operatorSession = true
	srv.approvalMgr = &approval.Manager{}
	if err := srv.RegisterProvider(fakeProvider{names: []string{"acme_module_tool"}}); err != nil {
		t.Fatalf("register module provider: %v", err)
	}

	srv.handleToolsList(Request{JSONRPC: "2.0", ID: json.RawMessage(`1`)})

	var resp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		t.Fatalf("decode tools/list: %v\nraw: %s", err, out.String())
	}
	if len(resp.Result.Tools) < 15 {
		t.Fatalf("only %d tools advertised; the optional surfaces did not switch on", len(resp.Result.Tools))
	}

	reserved := reservedCoreToolNames()
	for _, tool := range resp.Result.Tools {
		// Module tools are registered through RegisterProvider, which already
		// rejects reserved names; they are not core and must not be reserved.
		if tool.Name == "acme_module_tool" {
			continue
		}
		if !reserved[tool.Name] {
			t.Errorf("handleToolsList advertises %q but reservedCoreToolNames() omits it — a module could shadow it", tool.Name)
		}
	}
}

func TestRegisterProvider_AcceptsFreshNames(t *testing.T) {
	s := &Server{}
	if err := s.RegisterProvider(fakeProvider{names: []string{"acme_a", "acme_b"}}); err != nil {
		t.Fatalf("fresh provider should register: %v", err)
	}
	if len(s.providerTools) != 2 {
		t.Errorf("expected 2 provider tools, got %d", len(s.providerTools))
	}
}
