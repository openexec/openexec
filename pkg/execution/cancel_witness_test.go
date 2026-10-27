package execution

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

// These tests use a LOCAL scripted endpoint and synthetic content only. They
// show that a run ended by its caller's context leaves the same private
// witness as a failed one; they say nothing about why any real run was slow.

// runCancelled sends one real request to a local endpoint whose answer asks for
// a tool, and cancels the caller's context (cause: absolute budget) while the
// tool runs, so the loop ends at the next round boundary as OutcomeCancelled.
func runCancelled(t *testing.T, prompt string) (Result, error) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"r1","model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"f.txt\"}"}}]},"finish_reason":"tool_calls"}]}`)
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	tools := &cancellingToolExecutor{cancel: func() { cancel(errAbsoluteRunBudget) }}
	provider := fixtureProvider(t, compatible(t, server.URL), tools)
	req := fixtureRequest(t)
	req.Prompt = prompt
	return provider.Execute(ctx, req, nil)
}

func TestCancelledRunKeepsCaptureAndTrace(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "w")
	capturePath, tracePath := filepath.Join(dir, "cap.json"), filepath.Join(dir, "trace.json")
	t.Setenv(RequestCaptureEnv, capturePath)
	t.Setenv(RequestTraceEnv, tracePath)
	long := strings.Repeat("long evidence ", 400)
	prompt := "synthetic " + fixtureSecret + " " + long

	result, err := runCancelled(t, prompt)
	if !errors.Is(err, context.Canceled) || result.Outcome != OutcomeCancelled {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	capture := readCapture(t, capturePath)
	if capture["outcome"] != OutcomeCancelled || !strings.Contains(capture["error"].(string), "context canceled") || capture["truncated"] != true {
		t.Fatalf("capture header = %v", capture)
	}
	trace := readTrace(t, tracePath)
	if trace["outcome"] != OutcomeCancelled || !strings.Contains(asString(trace["error"]), "context canceled") {
		t.Fatalf("trace header = %v", trace)
	}
	for _, key := range []string{"redactionChangedContent", "imagesOmitted", "incomplete", "droppedEntries"} {
		if _, ok := trace[key]; !ok {
			t.Fatalf("fidelity field %s missing", key)
		}
	}
	if trace["redactionChangedContent"] != true || len(asList(trace["entries"])) != 1 {
		t.Fatalf("trace = %v", trace)
	}
	raw, _ := os.ReadFile(tracePath)
	rawCap, _ := os.ReadFile(capturePath)
	if strings.Contains(string(raw), fixtureSecret) || strings.Contains(string(rawCap), fixtureSecret) {
		t.Fatal("a secret reached the witness")
	}
	// The trace keeps request text whole; the capture cuts it.
	if !strings.Contains(string(raw), long) || strings.Contains(string(rawCap), long) {
		t.Fatal("trace must be untruncated and capture bounded")
	}
}

func TestCancelledRunWritesNothingWithoutOptIn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(RequestCaptureEnv, "")
	t.Setenv(RequestTraceEnv, "")
	t.Chdir(dir)
	result, _ := runCancelled(t, "plain")
	if result.Outcome != OutcomeCancelled {
		t.Fatalf("%+v", result)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("files appeared: %v", entries)
	}
}

// Near misses: success, a refusal that sends no request, and a cancellation
// before the first request all leave nothing.
func TestNoWitnessWithoutASentRequestOrForSuccess(t *testing.T) {
	dir := t.TempDir()
	capturePath, tracePath := filepath.Join(dir, "cap.json"), filepath.Join(dir, "trace.json")
	t.Setenv(RequestCaptureEnv, capturePath)
	t.Setenv(RequestTraceEnv, tracePath)
	assertNone := func(what string) {
		t.Helper()
		for _, p := range []string{capturePath, tracePath} {
			if _, err := os.Stat(p); !os.IsNotExist(err) {
				t.Fatalf("%s left %s: %v", what, p, err)
			}
		}
	}

	ok := &fakeAPIAdapter{responses: []*agent.Response{{Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "done"}}, StopReason: agent.StopReasonEnd}}}
	if r, _, err := runFixture(t, ok, &fixtureTools{}); err != nil || r.Outcome != OutcomeSucceeded {
		t.Fatalf("%+v %v", r, err)
	}
	assertNone("success")

	req := fixtureRequest(t)
	req.NativeSessionID = "not-for-api"
	if _, err := fixtureProvider(t, &fakeAPIAdapter{}, &fixtureTools{}).Execute(context.Background(), req, nil); err == nil {
		t.Fatal("refused request ran")
	}
	assertNone("refusal")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	adapter := &fakeAPIAdapter{}
	r, err := fixtureProvider(t, adapter, &fixtureTools{}).Execute(ctx, fixtureRequest(t), nil)
	if err == nil || r.Outcome != OutcomeCancelled || len(adapter.requests) != 0 {
		t.Fatalf("%+v %v requests=%d", r, err, len(adapter.requests))
	}
	assertNone("cancel before any request")
}
