package runtime

import (
	"reflect"
	"strings"
	"testing"
)

func TestRetentionUnitPublicEvidenceAPI(t *testing.T) {
	dir := t.TempDir()
	want := CommandEvidence{Argv: []string{"sh", "-c", "exit 2"}, Cwd: dir, ExitCode: 2, Stderr: "private token=sentinel", Toolchain: map[string]string{}}
	hash, path, err := RetainCommandEvidence(dir, want)
	if err != nil || path == "" {
		t.Fatal(err)
	}
	got, err := ReadCommandEvidence(dir, hash)
	if err != nil || !reflect.DeepEqual(got, &want) {
		t.Fatalf("roundtrip: %+v %v", got, err)
	}
	var b EvidenceBuffer
	b.Write([]byte(want.Stderr))
	if public := PublicVerificationStream(&b, nil); strings.Contains(public, "sentinel") || !strings.Contains(public, "[REDACTED]") {
		t.Fatal(public)
	}
	if _, err := ReadCommandEvidence(dir, "bad"); err == nil {
		t.Fatal("invalid identity")
	}
}
