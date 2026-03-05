package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client handles communication with the Elixir BEAM Runtime
type Client struct {
	addr       string
	httpClient *http.Client
}

func NewClient(addr string) *Client {
	return &Client{
		addr: addr,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type rpcRequest struct {
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
}

type rpcResponse struct {
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// StartTask tells the BEAM to spawn a supervised GenServer for a task
func (c *Client) StartTask(taskID string, projectPath string) error {
	req := rpcRequest{
		Method: "start_task",
		Params: map[string]interface{}{
			"task_id": taskID,
			"path":    projectPath,
		},
	}
	return c.call(req)
}

// RunIteration triggers a surgical AI loop iteration inside the BEAM
func (c *Client) RunIteration(taskID string) error {
	req := rpcRequest{
		Method: "run_iteration",
		Params: map[string]interface{}{
			"task_id": taskID,
		},
	}
	return c.call(req)
}

func (c *Client) call(rpcReq rpcRequest) error {
	data, err := json.Marshal(rpcReq)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Post(c.addr+"/rpc", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("BEAM runtime unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("BEAM runtime returned status %d", resp.StatusCode)
	}

	var rpcResp rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return err
	}

	if rpcResp.Error != "" {
		return fmt.Errorf("BEAM runtime error: %s", rpcResp.Error)
	}

	return nil
}
