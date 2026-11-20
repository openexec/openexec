package execution

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	d26CaptureSHA     = "0cbc9da8b5a6ed7d03db2b4a0a8ac16818f1c648f3db776090c83651bba20708"
	d26SentinelSecret = "SENTINEL-PRIVATE-CONTENT-9137"
)

func runProvision(t *testing.T, args ...string) (int, string) {
	t.Helper()
	script, err := filepath.Abs("../../scripts/provision-d26-capture.sh")
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("sh", append([]string{script}, args...)...).CombinedOutput()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatal(err)
		}
		code = ee.ExitCode()
	}
	return code, string(out)
}

// The provisioning script refuses wrong and missing files under the real
// recorded hash, and installs only on an exact match (throwaway files only).
func TestD26ProvisioningScript(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out", "capture.json")
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"x":"`+d26SentinelSecret+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out := runProvision(t, "--dest", dest, bad)
	if code == 0 || strings.Contains(out, d26SentinelSecret) || !strings.Contains(out, d26CaptureSHA) {
		t.Fatalf("wrong file not refused cleanly: code=%d out=%q", code, out)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Fatal("wrong file was installed")
	}
	if code, _ := runProvision(t, "--dest", dest, filepath.Join(dir, "absent.json")); code == 0 {
		t.Fatal("missing candidate accepted")
	}
	if _, err := os.Stat(dest); err == nil {
		t.Fatal("missing candidate installed something")
	}
	// Near miss: expected hash differs from the file's by one character.
	body := []byte("throwaway-provisioning-bytes")
	good := filepath.Join(dir, "good.json")
	if err := os.WriteFile(good, body, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(body))
	last := "0"
	if sum[63] == '0' {
		last = "1"
	}
	if code, _ := runProvision(t, "--expected-sha256", sum[:63]+last, "--dest", dest, good); code == 0 {
		t.Fatal("near-miss hash accepted")
	}
	if _, err := os.Stat(dest); err == nil {
		t.Fatal("near-miss installed")
	}
	if code, out := runProvision(t, "--expected-sha256", sum, "--dest", dest, good); code != 0 {
		t.Fatalf("exact match refused: %q", out)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != string(body) {
		t.Fatal("installed bytes differ")
	}
}

func d26GoBin(t *testing.T) string {
	t.Helper()
	p := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	lp, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not found for scratch run")
	}
	return lp
}

