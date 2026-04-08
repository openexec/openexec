package contracts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerate_WritesExpectedFilesAndContent(t *testing.T) {
	cf := &ContractFile{
		Features: map[string][]Operation{
			"auction_bidding": {
				{
					ID:            "place_bid",
					Feature:       "auction_bidding",
					Method:        "POST",
					Path:          "/api/listings/:id/bids",
					Mutates:       []string{"listings.current_high_bid", "bids"},
					MustPersist:   true,
					Authorization: "authenticated",
				},
				{
					ID:      "get_listing",
					Feature: "auction_bidding",
					Method:  "GET",
					Path:    "/api/listings/:id",
				},
			},
		},
	}

	tmp := t.TempDir()
	files, err := Generate(cf, tmp)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}

	// Verify the place_bid file landed where expected.
	placeBid := filepath.Join(tmp, "tests", "contracts", "auction_bidding", "place_bid.test.ts")
	if _, err := os.Stat(placeBid); err != nil {
		t.Fatalf("expected %s to exist: %v", placeBid, err)
	}

	content, err := os.ReadFile(placeBid)
	if err != nil {
		t.Fatalf("read place_bid file: %v", err)
	}
	c := string(content)

	mustContain := []string{
		"AUTO-GENERATED",
		"contract: auction_bidding > place_bid",
		"persists to database (no stubs)",
		"rejects unauthenticated requests with 401",
		"rejects malformed payloads with 400",
		"handles N concurrent calls",
		"schema.listings",
		"full CRUD lifecycle",
	}
	for _, needle := range mustContain {
		if !strings.Contains(c, needle) {
			t.Errorf("place_bid.test.ts missing %q\n--- content ---\n%s", needle, c)
		}
	}

	// Read-only op should NOT have roundtrip/concurrency blocks.
	getListing := filepath.Join(tmp, "tests", "contracts", "auction_bidding", "get_listing.test.ts")
	content, err = os.ReadFile(getListing)
	if err != nil {
		t.Fatalf("read get_listing file: %v", err)
	}
	c = string(content)
	if strings.Contains(c, "persists to database (no stubs)") {
		t.Errorf("GET operation should not include roundtrip block:\n%s", c)
	}
	if strings.Contains(c, "handles N concurrent calls") {
		t.Errorf("GET operation should not include concurrency block:\n%s", c)
	}
	if !strings.Contains(c, "contract: auction_bidding > get_listing") {
		t.Errorf("get_listing.test.ts missing describe header:\n%s", c)
	}
}

func TestGenerate_EmptyContractFileIsNoop(t *testing.T) {
	tmp := t.TempDir()
	files, err := Generate(&ContractFile{}, tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if files != nil {
		t.Errorf("expected nil file list, got %v", files)
	}
}

func TestLoadFromBytes_ParsesContractSection(t *testing.T) {
	yaml := []byte(`
feature_completeness_contracts:
  auction_bidding:
    - id: place_bid
      method: POST
      path: /api/listings/:id/bids
      must_persist: true
      mutates:
        - listings.current_high_bid
`)
	cf, err := LoadFromBytes(yaml)
	if err != nil {
		t.Fatalf("LoadFromBytes failed: %v", err)
	}
	ops, ok := cf.Features["auction_bidding"]
	if !ok || len(ops) != 1 {
		t.Fatalf("expected one operation under auction_bidding, got: %+v", cf.Features)
	}
	op := ops[0]
	if op.ID != "place_bid" || op.Method != "POST" || !op.MustPersist {
		t.Errorf("parsed operation is wrong: %+v", op)
	}
	if op.Feature != "auction_bidding" {
		t.Errorf("expected Feature backfilled, got %q", op.Feature)
	}
}

func TestHelpers(t *testing.T) {
	if got := tableName("listings.current_high_bid"); got != "listings" {
		t.Errorf("tableName: got %q", got)
	}
	if got := titleCase("current_high_bid"); got != "CurrentHighBid" {
		t.Errorf("titleCase: got %q", got)
	}
	if got := camelCase("current_high_bid"); got != "currentHighBid" {
		t.Errorf("camelCase: got %q", got)
	}
	if got := collectionPath("/api/listings/:id"); got != "/api/listings" {
		t.Errorf("collectionPath: got %q", got)
	}
	if got := collectionPath("/api/listings/{id}/bids"); got != "/api/listings/{id}/bids" {
		t.Errorf("collectionPath trailing non-dynamic: got %q", got)
	}
}
