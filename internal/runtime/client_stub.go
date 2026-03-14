//go:build !beam

package runtime

import "fmt"

// Client is a stub when BEAM runtime is not built.
type Client struct{}

func NewClient(addr string) *Client { return &Client{} }

func (c *Client) StartTask(taskID string, projectPath string) error {
    return fmt.Errorf("BEAM runtime not enabled: build with -tags=beam")
}

func (c *Client) RunIteration(taskID string) error {
    return fmt.Errorf("BEAM runtime not enabled: build with -tags=beam")
}