// d26Throwaway builds a throwaway events file whose projection equals the public
// structural chronology, so the scratch copy of the test reaches the capture check.
func d26Throwaway(t *testing.T) []byte {
	t.Helper()
	wantRaw, err := os.ReadFile("testdata/d26-structure.json")
	if err != nil {
		t.Fatal(err)
	}
	var want []d26StructuralEvent
	if err := json.Unmarshal(wantRaw, &want); err != nil {
		t.Fatal(err)
	}
	type tool struct {
		CallID     string              `json:"callId"`
		Name       string              `json:"name"`
		OutputTail string              `json:"outputTail,omitempty"`
		Input      map[string][]string `json:"input,omitempty"`
	}
	type ev struct {
		Sequence  int    `json:"sequence"`
		CreatedAt string `json:"createdAt"`
		Type      string `json:"type"`
		Text      string `json:"text,omitempty"`
		Tool      *tool  `json:"tool,omitempty"`
	}
	var evs []ev
	for _, w := range want {
		e := ev{Sequence: w.Sequence, CreatedAt: w.CreatedAt, Type: w.Type}
		if w.Type == "turn.error" || w.Type == "turn.failed" {
			e.Text = "context_overflow: assembled request bytes=693918 estimated_tokens=296242 context=0 dispatched=true"
		}
		if w.Type == "tool.started" || w.Type == "tool.completed" {
			e.Tool = &tool{CallID: fmt.Sprintf("c%d", w.Call), Name: w.ToolName, OutputTail: strings.Repeat("x", w.OutputTailBytes)}
			if w.Executable != "" {
				e.Tool.Input = map[string][]string{"command": {w.Executable}}
			}
		}
		evs = append(evs, e)
	}
	raw, err := json.Marshal(evs)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// d26ScratchRun copies the unchanged D26 test into a scratch module, points its
// absolute private paths at a temp tree, swaps in the throwaway events hash, and
// runs it. The real paths and the real test are never touched.
func d26ScratchRun(t *testing.T, withEvents bool, capture []byte) (string, bool) {
	t.Helper()
	scratch := t.TempDir()
	priv := filepath.Join(scratch, "private") + "/"
	runReq := filepath.Join(scratch, "run-requests")
	pkg := filepath.Join(scratch, "pkg")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{priv, runReq, filepath.Join(pkg, "testdata")} {
		must(os.MkdirAll(d, 0o755))
	}
	src, err := os.ReadFile("d26_evidence_test.go")
	must(err)
	events := d26Throwaway(t)
	text := string(src)
	for _, r := range [][2]string{
		{"/home/perttu/.agent-console/perttu/scratch/20261031T163533Z/", priv},
		{"/home/perttu/.agent-console/run-requests/", runReq + "/"},
		{"b089c85d67c841d464a7e3a1f0eb199c1b9472682ddb75c683d4f86768260014", fmt.Sprintf("%x", sha256.Sum256(events))},
	} {
		if !strings.Contains(text, r[0]) {
			t.Fatalf("expected path/hash not found in test source: %s", r[0])
		}
		text = strings.ReplaceAll(text, r[0], r[1])
	}
	must(os.WriteFile(filepath.Join(scratch, "go.mod"), []byte("module scratch\n\ngo 1.21\n"), 0o644))
	must(os.WriteFile(filepath.Join(pkg, "d26_evidence_test.go"), []byte(text), 0o644))
	structure, err := os.ReadFile("testdata/d26-structure.json")
	must(err)
	must(os.WriteFile(filepath.Join(pkg, "testdata", "d26-structure.json"), structure, 0o644))
	if withEvents {
		must(os.WriteFile(priv+"d26-events.json", events, 0o600))
	}
	if capture != nil {
		must(os.WriteFile(filepath.Join(runReq, "d26bd137975612543c90e5b983f60db8.json"), capture, 0o600))
	}
	cmd := exec.Command(d26GoBin(t), "test", "-count=1", "-v", "-run", "^TestD26PrivateOccurrenceEvidence$", "./pkg")
	cmd.Dir = scratch
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}

func TestD26ScratchBadCapturesFailClosed(t *testing.T) {
	cases := []struct {
		name    string
		capture []byte
		message string
	}{
		{"wrong-bytes-not-json", []byte("not json " + d26SentinelSecret), "required private artifact unavailable or malformed"},
		{"wrong-content-valid-json", []byte(`{"truncated":true,"note":"` + d26SentinelSecret + `"}`), "capture fidelity or dispatch facts changed"},
		{"missing-capture-events-present", nil, "required private artifact unavailable or malformed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, pass := d26ScratchRun(t, true, c.capture)
			if pass || !strings.Contains(out, "--- FAIL: TestD26PrivateOccurrenceEvidence") || strings.Contains(out, "SKIP") {
				t.Fatalf("scratch run did not fail closed (pass=%v)", pass)
			}
			if !strings.Contains(out, c.message) {
				t.Fatalf("expected fixed message %q not shown", c.message)
			}
			if strings.Contains(out, d26SentinelSecret) {
				t.Fatal("failure output leaked capture content")
			}
			t.Logf("scratch %s: FAIL (not skip), go test exit non-zero, fixed message %q, no content leaked", c.name, c.message)
		})
	}
	// Control: with the events file absent the same test skips, which is why the
	// missing-capture demonstration keeps the events file present.
	out, pass := d26ScratchRun(t, false, nil)
	if !pass || !strings.Contains(out, "--- SKIP: TestD26PrivateOccurrenceEvidence") {
		t.Fatal("control: absent events file should skip")
	}
	t.Log("control: events file absent -> SKIP (not a failure), so the missing-capture case keeps events present")
}
