package router

import (
	"context"
	"os"
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

	// Arrange
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.gguf")
	if err := os.WriteFile(modelPath, []byte("fake model"), 0644); err != nil {
		t.Fatalf("failed to create fake model: %v", err)
	}

	// Also create a fake binary
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	binPath := filepath.Join(binDir, "bitnet-cli")
	if err := os.WriteFile(binPath, []byte("#!/bin/bash\necho test"), 0755); err != nil {
		t.Fatalf("failed to create fake binary: %v", err)
	}

	// Create manager with our model path
	m := NewInferenceManager(modelPath)
	// Override the binary search by placing it in the first search location
	// We'll save and restore the working directory
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Act
	err := m.EnsureReady()

	// Assert: Model check passes (we may still fail on binary if not found)
	// Since we created ./bin/bitnet-cli relative to tmpDir, it should find it
	if err != nil {
		// If error is about model not found, that's a failure
		if strings.Contains(err.Error(), "model not found") {
			t.Errorf("model should be found: %v", err)
		}
		// Binary not found is expected if our setup didn't work
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

	// Arrange
	m := NewInferenceManager("/tmp/non-existent-model-abc123.gguf")

	// Act
	err := m.EnsureReady()

	// Assert
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

	// Arrange
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.gguf")
	if err := os.WriteFile(modelPath, []byte("fake model"), 0644); err != nil {
		t.Fatalf("failed to create fake model: %v", err)
	}

	// Create ./bin/bitnet-cli
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	binPath := filepath.Join(binDir, "bitnet-cli")
	if err := os.WriteFile(binPath, []byte("#!/bin/bash\necho test"), 0755); err != nil {
		t.Fatalf("failed to create fake binary: %v", err)
	}

	// Change to tmpDir so ./bin/bitnet-cli is found
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origDir)

	m := NewInferenceManager(modelPath)

	// Act
	err := m.EnsureReady()

	// Assert
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

	// Arrange
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.gguf")
	if err := os.WriteFile(modelPath, []byte("fake model"), 0644); err != nil {
		t.Fatalf("failed to create fake model: %v", err)
	}

	// Create fake HOME structure
	fakeHome := filepath.Join(tmpDir, "fakehome")
	binDir := filepath.Join(fakeHome, ".openexec", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	binPath := filepath.Join(binDir, "bitnet-cli")
	if err := os.WriteFile(binPath, []byte("#!/bin/bash\necho test"), 0755); err != nil {
		t.Fatalf("failed to create fake binary: %v", err)
	}

	// Set HOME to our fake home
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", fakeHome)
	defer os.Setenv("HOME", origHome)

	// Make sure ./bin/bitnet-cli does NOT exist in cwd
	origDir, _ := os.Getwd()
	noLocalBinDir := filepath.Join(tmpDir, "workdir")
	os.MkdirAll(noLocalBinDir, 0755)
	os.Chdir(noLocalBinDir)
	defer os.Chdir(origDir)

	m := NewInferenceManager(modelPath)

	// Act
	err := m.EnsureReady()

	// Assert
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

	// This test is harder to set up reliably without actually installing a binary
	// We can only verify the fallback mechanism works by checking LookPath would be called

	// Skip if bitnet-cli is not actually installed
	if _, err := exec.LookPath("bitnet-cli"); err != nil {
		t.Skip("bitnet-cli not in PATH, skipping system PATH test")
	}

	// Arrange
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.gguf")
	if err := os.WriteFile(modelPath, []byte("fake model"), 0644); err != nil {
		t.Fatalf("failed to create fake model: %v", err)
	}

	// Ensure no local binary exists
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Clear HOME to prevent ~/.openexec/bin lookup
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", "/nonexistent")
	defer os.Setenv("HOME", origHome)

	m := NewInferenceManager(modelPath)

	// Act
	err := m.EnsureReady()

	// Assert
	if err != nil {
		t.Fatalf("EnsureReady should succeed with PATH binary: %v", err)
	}
	// binPath should be the result of exec.LookPath
	if m.binPath == "" {
		t.Error("binPath should be set")
	}
}

func TestInferenceManager_NoBinary(t *testing.T) {
	// Scenario: No binary
	// Given: No binary anywhere
	// When: EnsureReady() called
	// Then: Returns error with install instructions

	// Arrange
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.gguf")
	if err := os.WriteFile(modelPath, []byte("fake model"), 0644); err != nil {
		t.Fatalf("failed to create fake model: %v", err)
	}

	// Ensure no local binary exists
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Set HOME to a path without the binary
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir) // No .openexec/bin here
	defer os.Setenv("HOME", origHome)

	// Temporarily modify PATH to exclude bitnet-cli
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "/nonexistent")
	defer os.Setenv("PATH", origPath)

	m := NewInferenceManager(modelPath)

	// Act
	err := m.EnsureReady()

	// Assert
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

	// Arrange
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.gguf")
	if err := os.WriteFile(modelPath, []byte("fake model"), 0644); err != nil {
		t.Fatalf("failed to create fake model: %v", err)
	}

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Unset HOME
	origHome := os.Getenv("HOME")
	os.Unsetenv("HOME")
	defer os.Setenv("HOME", origHome)

	// Also clear PATH to ensure binary not found
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "/nonexistent")
	defer os.Setenv("PATH", origPath)

	m := NewInferenceManager(modelPath)

	// Act
	err := m.EnsureReady()

	// Assert: Should fail due to missing binary, NOT panic due to empty HOME
	if err == nil {
		t.Fatal("expected error when no binary found")
	}
	// The path construction should have proceeded without panic
	// We verify by checking we get the expected "no binary" error
	if !strings.Contains(err.Error(), "bitnet-cli") {
		t.Errorf("error should mention bitnet-cli, got: %v", err)
	}
}

func TestInferenceManager_ModelPathIsDirectory(t *testing.T) {
	// Edge case: Model path is directory
	// Expected: os.Stat succeeds but inference will fail

	// Arrange
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model-dir")
	if err := os.MkdirAll(modelPath, 0755); err != nil {
		t.Fatalf("failed to create model directory: %v", err)
	}

	// Create a binary so EnsureReady can succeed
	binDir := filepath.Join(tmpDir, "bin")
	os.MkdirAll(binDir, 0755)
	binPath := filepath.Join(binDir, "bitnet-cli")
	os.WriteFile(binPath, []byte("#!/bin/bash\necho test"), 0755)

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	m := NewInferenceManager(modelPath)

	// Act
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

	// Arrange
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.gguf")
	if err := os.WriteFile(modelPath, []byte("fake model"), 0644); err != nil {
		t.Fatalf("failed to create fake model: %v", err)
	}

	// Create binary WITHOUT execute permission
	binDir := filepath.Join(tmpDir, "bin")
	os.MkdirAll(binDir, 0755)
	binPath := filepath.Join(binDir, "bitnet-cli")
	os.WriteFile(binPath, []byte("#!/bin/bash\necho test"), 0644) // Note: 0644 not 0755

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

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
