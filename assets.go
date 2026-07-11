package openexec

import (
	"embed"
	"io/fs"
)

// Agents holds the agent definitions
//
//go:embed all:agents
var agents embed.FS

// GetAgentsFS returns the sub-filesystem for the agent definitions
func GetAgentsFS() fs.FS {
	f, _ := fs.Sub(agents, "agents")
	return f
}
