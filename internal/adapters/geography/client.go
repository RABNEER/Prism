// Package geography provides bounded, cached live geographic evidence.
package geography

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"prism/internal/storage"
)

const Version = "0.2.0"

type Config struct {
	GeocodingURL   string
	OverpassURL    string
	UserAgent      string
	OverturePython string
	OvertureScript string
	PythonModules  string
}

func Defaults() Config {
	return Config{GeocodingURL: "https://geocoding-api.open-meteo.com/v1/search", OverpassURL: "https://overpass-api.de/api/interpreter", UserAgent: "PRISM-PropertyIntelligence/" + Version + " (educational prototype)"}
}

func (c *Client) Providers() []string {
	out := []string{"open_meteo_geonames", "osm_overpass"}
	if c.config.OvertureScript != "" {
		out = append(out, "overture_places")
	}
	return out
}

type Client struct {
	config      Config
	store       storage.Store
	http        *http.Client
	gate        chan struct{}
	lastRequest time.Time
	cooldowns   map[string]time.Time
}

func New(config Config, store storage.Store) (*Client, error) {
	for _, endpoint := range []string{config.GeocodingURL, config.OverpassURL} {
		u, err := url.Parse(endpoint)
		if err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"))) || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, fmt.Errorf("source endpoint must be HTTPS (HTTP loopback allowed for testing), without credentials, query or fragment")
		}
	}
	if strings.TrimSpace(config.UserAgent) == "" || strings.ContainsAny(config.UserAgent, "\r\n") {
		return nil, fmt.Errorf("valid application User-Agent required")
	}
	return &Client{config: config, store: store, http: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, gate: make(chan struct{}, 1), cooldowns: map[string]time.Time{}}, nil
}

type Receipt struct {
	Provider    string `json:"provider"`
	URL         string `json:"url"`
	RetrievedAt string `json:"retrieved_at"`
	SnapshotID  string `json:"snapshot_id"`
	CacheHit    bool   `json:"-"`
	Attribution string `json:"attribution"`
}

type snapshot struct {
	Kind        string          `json:"kind"`
	RequestHash string          `json:"request_hash"`
	Provider    string          `json:"provider"`
	URL         string          `json:"url"`
	RetrievedAt string          `json:"retrieved_at"`
	Attribution string          `json:"attribution"`
	Data        json.RawMessage `json:"data"`
}
type cacheEntry struct {
	SnapshotID string    `json:"snapshot_id"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// Keep failed/partial provider responses for audit, but do not reuse them.
func (c *Client) discard(receipt Receipt) {
	b, err := c.store.Get(receipt.SnapshotID)
	if err != nil {
		return
	}
	var snap snapshot
	if json.Unmarshal(b, &snap) != nil || len(snap.RequestHash) != 64 {
		return
	}
	if strings.ContainsAny(snap.RequestHash, "/\\.") {
		return
	}
	_ = os.Remove(filepath.Join(c.store.Root, "source-cache", snap.RequestHash+".json"))
}

// fetch serializes cache misses, limits response bytes, archives the source
// response before use, and never retries/falls back silently to stale data.
func (c *Client) fetch(ctx context.Context, provider, endpoint, method string, params url.Values, attribution string, ttl time.Duration) ([]byte, Receipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	select {
	case c.gate <- struct{}{}:
		defer func() { <-c.gate }()
	case <-ctx.Done():
		return nil, Receipt{}, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, Receipt{}, err
	}
	if until := c.cooldowns[provider]; time.Now().Before(until) {
		return nil, Receipt{}, fmt.Errorf("%s unavailable: cooling down after upstream error until %s", provider, until.UTC().Format(time.RFC3339))
	}
	encoded := params.Encode()
	key := storage.Digest([]byte(provider + "\n" + endpoint + "\n" + method + "\n" + encoded))
	cachePath := filepath.Join(c.store.Root, "source-cache", key+".json")
	var cached cacheEntry
	if b, err := os.ReadFile(cachePath); err == nil && json.Unmarshal(b, &cached) == nil && time.Now().Before(cached.ExpiresAt) {
		b, err := c.store.Get(cached.SnapshotID)
		if err != nil {
			return nil, Receipt{}, fmt.Errorf("cached source integrity: %w", err)
		}
		var snap snapshot
		if err = json.Unmarshal(b, &snap); err != nil || snap.RequestHash != key || snap.Provider != provider || snap.URL != endpoint || snap.Kind != "source_snapshot" {
			return nil, Receipt{}, fmt.Errorf("invalid cached source snapshot")
		}
		return snap.Data, Receipt{Provider: provider, URL: endpoint, RetrievedAt: snap.RetrievedAt, SnapshotID: cached.SnapshotID, CacheHit: true, Attribution: snap.Attribution}, nil
	}
	if wait := time.Until(c.lastRequest.Add(1100 * time.Millisecond)); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return nil, Receipt{}, ctx.Err()
		}
	}
	u := endpoint
	var body io.Reader
	if method == http.MethodGet {
		u += "?" + encoded
	} else {
		body = strings.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, Receipt{}, err
	}
	req.Header.Set("User-Agent", c.config.UserAgent)
	req.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	c.lastRequest = time.Now()
	res, err := c.http.Do(req)
	if err != nil {
		return nil, Receipt{}, fmt.Errorf("%s unavailable: %w", provider, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		until := time.Now().Add(time.Minute)
		if seconds, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && seconds > 0 {
			if next := time.Now().Add(time.Duration(seconds) * time.Second); next.After(until) {
				until = next
			}
		} else if date, err := http.ParseTime(res.Header.Get("Retry-After")); err == nil && date.After(until) {
			until = date
		}
		c.cooldowns[provider] = until
		return nil, Receipt{}, fmt.Errorf("%s HTTP %d (Retry-After: %s); no automatic retry", provider, res.StatusCode, res.Header.Get("Retry-After"))
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 2*1024*1024+1))
	if err != nil {
		return nil, Receipt{}, err
	}
	if len(b) > 2*1024*1024 {
		return nil, Receipt{}, fmt.Errorf("%s response exceeds 2 MiB; reduce query", provider)
	}
	if !json.Valid(b) {
		return nil, Receipt{}, fmt.Errorf("%s returned invalid JSON", provider)
	}
	// Compact JSON makes snapshots bounded and deterministic for a given response.
	var compact bytes.Buffer
	if err = json.Compact(&compact, b); err != nil {
		return nil, Receipt{}, err
	}
	snap := snapshot{Kind: "source_snapshot", RequestHash: key, Provider: provider, URL: endpoint, RetrievedAt: time.Now().UTC().Format(time.RFC3339Nano), Attribution: attribution, Data: compact.Bytes()}
	id, err := c.store.PutPackage(snap)
	if err != nil {
		return nil, Receipt{}, fmt.Errorf("archive source response: %w", err)
	}
	entry, _ := json.Marshal(cacheEntry{SnapshotID: id, ExpiresAt: time.Now().Add(ttl)})
	if err = os.MkdirAll(filepath.Dir(cachePath), 0700); err != nil {
		return nil, Receipt{}, err
	}
	if err = os.WriteFile(cachePath, entry, 0600); err != nil {
		return nil, Receipt{}, err
	}
	return snap.Data, Receipt{Provider: provider, URL: endpoint, RetrievedAt: snap.RetrievedAt, SnapshotID: id, Attribution: attribution}, nil
}
