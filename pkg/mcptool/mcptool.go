// Package mcptool is the CORE seam that lets a MODULE contribute MCP tools
// without the core MCP server importing the module. A module
// implements Provider; the composition root registers it into the server when
// the module is enabled. When the core ships alone, the module's tool code is
// simply absent — which is what makes a module separable and
// sellable. See docs/OPENEXEC_CORE_MODULE_STRATEGY.md.
package mcptool

import (
	"context"
	"encoding/json"
)

// Host is what a tool handler needs from the MCP server: response writing bound
// to the current request, the request context, and the workspace roots. The
// core server implements it per request; a provider never sees the server type.
type Host interface {
	// WriteResult writes a successful tools/call result for the current request.
	WriteResult(result interface{})
	// WriteToolError writes a tools/call result flagged isError (a tool-level
	// failure the model can read), for the current request.
	WriteToolError(message string)
	// WriteError writes a JSON-RPC protocol error for the current request.
	WriteError(code int, message string)
	// Context is the request/tracing context (never nil).
	Context() context.Context
	// WorkspaceRoots are the resolved project roots for this session.
	WorkspaceRoots() []string
}

// Tool is one MCP tool: its tools/list definition and a handler that reads the
// raw call arguments and writes its outcome through the Host.
type Tool struct {
	// Def is the JSON tools/list definition (name, description, inputSchema).
	Def map[string]interface{}
	// Handle runs the tool. args is the raw `arguments` object from the call.
	Handle func(h Host, args json.RawMessage)
}

// Provider is a module's MCP adapter: a named set of tools. The server merges
// every registered provider's tools into tools/list and dispatch.
type Provider interface {
	// Tools returns this provider's tools keyed by tool name.
	Tools() map[string]Tool
}
