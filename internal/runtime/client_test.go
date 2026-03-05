package runtime

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRuntimeClient(t *testing.T) {
	// Arrange: Create mock HTTP server to simulate BEAM
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rpc" {
			t.Errorf("expected /rpc path, got %s", r.URL.Path)
		}

		var req rpcRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Assert request content
		if req.Method == "start_task" {
			if req.Params["task_id"] != "T-001" {
				t.Errorf("unexpected task_id: %v", req.Params["task_id"])
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result":"ok"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)

	t.Run("StartTask", func(t *testing.T) {
		// Act
		err := client.StartTask("T-001", "/tmp/proj")

		// Assert
		if err != nil {
			t.Fatalf("StartTask failed: %v", err)
		}
	})

	t.Run("RunIteration", func(t *testing.T) {
		// Act
		err := client.RunIteration("T-001")

		// Assert
		if err != nil {
			t.Fatalf("RunIteration failed: %v", err)
		}
	})
}
