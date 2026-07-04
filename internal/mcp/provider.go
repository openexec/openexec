package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openexec/openexec/pkg/mcptool"
)

// reservedCoreToolNames returns every tool name core may advertise, across all
// modes. Derived from the same *ToolDef() constructors handleToolsList uses, so
// it cannot drift when a core tool is renamed; the infra names are a fixed,
// security-sensitive surface reserved unconditionally. A module tool may never
// take one of these names (see RegisterProvider).
func reservedCoreToolNames() map[string]bool {
	defs := []map[string]interface{}{
		axonSignalToolDef(), ReadFileToolDef(), GitApplyPatchToolDef(),
		OpenExecResultToolDef(), OpenExecActionToolDef(),
		BacklogListStoriesToolDef(), BacklogGetStoryToolDef(), BacklogClaimStoryToolDef(),
		BacklogCompleteTaskToolDef(), BacklogCompleteStoryToolDef(), BacklogAddTaskToolDef(),
		MemoryReadToolDef(), SkillProposeToolDef(),
		WriteFileToolDef(), RunShellCommandToolDef(),
		ApprovalListToolDef(), ApprovalDecideToolDef(),
		ForkSessionToolDef(), GetForkInfoToolDef(), ListSessionForksToolDef(),
	}
	names := make(map[string]bool, len(defs)+5)
	for _, d := range defs {
		if n, ok := d["name"].(string); ok && n != "" {
			names[n] = true
		}
	}
	for _, n := range []string{
		"ansible_run_playbook", "salt_apply_state", "ssh_run_query",
		"terraform_plan", "terraform_apply",
	} {
		names[n] = true
	}
	return names
}

// RegisterProvider adds a module's MCP tool provider to the server. Called by
// the composition root when a module is enabled, so the core server exposes the
// module's tools without importing the module.
//
// It fails LOUD on a name collision — with a reserved core tool name (which the
// provider would otherwise shadow at call time, since dispatchProvider runs
// before the core switch) or with a tool already registered by another module
// (which would silently clobber it). Registration is all-or-nothing: a
// conflicting provider adds none of its tools.
func (s *Server) RegisterProvider(p mcptool.Provider) error {
	if s.providerTools == nil {
		s.providerTools = map[string]mcptool.Tool{}
	}
	reserved := reservedCoreToolNames()
	incoming := p.Tools()
	for name := range incoming {
		if reserved[name] {
			return fmt.Errorf("mcp: provider tool %q collides with a reserved core tool name", name)
		}
		if _, dup := s.providerTools[name]; dup {
			return fmt.Errorf("mcp: provider tool %q is already registered by another module", name)
		}
	}
	for name, t := range incoming {
		s.providerTools[name] = t
	}
	return nil
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
