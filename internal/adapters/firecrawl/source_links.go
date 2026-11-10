package firecrawl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"prism/internal/storage"
	"strings"
	"time"
)

func collectionURL(path string) bool {
	path = strings.ToLower(path)
	return strings.Contains(path, "-pppfs") || strings.Contains(path, "-ffid") || strings.HasSuffix(path, "-gid/") || strings.Contains(path, "/search") || strings.Contains(path, "/sale/apartments-for-sale-in-") || strings.HasPrefix(path, "/flats-for-sale-in-") || strings.HasPrefix(path, "/flats-for-rent-in-")
}
func individualURL(path string) bool {
	path = strings.ToLower(path)
	return strings.Contains(path, "/propertydetails/") || strings.Contains(path, "-spid-") || strings.Contains(path, "/buy/resale/page/") || strings.Contains(path, "/resale-") || strings.Contains(path, "/property/buy/") || strings.Contains(path, "/property/rent/")
}

func (c *Client) expandSourceLinks(ctx context.Context, source SourceCandidate, limit int) ([]SourceCandidate, string, error) {
	base, err := publicCandidate(source.URL)
	if err != nil {
		return nil, "", err
	}
	payload, _ := json.Marshal(map[string]any{"url": base.String(), "formats": []string{"links"}, "onlyMainContent": true, "maxAge": 86400000, "timeout": 30000, "storeInCache": false})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" && !c.config.SelfHosted {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, "", fmt.Errorf("source link request HTTP %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1024*1024+1))
	if err != nil {
		return nil, "", err
	}
	if len(raw) > 1024*1024 || !json.Valid(raw) {
		return nil, "", fmt.Errorf("source link response invalid or exceeds 1 MiB")
	}
	snapshot, err := c.store.PutPackage(struct {
		Kind        string          `json:"kind"`
		URL         string          `json:"url"`
		RetrievedAt string          `json:"retrieved_at"`
		Response    json.RawMessage `json:"response"`
	}{"firecrawl_source_links", base.String(), time.Now().UTC().Format(time.RFC3339Nano), raw})
	if err != nil {
		return nil, "", err
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Links    []string `json:"links"`
			Metadata struct {
				SourceURL  string `json:"sourceURL"`
				StatusCode int    `json:"statusCode"`
			} `json:"metadata"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &response) != nil || !response.Success || response.Data.Metadata.StatusCode != 200 {
		return nil, snapshot, fmt.Errorf("source links unavailable or page blocked")
	}
	origin, err := publicCandidate(response.Data.Metadata.SourceURL)
	if err != nil || origin.Host != base.Host {
		return nil, snapshot, fmt.Errorf("source link provenance changed origin")
	}
	out := []SourceCandidate{}
	seen := map[string]bool{}
	for _, link := range response.Data.Links {
		ref, err := url.Parse(link)
		if err != nil {
			continue
		}
		u, err := publicCandidate(base.ResolveReference(ref).String())
		if err != nil || u.Host != base.Host || collectionURL(u.Path) || !individualURL(u.Path) || seen[u.String()] {
			continue
		}
		seen[u.String()] = true
		out = append(out, SourceCandidate{ID: storage.Digest([]byte(u.String())), URL: u.String(), Domain: u.Hostname(), Title: "Individual listing candidate", AccessStatus: "discovered_public_url_usage_unreviewed", DiscoverySnapshotID: snapshot, FoundOn: source.URL})
		if len(out) == limit {
			break
		}
	}
	return out, snapshot, nil
}
