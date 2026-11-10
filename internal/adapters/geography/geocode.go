package geography

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"prism/internal/coverage"
)

type GeocodeInput struct {
	Place string `json:"place" jsonschema:"Named city, town or village; not a private unit address"`
	State string `json:"state" jsonschema:"Indian state/UT; candidates from other states are excluded"`
}
type Candidate struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	State       string  `json:"state"`
	District    string  `json:"district"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	FeatureCode string  `json:"feature_code"`
	Precision   string  `json:"precision"`
}
type GeocodeResult struct {
	CacheHit   bool        `json:"cache_hit"`
	Status     string      `json:"status"`
	Candidates []Candidate `json:"candidates"`
	Source     Receipt     `json:"source"`
	Warnings   []string    `json:"warnings"`
}

func (c *Client) Geocode(ctx context.Context, in GeocodeInput) (GeocodeResult, error) {
	j, err := coverage.Resolve(in.State)
	if err != nil {
		return GeocodeResult{}, err
	}
	place := strings.Join(strings.Fields(in.Place), " ")
	if utf8.RuneCountInString(place) < 2 || len(place) > 150 || strings.ContainsAny(place, "\r\n,") {
		return GeocodeResult{}, fmt.Errorf("place must be a 2-150 character city/town/village name without commas")
	}
	params := url.Values{"name": {place + ", " + j.Name}, "countryCode": {"IN"}, "count": {"100"}, "language": {"en"}, "format": {"json"}}
	b, receipt, err := c.fetch(ctx, "open_meteo_geonames", c.config.GeocodingURL, http.MethodGet, params, "Location data: GeoNames via Open-Meteo, CC BY 4.0; https://open-meteo.com/en/docs/geocoding-api; https://www.geonames.org/", 7*24*time.Hour)
	if err != nil {
		return GeocodeResult{}, err
	}
	var response struct {
		Error   bool `json:"error"`
		Results []struct {
			ID          int64    `json:"id"`
			Name        string   `json:"name"`
			Latitude    *float64 `json:"latitude"`
			Longitude   *float64 `json:"longitude"`
			CountryCode string   `json:"country_code"`
			Admin1      string   `json:"admin1"`
			Admin2      string   `json:"admin2"`
			FeatureCode string   `json:"feature_code"`
		} `json:"results"`
	}
	if err = json.Unmarshal(b, &response); err != nil || response.Error {
		c.discard(receipt)
		return GeocodeResult{}, fmt.Errorf("invalid geocoding response")
	}
	out := GeocodeResult{Status: "no_candidates", Candidates: []Candidate{}, Source: receipt, Warnings: []string{"Named-place coordinates only: not a rooftop, apartment or cadastral location. Select a candidate explicitly; physical property identity remains unverified.", "Search results are bounded; an empty result does not prove the place does not exist."}}
	for _, r := range response.Results {
		state := providerState(r.Admin1)
		if r.CountryCode != "IN" || state != j.Name || r.ID <= 0 || r.Name == "" || r.Latitude == nil || r.Longitude == nil || !IndiaBounds(*r.Latitude, *r.Longitude) {
			continue
		}
		out.Candidates = append(out.Candidates, Candidate{ID: r.ID, Name: r.Name, State: state, District: r.Admin2, Latitude: *r.Latitude, Longitude: *r.Longitude, FeatureCode: r.FeatureCode, Precision: "named_place_centroid"})
		if len(out.Candidates) == 5 {
			break
		}
	}
	if len(out.Candidates) > 0 {
		out.Status = "candidates_found"
	}
	out.CacheHit = receipt.CacheHit
	return out, nil
}

func providerState(value string) string {
	aliases := map[string]string{"National Capital Territory of Delhi": "Delhi", "Union Territory of Puducherry": "Puducherry", "Union Territory of Chandigarh": "Chandigarh", "Union Territory of Andaman and Nicobar Islands": "Andaman and Nicobar Islands"}
	if alias, ok := aliases[value]; ok {
		value = alias
	}
	j, err := coverage.Resolve(value)
	if err != nil {
		return ""
	}
	return j.Name
}

// A coarse guard only, not a national/state boundary verification.
func IndiaBounds(lat, lon float64) bool {
	return !math.IsNaN(lat) && !math.IsNaN(lon) && !math.IsInf(lat, 0) && !math.IsInf(lon, 0) && lat >= 6 && lat <= 38 && lon >= 68 && lon <= 98
}

func coordinate(value float64) string { return strconv.FormatFloat(value, 'f', 6, 64) }
