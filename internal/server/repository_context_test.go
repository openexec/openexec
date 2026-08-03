package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/pkg/db/state"
)

func TestRepositoryContextAPIRefreshesAndSurvivesRestart(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package sample\nfunc Main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, ".openexec", "openexec.db")
	stateStore, err := state.NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{StateStore: stateStore, ProjectsDir: root}
	scanRequest := httptest.NewRequest(http.MethodPost, "/api/v1/repository-graph/scan", nil)
	scanResponse := httptest.NewRecorder()
	server.handleRepositoryGraphScan(scanResponse, scanRequest)
	if scanResponse.Code != http.StatusOK {
		t.Fatalf("scan failed: %d %s", scanResponse.Code, scanResponse.Body.String())
	}
	first := requestRepositoryContext(t, server, "/api/v1/repository-context?symbols=Main&task_id=task&run_id=run")
	if first.SchemaVersion != 1 || first.Freshness != knowledge.FreshnessCurrent || len(first.ResolvedSymbols) != 1 {
		t.Fatalf("unexpected first projection: %#v", first)
	}
	if err := stateStore.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := state.NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	server.StateStore = reopened
	second := requestRepositoryContext(t, server, "/api/v1/repository-context?symbols=Main&task_id=task&run_id=run")
	if second.OpenExecReference.ResourceVersion != first.OpenExecReference.ResourceVersion || second.RepositoryID != first.RepositoryID || second.GraphVersion != first.GraphVersion {
		t.Fatalf("projection did not round-trip restart: first=%#v second=%#v", first, second)
	}
	if _, err := os.Stat(filepath.Join(root, "main.go")); err != nil {
		t.Fatal(err)
	}
}

func requestRepositoryContext(t *testing.T, server *Server, target string) knowledge.RepositoryContextProjection {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	response := httptest.NewRecorder()
	server.handleRepositoryContext(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("repository context failed: %d %s", response.Code, response.Body.String())
	}
	var projection knowledge.RepositoryContextProjection
	if err := json.Unmarshal(response.Body.Bytes(), &projection); err != nil {
		t.Fatal(err)
	}
	return projection
}
