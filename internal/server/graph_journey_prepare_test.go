package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/graphjourney"
	"github.com/openexec/openexec/internal/knowledge"
)

// The reviewed deploy of 24 October 2026 restarted onto a build whose graph
// extractor had moved on. Its journey's first repository-context read had to
// re-extract the whole repository (~45s measured for this repository) inside
// the journey's 30s request deadline, and the deploy rolled back. These tests
// rebuild that state against the real handlers, with one process-scoped graph
// store as production has, and a repository whose extraction outlasts a
// request deadline scaled to it.
type migratedDaemon struct {
	fixture    graphQueryFixture
	store      *knowledge.Store
	extraction time.Duration
	deadline   time.Duration
}

func newMigratedDaemon(t *testing.T) migratedDaemon {
	t.Helper()
	fixture := newGraphQueryFixture(t)
	store, err := knowledge.NewStoreWithDB(fixture.server.StateStore.GetDB())
	if err != nil {
		t.Fatal(err)
	}
	fixture.server.KnowledgeStore = store
	root := fixture.identity.RootPath
	// Extraction must cost far more than a walk on every platform. Each read
	// rehashes every input file and the repository context probes exported
	// symbols and call edges, so files, bytes and functions slow the walk as
	// much as extraction. Every json-tagged field is an ambiguous candidate for
	// each same-named field, which extraction stores as one edge per pair and
	// a walk only counts: a few dense files of unexported tagged types make
	// extraction expensive without making the walk so.
	const files, types = 12, 20
	for i := 0; i < files; i++ {
		dir := filepath.Join(root, fmt.Sprintf("pkg%03d", i/4))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		next := i + 1
		if next%4 == 0 {
			next = i - 3
		}
		var source strings.Builder
		fmt.Fprintf(&source, "package pkg%03d\n", i/4)
		for j := 0; j < types; j++ {
			fmt.Fprintf(&source, "\ntype kind%d_%d struct {\n\tname  string `json:\"name\"`\n\tlabel string `json:\"label\"`\n}\n", i, j)
		}
		fmt.Fprintf(&source, "\nfunc Run%d(value string) string { return Step%d(value) }\n\nfunc Step%d(value string) string { return value }\n\nfunc Use%d() string { return Run%d(\"x\") }\n", i, i, i, i, next)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%04d.go", i)), []byte(source.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	started := time.Now()
	if _, err := store.ScanRepository(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	extraction := time.Since(started)
	// The slowest answer a current graph gives the journey: the whole walk
	// bounds each of its requests.
	current := httptest.NewServer(fixture.server.Mux)
	started = time.Now()
	_, walkErr := graphjourney.Walk(t.Context(), graphjourney.Config{BaseURL: current.URL, Token: fixture.token, Query: "Caller"})
	walk := time.Since(started)
	current.Close()
	if walkErr != nil {
		t.Fatalf("walk against the current graph: %v", walkErr)
	}
	// The generation as the previous build left it: current for its extractor.
	if _, err := fixture.server.StateStore.GetDB().Exec(`UPDATE graph_generations SET extractor_version = 'repository-graph-v2.3' WHERE status = 'current'`); err != nil {
		t.Fatal(err)
	}
	// Every request gets a deadline a current graph meets with room to spare
	// and the migration's extraction cannot, as 30s is for this repository.
	deadline := 2 * walk
	t.Logf("full extraction of the fixture took %s, a current walk %s; request deadline %s", extraction, walk, deadline)
	if extraction < 2*deadline {
		t.Fatalf("extraction %s does not outlast the request deadline %s; the fixture proves nothing", extraction, deadline)
	}
	return migratedDaemon{fixture: fixture, store: store, extraction: extraction, deadline: deadline}
}

// config gives every request the journey makes the fixture's deadline.
func (d migratedDaemon) config(t *testing.T, mux http.Handler) graphjourney.Config {
	daemon := httptest.NewServer(mux)
	t.Cleanup(daemon.Close)
	return graphjourney.Config{BaseURL: daemon.URL, Token: d.fixture.token, Query: "Caller",
		Client: &http.Client{Timeout: d.deadline}}
}

// The old ordering: the walk's first read pays for the migration and times out.
func TestGraphJourneyWithoutPreparationTimesOutOnExtractorMigration(t *testing.T) {
	d := newMigratedDaemon(t)
	evidence, err := graphjourney.Walk(t.Context(), d.config(t, d.fixture.server.Mux))
	if err == nil || !strings.HasPrefix(err.Error(), "repository context:") {
		t.Fatalf("walk err = %v, want the repository context read to miss its deadline (steps=%+v)", err, evidence.Steps)
	}
}

// The corrected route: preparation waits out the migration the daemon keeps
// running, then the same journey and re-query pass under the same deadline,
// answered by the migrated generation.
func TestPreparedGraphJourneyWalksAfterExtractorMigration(t *testing.T) {
	d := newMigratedDaemon(t)
	cfg := d.config(t, d.fixture.server.Mux)
	prepared, err := graphjourney.Prepare(t.Context(), cfg, 30*d.extraction)
	if err != nil {
		t.Fatalf("prepare: %v (steps=%+v)", err, prepared.Steps)
	}
	// Without a request that missed its deadline the test would prove nothing.
	if len(prepared.Steps) < 3 || prepared.Steps[1].Status != 0 {
		t.Fatalf("preparation never outlasted a request deadline: %+v", prepared.Steps)
	}
	if last := prepared.Steps[len(prepared.Steps)-1]; last.Status != http.StatusOK || prepared.CheckoutID != d.fixture.identity.CheckoutID {
		t.Fatalf("preparation ended on %+v for checkout %q", last, prepared.CheckoutID)
	}
	var extractor string
	if err := d.fixture.server.StateStore.GetDB().QueryRow(`SELECT extractor_version FROM graph_generations WHERE id = ? AND status = 'current'`, prepared.GraphVersion).Scan(&extractor); err != nil || extractor != knowledge.ExtractorVersion {
		t.Fatalf("prepared generation %s extractor=%q err=%v, want current %s", prepared.GraphVersion, extractor, err, knowledge.ExtractorVersion)
	}
	walked, err := graphjourney.Walk(t.Context(), cfg)
	if err != nil {
		t.Fatalf("walk after preparation: %v (steps=%+v)", err, walked.Steps)
	}
	if walked.GraphVersion != prepared.GraphVersion {
		t.Fatalf("journey answered from %s, prepared %s", walked.GraphVersion, prepared.GraphVersion)
	}
	if again, err := graphjourney.Requery(t.Context(), cfg, walked); err != nil || again.SourceSHA256 != walked.SourceSHA256 {
		t.Fatalf("requery err=%v evidence=%+v", err, again)
	}
}

// Preparation waits only for a request that missed its deadline. An answer
// that refuses is final: it fails at once, after one read, and never turns a
// stale or missing graph into a pass.
func TestGraphPreparationRefusesWithoutWaiting(t *testing.T) {
	// Refusal needs no slow migration: with refresh off the first read answers
	// at once, so these cases build the small graph and never time extraction.
	prepareSmall := func(t *testing.T, refresh, migrated bool) (graphjourney.Evidence, error) {
		fixture := newGraphQueryFixture(t)
		store, err := knowledge.NewStoreWithDB(fixture.server.StateStore.GetDB())
		if err != nil {
			t.Fatal(err)
		}
		store.SetRefreshOnRead(refresh)
		fixture.server.KnowledgeStore = store
		if migrated {
			// The generation as the previous build left it: current for its extractor.
			if _, err := fixture.server.StateStore.GetDB().Exec(`UPDATE graph_generations SET extractor_version = 'repository-graph-v2.3' WHERE status = 'current'`); err != nil {
				t.Fatal(err)
			}
		}
		daemon := httptest.NewServer(fixture.server.Mux)
		t.Cleanup(daemon.Close)
		return graphjourney.Prepare(t.Context(), graphjourney.Config{BaseURL: daemon.URL, Token: fixture.token}, time.Minute)
	}
	t.Run("stale graph the daemon will not refresh", func(t *testing.T) {
		prepared, err := prepareSmall(t, false, true)
		if err == nil || !strings.Contains(err.Error(), "answered 409 (graph_stale)") || len(prepared.Steps) != 2 {
			t.Fatalf("prepare err = %v steps = %+v, want one explicit stale refusal", err, prepared.Steps)
		}
	})
	// Near misses: the refusal comes from the stale generation and the
	// disabled refresh together, not from the small fixture.
	t.Run("stale graph the daemon refreshes", func(t *testing.T) {
		if prepared, err := prepareSmall(t, true, true); err != nil {
			t.Fatalf("prepare err = %v steps = %+v, want the refreshed graph", err, prepared.Steps)
		}
	})
	t.Run("current graph with refresh off", func(t *testing.T) {
		if prepared, err := prepareSmall(t, false, false); err != nil {
			t.Fatalf("prepare err = %v steps = %+v, want the current graph", err, prepared.Steps)
		}
	})
	t.Run("graph never built for the checkout", func(t *testing.T) {
		fixture := newGraphQueryFixture(t)
		fixture.server.ProjectsDir = t.TempDir()
		daemon := httptest.NewServer(fixture.server.Mux)
		t.Cleanup(daemon.Close)
		prepared, err := graphjourney.Prepare(t.Context(), graphjourney.Config{BaseURL: daemon.URL, Token: fixture.token}, time.Minute)
		if err == nil || !strings.Contains(err.Error(), "answered 404 (graph_missing)") || len(prepared.Steps) != 2 {
			t.Fatalf("prepare err = %v steps = %+v, want one explicit missing refusal", err, prepared.Steps)
		}
	})
	t.Run("wrong bearer", func(t *testing.T) {
		fixture := newGraphQueryFixture(t)
		daemon := httptest.NewServer(fixture.server.Mux)
		t.Cleanup(daemon.Close)
		prepared, err := graphjourney.Prepare(t.Context(), graphjourney.Config{BaseURL: daemon.URL, Token: "not-the-graph-bearer"}, time.Minute)
		if err == nil || !strings.Contains(err.Error(), "answered 401") || len(prepared.Steps) != 2 {
			t.Fatalf("prepare err = %v steps = %+v, want one unauthorized refusal", err, prepared.Steps)
		}
	})
	t.Run("migration longer than the preparation budget", func(t *testing.T) {
		d := newMigratedDaemon(t)
		prepared, err := graphjourney.Prepare(t.Context(), d.config(t, d.fixture.server.Mux), d.extraction/3)
		if err == nil || !strings.Contains(err.Error(), "graph not current within") {
			t.Fatalf("prepare err = %v steps = %+v, want the budget to run out", err, prepared.Steps)
		}
	})
	t.Run("answer that is not current", func(t *testing.T) {
		fixture := newGraphQueryFixture(t)
		relabel := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/repository-context" {
				fixture.server.Mux.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"checkout_id":%q,"graph_version":"graph_cached","freshness":"stale"}`, fixture.identity.CheckoutID)
		})
		daemon := httptest.NewServer(relabel)
		t.Cleanup(daemon.Close)
		if _, err := graphjourney.Prepare(t.Context(), graphjourney.Config{BaseURL: daemon.URL, Token: fixture.token}, time.Minute); err == nil || !strings.Contains(err.Error(), `freshness "stale"`) {
			t.Fatalf("prepare err = %v, want a refusal of a stale answer", err)
		}
	})
}
