package knowledge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManager(t *testing.T) {
	// Arrange
	m := NewManager()
	tmpDir := t.TempDir()

	p1 := filepath.Join(tmpDir, "project1")
	p2 := filepath.Join(tmpDir, "project2")
	// Create project dirs with .openexec subdirs for the store
	os.MkdirAll(filepath.Join(p1, ".openexec"), 0755)
	os.MkdirAll(filepath.Join(p2, ".openexec"), 0755)

	t.Run("Isolate Projects", func(t *testing.T) {
		// Act
		s1, _ := m.GetStore(p1)
		s2, _ := m.GetStore(p2)

		s1.SetEnvironment(&EnvironmentRecord{Env: "dev", RuntimeType: "local"})
		s2.SetEnvironment(&EnvironmentRecord{Env: "dev", RuntimeType: "k8s"})

		res1, _ := s1.GetEnvironment("dev")
		res2, _ := s2.GetEnvironment("dev")

		// Assert
		if res1.RuntimeType != "local" || res2.RuntimeType != "k8s" {
			t.Errorf("stores are not isolated: got %s and %s", res1.RuntimeType, res2.RuntimeType)
		}
	})

	t.Run("Re-use Active Store", func(t *testing.T) {
		// Act
		s1a, _ := m.GetStore(p1)
		s1b, _ := m.GetStore(p1)

		// Assert
		if s1a != s1b {
			t.Error("expected same store instance for same path")
		}
	})

	t.Run("Close All", func(t *testing.T) {
		// Act
		m.CloseAll()
		// Since we closed them, opening again should create new instances
		s1new, _ := m.GetStore(p1)
		if s1new == nil {
			t.Error("failed to re-open store after close all")
		}
	})
}
