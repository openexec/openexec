package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type compilerReplayIdentity struct {
	Manifest ScanManifest `json:"source_manifest"`
	Node     string       `json:"node_sha256"`
	Compiler string       `json:"typescript_sha256"`
}

// Report every failed comparison, without filenames, paths, hashes or bodies.
// This is the existing replay admission rule, split for diagnosis. It does not
// establish the historical mismatches of an older, combined refusal.
func compilerReplayIdentityDifferences(manifest ScanManifest, identity compilerReplayIdentity, r nativeChildRecord) []string {
	var differences []string
	for _, comparison := range []struct {
		name string
		same bool
	}{
		{"manifest.inputs", reflect.DeepEqual(manifest.Inputs, identity.Manifest.Inputs)},
		{"manifest.manifest_hash", manifest.ManifestHash == identity.Manifest.ManifestHash},
		{"manifest.worktree_state_hash", manifest.WorktreeStateHash == identity.Manifest.WorktreeStateHash},
		{"manifest.configuration_digest", manifest.ConfigurationDigest == identity.Manifest.ConfigurationDigest},
		{"identity.node_sha256/result.node_sha256", identity.Node == r.NodeSHA256},
		{"identity.typescript_sha256/result.typescript_sha256", identity.Compiler == r.CompilerSHA256},
		{"current.node_sha256/result.node_sha256", captureFileHash(r.Arguments[0]) == r.NodeSHA256},
		{"current.typescript_sha256/result.typescript_sha256", captureFileHash(r.Arguments[2]) == r.CompilerSHA256},
	} {
		if !comparison.same {
			differences = append(differences, comparison.name)
		}
	}
	return differences
}

