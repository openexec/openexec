package loop

import (
	"encoding/json"
	"os"
)

// MCPConfig represents a Claude Code MCP server configuration file.
type MCPConfig struct {
	MCPServers map[string]MCPServerEntry `json:"mcpServers"`
}

// MCPServerEntry is a single MCP server entry in the config.
type MCPServerEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// WriteMCPConfig writes an MCP config file with the given server entries.
// Returns the temp file path. Caller is responsible for cleanup (os.Remove).
func WriteMCPConfig(servers map[string]MCPServerEntry) (string, error) {
	cfg := MCPConfig{MCPServers: servers}

	data, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}

	f, err := os.CreateTemp("", "openexec-mcp-*.json")
	if err != nil {
		return "", err
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", err
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}

	return f.Name(), nil
}

// BuildMCPServers constructs the standard MCP server entries.
// Always includes openexec-signal. Includes tract if tractStore is non-empty.
func BuildMCPServers(openexecBinary string, tractStore string) map[string]MCPServerEntry {
	servers := map[string]MCPServerEntry{
		"openexec-signal": {Command: openexecBinary, Args: []string{"mcp-serve"}},
	}
	if tractStore != "" {
		servers["tract"] = MCPServerEntry{Command: "tract", Args: []string{"serve", "--store", tractStore}}
	}
	return servers
}
