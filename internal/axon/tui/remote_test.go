package axontui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/manager"
)

func TestRemoteSource_List(t *testing.T) {
	infos := []manager.PipelineInfo{
		{FWUID: "FWU-001", Status: manager.StatusRunning, Phase: "IM"},
		{FWUID: "FWU-002", Status: manager.StatusComplete},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/fwus" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(infos)
	}))
	defer srv.Close()

	source := &RemoteSource{
		baseURL: srv.URL,
		client:  srv.Client(),
	}

	list, err := source.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 items, got %d", len(list))
	}
	if list[0].FWUID != "FWU-001" {
		t.Errorf("expected FWU-001, got %s", list[0].FWUID)
	}
}

func TestRemoteSource_Status(t *testing.T) {
	info := manager.PipelineInfo{
		FWUID: "FWU-001", Status: manager.StatusRunning, Phase: "TD", Iteration: 2,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/fwu/FWU-001/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info)
	}))
	defer srv.Close()

	source := &RemoteSource{baseURL: srv.URL, client: srv.Client()}

	got, err := source.Status("FWU-001")
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if got.Phase != "TD" {
		t.Errorf("expected phase TD, got %s", got.Phase)
	}
}

func TestRemoteSource_Subscribe(t *testing.T) {
	events := []loop.Event{
		{Type: loop.EventPhaseStart, Phase: "TD", Agent: "clario"},
		{Type: loop.EventAssistantText, Text: "hello"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/fwu/FWU-001/events" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher := w.(http.Flusher)
		for _, e := range events {
			data, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	source := &RemoteSource{baseURL: srv.URL, client: srv.Client()}

	ch, unsub, err := source.Subscribe("FWU-001")
	if err != nil {
		t.Fatalf("Subscribe() error: %v", err)
	}
	defer unsub()

	var received []loop.Event
	timeout := time.After(2 * time.Second)
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				goto done
			}
			received = append(received, event)
		case <-timeout:
			goto done
		}
	}
done:

	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}
	if received[0].Type != loop.EventPhaseStart {
		t.Errorf("expected phase_start, got %s", received[0].Type)
	}
	if received[1].Text != "hello" {
		t.Errorf("expected 'hello', got %s", received[1].Text)
	}
}

func TestRemoteSource_PauseStop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fwu/FWU-001/pause":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "pausing"})
		case "/api/fwu/FWU-001/stop":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	source := &RemoteSource{baseURL: srv.URL, client: srv.Client()}

	if err := source.Pause("FWU-001"); err != nil {
		t.Fatalf("Pause() error: %v", err)
	}
	if err := source.Stop("FWU-001"); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
}

func TestRemoteSource_StatusNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "pipeline not found"})
	}))
	defer srv.Close()

	source := &RemoteSource{baseURL: srv.URL, client: srv.Client()}

	_, err := source.Status("FWU-999")
	if err == nil {
		t.Fatal("expected error for not found")
	}
}
