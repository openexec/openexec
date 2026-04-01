package router

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveModelPath_GlobalDir(t *testing.T) {
	tmpDir := t.TempDir()
	globalDir := filepath.Join(tmpDir, "global-models")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	modelFile := filepath.Join(globalDir, DefaultModelFilename)
	if err := os.WriteFile(modelFile, []byte("fake-gguf-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	mm := NewModelManager(DefaultModelName)
	mm.SetGlobalDir(globalDir)

	path, err := mm.ResolveModelPath()
	if err != nil {
		t.Fatalf("expected model to be found, got error: %v", err)
	}
	if path != modelFile {
		t.Errorf("expected path %q, got %q", modelFile, path)
	}
}

func TestResolveModelPath_ProjectDir(t *testing.T) {
	tmpDir := t.TempDir()
	globalDir := filepath.Join(tmpDir, "global-models")
	projectDir := filepath.Join(tmpDir, "project-models")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	globalFile := filepath.Join(globalDir, DefaultModelFilename)
	projectFile := filepath.Join(projectDir, DefaultModelFilename)
	if err := os.WriteFile(globalFile, []byte("global-model"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectFile, []byte("project-model"), 0o644); err != nil {
		t.Fatal(err)
	}

	mm := NewModelManager(DefaultModelName)
	mm.SetGlobalDir(globalDir)
	mm.SetProjectDir(projectDir)

	path, err := mm.ResolveModelPath()
	if err != nil {
		t.Fatalf("expected model to be found, got error: %v", err)
	}
	if path != projectFile {
		t.Errorf("expected project path %q to take precedence, got %q", projectFile, path)
	}
}

func TestResolveModelPath_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	globalDir := filepath.Join(tmpDir, "empty-global")
	projectDir := filepath.Join(tmpDir, "empty-project")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mm := NewModelManager(DefaultModelName)
	mm.SetGlobalDir(globalDir)
	mm.SetProjectDir(projectDir)

	_, err := mm.ResolveModelPath()
	if err == nil {
		t.Fatal("expected error when model not found")
	}
}

func TestEnsureModel_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	globalDir := filepath.Join(tmpDir, "global-models")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	modelFile := filepath.Join(globalDir, DefaultModelFilename)
	if err := os.WriteFile(modelFile, []byte("existing-model"), 0o644); err != nil {
		t.Fatal(err)
	}

	mm := NewModelManager(DefaultModelName)
	mm.SetGlobalDir(globalDir)
	// Set download URL to something that would fail — ensures no download attempt
	mm.SetDownloadURL("http://127.0.0.1:1/should-not-be-called")

	path, err := mm.EnsureModel()
	if err != nil {
		t.Fatalf("expected model to be found without download, got error: %v", err)
	}
	if path != modelFile {
		t.Errorf("expected path %q, got %q", modelFile, path)
	}
}

func TestDownload_Success(t *testing.T) {
	fakeContent := []byte("fake-gguf-model-binary-content-for-testing")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fakeContent)))
		w.WriteHeader(http.StatusOK)
		w.Write(fakeContent)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	globalDir := filepath.Join(tmpDir, "global-models")

	mm := NewModelManager(DefaultModelName)
	mm.SetGlobalDir(globalDir)
	mm.SetDownloadURL(server.URL + "/bitnet-2b.Q4_K_M.gguf")

	path, err := mm.Download()
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("downloaded file not found at %s: %v", path, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(data) != string(fakeContent) {
		t.Errorf("content mismatch: got %q, want %q", data, fakeContent)
	}

	// Verify no .tmp file remains
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); err == nil {
		t.Error("temporary file should have been cleaned up")
	}
}

func TestDownload_Checksum(t *testing.T) {
	fakeContent := []byte("checksum-verified-model-data")
	hasher := sha256.New()
	hasher.Write(fakeContent)
	correctHash := hex.EncodeToString(hasher.Sum(nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fakeContent)))
		w.WriteHeader(http.StatusOK)
		w.Write(fakeContent)
	}))
	defer server.Close()

	t.Run("correct checksum", func(t *testing.T) {
		tmpDir := t.TempDir()
		globalDir := filepath.Join(tmpDir, "global-models")

		mm := NewModelManager(DefaultModelName)
		mm.SetGlobalDir(globalDir)
		mm.SetDownloadURL(server.URL + "/model.gguf")
		mm.SetExpectedHash(correctHash)

		path, err := mm.Download()
		if err != nil {
			t.Fatalf("download with correct checksum should succeed: %v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("file should exist at %s", path)
		}
	})

	t.Run("wrong checksum", func(t *testing.T) {
		tmpDir := t.TempDir()
		globalDir := filepath.Join(tmpDir, "global-models")

		mm := NewModelManager(DefaultModelName)
		mm.SetGlobalDir(globalDir)
		mm.SetDownloadURL(server.URL + "/model.gguf")
		mm.SetExpectedHash("0000000000000000000000000000000000000000000000000000000000000000")

		_, err := mm.Download()
		if err == nil {
			t.Fatal("download with wrong checksum should fail")
		}
		if !strings.Contains(err.Error(), "checksum mismatch") {
			t.Errorf("error should mention checksum mismatch, got: %v", err)
		}

		// Verify .tmp file was cleaned up
		tmpPath := filepath.Join(globalDir, DefaultModelFilename+".tmp")
		if _, err := os.Stat(tmpPath); err == nil {
			t.Error("temporary file should have been cleaned up after checksum failure")
		}
	})
}

func TestDownload_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	globalDir := filepath.Join(tmpDir, "global-models")

	mm := NewModelManager(DefaultModelName)
	mm.SetGlobalDir(globalDir)
	mm.SetDownloadURL(server.URL + "/missing.gguf")

	_, err := mm.Download()
	if err == nil {
		t.Fatal("expected error for HTTP 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention HTTP status, got: %v", err)
	}
}

func TestDownload_NoGlobalDir(t *testing.T) {
	mm := NewModelManager(DefaultModelName)
	mm.SetGlobalDir("")

	_, err := mm.Download()
	if err == nil {
		t.Fatal("expected error when global dir not configured")
	}
}
