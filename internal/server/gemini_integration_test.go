package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/agent"
)

// MockProvider implements the agent.ProviderAdapter interface for testing.
type MockProvider struct {
	Response string
	Err      error
	Called   bool
}

func (m *MockProvider) Complete(ctx context.Context, req agent.Request) (*agent.Response, error) {
	m.Called = true
	if m.Err != nil {
		return nil, m.Err
	}
	return &agent.Response{
		Content: []agent.ContentBlock{
			{Type: agent.ContentTypeText, Text: m.Response},
		},
	}, nil
}

func (m *MockProvider) GetName() string     { return "gemini" }
func (m *MockProvider) GetModels() []string { return []string{"gemini-3.1-pro-preview"} }
func (m *MockProvider) GetModelInfo(id string) (*agent.ModelInfo, error) {
	return &agent.ModelInfo{}, nil
}
func (m *MockProvider) GetCapabilities(id string) (*agent.ProviderCapabilities, error) {
	return &agent.ProviderCapabilities{}, nil
}
func (m *MockProvider) Stream(ctx context.Context, req agent.Request) (<-chan agent.StreamEvent, error) {
	return nil, nil
}
func (m *MockProvider) ValidateRequest(req agent.Request) error { return nil }
func (m *MockProvider) EstimateTokens(content string) int       { return 0 }

func TestGeminiProviderBackedExecution(t *testing.T) {
	t.Skip("provider-backed execution path removed; Gemini uses CLI runner")
	// Force provider-backed execution (bypasses CLI path detection)
	t.Setenv("OPENEXEC_FORCE_PROVIDER", "1")

	// Setup workspace
	tmpDir := t.TempDir()
	openexecDir := filepath.Join(tmpDir, ".openexec")
	_ = os.MkdirAll(openexecDir, 0750)

	// Create project config with Gemini model
	configJSON := `{
		"name": "gemini-test",
		"execution": {
			"executor_model": "gemini-3.1-pro-preview",
			"parallel_enabled": false
		}
	}`
	_ = os.WriteFile(filepath.Join(tmpDir, "openexec.yaml"), []byte("project:\n  name: gemini-test\n"), 0644)
	_ = os.WriteFile(filepath.Join(openexecDir, "config.json"), []byte(configJSON), 0644)

	// Mock the Gemini provider
	mock := &MockProvider{Response: "Success from Mock Gemini"}
	agent.DefaultRegistry.Register(mock)
	defer func() {
		// Cleanup registry after test (hacky but works for global registry)
		// In a real app we might want a way to unregister or use a local registry
	}()

	// Initialize Server
	cfg := Config{
		Port:        0, // Random port
		ProjectsDir: tmpDir,
		DataDir:     t.TempDir(),
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Create a task ID for testing
	taskID := "T-001"

	// Start a loop via API
	ts := httptest.NewServer(s.Mux)
	defer ts.Close()

    resp, err := http.Post(ts.URL+"/api/fwu/T-001/start", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		var errData map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&errData)
		t.Fatalf("Expected 201 Created, got %d. Error: %v", resp.StatusCode, errData)
	}

	// Wait for the loop to hit the provider
	// Since it's provider-backed, it should be very fast
	deadline := time.Now().Add(5 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		if mock.Called {
			found = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !found {
		t.Error("Gemini provider was never called - engine didn't route to provider-backed path")
	}

	// Verify status
    statusResp, err := http.Get(ts.URL + "/api/fwu/" + taskID + "/status")
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}
	var info struct {
		Status string `json:"status"`
		Agent  string `json:"agent"`
	}
	_ = json.NewDecoder(statusResp.Body).Decode(&info)

	t.Logf("Task status: %s, Agent: %s", info.Status, info.Agent)

	// Task should be running or starting if provider was hit
	if info.Status == "" {
		t.Error("Empty task status")
	}
}

func TestGeminiRunnerMapping(t *testing.T) {
	// This test verifies that the server correctly maps the gemini model to the gemini runner
	// even if we don't have the binary (by checking the log or internal state)

	tmpDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0750)

	configJSON := `{
        "name": "mapping-test",
        "execution": {
            "executor_model": "gemini-3.1-pro-preview"
        }
    }`
	_ = os.WriteFile(filepath.Join(tmpDir, "openexec.yaml"), []byte("project:\n  name: test\n"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, ".openexec", "config.json"), []byte(configJSON), 0644)

	cfg := Config{
		Port:        0,
		ProjectsDir: tmpDir,
		DataDir:     t.TempDir(),
	}

	s, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	if s.Mgr == nil {
		t.Fatal("manager not initialized")
	}

	mgrCfg := s.Mgr.GetConfig()
	// Based on our Resolve logic, it should contain "gemini" if found on PATH,
	// but since we might not have it in CI, let's at least check the ExecutorModel was passed
	if mgrCfg.ExecutorModel != "gemini-3.1-pro-preview" {
		t.Errorf("expected ExecutorModel gemini-3.1-pro-preview, got %q", mgrCfg.ExecutorModel)
	}
}
