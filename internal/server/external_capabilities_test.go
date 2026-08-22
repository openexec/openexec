package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const externalCapabilityTestToken = "external-capability-test-token"

func TestExternalCapabilityAPIIsAuthenticatedProjectBoundAndDisablable(t *testing.T) {
	s, err := New(Config{Port: 0, ProjectsDir: t.TempDir(), DataDir: t.TempDir(), SkipPreflight: true,
		ExternalCapabilityToken: externalCapabilityTestToken,
		ExternalCredentialKey:   base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.StateStore.Close() })

	unauthorized := httptest.NewRequest(http.MethodGet, "/api/v1/external-connections?project_ref=openexec", nil)
	response := httptest.NewRecorder()
	s.Mux.ServeHTTP(response, unauthorized)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized list = %d; body=%s", response.Code, response.Body.String())
	}

	createdResponse := externalCapabilityRequest(t, s, http.MethodPost, "/api/v1/external-connections", map[string]any{
		"name": "Lovable", "provider": "lovable", "server_url": "https://mcp.lovable.dev", "project_ref": "openexec",
	})
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create = %d; body=%s", createdResponse.Code, createdResponse.Body.String())
	}
	var created struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Bindings []struct {
			ProjectRef   string   `json:"project_ref"`
			AllowedTools []string `json:"allowed_tools"`
		} `json:"bindings"`
	}
	if err := json.Unmarshal(createdResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Status != "pending_authorization" || len(created.Bindings) != 1 ||
		created.Bindings[0].ProjectRef != "openexec" {
		t.Fatalf("created projection = %#v", created)
	}
	for _, forbidden := range []string{"deploy_project", "query_database", "send_message"} {
		for _, allowed := range created.Bindings[0].AllowedTools {
			if allowed == forbidden {
				t.Fatalf("forbidden tool %q entered binding", forbidden)
			}
		}
	}

	crossProject := externalCapabilityRequest(t, s, http.MethodPost,
		"/api/v1/external-connections/"+created.ID+"/disable", map[string]string{"project_ref": "other"})
	if crossProject.Code != http.StatusForbidden {
		t.Fatalf("cross-project disable = %d; body=%s", crossProject.Code, crossProject.Body.String())
	}
	disabled := externalCapabilityRequest(t, s, http.MethodPost,
		"/api/v1/external-connections/"+created.ID+"/disable", map[string]string{"project_ref": "openexec"})
	if disabled.Code != http.StatusOK || !bytes.Contains(disabled.Body.Bytes(), []byte(`"status":"disabled"`)) {
		t.Fatalf("disable = %d; body=%s", disabled.Code, disabled.Body.String())
	}
	probe := externalCapabilityRequest(t, s, http.MethodPost,
		"/api/v1/external-connections/"+created.ID+"/probe", map[string]string{"project_ref": "openexec"})
	if probe.Code != http.StatusConflict {
		t.Fatalf("disabled probe = %d; body=%s", probe.Code, probe.Body.String())
	}
}

func TestExternalCapabilityCredentialsRequireDedicatedConfiguration(t *testing.T) {
	_, err := New(Config{ProjectsDir: t.TempDir(), DataDir: t.TempDir(), SkipPreflight: true,
		ExternalCapabilityToken: "configured-without-key"})
	if err == nil {
		t.Fatal("server accepted an external capability token without a credential key")
	}
	_, err = New(Config{ProjectsDir: t.TempDir(), DataDir: t.TempDir(), SkipPreflight: true,
		RepositoryGraphToken: "shared", ExternalCapabilityToken: "shared",
		ExternalCredentialKey: base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))})
	if err == nil {
		t.Fatal("server accepted a capability token shared with graph authority")
	}
}

func externalCapabilityRequest(t *testing.T, server *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	request.Header.Set("Authorization", "Bearer "+externalCapabilityTestToken)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Mux.ServeHTTP(response, request)
	return response
}
