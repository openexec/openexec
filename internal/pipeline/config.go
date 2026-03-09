package pipeline

import (
	"fmt"
	"io/fs"

	"gopkg.in/yaml.v3"
)

// PipelineDef describes a pipeline configuration loaded from YAML.
type PipelineDef struct {
	Phases []PhaseDef `yaml:"phases"`
}

// PhaseDef describes a single phase in a pipeline configuration.
type PhaseDef struct {
	ID            string            `yaml:"id"`
	Agent         string            `yaml:"agent"`
	Workflow      string            `yaml:"workflow"`
	MaxIterations int               `yaml:"max_iterations,omitempty"`
	Routes        map[string]string `yaml:"routes,omitempty"`
}

// LoadPipelineConfig loads a named pipeline configuration from the agents filesystem.
// The file is read from pipelines/{name}.yaml.
func LoadPipelineConfig(f fs.FS, name string) (*PipelineDef, error) {
	path := "pipelines/" + name + ".yaml"
	data, err := fs.ReadFile(f, path)
	if err != nil {
		return nil, fmt.Errorf("load pipeline %q: %w", name, err)
	}

	var def PipelineDef
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse pipeline %q: %w", name, err)
	}

	if err := def.Validate(); err != nil {
		return nil, fmt.Errorf("validate pipeline %q: %w", name, err)
	}

	return &def, nil
}

// DefaultPipelineDef returns the standard 5-phase pipeline definition.
func DefaultPipelineDef() *PipelineDef {
	return &PipelineDef{
		Phases: []PhaseDef{
			{ID: "TD", Agent: "clario", Workflow: "technical-design"},
			{ID: "IM", Agent: "spark", Workflow: "implement"},
			{ID: "RV", Agent: "blade", Workflow: "review", Routes: map[string]string{"spark": "IM", "hon": "RF"}},
			{ID: "RF", Agent: "hon", Workflow: "refactor"},
			{ID: "FL", Agent: "clario", Workflow: "feedback-loop"},
		},
	}
}

// Validate checks the pipeline definition for errors.
func (d *PipelineDef) Validate() error {
	if len(d.Phases) == 0 {
		return fmt.Errorf("pipeline has no phases")
	}

	seen := make(map[string]bool, len(d.Phases))
	for _, p := range d.Phases {
		if p.ID == "" {
			return fmt.Errorf("phase missing ID")
		}
		if seen[p.ID] {
			return fmt.Errorf("duplicate phase ID %q", p.ID)
		}
		seen[p.ID] = true

		if p.Agent == "" {
			return fmt.Errorf("phase %q missing agent", p.ID)
		}
		if p.Workflow == "" {
			return fmt.Errorf("phase %q missing workflow", p.ID)
		}
	}

	for _, p := range d.Phases {
		for target, destID := range p.Routes {
			if !seen[destID] {
				return fmt.Errorf("phase %q route %q references unknown phase %q", p.ID, target, destID)
			}
		}
	}

	return nil
}

// PhaseOrder returns the ordered list of phase IDs.
func (d *PipelineDef) PhaseOrder() []Phase {
	order := make([]Phase, len(d.Phases))
	for i, p := range d.Phases {
		order[i] = Phase(p.ID)
	}
	return order
}

// PhaseConfigs converts the pipeline definition to a map of PhaseConfigs.
func (d *PipelineDef) PhaseConfigs() map[Phase]PhaseConfig {
	configs := make(map[Phase]PhaseConfig, len(d.Phases))
	for _, p := range d.Phases {
		cfg := PhaseConfig{
			Agent:         p.Agent,
			Workflow:      p.Workflow,
			MaxIterations: p.MaxIterations,
		}
		if len(p.Routes) > 0 {
			routes := make(map[string]Phase, len(p.Routes))
			for target, destID := range p.Routes {
				routes[target] = Phase(destID)
			}
			cfg.Routes = routes
		}
		configs[Phase(p.ID)] = cfg
	}
	return configs
}
