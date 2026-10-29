package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/agent"
)

func dispatchTree(t *testing.T, d *agent.RequestDispatch) map[string]any {
	t.Helper()
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if d == nil {
		return nil
	}
	return fidelityTree(t, raw)
}

func assertDispatch(t *testing.T, got map[string]any, method, target string) {
	t.Helper()
	want := map[string]any{"method": method, "url": target, "provenance": agent.DispatchSourceKnown}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dispatch provenance mismatch: got %v want %v", got, want)
	}
}

// Two real model rounds: a tool call, then a refusal after the tool result.
// Both the final capture and each bounded trace entry must describe the body
// and target of that same round, through Execute, including its budget wrapper.
func TestDispatchProvenanceExecuteRoundTrip(t *testing.T) {
	for _, mode := range []string{"native", "bounded", "compatible"} {
		t.Run(mode, func(t *testing.T) {
			receivedRequests := make(chan fidelityDispatch, 8)
			var rounds atomic.Int32
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
				receivedRequests <- fidelityDispatch{path: r.URL.Path, method: r.Method, body: body}
				if rounds.Add(1) > 1 {
					w.WriteHeader(500)
					fmt.Fprint(w, `{"error":{"message":"public refusal"}}`)
					return
				}
				if r.URL.Path == "/api/chat" {
					fmt.Fprint(w, `{"done":true,"prompt_eval_count":10,"eval_count":2,"message":{"tool_calls":[{"function":{"name":"public_echo","arguments":{"value":"public value"}}}]}}`)
				} else {
					fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"public-call","type":"function","function":{"name":"public_echo","arguments":"{\"value\":\"public value\"}"}}]},"finish_reason":"tool_calls"}]}`)
				}
			}))
			defer srv.Close()
			adapter := fidelityAdapter(t, srv.URL+"/v1")
			if mode != "compatible" && !adapter.EnableLocalOllamaBounds(context.Background()) {
				t.Fatal("negotiation failed")
			}
			dir := t.TempDir()
			capturePath, tracePath := filepath.Join(dir, "capture.json"), filepath.Join(dir, "trace", "run.json")
			t.Setenv(RequestCaptureEnv, capturePath)
			t.Setenv(RequestTraceEnv, tracePath)
			provider, err := NewAPIProvider(APIProviderConfig{Adapter: adapter, Tools: fidelityRequest("").Tools, ToolExecutor: &fixtureTools{}, MaxSteps: 4})
			if err != nil {
				t.Fatal(err)
			}
			request := Request{ID: "public-provenance", Prompt: "Public request", Model: "synthetic-model", WorkingDir: t.TempDir(), Sandbox: Sandbox{Mode: SandboxReadOnly}}
			if mode == "bounded" {
				request.TokenBudget = 32768
			}
			result, err := provider.Execute(context.Background(), request, nil)
			var received []fidelityDispatch
			for len(receivedRequests) > 0 {
				received = append(received, <-receivedRequests)
			}
			if err == nil || result.Outcome != OutcomeFailed || len(received) != 2 {
				t.Fatalf("expected two-round failure: result=%+v err=%v rounds=%d", result, err, len(received))
			}
			capture, trace := readCapture(t, capturePath), readTrace(t, tracePath)
			entries := asList(trace["entries"])
			if len(entries) != 2 {
				t.Fatalf("trace rounds=%d", len(entries))
			}
			wantPath := "/api/chat"
			if mode == "compatible" {
				wantPath = "/v1/chat/completions"
			}
			for i, observed := range received {
				if observed.path != wantPath || observed.method != "POST" {
					t.Fatal("wrong actual serializer route")
				}
				entry := asMap(entries[i])
				assertDispatch(t, asMap(entry["dispatch"]), observed.method, srv.URL+observed.path)
				clean, _ := sanitizeCapture(fidelityTree(t, observed.body), int(^uint(0)>>1))
				if !reflect.DeepEqual(entry["wire"], clean) {
					t.Fatalf("trace body does not belong to round %d", i)
				}
			}
			last := received[1]
			assertDispatch(t, asMap(capture["dispatch"]), last.method, srv.URL+last.path)
			clean, truncated := sanitizeCapture(fidelityTree(t, last.body), captureTextLimit)
			if !truncated || capture["truncated"] != true || !reflect.DeepEqual(capture["request"], clean) {
				t.Fatal("capture body/bounds did not round trip")
			}
			// Locate the tool continuation after Execute's system/user turns.
			found := false
			for _, value := range asList(asMap(capture["request"])["messages"]) {
				calls := asList(asMap(value)["tool_calls"])
				if len(calls) == 0 {
					continue
				}
				args := asMap(asMap(calls[0])["function"])["arguments"]
				_, object := args.(map[string]any)
				if object != (mode != "compatible") {
					t.Fatal("capture coerced tool argument type")
				}
				if !object {
					if s, ok := args.(string); !ok || !json.Valid([]byte(s)) {
						t.Fatal("compatible string arguments lost")
					}
				}
				found = true
			}
			if !found {
				t.Fatal("no real tool continuation captured")
			}
			if trace["redactionChangedContent"] != true || trace["imagesOmitted"] != false || trace["incomplete"] != false || trace["droppedEntries"] != float64(0) {
				t.Fatal("trace fidelity flags changed")
			}
			for _, path := range []string{capturePath, tracePath} {
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				for _, leak := range []string{fixtureSecret, "public-test-key", "Authorization"} {
					if bytes.Contains(raw, []byte(leak)) {
						t.Fatal("synthetic credential reached persisted diagnostics")
					}
				}
			}
		})
	}
}

// Switching serializers on the SAME provider must not stamp the last route on
// earlier trace entries, or mismatch body and target across the old/new APIs.
func TestDispatchProvenanceTraceBindsEachRound(t *testing.T) {
	srv, observed := fidelityEndpoint(t)
	adapter := fidelityAdapter(t, srv.URL+"/v1")
	if !adapter.EnableLocalOllamaBounds(context.Background()) {
		t.Fatal("negotiation failed")
	}
	path := filepath.Join(t.TempDir(), "trace", "run.json")
	t.Setenv(RequestTraceEnv, path)
	trace := newRunTrace()
	for _, choice := range []string{"auto", "any"} {
		req := fidelityRequest("public " + choice)
		req.ToolChoice = choice
		started := time.Now()
		response, err := adapter.Complete(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		got := <-observed
		trace.record(&boundedAPIAdapter{ProviderAdapter: adapter}, started, req, response, nil)
		entry := trace.entries[len(trace.entries)-1]
		raw, _ := json.Marshal(entry)
		assertDispatch(t, asMap(fidelityTree(t, raw)["dispatch"]), got.method, srv.URL+got.path)
	}
	trace.write(adapter, Request{ID: "public"}, Result{Outcome: OutcomeFailed}, context.Canceled)
	persisted := readTrace(t, path)
	entries := asList(persisted["entries"])
	assertDispatch(t, asMap(asMap(entries[0])["dispatch"]), "POST", srv.URL+"/api/chat")
	assertDispatch(t, asMap(asMap(entries[1])["dispatch"]), "POST", srv.URL+"/v1/chat/completions")
	if _, ok := fidelityArguments(asMap(asMap(entries[0])["wire"])).(map[string]any); !ok {
		t.Fatal("native object changed")
	}
	if _, ok := fidelityArguments(asMap(asMap(entries[1])["wire"])).(string); !ok {
		t.Fatal("compatible string changed")
	}
}

type legacyDispatchAdapter struct {
	agent.ProviderAdapter
	body []byte
	at   time.Time
	base string
}

func (a *legacyDispatchAdapter) LastRequest() ([]byte, time.Time, string) {
	return a.body, a.at, a.base
}

type suppliedDispatchAdapter struct {
	*legacyDispatchAdapter
	dispatch *agent.RequestDispatch
}

func (a *suppliedDispatchAdapter) LastRequestWithDispatch() ([]byte, time.Time, string, *agent.RequestDispatch) {
	return a.body, a.at, a.base, a.dispatch
}

// Exercise the actual persistence boundary, including legacy providers that
// cannot implement the optional interface. Nothing here chooses a replay route.
func TestDispatchProvenanceUnknownAndPrivacy(t *testing.T) {
	valid := agent.RequestDispatch{Method: "POST", URL: "https://public.invalid/api/chat", Provenance: agent.DispatchSourceKnown}
	for _, name := range []string{"known", "legacy", "absent", "method", "provenance", "relative", "missing-host", "scheme", "malformed", "userinfo", "query", "empty-query", "fragment", "secret-path", "escaped-secret", "invalid-base", "stale", "empty-body", "null-body", "invalid-body"} {
		t.Run(name, func(t *testing.T) {
			started := time.Now()
			legacy := &legacyDispatchAdapter{ProviderAdapter: &fakeAPIAdapter{}, body: []byte(`{"messages":[{"role":"user","content":"public text"}]}`), at: started, base: "https://public.invalid/v1"}
			d := valid
			source := &suppliedDispatchAdapter{legacyDispatchAdapter: legacy, dispatch: &d}
			var adapter agent.ProviderAdapter = source
			switch name {
			case "legacy":
				adapter = legacy
			case "absent":
				source.dispatch = nil
			case "method":
				d.Method = "GET"
			case "provenance":
				d.Provenance = "inferred-from-arguments"
			case "relative":
				d.URL = "/api/chat"
			case "missing-host":
				d.URL = "https:///api/chat"
			case "scheme":
				d.URL = "file:///api/chat"
			case "malformed":
				d.URL = "http://%zz"
			case "userinfo":
				d.URL = "https://synthetic-user:arbitrary-password@public.invalid/api/chat"
				legacy.base = "https://synthetic-user:arbitrary-password@public.invalid/v1"
			case "query":
				d.URL = "https://public.invalid/api/chat?arbitrary=hidden-value"
				legacy.base = "https://public.invalid/v1?arbitrary=hidden-value"
			case "empty-query":
				d.URL = "https://public.invalid/api/chat?"
			case "fragment":
				d.URL = "https://public.invalid/api/chat#hidden-fragment"
				legacy.base = "https://public.invalid/v1#hidden-fragment"
			case "secret-path":
				d.URL = "https://public.invalid/sk-abcdefghijklmnopqrstuvwx"
				legacy.base = d.URL
			case "escaped-secret":
				d.URL = "https://public.invalid/sk%2Dabcdefghijklmnopqrstuvwx"
				legacy.base = d.URL
			case "invalid-base":
				legacy.base = "http://%zz"
			case "stale":
				legacy.at = started.Add(-time.Second)
			case "empty-body":
				legacy.body = nil
			case "null-body":
				legacy.body = []byte("null")
			case "invalid-body":
				legacy.body = []byte(`{"broken"`)
			}
			dir := t.TempDir()
			capturePath, tracePath := filepath.Join(dir, "capture.json"), filepath.Join(dir, "trace", "run.json")
			t.Setenv(RequestCaptureEnv, capturePath)
			t.Setenv(RequestTraceEnv, tracePath)
			captureRequest(adapter, started, Result{Outcome: OutcomeFailed}, context.Canceled)
			trace := newRunTrace()
			trace.record(adapter, started, agent.Request{}, nil, context.Canceled)
			trace.write(adapter, Request{}, Result{Outcome: OutcomeFailed}, context.Canceled)
			entry := asMap(asList(readTrace(t, tracePath)["entries"])[0])
			if name == "stale" || name == "invalid-body" || name == "empty-body" || name == "null-body" {
				if _, err := os.Stat(capturePath); !os.IsNotExist(err) {
					t.Fatal("unsent or malformed body acquired a capture")
				}
				if entry["wire"] != nil || entry["dispatch"] != nil {
					t.Fatal("unsent or malformed body acquired wire evidence")
				}
				return
			}
			capture := readCapture(t, capturePath)
			if name == "invalid-base" && capture["baseURL"] != "<unavailable>" {
				t.Fatal("malformed configured base was not withheld")
			}
			if name == "known" || name == "invalid-base" {
				assertDispatch(t, asMap(capture["dispatch"]), "POST", valid.URL)
				assertDispatch(t, asMap(entry["dispatch"]), "POST", valid.URL)
			} else if capture["dispatch"] != nil || entry["dispatch"] != nil {
				t.Fatal("unknown or privacy-altered target acquired invented provenance")
			}
			for _, path := range []string{capturePath, tracePath} {
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				for _, leak := range []string{"synthetic-user", "arbitrary-password", "hidden-value", "hidden-fragment", "sk-abcdefghijklmnopqrstuvwx", "sk%2Dabcdefghijklmnopqrstuvwx"} {
					if bytes.Contains(raw, []byte(leak)) {
						t.Fatal("URL secret reached persisted diagnostics")
					}
				}
			}
		})
	}
	// Checked-in legacy documents are read as-is; object shape is not provenance.
	for _, name := range []string{"native", "compatible"} {
		raw, err := os.ReadFile(filepath.Join("testdata", "capture-fidelity-"+name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		if fidelityTree(t, raw)["dispatch"] != nil {
			t.Fatal("historical fixture was rewritten")
		}
	}
}

// This negative control is part of the declared Go suite, not an external
// reproducer. Only the real provider's provenance assignment is removed in a
// disposable Go overlay. Bodies, dispatch, capture and trace code stay real.
func TestDispatchProvenanceRemovalControl(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("source location unavailable")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "../.."))
	source := filepath.Join(root, "pkg", "agent", "openai_provider.go")
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	assignment := []byte("p.lastDispatch = &RequestDispatch{Method: request.Method, URL: request.URL.String(), Provenance: DispatchSourceKnown}")
	if bytes.Count(raw, assignment) != 1 {
		t.Fatal("negative control cannot locate the real propagation assignment")
	}
	dir := t.TempDir()
	replacement := filepath.Join(dir, "openai_provider.go")
	if err := os.WriteFile(replacement, bytes.Replace(raw, assignment, []byte("p.lastDispatch = nil"), 1), 0600); err != nil {
		t.Fatal(err)
	}
	overlay, _ := json.Marshal(map[string]any{"Replace": map[string]string{source: replacement}})
	overlayPath := filepath.Join(dir, "overlay.json")
	if err := os.WriteFile(overlayPath, overlay, 0600); err != nil {
		t.Fatal(err)
	}
	selection := "^(TestDispatchProvenanceExecuteRoundTrip|TestCaptureFidelitySerializerDispatchAndReplay)$"
	removed := exec.Command("go", "test", "-overlay", overlayPath, "./pkg/execution", "-run", selection, "-count=1")
	removed.Dir = root
	output, err := removed.CombinedOutput()
	if err == nil || !bytes.Contains(output, []byte("dispatch provenance mismatch")) || !bytes.Contains(output, []byte("TestDispatchProvenanceExecuteRoundTrip/native")) || !bytes.Contains(output, []byte("TestCaptureFidelitySerializerDispatchAndReplay/native")) {
		t.Fatalf("removal did not fail at intended behavioral assertions: %v\n%s", err, output)
	}
	restored := exec.Command("go", "test", "./pkg/execution", "-run", selection, "-count=1")
	restored.Dir = root
	output, err = restored.CombinedOutput()
	if err != nil {
		t.Fatalf("restored source failed: %v\n%s", err, output)
	}
	t.Log("removing real propagation fails dispatch and Execute persistence assertions; restored source passes")
}

// URL privacy is exercised through the actual serializer and failed Execute,
// not only by supplying metadata to the persistence boundary.
func TestDispatchProvenancePrivateURLExecute(t *testing.T) {
	for _, kind := range []string{"known", "userinfo", "query", "fragment"} {
		t.Run(kind, func(t *testing.T) {
			received := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received <- r.Method
				io.Copy(io.Discard, r.Body)
				w.WriteHeader(500)
				fmt.Fprint(w, `{"error":{"message":"public synthetic refusal"}}`)
			}))
			defer server.Close()
			base := server.URL + "/v1"
			switch kind {
			case "userinfo":
				base = strings.Replace(base, "://", "://synthetic-user:arbitrary-password@", 1)
			case "query":
				base += "?arbitrary=hidden-value"
			case "fragment":
				base += "#hidden-fragment"
			}
			adapter := fidelityAdapter(t, base)
			provider, err := NewAPIProvider(APIProviderConfig{Adapter: adapter, Tools: fidelityRequest("").Tools, ToolExecutor: &recordingToolExecutor{}})
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			capturePath, tracePath := filepath.Join(dir, "capture.json"), filepath.Join(dir, "trace", "run.json")
			t.Setenv(RequestCaptureEnv, capturePath)
			t.Setenv(RequestTraceEnv, tracePath)
			result, err := provider.Execute(context.Background(), Request{ID: "public-url", Prompt: "Public input", Model: "synthetic-model", WorkingDir: t.TempDir(), Sandbox: Sandbox{Mode: SandboxReadOnly}}, nil)
			if err == nil || result.Outcome != OutcomeFailed {
				t.Fatal("expected failed dispatch")
			}
			if len(received) != 1 || <-received != "POST" {
				t.Fatal("expected one real POST attempt")
			}
			capture, trace := readCapture(t, capturePath), readTrace(t, tracePath)
			entry := asMap(asList(trace["entries"])[0])
			if kind == "known" {
				assertDispatch(t, asMap(capture["dispatch"]), "POST", server.URL+"/v1/chat/completions")
				assertDispatch(t, asMap(entry["dispatch"]), "POST", server.URL+"/v1/chat/completions")
			} else if capture["dispatch"] != nil || entry["dispatch"] != nil {
				t.Fatal("privacy-altered actual target became replayable evidence")
			}
			for _, path := range []string{capturePath, tracePath} {
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				for _, leak := range []string{"synthetic-user", "arbitrary-password", "hidden-value", "hidden-fragment", "public-test-key"} {
					if bytes.Contains(raw, []byte(leak)) {
						t.Fatal("URL/credential sentinel leaked to actual persisted diagnostics")
					}
				}
			}
		})
	}
}

func TestDispatchProvenanceRefusedExecuteExcludesPreviousAttempt(t *testing.T) {
	server, dispatches := fidelityEndpoint(t)
	adapter := fidelityAdapter(t, server.URL+"/v1")
	if _, err := adapter.Complete(context.Background(), fidelityRequest("public")); err != nil {
		t.Fatal(err)
	}
	<-dispatches
	_, at, _, before := adapter.LastRequestWithDispatch()
	dir := t.TempDir()
	capturePath, tracePath := filepath.Join(dir, "capture.json"), filepath.Join(dir, "trace", "run.json")
	t.Setenv(RequestCaptureEnv, capturePath)
	t.Setenv(RequestTraceEnv, tracePath)
	provider, err := NewAPIProvider(APIProviderConfig{Adapter: adapter, Tools: fidelityRequest("").Tools, ToolExecutor: &recordingToolExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Execute(context.Background(), Request{ID: "public-refused", Prompt: "Public", Model: "synthetic-model", WorkingDir: t.TempDir(), Sandbox: Sandbox{Mode: SandboxReadOnly}, NativeSessionID: "unsupported"}, nil)
	if err == nil || result.Outcome != OutcomeFailed {
		t.Fatal("expected local refusal")
	}
	_, after, _, dispatch := adapter.LastRequestWithDispatch()
	if !after.Equal(at) || !reflect.DeepEqual(dispatch, before) || len(dispatches) != 0 {
		t.Fatal("local refusal acquired a fresh attempt")
	}
	for _, path := range []string{capturePath, tracePath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("previous attempt was attributed to a refused run")
		}
	}
}
