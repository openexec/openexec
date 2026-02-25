package manifest

// Manifest holds a parsed agent manifest.
type Manifest struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Title       string        `yaml:"title"`
	Persona     string        `yaml:"persona"`
	Workflows   []WorkflowRef `yaml:"workflows"`
}

// WorkflowRef is a reference to a workflow with optional param values.
type WorkflowRef struct {
	ID     string            `yaml:"id"`
	Params map[string]string `yaml:"params,omitempty"`
}

// WorkflowParams returns the param values for the given workflow ID,
// or nil if the workflow is not listed or has no params.
func (m *Manifest) WorkflowParams(workflowID string) map[string]string {
	for _, w := range m.Workflows {
		if w.ID == workflowID {
			return w.Params
		}
	}
	return nil
}
