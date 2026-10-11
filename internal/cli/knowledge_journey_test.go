package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadGraphBearerAcceptsBareTokenAndDaemonEnvironmentFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	for name, body := range map[string]string{
		"bare":   "secret-token\n",
		"env":    "OPENEXEC_REPOSITORY_EVIDENCE_TOKEN=other\nOPENEXEC_REPOSITORY_GRAPH_TOKEN=secret-token\n",
		"quoted": "export OPENEXEC_REPOSITORY_GRAPH_TOKEN=\"secret-token\"\n",
	} {
		token, err := readGraphBearer(write(name, body))
		if err != nil || token != "secret-token" {
			t.Fatalf("%s: token=%q err=%v", name, token, err)
		}
	}
	// An environment file that only carries the external evidence bearer must
	// not authenticate the graph plane with it.
	if token, err := readGraphBearer(write("evidence-only", "OPENEXEC_REPOSITORY_EVIDENCE_TOKEN=other\n")); err == nil {
		t.Fatalf("evidence-only file yielded graph bearer %q", token)
	}
	if _, err := readGraphBearer(write("empty", "\n")); err == nil {
		t.Fatal("empty bearer file accepted")
	}
}
