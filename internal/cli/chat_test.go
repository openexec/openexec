package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestSendChatQuery(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse map[string]interface{}
		status         int
		want           string
		wantErr        bool
	}{
		{
			name: "successful response with result",
			serverResponse: map[string]interface{}{
				"result": "hello world",
			},
			status: http.StatusOK,
			want:   "hello world",
		},
		{
			name: "successful response with response field",
			serverResponse: map[string]interface{}{
				"response": "hello legacy",
			},
			status: http.StatusOK,
			want:   "hello legacy",
		},
		{
			name: "server error",
			serverResponse: map[string]interface{}{
				"error": "something went wrong",
			},
			status:  http.StatusInternalServerError,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				json.NewEncoder(w).Encode(tt.serverResponse)
			}))
			defer server.Close()

			// Parse port from server URL
			parts := strings.Split(server.URL, ":")
			port, _ := strconv.Atoi(parts[len(parts)-1])
			
			// Save global port and restore later
			oldPort := startPort
			startPort = port
			defer func() { startPort = oldPort }()

			got, err := sendChatQuery("test query")
			if (err != nil) != tt.wantErr {
				t.Errorf("sendChatQuery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("sendChatQuery() = %v, want %v", got, tt.want)
			}
		})
	}
}
