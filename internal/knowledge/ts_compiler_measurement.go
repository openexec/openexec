package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// All contents of this record are metadata. Neither compiler output, stderr,
// diagnostics nor source bodies are serialized. request.json contains private
// relative filenames and exclusions only, exactly as passed to cmd.Stdin.
type nativeChildRecord struct {
	Arguments        []string        `json:"arguments"`
	RequestSHA256    string          `json:"request_sha256"`
	RequestBytes     int             `json:"request_bytes"`
	FileCount        int             `json:"file_count"`
	SourceBytes      int64           `json:"source_bytes"`
	ScriptSHA256     string          `json:"script_sha256"`
	NodeSHA256       string          `json:"node_sha256"`
	CompilerSHA256   string          `json:"typescript_sha256"`
	NodeResolved     string          `json:"node_resolved_path"`
	CompilerResolved string          `json:"typescript_resolved_path"`
	NodeVersion      string          `json:"node_version"`
	CompilerVersion  string          `json:"typescript_version"`
	Started          time.Time       `json:"started"`
	ElapsedNS        int64           `json:"elapsed_ns"`
	ParentDeadline   string          `json:"parent_deadline"`
	ChildDeadline    string          `json:"child_deadline"`
	ParentError      string          `json:"parent_error"`
	ChildError       string          `json:"child_error"`
	Bound            string          `json:"extractor_bound"`
	Ending           string          `json:"ending"`
	ExitCode         int             `json:"exit_code"`
	ProcessState     string          `json:"process_state"`
	UserNS           int64           `json:"user_ns"`
	SystemNS         int64           `json:"system_ns"`
	OSUsage          json.RawMessage `json:"os_usage"`
	StdoutBytes      int             `json:"stdout_bytes"`
	StderrBytes      int             `json:"stderr_bytes"`
	Configuration    string          `json:"configuration"`
	ExtractedFiles   int             `json:"extracted_files"`
	Diagnostics      int             `json:"diagnostic_count"`
	ResponseDecoded  bool            `json:"response_decoded"`
}

func captureHash(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

func captureFileHash(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "unknown: file unreadable"
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "unknown: file unreadable"
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func contextError(ctx context.Context) string {
	if ctx.Err() == nil {
		return "none"
	}
	return ctx.Err().Error()
}

func contextDeadline(ctx context.Context) string {
	if d, ok := ctx.Deadline(); ok {
		return d.UTC().Format(time.RFC3339Nano)
	}
	return "none"
}

func nativeEnding(parent, child context.Context, err error) string {
	switch {
	case parent.Err() != nil:
		return "caller_cancellation"
	case child.Err() != nil:
		return "extractor_deadline"
	case err == nil:
		return "exit_0"
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if exit.ExitCode() == -1 {
			return "external_signal"
		}
		return "ordinary_nonzero_exit"
	}
	return "start_or_io_error"
}

func childMeasurement(o compilerObservation, root string) nativeChildRecord {
	r := nativeChildRecord{
		Arguments: append([]string(nil), o.Command.Args...), RequestSHA256: captureHash(o.Request),
		RequestBytes: len(o.Request), Started: o.Started.UTC(), ElapsedNS: o.Elapsed.Nanoseconds(),
		ParentDeadline: contextDeadline(o.Parent), ChildDeadline: contextDeadline(o.Child),
		ParentError: contextError(o.Parent), ChildError: contextError(o.Child), Bound: typeScriptExtractorTimeout.String(),
		Ending: nativeEnding(o.Parent, o.Child, o.RunError), ExitCode: -1,
		StdoutBytes: o.StdoutBytes, StderrBytes: o.StderrBytes,
		ScriptSHA256: captureHash(typeScriptCompilerScript),
	}
	var req tsCompilerRequest
	if json.Unmarshal(o.Request, &req) == nil {
		r.FileCount = len(req.Files)
		for _, rel := range req.Files {
			if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
				r.SourceBytes += info.Size()
			}
		}
	}
	if state := o.Command.ProcessState; state != nil {
		r.ExitCode, r.ProcessState = state.ExitCode(), state.String()
		r.UserNS, r.SystemNS = state.UserTime().Nanoseconds(), state.SystemTime().Nanoseconds()
		r.OSUsage, _ = json.Marshal(state.SysUsage())
	}
	var response tsCompilerResponse
	if o.RunError == nil && json.Unmarshal(o.Stdout, &response) == nil {
		r.ResponseDecoded = true
		r.Configuration, r.ExtractedFiles, r.Diagnostics = response.Configuration, len(response.Files), len(response.Diagnostics)
	}
	return r
}

// Probes happen AFTER the measured run; they cannot eat its 45 second budget.
func enrichChildMeasurement(r *nativeChildRecord) {
	r.NodeSHA256 = captureFileHash(r.Arguments[0])
	r.CompilerSHA256 = captureFileHash(r.Arguments[2])
	r.NodeResolved, _ = filepath.EvalSymlinks(r.Arguments[0])
	r.CompilerResolved, _ = filepath.EvalSymlinks(r.Arguments[2])
	r.NodeVersion = captureProbe(r.Arguments[0], "--version")
	r.CompilerVersion = captureProbe(r.Arguments[0], "-p", "require(process.argv[1]).version", r.Arguments[2])
}

func captureProbe(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var out captureProbeOutput
	cmd.Stdout = &out
	cmd.WaitDelay = 100 * time.Millisecond
	err := cmd.Run()
	if err != nil {
		return "unknown: metadata probe failed (" + contextError(ctx) + ")"
	}
	return strings.TrimSpace(string(out))
}

type captureProbeOutput []byte

func (b *captureProbeOutput) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := 4096 - len(*b); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		*b = append(*b, p...)
	}
	return n, nil
}
