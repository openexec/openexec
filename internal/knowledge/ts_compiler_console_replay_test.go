package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// consoleCheckout names an explicitly supplied, authorized, public checkout of
// the Console repository. Without it the driver test skips, so ordinary runs
// stay offline. Run it with:
//
//	go test ./internal/knowledge -run TestConsoleCheckoutExtractionRecord -count=1 -v -args -console-checkout=<dir>
var consoleCheckout = flag.String("console-checkout", "", "authorized public Console checkout for TestConsoleCheckoutExtractionRecord")

// replayExcludedParts mirrors the manifest rule used by the supervisor's
// external proof (docs/TYPESCRIPT_COMPILER_CANCELLATION_EVIDENCE.md).
var replayExcludedParts = map[string]bool{
	".git": true, ".openexec": true, ".gocache": true, "node_modules": true,
	"vendor": true, "dist": true, "build": true, "coverage": true, "_archive": true,
}

// replayManifest lists tracked and untracked-but-not-ignored TS/JS files of a
// git checkout, sorted, with hidden path parts (except .github) and the
// excluded directories removed.
func replayManifest(root string) ([]string, int64, error) {
	cmd := exec.Command("git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	out, err := cmd.Output()
	if err != nil {
		return nil, 0, fmt.Errorf("git ls-files: %w", err)
	}
	var files []string
	var bytesTotal int64
	for _, rel := range strings.Split(string(out), "\x00") {
		if rel == "" || !replayWanted(rel) {
			continue
		}
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, rel)
		bytesTotal += info.Size()
	}
	sort.Strings(files)
	return files, bytesTotal, nil
}

func replayWanted(rel string) bool {
	switch filepath.Ext(rel) {
	case ".ts", ".tsx", ".js", ".jsx":
	default:
		return false
	}
	for _, part := range strings.Split(rel, "/") {
		if replayExcludedParts[part] || (strings.HasPrefix(part, ".") && part != ".github") {
			return false
		}
	}
	return true
}

