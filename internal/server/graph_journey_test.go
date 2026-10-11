package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/graphjourney"
	"github.com/openexec/openexec/pkg/version"
)

// The deploy-time journey walks the real handlers over a socket; these tests
// also prove each guard it relies on actually refuses a daemon that broke it.
func TestGraphJourneyWalksRunningDaemon(t *testing.T) {
	fixture := newGraphQueryFixture(t)
	daemon := httptest.NewServer(fixture.server.Mux)
	t.Cleanup(daemon.Close)
	cfg := graphjourney.Config{BaseURL: daemon.URL, Token: fixture.token, Query: "Caller"}

	evidence, err := graphjourney.Walk(t.Context(), cfg)
	if err != nil {
		t.Fatalf("walk: %v\nsteps=%+v", err, evidence.Steps)
	}
	if evidence.CheckoutID != fixture.identity.CheckoutID || evidence.SymbolName != "Caller" || evidence.GraphVersion == "" || evidence.SourceSHA256 == "" {
		t.Fatalf("evidence = %+v", evidence)
	}
	byName := map[string]graphjourney.Step{}
	for _, step := range evidence.Steps {
		byName[step.Name] = step
	}
	for name, want := range map[string]int{
		"calls outgoing": http.StatusOK, "calls incoming": http.StatusOK, "impact": http.StatusOK, "source": http.StatusOK,
		"refuse missing bearer": http.StatusUnauthorized, "refuse other checkout": http.StatusForbidden,
		"refuse fabricated symbol": http.StatusNotFound, "refuse invalid direction": http.StatusBadRequest,
	} {
		if byName[name].Status != want {
			t.Fatalf("step %q status = %d, want %d (steps=%+v)", name, byName[name].Status, want, evidence.Steps)
		}
	}
	// Caller calls Target and nothing calls Caller: the two directions must
	// answer different questions over the wire.
	if byName["calls outgoing"].Nodes == 0 || byName["calls incoming"].Nodes != 0 {
		t.Fatalf("outgoing nodes=%d incoming nodes=%d", byName["calls outgoing"].Nodes, byName["calls incoming"].Nodes)
	}

	// The evidence names the build that answered and keeps each answer, but a
	// served source body is kept as a digest only.
	if evidence.Commit != version.Commit || byName["running build"].Status != http.StatusOK {
		t.Fatalf("evidence commit = %q, running build step = %+v", evidence.Commit, byName["running build"])
	}
	for _, name := range []string{"search", "calls incoming", "impact", "refuse missing bearer", "refuse other checkout"} {
		if len(byName[name].Response) == 0 || byName[name].ResponseSHA256 == "" {
			t.Fatalf("step %q recorded no response: %+v", name, byName[name])
		}
	}
	if source := byName["source"]; len(source.Response) != 0 || source.ResponseSHA256 == "" {
		t.Fatalf("source step copied the body or lost its digest: %+v", source)
	}

	again, err := graphjourney.Requery(t.Context(), cfg, evidence)
	if err != nil || again.SourceSHA256 != evidence.SourceSHA256 || again.GraphVersion != evidence.GraphVersion || again.Commit != version.Commit {
		t.Fatalf("requery err=%v evidence=%+v, prior=%+v", err, again, evidence)
	}
}

// The deploy passes the commit it installed: a daemon still running another
// build (the restart did not take) must fail the walk and the re-query before
// any graph read is judged.
func TestGraphJourneyRefusesAnotherRunningBuild(t *testing.T) {
	fixture := newGraphQueryFixture(t)
	daemon := httptest.NewServer(fixture.server.Mux)
	t.Cleanup(daemon.Close)
	cfg := graphjourney.Config{BaseURL: daemon.URL, Token: fixture.token, Query: "Caller", WantCommit: version.Commit}
	evidence, err := graphjourney.Walk(t.Context(), cfg)
	if err != nil {
		t.Fatalf("walk with the running commit: %v", err)
	}
	cfg.WantCommit = "deployed-but-not-running"
	if got, err := graphjourney.Walk(t.Context(), cfg); err == nil || !strings.Contains(err.Error(), "running build") || len(got.Steps) != 1 {
		t.Fatalf("walk err = %v steps = %+v, want a running-build refusal before any graph read", err, got.Steps)
	}
	if _, err := graphjourney.Requery(t.Context(), cfg, evidence); err == nil || !strings.Contains(err.Error(), "running build") {
		t.Fatalf("requery err = %v, want a running-build refusal", err)
	}
}

func TestGraphJourneyRequeryAfterDriftAnswersCurrentSource(t *testing.T) {
	fixture := newGraphQueryFixture(t)
	daemon := httptest.NewServer(fixture.server.Mux)
	t.Cleanup(daemon.Close)
	cfg := graphjourney.Config{BaseURL: daemon.URL, Token: fixture.token, Query: "Caller"}
	evidence, err := graphjourney.Walk(t.Context(), cfg)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	drifted := "package sample\n\nfunc Target() {}\nfunc Caller() { Target(); Target() }\nfunc Run() {}\n"
	if err := os.WriteFile(filepath.Join(fixture.identity.RootPath, "main.go"), []byte(drifted), 0o600); err != nil {
		t.Fatal(err)
	}
	again, err := graphjourney.Requery(t.Context(), cfg, evidence)
	if err != nil {
		t.Fatalf("requery after drift: %v steps=%+v", err, again.Steps)
	}
	// Either the held identity survives into a newer generation with the new
	// bytes, or the daemon refused it explicitly; never old bytes as current.
	if again.SourceSHA256 != "" && (again.GraphVersion == evidence.GraphVersion || again.SourceSHA256 == evidence.SourceSHA256) {
		t.Fatalf("drifted requery = %+v, prior = %+v", again, evidence)
	}
}

