package knowledge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

var consoleNativeCheckout = flag.String("console-native-checkout", "", "explicitly authorized disposable public Console checkout; scanned and written")
var consoleCaptureDirectory = flag.String("console-capture-dir", "", "new directory outside the checkout for source-safe native request and measurement artifacts")

// All contents of this record are metadata. Neither compiler output, stderr,
// diagnostics nor source bodies are serialized. request.json contains public
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
	b, err := os.ReadFile(path)
	if err != nil {
		return "unknown: file unreadable"
	}
	return captureHash(b)
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
func enrichChildMeasurement(t *testing.T, r *nativeChildRecord) {
	t.Helper()
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
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return "unknown: metadata probe failed (" + contextError(ctx) + ")"
	}
	return strings.TrimSpace(string(out))
}

// The replay uses the observed argv, bytes and 45s bound. The scanner already
// removed its temp script; restore identical bytes at the SAME pathname using
// O_EXCL, never overwrite an existing file. This mtime change is disclosed.
func replayNativeRequest(t *testing.T, parent context.Context, args []string, request []byte) nativeChildRecord {
	t.Helper()
	f, err := os.OpenFile(args[1], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(args[1])
	if _, err = f.Write(typeScriptCompilerScript); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	child, cancel := context.WithTimeout(parent, typeScriptExtractorTimeout)
	defer cancel()
	cmd := exec.CommandContext(child, args[0], args[1:]...)
	cmd.Stdin = bytes.NewReader(request)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	start := time.Now()
	runErr := cmd.Run()
	return childMeasurement(compilerObservation{Request: request, Command: cmd, Started: start,
		Elapsed: time.Since(start), Parent: parent, Child: child, RunError: runErr,
		StdoutBytes: out.Len(), StderrBytes: stderr.Len(), Stdout: out.Bytes()}, args[3])
}

func databaseLocation(t *testing.T, s *Store) string {
	t.Helper()
	var seq int
	var name, path string
	if err := s.db.QueryRow("PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	return path
}

func reopenCapturedGraph(t *testing.T, storeRoot string, original GraphGeneration) GraphGeneration {
	t.Helper()
	s, err := NewStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	g, err := s.activeGeneration(context.Background(), original.WorktreeID)
	if err != nil {
		t.Fatal(err)
	}
	if g.ID != original.ID || g.Status != original.Status || !reflect.DeepEqual(g.Capabilities, original.Capabilities) || !reflect.DeepEqual(g.Limitations, original.Limitations) {
		t.Fatal("persisted graph differs from scan result")
	}
	return g
}

func writeCapture(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestConsoleNativeRequestCapture is opt-in. One scan plus ONE direct replay;
// there is no retry, reduction, provider call or changed compiler budget. Only
// a disposable, clean, explicitly supplied public checkout may be used.
func TestConsoleNativeRequestCapture(t *testing.T) {
	if *consoleNativeCheckout == "" {
		t.Skip("requires -console-native-checkout and -console-capture-dir; offline by default")
	}
	root, err := filepath.Abs(*consoleNativeCheckout)
	if err != nil {
		t.Fatal(err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if *consoleCaptureDirectory == "" {
		t.Fatal("-console-capture-dir is required")
	}
	dir, err := filepath.Abs(*consoleCaptureDirectory)
	if err != nil {
		t.Fatal(err)
	}
	// Resolve the existing parent before comparing containment. A symlink
	// outside source must not redirect artifact writes into the source corpus.
	dirParent, err := filepath.EvalSymlinks(filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	dir = filepath.Join(dirParent, filepath.Base(dir))
	if rel, err := filepath.Rel(root, dir); err != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		t.Fatal("capture directory must be outside source checkout")
	}
	status, err := exec.Command("git", "-C", root, "status", "--porcelain", "--untracked-files=all").Output()
	if err != nil || len(status) != 0 {
		t.Fatal("source must be a clean, authorized disposable git checkout")
	}
	if _, err := os.Lstat(filepath.Join(root, ".openexec")); !os.IsNotExist(err) {
		t.Fatal("refusing source with existing .openexec; use a fresh disposable checkout")
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	build, ok := debug.ReadBuildInfo()
	if !ok {
		t.Fatal("native test executable build identity unavailable")
	}
	identity := map[string]any{"executable": executable, "executable_sha256": captureFileHash(executable),
		"build": build.String(), "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		"source_head":              captureProbe("git", "-C", root, "rev-parse", "HEAD"),
		"openexec_head_at_runtime": captureProbe("git", "rev-parse", "HEAD"), "command": os.Args,
		"source_provenance_note": "runtime checkout HEAD is separate from embedded build identity; Go test build info may omit VCS identity; retain the candidate build command",
		"instrumentation":        "native Go test executable; production ScanRepository and child; observer after Wait; not incident CLI binary"}
	encoded, _ := json.MarshalIndent(identity, "", "  ")
	writeCapture(t, dir, "identity.json", encoded)
	var native nativeChildRecord
	var request []byte
	observations := 0
	parent, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()
	ctx := context.WithValue(parent, compilerObservationKey{}, func(o compilerObservation) {
		observations++
		request = append([]byte(nil), o.Request...)
		native = childMeasurement(o, root)
		writeCapture(t, dir, "request.json", request)
		b, _ := json.MarshalIndent(native, "", "  ")
		writeCapture(t, dir, "native-child.json", b)
	})
	s, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	db := databaseLocation(t, s)
	start := time.Now()
	scan, scanErr := s.ScanRepository(ctx, root)
	elapsed := time.Since(start)
	closeErr := s.Close()
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if observations != 1 {
		t.Fatalf("observed %d child runs, want exactly one; scan error=%v", observations, scanErr != nil)
	}
	enrichChildMeasurement(t, &native)
	b, _ := json.MarshalIndent(native, "", "  ")
	writeCapture(t, dir, "native-child.json", b)
	graph := map[string]any{"database": db, "scan_elapsed_ns": elapsed.Nanoseconds(), "scan_error": scanErr != nil,
		"parent_error": contextError(parent), "generation": scan.Generation.ID}
	if scanErr == nil {
		g := reopenCapturedGraph(t, root, scan.Generation)
		limitations, _ := json.Marshal(g.Limitations)
		graph["reopened"] = true
		graph["status"], graph["capabilities"] = g.Status, g.Capabilities
		graph["limitations_count"], graph["limitations_sha256"] = len(g.Limitations), captureHash(limitations)
		graph["compiler_fallback"] = g.Capabilities["typescript.definitions"] == "static_lexical"
		graph["manifest_sha256"], graph["configuration_digest"] = g.ManifestHash, g.ConfigurationDigest
	}
	b, _ = json.MarshalIndent(graph, "", "  ")
	writeCapture(t, dir, "graph.json", b)
	// New observer parent has the same 600s allowance; no already-spent scan
	// time can reduce the direct child's 45s allowance.
	directParent, directCancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer directCancel()
	direct := replayNativeRequest(t, directParent, native.Arguments, request)
	enrichChildMeasurement(t, &direct)
	b, _ = json.MarshalIndent(direct, "", "  ")
	writeCapture(t, dir, "direct-child.json", b)
	after, manifestErr := BuildScanManifest(root)
	sameInputs := manifestErr == nil && after.ManifestHash == scan.Generation.ManifestHash && after.ConfigurationDigest == scan.Generation.ConfigurationDigest
	graph["source_inputs_unchanged_after_direct"] = sameInputs
	b, _ = json.MarshalIndent(graph, "", "  ")
	writeCapture(t, dir, "graph.json", b)
	if direct.RequestSHA256 != native.RequestSHA256 || !reflect.DeepEqual(direct.Arguments, native.Arguments) {
		t.Fatal("replay identity changed")
	}
	if direct.NodeSHA256 != native.NodeSHA256 || direct.CompilerSHA256 != native.CompilerSHA256 || direct.ScriptSHA256 != native.ScriptSHA256 {
		t.Fatal("comparison invalid: toolchain hashes changed")
	}
	if !sameInputs {
		t.Fatal("comparison invalid: source/configuration changed or cannot be revalidated")
	}
	t.Logf("native=%s direct=%s; request=%s; source-safe records: %s; no causal or recovery claim", native.Ending, direct.Ending, native.RequestSHA256, dir)
	if scanErr != nil {
		t.Fatal("native scan failed; captured records retained")
	}
	if !native.ResponseDecoded || !direct.ResponseDecoded {
		// A captured deadline is evidence, but must not silently pass as a
		// successful compiler run. Records survive the intentional test failure.
		t.Fatalf("compiler success unproved: native=%s direct=%s", native.Ending, direct.Ending)
	}
}

func TestNativeCaptureObservesConsumedRequestAndPersistedFallback(t *testing.T) {
	cases := []struct {
		name, body, ending string
		cancel             bool
		bound              time.Duration
		decoded            bool
	}{
		{"success", `cat > "$3/consumed.json"; printf '{"configuration":"fixture","files":[],"diagnostics":[]}'`, "exit_0", false, 5 * time.Second, true},
		{"deadline", `cat > "$3/consumed.json"; exec sleep 30`, "extractor_deadline", false, 200 * time.Millisecond, false},
		{"caller", `cat > "$3/consumed.json"; exec sleep 30`, "caller_cancellation", true, 5 * time.Second, false},
		{"signal", `cat > "$3/consumed.json"; kill -9 $$`, "external_signal", false, 5 * time.Second, false},
		{"ordinary", `cat > "$3/consumed.json"; echo PRIVATE_STDERR >&2; exit 3`, "ordinary_nonzero_exit", false, 5 * time.Second, false},
		{"malformed success", `cat > "$3/consumed.json"; printf PRIVATE_STDOUT`, "exit_0", false, 5 * time.Second, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := fakeNodeExtractor(t, tc.body)
			writeTestFile(t, root, "z.ts", "export const PRIVATE_SOURCE = 1\n")
			writeTestFile(t, root, "a.js", "export const a = 2\n")
			writeTestFile(t, root, "dist/excluded.ts", "export const excluded = 3\n")
			old := typeScriptExtractorTimeout
			typeScriptExtractorTimeout = tc.bound
			defer func() { typeScriptExtractorTimeout = old }()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				time.AfterFunc(500*time.Millisecond, cancel)
			}
			var captured compilerObservation
			var record nativeChildRecord
			calls := 0
			ctx = context.WithValue(ctx, compilerObservationKey{}, func(o compilerObservation) {
				calls++
				captured = o
				record = childMeasurement(o, root)
			})
			storeRoot := t.TempDir()
			s, err := NewStore(storeRoot)
			if err != nil {
				t.Fatal(err)
			}
			scan, scanErr := s.ScanRepository(ctx, root)
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("observer calls = %d, scan error: %v", calls, scanErr)
			}
			consumed, err := os.ReadFile(filepath.Join(root, "consumed.json"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(consumed, captured.Request) {
				t.Fatal("observer request differs from child-consumed stdin")
			}
			var request tsCompilerRequest
			if err := json.Unmarshal(consumed, &request); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(request.Files, []string{"a.js", "z.ts"}) {
				t.Fatalf("actual scanner files = %v", request.Files)
			}
			if record.Ending != tc.ending {
				t.Fatalf("ending=%s, want %s", record.Ending, tc.ending)
			}
			if record.RequestSHA256 != captureHash(consumed) || record.RequestBytes != len(consumed) || record.SourceBytes != int64(len("export const PRIVATE_SOURCE = 1\n")+len("export const a = 2\n")) {
				t.Fatalf("bad request measurement: %+v", record)
			}
			encoded, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(encoded, []byte("PRIVATE_SOURCE")) || bytes.Contains(encoded, []byte("PRIVATE_STDERR")) || bytes.Contains(encoded, []byte("PRIVATE_STDOUT")) {
				t.Fatal("private contents serialized")
			}
			if record.ResponseDecoded != tc.decoded {
				t.Fatal("malformed compiler output misclassified")
			}
			if record.Bound != tc.bound.String() || record.ChildDeadline == "none" || record.ParentDeadline != "none" || record.ElapsedNS <= 0 || record.ProcessState == "" {
				t.Fatalf("missing timing/process evidence: %+v", record)
			}
			if tc.ending == "ordinary_nonzero_exit" && (record.ExitCode != 3 || record.StderrBytes == 0) {
				t.Fatal("ordinary exit/stderr measurements lost")
			}
			if tc.cancel {
				if scanErr == nil || record.ParentError != context.Canceled.Error() {
					t.Fatal("caller cancellation lost")
				}
				return
			}
			if scanErr != nil {
				t.Fatal(scanErr)
			}
			g := reopenCapturedGraph(t, storeRoot, scan.Generation)
			want := "static_lexical"
			if tc.decoded {
				want = "compiler_exact"
			}
			for _, capability := range []string{"typescript.definitions", "typescript.imports", "typescript.exports"} {
				if g.Capabilities[capability] != want {
					t.Fatalf("persisted %s=%s, want %s", capability, g.Capabilities[capability], want)
				}
			}
			if !tc.decoded && !strings.Contains(strings.Join(g.Limitations, "\n"), "TypeScript compiler extraction unavailable; using lexical") {
				t.Fatal("fallback limitation not persisted")
			}
		})
	}
}

func TestNativeCaptureRealCompilerRoundTrip(t *testing.T) {
	if _, err := findCompatibleNode(context.Background()); err != nil {
		t.Skip(err)
	}
	compiler, err := filepath.Abs("../../ui/node_modules/typescript")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(compiler, "lib", "typescript.js")); err != nil {
		t.Skip("locked repository TypeScript unavailable")
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(compiler, filepath.Join(root, "node_modules", "typescript")); err != nil {
		t.Skipf("compiler symlink unavailable: %v", err)
	}
	writeTestFile(t, root, "tsconfig.json", `{"compilerOptions":{"target":"ES2022","module":"ESNext"},"include":["src"]}`)
	writeTestFile(t, root, "src/a.ts", "export function a(): number { return 1 }\n")
	writeTestFile(t, root, "src/b.ts", "import { a } from './a'; export const b = a()\n")
	writeTestFile(t, root, "dist/near-miss.ts", "export const ignored = 3\n")
	var native nativeChildRecord
	var request []byte
	ctx := context.WithValue(context.Background(), compilerObservationKey{}, func(o compilerObservation) {
		native = childMeasurement(o, root)
		request = append([]byte(nil), o.Request...)
	})
	s, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	scan, scanErr := s.ScanRepository(ctx, root)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if scanErr != nil {
		t.Fatal(scanErr)
	}
	g := reopenCapturedGraph(t, root, scan.Generation)
	if g.Capabilities["typescript.imports"] != "compiler_exact" || native.Ending != "exit_0" || !native.ResponseDecoded || native.FileCount != 2 || native.ExtractedFiles != 2 {
		t.Fatalf("compiler evidence missing: %+v; %v", native, g.Limitations)
	}
	direct := replayNativeRequest(t, context.Background(), native.Arguments, request)
	if direct.Ending != "exit_0" || !direct.ResponseDecoded || direct.RequestSHA256 != native.RequestSHA256 || direct.Configuration != native.Configuration || direct.ExtractedFiles != native.ExtractedFiles || !reflect.DeepEqual(direct.Arguments, native.Arguments) {
		t.Fatalf("exact replay differs: %+v", direct)
	}
	t.Logf("real compiler: request=%s/%d bytes; native=%dns direct=%dns; reopened compiler_exact", native.RequestSHA256, native.RequestBytes, native.ElapsedNS, direct.ElapsedNS)
}

func TestNativeCaptureStartFailureIsNotDeadline(t *testing.T) {
	ctx := context.Background()
	if got := nativeEnding(ctx, ctx, errors.New("signal: killed in unrelated text")); got != "start_or_io_error" {
		t.Fatal(got)
	}
	if got := nativeEnding(ctx, ctx, nil); got != "exit_0" {
		t.Fatal(got)
	}
}

// Run the public entry point as an actual native executable, not a helper
// standing in for its artifact writer or graph scanner. This tiny synthetic
// repository proves the control works; it is explicitly NOT Console evidence.
func TestNativeCaptureDriverArtifactsAndRefusals(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip(err)
	}
	if _, err := findCompatibleNode(context.Background()); err != nil {
		t.Skip(err)
	}
	compiler, err := filepath.Abs("../../ui/node_modules/typescript")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(compiler, "lib", "typescript.js")); err != nil {
		t.Skip("locked repository TypeScript unavailable")
	}
	// Exercise the /var -> /private/var alias seen on macOS on every OS.
	// The driver canonicalizes its supplied checkout before opening SQLite.
	root := filepath.Join(t.TempDir(), "checkout-alias")
	if err := os.Symlink(t.TempDir(), root); err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, ".gitignore", ".openexec/\nnode_modules/\n")
	writeTestFile(t, root, "tsconfig.json", `{"compilerOptions":{"target":"ES2022"},"include":["src"]}`)
	writeTestFile(t, root, "src/main.ts", "export const example: number = 1\n")
	for _, args := range [][]string{
		{"init", "-q"}, {"add", "."},
		{"-c", "user.name=Public Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "public fixture"},
	} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %v %s", err, out)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(compiler, filepath.Join(root, "node_modules", "typescript")); err != nil {
		t.Skipf("compiler symlink unavailable: %v", err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	invoke := func(dir string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, executable, "-test.run=^TestConsoleNativeRequestCapture$", "-test.v", "-console-native-checkout="+root, "-console-capture-dir="+dir).CombinedOutput()
	}
	// Refuse output within source BEFORE creating it or opening graph storage.
	inside := filepath.Join(root, "bad-capture")
	if out, err := invoke(inside); err == nil || !bytes.Contains(out, []byte("capture directory must be outside")) {
		t.Fatalf("inside source not refused: %v %s", err, out)
	}
	if _, err := os.Stat(inside); !os.IsNotExist(err) {
		t.Fatal("refused capture directory was created")
	}
	if _, err := os.Stat(filepath.Join(root, ".openexec")); !os.IsNotExist(err) {
		t.Fatal("refused run created graph store")
	}
	redirect := filepath.Join(t.TempDir(), "redirect")
	if err := os.Symlink(root, redirect); err != nil {
		t.Fatal(err)
	}
	if out, err := invoke(filepath.Join(redirect, "bad-capture")); err == nil || !bytes.Contains(out, []byte("capture directory must be outside")) {
		t.Fatalf("symlink redirect not refused: %v %s", err, out)
	}
	dirty := filepath.Join(root, "untracked.ts")
	if err := os.WriteFile(dirty, []byte("export const uncommitted = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := invoke(filepath.Join(t.TempDir(), "dirty")); err == nil || !bytes.Contains(out, []byte("source must be a clean")) {
		t.Fatalf("dirty source not refused: %v %s", err, out)
	}
	if err := os.Remove(dirty); err != nil {
		t.Fatal(err)
	}
	if out, err := invoke(t.TempDir()); err == nil || !bytes.Contains(out, []byte("file exists")) {
		t.Fatalf("existing artifact directory not refused: %v %s", err, out)
	}
	dir := filepath.Join(t.TempDir(), "capture")
	if out, err := invoke(dir); err != nil {
		t.Fatalf("native driver failed: %v %s", err, out)
	}
	request, err := os.ReadFile(filepath.Join(dir, "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"native-child.json", "direct-child.json"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		var r nativeChildRecord
		if err := json.Unmarshal(b, &r); err != nil {
			t.Fatal(err)
		}
		if r.RequestSHA256 != captureHash(request) || r.RequestBytes != len(request) || r.Ending != "exit_0" || !r.ResponseDecoded || r.FileCount != 1 || r.ExtractedFiles != 1 || r.Bound != "45s" || r.NodeVersion == "" || r.CompilerVersion == "" || len(r.OSUsage) == 0 {
			t.Fatalf("incomplete child artifact: %s", b)
		}
	}
	identityBytes, err := os.ReadFile(filepath.Join(dir, "identity.json"))
	if err != nil {
		t.Fatal(err)
	}
	var identity map[string]any
	if err := json.Unmarshal(identityBytes, &identity); err != nil {
		t.Fatal(err)
	}
	if identity["executable_sha256"] != captureFileHash(executable) {
		t.Fatal("native executable not identified")
	}
	graphBytes, err := os.ReadFile(filepath.Join(dir, "graph.json"))
	if err != nil {
		t.Fatal(err)
	}
	var graph map[string]any
	if err := json.Unmarshal(graphBytes, &graph); err != nil {
		t.Fatal(err)
	}
	if graph["reopened"] != true || graph["compiler_fallback"] != false || graph["database"] != filepath.Join(canonicalRoot, ".openexec", "openexec.db") {
		t.Fatalf("persisted graph evidence missing: %s", graphBytes)
	}
	// Also prove the supplied alias reaches that exact persisted database.
	aliasDB, err := os.Stat(filepath.Join(root, ".openexec", "openexec.db"))
	if err != nil {
		t.Fatal(err)
	}
	canonicalDB, err := os.Stat(filepath.Join(canonicalRoot, ".openexec", "openexec.db"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(aliasDB, canonicalDB) {
		t.Fatal("supplied checkout alias reaches a different database")
	}
	// The same checkout must never overwrite an existing graph or its receipt.
	if out, err := invoke(filepath.Join(t.TempDir(), "second")); err == nil || !bytes.Contains(out, []byte("existing .openexec")) {
		t.Fatalf("existing store not refused: %v %s", err, out)
	}
	// Drive both comparison guards through the executable. A wrapper delegates
	// all parsing to real Node/TypeScript but changes exactly one input on the
	// second extraction. The native capture must never call that a controlled
	// same-input replay. These are instrumentation tests, not causal controls
	// for the Console incident.
	realNode, err := findCompatibleNode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENEXEC_CAPTURE_REAL_NODE", realNode)
	originalSource, err := os.ReadFile(filepath.Join(root, "src/main.ts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, mutation, refusal string }{
		{"source", `printf '\nexport const changed = 2\n' >> "$3/src/main.ts"`, "source/configuration changed"},
		{"toolchain", `printf '\n# changed\n' >> "$0"`, "toolchain hashes changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.RemoveAll(filepath.Join(root, ".openexec")); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "src/main.ts"), originalSource, 0o644); err != nil {
				t.Fatal(err)
			}
			bin := t.TempDir()
			wrapper := "#!/bin/sh\ncase \"$1\" in -e|--version|-p) exec \"$OPENEXEC_CAPTURE_REAL_NODE\" \"$@\";; esac\n" +
				"if [ -e \"$3/.openexec/native-observed\" ]; then\n" + tc.mutation + "\nelse : > \"$3/.openexec/native-observed\"; fi\n" +
				"exec \"$OPENEXEC_CAPTURE_REAL_NODE\" \"$@\"\n"
			if err := os.WriteFile(filepath.Join(bin, "node"), []byte(wrapper), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			if out, err := invoke(filepath.Join(t.TempDir(), "changed")); err == nil || !bytes.Contains(out, []byte(tc.refusal)) {
				t.Fatalf("changed %s comparison not refused: %v %s", tc.name, err, out)
			}
		})
	}
}
