package matching

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"prism/internal/domain"
)

type Client struct{ Path string }
type Record struct {
	ID               string   `json:"id"`
	State            string   `json:"state"`
	District         string   `json:"district"`
	Locality         string   `json:"locality"`
	PropertyType     string   `json:"property_type"`
	Address          string   `json:"address"`
	Project          string   `json:"project"`
	Building         string   `json:"building"`
	UnitID           string   `json:"unit_id"`
	ProviderRecordID string   `json:"provider_record_id"`
	SourceID         string   `json:"source_id"`
	Category         string   `json:"category"`
	EventDate        string   `json:"event_date"`
	PriceType        string   `json:"price_type"`
	Price            *float64 `json:"price"`
}
type Pair struct {
	Left  Record `json:"left"`
	Right Record `json:"right"`
}
type Decision struct {
	LeftID     string   `json:"left_id"`
	RightID    string   `json:"right_id"`
	Kind       string   `json:"kind"`
	Similarity float64  `json:"similarity"`
	Reasons    []string `json:"reasons"`
	Conflicts  []string `json:"conflicts"`
}
type Output struct {
	ProtocolVersion string     `json:"protocol_version"`
	WorkerVersion   string     `json:"worker_version"`
	Decisions       []Decision `json:"decisions"`
	Error           string     `json:"error,omitempty"`
}

func FromEvidence(e domain.Evidence) Record {
	r := Record{ID: e.ID, State: e.State, District: e.District, Locality: e.Locality, PropertyType: e.PropertyType, Address: e.Address, Project: e.Project, Building: e.Building, UnitID: e.UnitID, ProviderRecordID: e.ProviderRecordID, SourceID: e.Source.ID, Category: e.Category, EventDate: e.EventDate}
	if e.Price != nil {
		r.Price = &e.Price.Amount
		r.PriceType = e.Price.Type
	}
	return r
}

func (c Client) Compare(ctx context.Context, pairs []Pair) (Output, error) {
	if len(pairs) > 20000 {
		return Output{}, fmt.Errorf("at most 20000 comparison pairs")
	}
	if c.Path == "" {
		return Output{}, fmt.Errorf("Rust matching worker is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	b, err := json.Marshal(struct {
		ProtocolVersion string `json:"protocol_version"`
		Pairs           []Pair `json:"pairs"`
	}{"1", pairs})
	if err != nil {
		return Output{}, err
	}
	cmd := exec.CommandContext(ctx, c.Path)
	cmd.Stdin = bytes.NewReader(append(b, '\n'))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		return Output{}, fmt.Errorf("matching worker failed: %w", err)
	}
	var out Output
	if err = json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return Output{}, fmt.Errorf("invalid matching response: %w", err)
	}
	if out.Error != "" || out.ProtocolVersion != "1" || len(out.Decisions) != len(pairs) {
		return Output{}, fmt.Errorf("matching protocol error: %s", out.Error)
	}
	for i, d := range out.Decisions {
		if d.LeftID != pairs[i].Left.ID || d.RightID != pairs[i].Right.ID {
			return Output{}, fmt.Errorf("matching response pair order mismatch")
		}
	}
	return out, nil
}
