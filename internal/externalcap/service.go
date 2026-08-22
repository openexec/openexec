// Package externalcap owns governed connections from OpenExec to external MCP
// capability providers. Agent Console may administer this package through the
// narrow server API, but connection policy and credentials remain here.
package externalcap

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"github.com/openexec/openexec/pkg/db/state"
	"golang.org/x/oauth2"
)

const (
	StatusPendingAuthorization = "pending_authorization"
	StatusAuthorizing          = "authorizing"
	StatusHealthy              = "healthy"
	StatusDisabled             = "disabled"
	StatusError                = "error"
	EffectRead                 = "read"
	defaultFlowLifetime        = 10 * time.Minute
)

var (
	ErrDisabled            = errors.New("external connection is disabled")
	ErrProjectNotBound     = errors.New("external connection is not bound to this project")
	ErrToolNotAllowed      = errors.New("external tool is not allowed by the project binding")
	ErrOAuthFlowNotFound   = errors.New("OAuth flow not found or expired")
	ErrCredentialKeyNeeded = errors.New("external credential encryption key is not configured")
)

var lovableReadTools = map[string]struct{}{
	"get_me": {}, "list_workspaces": {}, "get_workspace": {},
	"list_projects": {}, "get_project": {}, "list_template_projects": {}, "list_design_systems": {},
	"get_message": {}, "list_messages": {},
	"get_workspace_knowledge": {}, "get_project_knowledge": {}, "list_workspace_skills": {}, "get_workspace_skill": {},
	"get_diff": {}, "list_files": {}, "read_file": {}, "list_edits": {},
	"get_database_status": {},
	"list_connectors":     {}, "list_connections": {}, "list_custom_connectors": {}, "list_available_connectors": {},
	"get_project_analytics": {}, "get_project_analytics_trend": {},
}

type Store interface {
	CreateExternalConnection(context.Context, state.ExternalConnection, state.ExternalConnectionBinding) error
	ListExternalConnections(context.Context, string) ([]state.ExternalConnection, error)
	GetExternalConnection(context.Context, string) (state.ExternalConnection, error)
	GetExternalConnectionCredential(context.Context, string) ([]byte, error)
	GetExternalConnectionBinding(context.Context, string, string) (state.ExternalConnectionBinding, error)
	UpdateExternalConnectionCredential(context.Context, string, []byte, time.Time) error
	SetExternalConnectionStatus(context.Context, string, string, string, time.Time) error
	RecordExternalCatalog(context.Context, string, state.ExternalCatalogSnapshot, json.RawMessage) error
	RecordExternalInvocation(context.Context, state.ExternalInvocation) error
}

type Service struct {
	store      Store
	vault      *credentialVault
	httpClient *http.Client
	now        func() time.Time
	mu         sync.Mutex
	flows      map[string]*oauthFlow
}

type CreateInput struct {
	Name         string   `json:"name"`
	Provider     string   `json:"provider"`
	ServerURL    string   `json:"server_url"`
	ProjectRef   string   `json:"project_ref"`
	AllowedTools []string `json:"allowed_tools,omitempty"`
}

