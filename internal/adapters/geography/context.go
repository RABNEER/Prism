package geography

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

type ContextInput struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	RadiusM   int     `json:"radius_m,omitempty" jsonschema:"100-2000 metres; defaults to 500"`
	AsOf      string  `json:"as_of,omitempty" jsonschema:"Historical dates unsupported; omit for current OSM data"`
	Provider  string  `json:"provider,omitempty" jsonschema:"auto (default), overture, or overpass"`
}
type POI struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Category  string  `json:"category"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	DistanceM float64 `json:"distance_m"`
	URL       string  `json:"url"`
}
type ContextResult struct {
	ContextEvidence
	CacheHit bool `json:"cache_hit"`
}

type ContextEvidence struct {
	Status           string             `json:"status"`
	Latitude         float64            `json:"latitude"`
	Longitude        float64            `json:"longitude"`
	RadiusM          int                `json:"radius_m"`
	POIs             []POI              `json:"pois"`
	Counts           map[string]int     `json:"counts"`
	NearestDistanceM map[string]float64 `json:"nearest_distance_m"`
	Source           Receipt            `json:"source"`
	DatasetTimestamp string             `json:"dataset_timestamp"`
	DatasetRelease   string             `json:"dataset_release,omitempty"`
	Method           string             `json:"method"`
	Warnings         []string           `json:"warnings"`
}

func (c *Client) Context(ctx context.Context, in ContextInput) (ContextResult, error) {
	if in.Provider != "" && in.Provider != "auto" && in.Provider != "overture" && in.Provider != "overpass" {
		return ContextResult{}, fmt.Errorf("provider must be auto, overture or overpass")
	}
	if in.Provider == "overture" || ((in.Provider == "" || in.Provider == "auto") && c.config.OvertureScript != "") {
		return c.overture(ctx, in)
	}
	return c.overpass(ctx, in)
}

func (c *Client) overpass(ctx context.Context, in ContextInput) (ContextResult, error) {
	if !IndiaBounds(in.Latitude, in.Longitude) {
		return ContextResult{}, fmt.Errorf("coordinates outside coarse India bounds")
	}
	if in.AsOf != "" {
		return ContextResult{}, fmt.Errorf("historical location context is unsupported; current OSM data cannot be used as historical evidence")
	}
	if in.RadiusM == 0 {
		in.RadiusM = 500
	}
	if in.RadiusM < 100 || in.RadiusM > 2000 {
		return ContextResult{}, fmt.Errorf("radius_m must be 100-2000")
	}
	// Query only supported amenities; country area restricts returned elements.
	near := fmt.Sprintf("(around:%d,%s,%s)(area.india)", in.RadiusM, coordinate(in.Latitude), coordinate(in.Longitude))
	query := "[out:json][timeout:20][maxsize:16777216];area[\"ISO3166-1\"=\"IN\"][admin_level=2]->.india;(" +
		"nwr[amenity~\"^(school|college|university|hospital|clinic|bus_station)$\"]" + near + ";" +
		"nwr[railway~\"^(station|halt|subway_entrance)$\"]" + near + ";" +
		"nwr[highway=bus_stop]" + near + ";);out center qt 501;"
	b, receipt, err := c.fetch(ctx, "osm_overpass", c.config.OverpassURL, http.MethodPost, url.Values{"data": {query}}, "© OpenStreetMap contributors, ODbL; https://www.openstreetmap.org/copyright", 24*time.Hour)
	if err != nil {
		return ContextResult{}, err
	}
	var response struct {
		Version float64 `json:"version"`
		Remark  string  `json:"remark"`
		OSM     struct {
			Timestamp string `json:"timestamp_osm_base"`
		} `json:"osm3s"`
		Elements []struct {
			Type   string   `json:"type"`
			ID     int64    `json:"id"`
			Lat    *float64 `json:"lat"`
			Lon    *float64 `json:"lon"`
			Center *struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
			} `json:"center"`
			Tags map[string]string `json:"tags"`
		} `json:"elements"`
	}
	err = json.Unmarshal(b, &response)
	_, timestampErr := time.Parse(time.RFC3339, response.OSM.Timestamp)
	if err != nil || response.Version != 0.6 || response.Remark != "" || response.Elements == nil || timestampErr != nil {
		c.discard(receipt)
		return ContextResult{}, fmt.Errorf("Overpass returned invalid or incomplete results; refusing to present them as complete")
	}
	out := ContextResult{CacheHit: receipt.CacheHit, ContextEvidence: ContextEvidence{Status: "available", Latitude: in.Latitude, Longitude: in.Longitude, RadiusM: in.RadiusM, POIs: []POI{}, Counts: map[string]int{}, NearestDistanceM: map[string]float64{}, Source: receipt, DatasetTimestamp: response.OSM.Timestamp, Method: "haversine_point_or_bbox_center_v1", Warnings: []string{"OSM coverage is incomplete. Counts describe mapped elements, not unique verified facilities; no cross-element facility deduplication.", "Distances are straight-line to mapped points or bounding-box centers, not travel distances or nearest boundaries.", "Input coordinates use a coarse India bounds check; their country/state/property identity has not been independently verified. Returned elements are country-area filtered."}}}
	if len(response.Elements) > 500 {
		out.Status = "truncated"
		out.Warnings = append(out.Warnings, "Source query reached its 500-element cap; counts and nearest distances are partial.")
	}
	for i, r := range response.Elements {
		if i >= 500 {
			break
		}
		var lat, lon float64
		if r.Lat != nil && r.Lon != nil {
			lat, lon = *r.Lat, *r.Lon
		} else if r.Center != nil {
			lat, lon = r.Center.Lat, r.Center.Lon
		} else {
			continue
		}
		if !IndiaBounds(lat, lon) {
			continue
		}
		category := poiCategory(r.Tags)
		if category == "" {
			continue
		}
		distance := haversine(in.Latitude, in.Longitude, lat, lon)
		// Around filters may include a large polygon whose center lies outside.
		if distance > float64(in.RadiusM) {
			continue
		}
		id := r.Type + "/" + strconv.FormatInt(r.ID, 10)
		out.POIs = append(out.POIs, POI{ID: id, Name: r.Tags["name"], Category: category, Latitude: lat, Longitude: lon, DistanceM: math.Round(distance*10) / 10, URL: "https://www.openstreetmap.org/" + id})
	}
	sort.Slice(out.POIs, func(i, j int) bool {
		if out.POIs[i].DistanceM == out.POIs[j].DistanceM {
			return out.POIs[i].ID < out.POIs[j].ID
		}
		return out.POIs[i].DistanceM < out.POIs[j].DistanceM
	})
	for _, p := range out.POIs {
		out.Counts[p.Category]++
		if _, exists := out.NearestDistanceM[p.Category]; !exists {
			out.NearestDistanceM[p.Category] = p.DistanceM
		}
	}
	out.Warnings = append(out.Warnings, "Large facilities with centers outside the radius are excluded even if their geometry intersects the query area.")
	return out, nil
}

func poiCategory(tags map[string]string) string {
	switch tags["amenity"] {
	case "school", "college", "university":
		return "education"
	case "hospital", "clinic":
		return "healthcare"
	case "bus_station":
		return "bus"
	}
	switch tags["railway"] {
	case "station", "halt", "subway_entrance":
		return "rail"
	}
	if tags["highway"] == "bus_stop" {
		return "bus"
	}
	return ""
}

func haversine(a, b, c, d float64) float64 {
	rad := math.Pi / 180
	dy := (c - a) * rad
	dx := (d - b) * rad
	v := math.Sin(dy/2)*math.Sin(dy/2) + math.Cos(a*rad)*math.Cos(c*rad)*math.Sin(dx/2)*math.Sin(dx/2)
	if v > 1 {
		v = 1
	}
	return 6371008.8 * 2 * math.Atan2(math.Sqrt(v), math.Sqrt(1-v))
}
