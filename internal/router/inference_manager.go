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
	modelPath  string
	binPath    string
	projectDir string // optional project dir for per-project model lookup
}

func NewInferenceManager(modelPath string) *InferenceManager {
	return &InferenceManager{
		modelPath: modelPath,
	}
}

// SetProjectDir sets the project directory for per-project model lookup.
func (m *InferenceManager) SetProjectDir(dir string) {
	m.projectDir = dir
}

// EnsureReady checks if the inference engine is available and attempts to locate it.
// If modelPath is empty or the default placeholder, it uses ModelManager for
// auto-discovery and download.
func (m *InferenceManager) EnsureReady() error {
	// 1. If modelPath is empty or the default placeholder, use ModelManager
	if m.modelPath == "" || m.modelPath == "/models/bitnet-2b.gguf" {
		mm := NewModelManager(DefaultModelName)
		if m.projectDir != "" {
			mm.SetProjectDir(filepath.Join(m.projectDir, ".openexec", "models"))
		}
		modelPath, err := mm.EnsureModel()
		if err != nil {
			return fmt.Errorf("model not available: %w", err)
		}
		m.modelPath = modelPath
	} else {
		// User explicitly set a path — use it directly
		if _, err := os.Stat(m.modelPath); os.IsNotExist(err) {
			return fmt.Errorf("local model not found at %s. Please run 'openexec setup models'", m.modelPath)
		}
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
