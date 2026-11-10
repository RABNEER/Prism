package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"prism/internal/coverage"
	"prism/internal/domain"
	"prism/internal/storage"
)

func run() error {
	file := flag.String("import", "", "JSON array of source evidence observations to import")
	data := flag.String("data-dir", "data", "Local development evidence directory")
	flag.Parse()
	if *file == "" {
		return fmt.Errorf("usage: prism-admin -import file.json [-data-dir data]")
	}
	f, err := os.Open(*file)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 4*1024*1024+1))
	if err != nil {
		return err
	}
	if len(b) > 4*1024*1024 {
		return fmt.Errorf("import exceeds 4 MiB limit")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var records []domain.Evidence
	if err = dec.Decode(&records); err != nil {
		return err
	}
	if err = dec.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected one JSON array")
	}
	if len(records) == 0 || len(records) > 1000 {
		return fmt.Errorf("import must contain 1-1000 records")
	}
	// Validate the entire batch before performing any writes.
	for i := range records {
		e := &records[i]
		if e.ID != "" {
			return fmt.Errorf("record %d: IDs are server-generated", i)
		}
		j, err := coverage.Resolve(e.State)
		if err != nil {
			return fmt.Errorf("record %d: %w", i, err)
		}
		e.State = j.Name
		e.District = strings.TrimSpace(e.District)
		e.Locality = strings.TrimSpace(e.Locality)
		if err = e.Validate(); err != nil {
			return fmt.Errorf("record %d: %w", i, err)
		}
	}
	store := storage.Store{Root: *data}
	for _, e := range records {
		saved, err := store.PutEvidence(e)
		if err != nil {
			return err
		}
		fmt.Println(saved.ID)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
