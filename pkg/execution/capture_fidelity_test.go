package execution

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/agent"
)

// These public controls drive the production serializers and capture writer.
// The local server models the reported string-arguments parser. It is NOT the
// historical configured endpoint and cannot reproduce its two-hour tool loop.
func fidelityRequest(text string) agent.Request {
	args, _ := json.Marshal(map[string]string{"value": text})
	return agent.Request{Model: "synthetic-model", Messages: []agent.Message{
		agent.NewTextMessage(agent.RoleUser, "Return the public value."),
		{Role: agent.RoleAssistant, Content: []agent.ContentBlock{{Type: agent.ContentTypeToolUse,
			ToolUseID: "public-call", ToolName: "public_echo", ToolInput: args}}},
		agent.NewToolResultMessage("public-call", "public result", nil),
	}, Tools: []agent.ToolDefinition{{Name: "public_echo", Description: "Public synthetic echo",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}}}`)}}}
}

func fidelityAdapter(t *testing.T, base string) *agent.OpenAIProvider {
	t.Helper()
	adapter, err := agent.NewOpenAIProvider(agent.OpenAIProviderConfig{
		Name: "public-synthetic", APIKey: "public-test-key", BaseURL: base, Models: []string{"synthetic-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

type fidelityDispatch struct {
	path string
	body []byte
}

func fidelityEndpoint(t *testing.T) (*httptest.Server, <-chan fidelityDispatch) {
	t.Helper()
	dispatches := make(chan fidelityDispatch, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			fmt.Fprint(w, `{"version":"0.32.13"}`)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		dispatches <- fidelityDispatch{r.URL.Path, body}
		if r.URL.Path == "/api/chat" {
			// Native arguments are objects, as emitted by completeLocalOllama.
			var req struct {
				Messages []struct {
					Calls []struct {
						Function struct {
							Arguments map[string]any `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"messages"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, `{"done":true,"done_reason":"stop","prompt_eval_count":10,"eval_count":2,"message":{"content":"public ok"}}`)
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		// This is a schema control, not a copy of the unavailable server parser.
		var req struct {
			Stream   bool `json:"stream"`
			Messages []struct {
				Calls []struct {
					Function struct {
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"public ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"public ok"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(srv.Close)
	return srv, dispatches
}

func fidelityTree(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var tree map[string]any
	if err := json.Unmarshal(data, &tree); err != nil {
		t.Fatal(err)
	}
	return tree
}

func fidelityArguments(tree map[string]any) any {
	m := asMap(asList(tree["messages"])[1])
	return asMap(asMap(asList(m["tool_calls"])[0])["function"])["arguments"]
}

func TestCaptureFidelitySerializerDispatchAndReplay(t *testing.T) {
	for _, mode := range []string{"compatible", "native", "bounded", "negotiated-stream", "forced-choice"} {
		t.Run(mode, func(t *testing.T) {
			srv, dispatches := fidelityEndpoint(t)
			adapter := fidelityAdapter(t, srv.URL+"/v1")
			if mode != "compatible" && !adapter.EnableLocalOllamaBounds(context.Background()) {
				t.Fatal("public native negotiation failed")
			}
			req := fidelityRequest("public value")
			if mode == "forced-choice" {
				req.ToolChoice = "any"
			}
			started := time.Now()
			var err error
			switch mode {
			case "bounded":
				_, err = adapter.CompleteBounded(context.Background(), req, 2048, 16)
			case "negotiated-stream":
				var events <-chan agent.StreamEvent
				events, err = adapter.Stream(context.Background(), req)
				if err == nil {
					for event := range events {
						if event.Error != nil {
							t.Fatalf("stream: %v", event.Error)
						}
					}
				}
			default:
				_, err = adapter.Complete(context.Background(), req)
			}
			if err != nil {
				t.Fatal(err)
			}
			dispatched := <-dispatches
			body, at, base := adapter.LastRequest()
			if !bytes.Equal(body, dispatched.body) || at.Before(started) || base != srv.URL+"/v1" {
				t.Fatal("LastRequest did not remember dispatched bytes, time and configured base")
			}
			native := mode == "native" || mode == "bounded"
			wantPath := "/v1/chat/completions"
			if native {
				wantPath = "/api/chat"
			}
			if dispatched.path != wantPath {
				t.Fatalf("route=%s, want %s", dispatched.path, wantPath)
			}
			tree := fidelityTree(t, body)
			_, object := fidelityArguments(tree).(map[string]any)
			if object != native {
				t.Fatalf("argument type=%T native=%t", fidelityArguments(tree), native)
			}
			if !native {
				args, ok := fidelityArguments(tree).(string)
				if !ok || !json.Valid([]byte(args)) {
					t.Fatal("compatible arguments are not a valid JSON string")
				}
			}
			if mode == "native" || mode == "compatible" {
				golden, err := os.ReadFile(filepath.Join("testdata", "capture-fidelity-"+mode+".json"))
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(tree, asMap(fidelityTree(t, golden)["request"])) {
					t.Fatal("public capture control differs from actual serializer")
				}
			}
			path := filepath.Join(t.TempDir(), "capture.json")
			t.Setenv(RequestCaptureEnv, path)
			captureRequest(adapter, started, Result{Outcome: OutcomeFailed}, context.Canceled)
			capture := readCapture(t, path)
			if capture["truncated"] != false || capture["error"] != "context canceled" ||
				!reflect.DeepEqual(tree, capture["request"]) || capture["baseURL"] != base {
				t.Fatal("capture changed public types or recorded a dispatched route it does not know")
			}
			tracePath := filepath.Join(t.TempDir(), "trace", "run.json")
			t.Setenv(RequestTraceEnv, tracePath)
			trace := newRunTrace()
			trace.record(adapter, started, req, nil, context.Canceled)
			trace.write(adapter, Request{ID: "public"}, Result{Outcome: OutcomeFailed}, context.Canceled)
			persisted := readTrace(t, tracePath)
			if !reflect.DeepEqual(asMap(asList(persisted["entries"])[0])["wire"], tree) ||
				persisted["redactionChangedContent"] != false || persisted["imagesOmitted"] != false || persisted["incomplete"] != false {
				t.Fatal("public trace tree/fidelity flags did not round trip")
			}
			// Replay the persisted tree without converting any arguments. Only the
			// destination is varied, exposing the base-URL/route ambiguity.
			replay, err := json.Marshal(capture["request"])
			if err != nil {
				t.Fatal(err)
			}
			res, err := srv.Client().Post(base+"/chat/completions", "application/json", bytes.NewReader(replay))
			if err != nil {
				t.Fatal(err)
			}
			response, _ := io.ReadAll(res.Body)
			res.Body.Close()
			<-dispatches
			if native {
				if res.StatusCode != 400 || !strings.Contains(string(response), "cannot unmarshal object") ||
					!strings.Contains(string(response), "tool_calls.function.arguments of type string") {
					t.Fatalf("local parser control: status=%d body=%s", res.StatusCode, response)
				}
				res, err = srv.Client().Post(srv.URL+"/api/chat", "application/json", bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				res.Body.Close()
				<-dispatches
				if res.StatusCode != 200 {
					t.Fatal("unmodified native bytes refused at their actual synthetic route")
				}
			} else if res.StatusCode != 200 {
				t.Fatalf("string-argument near miss was refused: %d", res.StatusCode)
			}
			t.Logf("public-only: mode=%s route=%s wireSHA256=%x replaySHA256=%x argumentType=%T", mode, dispatched.path,
				sha256.Sum256(body), sha256.Sum256(replay), fidelityArguments(tree))
		})
	}
}

func TestCaptureFidelityTruncationPreservesTypesButLosesContent(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
			srv, dispatches := fidelityEndpoint(t)
			adapter := fidelityAdapter(t, srv.URL+"/v1")
			if native && !adapter.EnableLocalOllamaBounds(context.Background()) {
				t.Fatal("negotiation failed")
			}
			started := time.Now()
			_, err := adapter.Complete(context.Background(), fidelityRequest(strings.Repeat("x", captureTextLimit+1000)))
			if err != nil {
				t.Fatal(err)
			}
			wire := <-dispatches
			path := filepath.Join(t.TempDir(), "capture.json")
			t.Setenv(RequestCaptureEnv, path)
			captureRequest(adapter, started, Result{Outcome: OutcomeFailed}, context.Canceled)
			capture := readCapture(t, path)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if capture["truncated"] != true || capture["textLimit"] != float64(captureTextLimit) ||
				!strings.Contains(string(raw), "characters cut") || reflect.DeepEqual(capture["request"], fidelityTree(t, wire.body)) {
				t.Fatal("capture did not expose content loss")
			}
			args := fidelityArguments(asMap(capture["request"]))
			if native {
				object, ok := args.(map[string]any)
				if !ok || len(asString(object["value"])) >= captureTextLimit+1000 {
					t.Fatal("native object leaf was not truncated")
				}
			} else if text, ok := args.(string); !ok || !json.Valid([]byte(text)) {
				t.Fatal("cutting inside this string value should retain valid inner JSON despite content loss")
			}
		})
	}
}

func TestCaptureFidelityTruncationCanBreakCompatibleArgumentJSON(t *testing.T) {
	srv, dispatches := fidelityEndpoint(t)
	adapter := fidelityAdapter(t, srv.URL+"/v1")
	req := fidelityRequest("")
	req.Tools[0].InputSchema = json.RawMessage(`{"type":"object","properties":{"values":{"type":"array","items":{"type":"integer"}}}}`)
	values := make([]int, 2000)
	for i := range values {
		values[i] = i
	}
	args, err := json.Marshal(map[string]any{"values": values})
	if err != nil {
		t.Fatal(err)
	}
	req.Messages[1].Content[0].ToolInput = args
	started := time.Now()
	if _, err := adapter.Complete(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	wire := <-dispatches
	if !json.Valid([]byte(asString(fidelityArguments(fidelityTree(t, wire.body))))) {
		t.Fatal("original compatible arguments were already invalid")
	}
	path := filepath.Join(t.TempDir(), "capture.json")
	t.Setenv(RequestCaptureEnv, path)
	captureRequest(adapter, started, Result{Outcome: OutcomeFailed}, context.Canceled)
	capture := readCapture(t, path)
	text, ok := fidelityArguments(asMap(capture["request"])).(string)
	if !ok || capture["truncated"] != true || json.Valid([]byte(text)) || !strings.Contains(text, "characters cut") {
		t.Fatal("expected a retained string whose numeric array is broken by the cut marker")
	}
}

func TestCaptureFidelityNativeCancellationIsNotCallerVerdict(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	cause := errors.New("synthetic caller budget cause")
	received, release := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			fmt.Fprint(w, `{"version":"0.32.13"}`)
			return
		}
		if r.URL.Path != "/api/chat" {
			t.Errorf("wrong native path: %s", r.URL.Path)
		}
		close(received)
		<-release
	}))
	defer srv.Close()
	defer close(release)
	adapter := fidelityAdapter(t, srv.URL+"/v1")
	if !adapter.EnableLocalOllamaBounds(ctx) {
		t.Fatal("negotiation failed")
	}
	path := filepath.Join(t.TempDir(), "cancel.json")
	t.Setenv(RequestCaptureEnv, path)
	provider, err := NewAPIProvider(APIProviderConfig{Adapter: adapter,
		Tools: fidelityRequest("").Tools, ToolExecutor: &recordingToolExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		select {
		case <-received:
			cancel(cause)
		case <-ctx.Done():
		}
	}()
	result, err := provider.Execute(ctx, Request{ID: "public-cancel", WorkingDir: t.TempDir(),
		Prompt: "Public cancellation control", Model: "synthetic-model", Sandbox: Sandbox{Mode: SandboxReadOnly}}, nil)
	if !errors.Is(err, context.Canceled) || context.Cause(ctx) != cause || result.Outcome != OutcomeFailed || result.Reason != "" {
		t.Fatalf("native cancellation/caller cause conflated: result=%+v err=%v cause=%v", result, err, context.Cause(ctx))
	}
	capture := readCapture(t, path)
	if capture["error"] != "context canceled" || capture["reason"] != "" || capture["outcome"] != OutcomeFailed {
		t.Fatal("capture invented a caller verdict")
	}
}
