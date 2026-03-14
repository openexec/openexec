package manager

import (
    "database/sql"
    "encoding/json"
    "os"
    "path/filepath"
    "time"

    "github.com/google/uuid"
    "github.com/openexec/openexec/internal/loop"
)

type checkpointRecord struct {
    RunID     string            `json:"run_id"`
    Phase     string            `json:"phase,omitempty"`
    Iteration int               `json:"iteration"`
    Timestamp string            `json:"timestamp"`
    Artifacts map[string]string `json:"artifacts,omitempty"`
}

// writeCheckpointJSONL appends a checkpoint line to .openexec/checkpoints/<run_id>.jsonl
func writeCheckpointJSONL(workDir, runID string, event loop.Event) {
    if workDir == "" || runID == "" {
        return
    }
    if len(event.Artifacts) == 0 {
        return
    }
    dir := filepath.Join(workDir, ".openexec", "checkpoints")
    _ = os.MkdirAll(dir, 0o755)
    path := filepath.Join(dir, runID+".jsonl")
    rec := checkpointRecord{
        RunID:     runID,
        Phase:     event.Phase,
        Iteration: event.Iteration,
        Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
        Artifacts: event.Artifacts,
    }
    b, err := json.Marshal(rec)
    if err != nil {
        return
    }
    f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
    if err != nil {
        return
    }
    defer f.Close()
    _, _ = f.Write(append(b, '\n'))
}

// writeCheckpointSQLite inserts a checkpoint row into the audit database if available.
func writeCheckpointSQLite(db *sql.DB, runID string, event loop.Event) {
    if db == nil || runID == "" || len(event.Artifacts) == 0 {
        return
    }
    b, err := json.Marshal(event.Artifacts)
    if err != nil { return }

    _, _ = db.Exec(
        `INSERT INTO run_checkpoints (id, run_id, phase, iteration, timestamp, artifacts)
         VALUES (?, ?, ?, ?, ?, ?)`,
        uuid.New().String(), runID, event.Phase, event.Iteration, time.Now().UTC().Format(time.RFC3339Nano), string(b),
    )
}
