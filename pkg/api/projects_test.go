package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/openexec/openexec/internal/execution/gates"
)

func TestHandleListProjects(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock project
	projectPath := filepath.Join(tmpDir, "test-project")
	err := os.MkdirAll(projectPath, 0755)
	if err != nil {
		t.Fatal(err)
	}

	yamlContent := "project:\n  name: test-project\n  type: fullstack-webapp"
	err = os.WriteFile(filepath.Join(projectPath, "openexec.yaml"), []byte(yamlContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	srv := New(nil, nil, nil, tmpDir, ":0")

	req := httptest.NewRequest("GET", "/api/projects", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var projects []ProjectInfo
	if err := json.NewDecoder(rec.Body).Decode(&projects); err != nil {
		t.Fatal(err)
	}

	if len(projects) != 1 {
		t.Fatalf("len(projects) = %d, want 1", len(projects))
	}

	if projects[0].Name != "test-project" {
		t.Errorf("name = %s, want test-project", projects[0].Name)
	}
}

func TestHandleInitProject(t *testing.T) {
	tmpDir := t.TempDir()
	srv := New(nil, nil, nil, tmpDir, ":0")

	initReq := InitProjectRequest{
		Name: "new-app",
		Path: "new-app",
	}
	body, _ := json.Marshal(initReq)

	req := httptest.NewRequest("POST", "/api/projects/init", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	// Verify filesystem
	if _, err := os.Stat(filepath.Join(tmpDir, "new-app", "openexec.yaml")); os.IsNotExist(err) {
		t.Error("openexec.yaml was not created")
	}
	// The empty project created over the API declares no gate it cannot run,
	// and its persisted openexec.yaml still lists as a project.
	cfg, err := gates.LoadConfig(filepath.Join(tmpDir, "new-app"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.GetEnabledGates()) != 0 || cfg.Project.Name != "new-app" {
		t.Fatalf("API-created project declared %v (name %q)", cfg.GetEnabledGates(), cfg.Project.Name)
	}
	list := httptest.NewRecorder()
	srv.Handler().ServeHTTP(list, httptest.NewRequest("GET", "/api/projects", nil))
	var projects []ProjectInfo
	if err := json.Unmarshal(list.Body.Bytes(), &projects); err != nil || len(projects) != 1 || projects[0].Name != "new-app" {
		t.Fatalf("created project not listed: %s %v", list.Body.String(), err)
	}
}
