package execution

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/openexec/openexec/pkg/agent"
)

// RequestTraceEnv names a file the caller wants the whole model conversation of
// a failed or inconclusive API run written to: every request the run sent, the
// response to each, and the events it emitted, in order. RequestCaptureEnv keeps
// only the last request with each message cut to about 2000 characters, which
// cannot reconstruct a multi-round run (see docs/NATIVE_TRIAGE_SEQUENCE_BOUNDARY.md).
// Unset, nothing is recorded or written. The file is private (0600, directory
// 0700) and holds owner content untruncated; it is meant for the host that ran
// the turn, never published. It has the same secret redaction as the capture,
// which is a privacy transformation and not wire fidelity: the file says
// whether redaction changed anything. A trace that cannot be written changes
// nothing about the run.
const RequestTraceEnv = "OPENEXEC_REQUEST_TRACE"

// traceLimit bounds the file. Past it later entries are dropped and the trace
// says so, instead of failing the run or silently looking complete.
var traceLimit = 256 << 20

type runTrace struct {
	path string

	mu       sync.Mutex
	entries  []map[string]any
	events   []map[string]any
	bytes    int
	dropped  int
	redacted bool
	omitted  bool
}

// newRunTrace returns nil, the no-op trace, unless the caller opted in.
func newRunTrace() *runTrace {
	path := strings.TrimSpace(os.Getenv(RequestTraceEnv))
	if path == "" {
		return nil
	}
	return &runTrace{path: path}
}

func (t *runTrace) wrapSink(sink EventSink) EventSink {
	return func(event Event) error {
		t.recordEvent(event)
		if sink == nil {
			return nil
		}
		return sink(event)
	}
}

func (t *runTrace) recordEvent(event Event) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, t.clean(event))
}

// record keeps one model round trip: the request as the loop built it, the
// request body as the adapter put it on the wire when the adapter remembers it,
// and the response or error.
func (t *runTrace) record(adapter agent.ProviderAdapter, sent time.Time, request agent.Request, response *agent.Response, err error) {
	if t == nil {
		return
	}
	entry := map[string]any{"sentAt": sent.UTC().Format(time.RFC3339Nano)}
	t.mu.Lock()
	defer t.mu.Unlock()
	entry["request"] = t.clean(request)
	if response != nil {
		entry["response"] = t.clean(response)
	}
	if err != nil {
		entry["error"] = redactSecrets(err.Error())
	}
	if last := lastRequestOf(adapter); last != nil {
		if body, at, _ := last.LastRequest(); len(body) > 0 && !at.Before(sent) {
			var wire map[string]any
			if json.Unmarshal(body, &wire) == nil {
				entry["wire"] = t.cleanMap(wire)
			}
		}
	}
	raw, _ := json.Marshal(entry)
	if t.bytes+len(raw) > traceLimit {
		t.dropped++
		return
	}
	t.bytes += len(raw)
	t.entries = append(t.entries, entry)
}

// clean converts any value to its JSON tree and redacts it without truncation.
func (t *runTrace) clean(value any) map[string]any {
	raw, err := json.Marshal(value)
	var tree map[string]any
	if err != nil || json.Unmarshal(raw, &tree) != nil {
		return map[string]any{"unrecordable": true}
	}
	return t.cleanMap(tree)
}

func (t *runTrace) cleanMap(tree map[string]any) map[string]any {
	// math.MaxInt: sanitizeCapture's bound never applies, only redaction and
	// the image omission shared with the capture.
	out, omitted := sanitizeCapture(tree, math.MaxInt)
	t.omitted = t.omitted || omitted
	if !omitted && !reflect.DeepEqual(tree, out) {
		t.redacted = true
	}
	return out
}

// write stores the trace for a run that ended failed, inconclusive or cancelled. Best
// effort, like the capture.
func (t *runTrace) write(adapter agent.ProviderAdapter, request Request, result Result, runErr error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.entries) == 0 && t.dropped == 0 {
		return // no model request was sent: nothing to replay
	}
	message := ""
	if runErr != nil {
		message = runErr.Error()
	}
	baseURL := ""
	if last := lastRequestOf(adapter); last != nil {
		_, _, baseURL = last.LastRequest()
	}
	doc := map[string]any{
		"version":    1,
		"kind":       "openexec-request-trace",
		"capturedAt": time.Now().UTC().Format(time.RFC3339),
		"provider":   adapter.GetName(),
		"baseURL":    redactSecrets(baseURL),
		"requestID":  request.ID,
		"outcome":    result.Outcome,
		"reason":     result.Reason,
		"error":      redactSecrets(message),
		"finalText":  redactSecrets(result.FinalText),
		// Fidelity: entries[].request is the conversation the loop built;
		// entries[].wire is the adapter's body, present only for adapters that
		// remember it. Redaction replaces secret-shaped strings; when
		// redactionChangedContent is true the trace is not byte-identical to
		// what was sent. Images are omitted.
		"redactionChangedContent": t.redacted,
		"imagesOmitted":           t.omitted,
		"incomplete":              t.dropped > 0,
		"droppedEntries":          t.dropped,
		"entries":                 t.entries,
		"events":                  t.events,
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return
	}
	_ = writePrivate(t.path, data)
}
