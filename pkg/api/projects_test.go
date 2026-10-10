package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
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
	// Default kind is a repository project: it gets a git repository.
	if _, err := os.Stat(filepath.Join(tmpDir, "new-app", ".git")); err != nil {
		t.Errorf("repository project has no .git: %v", err)
	}
}

func postInit(t *testing.T, srv *Server, req InitProjectRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/api/projects/init", bytes.NewReader(body)))
	return rec
}

func TestHandleInitProjectChatKindRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	srv := New(nil, nil, nil, tmpDir, ":0")

	// A chat project needs only a name; it lands under the projects root.
	if rec := postInit(t, srv, InitProjectRequest{Name: "ideas", Kind: "chat"}); rec.Code != http.StatusCreated {
		t.Fatalf("chat init status = %d, body %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "ideas", ".git")); !os.IsNotExist(err) {
		t.Errorf("chat project must not get a git repository (stat err=%v)", err)
	}
	if rec := postInit(t, srv, InitProjectRequest{Name: "code", Path: "code", Kind: "repository"}); rec.Code != http.StatusCreated {
		t.Fatalf("repository init status = %d, body %s", rec.Code, rec.Body.String())
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/projects", nil))
	var projects []ProjectInfo
	if err := json.NewDecoder(rec.Body).Decode(&projects); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{}
	for _, p := range projects {
		kinds[p.Name] = p.Kind
	}
	if kinds["ideas"] != "chat" || kinds["code"] != "repository" {
		t.Errorf("listed kinds = %v, want ideas=chat code=repository", kinds)
	}
}

func TestHandleInitProjectRejectsBadRequests(t *testing.T) {
	tmpDir := t.TempDir()
	srv := New(nil, nil, nil, tmpDir, ":0")

	cases := map[string]InitProjectRequest{
		"unknown kind":              {Name: "x", Path: "x", Kind: "wiki"},
		"repository without path":   {Name: "x", Kind: "repository"},
		"chat without name or path": {Kind: "chat"},
		"chat name escaping root":   {Name: "../escape", Kind: "chat"},
	}
	for name, req := range cases {
		if rec := postInit(t, srv, req); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400 (body %s)", name, rec.Code, rec.Body.String())
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(tmpDir), "escape")); !os.IsNotExist(err) {
		t.Error("chat name escaped the projects root")
	}
}
