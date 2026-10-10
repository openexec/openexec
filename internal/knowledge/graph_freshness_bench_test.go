package knowledge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The V2.1 gate recomputes the scan manifest on every read: it lists the
// inputs and reads and hashes every one, so a gated read costs O(input bytes)
// before its indexed query runs. These benchmarks report that cost against
// the ungated query it guards. Run with:
//
//	go test ./internal/knowledge -run '^$' -bench GatedRead -benchmem

func benchmarkRepository(b *testing.B, files int) (*Store, RepositoryIdentity, GraphGeneration) {
	b.Helper()
	ctx := context.Background()
	root := b.TempDir()
	// ~2 KiB per file: a small Go source file with a few functions.
	body := strings.Repeat("// padding line to give the file a realistic size for hashing.\n", 28)
	for i := 0; i < files; i++ {
		dir := filepath.Join(root, fmt.Sprintf("pkg%03d", i/50))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.Fatal(err)
		}
		source := fmt.Sprintf("package pkg%03d\n\n%sfunc Run%d(value string) string { return value }\n", i/50, body, i)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d.go", i)), []byte(source), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	store, err := NewStore(root)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { store.Close() })
	if _, err := store.ScanRepository(ctx, root); err != nil {
		b.Fatal(err)
	}
	identity, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		b.Fatal(err)
	}
	generation, _, err := store.freshGeneration(ctx, identity)
	if err != nil {
		b.Fatal(err)
	}
	return store, identity, generation
}

func BenchmarkGatedRead(b *testing.B) {
	for _, files := range []int{100, 1000} {
		store, identity, generation := benchmarkRepository(b, files)
		ctx := context.Background()
		b.Run(fmt.Sprintf("files=%d/ungated_resolve", files), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := store.resolveGraphSymbolIn(ctx, generation, "Run1", "", "", 20); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("files=%d/gate_only", files), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, _, err := store.freshGeneration(ctx, identity); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("files=%d/gated_resolve", files), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := store.ResolveGraphSymbol(ctx, identity, "Run1", "", "", 20); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("files=%d/gated_repository_context", files), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := store.BuildRepositoryContext(ctx, identity, []string{"Run1"}, "", "", "", nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkGatedReadManifestThisRepository measures the gate's dominant cost
// on a real checkout — this one — without scanning it.
func BenchmarkGatedReadManifestThisRepository(b *testing.B) {
	root, err := filepath.Abs("../..")
	if err != nil {
		b.Fatal(err)
	}
	manifest, err := BuildScanManifest(root)
	if err != nil {
		b.Skipf("manifest unavailable: %v", err)
	}
	var bytes int64
	for _, input := range manifest.Inputs {
		bytes += input.Size
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := BuildScanManifest(root); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(len(manifest.Inputs)), "inputs")
	b.ReportMetric(float64(bytes)/(1<<20), "input_MiB")
}
