package workflow

// Template holds a parsed workflow template.
type Template struct {
	ID           string
	Params       map[string]string // declared param name → description (nil if no params)
	Instructions string            // raw template text (may contain {{param}})
	Process      string            // raw template text (may contain {{param}})
}
