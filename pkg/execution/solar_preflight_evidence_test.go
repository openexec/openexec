package execution

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"
)

// Read-only occurrence evidence on the diagnostic host. Public synthetic tests
// run everywhere; a skip here is explicitly NOT original-occurrence proof.
func TestSolarPreflightPrivateEvidence(t *testing.T) {
	const root = "/home/perttu/.agent-console/"
	const scratch = root + "perttu/scratch/20261101T021850Z/"
	artifacts := []struct{ path, hash string }{
		{root + "run-requests/5ead407d6eac762228dbf53d77f8a1a3.json", "3cabaae5f5000ca593f87358aa504b5ecbb5e5204afb7116bff577f056f210a3"},
		{scratch + "solar-replay-report.json", "0df07166ae499d637733ba3c8978486985988598b89fc086b2d2571c70651f4f"},
		{scratch + "solar-replay-minimal.json", "e8fdcbe352b77fed6b36b02c181cf0306c7552d3563a86cf728c40f20b666804"},
		{scratch + "solar-postboot-events.json", "7d5f1ac07f4973f251997a1c4a146bb216e1c1d278e4f9ae6df826d3638d08ac"},
	}
	if _, err := os.Stat(artifacts[0].path); os.IsNotExist(err) || os.IsPermission(err) {
		t.Skip("private Solar evidence unavailable; synthetic controls are not an original replay")
	}
	read := func(index int, value any) []byte {
		t.Helper()
		a := artifacts[index]
		raw, err := os.ReadFile(a.path)
		if err != nil || fmt.Sprintf("%x", sha256.Sum256(raw)) != a.hash {
			t.Fatalf("private artifact %d unavailable or identity changed", index)
		}
		if value != nil && json.Unmarshal(raw, value) != nil {
			t.Fatalf("private artifact %d malformed", index)
		}
		return raw
	}
	var capture struct {
		CapturedAt, SentAt string
		Truncated          bool
		TextLimit          int
		Dispatch           struct{ Method, URL, Provenance string }
		Request            map[string]any
	}
	read(0, &capture)
	u, err := url.Parse(capture.Dispatch.URL)
	if err != nil || u.Path != "/api/chat" || capture.Dispatch.Method != "POST" || capture.Dispatch.Provenance != "source-known-transport-attempt" ||
		!capture.Truncated || capture.TextLimit != 2000 || capture.SentAt != "2026-11-01T02:39:14Z" || capture.CapturedAt != "2026-11-01T02:41:58Z" {
		t.Fatal("capture timing, dispatch or fidelity facts changed")
	}
	r := capture.Request
	if len(asList(r["messages"])) != 100 || len(asList(r["tools"])) != 3 || r["model"] != "qwen38-27b-mtp2-128k" || r["stream"] != false || r["truncate"] != false ||
		!reflect.DeepEqual(asMap(r["options"]), map[string]any{"num_ctx": float64(65536), "num_predict": float64(21998)}) {
		t.Fatal("last-dispatched request configuration changed")
	}
	var replay struct {
		Reproduced, Partial bool
		Requests            int
		Seconds             float64
		First               struct {
			Outcome   string
			Signature *string
		}
		MinimalRequest map[string]any
	}
	read(1, &replay)
	var minimal map[string]any
	read(2, &minimal)
	if replay.Reproduced || replay.Partial || replay.Requests != 1 || replay.Seconds != 37 || replay.First.Outcome != "pass" || replay.First.Signature != nil ||
		!reflect.DeepEqual(minimal, r) || !reflect.DeepEqual(replay.MinimalRequest, r) {
		t.Fatal("one-request passing replay or truncated-payload identity changed")
	}
	var events []struct {
		Sequence        int
		CreatedAt, Type string
		Text            string
		Tool            struct{ CallID string }
	}
	read(3, &events)
	if len(events) != 164 {
		t.Fatal("original event count changed")
	}
	types := map[string]int{}
	starts, completed := map[string]bool{}, map[string]bool{}
	last := time.Time{}
	for i, e := range events {
		at, err := time.Parse(time.RFC3339Nano, e.CreatedAt)
		if err != nil || at.Before(last) || e.Sequence != i+1 {
			t.Fatal("original event ordering changed")
		}
		last = at
		types[e.Type]++
		if e.Type == "tool.started" {
			if starts[e.Tool.CallID] {
				t.Fatal("duplicate start")
			}
			starts[e.Tool.CallID] = true
		}
		if e.Type == "tool.completed" {
			if !starts[e.Tool.CallID] || completed[e.Tool.CallID] {
				t.Fatal("unpaired completion")
			}
			completed[e.Tool.CallID] = true
		}
		if e.Type == "turn.error" || e.Type == "turn.failed" {
			if e.Text != "context_overflow: assembled request bytes=274654 estimated_tokens=69793 context=65536 dispatched=false" {
				t.Fatal("original terminal boundary changed")
			}
		}
	}
	if !reflect.DeepEqual(starts, completed) || !reflect.DeepEqual(types, map[string]int{"turn.started": 1, "tool.started": 80, "tool.completed": 80, "assistant.delta": 1, "turn.error": 1, "turn.failed": 1}) ||
		events[0].CreatedAt != "2026-11-01T02:27:09.710890106Z" || events[162].Type != "turn.error" || events[162].CreatedAt != "2026-11-01T02:42:07.879089792Z" ||
		events[163].Type != "turn.failed" || events[163].CreatedAt != "2026-11-01T02:42:12.702226001Z" {
		t.Fatal("original chronology changed")
	}
	// Hash again after all assertions. Never write or print private content.
	for i := range artifacts {
		first, second := read(i, nil), read(i, nil)
		if !bytes.Equal(first, second) {
			t.Fatal("private evidence changed during re-read")
		}
	}
	t.Log("four original private artifacts re-read unchanged; 164 ordered events; last dispatch predates preflight refusal; replay PASS uses truncated payload")
}
