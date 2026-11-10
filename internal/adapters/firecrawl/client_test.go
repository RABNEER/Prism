package firecrawl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"prism/internal/storage"
)

func TestPuneListingAndPriceSemantics(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Error("missing API authentication")
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["url"] != "https://listing.example/apartment" {
			t.Error("wrong URL")
		}
		formats := request["formats"].([]any)
		if len(formats) != 2 || formats[0] != "markdown" || formats[1] != "rawHtml" {
			t.Error("model-based formats requested")
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"markdown": "# Test apartment\nProperty type: apartment\nTransaction type: sale\nAddress: Test apartment, Baner, Pune\nLocality: Baner\nDistrict: Pune\nState: Maharashtra\nPrice: ₹80 L\nCarpet area: 1,000 sqft\nPublished date: 2026-01-01", "metadata": map[string]any{"sourceURL": "https://listing.example/apartment", "statusCode": 200}}})
	}))
	defer server.Close()
	store := storage.Store{Root: t.TempDir()}
	c, err := New(Config{Endpoint: server.URL, APIKey: "test-secret", Sources: map[string]string{"listing.example": "Fixture test source"}}, store)
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Scrape(context.Background(), Input{URL: "https://listing.example/apartment", Locality: "Baner"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Record.Category != "listing" || out.Record.Price.Type != "asking_price" || out.Record.Price.Amount != 8000000 || out.Record.Area.Value != 1000 || out.Record.SourceSnapshotID == "" {
		t.Fatalf("incorrect extraction: %+v", out)
	}
	if _, err = store.Get(out.Record.SourceSnapshotID); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Scrape(context.Background(), Input{URL: "https://unlisted.example/apartment", Locality: "Baner"}); err == nil {
		t.Fatal("unpermitted source accepted")
	}
	if _, err = c.Scrape(context.Background(), Input{URL: "https://listing.example/apartment", Locality: "Mumbai"}); err == nil {
		t.Fatal("outside-pilot location accepted")
	}
	if calls != 1 {
		t.Fatal("invalid request reached upstream")
	}
}

func TestStrictPriceParsingAndDisabledAccess(t *testing.T) {
	for text, expected := range map[string]float64{"₹1.2 Cr": 12000000, "INR 80,00,000": 8000000, "₹50,000/month": 50000} {
		n, ok := parsePrice(text)
		if !ok || n != expected {
			t.Fatalf("parse %q => %v %v", text, n, ok)
		}
	}
	for _, text := range []string{"Call for price", "80L onwards", "₹50,000/year", "-100"} {
		if _, ok := parsePrice(text); ok {
			t.Fatalf("ambiguous price accepted %q", text)
		}
	}
	c, _ := New(Config{Endpoint: "https://api.firecrawl.dev/v2/scrape"}, storage.Store{Root: t.TempDir()})
	if c.Status().Configured {
		t.Fatal("missing configuration treated as ready")
	}
	if _, err := c.Scrape(context.Background(), Input{URL: "https://listing.example/apartment", Locality: "Baner"}); err == nil {
		t.Fatal("unconfigured network request accepted")
	}
}

func TestSelfHostedRequiresLoopback(t *testing.T) {
	for _, endpoint := range []string{"https://api.firecrawl.dev/v2/scrape", "https://remote.example/v2/scrape"} {
		if _, err := New(Config{Endpoint: endpoint, SelfHosted: true}, storage.Store{}); err == nil {
			t.Fatal("remote endpoint accepted as self-hosted")
		}
	}
	c, err := New(Config{Endpoint: "http://127.0.0.1:3002/v2/scrape", SelfHosted: true, Sources: map[string]string{"listing.example": "mock only"}}, storage.Store{})
	if err != nil || !c.Status().Configured || c.Status().Authentication != "self_hosted" {
		t.Fatal("explicit loopback server not configured", err)
	}
}

func TestRentalPeriodAndAreaBasisMustBeSupported(t *testing.T) {
	for _, item := range []struct{ text, purpose string }{{"₹50,000", "rent"}, {"₹50,000/month", "sale"}, {"₹50,000/year", "rent"}} {
		if _, ok := parseListingPrice(item.text, item.purpose); ok {
			t.Fatalf("unsupported price period accepted: %+v", item)
		}
	}
	if amount, ok := parseListingPrice("₹50,000/month", "rent"); !ok || amount != 50000 {
		t.Fatal("explicit monthly rent rejected")
	}
	for _, item := range []struct{ markdown, proposed, want string }{
		{"Carpet area 100 sqm", "carpet", "carpet"},
		{"Area 100 sqm", "carpet", "unknown"},
		{"Super built-up area 100 sqm", "built_up", "unknown"},
		{"Carpet 100 sqm; built-up 120 sqm", "carpet", "unknown"},
	} {
		if got := sourceAreaBasis(item.markdown, "100 sqm", item.proposed); got != item.want {
			t.Fatalf("area basis %q => %q, want %q", item.markdown, got, item.want)
		}
	}
}
