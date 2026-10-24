package knowledge_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/pkg/db/sqlitecfg"
)

// The reviewed graph deploy of 24 October 2026 restarted onto a build whose
// extractor version had moved (repository-graph-v2.3 -> v2.4). Its first
// repository-context read had to re-extract the whole repository — measured
// ~45s for this repository — inside the journey's 30s request deadline. The
// cancelled read discarded the half-built generation and had already
// labelled the previous build's generation stale, so the restored build had
// to rescan too. These tests reproduce that state on a repository large
// enough that its extraction outlasts the deadlines they impose.

// previousExtractor is the version the build before the migration wrote.
const previousExtractor = "repository-graph-v2.3"

type migrationFixture struct {
	root       string
	store      *knowledge.Store
	identity   knowledge.RepositoryIdentity
	db         *sql.DB
	previousID string
	// extraction is how long one full extraction of this fixture took here.
	extraction time.Duration
}

// writeBulkGoSources writes packages of small Go files that call each other,
// so extraction stores real symbols and call edges for every file.
func writeBulkGoSources(t *testing.T, root string, files int) {
	t.Helper()
	padding := strings.Repeat("// realistic padding so the input set is hashed, not only listed.\n", 12)
	for i := 0; i < files; i++ {
		pkg := fmt.Sprintf("pkg%03d", i/40)
		next := i + 1
		if next%40 == 0 {
			next = i - 39
		}
		source := fmt.Sprintf("package %s\n\n%stype Kind%d struct{ Name string `json:\"name\"` }\n\nfunc Run%d(value string) string { return Step%d(value) }\n\nfunc Step%d(value string) string { return value }\n\nfunc Use%d() string { return Run%d(\"x\") }\n", pkg, padding, i, i, i, i, i, next)
		writeFile(t, root, filepath.Join(pkg, fmt.Sprintf("f%04d.go", i)), source)
	}
}

func newMigrationFixture(t *testing.T) migrationFixture {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	writeBulkGoSources(t, root, 150)
	store, err := knowledge.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	started := time.Now()
	scanned, err := store.ScanRepository(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	extraction := time.Since(started)
	identity, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", sqlitecfg.DSN(filepath.Join(root, ".openexec", "openexec.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	// The generation as the previous build left it: current, for its extractor.
	if _, err := db.Exec(`UPDATE graph_generations SET extractor_version = ? WHERE id = ?`, previousExtractor, scanned.Generation.ID); err != nil {
		t.Fatal(err)
	}
	t.Logf("full extraction of the fixture took %s", extraction)
	return migrationFixture{root: root, store: store, identity: identity, db: db, previousID: scanned.Generation.ID, extraction: extraction}
}

// deadline is a caller deadline the migration's extraction cannot meet.
func (f migrationFixture) deadline() time.Duration { return f.extraction / 5 }

func (f migrationFixture) status(t *testing.T, generationID string) string {
	t.Helper()
	var status string
	if err := f.db.QueryRow(`SELECT status FROM graph_generations WHERE id = ?`, generationID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

// current returns the extractor and id of the one current generation, or ""
// when no generation is current.
func (f migrationFixture) current(t *testing.T) (extractor, id string) {
	t.Helper()
	rows, err := f.db.Query(`SELECT extractor_version, id FROM graph_generations WHERE worktree_id = ? AND status = 'current'`, f.identity.WorktreeID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
		if err := rows.Scan(&extractor, &id); err != nil {
			t.Fatal(err)
		}
	}
	if count > 1 {
		t.Fatalf("%d generations are current", count)
	}
	return extractor, id
}

func (f migrationFixture) readContext(ctx context.Context) (knowledge.RepositoryContextProjection, error) {
	return f.store.BuildRepositoryContext(ctx, f.identity, nil, "", "", "", nil)
}

// A reader that gives up must not throw away the migration it started: the
// next reader, under the same short deadline, is answered from the new
// generation. Before the fix every such reader cancelled the extraction and
// none ever finished.
func TestReadRefreshOutlivesCancelledReader(t *testing.T) {
	f := newMigrationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), f.deadline())
	defer cancel()
	_, _ = f.readContext(ctx)
	if ctx.Err() == nil {
		t.Fatalf("the read finished inside %s, so the fixture does not outlast the deadline", f.deadline())
	}
	extractor, id := f.current(t)
	if extractor != knowledge.ExtractorVersion {
		t.Fatalf("after the cancelled read the current generation is %q (%s), want the migrated %q", id, extractor, knowledge.ExtractorVersion)
	}
	if got := f.status(t, f.previousID); got != "superseded" {
		t.Fatalf("previous generation is %q, want superseded by the migration", got)
	}
	again, cancelAgain := context.WithTimeout(context.Background(), f.deadline())
	defer cancelAgain()
	projection, err := f.readContext(again)
	if err != nil {
		t.Fatalf("read after the migration under the same deadline: %v", err)
	}
	if projection.Freshness != knowledge.FreshnessCurrent || projection.GraphVersion != id {
		t.Fatalf("projection = %s/%s, want current %s", projection.GraphVersion, projection.Freshness, id)
	}
}

// waitForBuilding blocks until a generation other than known is building,
// which is after the read gate decided to refresh.
func (f migrationFixture) waitForBuilding(t *testing.T, done <-chan error) {
	t.Helper()
	for {
		var building int
		if err := f.db.QueryRow(`SELECT COUNT(*) FROM graph_generations WHERE worktree_id = ? AND status = 'building'`, f.identity.WorktreeID).Scan(&building); err != nil {
			t.Fatal(err)
		}
		if building > 0 {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("refresh finished (%v) before it was observed building", err)
		case <-time.After(time.Millisecond):
		}
	}
}

// While a newer build re-extracts, the previous build's generation stays
// current for the previous extractor: a rollback at that moment restores a
// build that answers at once instead of rescanning too. Drifted inputs, the
// near miss, still make it stale for every reader.
func TestExtractorMigrationLeavesPreviousGenerationCurrent(t *testing.T) {
	cases := []struct {
		name  string
		drift func(t *testing.T, f migrationFixture)
		want  string
	}{
		{name: "extractor changed", drift: func(*testing.T, migrationFixture) {}, want: "current"},
		{name: "inputs changed", drift: func(t *testing.T, f migrationFixture) {
			if _, err := f.db.Exec(`UPDATE graph_generations SET extractor_version = ? WHERE id = ?`, knowledge.ExtractorVersion, f.previousID); err != nil {
				t.Fatal(err)
			}
			writeFile(t, f.root, filepath.Join("pkg000", "f0000.go"), "package pkg000\n\nfunc Run0(value string) string { return value }\n")
		}, want: "stale"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newMigrationFixture(t)
			c.drift(t, f)
			done := make(chan error, 1)
			go func() {
				_, err := f.readContext(context.Background())
				done <- err
			}()
			f.waitForBuilding(t, done)
			if got := f.status(t, f.previousID); got != c.want {
				t.Fatalf("while refreshing, the previous generation is %q, want %q", got, c.want)
			}
			if err := <-done; err != nil {
				t.Fatalf("refresh: %v", err)
			}
		})
	}
}

