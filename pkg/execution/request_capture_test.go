package execution

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

func readCapture(t *testing.T, path string) map[string]any {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("no capture written: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("capture mode = %v, want 0600", info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var capture map[string]any
	if err := json.Unmarshal(raw, &capture); err != nil {
		t.Fatal(err)
	}
	return capture
}

func compatible(t *testing.T, url string) *agent.OpenAIProvider {
	t.Helper()
	provider, err := agent.NewOpenAIProvider(agent.OpenAIProviderConfig{
		Name: "fake-endpoint", APIKey: "test-key-not-in-the-body", BaseURL: url, Models: []string{"test-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

// A request the endpoint refuses is kept, private, bounded and without
// secrets, beside the error it got; a run that succeeds or never sent a
// request leaves nothing, and without the variable nothing is written.
func TestAPIProviderCapturesTheFailedRequest(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"code":500,"message":"Error: synthetic refusal of request 3fa2c9d1","type":"server_error"}}`)
	}))
	defer failing.Close()
	path := filepath.Join(t.TempDir(), "captures", "run1.json")
	t.Setenv(RequestCaptureEnv, path)
	provider, _ := NewAPIProvider(APIProviderConfig{Adapter: compatible(t, failing.URL)})
	prompt := "please use sk-abcdefghijklmnopqrstuvwx and token=supersecretvalue1 " + strings.Repeat("long evidence ", 400)
	result, err := provider.Execute(context.Background(), Request{
		ID: "capture-1", WorkingDir: t.TempDir(), Prompt: prompt, Model: "test-model", System: "standing context",
		Sandbox: Sandbox{Mode: "read-only"},
	}, nil)
	if err == nil || result.Outcome != OutcomeFailed {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	capture := readCapture(t, path)
	raw, _ := json.Marshal(capture)
	text := string(raw)
	if capture["baseURL"] != failing.URL || capture["outcome"] != OutcomeFailed || !strings.Contains(capture["error"].(string), "synthetic refusal") ||
		capture["truncated"] != true {
		t.Fatalf("capture header = %v", capture)
	}
	if strings.Contains(text, "sk-abcdefghijklmnopqrstuvwx") || strings.Contains(text, "supersecretvalue1") || strings.Contains(text, "test-key-not-in-the-body") {
		t.Fatalf("a secret reached the capture: %s", text)
	}
	request := capture["request"].(map[string]any)
	messages := request["messages"].([]any)
	if request["model"] != "test-model" || len(messages) == 0 {
		t.Fatalf("request shape lost: %v", request)
	}
	last := messages[len(messages)-1].(map[string]any)
	if last["role"] != "user" || !strings.Contains(last["content"].(string), "characters cut") || len([]rune(last["content"].(string))) > captureTextLimit+40 {
		t.Fatalf("message not kept with its role and bounded: %v", last)
	}

	succeeding := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"1\",\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer succeeding.Close()
	quiet := filepath.Join(t.TempDir(), "run2.json")
	t.Setenv(RequestCaptureEnv, quiet)
	ok, _ := NewAPIProvider(APIProviderConfig{Adapter: compatible(t, succeeding.URL)})
	if result, err := ok.Execute(context.Background(), Request{ID: "ok", WorkingDir: t.TempDir(), Prompt: "hi", Model: "test-model", Sandbox: Sandbox{Mode: "read-only"}}, nil); err != nil || result.Outcome != OutcomeSucceeded {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	refused, _ := NewAPIProvider(APIProviderConfig{Adapter: compatible(t, failing.URL)})
	if _, err := refused.Execute(context.Background(), Request{ID: "bad", WorkingDir: t.TempDir(), Prompt: "hi", Model: "test-model",
		Sandbox: Sandbox{Mode: "read-only"}, NativeSessionID: "not-for-api"}, nil); err == nil {
		t.Fatal("a refused request ran")
	}
	if _, err := os.Stat(quiet); !os.IsNotExist(err) {
		t.Fatalf("a successful or refused run left a capture: %v", err)
	}
	t.Setenv(RequestCaptureEnv, "")
	unset := filepath.Join(t.TempDir(), "run3.json")
	again, _ := NewAPIProvider(APIProviderConfig{Adapter: compatible(t, failing.URL)})
	_, _ = again.Execute(context.Background(), Request{ID: "x", WorkingDir: t.TempDir(), Prompt: "hi", Model: "test-model", Sandbox: Sandbox{Mode: "read-only"}}, nil)
	if _, err := os.Stat(unset); !os.IsNotExist(err) {
		t.Fatal("a capture was written without the variable")
	}
}

// A run that ends at its tool-round cap keeps its last request (the final
// synthesis, with the conversation's roles and tool calls) and says why it
// ended.
func TestAPIProviderCapturesTheLastRequestAtTheCap(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			_, _ = io.WriteString(w, `{"id":"1","model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call-1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"2","model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "cap.json")
	t.Setenv(RequestCaptureEnv, path)
	provider, _ := NewAPIProvider(APIProviderConfig{
		Adapter: compatible(t, server.URL), ToolExecutor: &recordingToolExecutor{}, MaxSteps: 1,
		Tools: []agent.ToolDefinition{{Name: "read_file", Description: "Read a file", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)}},
	})
	result, err := provider.Execute(context.Background(), Request{ID: "cap", WorkingDir: t.TempDir(), Prompt: "analyze", Model: "test-model", Sandbox: Sandbox{Mode: "read-only"}}, nil)
	if err != nil || result.Outcome != OutcomeInconclusive || result.Reason != ReasonMaxTurns {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	capture := readCapture(t, path)
	request := capture["request"].(map[string]any)
	var roles []string
	for _, message := range request["messages"].([]any) {
		roles = append(roles, message.(map[string]any)["role"].(string))
	}
	if capture["outcome"] != OutcomeInconclusive || capture["reason"] != ReasonMaxTurns || capture["error"] != "inconclusive: max_turns" ||
		strings.Join(roles, ",") != "system,user,assistant,tool,user" || !strings.Contains(request["messages"].([]any)[0].(map[string]any)["content"].(string), "tool-call budget") {
		t.Fatalf("cap capture = %v", capture)
	}
}
