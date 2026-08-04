package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
)

func TestGraphSearchTreatsWildcardsAsLiterals(t *testing.T) {
	fixture := newGraphQueryFixture(t)
	// No fixture symbol contains "_", so a search for it must find nothing.
	// LIKE '%_%' matches any non-empty string, so an unescaped pattern returns
	// the whole repository — the opposite of what the owner asked for, and
	// exactly what happens on the first snake_case codebase.
	response := graphRequest(t, fixture, "/api/v1/repository-graph/symbols?q=_", fixture.identity.CheckoutID)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	var result knowledge.QueryEnvelope[knowledge.SymbolSearchResult]
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Result.Candidates) != 0 {
		names := []string{}
		for _, c := range result.Result.Candidates {
			names = append(names, c.Symbol.DisplayName)
		}
		t.Fatalf("underscore matched %d symbols that do not contain one: %v", len(names), names)
	}
}