type OAuthStart struct {
	ConnectionID     string    `json:"connection_id"`
	AuthorizationURL string    `json:"authorization_url"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type OAuthCallback struct {
	Code  string `json:"code"`
	State string `json:"state"`
	Iss   string `json:"iss,omitempty"`
}

type credentialEnvelope struct {
	Config oauthConfig   `json:"config"`
	Token  *oauth2.Token `json:"token"`
}

type oauthConfig struct {
	ClientID     string          `json:"client_id"`
	ClientSecret string          `json:"client_secret,omitempty"`
	RedirectURL  string          `json:"redirect_url"`
	Scopes       []string        `json:"scopes,omitempty"`
	Endpoint     oauth2.Endpoint `json:"endpoint"`
}

func (c oauthConfig) SDK() *oauth2.Config {
	return &oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, RedirectURL: c.RedirectURL,
		Scopes: append([]string(nil), c.Scopes...), Endpoint: c.Endpoint}
}

type oauthFlow struct {
	connectionID string
	projectRef   string
	expiresAt    time.Time
	callback     chan auth.AuthorizationResult
	done         chan error
	cancel       context.CancelFunc
}

func NewService(store Store, key string) (*Service, error) {
	vault, err := newCredentialVault(key)
	if err != nil {
		return nil, err
	}
	return &Service{store: store, vault: vault, httpClient: safeHTTPClient(), now: time.Now, flows: map[string]*oauthFlow{}}, nil
}

func (s *Service) Create(ctx context.Context, input CreateInput) (state.ExternalConnection, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	input.ProjectRef = strings.TrimSpace(input.ProjectRef)
	serverURL, err := validateServerURL(input.ServerURL)
	if err != nil {
		return state.ExternalConnection{}, err
	}
	if input.Name == "" || input.ProjectRef == "" {
		return state.ExternalConnection{}, errors.New("name and project_ref are required")
	}
	if input.Provider == "" {
		input.Provider = "mcp"
	}
	allowed := normalizeAllowedTools(input.Provider, input.AllowedTools)
	now := s.now().UTC()
	id := "external_connection_" + uuid.NewString()
	connection := state.ExternalConnection{ID: id, Name: input.Name, Provider: input.Provider, ServerURL: serverURL,
		CredentialRef: "external-credential:" + id, Status: StatusPendingAuthorization, Identity: json.RawMessage(`{}`),
		CreatedAt: now, UpdatedAt: now}
	binding := state.ExternalConnectionBinding{ConnectionID: id, ProjectRef: input.ProjectRef,
		AllowedEffects: []string{EffectRead}, AllowedTools: allowed, CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateExternalConnection(ctx, connection, binding); err != nil {
		return state.ExternalConnection{}, err
	}
	connection.Bindings = []state.ExternalConnectionBinding{binding}
	return connection, nil
}

func (s *Service) List(ctx context.Context, projectRef string) ([]state.ExternalConnection, error) {
	return s.store.ListExternalConnections(ctx, strings.TrimSpace(projectRef))
}

func (s *Service) StartOAuth(ctx context.Context, connectionID, projectRef, redirectURL, clientMetadataURL string) (OAuthStart, error) {
	connection, binding, err := s.authorizedConnection(ctx, connectionID, projectRef, false)
	if err != nil {
		return OAuthStart{}, err
	}
	redirect, err := validateRedirectURL(redirectURL)
	if err != nil {
		return OAuthStart{}, err
	}
	metadata, err := validateClientMetadataURL(clientMetadataURL, redirect)
	if err != nil {
		return OAuthStart{}, err
	}
	flowCtx, cancel := context.WithTimeout(context.Background(), defaultFlowLifetime)
	flow := &oauthFlow{connectionID: connection.ID, projectRef: binding.ProjectRef,
		expiresAt: s.now().UTC().Add(defaultFlowLifetime), callback: make(chan auth.AuthorizationResult, 1),
		done: make(chan error, 1), cancel: cancel}
	authorizationURL := make(chan string, 1)
	go func() {
		flow.done <- s.authorizeAndDiscover(flowCtx, connection, binding, redirectURL, metadata.String(), authorizationURL, flow.callback)
		cancel()
	}()
	select {
	case rawURL := <-authorizationURL:
		parsed, parseErr := url.Parse(rawURL)
		if parseErr != nil || parsed.Query().Get("state") == "" {
			cancel()
			return OAuthStart{}, errors.New("authorization server returned an invalid authorization URL")
		}
		stateValue := parsed.Query().Get("state")
		s.mu.Lock()
		s.pruneFlowsLocked()
		s.flows[stateValue] = flow
		s.mu.Unlock()
		_ = s.store.SetExternalConnectionStatus(context.Background(), connection.ID, StatusAuthorizing, "", s.now().UTC())
		return OAuthStart{ConnectionID: connection.ID, AuthorizationURL: rawURL, ExpiresAt: flow.expiresAt}, nil
	case err := <-flow.done:
		cancel()
		return OAuthStart{}, err
	case <-ctx.Done():
		cancel()
		return OAuthStart{}, ctx.Err()
	case <-time.After(20 * time.Second):
		cancel()
		return OAuthStart{}, errors.New("OAuth discovery did not return an authorization URL")
	}
}

func (s *Service) CompleteOAuth(ctx context.Context, callback OAuthCallback) (state.ExternalConnection, error) {
	callback.Code, callback.State, callback.Iss = strings.TrimSpace(callback.Code), strings.TrimSpace(callback.State), strings.TrimSpace(callback.Iss)
	if callback.Code == "" || callback.State == "" {
		return state.ExternalConnection{}, errors.New("code and state are required")
	}
	s.mu.Lock()
	s.pruneFlowsLocked()
	flow := s.flows[callback.State]
	if flow != nil {
		delete(s.flows, callback.State)
	}
	s.mu.Unlock()
	if flow == nil {
		return state.ExternalConnection{}, ErrOAuthFlowNotFound
	}
	select {
	case flow.callback <- auth.AuthorizationResult{Code: callback.Code, State: callback.State, Iss: callback.Iss}:
	case <-ctx.Done():
		flow.cancel()
		return state.ExternalConnection{}, ctx.Err()
	}
	select {
	case err := <-flow.done:
		if err != nil {
			_ = s.store.SetExternalConnectionStatus(context.Background(), flow.connectionID, StatusError, err.Error(), s.now().UTC())
			return state.ExternalConnection{}, err
		}
		return s.store.GetExternalConnection(ctx, flow.connectionID)
	case <-ctx.Done():
		flow.cancel()
		return state.ExternalConnection{}, ctx.Err()
	}
}

func (s *Service) Probe(ctx context.Context, connectionID, projectRef string) (state.ExternalConnection, error) {
	connection, binding, err := s.authorizedConnection(ctx, connectionID, projectRef, true)
	if err != nil {
		return state.ExternalConnection{}, err
	}
	ciphertext, err := s.store.GetExternalConnectionCredential(ctx, connection.ID)
	if err != nil {
		return state.ExternalConnection{}, err
	}
	envelope, err := s.vault.open(ciphertext)
	if err != nil {
		return state.ExternalConnection{}, err
	}
	if err := s.discover(ctx, connection, binding, envelope, nil); err != nil {
		_ = s.store.SetExternalConnectionStatus(context.Background(), connection.ID, StatusError, err.Error(), s.now().UTC())
		return state.ExternalConnection{}, err
	}
	return s.store.GetExternalConnection(ctx, connection.ID)
}

func (s *Service) Disable(ctx context.Context, connectionID, projectRef string) (state.ExternalConnection, error) {
	connection, _, err := s.authorizedConnection(ctx, connectionID, projectRef, false)
	if err != nil {
		return state.ExternalConnection{}, err
	}
	if err := s.store.SetExternalConnectionStatus(ctx, connection.ID, StatusDisabled, "disabled by owner", s.now().UTC()); err != nil {
		return state.ExternalConnection{}, err
	}
	s.cancelFlowsForConnection(connection.ID)
	return s.store.GetExternalConnection(ctx, connection.ID)
}

func (s *Service) authorizeAndDiscover(ctx context.Context, connection state.ExternalConnection, binding state.ExternalConnectionBinding,
	redirectURL, clientMetadataURL string, authorizationURL chan<- string, callback <-chan auth.AuthorizationResult) error {
	var persisted credentialEnvelope
	handler, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		RedirectURL:                    redirectURL,
		ClientIDMetadataDocumentConfig: &auth.ClientIDMetadataDocumentConfig{URL: clientMetadataURL},
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{Metadata: &oauthex.ClientRegistrationMetadata{
			RedirectURIs: []string{redirectURL}, TokenEndpointAuthMethod: "none",
			GrantTypes: []string{"authorization_code", "refresh_token"}, ResponseTypes: []string{"code"},
			ClientName: "OpenExec", SoftwareID: "openexec", ApplicationType: "web",
		}},
		AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			select {
			case authorizationURL <- args.URL:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			select {
			case result := <-callback:
				return &result, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
		RequestRefreshToken: true,
		Client:              s.httpClient,
		NewTokenSource: func(tokenCtx context.Context, config *oauth2.Config, token *oauth2.Token) (oauth2.TokenSource, error) {
			persisted = credentialEnvelope{Config: oauthConfig{ClientID: config.ClientID, ClientSecret: config.ClientSecret,
				RedirectURL: config.RedirectURL, Scopes: append([]string(nil), config.Scopes...), Endpoint: config.Endpoint}, Token: token}
			return &persistingTokenSource{source: config.TokenSource(tokenCtx, token), save: func(next *oauth2.Token) error {
				persisted.Token = next
				return s.saveCredential(context.Background(), connection.ID, persisted)
			}}, nil
		},
	})
	if err != nil {
		return err
	}
	if err := s.discover(ctx, connection, binding, credentialEnvelope{}, handler); err != nil {
		return err
	}
	if persisted.Token == nil {
		return errors.New("OAuth completed without a token")
	}
	return s.saveCredential(context.Background(), connection.ID, persisted)
}

func (s *Service) discover(ctx context.Context, connection state.ExternalConnection, binding state.ExternalConnectionBinding,
	envelope credentialEnvelope, handler auth.OAuthHandler) error {
	if connection.Status == StatusDisabled {
		return ErrDisabled
	}
	if handler == nil {
		if envelope.Token == nil {
			return errors.New("connection has no stored OAuth credential")
		}
		config := envelope.Config.SDK()
		clientCredentials := &oauthex.ClientCredentials{ClientID: envelope.Config.ClientID}
		if envelope.Config.ClientSecret != "" {
			clientCredentials.ClientSecretAuth = &oauthex.ClientSecretAuth{ClientSecret: envelope.Config.ClientSecret}
		}
		restoredHandler, restoreErr := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
			RedirectURL:         envelope.Config.RedirectURL,
			PreregisteredClient: clientCredentials,
			AuthorizationCodeFetcher: func(context.Context, *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
				return nil, errors.New("stored OAuth credential requires reauthorization")
			},
			Client: s.httpClient,
			InitialTokenSource: &persistingTokenSource{source: config.TokenSource(ctx, envelope.Token), save: func(next *oauth2.Token) error {
				envelope.Token = next
				return s.saveCredential(context.Background(), connection.ID, envelope)
			}},
		})
		if restoreErr != nil {
			return fmt.Errorf("restore OAuth handler: %w", restoreErr)
		}
		handler = restoredHandler
	}
	transport := &mcp.StreamableClientTransport{Endpoint: connection.ServerURL, HTTPClient: s.httpClient,
		OAuthHandler: handler, DisableStandaloneSSE: true, MaxRetries: 0}
	client := mcp.NewClient(&mcp.Implementation{Name: "openexec", Title: "OpenExec", Version: "phase0"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("connect external MCP: %w", err)
	}
	defer session.Close()
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("discover external tools: %w", err)
	}
	tools := append([]*mcp.Tool(nil), listed.Tools...)
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	toolsJSON, err := json.Marshal(tools)
	if err != nil {
		return fmt.Errorf("encode external catalog: %w", err)
	}
	digest := sha256.Sum256(toolsJSON)
	allowed := intersection(binding.AllowedTools, toolNames(tools))
	if !slices.Contains(allowed, "get_me") {
		return errors.New("connection cannot become healthy: bound safe identity tool get_me is unavailable")
	}
	started := s.now().UTC()
	invocation := state.ExternalInvocation{ID: "external_invocation_" + uuid.NewString(), ConnectionID: connection.ID,
		ProjectRef: binding.ProjectRef, ToolName: "get_me", CatalogDigest: hex.EncodeToString(digest[:]), Effect: EffectRead,
		Status: "running", StartedAt: started}
	result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_me", Arguments: map[string]any{}})
	completed := s.now().UTC()
	invocation.CompletedAt = &completed
	if callErr != nil || result.IsError {
		invocation.Status = "failed"
		if callErr != nil {
			invocation.ErrorMessage = callErr.Error()
		} else {
			invocation.ErrorMessage = "get_me returned an MCP tool error"
		}
		_ = s.store.RecordExternalInvocation(context.Background(), invocation)
		return errors.New(invocation.ErrorMessage)
	}
	invocation.Status = "succeeded"
	if err := s.store.RecordExternalInvocation(context.Background(), invocation); err != nil {
		return err
	}
	identity, err := json.Marshal(result)
	if err != nil {
		return err
	}
	initialize := session.InitializeResult()
	snapshot := state.ExternalCatalogSnapshot{ID: "external_catalog_" + uuid.NewString(), ConnectionID: connection.ID,
		Digest: hex.EncodeToString(digest[:]), Tools: toolsJSON, CreatedAt: completed}
	if initialize != nil {
		snapshot.ProtocolVersion = initialize.ProtocolVersion
		if initialize.ServerInfo != nil {
			snapshot.ServerName, snapshot.ServerVersion = initialize.ServerInfo.Name, initialize.ServerInfo.Version
		}
	}
	err = s.store.RecordExternalCatalog(context.Background(), connection.ID, snapshot, identity)
	if errors.Is(err, state.ErrExternalConnectionDisabled) {
		return ErrDisabled
	}
	return err
}

func (s *Service) authorizedConnection(ctx context.Context, connectionID, projectRef string, requireEnabled bool) (state.ExternalConnection, state.ExternalConnectionBinding, error) {
	connection, err := s.store.GetExternalConnection(ctx, strings.TrimSpace(connectionID))
	if err != nil {
		return state.ExternalConnection{}, state.ExternalConnectionBinding{}, err
	}
	binding, err := s.store.GetExternalConnectionBinding(ctx, connection.ID, strings.TrimSpace(projectRef))
	if errors.Is(err, state.ErrExternalConnectionNotFound) {
		return state.ExternalConnection{}, state.ExternalConnectionBinding{}, ErrProjectNotBound
	}
	if err != nil {
		return state.ExternalConnection{}, state.ExternalConnectionBinding{}, err
	}
	if requireEnabled && connection.Status == StatusDisabled {
		return state.ExternalConnection{}, state.ExternalConnectionBinding{}, ErrDisabled
	}
	return connection, binding, nil
}

func (s *Service) saveCredential(ctx context.Context, connectionID string, envelope credentialEnvelope) error {
	sealed, err := s.vault.seal(envelope)
	if err != nil {
		return err
	}
	return s.store.UpdateExternalConnectionCredential(ctx, connectionID, sealed, s.now().UTC())
}

func (s *Service) pruneFlowsLocked() {
	now := s.now().UTC()
	for key, flow := range s.flows {
		if !flow.expiresAt.After(now) {
			flow.cancel()
			delete(s.flows, key)
		}
	}
}

func (s *Service) cancelFlowsForConnection(connectionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for stateValue, flow := range s.flows {
		if flow.connectionID != connectionID {
			continue
		}
		flow.cancel()
		delete(s.flows, stateValue)
	}
}

func normalizeAllowedTools(provider string, requested []string) []string {
	set := map[string]struct{}{}
	if provider == "lovable" && len(requested) == 0 {
		for name := range lovableReadTools {
			set[name] = struct{}{}
		}
	}
	for _, name := range requested {
		name = strings.TrimSpace(name)
		if name == "" || EffectForTool(provider, name) != EffectRead {
			continue
		}
		set[name] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for name := range set {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func EffectForTool(provider, tool string) string {
	if strings.EqualFold(provider, "lovable") {
		if _, ok := lovableReadTools[tool]; ok {
			return EffectRead
		}
	}
	return "consequential"
}

func toolNames(tools []*mcp.Tool) []string {
	result := make([]string, 0, len(tools))
	for _, tool := range tools {
		result = append(result, tool.Name)
	}
	return result
}

func intersection(left, right []string) []string {
	set := map[string]struct{}{}
	for _, value := range right {
		set[value] = struct{}{}
	}
	var result []string
	for _, value := range left {
		if _, ok := set[value]; ok {
			result = append(result, value)
		}
	}
	return result
}

type persistingTokenSource struct {
	mu     sync.Mutex
	source oauth2.TokenSource
	save   func(*oauth2.Token) error
	last   string
}

func (s *persistingTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, err := s.source.Token()
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(token)
	digest := sha256.Sum256(raw)
	next := hex.EncodeToString(digest[:])
	if next != s.last {
		if err := s.save(token); err != nil {
			return nil, err
		}
		s.last = next
	}
	return token, nil
}

type credentialVault struct{ aead cipher.AEAD }

func newCredentialVault(encoded string) (*credentialVault, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, ErrCredentialKeyNeeded
	}
	key, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		key, err = base64.StdEncoding.DecodeString(encoded)
	}
	if err != nil || len(key) != 32 {
		return nil, errors.New("external credential encryption key must be a base64-encoded 32-byte key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &credentialVault{aead: aead}, nil
}

func (v *credentialVault) seal(envelope credentialEnvelope) ([]byte, error) {
	plain, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return v.aead.Seal(nonce, nonce, plain, []byte("openexec-external-credential-v1")), nil
}

func (v *credentialVault) open(ciphertext []byte) (credentialEnvelope, error) {
	if len(ciphertext) < v.aead.NonceSize() {
		return credentialEnvelope{}, errors.New("external credential is missing or corrupt")
	}
	nonce, body := ciphertext[:v.aead.NonceSize()], ciphertext[v.aead.NonceSize():]
	plain, err := v.aead.Open(nil, nonce, body, []byte("openexec-external-credential-v1"))
	if err != nil {
		return credentialEnvelope{}, errors.New("external credential could not be decrypted")
	}
	var envelope credentialEnvelope
	if err := json.Unmarshal(plain, &envelope); err != nil {
		return credentialEnvelope{}, errors.New("external credential is invalid")
	}
	return envelope, nil
}

func validateServerURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("server_url must be an absolute HTTPS URL without credentials or a fragment")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func validateRedirectURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, errors.New("redirect_url must be an absolute HTTPS URL")
	}
	return parsed, nil
}

func validateClientMetadataURL(raw string, redirect *url.URL) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" || parsed.Path == "" || parsed.Path == "/" {
		return nil, errors.New("client_metadata_url must be an absolute non-root HTTPS URL without credentials, query, or fragment")
	}
	if redirect == nil || !strings.EqualFold(parsed.Scheme, redirect.Scheme) || !strings.EqualFold(parsed.Host, redirect.Host) {
		return nil, errors.New("client_metadata_url must use the redirect_url origin")
	}
	return parsed, nil
}

func safeHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, address := range addresses {
			if !publicIP(address.IP) {
				return nil, fmt.Errorf("external capability host %s resolved to a non-public address", host)
			}
		}
		var dialErr error
		for _, resolved := range addresses {
			connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(resolved.IP.String(), port))
			if err == nil {
				return connection, nil
			}
			dialErr = err
		}
		return nil, fmt.Errorf("dial external capability host %s: %w", host, dialErr)
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Scheme != "https" {
			return http.ErrUseLastResponse
		}
		return nil
	}
	return client
}

func publicIP(ip net.IP) bool {
	return ip != nil && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() && !ip.IsUnspecified() && !ip.IsMulticast()
}
