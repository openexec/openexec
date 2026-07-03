package mcp

// SetMemoryLoader injects the merged-memory loader for the memory_read tool, so
// the MCP server does not import internal/memory (the composition root wires it).
func (s *Server) SetMemoryLoader(fn func(workspaceRoot string) (string, error)) {
	s.memoryLoader = fn
}
