package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openexec/openexec/pkg/agent"
)

// Gateway limits. A gateway is another process answering over a socket, and
// everything it returns is about to be handed to a model: unbounded means a
// hung caller or a context filled with someone else's payload.
const (
	gatewayTimeout         = 60 * time.Second
	gatewayMaxResponse     = 1 << 20
	gatewayMaxToolResponse = 64 << 10
)

// GatewayToolExecutor runs tools that belong to the caller, not to OpenExec.
//
// The workspace executor owns files, and its authorization is a set of roots.
// This owns nothing: it forwards a named call to a loopback endpoint the caller
// supplied for this run and returns what comes back. That inversion is the
// point — Agent Console has verbs about its own state (which provider is
// signed out, what failed last night) that OpenExec has no business
// implementing, and a model that needs them must not be given a shell to
// approximate them with.
//
// What OpenExec still owns is the boundary:
//
//   - loopback only, so a request cannot be pointed at a host on the network;
//   - no redirects, so it cannot be pointed at one afterwards;
//   - bounded responses, in both directions;
//   - only tools the gateway itself listed, by exact name;
//   - the request context, so cancelling a run cancels the call in flight.
type GatewayToolExecutor struct {
	endpoint string
	client   *http.Client
	tools    []agent.ToolDefinition
}

var _ ToolExecutor = (*GatewayToolExecutor)(nil)

