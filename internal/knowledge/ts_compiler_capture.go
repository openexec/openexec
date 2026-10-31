package knowledge

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
)

// This is a private, opt-in evidence sink at the actual CLI child boundary.
// It never writes source bodies, compiler stdout/stderr, or the environment.
// The caller owns retention; filenames, paths and hashes can still be private.
const compilerCaptureEnv = "OPENEXEC_TS_COMPILER_CAPTURE_DIR"

type compilerCapture struct {
	dir string
}

func beginCompilerCapture(root string, request []byte, node, compiler string) (*compilerCapture, error) {
	dir := os.Getenv(compilerCaptureEnv)
	if dir == "" {
		return nil, nil
	}
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("capture parent must be absolute")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return nil, fmt.Errorf("capture parent must be an existing private directory, not a symlink")
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return nil, fmt.Errorf("capture parent must be outside the source repository")
	}
	dir, err = os.MkdirTemp(dir, "compiler-")
	if err != nil {
		return nil, err
	}
	c := &compilerCapture{dir: dir}
	// Written BEFORE starting the bounded child. If the entire CLI disappears,
	// absence of result.json is an incomplete capture, never a successful run.
	for name, data := range map[string][]byte{"request.json": request, "extractor.mjs": typeScriptCompilerScript} {
		if err := c.write(name, data); err != nil {
			return nil, err
		}
	}
	manifest, err := BuildScanManifest(root)
	if err != nil {
		return nil, err
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	build := "unknown"
	if b, ok := debug.ReadBuildInfo(); ok {
		build = b.String()
	}
	identity := map[string]any{
		"schema": 1, "executable": executable, "executable_sha256": captureFileHash(executable),
		"build": build, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		"source_root": root, "source_manifest": manifest,
		"source_head":    captureProbe("git", "-C", root, "rev-parse", "HEAD"),
		"request_sha256": captureHash(request), "script_sha256": captureHash(typeScriptCompilerScript),
		"node_path": node, "node_sha256": captureFileHash(node),
		"typescript_path": compiler, "typescript_sha256": captureFileHash(compiler),
		"extractor_bound": typeScriptExtractorTimeout.String(), "process_before": compilerProcessSnapshot(),
		"measurement_note": "opt-in hashes/manifest precede child deadline; version probes follow Wait; not an incident identity unless captured on that worker",
	}
	if err := c.writeJSON("identity.json", identity); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *compilerCapture) finish(record nativeChildRecord, after map[string]string) error {
	enrichChildMeasurement(&record)
	return c.writeJSON("result.json", struct {
		Child nativeChildRecord `json:"child"`
		After map[string]string `json:"process_after"`
	}{record, after})
}

func (c *compilerCapture) writeJSON(name string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return c.write(name, b)
}

func (c *compilerCapture) write(name string, data []byte) error {
	f, err := os.OpenFile(filepath.Join(c.dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// Two fixed snapshots, no polling, subprocesses, source or environment dumps.
// Missing host files are explicitly unknown (including on macOS).
func compilerProcessSnapshot() map[string]string {
	result := map[string]string{}
	read := func(path string) string {
		f, err := os.Open(path)
		if err != nil {
			return "unknown: unavailable"
		}
		defer f.Close()
		b, err := io.ReadAll(io.LimitReader(f, 4096))
		if err != nil {
			return "unknown: unreadable"
		}
		return string(b)
	}
	for _, path := range []string{"/proc/loadavg", "/proc/pressure/cpu", "/proc/pressure/memory", "/proc/self/cgroup"} {
		result[path] = read(path)
	}
	for _, line := range strings.Split(result["/proc/self/cgroup"], "\n") {
		if strings.HasPrefix(line, "0::/") {
			group := strings.TrimPrefix(line, "0::")
			// cgroup v2 membership is kernel-supplied, but still constrain paths.
			if strings.Contains(group, "..") {
				break
			}
			for _, name := range []string{"cpu.max", "cpu.stat", "cpu.pressure", "memory.max", "memory.current", "memory.events"} {
				path := filepath.Join("/sys/fs/cgroup", group, name)
				result[path] = read(path)
			}
		}
	}
	return result
}
