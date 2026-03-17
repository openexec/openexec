package state

import (
    "context"
    "database/sql"
    "sync"
    "time"
)

// AsyncWriter provides a simple non-blocking write queue with graceful shutdown.
type AsyncWriter struct {
    db   *sql.DB
    ch   chan func(*sql.DB)
    wg   sync.WaitGroup
}

// NewAsyncWriter creates an AsyncWriter with a bounded queue and starts a single worker.
func NewAsyncWriter(db *sql.DB, queueSize int) *AsyncWriter {
    if queueSize <= 0 { queueSize = 256 }
    aw := &AsyncWriter{db: db, ch: make(chan func(*sql.DB), queueSize)}
    aw.wg.Add(1)
    go func() {
        defer aw.wg.Done()
        for fn := range aw.ch {
            if fn != nil { fn(aw.db) }
        }
    }()
    return aw
}

// WriteAsync enqueues a write function; drops if queue is full.
func (w *AsyncWriter) WriteAsync(fn func(*sql.DB)) {
    select {
    case w.ch <- fn:
    default:
        // Drop on backpressure to avoid blocking orchestrator; in practice
        // you may want to increment a metric/log a warning.
    }
}

// Close flushes outstanding writes with a timeout and stops the worker.
func (w *AsyncWriter) Close(ctx context.Context) {
    // Stop accepting new writes
    close(w.ch)
    done := make(chan struct{})
    go func() {
        w.wg.Wait()
        close(done)
    }()
    // Wait for completion or timeout
    deadline, ok := ctx.Deadline()
    if !ok {
        // default 3s
        t := time.Now().Add(3 * time.Second)
        var cancel context.CancelFunc
        ctx, cancel = context.WithDeadline(ctx, t)
        defer cancel()
    } else {
        _ = deadline
    }
    select {
    case <-done:
    case <-ctx.Done():
        // Timed out; best-effort shutdown
    }
}

