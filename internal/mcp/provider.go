package mcp

import (
	"context"
	"encoding/json"

	"github.com/openexec/openexec/internal/mcptool"
)

// RegisterProvider adds a module's MCP tool provider to the server. Called by
// the composition root when a module (e.g. governance) is enabled, so the core
// server exposes the module's tools without importing the module.
func (s *Server) RegisterProvider(p mcptool.Provider) {
	if s.providerTools == nil {
		s.providerTools = map[string]mcptool.Tool{}
	}
	for name, t := range p.Tools() {
		s.providerTools[name] = t
	}
}

// dispatchProvider runs a provider-contributed tool if one matches params.Name,
// reporting whether it handled the call.
func (s *Server) dispatchProvider(req Request, params toolsCallParams) bool {
	t, ok := s.providerTools[params.Name]
	if !ok {
		return false
	}
	t.Handle(&toolHost{s: s, id: req.ID}, params.Arguments)
	return true
}

// providerToolDefs returns the tools/list definitions of all registered
// provider tools.
func (s *Server) providerToolDefs() []interface{} {
	defs := make([]interface{}, 0, len(s.providerTools))
	for _, t := range s.providerTools {
		if t.Def != nil {
			defs = append(defs, t.Def)
		}
	}
	return defs
}

// toolHost binds the current request to the mcptool.Host interface, so a
// provider handler writes responses/reads context without seeing *Server.
type toolHost struct {
	s  *Server
	id json.RawMessage
}

func (h *toolHost) WriteResult(result interface{})  { h.s.writeResult(h.id, result) }
func (h *toolHost) WriteToolError(message string)   { h.s.writeToolError(h.id, message) }
func (h *toolHost) WriteError(code int, msg string) { h.s.writeError(h.id, code, msg) }
func (h *toolHost) Context() context.Context        { return h.s.toolCtx() }
func (h *toolHost) WorkspaceRoots() []string        { return h.s.workspaceRoots }

var _ mcptool.Host = (*toolHost)(nil)

// toolCtx returns the server's tracing context or a background context.
func (s *Server) toolCtx() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}
