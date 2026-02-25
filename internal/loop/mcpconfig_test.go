package loop

import (
	"encoding/json"
	"os"
	"testing"
)

func TestWriteMCPConfig(t *testing.T) {
	servers := BuildMCPServers("/usr/local/bin/axon", "")
	path, err := WriteMCPConfig(servers)
	if err != nil {
		t.Fatalf("WriteMCPConfig: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	var cfg MCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	entry, ok := cfg.MCPServers["axon-signal"]
	if !ok {
		t.Fatal("missing 'axon-signal' server entry")
	}
	if entry.Command != "/usr/local/bin/axon" {
		t.Errorf("command = %q", entry.Command)
	}
	if len(entry.Args) != 1 || entry.Args[0] != "mcp-serve" {
		t.Errorf("args = %v", entry.Args)
	}
}

func TestBuildMCPServersWithTract(t *testing.T) {
	servers := BuildMCPServers("/usr/local/bin/axon", "/data/tracts")

	if _, ok := servers["axon-signal"]; !ok {
		t.Fatal("missing 'axon-signal' entry")
	}

	tract, ok := servers["tract"]
	if !ok {
		t.Fatal("missing 'tract' entry")
	}
	if tract.Command != "tract" {
		t.Errorf("tract command = %q, want %q", tract.Command, "tract")
	}
	expected := []string{"serve", "--store", "/data/tracts"}
	if len(tract.Args) != len(expected) {
		t.Fatalf("tract args = %v, want %v", tract.Args, expected)
	}
	for i, a := range tract.Args {
		if a != expected[i] {
			t.Errorf("tract args[%d] = %q, want %q", i, a, expected[i])
		}
	}
}

func TestBuildMCPServersWithoutTract(t *testing.T) {
	servers := BuildMCPServers("/usr/local/bin/axon", "")

	if _, ok := servers["axon-signal"]; !ok {
		t.Fatal("missing 'axon-signal' entry")
	}
	if _, ok := servers["tract"]; ok {
		t.Error("'tract' entry should not be present when tractStore is empty")
	}
	if len(servers) != 1 {
		t.Errorf("expected 1 server entry, got %d", len(servers))
	}
}

func TestWriteMCPConfigMultipleServers(t *testing.T) {
	servers := BuildMCPServers("/usr/local/bin/axon", "/data/tracts")
	path, err := WriteMCPConfig(servers)
	if err != nil {
		t.Fatalf("WriteMCPConfig: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	var cfg MCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, ok := cfg.MCPServers["axon-signal"]; !ok {
		t.Fatal("missing 'axon-signal' in written config")
	}
	if _, ok := cfg.MCPServers["tract"]; !ok {
		t.Fatal("missing 'tract' in written config")
	}
	if len(cfg.MCPServers) != 2 {
		t.Errorf("expected 2 server entries in config, got %d", len(cfg.MCPServers))
	}
}

func TestBuildCommandWithMCPConfig(t *testing.T) {
	cfg := Config{
		Prompt:        "test prompt",
		MCPConfigPath: "/tmp/mcp.json",
	}
	name, args := buildCommand(cfg)

	if name != "claude" {
		t.Errorf("name = %q", name)
	}

	// Check --mcp-config is present.
	found := false
	for i, a := range args {
		if a == "--mcp-config" && i+1 < len(args) && args[i+1] == "/tmp/mcp.json" {
			found = true
		}
	}
	if !found {
		t.Errorf("args %v missing --mcp-config /tmp/mcp.json", args)
	}
}

func TestBuildCommandWithoutMCPConfig(t *testing.T) {
	cfg := Config{Prompt: "test prompt"}
	_, args := buildCommand(cfg)

	for _, a := range args {
		if a == "--mcp-config" {
			t.Error("--mcp-config should not be present when MCPConfigPath is empty")
		}
	}
}
