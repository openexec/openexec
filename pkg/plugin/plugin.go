// Package plugin is the public extension point for OpenExec modules. A module
// living in its own (e.g. proprietary) repository imports the open-source core
// as a dependency and registers itself here — typically from an init() — so the
// core composition root exposes the module's MCP tools and CLI commands WITHOUT
// core importing the module. Registration order:
//
//	// proprietary cmd/openexec/main.go
//	import (
//	    _ "example.com/openexec-governance/register" // its init() calls plugin.Register*
//	    "github.com/openexec/openexec/pkg/openexec"
//	)
//	func main() { openexec.Main() }
//
// init() runs before main(), so the registries are populated before the CLI
// reads them.
package plugin

import (
	"github.com/openexec/openexec/pkg/mcptool"
	"github.com/spf13/cobra"
)

// MCPProvider is a module's MCP tool provider registration. Name is the module
// name used for config gating (project `modules:` block / ShouldLoad); New
// builds the provider when the MCP server starts.
type MCPProvider struct {
	Name string
	New  func() mcptool.Provider
}

var (
	mcpProviders []MCPProvider
	commands     []*cobra.Command
)

// RegisterMCPProvider registers a module's MCP tool provider under a module name
// (used for config gating). Call from an init().
func RegisterMCPProvider(name string, new func() mcptool.Provider) {
	mcpProviders = append(mcpProviders, MCPProvider{Name: name, New: new})
}

// MCPProviders returns the registered MCP providers. Composition-root use.
func MCPProviders() []MCPProvider { return mcpProviders }

// RegisterCommand registers a top-level CLI command contributed by a module
// (e.g. `openexec governance ...`). Call from an init().
func RegisterCommand(cmd *cobra.Command) {
	commands = append(commands, cmd)
}

// Commands returns the registered module CLI commands. Composition-root use.
func Commands() []*cobra.Command { return commands }
