package manager

import (
	"bytes"
	"log"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/pkg/audit"
)

// TestRunStepAuditWriteFailureIsReported: a stage receipt the audit logger
// refuses must be reported through the manager log, not discarded silently.
func TestRunStepAuditWriteFailureIsReported(t *testing.T) {
	tmp := t.TempDir()
	logger, err := audit.NewLogger(filepath.Join(tmp, "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(Config{WorkDir: tmp, AuditLogger: logger})
	if err != nil {
		t.Fatal(err)
	}
	// A closed logger refuses every write.
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)

	events := make(chan loop.Event, 1)
	events <- loop.Event{Type: loop.EventStageFailed, StageName: "apply", ErrText: "refused"}
	close(events)
	m.consumeEvents("audit-failure-run", events)

	out := buf.String()
	if !strings.Contains(out, "Audit write (run_step audit-failure-run stage_failed) failed") ||
		!strings.Contains(out, "audit logger is closed") {
		t.Fatalf("audit write failure not reported; log:\n%s", out)
	}
}
