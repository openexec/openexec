package knowledge

import (
	"encoding/json"
	"os"
	"path"
	"reflect"
	"testing"
	"time"
)

// This checks the integrity of transcribed, supervisor-supplied metadata. It
// does not authenticate private artifacts or claim to re-execute Console.
// The executable driver/real compiler tests separately exercise the real path.
func TestNativeCaptureSuppliedConsoleRecordIntegrity(t *testing.T) {
	b, err := os.ReadFile("../../docs/typescript-native-request-20261029.json")
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		Cases []struct {
			Case      string            `json:"case"`
			Head      string            `json:"source_head"`
			Artifacts map[string]string `json:"artifact_sha256"`
			Native    nativeChildRecord `json:"native-child"`
			Direct    nativeChildRecord `json:"direct-child"`
			Graph     struct {
				Database     string            `json:"database"`
				Reopened     bool              `json:"reopened"`
				Fallback     bool              `json:"compiler_fallback"`
				ScanError    bool              `json:"scan_error"`
				SameInputs   bool              `json:"source_inputs_unchanged_after_direct"`
				Status       string            `json:"status"`
				Capabilities map[string]string `json:"capabilities"`
			} `json:"graph"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(b, &record); err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{
		"new-deadline":   "2ebcc487ae0a2cd588840f10902038d34da5fc8d",
		"original-train": "8ec35a445faec933bc0a129216f6ea45453cbcb9",
		"recorded-main":  "bbbda1c6e7d746c4bb605f3ddb736283828811fb",
	}
	if len(record.Cases) != len(expected) {
		t.Fatal("missing or additional external execution case")
	}
	for _, c := range record.Cases {
		t.Run(c.Case, func(t *testing.T) {
			if expected[c.Case] != c.Head || c.Head == "" {
				t.Fatal("unexpected or duplicate case/source identity")
			}
			delete(expected, c.Case)
			for _, name := range []string{"identity.json", "request.json", "native-child.json", "direct-child.json", "graph.json"} {
				if len(c.Artifacts[name]) != 64 {
					t.Fatalf("missing original artifact hash: %s", name)
				}
			}
			n, d := c.Native, c.Direct
			if n.RequestSHA256 != c.Artifacts["request.json"] || n.RequestSHA256 != d.RequestSHA256 || n.RequestBytes != d.RequestBytes || n.FileCount != d.FileCount || n.SourceBytes != d.SourceBytes || !reflect.DeepEqual(n.Arguments, d.Arguments) || n.NodeSHA256 != d.NodeSHA256 || n.CompilerSHA256 != d.CompilerSHA256 || n.ScriptSHA256 != d.ScriptSHA256 || n.Configuration != d.Configuration {
				t.Fatal("same-input comparison identity differs")
			}
			for _, r := range []nativeChildRecord{n, d} {
				if r.Ending != "exit_0" || r.ExitCode != 0 || !r.ResponseDecoded || r.ParentError != "none" || r.ChildError != "none" || r.Bound != "45s" || r.ElapsedNS <= 0 || r.FileCount != r.ExtractedFiles || r.RequestBytes <= 0 || r.SourceBytes <= 0 || len(r.OSUsage) == 0 {
					t.Fatal("non-reproduction metadata incomplete or inconsistent")
				}
				parent, pErr := time.Parse(time.RFC3339Nano, r.ParentDeadline)
				child, cErr := time.Parse(time.RFC3339Nano, r.ChildDeadline)
				if pErr != nil || cErr != nil || !parent.After(child) || !child.After(r.Started.Add(time.Duration(r.ElapsedNS))) {
					t.Fatal("successful child must end before both deadlines")
				}
				// The context deadline is set just before the recorded start.
				if allowance := child.Sub(r.Started); allowance < 44*time.Second || allowance > 45*time.Second {
					t.Fatal("recorded bound differs from unchanged 45s allowance")
				}
			}
			g := c.Graph
			if len(n.Arguments) != 4 || g.Database != path.Join(n.Arguments[3], ".openexec", "openexec.db") || !g.Reopened || g.Fallback || g.ScanError || !g.SameInputs || g.Status != "current" {
				t.Fatal("persisted graph evidence incomplete")
			}
			for _, capability := range []string{"typescript.definitions", "typescript.imports", "typescript.exports"} {
				if g.Capabilities[capability] != "compiler_exact" {
					t.Fatalf("compiler success unsupported: %s", capability)
				}
			}
		})
	}
}
