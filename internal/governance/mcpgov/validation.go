package mcpgov

// ValidationError is a field-level validation failure for a tool's arguments.
// A local copy of the mcp type so this module does not import the core mcp
// package (modules never import each other).
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }
