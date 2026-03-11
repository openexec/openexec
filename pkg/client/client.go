package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a thin wrapper around the OpenExec API.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// New creates a new OpenExec client.
func New(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// QueryRequest is the payload for a DCP query.
type QueryRequest struct {
	Query string `json:"query"`
}

// QueryResponse is the standard response from the DCP.
type QueryResponse struct {
	Response string `json:"response"`
	Result   any    `json:"result"`
	Error    string `json:"error"`
}

// Query sends a conversational query to the Deterministic Control Plane.
func (c *Client) Query(ctx context.Context, query string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/dcp/query", c.BaseURL)

	reqBody := QueryRequest{Query: query}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var qResp QueryResponse
	if err := json.Unmarshal(body, &qResp); err != nil {
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("server error (%d): %s", resp.StatusCode, string(body))
		}
		return "", fmt.Errorf("decode failed: %w", err)
	}

	if qResp.Error != "" {
		return "", fmt.Errorf("agent error: %s", qResp.Error)
	}

	// Format output
	if qResp.Result != nil {
		switch v := qResp.Result.(type) {
		case string:
			return v, nil
		default:
			pretty, _ := json.MarshalIndent(v, "", "  ")
			return string(pretty), nil
		}
	}

	return qResp.Response, nil
}
