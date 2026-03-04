package loop

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// SessionRecorder handles capturing execution evidence to disk.
type SessionRecorder struct {
	baseDir string
	fwuID   string
	dir     string

	stdoutFile *os.File
	stderrFile *os.File

	meta MetaData
}

// MetaData stores session execution details.
type MetaData struct {
	FwuID      string    `json:"fwu_id"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time,omitempty"`
	ExitCode   int       `json:"exit_code"`
	Error      string    `json:"error,omitempty"`
	Config     Config    `json:"config"`
	PromptHash string    `json:"prompt_hash"`
}

// NewSessionRecorder creates a recorder that stores evidence in baseDir/fwuID/timestamp/.
func NewSessionRecorder(baseDir, fwuID string) *SessionRecorder {
	return &SessionRecorder{
		baseDir: baseDir,
		fwuID:   fwuID,
	}
}

// Start prepares the evidence directory and opens capture files.
func (r *SessionRecorder) Start(cfg Config) error {
	if r.baseDir == "" {
		return nil
	}

	timestamp := time.Now().Format("20060102-150405")
	r.dir = filepath.Join(r.baseDir, r.fwuID, timestamp)

	if err := os.MkdirAll(r.dir, 0750); err != nil {
		return fmt.Errorf("failed to create evidence directory: %w", err)
	}

	var err error
	r.stdoutFile, err = os.Create(filepath.Join(r.dir, "stdout.jsonl"))
	if err != nil {
		return fmt.Errorf("failed to create stdout file: %w", err)
	}

	r.stderrFile, err = os.Create(filepath.Join(r.dir, "stderr.log"))
	if err != nil {
		_ = r.stdoutFile.Close() // cleanup
		return fmt.Errorf("failed to create stderr file: %w", err)
	}

	// Calculate prompt hash
	hash := sha256.Sum256([]byte(cfg.Prompt))
	promptHash := hex.EncodeToString(hash[:])

	r.meta = MetaData{
		FwuID:      r.fwuID,
		StartTime:  time.Now(),
		Config:     cfg,
		PromptHash: promptHash,
	}

	return r.writeMeta()
}

// Finish updates metadata with exit status and closes files.
func (r *SessionRecorder) Finish(exitCode int, runErr error) error {
	if r.dir == "" {
		return nil
	}

	defer r.closeFiles()

	r.meta.EndTime = time.Now()
	r.meta.ExitCode = exitCode
	if runErr != nil {
		r.meta.Error = runErr.Error()
	}

	return r.writeMeta()
}

func (r *SessionRecorder) writeMeta() error {
	data, err := json.MarshalIndent(r.meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Overwrite meta.json with restricted permissions
	return os.WriteFile(filepath.Join(r.dir, "meta.json"), data, 0600)
}

func (r *SessionRecorder) closeFiles() {
	if r.stdoutFile != nil {
		_ = r.stdoutFile.Close()
		r.stdoutFile = nil
	}
	if r.stderrFile != nil {
		_ = r.stderrFile.Close()
		r.stderrFile = nil
	}
}

// Stdout returns the writer for stdout capture, or nil if not recording.
func (r *SessionRecorder) Stdout() io.Writer {
	if r.stdoutFile == nil {
		return nil
	}
	return r.stdoutFile
}

// Stderr returns the writer for stderr capture, or nil if not recording.
func (r *SessionRecorder) Stderr() io.Writer {
	if r.stderrFile == nil {
		return nil
	}
	return r.stderrFile
}

// Dir returns the path to the current session evidence directory.
func (r *SessionRecorder) Dir() string {
	return r.dir
}