// NewGatewayToolExecutor validates the endpoint and asks it what it offers.
//
// The tool list comes from the gateway rather than from configuration here:
// OpenExec must not learn what the console's verbs mean, only that these are
// the names it may forward.
func NewGatewayToolExecutor(ctx context.Context, endpoint string) (*GatewayToolExecutor, error) {
	if err := validateGatewayEndpoint(endpoint); err != nil {
		return nil, err
	}
	executor := &GatewayToolExecutor{
		endpoint: endpoint,
		client: &http.Client{
			Timeout: gatewayTimeout,
			// A gateway that answers with a redirect is not the gateway the
			// caller named. Following one is how a loopback check becomes
			// decorative.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
	tools, err := executor.list(ctx)
	if err != nil {
		return nil, err
	}
	executor.tools = tools
	return executor, nil
}

// Tools are what the gateway said it has, in the shape a model is offered.
func (e *GatewayToolExecutor) Tools() []agent.ToolDefinition {
	return append([]agent.ToolDefinition(nil), e.tools...)
}

// SupportsWorkspaceWrite is false, and ValidateAccess below is why: a gateway
// run is read-only by construction. Declaring it keeps a caller from choosing
// this provider for editing work and meeting the refusal at the turn instead
// of at the choice.
func (e *GatewayToolExecutor) SupportsWorkspaceWrite() bool { return false }

// ValidateAccess refuses the combination that would make the boundary
// meaningless. A gateway run has no writable roots and no file tools; asking
// for workspace-write alongside one is either a mistake or an attempt to have
// both, and neither should start.
func (e *GatewayToolExecutor) ValidateAccess(workingDir string, sandbox Sandbox, writableRoots []string) error {
	if sandbox.Mode != SandboxReadOnly {
		return fmt.Errorf("a tool gateway runs read-only; %q was requested", sandbox.Mode)
	}
	if len(writableRoots) > 0 {
		return errors.New("a tool gateway run cannot declare writable roots")
	}
	return nil
}

// servesGateway marks this executor as forwarding verbs to a caller-owned
// endpoint. Read through an interface rather than a type assertion, because a
// composite executor forwards some of its verbs and owns the rest, and a
// provider that decided by concrete type reported that one as having no
// gateway — a false capability in the direction that matters.
func (e *GatewayToolExecutor) servesGateway() bool { return true }

func (e *GatewayToolExecutor) ExecuteTool(ctx context.Context, request ToolRequest) (string, error) {
	if err := e.ValidateAccess(request.WorkingDir, request.Sandbox, request.WritableRoots); err != nil {
		return "", err
	}
	return e.executeAuthorized(ctx, request)
}

// executeAuthorized forwards a call whose access has already been decided.
//
// Split from ExecuteTool for the composite executor, where the standalone
// read-only rule is the wrong question: those runs are authorized as a whole
// by the workspace executor, against the sandbox and roots the caller granted.
// The name check stays here, on every path — it is what keeps a model to the
// verbs the endpoint actually offered.
func (e *GatewayToolExecutor) executeAuthorized(ctx context.Context, request ToolRequest) (string, error) {
	if !e.offers(request.Name) {
		// The model invented a name, or a gateway changed under a running
		// turn. Either way this is not one of the verbs that was authorized.
		return "", fmt.Errorf("tool %q is not offered by this gateway", request.Name)
	}
	arguments := request.Input
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	result, err := e.call(ctx, "tools/call", map[string]any{
		"name": request.Name, "arguments": json.RawMessage(arguments),
	})
	if err != nil {
		return "", err
	}
	var payload struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		return "", fmt.Errorf("gateway returned an unreadable result for %q: %w", request.Name, err)
	}
	var text strings.Builder
	for _, block := range payload.Content {
		if block.Type != "" && block.Type != "text" {
			// Named rather than dropped: a result that silently became empty
			// reads as a tool that did nothing.
			text.WriteString("[" + block.Type + "]")
			continue
		}
		text.WriteString(block.Text)
	}
	answer := text.String()
	if len(answer) > gatewayMaxToolResponse {
		answer = answer[:gatewayMaxToolResponse] + "\n\n[truncated]"
	}
	if payload.IsError {
		return answer, fmt.Errorf("gateway tool %q failed", request.Name)
	}
	return answer, nil
}

func (e *GatewayToolExecutor) offers(name string) bool {
	for _, tool := range e.tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func (e *GatewayToolExecutor) list(ctx context.Context) ([]agent.ToolDefinition, error) {
	result, err := e.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var payload struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		return nil, fmt.Errorf("gateway returned an unreadable tool list: %w", err)
	}
	tools := make([]agent.ToolDefinition, 0, len(payload.Tools))
	for _, tool := range payload.Tools {
		if strings.TrimSpace(tool.Name) == "" {
			return nil, errors.New("gateway offered a tool with no name")
		}
		schema := tool.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		if !json.Valid(schema) {
			return nil, fmt.Errorf("gateway offered tool %q with an invalid schema", tool.Name)
		}
		tools = append(tools, agent.ToolDefinition{
			Name: tool.Name, Description: tool.Description, InputSchema: schema,
		})
	}
	if len(tools) == 0 {
		return nil, errors.New("gateway offered no tools")
	}
	return tools, nil
}

func (e *GatewayToolExecutor) call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": method, "params": params,
	})
	if err != nil {
		return nil, err
	}
	// Context-bound, so cancelling the run cancels a call in flight rather
	// than leaving the model waiting on a gateway nobody is listening to.
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := e.client.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("tool gateway: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tool gateway answered %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, gatewayMaxResponse))
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("tool gateway returned unreadable JSON: %w", err)
	}
	if envelope.Error != nil {
		return nil, fmt.Errorf("tool gateway: %s", envelope.Error.Message)
	}
	return envelope.Result, nil
}

// validateGatewayEndpoint refuses anything that is not a plain loopback HTTP
// URL.
//
// The endpoint arrives on the request, which means it arrives from whatever
// launched OpenExec. Checking it here is not distrust of Agent Console; it is
// the difference between a boundary and a convention, and the cost of a
// mistake is a model given an arbitrary HTTP client.
func validateGatewayEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("tool gateway endpoint is not a URL: %w", err)
	}
	if parsed.Scheme != "http" {
		return fmt.Errorf("tool gateway endpoint must be http on loopback, got scheme %q", parsed.Scheme)
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return nil
	}
	address := net.ParseIP(host)
	if address == nil || !address.IsLoopback() {
		return fmt.Errorf("tool gateway endpoint %q is not on loopback", host)
	}
	return nil
}
