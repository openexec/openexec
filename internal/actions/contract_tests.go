package actions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/openexec/openexec/internal/contracts"
	"github.com/openexec/openexec/internal/types"
)

// GenerateContractTestsAction renders Vitest feature-completeness
// contract tests from a blueprint YAML file into a target SvelteKit
// project. It is registered under the name "contracts.GenerateTests"
// and wired into the default blueprint between implement and lint.
type GenerateContractTestsAction struct {
	projectDir string
}

// NewGenerateContractTestsAction constructs a new action bound to the
// given project directory. The project directory is used as the
// default target directory if the stage inputs don't supply one.
func NewGenerateContractTestsAction(projectDir string) *GenerateContractTestsAction {
	return &GenerateContractTestsAction{projectDir: projectDir}
}

// Name implements types.Action.
func (a *GenerateContractTestsAction) Name() string {
	return "contracts.GenerateTests"
}

// Execute resolves contract_path and target_dir from the request
// inputs, loads the blueprint YAML, and renders contract test files.
// If the project does not have a contract (empty or missing file), it
// logs a skip and returns a completed status — this stage must never
// fail the build for projects that have nothing to contract.
func (a *GenerateContractTestsAction) Execute(ctx context.Context, req ActionRequest) (ActionResponse, error) {
	contractPath := stringInput(req.Inputs, "contract_path")
	targetDir := stringInput(req.Inputs, "target_dir")

	if targetDir == "" {
		if req.WorkspaceDir != "" {
			targetDir = req.WorkspaceDir
		} else {
			targetDir = a.projectDir
		}
	}

	if contractPath == "" {
		// Best-effort: look for a blueprint YAML next to the project.
		candidate := resolveDefaultContractPath(a.projectDir)
		if candidate != "" {
			contractPath = candidate
		}
	}

	if contractPath == "" {
		return ActionResponse{
			Status: types.StageStatusCompleted,
			Output: "contracts: skipped (no contract_path resolved)",
		}, nil
	}

	if _, err := os.Stat(contractPath); err != nil {
		return ActionResponse{
			Status: types.StageStatusCompleted,
			Output: fmt.Sprintf("contracts: skipped (%s not found)", contractPath),
		}, nil
	}

	cf, err := contracts.LoadFromYAML(contractPath)
	if err != nil {
		return ActionResponse{
			Status: types.StageStatusFailed,
			Error:  fmt.Sprintf("load contract yaml: %v", err),
		}, nil
	}

	if cf.IsEmpty() {
		return ActionResponse{
			Status: types.StageStatusCompleted,
			Output: "contracts: skipped (feature_completeness_contracts is empty)",
		}, nil
	}

	files, err := contracts.Generate(cf, targetDir)
	if err != nil {
		return ActionResponse{
			Status: types.StageStatusFailed,
			Error:  fmt.Sprintf("generate contract tests: %v", err),
		}, nil
	}

	return ActionResponse{
		Status: types.StageStatusCompleted,
		Output: fmt.Sprintf("contracts: generated %d test file(s) under %s", len(files), filepath.Join(targetDir, "tests", "contracts")),
		Artifacts: map[string]string{
			"contract_tests_generated": fmt.Sprintf("%d", len(files)),
		},
	}, nil
}

// stringInput safely reads a string key from a map[string]any.
func stringInput(inputs map[string]any, key string) string {
	if inputs == nil {
		return ""
	}
	if v, ok := inputs[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// resolveDefaultContractPath looks for the first blueprint YAML file
// inside the project's `.openexec/blueprints/` or `blueprints/` folder.
// It returns an empty string if none is found.
func resolveDefaultContractPath(projectDir string) string {
	candidates := []string{
		filepath.Join(projectDir, ".openexec", "blueprints", "active.yaml"),
		filepath.Join(projectDir, ".openexec", "blueprint.yaml"),
		filepath.Join(projectDir, "blueprints", "active.yaml"),
		filepath.Join(projectDir, "blueprint.yaml"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}