func TestGraphJourneyRefusesBrokenDaemons(t *testing.T) {
	fixture := newGraphQueryFixture(t)
	cases := map[string]func(http.Handler) http.Handler{
		// A daemon that stopped requiring the bearer.
		"bearer not enforced": func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+fixture.token)
				next.ServeHTTP(w, r)
			})
		},
		// A daemon that answers whatever checkout it serves for any caller.
		"checkout not enforced": func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-OpenExec-Checkout-ID") != "" {
					r.Header.Set("X-OpenExec-Checkout-ID", fixture.identity.CheckoutID)
				}
				next.ServeHTTP(w, r)
			})
		},
		// A proxy that labels answers with a cached generation instead of the
		// one OpenExec answered from.
		"generation header from cache": func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-OpenExec-Graph-Version", "cached-projection-version")
				next.ServeHTTP(headerPinned{w}, r)
			})
		},
		// A daemon that answers an invalid direction instead of refusing it.
		"direction not validated": func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("direction") == "sideways" {
					r.URL.RawQuery = strings.Replace(r.URL.RawQuery, "sideways", "outgoing", 1)
				}
				next.ServeHTTP(w, r)
			})
		},
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			daemon := httptest.NewServer(breakIt(fixture.server.Mux))
			t.Cleanup(daemon.Close)
			evidence, err := graphjourney.Walk(t.Context(), graphjourney.Config{BaseURL: daemon.URL, Token: fixture.token, Query: "Caller"})
			if err == nil {
				t.Fatalf("walk accepted a broken daemon: %+v", evidence.Steps)
			}
			if strings.HasPrefix(err.Error(), "running build") {
				t.Fatalf("walk refused before reaching the broken guard: %v", err)
			}
			t.Logf("refused: %v", err)
		})
	}
}

func TestGraphJourneyRequeryRefusesChangedBytesUnderHeldVersion(t *testing.T) {
	fixture := newGraphQueryFixture(t)
	daemon := httptest.NewServer(fixture.server.Mux)
	t.Cleanup(daemon.Close)
	cfg := graphjourney.Config{BaseURL: daemon.URL, Token: fixture.token, Query: "Caller"}
	evidence, err := graphjourney.Walk(t.Context(), cfg)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	evidence.SourceSHA256 = "bytes-the-caller-held-under-this-version"
	if _, err := graphjourney.Requery(t.Context(), cfg, evidence); err == nil || !strings.Contains(err.Error(), "unchanged graph version") {
		t.Fatalf("requery err = %v, want changed-bytes refusal", err)
	}
}

// Dependencies answer "what does this symbol use", calls answer "what does it
// invoke": a symbol that only takes Target as a value depends on it without
// calling it. Pinning this keeps the two endpoints from collapsing into one.
func TestRepositoryGraphDependenciesIncludeReferencesThatCallsExclude(t *testing.T) {
	fixture := newGraphQueryFixture(t)
	holder := "package sample\n\nfunc Holder() { f := Target; _ = f }\n"
	if err := os.WriteFile(filepath.Join(fixture.identity.RootPath, "holder.go"), []byte(holder), 0o600); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/repository-graph/symbols/" + graphSymbolID(t, fixture, "Holder")
	calls, _ := relationshipNames(t, graphRequest(t, fixture, base+"/calls?direction=outgoing", fixture.identity.CheckoutID))
	if calls["Target"] {
		t.Fatalf("outgoing calls for Holder = %v, want no call to Target", calls)
	}
	dependencies, result := relationshipNames(t, graphRequest(t, fixture, base+"/dependencies", fixture.identity.CheckoutID))
	if !dependencies["Target"] {
		t.Fatalf("dependencies for Holder = %v, want the referenced Target", dependencies)
	}
	for _, edge := range result.Result.Edges {
		if edge.Type != "calls" && edge.Type != "references" {
			t.Fatalf("dependency edge type %q outside calls/references", edge.Type)
		}
	}
}

// headerPinned keeps a header the wrapper set from being replaced by the
// handler, modelling a proxy that overwrites the upstream generation label.
type headerPinned struct{ http.ResponseWriter }

func (h headerPinned) WriteHeader(status int) {
	h.ResponseWriter.Header().Set("X-OpenExec-Graph-Version", "cached-projection-version")
	h.ResponseWriter.WriteHeader(status)
}

func (h headerPinned) Write(body []byte) (int, error) {
	h.ResponseWriter.Header().Set("X-OpenExec-Graph-Version", "cached-projection-version")
	return h.ResponseWriter.Write(body)
}
