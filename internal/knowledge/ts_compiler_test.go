package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFindCompatibleNodeSkipsObsoleteRuntime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses POSIX executables")
	}
	root := t.TempDir()
	oldDirectory := filepath.Join(root, "old")
	newDirectory := filepath.Join(root, "new")
	for _, directory := range []string{oldDirectory, newDirectory} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(oldDirectory, "node"), []byte("#!/bin/sh\nprintf 6\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	newNode := filepath.Join(newDirectory, "node")
	if err := os.WriteFile(newNode, []byte("#!/bin/sh\nprintf 20\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", oldDirectory+string(os.PathListSeparator)+newDirectory)
	t.Setenv("NVM_BIN", "")
	t.Setenv("NVM_DIR", "")
	t.Setenv("VOLTA_HOME", "")

	found, err := findCompatibleNode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if found != newNode {
		t.Fatalf("compatible Node = %q, want %q", found, newNode)
	}
}

// fakeNodeExtractor installs a POSIX "node" that reports major version 20 for
// the probe and runs body for the extractor invocation.
func fakeNodeExtractor(t *testing.T, body string) (root string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses POSIX executables")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = \"-e\" ]; then printf 20; exit 0; fi\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NVM_BIN", "")
	t.Setenv("NVM_DIR", "")
	t.Setenv("VOLTA_HOME", "")
	root = t.TempDir()
	compiler := filepath.Join(root, "node_modules", "typescript", "lib")
	if err := os.MkdirAll(compiler, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(compiler, "typescript.js"), []byte("//"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestExtractorFailureNamesWhoEndedIt(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		timeout time.Duration
		cancel  bool
		want    string
		notWant string
	}{
		{name: "deadline", body: "exec sleep 30", timeout: 300 * time.Millisecond, want: "extractor deadline 300ms exceeded", notWant: "killed from outside"},
		{name: "external kill", body: "kill -9 $$", timeout: 30 * time.Second, want: "killed from outside this process", notWant: "exceeded"},
		{name: "caller cancel", body: "exec sleep 30", timeout: 30 * time.Second, cancel: true, want: "caller context ended", notWant: "killed from outside"},
		{name: "plain failure", body: "echo boom >&2; exit 3", timeout: 30 * time.Second, want: "exited on its own", notWant: "killed from outside"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := fakeNodeExtractor(t, tc.body)
			old := typeScriptExtractorTimeout
			typeScriptExtractorTimeout = tc.timeout
			t.Cleanup(func() { typeScriptExtractorTimeout = old })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				time.AfterFunc(200*time.Millisecond, cancel)
			}
			_, _, _, err := extractTypeScriptWithCompiler(ctx, root, []string{"a.ts"})
			if err == nil {
				t.Fatal("expected extractor failure")
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.want) {
				t.Fatalf("error %q lacks %q", msg, tc.want)
			}
			if tc.notWant != "" && strings.Contains(msg, tc.notWant) {
				t.Fatalf("error %q must not contain %q", msg, tc.notWant)
			}
			if tc.name == "plain failure" && (!strings.Contains(msg, "exit status 3") || strings.Contains(msg, "signal")) {
				t.Fatalf("exit 3 misdescribed: %q", msg)
			}
			if tc.name == "plain failure" && !strings.Contains(msg, "boom") {
				t.Fatalf("stderr dropped: %q", msg)
			}
		})
	}
}
