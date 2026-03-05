package router

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// InferenceManager handles the lifecycle of the local 1-bit model
type InferenceManager struct {
	modelPath string
	binPath   string
}

func NewInferenceManager(modelPath string) *InferenceManager {
	return &InferenceManager{
		modelPath: modelPath,
	}
}

// EnsureReady checks if the inference engine is available and attempts to locate it
func (m *InferenceManager) EnsureReady() error {
	// 1. Check if model exists
	if _, err := os.Stat(m.modelPath); os.IsNotExist(err) {
		return fmt.Errorf("local model not found at %s. Please run 'openexec setup models'", m.modelPath)
	}

	// 2. Try to find a local inference engine (embedded-first approach)
	// We check for local bin, then PATH
	possiblePaths := []string{
		"./bin/bitnet-cli",
		filepath.Join(os.Getenv("HOME"), ".openexec", "bin", "bitnet-cli"),
	}

	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			m.binPath = p
			return nil
		}
	}

	// Fallback to system PATH
	if p, err := exec.LookPath("bitnet-cli"); err == nil {
		m.binPath = p
		return nil
	}

	return fmt.Errorf("no inference engine (bitnet-cli) found. Please install the OpenExec local brain pack")
}

// RunInference executes the local model with the given prompt
func (m *InferenceManager) RunInference(ctx context.Context, prompt string) (string, error) {
	if m.binPath == "" {
		if err := m.EnsureReady(); err != nil {
			return "", err
		}
	}

	// #nosec G204 - binPath is controlled by EnsureReady
	cmd := exec.CommandContext(ctx, m.binPath, "--model", m.modelPath, "--prompt", prompt, "--n-predict", "128")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inference failed: %w\nOutput: %s", err, string(out))
	}

	return string(out), nil
}
