package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_Query(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse QueryResponse
		status         int
		want           string
		wantErr        bool
	}{
		{
			name: "successful response with result",
			serverResponse: QueryResponse{
				Result: "hello world",
			},
			status: http.StatusOK,
			want:   "hello world",
		},
		{
			name: "successful response with response field",
			serverResponse: QueryResponse{
				Response: "hello response",
			},
			status: http.StatusOK,
			want:   "hello response",
		},
		{
			name: "agent error in body",
			serverResponse: QueryResponse{
				Error: "agent failed",
			},
			status:  http.StatusOK,
			wantErr: true,
		},
		{
			name: "server error 500",
			serverResponse: QueryResponse{
				Error: "internal error",
			},
			status:  http.StatusInternalServerError,
			wantErr: true,
		},
		{
			name: "complex result formatting",
			serverResponse: QueryResponse{
				Result: map[string]string{"foo": "bar"},
			},
			status: http.StatusOK,
			want:   "{\n  \"foo\": \"bar\"\n}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_ = json.NewEncoder(w).Encode(tt.serverResponse)
			}))
			defer server.Close()

			c := New(server.URL)
			got, err := c.Query(context.Background(), "test query")
			if (err != nil) != tt.wantErr {
				t.Errorf("Client.Query() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Client.Query() = %v, want %v", got, tt.want)
			}
		})
	}
}
