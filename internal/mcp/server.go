package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

const protocolVersion = "2024-11-05"

// Request is a JSON-RPC 2.0 request or notification.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Server is a minimal MCP server that exposes the axon_signal tool.
type Server struct {
	in  io.Reader
	out io.Writer
}

// NewServer creates an MCP server reading from in, writing to out.
func NewServer(in io.Reader, out io.Writer) *Server {
	return &Server{in: in, out: out}
}

// Serve reads JSON-RPC requests from in, dispatches them, and writes responses to out.
// Blocks until in is closed (EOF). Returns nil on clean EOF.
func (s *Server) Serve() error {
	scanner := bufio.NewScanner(s.in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeError(nil, -32700, "parse error")
			continue
		}

		s.dispatch(req)
	}

	return scanner.Err()
}

func (s *Server) dispatch(req Request) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "notifications/initialized":
		// Notification — no response.
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	default:
		if req.ID != nil {
			s.writeError(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
		}
	}
}

func (s *Server) handleInitialize(req Request) {
	s.writeResult(req.ID, map[string]interface{}{
		"protocolVersion": protocolVersion,
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "axon-signal",
			"version": "1.0.0",
		},
	})
}

func (s *Server) handleToolsList(req Request) {
	s.writeResult(req.ID, map[string]interface{}{
		"tools": []interface{}{axonSignalToolDef()},
	})
}

func axonSignalToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "axon_signal",
		"description": "Send a structured signal to the Axon orchestrator. Use this to report progress, signal phase completion, request routing, or flag issues.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"type": map[string]interface{}{
					"type": "string",
					"enum": []string{
						"phase-complete", "blocked", "decision-point", "progress",
						"planning-mismatch", "scope-discovery", "route",
					},
					"description": "The signal type.",
				},
				"reason": map[string]interface{}{
					"type":        "string",
					"description": "Human-readable reason for the signal.",
				},
				"target": map[string]interface{}{
					"type":        "string",
					"description": "Target agent for route signals (e.g. 'spark', 'hon').",
				},
				"metadata": map[string]interface{}{
					"type":        "object",
					"description": "Additional structured metadata.",
				},
			},
			"required": []string{"type"},
		},
	}
}

// toolsCallParams is the shape of params for tools/call.
type toolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) handleToolsCall(req Request) {
	var params toolsCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.writeError(req.ID, -32602, "invalid params")
		return
	}

	if params.Name != "axon_signal" {
		s.writeError(req.ID, -32602, fmt.Sprintf("unknown tool: %s", params.Name))
		return
	}

	var sig Signal
	if err := json.Unmarshal(params.Arguments, &sig); err != nil {
		s.writeError(req.ID, -32602, fmt.Sprintf("invalid signal arguments: %v", err))
		return
	}

	if err := sig.Validate(); err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("invalid signal: %v", err))
		return
	}

	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": fmt.Sprintf("Signal received: %s", sig.Type),
			},
		},
	})
}

func (s *Server) writeResult(id json.RawMessage, result interface{}) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.writeJSON(resp)
}

func (s *Server) writeError(id json.RawMessage, code int, message string) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	}
	s.writeJSON(resp)
}

func (s *Server) writeToolError(id json.RawMessage, message string) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": message,
				},
			},
			"isError": true,
		},
	}
	s.writeJSON(resp)
}

func (s *Server) writeJSON(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	data = append(data, '\n')
	_, _ = s.out.Write(data)
}
