// Package execution is the public boundary for invoking OpenExec execution
// engines. Callers decide whether a request may run; an Executor performs the
// work described by an already-authorized request.
package execution

import (
	"context"
	"io"
	"time"
)

// Sandbox describes the containment promised for an execution. It is evidence,
// not a permission request: an Executor must reject modes it cannot enforce.
type Sandbox struct {
	Mode      string `json:"mode"`
	Isolation string `json:"isolation"`
}

// Request is one authorized execution unit. Prompt is intentionally opaque to
// the engine so callers can supply their own task instructions.
type Request struct {
	ID         string
	WorkingDir string
	Prompt     string
	Model      string
	Sandbox    Sandbox
}

// Result records what the executor actually used and how it terminated.
type Result struct {
	Executor  string    `json:"executor"`
	Model     string    `json:"model"`
	Sandbox   Sandbox   `json:"sandbox"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	Outcome   string    `json:"outcome"`
}

const (
	OutcomeSucceeded = "succeeded"
	OutcomeFailed    = "failed"
)

// Executor performs an authorized request. Implementations must return a
// Result even on execution failure so callers can preserve provenance.
type Executor interface {
	Execute(context.Context, Request) (Result, error)
}

// Streams carries optional process output destinations.
type Streams struct {
	Stdout io.Writer
	Stderr io.Writer
}
