// Package project is the PUBLIC seam exposing a read-only view of project
// configuration to out-of-tree modules. It projects only the handful of fields
// a product layer needs, so the full internal ProjectConfig surface stays
// private and the public contract remains small and stable.
package project

import internalproject "github.com/openexec/openexec/internal/project"

// View is a read-only projection of the project-config fields modules consume.
type View struct {
	// BaseBranch is the repository's base branch (e.g. for opening PRs).
	BaseBranch string
	// Port is the execution engine port.
	Port int
	// PlannerModel is the model used for the planning phase.
	PlannerModel string
	// ExecutorModel is the model used for task execution.
	ExecutorModel string

	apiName, apiBaseURL, apiKey, apiModel string
}

// ActiveAPI returns the resolved active API provider credentials, mirroring the
// internal ExecutionConfig.ActiveAPI contract.
func (v *View) ActiveAPI() (name, baseURL, apiKey, model string) {
	return v.apiName, v.apiBaseURL, v.apiKey, v.apiModel
}

// Load reads the project config at projectDir and projects the module-visible
// view. It returns (nil, nil) when no config is present.
func Load(projectDir string) (*View, error) {
	cfg, err := internalproject.LoadProjectConfig(projectDir)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}
	name, baseURL, apiKey, model := cfg.Execution.ActiveAPI()
	return &View{
		BaseBranch:    cfg.BaseBranch,
		Port:          cfg.Execution.Port,
		PlannerModel:  cfg.Execution.PlannerModel,
		ExecutorModel: cfg.Execution.ExecutorModel,
		apiName:       name,
		apiBaseURL:    baseURL,
		apiKey:        apiKey,
		apiModel:      model,
	}, nil
}
