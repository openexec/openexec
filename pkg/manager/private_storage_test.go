package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/openexec/openexec/pkg/runtime"
)

func TestPrivateStorageReferenceReload(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "protected", true: "legacy-refused"}[legacy], func(t *testing.T) {
			f := newRecaptureFixture(t, "exit 0")
			ctx := context.Background()
			hash, path, err := runtime.RetainCommandEvidence(f.env.dir, runtime.CommandEvidence{Argv: []string{"sh", "-c", "exit 2"}, Cwd: f.env.dir, ExitCode: 2})
			if err != nil {
				t.Fatal(err)
			}
			if legacy {
				old := filepath.Join(f.env.dir, ".openexec-verification", hash+".json")
				if err = os.MkdirAll(filepath.Dir(old), 0700); err != nil {
					t.Fatal(err)
				}
				if err = os.Rename(path, old); err != nil {
					t.Fatal(err)
				}
				path = old
			}
			if err = f.env.mgr.state.RecordArtifact(ctx, hash, "test_log", path, 0); err != nil {
				t.Fatal(err)
			}
			refs := recaptureReceipt("verify", 2)
			refs[hash] = path
			f.restart(t)
			task, err := f.env.rel.TaskSnapshot(ctx, "A")
			if err != nil {
				t.Fatal(err)
			}
			command, err := f.env.mgr.resolveRecaptureCommand(task, "verify", refs)
			if legacy {
				if err == nil || command != "" {
					t.Fatal("legacy command replay accepted")
				}
				if _, err = runtime.ReadCommandEvidence(f.env.dir, hash); err == nil {
					t.Fatal("legacy private read accepted")
				}
			} else if err != nil || command != "exit 2" {
				t.Fatal("protected command lost after reload")
			}
			if f.env.mgr.diagnosticFreeReceipt(refs) != legacy {
				t.Fatal("incorrect private context availability")
			}
			ref, err := f.env.mgr.state.GetArtifact(ctx, hash)
			if err != nil || ref == nil || ref.Path != path {
				t.Fatal("persisted reference changed")
			}
			if _, err = os.Stat(path); err != nil {
				t.Fatal("original evidence removed")
			}
		})
	}
}
