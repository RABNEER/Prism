package services

import (
	"context"
	"testing"

	"prism/internal/coverage"
	"prism/internal/domain"
	"prism/internal/storage"
)

func record(state, mode, published, event string) domain.Evidence {
	return domain.Evidence{State: state, District: "District", Locality: "Village", PropertyType: "plot", EntityLevel: "locality", Address: "Test fixture", ProviderRecordID: "test", Source: domain.Source{ID: "test", Name: "Test", URL: "https://example.org/test", AccessReference: "test fixture"}, Category: "geographic", DataMode: mode, PublishedDate: published, EventDate: event, RetrievedAt: "2026-11-07T00:00:00Z", Warnings: []string{}}
}

func TestNationalRouting(t *testing.T) {
	s := Service{Store: storage.Store{Root: t.TempDir()}}
	out, err := s.Coverage(context.Background(), CoverageInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Entries) != 36 {
		t.Fatalf("got %d jurisdictions", len(out.Entries))
	}
	states, uts := 0, 0
	seen := map[string]bool{}
	for _, j := range coverage.All() {
		if j.Kind == "state" {
			states++
		} else {
			uts++
		}
		for _, alias := range append([]string{j.Name}, j.Aliases...) {
			key := coverage.Normalize(alias)
			if seen[key] {
				t.Fatalf("duplicate alias %q", alias)
			}
			seen[key] = true
			r, err := coverage.Resolve(alias)
			if err != nil || r.Name != j.Name {
				t.Fatalf("failed alias %q", alias)
			}
		}
	}
	if states != 28 || uts != 8 {
		t.Fatalf("states=%d uts=%d", states, uts)
	}
	if _, err := coverage.Resolve("Atlantis"); err == nil {
		t.Fatal("accepted unknown jurisdiction")
	}
}

func TestSearchIsolationDatesAndFixtures(t *testing.T) {
	s := Service{Store: storage.Store{Root: t.TempDir()}}
	for _, e := range []domain.Evidence{
		record("Maharashtra", "imported", "2026-01-01", "2025-12-01"),
		record("Karnataka", "imported", "2026-01-01", "2025-12-01"),
		record("Maharashtra", "synthetic_fixture", "2026-01-01", "2025-12-01"),
		record("Maharashtra", "imported", "", ""),
		record("Maharashtra", "imported", "2026-01-01", "2026-12-01"),
		record("Maharashtra", "imported", "2026-12-01", "2025-12-01"),
	} {
		if _, err := s.Store.PutEvidence(e); err != nil {
			t.Fatal(err)
		}
	}
	q := domain.Query{State: "MH", PropertyType: "plot", Locality: " village ", AsOf: "2026-06-01"}
	out, err := s.Search(context.Background(), SearchInput{Query: q})
	if err != nil {
		t.Fatal(err)
	}
	if out.Total != 1 {
		t.Fatalf("expected 1, got %d", out.Total)
	}
	out, err = s.Search(context.Background(), SearchInput{Query: q, IncludeFixtures: true})
	if err != nil || out.Total != 2 {
		t.Fatalf("fixture inclusion: %d %v", out.Total, err)
	}
	q.AsOf = "2026-99-99"
	if _, err = s.Search(context.Background(), SearchInput{Query: q}); err == nil {
		t.Fatal("accepted invalid date")
	}
	if _, err = s.Store.Get("../../secret"); err == nil {
		t.Fatal("accepted unsafe ID")
	}
}

func TestPackageSnapshotAndHonestCoverage(t *testing.T) {
	s := Service{Store: storage.Store{Root: t.TempDir()}}
	e := record("Maharashtra", "synthetic_fixture", "2026-01-01", "")
	if _, err := s.Store.PutEvidence(e); err != nil {
		t.Fatal(err)
	}
	c, err := s.Coverage(context.Background(), CoverageInput{State: "MH"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Entries[0].Status != "not_yet_integrated" || c.Entries[0].ImportedRecords != 0 {
		t.Fatal("fixture inflated coverage")
	}
	in := BuildInput{Query: domain.Query{State: "MH", PropertyType: "plot"}}
	a, err := s.Build(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if a.Package.Status != "no_evidence" {
		t.Fatal("invented evidence")
	}
	b, err := s.Build(context.Background(), in)
	if err != nil || a.ID != b.ID {
		t.Fatal("snapshot is not reproducible")
	}
	if _, err = s.Store.Get(a.ID); err != nil {
		t.Fatal(err)
	}
	resolved, err := s.Resolve(ResolveInput{State: "RJ", PropertyType: "plot", Address: "  Example   Village  ", Locality: "Example Village"})
	if err != nil || resolved.EntityVerified || resolved.NormalizedAddress != "Example Village" {
		t.Fatal("bad rural resolution", err)
	}
}

func TestPriceSemantics(t *testing.T) {
	e := record("Maharashtra", "imported", "2026-01-01", "")
	e.Category = "listing"
	e.Price = &domain.Price{Amount: 100, Currency: "INR", Type: "registered_consideration"}
	if e.Validate() == nil {
		t.Fatal("asking source accepted as transaction label")
	}
	e.Price.Type = "reference_rate"
	if e.Validate() == nil {
		t.Fatal("accepted reference rate without denominator")
	}
	e.Price.Type = "asking_price"
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, category := range []string{"transaction", "geographic", "infrastructure", "reference_rate"} {
		e.Category = category
		if e.Validate() == nil {
			t.Fatalf("asking price accepted for category %q", category)
		}
	}
	e.Category = "transaction"
	e.Price.Type = "monthly_rent"
	if e.Validate() == nil {
		t.Fatal("rent accepted as sale transaction")
	}
	e.Price.Type = "registered_consideration"
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
}
