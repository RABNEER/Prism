package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"prism/internal/domain"
)

func TestRejectChangedEvidenceAndPackages(t *testing.T) {
	s := Store{Root: t.TempDir()}
	e := domain.Evidence{State: "Maharashtra", District: "Test", Locality: "Test", PropertyType: "apartment", EntityLevel: "project", Address: "Original fixture", ProviderRecordID: "fixture-1", Source: domain.Source{ID: "fixture", Name: "Fixture", URL: "https://example.org/test", AccessReference: "Synthetic test"}, Category: "project", DataMode: "synthetic_fixture", RetrievedAt: "2026-11-07T00:00:00Z"}
	e, err := s.PutEvidence(e)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(e.ID); err != nil {
		t.Fatal(err)
	}
	e.Address = "Changed fixture"
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Root, "evidence", e.ID+".json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(e.ID); err == nil {
		t.Fatal("Get accepted altered evidence under original ID")
	}
	if _, err := s.List(); err == nil {
		t.Fatal("List accepted altered evidence")
	}
	id, err := s.PutPackage(map[string]string{"status": "partial"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(id); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Root, "packages", id+".json"), []byte(`{"status":"complete"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(id); err == nil {
		t.Fatal("Get accepted altered package under original ID")
	}
}
