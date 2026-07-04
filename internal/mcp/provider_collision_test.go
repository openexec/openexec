package mcp

import (
	"encoding/json"
	"testing"

	"github.com/openexec/openexec/internal/mcptool"
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

func TestRegisterProvider_AcceptsFreshNames(t *testing.T) {
	s := &Server{}
	if err := s.RegisterProvider(fakeProvider{names: []string{"acme_a", "acme_b"}}); err != nil {
		t.Fatalf("fresh provider should register: %v", err)
	}
	if len(s.providerTools) != 2 {
		t.Errorf("expected 2 provider tools, got %d", len(s.providerTools))
	}
}
