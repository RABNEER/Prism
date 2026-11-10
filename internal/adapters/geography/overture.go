package geography

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"prism/internal/storage"
)

type overturePlace struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Category       string           `json:"category"`
	Latitude       float64          `json:"latitude"`
	Longitude      float64          `json:"longitude"`
	SourceCategory string           `json:"source_category"`
	Sources        []map[string]any `json:"sources"`
}
type overtureResponse struct {
	Provider    string          `json:"provider"`
	Release     string          `json:"release"`
	RetrievedAt string          `json:"retrieved_at"`
	BBox        []float64       `json:"bbox"`
	Places      []overturePlace `json:"places"`
	Attribution string          `json:"attribution"`
	Error       string          `json:"error,omitempty"`
}

func (c *Client) overture(ctx context.Context, in ContextInput) (ContextResult, error) {
	if !IndiaBounds(in.Latitude, in.Longitude) {
		return ContextResult{}, fmt.Errorf("coordinates outside coarse India bounds")
	}
	if in.AsOf != "" {
		return ContextResult{}, fmt.Errorf("historical Overture feature selection is not implemented")
	}
	if in.RadiusM == 0 {
		in.RadiusM = 500
	}
	if in.RadiusM < 100 || in.RadiusM > 2000 {
		return ContextResult{}, fmt.Errorf("radius_m must be 100-2000")
	}
	if c.config.OvertureScript == "" {
		return ContextResult{}, fmt.Errorf("Overture worker not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	select {
	case c.gate <- struct{}{}:
		defer func() { <-c.gate }()
	case <-ctx.Done():
		return ContextResult{}, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return ContextResult{}, err
	}
	key := storage.Digest([]byte("overture_v1|" + coordinate(in.Latitude) + "|" + coordinate(in.Longitude) + "|" + strconv.Itoa(in.RadiusM)))
	path := filepath.Join(c.store.Root, "source-cache", key+".json")
	var response overtureResponse
	var cached cacheEntry
	id := ""
	hit := false
	if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, &cached) == nil && time.Now().Before(cached.ExpiresAt) {
		b, err = c.store.Get(cached.SnapshotID)
		if err != nil {
			return ContextResult{}, err
		}
		if err = json.Unmarshal(b, &response); err != nil {
			return ContextResult{}, err
		}
		id = cached.SnapshotID
		hit = true
	} else {
		cmd := exec.CommandContext(ctx, c.config.OverturePython, "-S", c.config.OvertureScript, "--latitude", coordinate(in.Latitude), "--longitude", coordinate(in.Longitude), "--radius-m", strconv.Itoa(in.RadiusM), "--modules", c.config.PythonModules)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			return ContextResult{}, fmt.Errorf("Overture retrieval failed: %w; %s", err, bounded(stdout.String(), 1000))
		}
		if stdout.Len() > 8*1024*1024 {
			return ContextResult{}, fmt.Errorf("Overture source result exceeds 8 MiB")
		}
		if err = json.Unmarshal(stdout.Bytes(), &response); err != nil {
			return ContextResult{}, fmt.Errorf("invalid Overture worker response")
		}
		if err = validateOverture(response, in); err != nil {
			return ContextResult{}, err
		}
		id, err = c.store.PutPackage(response)
		if err != nil {
			return ContextResult{}, err
		}
		b, _ := json.Marshal(cacheEntry{SnapshotID: id, ExpiresAt: time.Now().Add(24 * time.Hour)})
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return ContextResult{}, err
		}
		if err = os.WriteFile(path, b, 0600); err != nil {
			return ContextResult{}, err
		}
	}
	if err := validateOverture(response, in); err != nil {
		return ContextResult{}, err
	}
	out := ContextResult{CacheHit: hit, ContextEvidence: ContextEvidence{Status: "available", Latitude: in.Latitude, Longitude: in.Longitude, RadiusM: in.RadiusM, POIs: []POI{}, Counts: map[string]int{}, NearestDistanceM: map[string]float64{}, Source: Receipt{Provider: "overture_places", URL: "https://docs.overturemaps.org/guides/places/", RetrievedAt: response.RetrievedAt, SnapshotID: id, Attribution: response.Attribution}, DatasetRelease: response.Release, Method: "overture_point_haversine_v1", Warnings: []string{"Release-based Overture place data, not an exhaustive or live census of amenities. Country guard uses source addresses; exact state/property identity is unverified.", "Only supported mapped place categories with country IN are included. Distances are straight-line to mapped points, not travel times.", "Per-feature upstream source metadata is preserved in the archived extract. Confidence/coverage is not a price-accuracy measurement."}}}
	for _, p := range response.Places {
		d := haversine(in.Latitude, in.Longitude, p.Latitude, p.Longitude)
		if d > float64(in.RadiusM) {
			continue
		}
		out.POIs = append(out.POIs, POI{ID: p.ID, Name: p.Name, Category: p.Category, Latitude: p.Latitude, Longitude: p.Longitude, DistanceM: math.Round(d*10) / 10, URL: "https://explore.overturemaps.org/"})
	}
	sort.Slice(out.POIs, func(i, j int) bool {
		if out.POIs[i].DistanceM == out.POIs[j].DistanceM {
			return out.POIs[i].ID < out.POIs[j].ID
		}
		return out.POIs[i].DistanceM < out.POIs[j].DistanceM
	})
	if len(out.POIs) > 500 {
		out.Status = "truncated"
		out.POIs = out.POIs[:500]
		out.Warnings = append(out.Warnings, "Result capped at 500 mapped places; counts are partial.")
	}
	for _, p := range out.POIs {
		out.Counts[p.Category]++
		if _, ok := out.NearestDistanceM[p.Category]; !ok {
			out.NearestDistanceM[p.Category] = p.DistanceM
		}
	}
	return out, nil
}

func validateOverture(r overtureResponse, in ContextInput) error {
	if r.Error != "" || r.Provider != "overture_places" || r.Release == "" || r.Attribution == "" || r.Places == nil || len(r.BBox) != 4 {
		return fmt.Errorf("invalid or incomplete Overture extract: %s", r.Error)
	}
	if _, err := time.Parse(time.RFC3339Nano, r.RetrievedAt); err != nil {
		return fmt.Errorf("invalid Overture retrieval time")
	}
	dy := float64(in.RadiusM) / 110000
	dx := float64(in.RadiusM) / (110000 * math.Cos(in.Latitude*math.Pi/180))
	if r.BBox[0] > in.Longitude-dx+0.00001 || r.BBox[1] > in.Latitude-dy+0.00001 || r.BBox[2] < in.Longitude+dx-0.00001 || r.BBox[3] < in.Latitude+dy-0.00001 {
		return fmt.Errorf("Overture extract does not cover requested bounding box")
	}
	for _, p := range r.Places {
		if !IndiaBounds(p.Latitude, p.Longitude) || p.ID == "" || (p.Category != "education" && p.Category != "healthcare" && p.Category != "bus" && p.Category != "rail") {
			return fmt.Errorf("invalid Overture place")
		}
	}
	return nil
}
func bounded(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
