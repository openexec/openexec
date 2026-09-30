package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRetentionUnitCaptureAndRedaction(t *testing.T) {
	for _, size := range []int{0, StreamLimit - 1, StreamLimit, StreamLimit + 1, StreamLimit * 3} {
		var b Buffer
		input := strings.Repeat("x", size)
		for i := 0; i < len(input); i += 17 {
			part := input[i:min(i+17, len(input))]
			n, err := b.Write([]byte(part))
			if n != len(part) || err != nil {
				t.Fatal(n, err)
			}
		}
		if b.Len() != min(size, StreamLimit) || b.String() != input[:min(size, StreamLimit)] || b.Truncated != (size > StreamLimit) {
			t.Fatal("capture boundary", size)
		}
		if size > StreamLimit && PublicStream(&b, nil) != "[truncated]" {
			t.Fatal("partial line exposed")
		}
	}
	var b Buffer
	b.Write([]byte("safe\n" + strings.Repeat("x", StreamLimit)))
	if got := PublicStream(&b, nil); got != "safe\n[truncated]" {
		t.Fatal(got)
	}
	for _, key := range []string{"password", "passwd", "token", "secret", "api_key", "API-KEY", "authorization"} {
		for _, value := range []string{"sentinel", `"sentinel with spaces"`, "'sentinel with spaces'"} {
			command := key + "=" + value
			secrets := CommandSecrets(command)
			if len(secrets) != 1 || strings.Contains(Public(command+" "+secrets[0], append(secrets, "")), "sentinel") {
				t.Fatal("redaction", command)
			}
		}
	}
	if Public(strings.Repeat("x", StreamLimit+1), nil) != strings.Repeat("x", StreamLimit) {
		t.Fatal("public limit")
	}
	if len(CommandSecrets("echo safe")) != 0 {
		t.Fatal("unexpected secret")
	}
	if got := Public("missing_verifier_command_98312: not found", CommandSecrets("missing_verifier_command_98312")); got != "missing_verifier_command_98312: not found" {
		t.Fatal("command name redacted", got)
	}
	values := map[string]string{"go_version": strings.Repeat("v", 200), "node_version": "v22", "npm_version": "10", "python_version": "3", "rustc_version": "1", "PATH": "private"}
	got := Toolchain(values)
	if len(got) != 5 || len(got["go_version"]) != metadataLimit || got["node_version"] != "v22" {
		t.Fatal(got)
	}
}

func TestRetentionUnitPrivateRoundTripAndRefusals(t *testing.T) {
	dir := t.TempDir()
	want := Command{Argv: []string{"tool", "argument with spaces", "token=private"}, Cwd: dir, ExitCode: 2, Stdout: strings.Repeat("o", StreamLimit+1), Stderr: strings.Repeat("e", StreamLimit+1), Toolchain: map[string]string{"go_version": "v1", "PATH": "excluded"}}
	hash, path, err := Write(dir, want)
	if err != nil {
		t.Fatal(err)
	}
	want.Stdout = want.Stdout[:StreamLimit]
	want.Stderr = want.Stderr[:StreamLimit]
	want.StdoutTruncated = true
	want.StderrTruncated = true
	delete(want.Toolchain, "PATH")
	got, err := Read(dir, hash)
	if err != nil || !reflect.DeepEqual(got, &want) {
		t.Fatalf("roundtrip: %+v %v", got, err)
	}
	for p, mode := range map[string]os.FileMode{filepath.Dir(path): 0700, path: 0600} {
		info, err := os.Stat(p)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatal("permissions", p, err)
		}
	}
	second, _, err := Write(dir, want)
	if err != nil || second != hash {
		t.Fatal("identity unstable", err)
	}
	for _, bad := range []string{"", "../escape", strings.ToUpper(hash), strings.Repeat("0", 64)} {
		if _, err := Read(dir, bad); err == nil {
			t.Fatal("invalid identity accepted", bad)
		}
	}
	if _, err := Read(filepath.Join(dir, "missing"), hash); err == nil {
		t.Fatal("missing root")
	}
	if _, err := Read(t.TempDir(), hash); err == nil {
		t.Fatal("missing evidence directory")
	}
	if _, _, err := Write(filepath.Join(dir, "missing"), Command{}); err == nil {
		t.Fatal("missing root write")
	}
	if _, _, err := Write(dir, Command{Argv: []string{strings.Repeat("x", maxEvidenceBytes)}}); err == nil {
		t.Fatal("oversized evidence")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir, hash); err == nil {
		t.Fatal("public file accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir, hash); err == nil {
		t.Fatal("corrupt evidence accepted")
	}
	raw := []byte("not JSON")
	sum := sha256.Sum256(raw)
	badHash := hex.EncodeToString(sum[:])
	if err := os.WriteFile(filepath.Join(dir, directory, badHash+".json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir, badHash); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	if err := os.Chmod(filepath.Join(dir, directory), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir, hash); err == nil {
		t.Fatal("public directory accepted")
	}
	if _, _, err := Write(dir, Command{}); err == nil {
		t.Fatal("public directory written")
	}
}
