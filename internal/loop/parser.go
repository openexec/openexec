package loop

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"github.com/openexec/openexec/pkg/util"
)

// Parser reads line-delimited stream-JSON from Claude Code and emits typed Events.
type Parser struct {
	events    chan<- Event
	iteration int
	tracker   *SignalTracker // optional, set by Loop for signal tracking
}

// NewParser creates a Parser that sends events to ch.
// iteration is the current loop iteration number stamped onto each event.
func NewParser(ch chan<- Event, iteration int) *Parser {
	return &Parser{events: ch, iteration: iteration}
}

// Parse reads from r until EOF, sending an Event for each recognized JSON line.
// Unrecognized or malformed lines are silently skipped.
func (p *Parser) Parse(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	// Allow up to 1 MB per line (Claude Code can emit large tool inputs).
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		p.parseLine(line)
	}
	return scanner.Err()
}

// rawMessage is the top-level JSON envelope from stream-json output.
type rawMessage struct {
	Type    string          `json:"type"`
	Subtype string          `json:"subtype,omitempty"`
	Message json.RawMessage `json:"message,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	// tool_result fields
	Content json.RawMessage `json:"content,omitempty"`
}

// messageBody is the shape of the "message" field.
type messageBody struct {
	Content []contentItem `json:"content"`
}

// contentItem is a single item inside message.content[].
type contentItem struct {
	Type  string                 `json:"type"`
	Text  string                 `json:"text,omitempty"`
	Name  string                 `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`
}

func (p *Parser) parseLine(line []byte) {
	var raw rawMessage
	if err := util.UnmarshalRobust(string(line), &raw); err != nil {
		return // malformed JSON — skip
	}

	switch raw.Type {
	case "system":
		// Session init — skip.
		return

	case "assistant":
		p.parseAssistant(raw.Message)

	case "tool_result":
		p.parseToolResult(raw.Content)

	case "result":
		// End of output — process about to exit. Nothing to emit.
		return

	default:
		// Unrecognized — skip.
		return
	}
}

func (p *Parser) parseAssistant(data json.RawMessage) {
	if data == nil {
		return
	}

	// Emit activity heartbeat
	p.emit(Event{Type: EventProgress, Iteration: p.iteration})

	var body messageBody
	if err := util.UnmarshalRobust(string(data), &body); err != nil {
		return
	}
	for _, item := range body.Content {
		switch item.Type {
		case "text":
			p.emit(Event{
				Type:      EventAssistantText,
				Iteration: p.iteration,
				Text:      item.Text,
			})

			// SELF-HEALING: Detect if agent claims task is already done or scope is misaligned
			txtLower := strings.ToLower(item.Text)
			isComplete := strings.Contains(txtLower, "already completed") || 
						 strings.Contains(txtLower, "already done") ||
						 strings.Contains(txtLower, "implementation is complete") ||
						 strings.Contains(txtLower, "criteria appear to be met")
			
			if isComplete {
				p.emitSignal(map[string]interface{}{
					"type":   "complete",
					"reason": "Agent verified implementation already exists",
				})
			}

			// If agent finds a semantic mismatch (scope), allow it to fix it if it's clear
			if strings.Contains(txtLower, "planning mismatch") && strings.Contains(txtLower, "analysis reveals") {
				p.emitSignal(map[string]interface{}{
					"type":   "progress",
					"text":   "Reconciling task metadata based on agent analysis",
				})
			}
		case "tool_use":
			if isOpenExecSignal(item.Name) {
				p.emitSignal(item.Input)
			} else {
				p.emit(Event{
					Type:      EventToolStart,
					Iteration: p.iteration,
					Tool:      item.Name,
					ToolInput: item.Input,
				})
			}
		}
	}
}

func (p *Parser) parseToolResult(data json.RawMessage) {
    if data == nil {
        return
    }

	// Emit activity heartbeat
	p.emit(Event{Type: EventProgress, Iteration: p.iteration})

    // Content can be an array of items or a simple string.
    // Prefer array parsing to extract structured text and artifact markers.
    var items []contentItem
    if err := util.UnmarshalRobust(string(data), &items); err == nil && len(items) > 0 {
        var b strings.Builder
        artifacts := map[string]string{}
        for _, it := range items {
            if it.Type == "text" && it.Text != "" {
                if b.Len() > 0 { b.WriteString("\n") }
                b.WriteString(it.Text)
                // Detect artifact markers like: ARTIFACT:patch <hash> <path>
                for _, line := range strings.Split(it.Text, "\n") {
                    line = strings.TrimSpace(line)
                    if strings.HasPrefix(line, "ARTIFACT:patch ") {
                        parts := strings.SplitN(line, " ", 3)
                        if len(parts) >= 3 {
                            artifacts["patch_hash"] = parts[1]
                            artifacts["patch_path"] = parts[2]
                        }
                    }
                }
            }
        }
        p.emit(Event{Type: EventToolResult, Iteration: p.iteration, Text: b.String(), Artifacts: artifacts})
        return
    }

    // Fallback to plain string
    var s string
    if err := util.UnmarshalRobust(string(data), &s); err == nil {
        p.emit(Event{Type: EventToolResult, Iteration: p.iteration, Text: s})
        return
    }
    // Last resort: raw JSON string
    p.emit(Event{Type: EventToolResult, Iteration: p.iteration, Text: string(data)})
}

func (p *Parser) emit(e Event) {
	if p.events != nil {
		p.events <- e
	}
}

// isOpenExecSignal returns true if the tool name is an openexec_signal or axon_signal call.
// Handles both direct ("openexec_signal") and MCP-prefixed ("mcp__axon-signal__axon_signal").
func isOpenExecSignal(toolName string) bool {
	return strings.HasSuffix(toolName, "openexec_signal") || strings.HasSuffix(toolName, "axon_signal")
}

func (p *Parser) emitSignal(input map[string]interface{}) {
	sigType, _ := input["type"].(string)
	reason, _ := input["reason"].(string)
	target, _ := input["target"].(string)

	if p.tracker != nil {
		p.tracker.RecordSignal(sigType, p.iteration)
	}

	p.emit(Event{
		Type:         EventSignalReceived,
		Iteration:    p.iteration,
		SignalType:   sigType,
		SignalTarget: target,
		Text:         reason,
	})
}
