package ai

import (
	"context"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ImpactFile is one file the change is expected to touch.
type ImpactFile struct {
	Path   string `yaml:"path" json:"path"`
	Action string `yaml:"action" json:"action"` // create | modify | delete
	Reason string `yaml:"reason" json:"reason"`
}

// ImpactReport is the file-level "affects this and that" analysis.
type ImpactReport struct {
	Files []ImpactFile `yaml:"files" json:"files"`
	Notes string       `yaml:"notes" json:"notes"`
}

const impactPrompt = `You are analyzing which files a change will touch, for human review BEFORE any code is written.

Given the change intent and excerpts from the actual repository files below, list the specific files that will be CREATED or MODIFIED to implement the change, each with a one-line reason. Only list files you can justify from the intent and the provided excerpts — do NOT invent paths. If you are unsure of an exact path, say so in notes rather than guessing.

Return ONLY YAML in this shape:
files:
  - path: relative/path/to/file
    action: create | modify | delete
    reason: one line
notes: anything uncertain or files that still need to be located

CHANGE INTENT:
%s

REPOSITORY EXCERPTS:
%s
`

// AnalyzeImpact runs a single-shot file-level impact analysis over the change
// intent plus real repository excerpts. It never invents paths (the prompt
// forbids it); an empty file list with notes is a valid, honest result. The
// caller supplies the excerpts (relevance-ranked, already read from disk) so
// this stays pure and testable.
func AnalyzeImpact(ctx context.Context, completer Completer, intent, repoExcerpts string) (*ImpactReport, error) {
	if completer == nil {
		return nil, fmt.Errorf("governance/ai: impact analysis requires a completer")
	}
	raw, err := completer.Complete(ctx, fmt.Sprintf(impactPrompt, intent, repoExcerpts))
	if err != nil {
		return nil, fmt.Errorf("governance/ai: impact completion failed: %w", err)
	}
	rep := &ImpactReport{}
	_ = yaml.Unmarshal([]byte(extractStructured(raw, impactKeys)), rep)

	// Normalize actions and drop empty-path rows.
	clean := rep.Files[:0]
	for _, f := range rep.Files {
		f.Path = strings.TrimSpace(f.Path)
		if f.Path == "" {
			continue
		}
		f.Action = strings.ToLower(strings.TrimSpace(f.Action))
		if f.Action != "create" && f.Action != "modify" && f.Action != "delete" {
			f.Action = "modify"
		}
		clean = append(clean, f)
	}
	rep.Files = clean
	return rep, nil
}

var impactKeys = []string{"files", "notes"}
