package firecrawl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"prism/internal/storage"
	"testing"
)

func TestDiscoveryAndIngestionWithoutManualDomains(t *testing.T) {
	searches, scrapes := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if r.URL.Path == "/v2/search" {
			searches++
			if body["scrapeOptions"] != nil {
				t.Error("discovery should not scrape pages")
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"web": []map[string]string{
				{"url": "https://listing.example/apartment#top", "title": "One apartment", "description": "Unverified search text"},
				{"url": "https://listing.example/apartment", "title": "Duplicate"},
				{"url": "https://listing.example/search", "title": "Collection"},
				{"url": "http://127.0.0.1/private"}, {"url": "https://127.0.0.1/private"}, {"url": "https://localhost.local/private"},
			}}})
		} else {
			scrapes++
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"markdown": "# Apartment\nProperty type: apartment\nTransaction type: sale\nAddress: Building, Baner, Pune\nLocality: Baner\nDistrict: Pune\nState: Maharashtra\nPrice: INR 8000000\nCarpet area: 100 sqm", "metadata": map[string]any{"sourceURL": body["url"], "statusCode": 200}}})
		}
	}))
	defer server.Close()
	store := storage.Store{Root: t.TempDir()}
	c, err := New(Config{Endpoint: server.URL + "/v2/scrape", SelfHosted: true}, store)
	if err != nil {
		t.Fatal(err)
	}
	if c.Status().Configured {
		t.Fatal("manual-source ingestion should still report no configured domains")
	}
	found, err := c.Discover(context.Background(), DiscoveryInput{Locality: "Baner"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Discovery.Candidates) != 2 || found.Status != "candidates_found" {
		t.Fatal("filtering/dedup failed", found)
	}
	if _, err = store.Get(found.Discovery.SearchSnapshotID); err != nil {
		t.Fatal("search archive absent", err)
	}
	result, err := c.IngestDiscovered(context.Background(), DiscoveredInput{DiscoveryID: found.DiscoveryID, CandidateID: found.Discovery.Candidates[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Record.Price == nil || result.Record.Price.Amount != 8000000 || result.Record.Source.AccessReference == "" {
		t.Fatal("candidate ingestion failed", result)
	}
	if len(c.config.Sources) != 0 {
		t.Fatal("candidate unexpectedly changed global source configuration")
	}
	for _, in := range []DiscoveredInput{{found.DiscoveryID, "forged-id"}, {found.DiscoveryID, found.Discovery.Candidates[1].ID}, {found.Discovery.SearchSnapshotID, found.Discovery.Candidates[0].ID}} {
		if _, err := c.IngestDiscovered(context.Background(), in); err == nil {
			t.Fatal("invalid or collection candidate accepted")
		}
	}
	if searches != 1 || scrapes != 1 {
		t.Fatal("unexpected upstream requests", searches, scrapes)
	}
	wrongPurpose, err := c.Discover(context.Background(), DiscoveryInput{Locality: "Baner", Purpose: "rent"})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := store.List()
	if _, err := c.IngestDiscovered(context.Background(), DiscoveredInput{wrongPurpose.DiscoveryID, wrongPurpose.Discovery.Candidates[0].ID}); err == nil {
		t.Fatal("sale listing accepted for rental discovery")
	}
	after, _ := store.List()
	if len(before) != len(after) {
		t.Fatal("mismatched purpose imported before validation")
	}
}

func TestDiscoveryGuardsAndEmptyProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"success":true,"data":{}}`)) }))
	defer server.Close()
	c, _ := New(Config{Endpoint: server.URL, SelfHosted: true}, storage.Store{Root: t.TempDir()})
	for _, in := range []DiscoveryInput{{Locality: "Mumbai"}, {Locality: "Baner", Purpose: "any"}, {Locality: "Baner", Limit: 11}} {
		if _, err := c.Discover(context.Background(), in); err == nil {
			t.Fatal("invalid pilot/search accepted")
		}
	}
	out, err := c.Discover(context.Background(), DiscoveryInput{Locality: "Kharadi"})
	if err != nil || out.Status != "no_results_or_provider_unavailable" {
		t.Fatal("empty response overclaimed", out, err)
	}
}

func TestDiscoveryExpandsOneIndexAndFiltersLinks(t *testing.T) {
	indexRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/search" {
			w.Write([]byte(`{"success":true,"data":{"web":[{"url":"https://listing.example/search"},{"url":"https://other.example/search"}]}}`))
			return
		}
		indexRequests++
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["formats"].([]any)[0] != "links" {
			t.Error("non-link source discovery requested")
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"links": []string{"/propertyDetails/unit#top", "/propertyDetails/unit", "https://other.example/propertyDetails/unit", "/contact", "https://127.0.0.1/propertyDetails/private"}, "metadata": map[string]any{"sourceURL": "https://listing.example/search", "statusCode": 200}}})
	}))
	defer server.Close()
	c, _ := New(Config{Endpoint: server.URL + "/v2/scrape", SelfHosted: true}, storage.Store{Root: t.TempDir()})
	out, err := c.Discover(context.Background(), DiscoveryInput{Locality: "Baner", IndividualPages: true, Limit: 1})
	if err != nil || len(out.Discovery.Candidates) != 1 || out.Discovery.Candidates[0].URL != "https://listing.example/propertyDetails/unit" || out.Discovery.Candidates[0].FoundOn == "" || len(out.Discovery.ExpansionSnapshotIDs) != 1 || indexRequests != 1 {
		t.Fatal("bounded index expansion failed", out, err, indexRequests)
	}
}
