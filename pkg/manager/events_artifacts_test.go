package manager

import (
    "context"
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/openexec/openexec/internal/loop"
    "github.com/openexec/openexec/pkg/audit"
)

func TestAuditIncludesArtifactsAndCheckpointWritten(t *testing.T) {
    tmp := t.TempDir()
    logger, err := audit.NewLogger(filepath.Join(tmp, "audit.db"))
    if err != nil { t.Fatal(err) }
    m := New(Config{WorkDir: tmp, AuditLogger: logger})

    // seed pipelines map entry
    m.mu.Lock()
    m.pipelines["RUN-1"] = &entry{info: PipelineInfo{FWUID: "RUN-1", StartedAt: time.Now()}}
    m.mu.Unlock()

    ch := make(chan loop.Event, 1)
    go m.consumeEvents("RUN-1", ch)

    // send a tool_result with artifacts
    ch <- loop.Event{Type: loop.EventToolResult, Iteration: 1, Artifacts: map[string]string{"patch_hash":"abc","patch_path":"/tmp/p.patch"}}
    close(ch)
    // allow logger goroutine to process
    time.Sleep(100 * time.Millisecond)

    // Check audit entries
    res, err := logger.Query(context.Background(), &audit.QueryFilter{EventTypes: []audit.EventType{audit.EventRunStep}, Limit: 10})
    if err != nil { t.Fatal(err) }
    if len(res.Entries) == 0 { t.Fatal("expected at least one audit entry") }
    found := false
    for _, e := range res.Entries {
        var md map[string]interface{}
        _ = e.GetMetadata(&md)
        if md != nil {
            if arts, ok := md["artifacts"].(map[string]interface{}); ok {
                if arts["patch_hash"] == "abc" {
                    found = true
                    break
                }
            }
        }
    }
    if !found { t.Fatal("expected artifacts with patch_hash in audit metadata") }

    // Check checkpoint JSONL
    ck := filepath.Join(tmp, ".openexec", "checkpoints", "RUN-1.jsonl")
    if _, err := os.Stat(ck); err != nil {
        t.Fatalf("expected checkpoint file at %s", ck)
    }
}