// A refresh that outlives its reader still revalidates: inputs that change
// while it builds are refused as stale, nothing partial or inconsistent is
// promoted, and the previous generation is left as it was.
func TestReadRefreshRefusesInputsChangedWhileReaderGaveUp(t *testing.T) {
	f := newMigrationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), f.deadline())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := f.readContext(ctx)
		done <- err
	}()
	f.waitForBuilding(t, done)
	<-ctx.Done()
	changed := "package pkg001\n\nfunc Run40(value string) string { return value + \"changed\" }\n"
	writeFile(t, f.root, filepath.Join("pkg001", "f0040.go"), changed)
	err := <-done
	if !knowledge.IsStaleGraph(err) {
		t.Fatalf("read across a concurrent change = %v, want an explicit stale refusal", err)
	}
	if extractor, id := f.current(t); id != f.previousID || extractor != previousExtractor {
		t.Fatalf("current generation = %s (%s), want the untouched previous %s", id, extractor, f.previousID)
	}
	var promotedPartial int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM graph_generations WHERE worktree_id = ? AND status IN ('partial','building')`, f.identity.WorktreeID).Scan(&promotedPartial); err != nil {
		t.Fatal(err)
	}
	if promotedPartial != 0 {
		t.Fatalf("%d partial or building generations left behind", promotedPartial)
	}
	projection, err := f.readContext(context.Background())
	if err != nil || projection.Freshness != knowledge.FreshnessCurrent {
		t.Fatalf("read after the change settled: %+v %v", projection.Freshness, err)
	}
	manifest, err := knowledge.BuildScanManifest(f.root)
	if err != nil {
		t.Fatal(err)
	}
	var manifestHash string
	if err := f.db.QueryRow(`SELECT manifest_hash FROM graph_generations WHERE id = ?`, projection.GraphVersion).Scan(&manifestHash); err != nil {
		t.Fatal(err)
	}
	if manifestHash != manifest.ManifestHash {
		t.Fatalf("current generation describes manifest %s, the repository is %s", manifestHash, manifest.ManifestHash)
	}
}

// An explicit scan is still the caller's to cancel. A cancelled one leaves no
// current or partial generation of its own, and a reopened store migrates
// from the untouched previous generation.
func TestCancelledScanLeavesNoFalseGenerationAcrossReopen(t *testing.T) {
	f := newMigrationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), f.deadline())
	defer cancel()
	if _, err := f.store.ScanRepository(ctx, f.root); err == nil {
		t.Fatalf("scan finished inside %s, so the fixture does not outlast the deadline", f.deadline())
	}
	if extractor, id := f.current(t); id != f.previousID || extractor != previousExtractor {
		t.Fatalf("after the cancelled scan the current generation is %s (%s), want the previous %s", id, extractor, f.previousID)
	}
	var leftover int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM graph_generations WHERE worktree_id = ? AND status IN ('partial','building')`, f.identity.WorktreeID).Scan(&leftover); err != nil {
		t.Fatal(err)
	}
	if leftover != 0 {
		t.Fatalf("cancelled scan left %d partial or building generations", leftover)
	}
	if err := f.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := knowledge.NewStore(f.root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	projection, err := reopened.BuildRepositoryContext(context.Background(), f.identity, nil, "", "", "", nil)
	if err != nil || projection.Freshness != knowledge.FreshnessCurrent {
		t.Fatalf("reopened read: %+v %v", projection.Freshness, err)
	}
	if extractor, id := f.current(t); extractor != knowledge.ExtractorVersion || id != projection.GraphVersion {
		t.Fatalf("reopened read answered %s, current is %s (%s)", projection.GraphVersion, id, extractor)
	}
}
