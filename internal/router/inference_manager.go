package router

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// InferenceManager handles the lifecycle of the local 1-bit model
type InferenceManager struct {
	modelPath  string
	binPath    string
	projectDir string // optional project dir for per-project model lookup

	readyMu sync.Mutex
	ready   bool // Set once EnsureReady has successfully resolved model + binary
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
//
// Once the model and binary have been successfully resolved, the result is cached
// and subsequent calls return immediately. This avoids re-running filesystem
// probes (and any potential re-download path) on every ParseIntent invocation.
func (m *InferenceManager) EnsureReady() error {
	m.readyMu.Lock()
	defer m.readyMu.Unlock()

	// Fast path: already resolved and files still present.
	if m.ready && m.modelPath != "" && m.binPath != "" {
		if _, err := os.Stat(m.modelPath); err == nil {
			if _, err := os.Stat(m.binPath); err == nil {
				return nil
			}
		}
		// Something disappeared from disk — fall through and re-resolve.
		m.ready = false
	}

	// 1. Resolve the model file.
	//
	// Resolution order:
	//   - If the user gave an explicit path that exists on disk, use it as-is.
	//   - Otherwise (empty, placeholder, or explicit-but-missing), delegate
	//     to ModelManager which checks project-local first, then the
	//     user-level cache at ~/.openexec/models/, and downloads to the
	//     user-level cache on miss. This means enabling bitnet_routing in
	//     a fresh project will Just Work, and the model is shared across
	//     every openexec project on the machine.
	const placeholderPath = "/models/bitnet-2b.gguf"
	useModelManager := true
	if m.modelPath != "" && m.modelPath != placeholderPath {
		if _, err := os.Stat(m.modelPath); err == nil {
			useModelManager = false
			log.Printf("[InferenceManager] Using configured model: %s", m.modelPath)
		} else {
			log.Printf("[InferenceManager] Configured model %s not found on disk; falling back to user-level cache / auto-download", m.modelPath)
		}
	}
	if useModelManager {
		mm := NewModelManager(DefaultModelName)
		if m.projectDir != "" {
			mm.SetProjectDir(filepath.Join(m.projectDir, ".openexec", "models"))
		}
		modelPath, err := mm.ResolveModelPath()
		if err != nil {
			return fmt.Errorf("model not available: %w (run 'openexec setup' or enable internet access for auto-download at startup)", err)
		}
		log.Printf("[InferenceManager] Resolved model: %s", modelPath)
		m.modelPath = modelPath
	}

	// 2. Resolve the inference binary.
	// Search order mirrors the model resolution: project-local first, then
	// the user-level location at ~/.openexec/bin/, then system PATH. The
	// user-level location is shared across every openexec project on the
	// machine, so installing once is enough.
	userBin := filepath.Join(os.Getenv("HOME"), ".openexec", "bin", "bitnet-cli")
	possiblePaths := []string{
		"./bin/bitnet-cli",
		userBin,
	}

	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			m.binPath = p
			m.ready = true
			log.Printf("[InferenceManager] Using inference binary: %s", p)
			return nil
		}
	}

	// Fallback to system PATH
	if p, err := exec.LookPath("bitnet-cli"); err == nil {
		m.binPath = p
		m.ready = true
		log.Printf("[InferenceManager] Using inference binary from PATH: %s", p)
		return nil
	}

	return fmt.Errorf("inference engine (bitnet-cli) not found.\n  Install it once at the user level so every openexec project can share it:\n    %s\n  Or place it at ./bin/bitnet-cli inside the current project, or anywhere on $PATH", userBin)
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
