package axontui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/manager"
)

// RemoteSource connects to a running axon serve via HTTP API + SSE.
// Used when the TUI starts with --port (remote mode).
type RemoteSource struct {
	baseURL string
	client  *http.Client
}

// NewRemoteSource creates a RemoteSource targeting the given host:port.
func NewRemoteSource(host string, port int) *RemoteSource {
	return &RemoteSource{
		baseURL: fmt.Sprintf("http://%s:%d", host, port),
		client:  &http.Client{},
	}
}

func (s *RemoteSource) List() ([]manager.PipelineInfo, error) {
	resp, err := s.client.Get(s.baseURL + "/api/fwus")
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list: status %d", resp.StatusCode)
	}

	var list []manager.PipelineInfo
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("list: decode: %w", err)
	}
	return list, nil
}

func (s *RemoteSource) Status(fwuID string) (manager.PipelineInfo, error) {
	resp, err := s.client.Get(s.baseURL + "/api/fwu/" + fwuID + "/status")
	if err != nil {
		return manager.PipelineInfo{}, fmt.Errorf("status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return manager.PipelineInfo{}, fmt.Errorf("pipeline %s not found", fwuID)
	}
	if resp.StatusCode != http.StatusOK {
		return manager.PipelineInfo{}, fmt.Errorf("status: status %d", resp.StatusCode)
	}

	var info manager.PipelineInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return manager.PipelineInfo{}, fmt.Errorf("status: decode: %w", err)
	}
	return info, nil
}

func (s *RemoteSource) Subscribe(fwuID string) (<-chan loop.Event, func(), error) {
	req, err := http.NewRequest("GET", s.baseURL+"/api/fwu/"+fwuID+"/events", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("subscribe: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("subscribe: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return nil, nil, fmt.Errorf("pipeline %s not found", fwuID)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, nil, fmt.Errorf("subscribe: status %d", resp.StatusCode)
	}

	ch := make(chan loop.Event, 64)

	go readSSE(resp.Body, ch)

	unsub := func() {
		_ = resp.Body.Close()
	}

	return ch, unsub, nil
}

// readSSE parses SSE data lines into loop.Event and sends them to ch.
// Closes ch when the reader is exhausted or the body is closed.
func readSSE(body io.Reader, ch chan<- loop.Event) {
	defer close(ch)
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		var event loop.Event
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		ch <- event
	}
}

func (s *RemoteSource) Pause(fwuID string) error {
	resp, err := s.client.Post(s.baseURL+"/api/fwu/"+fwuID+"/pause", "", nil)
	if err != nil {
		return fmt.Errorf("pause: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return readAPIError(resp)
	}
	return nil
}

func (s *RemoteSource) Stop(fwuID string) error {
	resp, err := s.client.Post(s.baseURL+"/api/fwu/"+fwuID+"/stop", "", nil)
	if err != nil {
		return fmt.Errorf("stop: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return readAPIError(resp)
	}
	return nil
}

func readAPIError(resp *http.Response) error {
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return fmt.Errorf("%s", body.Error)
}
