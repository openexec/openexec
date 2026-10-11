// Package graphjourney walks the Explore journey Agent Console drives against a
// running repository-graph daemon: search, explicit selection, both call
// directions, impact and source, plus the refusals that must still hold. The
// reviewed graph deploy runs it after a restart so a merged build produces its
// own live acceptance evidence instead of relying on a manual walk.
package graphjourney

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const graphVersionHeader = "X-OpenExec-Graph-Version"

// Config names one running daemon. Token is the graph-plane bearer the daemon
// was started with; the checkout identity is read from the daemon itself.
type Config struct {
	BaseURL string
	Token   string
	Query   string
	Client  *http.Client
}

// Step records one exchange. Only statuses, provenance and digests are kept:
// evidence must not copy source bodies or credentials into deploy logs.
type Step struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	Status       int    `json:"status"`
	Want         int    `json:"want"`
	GraphVersion string `json:"graph_version,omitempty"`
	Freshness    string `json:"freshness,omitempty"`
	Nodes        int    `json:"nodes,omitempty"`
	Truncated    bool   `json:"truncated,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

type Evidence struct {
	BaseURL      string    `json:"base_url"`
	At           time.Time `json:"at"`
	CheckoutID   string    `json:"checkout_id"`
	SymbolID     string    `json:"symbol_id"`
	SymbolName   string    `json:"symbol_name"`
	GraphVersion string    `json:"graph_version"`
	SourceSHA256 string    `json:"source_sha256"`
	Steps        []Step    `json:"steps"`
}

type envelope struct {
	Provenance struct {
		GraphVersion string `json:"graph_version"`
		Freshness    string `json:"freshness"`
	} `json:"provenance"`
	Truncated  bool            `json:"truncated"`
	Pagination json.RawMessage `json:"pagination"`
	Reason     string          `json:"reason"`
	Result     struct {
		Candidates []struct {
			Symbol struct {
				ID          string `json:"symbol_id"`
				DisplayName string `json:"display_name"`
			} `json:"symbol"`
		} `json:"candidates"`
		Symbol struct {
			ID string `json:"symbol_id"`
		} `json:"symbol"`
		Nodes  []json.RawMessage `json:"nodes"`
		Source struct {
			Content   string `json:"content"`
			RangeHash string `json:"source_range_hash"`
		} `json:"source"`
	} `json:"result"`
}

type walker struct {
	cfg      Config
	evidence *Evidence
}

// Walk runs the journey once and returns its evidence. The returned error
// names the first step that did not answer as an authorized, current,
// self-identifying graph read or refusal; evidence up to that step is kept.
func Walk(ctx context.Context, cfg Config) (Evidence, error) {
	w, err := newWalker(cfg)
	if err != nil {
		return Evidence{}, err
	}
	ev := w.evidence
	var projection struct {
		CheckoutID string `json:"checkout_id"`
	}
	if _, err := w.get(ctx, "repository context", "/api/v1/repository-context", "", cfg.Token, http.StatusOK, &projection); err != nil {
		return *ev, err
	}
	if projection.CheckoutID == "" {
		return *ev, fmt.Errorf("repository context did not name a checkout")
	}
	ev.CheckoutID = projection.CheckoutID

	search, err := w.graph(ctx, "search", "/symbols?"+url.Values{"q": {cfg.Query}, "page_size": {"5"}}.Encode(), http.StatusOK)
	if err != nil {
		return *ev, err
	}
	if len(search.Pagination) == 0 || string(search.Pagination) == "null" {
		return *ev, fmt.Errorf("search for %q did not disclose pagination", cfg.Query)
	}
	if len(search.Result.Candidates) == 0 {
		return *ev, fmt.Errorf("search for %q returned no candidates", cfg.Query)
	}
	// Selection is explicit: the walk holds the opaque identity the search
	// answered, never a name the daemon would have to resolve again.
	ev.SymbolID = search.Result.Candidates[0].Symbol.ID
	ev.SymbolName = search.Result.Candidates[0].Symbol.DisplayName
	ev.GraphVersion = search.Provenance.GraphVersion
	symbol := "/symbols/" + url.PathEscape(ev.SymbolID)

	detail, err := w.graph(ctx, "select", symbol, http.StatusOK)
	if err != nil {
		return *ev, err
	}
	if detail.Result.Symbol.ID != ev.SymbolID {
		return *ev, fmt.Errorf("detail answered symbol %q, selected %q", detail.Result.Symbol.ID, ev.SymbolID)
	}
	for _, step := range []struct{ name, path string }{
		{"calls outgoing", symbol + "/calls?direction=outgoing"},
		{"calls incoming", symbol + "/calls?direction=incoming"},
		{"dependencies", symbol + "/dependencies"},
		{"impact", symbol + "/impact"},
	} {
		if _, err := w.graph(ctx, step.name, step.path, http.StatusOK); err != nil {
			return *ev, err
		}
	}
	source, err := w.graph(ctx, "source", symbol+"/source", http.StatusOK)
	if err != nil {
		return *ev, err
	}
	if source.Result.Source.RangeHash == "" {
		return *ev, fmt.Errorf("source for %s carried no range hash", ev.SymbolID)
	}
	ev.SourceSHA256 = digest(source.Result.Source.Content)

	refusals := []struct {
		name, path, checkout, token string
		want                        int
	}{
		{"refuse missing bearer", symbol + "/source", ev.CheckoutID, "", http.StatusUnauthorized},
		{"refuse wrong bearer", symbol + "/source", ev.CheckoutID, "not-the-graph-bearer", http.StatusUnauthorized},
		{"refuse other checkout", symbol + "/source", "other-checkout-" + ev.CheckoutID, cfg.Token, http.StatusForbidden},
		{"refuse missing checkout", symbol + "/source", "", cfg.Token, http.StatusForbidden},
		{"refuse fabricated symbol", "/symbols/" + url.PathEscape("fabricated:../../etc/passwd") + "/source", ev.CheckoutID, cfg.Token, http.StatusNotFound},
		{"refuse invalid direction", symbol + "/calls?direction=sideways", ev.CheckoutID, cfg.Token, http.StatusBadRequest},
		{"refuse malformed depth", symbol + "/impact?depth=0", ev.CheckoutID, cfg.Token, http.StatusBadRequest},
	}
	for _, refusal := range refusals {
		if _, err := w.get(ctx, refusal.name, "/api/v1/repository-graph"+refusal.path, refusal.checkout, refusal.token, refusal.want, nil); err != nil {
			return *ev, err
		}
	}
	return *ev, nil
}

// Requery re-reads the identity a previous walk held, as a reloaded Console
// would. The same generation must return the same source byte-for-byte; a
// newer generation must answer current hash-validated source or refuse it
// explicitly as stale. Silently serving changed bytes under the held version
// is the failure this guards.
func Requery(ctx context.Context, cfg Config, prior Evidence) (Evidence, error) {
	w, err := newWalker(cfg)
	if err != nil {
		return Evidence{}, err
	}
	ev := w.evidence
	ev.CheckoutID, ev.SymbolID, ev.SymbolName = prior.CheckoutID, prior.SymbolID, prior.SymbolName
	if ev.CheckoutID == "" || ev.SymbolID == "" {
		return *ev, fmt.Errorf("prior evidence holds no checkout and symbol to re-query")
	}
	symbol := "/symbols/" + url.PathEscape(ev.SymbolID)
	var body envelope
	status, err := w.get(ctx, "requery source", "/api/v1/repository-graph"+symbol+"/source", ev.CheckoutID, cfg.Token, 0, &body)
	if err != nil {
		return *ev, err
	}
	switch {
	case status == http.StatusConflict && body.Reason == "graph_stale":
		return *ev, nil
	case status != http.StatusOK:
		return *ev, fmt.Errorf("requery source answered %d (%s), want 200 or an explicit graph_stale 409", status, body.Reason)
	}
	ev.GraphVersion = body.Provenance.GraphVersion
	ev.SourceSHA256 = digest(body.Result.Source.Content)
	if body.Result.Symbol.ID != ev.SymbolID {
		return *ev, fmt.Errorf("requery source answered symbol %q, held %q", body.Result.Symbol.ID, ev.SymbolID)
	}
	if body.Provenance.Freshness != "current" {
		return *ev, fmt.Errorf("requery source answered from %q generation, want current", body.Provenance.Freshness)
	}
	if body.Result.Source.RangeHash == "" {
		return *ev, fmt.Errorf("requery source carried no range hash")
	}
	if ev.GraphVersion == prior.GraphVersion && ev.SourceSHA256 != prior.SourceSHA256 {
		return *ev, fmt.Errorf("requery source changed under unchanged graph version %s", ev.GraphVersion)
	}
	return *ev, nil
}

func newWalker(cfg Config) (*walker, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.Token) == "" {
		return nil, fmt.Errorf("graph journey needs a base URL and the graph bearer")
	}
	if cfg.Query == "" {
		cfg.Query = "Execute"
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 30 * time.Second}
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &walker{cfg: cfg, evidence: &Evidence{BaseURL: cfg.BaseURL, At: time.Now().UTC()}}, nil
}

// graph performs an authorized, checkout-scoped read and requires the answer
// to name its generation consistently in header and body, and to be current.
func (w *walker) graph(ctx context.Context, name, path string, want int) (envelope, error) {
	var body envelope
	header, err := w.request(ctx, name, "/api/v1/repository-graph"+path, w.evidence.CheckoutID, w.cfg.Token, want, &body)
	if err != nil {
		return body, err
	}
	step := &w.evidence.Steps[len(w.evidence.Steps)-1]
	step.GraphVersion, step.Freshness = body.Provenance.GraphVersion, body.Provenance.Freshness
	step.Nodes, step.Truncated = len(body.Result.Nodes), body.Truncated
	if body.Provenance.GraphVersion == "" {
		return body, fmt.Errorf("%s: answer did not identify its graph generation", name)
	}
	if got := header.Get(graphVersionHeader); got != body.Provenance.GraphVersion {
		return body, fmt.Errorf("%s: %s header %q disagrees with provenance.graph_version %q", name, graphVersionHeader, got, body.Provenance.GraphVersion)
	}
	if body.Provenance.Freshness != "current" {
		return body, fmt.Errorf("%s: answered from %q generation, want current", name, body.Provenance.Freshness)
	}
	return body, nil
}

func (w *walker) get(ctx context.Context, name, path, checkout, token string, want int, into any) (int, error) {
	_, err := w.request(ctx, name, path, checkout, token, want, into)
	return w.evidence.Steps[len(w.evidence.Steps)-1].Status, err
}

// request records the step before judging it, so a failing walk still shows
// what the daemon answered. want 0 accepts any status for the caller to judge.
func (w *walker) request(ctx context.Context, name, path, checkout, token string, want int, into any) (http.Header, error) {
	step := Step{Name: name, Path: path, Want: want}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, w.cfg.BaseURL+path, nil)
	if err != nil {
		w.evidence.Steps = append(w.evidence.Steps, step)
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if checkout != "" {
		request.Header.Set("X-OpenExec-Checkout-ID", checkout)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := w.cfg.Client.Do(request)
	if err != nil {
		w.evidence.Steps = append(w.evidence.Steps, step)
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	step.Status = response.StatusCode
	var reason struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(raw, &reason)
	step.Reason = reason.Reason
	w.evidence.Steps = append(w.evidence.Steps, step)
	if err != nil {
		return nil, fmt.Errorf("%s: read answer: %w", name, err)
	}
	if want != 0 && response.StatusCode != want {
		return nil, fmt.Errorf("%s: %s answered %d, want %d", name, path, response.StatusCode, want)
	}
	if into != nil {
		if err := json.Unmarshal(raw, into); err != nil && (want == 0 || want == http.StatusOK) {
			return nil, fmt.Errorf("%s: decode answer: %w", name, err)
		}
	}
	return response.Header, nil
}

func digest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
