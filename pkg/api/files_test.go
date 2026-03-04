package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleListDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create some subdirectories
	err := os.MkdirAll(filepath.Join(tmpDir, "dir1"), 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(filepath.Join(tmpDir, "dir2"), 0755)
	if err != nil {
		t.Fatal(err)
	}
	// Create a file (should be ignored)
	err = os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("test"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	srv := New(nil, nil, nil, tmpDir, ":0")

	req := httptest.NewRequest("GET", "/api/directories?path="+tmpDir, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var dirs []DirectoryInfo
	if err := json.NewDecoder(rec.Body).Decode(&dirs); err != nil {
		t.Fatal(err)
	}

	// Should have dir1, dir2 (and potentially .. if not at filesystem root)
	// We check for at least dir1 and dir2
	foundDir1 := false
	foundDir2 := false
	for _, d := range dirs {
		if d.Name == "dir1" {
			foundDir1 = true
		}
		if d.Name == "dir2" {
			foundDir2 = true
		}
		if d.Name == "file.txt" {
			t.Error("found file in directory list")
		}
	}

	if !foundDir1 || !foundDir2 {
		t.Errorf("missing directories: foundDir1=%v, foundDir2=%v", foundDir1, foundDir2)
	}
}
