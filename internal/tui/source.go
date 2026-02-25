package tui

// Source provides an interface for fetching project data
type Source interface {
	// List returns all projects
	List() ([]ProjectInfo, error)
	// Status returns the current status of a project
	Status(name string) (ProjectInfo, error)
	// Subscribe returns a channel for receiving project updates
	Subscribe(name string) (<-chan ProjectInfo, func(), error)
}

// MockSource is a simple in-memory source for testing
type MockSource struct {
	projects map[string]ProjectInfo
}

// NewMockSource creates a new mock source
func NewMockSource() *MockSource {
	return &MockSource{
		projects: make(map[string]ProjectInfo),
	}
}

// List returns all projects
func (m *MockSource) List() ([]ProjectInfo, error) {
	projects := make([]ProjectInfo, 0, len(m.projects))
	for _, proj := range m.projects {
		projects = append(projects, proj)
	}
	return projects, nil
}

// Status returns the current status of a project
func (m *MockSource) Status(name string) (ProjectInfo, error) {
	if proj, ok := m.projects[name]; ok {
		return proj, nil
	}
	return ProjectInfo{}, nil
}

// Subscribe returns a channel for receiving project updates
func (m *MockSource) Subscribe(name string) (<-chan ProjectInfo, func(), error) {
	ch := make(chan ProjectInfo, 64)
	return ch, func() { close(ch) }, nil
}

// AddProject adds a project to the mock source
func (m *MockSource) AddProject(proj ProjectInfo) {
	m.projects[proj.Name] = proj
}
