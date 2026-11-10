package services

import (
	"context"
	"os"
	"testing"

	"prism/internal/domain"
	"prism/internal/matching"
	"prism/internal/storage"
)

func TestComparablePoolsAndExclusions(t *testing.T) {
	s := Service{Store: storage.Store{Root: t.TempDir()}}
	makeRecord := func(id, unit, kind, basis, date string, amount float64) domain.Evidence {
		e := record("Maharashtra", "synthetic_fixture", "2026-01-01", date)
		e.ID = ""
		e.ProviderRecordID = id
		e.EntityLevel = "unit"
		e.UnitID = unit
		e.Address = "Fixture apartment " + unit
		e.Project = "Fixture Project"
		e.Area = &domain.Area{Value: 100, Unit: "sq_m", Basis: basis}
		e.Category = "listing"
		if kind == "registered_consideration" {
			e.Category = "transaction"
		}
		e.Price = &domain.Price{Amount: amount, Currency: "INR", Type: kind}
		return e
	}
	for _, e := range []domain.Evidence{
		makeRecord("sale-1", "101", "registered_consideration", "carpet", "2026-01-01", 10000000),
		makeRecord("ask-1", "102", "asking_price", "carpet", "2026-01-01", 12000000),
		makeRecord("basis-1", "103", "registered_consideration", "built_up", "2026-01-01", 10000000),
		makeRecord("old-1", "104", "registered_consideration", "carpet", "2020-01-01", 10000000),
		makeRecord("self", "999", "registered_consideration", "carpet", "2026-01-01", 10000000),
	} {
		if _, err := s.Store.PutEvidence(e); err != nil {
			t.Fatal(err)
		}
	}
	in := ComparablesInput{Subject: Subject{Query: domain.Query{State: "MH", PropertyType: "plot", District: "District", Locality: "Village"}, Project: "Fixture Project", UnitID: "999", Area: domain.Area{Value: 1076.391041671, Unit: "sq_ft", Basis: "carpet"}}, PriceType: "registered_consideration", AsOf: "2026-06-01", IncludeFixtures: true}
	out, err := s.Comparables(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Candidates) != 1 || out.Candidates[0].Evidence.ProviderRecordID != "sale-1" {
		t.Fatalf("wrong sale pool: %+v", out)
	}
	if out.Candidates[0].AreaDifferencePercent != 0 {
		t.Fatal("area conversion incorrect")
	}
	in.PriceType = "asking_price"
	out, err = s.Comparables(context.Background(), in)
	if err != nil || len(out.Candidates) != 1 || out.Candidates[0].Evidence.ProviderRecordID != "ask-1" {
		t.Fatal("asking pool mixed", out, err)
	}
	in.IncludeFixtures = false
	out, err = s.Comparables(context.Background(), in)
	if err != nil || len(out.Candidates) != 0 {
		t.Fatal("fixture exclusion failed")
	}
	in.Subject.Area.Basis = "unknown"
	if _, err = s.Comparables(context.Background(), in); err == nil {
		t.Fatal("unknown basis accepted")
	}
}

func TestRustReconciliationIntegration(t *testing.T) {
	path := os.Getenv("PRISM_MATCHING_WORKER")
	if path == "" {
		t.Skip("set PRISM_MATCHING_WORKER to built Rust worker")
	}
	s := Service{Store: storage.Store{Root: t.TempDir()}, Matcher: &matching.Client{Path: path}}
	e := record("Maharashtra", "synthetic_fixture", "2026-01-01", "2026-01-01")
	e.EntityLevel = "unit"
	e.UnitID = "101"
	e.Project = "Fixture"
	e.RetrievedAt = "2026-01-02T00:00:00Z"
	a, err := s.Store.PutEvidence(e)
	if err != nil {
		t.Fatal(err)
	}
	e.RetrievedAt = "2026-01-03T00:00:00Z"
	b, err := s.Store.PutEvidence(e)
	if err != nil {
		t.Fatal(err)
	}
	e.UnitID = "102"
	e.ProviderRecordID = "different-unit"
	_, err = s.Store.PutEvidence(e)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.Reconcile(context.Background(), ReconcileInput{Query: domain.Query{State: "MH", PropertyType: "plot"}, IncludeFixtures: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.DuplicateGroups) != 1 || len(out.DuplicateGroups[0].RecordIDs) != 2 || len(out.RetainedIDs) != 2 {
		t.Fatalf("unsafe deduplication: %+v", out)
	}
	found := false
	for _, d := range out.Decisions {
		if (d.LeftID == a.ID && d.RightID == b.ID) || (d.LeftID == b.ID && d.RightID == a.ID) {
			found = d.Kind == "same_source_observation"
		}
	}
	if !found {
		t.Fatal("repeat observation not recognized")
	}
}
