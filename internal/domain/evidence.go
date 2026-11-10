package domain

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

const SchemaVersion = "0.3.0"

var PropertyTypes = []string{"apartment", "house", "plot", "office", "shop"}

func ValidPropertyType(value string) bool {
	for _, t := range PropertyTypes {
		if value == t {
			return true
		}
	}
	return false
}

type Query struct {
	State        string `json:"state" jsonschema:"Indian state or union territory name or supported alias"`
	PropertyType string `json:"property_type" jsonschema:"apartment, house, plot, office, or shop"`
	District     string `json:"district,omitempty"`
	Locality     string `json:"locality,omitempty" jsonschema:"Exact locality filter; omit for statewide search"`
	AsOf         string `json:"as_of,omitempty" jsonschema:"Optional YYYY-MM-DD cutoff; excludes evidence without a known publication date"`
}

func (q Query) Validate() error {
	if !ValidPropertyType(q.PropertyType) {
		return fmt.Errorf("unsupported property_type %q", q.PropertyType)
	}
	if q.AsOf != "" {
		if _, err := time.Parse(time.DateOnly, q.AsOf); err != nil {
			return fmt.Errorf("as_of must be YYYY-MM-DD")
		}
	}
	return nil
}

type Source struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	URL             string `json:"url"`
	AccessReference string `json:"access_reference" jsonschema:"Reference documenting permission or permitted reuse"`
}

type Area struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
	Basis string  `json:"basis"`
}

type Price struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
	Type     string  `json:"type"`
}

type Coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// A record represents one source observation, not a verified physical identity.
type Evidence struct {
	ID               string       `json:"id"`
	State            string       `json:"state"`
	District         string       `json:"district"`
	Locality         string       `json:"locality"`
	PropertyType     string       `json:"property_type"`
	EntityLevel      string       `json:"entity_level"`
	Address          string       `json:"address"`
	Project          string       `json:"project,omitempty"`
	ProviderRecordID string       `json:"provider_record_id"`
	Source           Source       `json:"source"`
	Category         string       `json:"category"`
	DataMode         string       `json:"data_mode"`
	EventDate        string       `json:"event_date,omitempty"`
	PublishedDate    string       `json:"published_date,omitempty"`
	RetrievedAt      string       `json:"retrieved_at"`
	Area             *Area        `json:"area,omitempty"`
	Price            *Price       `json:"price,omitempty"`
	Warnings         []string     `json:"warnings"`
	UnitID           string       `json:"unit_id,omitempty"`
	Building         string       `json:"building,omitempty"`
	Location         *Coordinates `json:"location,omitempty"`
	Bedrooms         *int         `json:"bedrooms,omitempty"`
	Floor            *int         `json:"floor,omitempty"`
	SourceSnapshotID string       `json:"source_snapshot_id,omitempty"`
}

func (e Evidence) Validate() error {
	for name, value := range map[string]string{"state": e.State, "district": e.District, "locality": e.Locality, "address": e.Address, "provider_record_id": e.ProviderRecordID, "source.id": e.Source.ID, "source.name": e.Source.Name, "source.access_reference": e.Source.AccessReference} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if !ValidPropertyType(e.PropertyType) {
		return fmt.Errorf("unsupported property_type")
	}
	if !oneOf(e.EntityLevel, "unit", "building", "project", "locality") {
		return fmt.Errorf("invalid entity_level")
	}
	if !oneOf(e.Category, "transaction", "project", "listing", "reference_rate", "geographic", "infrastructure") {
		return fmt.Errorf("invalid category")
	}
	if !oneOf(e.DataMode, "imported", "synthetic_fixture") {
		return fmt.Errorf("data_mode must be imported or synthetic_fixture")
	}
	u, err := url.Parse(e.Source.URL)
	if err != nil || u.Host == "" || !oneOf(u.Scheme, "https", "http") {
		return fmt.Errorf("source.url must be an HTTP(S) URL")
	}
	if _, err := time.Parse(time.RFC3339, e.RetrievedAt); err != nil {
		return fmt.Errorf("retrieved_at must be RFC3339")
	}
	for _, d := range []string{e.EventDate, e.PublishedDate} {
		if d != "" {
			if _, err := time.Parse(time.DateOnly, d); err != nil {
				return fmt.Errorf("dates must be YYYY-MM-DD")
			}
		}
	}
	if e.Area != nil && (e.Area.Value <= 0 || !oneOf(e.Area.Unit, "sq_ft", "sq_m", "acre", "hectare") || !oneOf(e.Area.Basis, "carpet", "built_up", "super_built_up", "land", "unknown")) {
		return fmt.Errorf("invalid area value, unit or basis")
	}
	if e.Price != nil {
		if e.Price.Amount <= 0 || e.Price.Currency != "INR" || !oneOf(e.Price.Type, "registered_consideration", "asking_price", "reference_rate", "monthly_rent") {
			return fmt.Errorf("invalid price amount, currency or type")
		}
		if e.Price.Type == "reference_rate" {
			return fmt.Errorf("reference rates require a rate denominator; rate ingestion is not implemented yet")
		}
		if e.Category == "listing" && !oneOf(e.Price.Type, "asking_price", "monthly_rent") {
			return fmt.Errorf("listing cannot be a registered-sale label")
		}
		if e.Price.Type == "registered_consideration" && e.Category != "transaction" {
			return fmt.Errorf("registered consideration requires transaction category")
		}
		if e.Category == "transaction" && e.Price.Type != "registered_consideration" {
			return fmt.Errorf("sale transaction evidence requires registered consideration, not an asking price or rent")
		}
		if !oneOf(e.Category, "transaction", "listing", "project") {
			return fmt.Errorf("price is not supported for category %q", e.Category)
		}
	}
	if e.Location != nil && (e.Location.Latitude < -90 || e.Location.Latitude > 90 || e.Location.Longitude < -180 || e.Location.Longitude > 180) {
		return fmt.Errorf("invalid coordinates")
	}
	if e.Bedrooms != nil && (*e.Bedrooms < 0 || *e.Bedrooms > 100) {
		return fmt.Errorf("invalid bedrooms")
	}
	return nil
}

func oneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
