package router

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const (
	// DefaultModelName is the default BitNet model identifier.
	DefaultModelName = "bitnet-2b"

	// DefaultDownloadURL is the default URL for downloading the BitNet model.
	DefaultDownloadURL = "https://huggingface.co/openexec/bitnet-2b-gguf/resolve/main/bitnet-2b.Q4_K_M.gguf"

	// DefaultModelFilename is the GGUF filename stored on disk.
	DefaultModelFilename = "bitnet-2b.Q4_K_M.gguf"

	// globalModelsSubdir is the subdirectory under ~/.openexec/ for models.
	globalModelsSubdir = "models"

	// projectModelsSubdir is the subdirectory under .openexec/ for per-project models.
	projectModelsSubdir = ".openexec/models"
)

// ModelManager handles GGUF model file discovery and auto-download.
type ModelManager struct {
	modelName    string // e.g. "bitnet-2b"
	downloadURL  string // URL to download from
	expectedHash string // SHA-256 for verification (empty = skip)
	globalDir    string // ~/.openexec/models/
	projectDir   string // .openexec/models/ (optional)
}

// NewModelManager creates a ModelManager with default settings for the given model name.
func NewModelManager(modelName string) *ModelManager {
	homeDir, _ := os.UserHomeDir()
	globalDir := ""
	if homeDir != "" {
		globalDir = filepath.Join(homeDir, ".openexec", globalModelsSubdir)
	}

	return &ModelManager{
		modelName:   modelName,
		downloadURL: DefaultDownloadURL,
		globalDir:   globalDir,
	}
}

// SetProjectDir sets the project-specific model directory.
func (m *ModelManager) SetProjectDir(dir string) {
	m.projectDir = dir
}

// SetDownloadURL allows overriding the model download URL.
func (m *ModelManager) SetDownloadURL(url string) {
	m.downloadURL = url
}

// SetExpectedHash sets the expected SHA-256 hash for download verification.
func (m *ModelManager) SetExpectedHash(hash string) {
	m.expectedHash = hash
}

// SetGlobalDir overrides the global model directory (primarily for testing).
func (m *ModelManager) SetGlobalDir(dir string) {
	m.globalDir = dir
}

// modelFilename returns the expected filename for the model on disk.
func (m *ModelManager) modelFilename() string {
	return DefaultModelFilename
}

// ResolveModelPath looks up the model file in: project dir -> global dir.
// Returns the absolute path if found, or an error if not found anywhere.
func (m *ModelManager) ResolveModelPath() (string, error) {
	filename := m.modelFilename()

	// 1. Check project-local directory first (higher precedence)
	if m.projectDir != "" {
		projectPath := filepath.Join(m.projectDir, filename)
		if info, err := os.Stat(projectPath); err == nil && !info.IsDir() {
			return projectPath, nil
		}
	}

	// 2. Check global directory
	if m.globalDir != "" {
		globalPath := filepath.Join(m.globalDir, filename)
		if info, err := os.Stat(globalPath); err == nil && !info.IsDir() {
			return globalPath, nil
		}
	}

	return "", fmt.Errorf("model %q not found in any search path (project: %q, global: %q)",
		m.modelName, m.projectDir, m.globalDir)
}

// EnsureModel returns the path to the model, downloading it if necessary.
func (m *ModelManager) EnsureModel() (string, error) {
	// Try to find it locally first
	if path, err := m.ResolveModelPath(); err == nil {
		return path, nil
	}

	// Not found — download to global directory
	return m.Download()
}

// Download fetches the model to the global directory with progress output.
// It writes to a temporary file first, then renames on success.
// Returns the final model path.
func (m *ModelManager) Download() (string, error) {
	if m.globalDir == "" {
		return "", fmt.Errorf("cannot download model: global directory not configured (HOME not set?)")
	}

	if m.downloadURL == "" {
		return "", fmt.Errorf("cannot download model: no download URL configured")
	}

	// Create target directory
	if err := os.MkdirAll(m.globalDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create model directory %s: %w", m.globalDir, err)
	}

	filename := m.modelFilename()
	finalPath := filepath.Join(m.globalDir, filename)
	tmpPath := finalPath + ".tmp"

	// Clean up partial download on failure
	defer func() {
		// Only clean up if the tmp file still exists (wasn't renamed)
		if _, err := os.Stat(tmpPath); err == nil {
			os.Remove(tmpPath)
		}
	}()

	fmt.Fprintf(os.Stderr, "[ModelManager] Downloading %s from %s\n", m.modelName, m.downloadURL)

	resp, err := http.Get(m.downloadURL) //nolint:gosec // URL is user-configurable
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: HTTP %d %s", resp.StatusCode, resp.Status)
	}

	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	// Set up progress tracking and optional checksum
	hasher := sha256.New()
	var written int64
	totalSize := resp.ContentLength

	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := tmpFile.Write(buf[:n]); err != nil {
				tmpFile.Close()
				return "", fmt.Errorf("write failed: %w", err)
			}
			if m.expectedHash != "" {
				hasher.Write(buf[:n])
			}
			written += int64(n)

			// Progress output to stderr
			if totalSize > 0 {
				pct := float64(written) / float64(totalSize) * 100
				fmt.Fprintf(os.Stderr, "\r[ModelManager] Progress: %d / %d bytes (%.1f%%)", written, totalSize, pct)
			} else {
				fmt.Fprintf(os.Stderr, "\r[ModelManager] Progress: %d bytes downloaded", written)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			tmpFile.Close()
			return "", fmt.Errorf("download interrupted: %w", readErr)
		}
	}

	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\n[ModelManager] Download complete: %d bytes\n", written)

	// Verify checksum if expected
	if m.expectedHash != "" {
		actualHash := hex.EncodeToString(hasher.Sum(nil))
		if actualHash != m.expectedHash {
			return "", fmt.Errorf("checksum mismatch: expected %s, got %s", m.expectedHash, actualHash)
		}
		fmt.Fprintf(os.Stderr, "[ModelManager] Checksum verified: %s\n", actualHash)
	}

	// Atomic rename from tmp to final
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return "", fmt.Errorf("failed to finalize model file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "[ModelManager] Model ready at %s\n", finalPath)
	return finalPath, nil
}
