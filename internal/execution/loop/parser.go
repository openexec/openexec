package loop

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
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
	if err := json.Unmarshal(line, &raw); err != nil {
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
	var body messageBody
	if err := json.Unmarshal(data, &body); err != nil {
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
		case "tool_use":
			if isAxonSignal(item.Name) {
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
	// Content can be a string or a structured array. Try string first.
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		p.emit(Event{
			Type:      EventToolResult,
			Iteration: p.iteration,
			Text:      s,
		})
		return
	}
	// Fall back to stringifying the raw JSON.
	p.emit(Event{
		Type:      EventToolResult,
		Iteration: p.iteration,
		Text:      string(data),
	})
}

func (p *Parser) emit(e Event) {
	p.events <- e
}

// isAxonSignal returns true if the tool name is an axon_signal call.
// Handles both direct ("axon_signal") and MCP-prefixed ("mcp__axon-signal__axon_signal").
func isAxonSignal(toolName string) bool {
	return strings.HasSuffix(toolName, "axon_signal")
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
