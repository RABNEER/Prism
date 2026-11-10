package geography

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"prism/internal/storage"
)

func mock(t *testing.T, handler http.HandlerFunc) (*Client, *int) {
	t.Helper()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "PRISM") {
			t.Error("missing application identifier")
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	c, err := New(Config{GeocodingURL: server.URL + "/search", OverpassURL: server.URL + "/overpass", UserAgent: "PRISM-test"}, storage.Store{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return c, &calls
}

func TestGeocodingFilterCacheAndArchive(t *testing.T) {
	c, calls := mock(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("countryCode") != "IN" || r.URL.Query().Get("name") != "Pune, Maharashtra" {
			t.Error("missing country/state filter")
		}
		fmt.Fprint(w, `{"results":[{"id":1,"name":"Pune","country_code":"IN","admin1":"Maharashtra","admin2":"Pune","latitude":18.52,"longitude":73.85,"feature_code":"PPLA2"},{"id":2,"name":"Other","country_code":"IN","admin1":"Karnataka","latitude":12.9,"longitude":77.6}]}`)
	})
	a, err := c.Geocode(context.Background(), GeocodeInput{Place: "Pune", State: "MH"})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Candidates) != 1 || a.Candidates[0].State != "Maharashtra" || a.CacheHit {
		t.Fatal("bad candidate result", a)
	}
	b, err := c.Geocode(context.Background(), GeocodeInput{Place: "Pune", State: "MH"})
	if err != nil {
		t.Fatal(err)
	}
	if !b.CacheHit || *calls != 1 || a.Source.SnapshotID != b.Source.SnapshotID {
		t.Fatal("cache failed")
	}
	archive, err := c.store.Get(a.Source.SnapshotID)
	if err != nil {
		t.Fatal(err)
	}
	var snap snapshot
	if err = json.Unmarshal(archive, &snap); err != nil || snap.Provider != "open_meteo_geonames" || snap.Attribution == "" {
		t.Fatal("missing source lineage")
	}
	if _, err = c.Geocode(context.Background(), GeocodeInput{Place: "Pune, India", State: "MH"}); err == nil {
		t.Fatal("accepted qualifier injection")
	}
}

func TestContextDistancesDatesAndPartialResponse(t *testing.T) {
	c, calls := mock(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		q := r.Form.Get("data")
		if r.Method != "POST" || !strings.Contains(q, "area.india") || !strings.Contains(q, "around:500") {
			t.Error("unbounded or incorrect query")
		}
		fmt.Fprint(w, `{"version":0.6,"osm3s":{"timestamp_osm_base":"2026-11-07T00:00:00Z"},"elements":[{"type":"node","id":1,"lat":18.5205,"lon":73.85,"tags":{"amenity":"school","name":"Fixture school"}},{"type":"way","id":2,"center":{"lat":18.521,"lon":73.85},"tags":{"amenity":"hospital"}},{"type":"node","id":3,"lat":20,"lon":75,"tags":{"amenity":"school"}}]}`)
	})
	in := ContextInput{Latitude: 18.52, Longitude: 73.85}
	a, err := c.Context(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.POIs) != 2 || a.Counts["education"] != 1 || a.Counts["healthcare"] != 1 || a.NearestDistanceM["education"] < 55 || a.NearestDistanceM["education"] > 56 {
		t.Fatal("bad feature calculation", a)
	}
	b, err := c.Context(context.Background(), in)
	if err != nil || !b.CacheHit || *calls != 1 {
		t.Fatal("context cache failed", err)
	}
	before := *calls
	in.AsOf = "2025-01-01"
	if _, err = c.Context(context.Background(), in); err == nil || *calls != before {
		t.Fatal("historical context fetched current data")
	}
	in.AsOf = ""
	in.RadiusM = 999999
	if _, err = c.Context(context.Background(), in); err == nil {
		t.Fatal("accepted unbounded radius")
	}
	partial, pcalls := mock(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version":0.6,"remark":"runtime error: timeout","elements":[]}`)
	})
	for i := 0; i < 2; i++ {
		if _, err := partial.Context(context.Background(), ContextInput{Latitude: 18.52, Longitude: 73.85}); err == nil {
			t.Fatal("accepted partial provider response")
		}
	}
	if *pcalls != 2 {
		t.Fatal("failed semantic response was cached")
	}
}

func TestFailureAndCancellation(t *testing.T) {
	c, calls := mock(t, func(w http.ResponseWriter, r *http.Request) { w.Header().Set("Retry-After", "60"); w.WriteHeader(429) })
	_, err := c.Geocode(context.Background(), GeocodeInput{Place: "Pune", State: "MH"})
	if err == nil || !strings.Contains(err.Error(), "429") || *calls != 1 {
		t.Fatal("failure not surfaced", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = c.Geocode(ctx, GeocodeInput{Place: "Pune", State: "MH"}); err == nil {
		t.Fatal("cancel ignored")
	}
}

func TestRejectIncompleteOverpassEnvelope(t *testing.T) {
	for _, payload := range []string{
		`{"version":0.6}`,
		`{"version":0.6,"elements":[]}`,
		`{"version":0.6,"osm3s":{"timestamp_osm_base":"2026-11-07T00:00:00Z"}}`,
		`{"version":0.6,"osm3s":{"timestamp_osm_base":"invalid"},"elements":[]}`,
		`{"version":0.6,"osm3s":{"timestamp_osm_base":"2026-11-07T00:00:00Z"},"elements":null}`,
	} {
		t.Run(payload, func(t *testing.T) {
			c, _ := mock(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, payload) })
			if _, err := c.Context(context.Background(), ContextInput{Latitude: 18.52, Longitude: 73.85}); err == nil {
				t.Fatal("incomplete source response became available empty context")
			}
		})
	}
}

// Opt-in only: no external calls occur during routine tests.
func TestLiveGeographicSources(t *testing.T) {
	if os.Getenv("PRISM_LIVE_TEST") != "1" {
		t.Skip("set PRISM_LIVE_TEST=1 for a few on-demand live requests")
	}
	root := os.Getenv("PRISM_LIVE_DATA_DIR")
	if root == "" {
		root = t.TempDir()
	}
	c, err := New(Defaults(), storage.Store{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []GeocodeInput{{Place: "Pune", State: "Maharashtra"}, {Place: "Bengaluru", State: "Karnataka"}} {
		out, err := c.Geocode(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Candidates) == 0 {
			t.Fatalf("no live candidates for %s", in.Place)
		}
		t.Logf("%s: %d candidates; source=%s cache=%v", in.Place, len(out.Candidates), out.Source.SnapshotID, out.CacheHit)
	}
	out, err := c.Context(context.Background(), ContextInput{Latitude: 18.5204, Longitude: 73.8567, RadiusM: 500})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Pune current context: %d mapped elements; source=%s timestamp=%s", len(out.POIs), out.Source.SnapshotID, out.DatasetTimestamp)
	if out.Source.SnapshotID == "" || out.DatasetTimestamp == "" {
		t.Fatal("live source provenance missing")
	}
}
