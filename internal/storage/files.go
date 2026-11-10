package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"prism/internal/domain"
)

// Store is a development file store, to be replaced by PostgreSQL/PostGIS.
type Store struct{ Root string }

func Digest(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

func (s Store) PutEvidence(e domain.Evidence) (domain.Evidence, error) {
	e.ID = ""
	if e.Warnings == nil {
		e.Warnings = []string{}
	}
	b, err := json.Marshal(e)
	if err != nil {
		return e, err
	}
	e.ID = "ev_" + Digest(b)
	b, err = json.Marshal(e)
	if err != nil {
		return e, err
	}
	err = s.write("evidence", e.ID, b)
	return e, err
}

func (s Store) write(folder, id string, b []byte) error {
	dir := filepath.Join(s.Root, folder)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".pending-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	path := filepath.Join(dir, id+".json")
	if old, err := os.ReadFile(path); err == nil {
		if bytes.Equal(old, b) {
			return nil
		}
		return fmt.Errorf("immutable record conflict")
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func validID(id string) bool {
	parts := strings.SplitN(id, "_", 2)
	if len(parts) != 2 || (parts[0] != "ev" && parts[0] != "pkg") || len(parts[1]) != 64 {
		return false
	}
	_, err := hex.DecodeString(parts[1])
	return err == nil
}

func (s Store) Get(id string) (json.RawMessage, error) {
	if !validID(id) {
		return nil, fmt.Errorf("invalid evidence/package ID")
	}
	folder := "evidence"
	if strings.HasPrefix(id, "pkg_") {
		folder = "packages"
	}
	b, err := os.ReadFile(filepath.Join(s.Root, folder, id+".json"))
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("record not found")
	}
	if err != nil {
		return nil, err
	}
	if folder == "packages" {
		if "pkg_"+Digest(b) != id {
			return nil, fmt.Errorf("package integrity check failed for %s", id)
		}
	} else {
		var e domain.Evidence
		if err := json.Unmarshal(b, &e); err != nil {
			return nil, fmt.Errorf("invalid evidence record: %w", err)
		}
		if e.ID != id {
			return nil, fmt.Errorf("evidence ID mismatch for %s", id)
		}
		stored, err := json.Marshal(e)
		if err != nil || !bytes.Equal(stored, b) {
			return nil, fmt.Errorf("noncanonical or altered evidence record for %s", id)
		}
		if err := e.Validate(); err != nil {
			return nil, fmt.Errorf("invalid stored evidence: %w", err)
		}
		e.ID = ""
		canonical, err := json.Marshal(e)
		if err != nil {
			return nil, err
		}
		if "ev_"+Digest(canonical) != id {
			return nil, fmt.Errorf("evidence integrity check failed for %s", id)
		}
	}
	return b, nil
}

func (s Store) List() ([]domain.Evidence, error) {
	entries, err := os.ReadDir(filepath.Join(s.Root, "evidence"))
	if os.IsNotExist(err) {
		return []domain.Evidence{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(entries) > 10000 {
		return nil, fmt.Errorf("development store limit exceeded; migrate to indexed storage")
	}
	out := []domain.Evidence{}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "ev_") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		b, err := s.Get(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		var e domain.Evidence
		if err = json.Unmarshal(b, &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s Store) PutPackage(value any) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	id := "pkg_" + Digest(b)
	return id, s.write("packages", id, b)
}
