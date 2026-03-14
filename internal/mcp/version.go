// Package mcp provides MCP (Model Context Protocol) server functionality.
package mcp

// ToolRegistryVersion is the semantic version of the MCP tool definitions.
// Increment this when tool schemas, behaviors, or the tool set changes.
// Format: MAJOR.MINOR.PATCH
//   - MAJOR: Breaking changes to tool schemas or removal of tools
//   - MINOR: New tools or non-breaking additions to existing tools
//   - PATCH: Bug fixes, documentation improvements
const ToolRegistryVersion = "1.0.0"
