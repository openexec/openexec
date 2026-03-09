package router

import (
	"os"
	"path/filepath"
	"testing"
)

// testEnv holds test environment state for cleanup.
type testEnv struct {
	t        *testing.T
	tmpDir   string
	origDir  string
	origHome string
	origPath string
	cleanup  []func()
}

// newTestEnv creates a new test environment with a temporary directory.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	return &testEnv{
		t:      t,
		tmpDir: t.TempDir(),
	}
}

// createFakeModel creates a fake model file in the temp directory.
func (e *testEnv) createFakeModel(name string) string {
	e.t.Helper()
	modelPath := filepath.Join(e.tmpDir, name)
	if err := os.WriteFile(modelPath, []byte("fake model"), 0644); err != nil {
		e.t.Fatalf("failed to create fake model: %v", err)
	}
	return modelPath
}

// createFakeBinary creates a fake bitnet-cli binary in the temp directory.
// If executable is true, the binary is given execute permissions.
func (e *testEnv) createFakeBinary(executable bool) string {
	e.t.Helper()
	binDir := filepath.Join(e.tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		e.t.Fatalf("failed to create bin dir: %v", err)
	}
	binPath := filepath.Join(binDir, "bitnet-cli")
	perm := os.FileMode(0644)
	if executable {
		perm = 0755
	}
	if err := os.WriteFile(binPath, []byte("#!/bin/bash\necho test"), perm); err != nil {
		e.t.Fatalf("failed to create fake binary: %v", err)
	}
	return binPath
}

// chdir changes to the temp directory and schedules cleanup to restore.
func (e *testEnv) chdir() {
	e.t.Helper()
	origDir, err := os.Getwd()
	if err != nil {
		e.t.Fatalf("failed to get current dir: %v", err)
	}
	e.origDir = origDir
	if err := os.Chdir(e.tmpDir); err != nil {
		e.t.Fatalf("failed to chdir: %v", err)
	}
	e.cleanup = append(e.cleanup, func() { os.Chdir(e.origDir) })
}

// chdirTo changes to the specified directory and schedules cleanup.
func (e *testEnv) chdirTo(dir string) {
	e.t.Helper()
	origDir, err := os.Getwd()
	if err != nil {
		e.t.Fatalf("failed to get current dir: %v", err)
	}
	e.origDir = origDir
	if err := os.MkdirAll(dir, 0755); err != nil {
		e.t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		e.t.Fatalf("failed to chdir: %v", err)
	}
	e.cleanup = append(e.cleanup, func() { os.Chdir(e.origDir) })
}

// setHome sets HOME to the specified directory and schedules cleanup.
func (e *testEnv) setHome(home string) {
	e.t.Helper()
	e.origHome = os.Getenv("HOME")
	os.Setenv("HOME", home)
	e.cleanup = append(e.cleanup, func() { os.Setenv("HOME", e.origHome) })
}

// unsetHome unsets HOME and schedules cleanup.
func (e *testEnv) unsetHome() {
	e.t.Helper()
	e.origHome = os.Getenv("HOME")
	os.Unsetenv("HOME")
	e.cleanup = append(e.cleanup, func() { os.Setenv("HOME", e.origHome) })
}

// setPath sets PATH to the specified value and schedules cleanup.
func (e *testEnv) setPath(path string) {
	e.t.Helper()
	e.origPath = os.Getenv("PATH")
	os.Setenv("PATH", path)
	e.cleanup = append(e.cleanup, func() { os.Setenv("PATH", e.origPath) })
}

// createFakeHome creates a fake HOME structure with .openexec/bin/bitnet-cli.
func (e *testEnv) createFakeHome() string {
	e.t.Helper()
	fakeHome := filepath.Join(e.tmpDir, "fakehome")
	binDir := filepath.Join(fakeHome, ".openexec", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		e.t.Fatalf("failed to create bin dir: %v", err)
	}
	binPath := filepath.Join(binDir, "bitnet-cli")
	if err := os.WriteFile(binPath, []byte("#!/bin/bash\necho test"), 0755); err != nil {
		e.t.Fatalf("failed to create fake binary: %v", err)
	}
	return fakeHome
}

// createModelDir creates a directory (not a file) at the model path.
func (e *testEnv) createModelDir(name string) string {
	e.t.Helper()
	modelPath := filepath.Join(e.tmpDir, name)
	if err := os.MkdirAll(modelPath, 0755); err != nil {
		e.t.Fatalf("failed to create model directory: %v", err)
	}
	return modelPath
}

// done runs all cleanup functions in reverse order.
func (e *testEnv) done() {
	for i := len(e.cleanup) - 1; i >= 0; i-- {
		e.cleanup[i]()
	}
}
