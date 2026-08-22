package externalcap

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/openexec/openexec/pkg/db/state"
	"golang.org/x/oauth2"
)

func TestLovableOAuthDiscoveryPersistenceAndDisable(t *testing.T) {
	var identityCalls atomic.Int32
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "fake-lovable", Version: "1"}, nil)
	mcp.AddTool(mcpServer, &mcp.Tool{Name: "get_me"}, func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, map[string]any, error) {
		identityCalls.Add(1)
		return nil, map[string]any{"id": "owner-1", "workspace": "design"}, nil
	})
	mcp.AddTool(mcpServer, &mcp.Tool{Name: "deploy_project"}, func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, map[string]any, error) {
		return nil, map[string]any{"deployed": true}, nil
	})
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpServer },
		&mcp.StreamableHTTPOptions{JSONResponse: true})

	var server *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{"resource": server.URL + "/mcp", "authorization_servers": []string{server.URL}})
	})
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{"resource": server.URL, "authorization_servers": []string{server.URL}})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{"issuer": server.URL, "authorization_endpoint": server.URL + "/authorize",
			"token_endpoint": server.URL + "/token", "registration_endpoint": server.URL + "/register",
			"client_id_metadata_document_supported": true,
			"scopes_supported":                      []string{"read", "offline_access"},
			"code_challenge_methods_supported":      []string{"S256"},
			"token_endpoint_auth_methods_supported": []string{"none"}})
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{"client_id": "openexec-test", "token_endpoint_auth_method": "none"})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.Form.Get("code") != "valid-code" {
			http.Error(w, "bad code", http.StatusBadRequest)
			return
		}
		writeJSON(t, w, map[string]any{"access_token": "secret-access-token", "refresh_token": "secret-refresh-token",
			"token_type": "Bearer", "expires_in": 3600, "scope": "read offline_access"})
	})
	mux.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-access-token" {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+server.URL+`/.well-known/oauth-protected-resource/mcp", scope="read"`)
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	}))
	server = httptest.NewTLSServer(mux)
	defer server.Close()
	defer server.CloseClientConnections()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := state.NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	key := base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	service, err := NewService(store, key)
	if err != nil {
		t.Fatal(err)
	}
	service.httpClient = server.Client()
	created, err := service.Create(context.Background(), CreateInput{Name: "Lovable", Provider: "lovable",
		ServerURL: server.URL + "/mcp", ProjectRef: "openexec"})
	if err != nil {
		t.Fatal(err)
	}
	started, err := service.StartOAuth(context.Background(), created.ID, "openexec",
		"https://console.example.test/oauth/external-connections/callback",
		"https://console.example.test/oauth/client-metadata.json")
	if err != nil {
		t.Fatal(err)
	}
	authURL, err := url.Parse(started.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	if authURL.Query().Get("resource") != server.URL+"/mcp" || authURL.Query().Get("code_challenge") == "" ||
		authURL.Query().Get("client_id") != "https://console.example.test/oauth/client-metadata.json" {
		t.Fatalf("OAuth URL lacks resource binding or PKCE: %s", started.AuthorizationURL)
	}
	completeCtx, cancelComplete := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelComplete()
	completed, err := service.CompleteOAuth(completeCtx, OAuthCallback{
		Code: "valid-code", State: authURL.Query().Get("state"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != StatusHealthy || completed.ToolCount != 2 || completed.CatalogDigest == "" || identityCalls.Load() != 1 {
		t.Fatalf("unexpected healthy connection: %#v identity_calls=%d", completed, identityCalls.Load())
	}
	credential, err := store.GetExternalConnectionCredential(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(credential), "secret-access-token") || strings.Contains(string(credential), "secret-refresh-token") {
		t.Fatal("credential tokens were persisted in plaintext")
	}
	cancelled, err := service.Create(context.Background(), CreateInput{Name: "Cancelled Lovable", Provider: "lovable",
		ServerURL: server.URL + "/mcp", ProjectRef: "openexec"})
	if err != nil {
		t.Fatal(err)
	}
	cancelledStart, err := service.StartOAuth(context.Background(), cancelled.ID, "openexec",
		"https://console.example.test/oauth/external-connections/callback",
		"https://console.example.test/oauth/client-metadata.json")
	if err != nil {
		t.Fatal(err)
	}
	cancelledURL, err := url.Parse(cancelledStart.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Disable(context.Background(), cancelled.ID, "openexec"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteOAuth(context.Background(), OAuthCallback{Code: "valid-code", State: cancelledURL.Query().Get("state")}); err != ErrOAuthFlowNotFound {
		t.Fatalf("callback after disable = %v, want %v", err, ErrOAuthFlowNotFound)
	}
	if identityCalls.Load() != 1 {
		t.Fatalf("disabled authorization reached provider: identity_calls=%d", identityCalls.Load())
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := state.NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	connections, err := reopened.ListExternalConnections(context.Background(), "openexec")
	if err != nil {
		t.Fatal(err)
	}
	restartedConnection := connectionByID(connections, created.ID)
	if restartedConnection == nil || restartedConnection.Status != StatusHealthy || restartedConnection.CatalogDigest != completed.CatalogDigest {
		t.Fatalf("restart projection = %#v", connections)
	}
	if cancelledConnection := connectionByID(connections, cancelled.ID); cancelledConnection == nil || cancelledConnection.Status != StatusDisabled {
		t.Fatalf("cancelled authorization projection = %#v", connections)
	}
	restarted, err := NewService(reopened, key)
	if err != nil {
		t.Fatal(err)
	}
	restarted.httpClient = server.Client()
	if _, err := restarted.Probe(context.Background(), created.ID, "openexec"); err != nil {
		t.Fatalf("restart probe: %v", err)
	}
	if identityCalls.Load() != 2 {
		t.Fatalf("restart probe did not reach provider: identity_calls=%d", identityCalls.Load())
	}
	if _, err := restarted.Disable(context.Background(), created.ID, "openexec"); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Probe(context.Background(), created.ID, "openexec"); err != ErrDisabled {
		t.Fatalf("disabled probe error = %v, want %v", err, ErrDisabled)
	}
	if identityCalls.Load() != 2 {
		t.Fatalf("disabled probe reached provider: identity_calls=%d", identityCalls.Load())
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}

	reopenedDisabled, err := state.NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedDisabled.Close()
	connections, err = reopenedDisabled.ListExternalConnections(context.Background(), "openexec")
	if err != nil {
		t.Fatal(err)
	}
	disabledConnection := connectionByID(connections, created.ID)
	if disabledConnection == nil || disabledConnection.Status != StatusDisabled || disabledConnection.CatalogDigest != completed.CatalogDigest {
		t.Fatalf("disabled restart projection = %#v", connections)
	}
}

func TestLovableOAuthRefusesDynamicRegistrationFallback(t *testing.T) {
	var registrationCalls atomic.Int32
	var server *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{"resource": server.URL + "/mcp", "authorization_servers": []string{server.URL}})
	})
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{"resource": server.URL, "authorization_servers": []string{server.URL}})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{
			"issuer":                                server.URL,
			"authorization_endpoint":                server.URL + "/authorize",
			"token_endpoint":                        server.URL + "/token",
			"registration_endpoint":                 server.URL + "/register",
			"code_challenge_methods_supported":      []string{"S256"},
			"token_endpoint_auth_methods_supported": []string{"none"},
		})
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, _ *http.Request) {
		registrationCalls.Add(1)
		writeJSON(t, w, map[string]any{"client_id": "must-not-be-used", "token_endpoint_auth_method": "none"})
	})
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+server.URL+`/.well-known/oauth-protected-resource/mcp"`)
		http.Error(w, "authorization required", http.StatusUnauthorized)
	})
	server = httptest.NewTLSServer(mux)
	defer server.Close()

	store, err := state.NewStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service, err := NewService(store, base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	if err != nil {
		t.Fatal(err)
	}
	service.httpClient = server.Client()
	created, err := service.Create(context.Background(), CreateInput{
		Name: "Lovable without CIMD", Provider: "lovable", ServerURL: server.URL + "/mcp", ProjectRef: "openexec",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.StartOAuth(context.Background(), created.ID, "openexec",
		"https://console.example.test/oauth/external-connections/callback",
		"https://console.example.test/oauth/client-metadata.json")
	if !errors.Is(err, ErrLovableCIMDRequired) || !strings.Contains(err.Error(), "client_id_metadata_document_supported") {
		t.Fatalf("missing CIMD error = %v", err)
	}
	if got := registrationCalls.Load(); got != 0 {
		t.Fatalf("Lovable dynamic registration calls = %d, want 0", got)
	}
}

func connectionByID(connections []state.ExternalConnection, id string) *state.ExternalConnection {
	for index := range connections {
		if connections[index].ID == id {
			return &connections[index]
		}
	}
	return nil
}

func TestLovablePolicyDoesNotTrustToolAnnotationsOrNames(t *testing.T) {
	for _, tool := range []string{"deploy_project", "query_database", "send_message", "create_project", "set_project_visibility", "read_but_actually_write"} {
		if effect := EffectForTool("lovable", tool); effect == EffectRead {
			t.Fatalf("%s classified as read", tool)
		}
	}
	for _, tool := range []string{"get_me", "list_projects", "get_project", "read_file", "list_design_systems"} {
		if effect := EffectForTool("lovable", tool); effect != EffectRead {
			t.Fatalf("%s classified as %s", tool, effect)
		}
	}
}

func TestProjectBindingCannotBeReused(t *testing.T) {
	store, err := state.NewStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service, err := NewService(store, base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.Create(context.Background(), CreateInput{Name: "Lovable", Provider: "lovable",
		ServerURL: "https://mcp.lovable.dev", ProjectRef: "openexec"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Disable(context.Background(), created.ID, "another-project"); err != ErrProjectNotBound {
		t.Fatalf("cross-project disable error = %v", err)
	}
}

func TestOAuthClientMetadataMustBeAttestedByCallbackOrigin(t *testing.T) {
	store, err := state.NewStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service, err := NewService(store, base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.Create(context.Background(), CreateInput{Name: "Lovable", Provider: "lovable",
		ServerURL: "https://mcp.lovable.dev", ProjectRef: "openexec"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.StartOAuth(context.Background(), created.ID, "openexec",
		"https://console.example.test/oauth/external-connections/callback",
		"https://attacker.example/oauth/client-metadata.json")
	if err == nil || !strings.Contains(err.Error(), "must use the redirect_url origin") {
		t.Fatalf("cross-origin client metadata error = %v", err)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func TestCredentialVaultRejectsWrongKey(t *testing.T) {
	one, _ := newCredentialVault(base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	two, _ := newCredentialVault(base64.RawStdEncoding.EncodeToString([]byte("abcdef0123456789abcdef0123456789")))
	ciphertext, err := one.seal(credentialEnvelope{Token: &oauth2.Token{AccessToken: "secret", Expiry: time.Now().Add(time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := two.open(ciphertext); err == nil {
		t.Fatal("wrong key decrypted credential")
	}
}
