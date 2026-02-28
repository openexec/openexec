package agent

import (
	"context"
)

// Message represents a single turn in a conversation
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Request is the payload sent to a provider
type Request struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	System   string    `json:"system,omitempty"`
}

// ResponseChunk is a part of a streaming LLM response
type ResponseChunk struct {
	Text  string `json:"text,omitempty"`
	Error error  `json:"error,omitempty"`
	Done  bool   `json:"done"`
}

// Provider defines the unified interface for AI models
type Provider interface {
	GenerateStream(ctx context.Context, req Request) (<-chan ResponseChunk, error)
	GetName() string
	GetModels() []string
}
