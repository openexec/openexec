package actions

import (
    "encoding/json"
    "errors"
)

// Action represents a typed, deterministic instruction the executor can perform.
type Action struct {
    ID        string                 `json:"id"`
    Type      string                 `json:"type"`      // e.g., "patch", "run", "test"
    Params    map[string]interface{} `json:"params"`    // type-specific params
    Workspace string                 `json:"workspace"` // required; must be within project root
    StoryID   string                 `json:"story_id,omitempty"`
    TaskID    string                 `json:"task_id,omitempty"`
    Version   int                    `json:"version"`
}

// Validate checks minimal schema rules without external dependencies.
func (a *Action) Validate() error {
    if a.ID == "" { return errors.New("id is required") }
    if a.Type == "" { return errors.New("type is required") }
    if a.Workspace == "" { return errors.New("workspace is required") }
    if a.Version <= 0 { return errors.New("version must be > 0") }
    return nil
}

// ParseAction parses JSON bytes into an Action and validates it.
func ParseAction(b []byte) (*Action, error) {
    var a Action
    if err := json.Unmarshal(b, &a); err != nil { return nil, err }
    if err := a.Validate(); err != nil { return nil, err }
    return &a, nil
}

