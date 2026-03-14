package tract

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/openexec/openexec/internal/mcp"
)

// Client is a Tract MCP client that communicates over JSON-RPC stdio.
type Client struct {
	in     io.Writer      // send requests to Tract
	out    *bufio.Scanner // read responses from Tract
	cmd    *exec.Cmd      // nil when created from raw io (tests)
	stderr *bytes.Buffer  // capture subprocess errors
	nextID int
	mu     sync.Mutex
}

// NewClient creates a Client from raw io (for testing).
func NewClient(in io.Writer, out io.Reader) *Client {
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return &Client{
		in:     in,
		out:    scanner,
		nextID: 1,
		stderr: new(bytes.Buffer),
	}
}

// StartSubprocess launches `tract serve --store <name>` and returns a connected Client.
// The subprocess is killed when Close is called or ctx is cancelled.
func StartSubprocess(ctx context.Context, store string) (*Client, error) {
	// 1. Try "tract" in system path first
	bin := "tract"
	if _, err := exec.LookPath(bin); err != nil {
		// 2. Try sibling directory (dev environment)
		if execPath, err := os.Executable(); err == nil {
			siblingTract := filepath.Join(filepath.Dir(filepath.Dir(execPath)), "tract", "tract")
			if _, err := os.Stat(siblingTract); err == nil {
				bin = siblingTract
			} else {
				// 3. Try standard installation path
				home, _ := os.UserHomeDir()
				homeBin := filepath.Join(home, "bin", "tract")
				if _, err := os.Stat(homeBin); err == nil {
					bin = homeBin
				}
			}
		}
	}

	cmd := exec.CommandContext(ctx, bin, "serve", "--store", store)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("tract stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("tract stdout pipe: %w", err)
	}
	
	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("tract start: %w", err)
	}
	
	// Give the subprocess a moment to initialize its IO pipes
	time.Sleep(100 * time.Millisecond)

	// Check if process died immediately
	state := cmd.ProcessState
	if state != nil && state.Exited() {
		return nil, fmt.Errorf("tract process exited immediately: %s", stderr.String())
	}

	c := NewClient(stdin, stdout)
	c.cmd = cmd
	c.stderr = stderr
	return c, nil
}

// Brief calls tract_brief for the given FWU ID and returns the parsed response.
func (c *Client) Brief(fwuID string) (*BriefResponse, error) {
	// Step 1: Send initialize request.
	initParams := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "axon",
			"version": "1.0.0",
		},
	}
	if _, err := c.call("initialize", initParams); err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}

	// Step 2: Send notifications/initialized (no response expected).
	if err := c.notify("notifications/initialized"); err != nil {
		return nil, fmt.Errorf("notify initialized: %w", err)
	}

	// Step 3: Send tools/call for tract_brief.
	callParams := map[string]interface{}{
		"name": "tract_brief",
		"arguments": map[string]interface{}{
			"fwu_id": fwuID,
		},
	}
	resp, err := c.call("tools/call", callParams)
	if err != nil {
		return nil, fmt.Errorf("tools/call: %w", err)
	}

	// Step 4: Unwrap MCP tool response.
	// The result is: {"content":[{"type":"text","text":"{...JSON...}"}]}
	briefJSON, err := extractToolText(resp.Result)
	if err != nil {
		return nil, fmt.Errorf("unwrap tool response: %w", err)
	}

	var brief BriefResponse
	if err := json.Unmarshal([]byte(briefJSON), &brief); err != nil {
		return nil, fmt.Errorf("unmarshal brief: %w", err)
	}
	return &brief, nil
}

// Close shuts down the connection. If a subprocess was started, kills it.
func (c *Client) Close() error {
	if c.cmd != nil {
		if closer, ok := c.in.(io.Closer); ok {
			_ = closer.Close()
		}
		return c.cmd.Wait()
	}
	return nil
}

// call sends a JSON-RPC request and reads the response.
func (c *Client) call(method string, params interface{}) (*mcp.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.nextID
	c.nextID++

	rawParams, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	idBytes, err := json.Marshal(id)
	if err != nil {
		return nil, err
	}

	req := mcp.Request{
		JSONRPC: "2.0",
		ID:      idBytes,
		Method:  method,
		Params:  rawParams,
	}

	if err := c.send(req); err != nil {
		if c.stderr != nil && c.stderr.Len() > 0 {
			return nil, fmt.Errorf("send: %w (stderr: %s)", err, c.stderr.String())
		}
		return nil, fmt.Errorf("send: %w", err)
	}

	return c.readResponse()
}

// notify sends a JSON-RPC notification (no ID, no response expected).
func (c *Client) notify(method string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	req := mcp.Request{
		JSONRPC: "2.0",
		Method:  method,
	}
	return c.send(req)
}

func (c *Client) send(req mcp.Request) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = c.in.Write(data)
	return err
}

func (c *Client) readResponse() (*mcp.Response, error) {
	if !c.out.Scan() {
		errMsg := "unexpected EOF"
		if err := c.out.Err(); err != nil {
			errMsg = err.Error()
		}
		if c.stderr != nil && c.stderr.Len() > 0 {
			return nil, fmt.Errorf("read response: %s (stderr: %s)", errMsg, c.stderr.String())
		}
		return nil, fmt.Errorf("read response: %s", errMsg)
	}

	var resp mcp.Response
	if err := json.Unmarshal(c.out.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp, nil
}

// extractToolText extracts the text from an MCP tool response result.
// Expected structure: {"content":[{"type":"text","text":"..."}], "isError": false}
func extractToolText(result interface{}) (string, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal result: %w", err)
	}

	var wrapper struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return "", fmt.Errorf("unmarshal tool wrapper: %w", err)
	}

	if len(wrapper.Content) == 0 {
		return "", fmt.Errorf("empty content array")
	}

	if wrapper.IsError {
		return "", fmt.Errorf("tool error: %s", wrapper.Content[0].Text)
	}

	return wrapper.Content[0].Text, nil
}