func TestReplayManifestSelectsOnlyTheRecordedFileSet(t *testing.T) {
	cases := map[string]bool{
		"ui/src/a.ts": true, "ui/src/b.tsx": true, "x.js": true, "y.jsx": true,
		".github/scripts/a.js": true,
		"ui/src/a.css":         false, "ui/src/a.d.mts": false, "README.md": false,
		"node_modules/p/i.js": false, "ui/node_modules/p/i.ts": false,
		"dist/a.js": false, "build/a.js": false, "coverage/a.js": false,
		"_archive/a.ts": false, "vendor/a.js": false, ".hidden/a.ts": false,
		".openexec/a.ts": false, "src/.cache/a.ts": false,
		// a file merely named like an excluded directory is not excluded
		"src/distance.ts": true, "src/builder.ts": true,
	}
	for rel, want := range cases {
		if got := replayWanted(rel); got != want {
			t.Errorf("replayWanted(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestReplayManifestIsDeterministicOverAGitFixture(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("b.ts", "export const b = 1\n")
	write("a.ts", "export const a = 1\n")
	write("node_modules/x/i.js", "x")
	write("notes.md", "n")
	write("ignored.ts", "export {}\n")
	write(".gitignore", "ignored.ts\n")
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git init: %v %s", err, out)
	}
	files, size, err := replayManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(files, ",") != "a.ts,b.ts" {
		t.Fatalf("manifest = %v, want [a.ts b.ts] (ignored, node_modules and non-TS excluded)", files)
	}
	if size != int64(len("export const a = 1\n")*2) {
		t.Fatalf("bytes = %d", size)
	}
}

// consoleReplayRecord is the safe, content-free measurement of one run.
type consoleReplayRecord struct {
	Checkout        string   `json:"checkout_name"`
	Head            string   `json:"head"`
	Command         []string `json:"command"`
	GoVersion       string   `json:"go_version"`
	Node            string   `json:"node_path"`
	NodeVersion     string   `json:"node_version"`
	TypeScript      string   `json:"typescript_path"`
	TypeScriptVer   string   `json:"typescript_version"`
	FileCount       int      `json:"file_count"`
	FileBytes       int64    `json:"file_bytes"`
	RequestSHA256   string   `json:"request_sha256"`
	ExcludedDirs    int      `json:"excluded_directories"`
	ParentDeadline  string   `json:"parent_deadline"`
	ChildDeadline   string   `json:"child_deadline"`
	ElapsedMillis   int64    `json:"elapsed_ms"`
	Extracted       int      `json:"extracted_files"`
	Configuration   string   `json:"configuration"`
	Limitations     int      `json:"limitations"`
	Error           string   `json:"error,omitempty"`
	Reproduced      bool     `json:"incident_signature_reproduced"`
	ParentCancelled bool     `json:"parent_cancelled"`
}

// incidentSignature is the ending the incident reported: the extractor killed
// by a signal. Any other error is a different failure, not a reproduction.
func incidentSignature(err error) bool {
	return err != nil && strings.Contains(err.Error(), "signal: killed")
}

func TestIncidentSignatureDistinguishesOtherFailures(t *testing.T) {
	if incidentSignature(nil) {
		t.Fatal("nil error matched")
	}
	if !incidentSignature(fmt.Errorf("TypeScript compiler extractor: signal: killed (...)")) {
		t.Fatal("signal: killed not matched")
	}
	if incidentSignature(fmt.Errorf("TypeScript compiler extractor: exit status 1 (extractor exited on its own)")) {
		t.Fatal("ordinary exit matched as incident signature")
	}
	if incidentSignature(fmt.Errorf("local TypeScript compiler unavailable")) {
		t.Fatal("missing compiler matched as incident signature")
	}
}

func commandLine(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "unobtainable: " + err.Error()
	}
	return strings.TrimSpace(string(out))
}

// TestConsoleCheckoutExtractionRecord runs the production extractor
// (extractTypeScriptWithCompiler, unchanged 45 s bound) on a supplied Console
// checkout and logs one JSON record. It fails only when the incident signature
// ("signal: killed") reproduces; any other outcome is recorded, not minimized.
func TestConsoleCheckoutExtractionRecord(t *testing.T) {
	if *consoleCheckout == "" {
		t.Skip("no -console-checkout supplied; this driver needs an authorized public Console checkout")
	}
	if runtime.GOOS == "windows" {
		t.Skip("driver uses POSIX tooling")
	}
	root, err := filepath.Abs(*consoleCheckout)
	if err != nil {
		t.Fatal(err)
	}
	files, size, err := replayManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("manifest for %s is empty", root)
	}
	ctx := context.Background()
	node, nodeErr := findCompatibleNode(ctx)
	compiler := findTypeScriptCompiler(root, files)
	excluded := make([]string, 0, len(graphIgnoredDirectories))
	for d := range graphIgnoredDirectories {
		excluded = append(excluded, d)
	}
	sort.Strings(excluded)
	request, _ := json.Marshal(tsCompilerRequest{Files: files, ExcludedDirectories: excluded})
	rec := consoleReplayRecord{
		Checkout:       filepath.Base(root),
		Head:           commandLine(t, "git", "-C", root, "rev-parse", "HEAD"),
		Command:        []string{"extractTypeScriptWithCompiler", "<console-checkout>"},
		GoVersion:      runtime.Version(),
		Node:           node,
		TypeScript:     compiler,
		FileCount:      len(files),
		FileBytes:      size,
		RequestSHA256:  fmt.Sprintf("%x", sha256.Sum256(request)),
		ExcludedDirs:   len(excluded),
		ParentDeadline: "none (context.Background)",
		ChildDeadline:  typeScriptExtractorTimeout.String(),
	}
	if nodeErr == nil {
		rec.NodeVersion = commandLine(t, node, "--version")
		if compiler != "" {
			rec.TypeScriptVer = commandLine(t, node, "-p", "require(process.argv[1]).version", compiler)
		}
	}
	started := time.Now()
	extracted, configuration, limitations, runErr := extractTypeScriptWithCompiler(ctx, root, files)
	rec.ElapsedMillis = time.Since(started).Milliseconds()
	rec.Extracted, rec.Configuration, rec.Limitations = len(extracted), configuration, len(limitations)
	rec.ParentCancelled = ctx.Err() != nil
	if runErr != nil {
		rec.Error = runErr.Error()
		rec.Reproduced = incidentSignature(runErr)
	}
	encoded, _ := json.MarshalIndent(rec, "", "  ")
	t.Logf("console extraction record:\n%s", encoded)
	if rec.Reproduced {
		t.Fatalf("incident signature reproduced: %s", rec.Error)
	}
	if runErr != nil {
		t.Logf("not the incident signature; recorded without minimizing")
	}
}