// Reuses the native driver's ONE identical-stdin replay. It never scans and
// refuses a changed source/toolchain/script or an already attempted replay.
func replayPrivateCompilerCapture(t *testing.T, dir string) nativeChildRecord {
	t.Helper()
	var original struct {
		Child nativeChildRecord `json:"child"`
	}
	b, err := os.ReadFile(filepath.Join(dir, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &original); err != nil {
		t.Fatal(err)
	}
	r := original.Child
	request, err := os.ReadFile(filepath.Join(dir, "request.json"))
	if err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(filepath.Join(dir, "extractor.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	var identity compilerReplayIdentity
	b, err = os.ReadFile(filepath.Join(dir, "identity.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &identity); err != nil {
		t.Fatal(err)
	}
	if len(r.Arguments) != 4 || r.Bound != "45s" || typeScriptExtractorTimeout != 45*time.Second || captureHash(request) != r.RequestSHA256 || captureHash(script) != r.ScriptSHA256 || !bytes.Equal(script, typeScriptCompilerScript) {
		t.Fatal("capture request/script/argv/bound mismatch")
	}
	manifest, err := BuildScanManifest(r.Arguments[3])
	if err != nil {
		t.Fatal(err)
	}
	if differences := compilerReplayIdentityDifferences(manifest, identity, r); len(differences) != 0 {
		t.Fatalf("source/configuration/toolchain changed (%s); refusing misleading replay", strings.Join(differences, ", "))
	}
	// Reserve before launching: a second invocation must not repeat the probe.
	c := &compilerCapture{dir: dir}
	if err := c.write("replay.json", []byte(`{"status":"started; no result yet"}`)); err != nil {
		t.Fatal(err)
	}
	direct := replayNativeRequest(t, context.Background(), r.Arguments, request)
	enrichChildMeasurement(&direct)
	b, err = json.MarshalIndent(direct, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "replay.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if direct.Ending != "exit_0" || !direct.ResponseDecoded {
		t.Fatalf("captured request replay=%s; record retained privately; not a repair", direct.Ending)
	}
	return direct
}

func privateCaptureParent(t *testing.T) string {
	t.Helper()
	p := t.TempDir()
	if err := os.Chmod(p, 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

func readPrivateCapture(t *testing.T, parent string) (string, nativeChildRecord) {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 1 {
		t.Fatalf("capture entries=%v, err=%v", entries, err)
	}
	dir := filepath.Join(parent, entries[0].Name())
	var result struct {
		Child nativeChildRecord `json:"child"`
		After map[string]string `json:"process_after"`
	}
	b, err := os.ReadFile(filepath.Join(dir, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.After) == 0 {
		t.Fatal("no end snapshot")
	}
	for _, name := range []string{"", "request.json", "extractor.mjs", "identity.json", "result.json"} {
		info, err := os.Stat(filepath.Join(dir, name))
		want := os.FileMode(0o600)
		if name == "" {
			want = 0o700
		}
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("private mode %s: %v %v", name, info, err)
		}
	}
	return dir, result.Child
}

// No context observer is installed: this exercises the ordinary producer used
// by CLI scan, implicit graph builds and server refresh through this extractor.
func TestCompilerPrivateCaptureProducer(t *testing.T) {
	for _, tc := range []struct {
		name, body, ending string
		bound              time.Duration
		cancel             bool
	}{
		{"exit", `cat > "$3/consumed.json"; echo PRIVATE_STDERR >&2; exit 3`, "ordinary_nonzero_exit", 45 * time.Second, false},
		{"probe_after_ending", `cat > "$3/consumed.json"; exit 3`, "ordinary_nonzero_exit", 200 * time.Millisecond, false},
		{"deadline", `cat > "$3/consumed.json"; exec sleep 30`, "extractor_deadline", 200 * time.Millisecond, false},
		{"cancel", `cat > "$3/consumed.json"; exec sleep 30`, "caller_cancellation", 45 * time.Second, true},
		{"signal", `cat > "$3/consumed.json"; kill -9 $$`, "external_signal", 45 * time.Second, false},
		{"malformed", `cat > "$3/consumed.json"; printf PRIVATE_STDOUT`, "exit_0", 45 * time.Second, false},
		{"success", `cat > "$3/consumed.json"; printf '{"configuration":"fixture","files":[]}'`, "exit_0", 45 * time.Second, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probeDelay := ""
			if tc.name == "probe_after_ending" {
				probeDelay = "sleep 0.3; "
			}
			root := fakeNodeExtractor(t, `if [ "$1" = "--version" ]; then `+probeDelay+`echo v20.0.0; exit; fi
if [ "$1" = "-p" ]; then echo 5.8.3; exit; fi
`+tc.body)
			writeTestFile(t, root, "a.ts", "export const PRIVATE_SOURCE = 1\n")
			writeTestFile(t, root, "tsconfig.json", `{"include":["a.ts"]}`)
			parent := privateCaptureParent(t)
			t.Setenv(compilerCaptureEnv, parent)
			old := typeScriptExtractorTimeout
			typeScriptExtractorTimeout = tc.bound
			t.Cleanup(func() { typeScriptExtractorTimeout = old })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				time.AfterFunc(500*time.Millisecond, cancel)
			}
			_, _, _, runErr := extractTypeScriptWithCompiler(ctx, root, []string{"a.ts"})
			if (runErr == nil) != (tc.name == "success") {
				t.Fatalf("extract result: %v", runErr)
			}
			dir, record := readPrivateCapture(t, parent)
			consumed, err := os.ReadFile(filepath.Join(root, "consumed.json"))
			if err != nil {
				t.Fatal(err)
			}
			request, _ := os.ReadFile(filepath.Join(dir, "request.json"))
			if !bytes.Equal(request, consumed) || record.RequestSHA256 != captureHash(consumed) {
				t.Fatal("not actual child stdin")
			}
			if record.Ending != tc.ending || record.Bound != tc.bound.String() || record.ChildDeadline == "none" || record.ElapsedNS <= 0 || record.ProcessState == "" || len(record.OSUsage) == 0 {
				t.Fatalf("bad child record: %+v", record)
			}
			if record.NodeVersion != "v20.0.0" || record.CompilerVersion != "5.8.3" {
				t.Fatal("toolchain versions lost")
			}
			if tc.name == "probe_after_ending" && (record.ChildError != "none" || strings.Contains(runErr.Error(), "extractor deadline ")) {
				t.Fatalf("metadata probe ending: child_error=%q error=%v", record.ChildError, runErr)
			}
			if tc.name == "exit" && (record.ExitCode != 3 || record.StderrBytes == 0) {
				t.Fatal("nonzero exit lost")
			}
			var identity struct {
				ExecutableHash string       `json:"executable_sha256"`
				Manifest       ScanManifest `json:"source_manifest"`
				Node           string       `json:"node_sha256"`
				Compiler       string       `json:"typescript_sha256"`
				Script         string       `json:"script_sha256"`
			}
			b, _ := os.ReadFile(filepath.Join(dir, "identity.json"))
			if err := json.Unmarshal(b, &identity); err != nil {
				t.Fatal(err)
			}
			executable, _ := os.Executable()
			if identity.ExecutableHash != captureFileHash(executable) || identity.Manifest.ConfigurationDigest == "" || identity.Manifest.ManifestHash == "" || identity.Node != record.NodeSHA256 || identity.Compiler != record.CompilerSHA256 || identity.Script != captureHash(typeScriptCompilerScript) {
				t.Fatal("identity/provenance missing")
			}
			for _, name := range []string{"identity.json", "result.json", "request.json"} {
				b, _ := os.ReadFile(filepath.Join(dir, name))
				for _, secret := range []string{"PRIVATE_SOURCE", "PRIVATE_STDERR", "PRIVATE_STDOUT"} {
					if bytes.Contains(b, []byte(secret)) {
						t.Fatalf("%s contains source/output", name)
					}
				}
			}
			// Existing artifacts are never overwritten.
			if err := (&compilerCapture{dir: dir}).write("request.json", []byte("overwrite")); err == nil {
				t.Fatal("overwrote capture")
			}
		})
	}
}

func TestCompilerPrivateCaptureRefusalsAndDefault(t *testing.T) {
	root := fakeNodeExtractor(t, `exit 3`)
	request := []byte(`{"files":["a.ts"]}`)
	inside := filepath.Join(root, "private")
	if err := os.Mkdir(inside, 0o700); err != nil {
		t.Fatal(err)
	}
	wide := t.TempDir()
	if err := os.Chmod(wide, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(privateCaptureParent(t), link); err != nil {
		t.Fatal(err)
	}
	insideLink := filepath.Join(t.TempDir(), "inside-link")
	if err := os.Symlink(inside, insideLink); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"relative", inside, wide, link, insideLink, filepath.Join(t.TempDir(), "missing")} {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Setenv(compilerCaptureEnv, dir)
			if c, err := beginCompilerCapture(root, request, "node", "typescript"); c != nil || err == nil {
				t.Fatal("unsafe destination accepted")
			}
			_, _, _, err := extractTypeScriptWithCompiler(context.Background(), root, []string{"a.ts"})
			if err == nil || !strings.Contains(err.Error(), "exit status 3") {
				t.Fatalf("capture refusal changed extractor result: %v", err)
			}
		})
	}
	t.Setenv(compilerCaptureEnv, "")
	if c, err := beginCompilerCapture("nonexistent", request, "missing", "missing"); c != nil || err != nil {
		t.Fatal("default capture performed I/O")
	}
	// Similar prefix is outside root and must be accepted; containment is path-based.
	near := root + "-private"
	if err := os.Mkdir(near, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(near) })
	t.Setenv(compilerCaptureEnv, near)
	if c, err := beginCompilerCapture(root, request, "missing", "missing"); c == nil || err != nil {
		t.Fatalf("outside near miss refused: %v", err)
	}
}

func TestCompilerPrivateCaptureProbeBound(t *testing.T) {
	var b captureProbeOutput
	if n, err := b.Write(bytes.Repeat([]byte("a"), 20000)); n != 20000 || err != nil || len(b) != 4096 {
		t.Fatal("unbounded metadata")
	}
}

func TestCompilerPrivateCaptureReplayRefusals(t *testing.T) {
	root := fakeNodeExtractor(t, `if [ "$1" = "--version" ]; then echo v22; exit; fi
if [ "$1" = "-p" ]; then echo 5.8.3; exit; fi
cat >/dev/null; printf '{"configuration":"fixture","files":[]}'`)
	writeTestFile(t, root, "a.ts", "export const a = 1\n")
	parent := privateCaptureParent(t)
	t.Setenv(compilerCaptureEnv, parent)
	if _, _, _, err := extractTypeScriptWithCompiler(context.Background(), root, []string{"a.ts"}); err != nil {
		t.Fatal(err)
	}
	dir, _ := readPrivateCapture(t, parent)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	invoke := func(want string) {
		t.Helper()
		out, err := exec.Command(executable, "-test.run=^TestConsoleNativeRequestCapture$", "-compiler-private-capture="+dir).CombinedOutput()
		if err == nil || !bytes.Contains(out, []byte(want)) {
			t.Fatalf("refusal %s: err=%v output=%s", want, err, out)
		}
		if _, err := os.Stat(filepath.Join(dir, "replay.json")); !os.IsNotExist(err) {
			t.Fatal("refused replay launched or reserved")
		}
	}
	requestPath := filepath.Join(dir, "request.json")
	request, _ := os.ReadFile(requestPath)
	if err := os.WriteFile(requestPath, []byte(`{"files":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	invoke("capture request/script/argv/bound mismatch")
	if err := os.WriteFile(requestPath, request, 0o600); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "a.ts", "export const a = 2\n")
	invoke("source/configuration/toolchain changed")
	writeTestFile(t, root, "a.ts", "export const a = 1\n")
	resultPath := filepath.Join(dir, "result.json")
	originalResult, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var altered map[string]any
	if err := json.Unmarshal(originalResult, &altered); err != nil {
		t.Fatal(err)
	}
	altered["child"].(map[string]any)["node_sha256"] = "changed-toolchain"
	b, err := json.Marshal(altered)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, b, 0o600); err != nil {
		t.Fatal(err)
	}
	invoke("source/configuration/toolchain changed")
	if err := os.WriteFile(resultPath, originalResult, 0o600); err != nil {
		t.Fatal(err)
	}
	replayPrivateCompilerCapture(t, dir)
	out, err := exec.Command(executable, "-test.run=^TestConsoleNativeRequestCapture$", "-compiler-private-capture="+dir).CombinedOutput()
	if err == nil || !bytes.Contains(out, []byte("file exists")) {
		t.Fatalf("repeated replay allowed: %v %s", err, out)
	}
}

// Drive the public test executable entry point: each refusal must happen before
// the exclusive replay reservation AND before executing the child. The fixture
// is synthetic; none of these runs uses the retained natural Console capture.
func TestCompilerPrivateCaptureReplayComparisonDetails(t *testing.T) {
	root := fakeNodeExtractor(t, `if [ "$1" = "--version" ]; then echo v22; exit; fi
if [ "$1" = "-p" ]; then echo 5.8.3; exit; fi
cat >/dev/null; touch "$3/launched"; printf '{"configuration":"fixture","files":[]}'`)
	writeTestFile(t, root, "PRIVATE_SOURCE.ts", "export const PRIVATE_BODY = 1\n")
	writeTestFile(t, root, "tsconfig.json", `{"include":["PRIVATE_SOURCE.ts"]}`)
	parent := privateCaptureParent(t)
	t.Setenv(compilerCaptureEnv, parent)
	if _, _, _, err := extractTypeScriptWithCompiler(t.Context(), root, []string{"PRIVATE_SOURCE.ts"}); err != nil {
		t.Fatal(err)
	}
	dir, record := readPrivateCapture(t, parent)
	if err := os.Remove(filepath.Join(root, "launched")); err != nil {
		t.Fatal(err)
	}
	identityPath := filepath.Join(dir, "identity.json")
	identityBytes, err := os.ReadFile(identityPath)
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*testing.T, *compilerReplayIdentity)
		want   []string
	}{
		{"inputs", func(t *testing.T, i *compilerReplayIdentity) { i.Manifest.Inputs[0].Size++ }, []string{"manifest.inputs"}},
		{"manifest_hash", func(t *testing.T, i *compilerReplayIdentity) { i.Manifest.ManifestHash = "PRIVATE_HASH" }, []string{"manifest.manifest_hash"}},
		{"worktree_state", func(t *testing.T, i *compilerReplayIdentity) { i.Manifest.WorktreeStateHash = "PRIVATE_HASH" }, []string{"manifest.worktree_state_hash"}},
		{"configuration_digest", func(t *testing.T, i *compilerReplayIdentity) { i.Manifest.ConfigurationDigest = "PRIVATE_HASH" }, []string{"manifest.configuration_digest"}},
		{"node_record", func(t *testing.T, i *compilerReplayIdentity) { i.Node = "PRIVATE_HASH" }, []string{"identity.node_sha256/result.node_sha256"}},
		{"compiler_record", func(t *testing.T, i *compilerReplayIdentity) { i.Compiler = "PRIVATE_HASH" }, []string{"identity.typescript_sha256/result.typescript_sha256"}},
		{"node_file", func(t *testing.T, i *compilerReplayIdentity) { appendReplayToolchainFixture(t, record.Arguments[0]) }, []string{"current.node_sha256/result.node_sha256"}},
		{"compiler_file", func(t *testing.T, i *compilerReplayIdentity) { appendReplayToolchainFixture(t, record.Arguments[2]) }, []string{"current.typescript_sha256/result.typescript_sha256"}},
		{"source_drift", func(t *testing.T, i *compilerReplayIdentity) {
			writeTestFile(t, root, "PRIVATE_SOURCE.ts", "export const PRIVATE_BODY = 2\n")
			t.Cleanup(func() { writeTestFile(t, root, "PRIVATE_SOURCE.ts", "export const PRIVATE_BODY = 1\n") })
		}, []string{"manifest.inputs", "manifest.manifest_hash", "manifest.worktree_state_hash"}},
		{"configuration_drift", func(t *testing.T, i *compilerReplayIdentity) {
			writeTestFile(t, root, "tsconfig.json", `{"include":["PRIVATE_SOURCE.ts"],"compilerOptions":{"noEmit":true}}`)
			t.Cleanup(func() { writeTestFile(t, root, "tsconfig.json", `{"include":["PRIVATE_SOURCE.ts"]}`) })
		}, []string{"manifest.inputs", "manifest.manifest_hash", "manifest.worktree_state_hash", "manifest.configuration_digest"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var identity compilerReplayIdentity
			if err := json.Unmarshal(identityBytes, &identity); err != nil {
				t.Fatal(err)
			}
			tc.change(t, &identity)
			b, err := json.Marshal(identity)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(identityPath, b, 0o600); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.WriteFile(identityPath, identityBytes, 0o600); err != nil {
					t.Fatal(err)
				}
			})
			out, err := exec.Command(executable, "-test.run=^TestConsoleNativeRequestCapture$", "-compiler-private-capture="+dir).CombinedOutput()
			if err == nil || !strings.Contains(string(out), "changed ("+strings.Join(tc.want, ", ")+"); refusing misleading replay") {
				t.Fatalf("missing exact comparison details: err=%v output=%s", err, out)
			}
			if bytes.Contains(out, []byte("PRIVATE_")) || bytes.Contains(out, []byte(root)) || bytes.Contains(out, []byte(record.NodeSHA256)) || bytes.Contains(out, []byte(record.CompilerSHA256)) {
				t.Fatal("refusal leaked private identity values")
			}
			for _, name := range []string{filepath.Join(dir, "replay.json"), filepath.Join(root, "launched")} {
				if _, err := os.Stat(name); !os.IsNotExist(err) {
					t.Fatal("refused replay reserved or launched")
				}
			}
		})
	}
	// Near miss: mtime alone is not an identity difference. Unchanged bytes
	// must still reach the existing replay driver and decode its child response.
	if err := os.Chtimes(filepath.Join(root, "PRIVATE_SOURCE.ts"), time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	r := replayPrivateCompilerCapture(t, dir)
	if !r.ResponseDecoded || r.Ending != "exit_0" {
		t.Fatal("unchanged content refused")
	}
	if _, err := os.Stat(filepath.Join(root, "launched")); err != nil {
		t.Fatal("admitted child did not run")
	}
}

func appendReplayToolchainFixture(t *testing.T, name string) {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, append(append([]byte(nil), b...), []byte("\n# PRIVATE_TOOLCHAIN\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.WriteFile(name, b, 0o600); err != nil {
			t.Fatal(err)
		}
	})
}
