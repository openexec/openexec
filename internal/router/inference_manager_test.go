package router

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestInferenceManager_EnsureReady implements test seeds from T-US-001-001 section 4.2
// These tests verify the documented environment dependency behaviors.

func TestInferenceManager_ModelExists(t *testing.T) {
	// Scenario: Model exists
	// Given: Model file at configured path
	// When: EnsureReady() called
	// Then: Returns nil (model found), binPath set OR error for missing binary

	env := newTestEnv(t)
	defer env.done()

	modelPath := env.createFakeModel("model.gguf")
	env.createFakeBinary(true)
	env.chdir()

	m := NewInferenceManager(modelPath)
	err := m.EnsureReady()

	// Assert: Model check passes (we may still fail on binary if not found)
	if err != nil {
		if strings.Contains(err.Error(), "model not found") {
			t.Errorf("model should be found: %v", err)
		}
		if !strings.Contains(err.Error(), "bitnet-cli") {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

func TestInferenceManager_ModelMissing(t *testing.T) {
	// Scenario: Model missing
	// Given: No model file
	// When: EnsureReady() called
	// Then: Returns error with setup instructions

	m := NewInferenceManager("/tmp/non-existent-model-abc123.gguf")
	err := m.EnsureReady()

	if err == nil {
		t.Fatal("expected error for missing model")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
	if !strings.Contains(err.Error(), "openexec setup models") {
		t.Errorf("error should include setup instructions, got: %v", err)
	}
}

func TestInferenceManager_BinaryInLocal(t *testing.T) {
	// Scenario: Binary in local
	// Given: ./bin/bitnet-cli exists
	// When: EnsureReady() called
	// Then: Uses local binary

	env := newTestEnv(t)
	defer env.done()

	modelPath := env.createFakeModel("model.gguf")
	env.createFakeBinary(true)
	env.chdir()

	m := NewInferenceManager(modelPath)
	err := m.EnsureReady()

	if err != nil {
		t.Fatalf("EnsureReady should succeed with local binary: %v", err)
	}
	if m.binPath != "./bin/bitnet-cli" {
		t.Errorf("expected binPath to be './bin/bitnet-cli', got %q", m.binPath)
	}
}

func TestInferenceManager_BinaryInHome(t *testing.T) {
	// Scenario: Binary in home
	// Given: ~/.openexec/bin/bitnet-cli exists
	// When: EnsureReady() called
	// Then: Uses home binary

	env := newTestEnv(t)
	defer env.done()

	modelPath := env.createFakeModel("model.gguf")
	fakeHome := env.createFakeHome()
	env.setHome(fakeHome)

	// Make sure ./bin/bitnet-cli does NOT exist in cwd
	noLocalBinDir := filepath.Join(env.tmpDir, "workdir")
	env.chdirTo(noLocalBinDir)

	m := NewInferenceManager(modelPath)
	err := m.EnsureReady()

	if err != nil {
		t.Fatalf("EnsureReady should succeed with home binary: %v", err)
	}
	expectedPath := filepath.Join(fakeHome, ".openexec", "bin", "bitnet-cli")
	if m.binPath != expectedPath {
		t.Errorf("expected binPath to be %q, got %q", expectedPath, m.binPath)
	}
}

func TestInferenceManager_BinaryInPATH(t *testing.T) {
	// Scenario: Binary in PATH
	// Given: bitnet-cli in system PATH
	// When: EnsureReady() called
	// Then: Uses system binary

	// Skip if bitnet-cli is not actually installed
	if _, err := exec.LookPath("bitnet-cli"); err != nil {
		t.Skip("bitnet-cli not in PATH, skipping system PATH test")
	}

	env := newTestEnv(t)
	defer env.done()

	modelPath := env.createFakeModel("model.gguf")
	env.chdir()
	env.setHome("/nonexistent")

	m := NewInferenceManager(modelPath)
	err := m.EnsureReady()

	if err != nil {
		t.Fatalf("EnsureReady should succeed with PATH binary: %v", err)
	}
	if m.binPath == "" {
		t.Error("binPath should be set")
	}
}

func TestInferenceManager_NoBinary(t *testing.T) {
	// Scenario: No binary
	// Given: No binary anywhere
	// When: EnsureReady() called
	// Then: Returns error with install instructions

	env := newTestEnv(t)
	defer env.done()

	modelPath := env.createFakeModel("model.gguf")
	env.chdir()
	env.setHome(env.tmpDir) // No .openexec/bin here
	env.setPath("/nonexistent")

	m := NewInferenceManager(modelPath)
	err := m.EnsureReady()

	if err == nil {
		t.Fatal("expected error when no binary found")
	}
	if !strings.Contains(err.Error(), "bitnet-cli") {
		t.Errorf("error should mention bitnet-cli, got: %v", err)
	}
	if !strings.Contains(err.Error(), "OpenExec local brain pack") {
		t.Errorf("error should include install instructions, got: %v", err)
	}
}

// Edge case tests from T-US-001-001 section 4.3

func TestInferenceManager_HomeNotSet(t *testing.T) {
	// Edge case: HOME not set
	// Expected: os.Getenv returns empty string, path construction proceeds

	env := newTestEnv(t)
	defer env.done()

	modelPath := env.createFakeModel("model.gguf")
	env.chdir()
	env.unsetHome()
	env.setPath("/nonexistent")

	m := NewInferenceManager(modelPath)
	err := m.EnsureReady()

	// Assert: Should fail due to missing binary, NOT panic due to empty HOME
	if err == nil {
		t.Fatal("expected error when no binary found")
	}
	if !strings.Contains(err.Error(), "bitnet-cli") {
		t.Errorf("error should mention bitnet-cli, got: %v", err)
	}
}

func TestInferenceManager_ModelPathIsDirectory(t *testing.T) {
	// Edge case: Model path is directory
	// Expected: os.Stat succeeds but inference will fail

	env := newTestEnv(t)
	defer env.done()

	modelPath := env.createModelDir("model-dir")
	env.createFakeBinary(true)
	env.chdir()

	m := NewInferenceManager(modelPath)
	err := m.EnsureReady()

	// Assert: EnsureReady succeeds (os.Stat works on directories)
	if err != nil {
		t.Fatalf("EnsureReady should succeed even for directory: %v", err)
	}
	// Note: RunInference would fail, but that's not tested here
}

func TestInferenceManager_BinaryNotExecutable(t *testing.T) {
	// Edge case: Binary not executable
	// Expected: EnsureReady succeeds, RunInference fails

	env := newTestEnv(t)
	defer env.done()

	modelPath := env.createFakeModel("model.gguf")
	env.createFakeBinary(false) // Not executable
	env.chdir()

	m := NewInferenceManager(modelPath)

	// Act: EnsureReady
	err := m.EnsureReady()

	// Assert: EnsureReady succeeds (os.Stat doesn't check executability)
	if err != nil {
		t.Fatalf("EnsureReady should succeed even for non-executable binary: %v", err)
	}

	// Act: RunInference should fail
	ctx := context.Background()
	_, err = m.RunInference(ctx, "test prompt")

	// Assert: RunInference fails
	if err == nil {
		t.Error("RunInference should fail for non-executable binary")
	}
}

// TestNewInferenceManager verifies constructor behavior
func TestNewInferenceManager(t *testing.T) {
	modelPath := "/test/model.gguf"
	m := NewInferenceManager(modelPath)

	if m.modelPath != modelPath {
		t.Errorf("expected modelPath %q, got %q", modelPath, m.modelPath)
	}
	if m.binPath != "" {
		t.Error("binPath should be empty before EnsureReady")
	}
}
