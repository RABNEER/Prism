package firecrawl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"prism/internal/coverage"
	"prism/internal/storage"
)

type DiscoveryInput struct {
	IndividualPages bool   `json:"individual_pages,omitempty" jsonschema:"Prefer individual pages by excluding known collection URL patterns from the search query; heuristic only"`
	Locality        string `json:"locality" jsonschema:"Baner or Kharadi, Pune"`
	Purpose         string `json:"purpose,omitempty" jsonschema:"sale or rent; defaults to sale"`
	Limit           int    `json:"limit,omitempty" jsonschema:"1–10 candidate pages; defaults to 5"`
}
type SourceCandidate struct {
	DiscoverySnapshotID string `json:"discovery_snapshot_id,omitempty"`
	FoundOn             string `json:"found_on,omitempty"`
	ID                  string `json:"id"`
	URL                 string `json:"url"`
	Domain              string `json:"domain"`
	Title               string `json:"title"`
	Description         string `json:"description"`
	LikelyCollection    bool   `json:"likely_collection"`
	AccessStatus        string `json:"access_status"`
}
type Discovery struct {
	ExpansionSnapshotIDs []string          `json:"expansion_snapshot_ids,omitempty"`
	Kind                 string            `json:"kind"`
	Query                string            `json:"query"`
	Locality             string            `json:"locality"`
	Purpose              string            `json:"purpose"`
	RetrievedAt          string            `json:"retrieved_at"`
	Candidates           []SourceCandidate `json:"candidates"`
	SearchSnapshotID     string            `json:"search_snapshot_id"`
	Warnings             []string          `json:"warnings"`
}
type DiscoveryResult struct {
	DiscoveryID string    `json:"discovery_id"`
	Status      string    `json:"status"`
	Discovery   Discovery `json:"discovery"`
}
type DiscoveredInput struct {
	DiscoveryID string `json:"discovery_id"`
	CandidateID string `json:"candidate_id"`
}

func publicCandidate(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Port() != "" {
		return nil, fmt.Errorf("public HTTPS URL required")
	}
	host := strings.ToLower(u.Hostname())
	if net.ParseIP(host) != nil || !strings.Contains(host, ".") || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return nil, fmt.Errorf("non-public hostname")
	}
	u.Host = host
	u.Fragment = ""
	return u, nil
}

func (c *Client) Discover(ctx context.Context, in DiscoveryInput) (DiscoveryResult, error) {
	auth := c.Status().Authentication
	if auth == "missing" {
		return DiscoveryResult{}, fmt.Errorf("configure Firecrawl or explicit loopback self-hosted mode for discovery")
	}
	locality := coverage.Normalize(in.Locality)
	if locality != "baner" && locality != "kharadi" {
		return DiscoveryResult{}, fmt.Errorf("discovery pilot covers Baner and Kharadi")
	}
	if in.Purpose == "" {
		in.Purpose = "sale"
	}
	if in.Purpose != "sale" && in.Purpose != "rent" {
		return DiscoveryResult{}, fmt.Errorf("purpose must be sale or rent")
	}
	if in.Limit == 0 {
		in.Limit = 5
	}
	if in.Limit < 1 || in.Limit > 10 {
		return DiscoveryResult{}, fmt.Errorf("limit must be 1–10")
	}
	canonical := "Baner"
	if locality == "kharadi" {
		canonical = "Kharadi"
	}
	query := fmt.Sprintf(`"%s" "Pune" apartment "for %s"`, canonical, in.Purpose)
	if in.IndividualPages {
		query += " -inurl:pppfs -inurl:ffid -inurl:gid -inurl:search"
	}
	base, _ := url.Parse(c.config.Endpoint)
	base.Path = "/v2/search"
	base.RawPath = ""
	payload, _ := json.Marshal(map[string]any{"query": query, "limit": in.Limit, "sources": []string{"web"}, "country": "in", "lang": "en"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String(), bytes.NewReader(payload))
	if err != nil {
		return DiscoveryResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" && !c.config.SelfHosted {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return DiscoveryResult{}, fmt.Errorf("Firecrawl search unavailable: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return DiscoveryResult{}, fmt.Errorf("Firecrawl search HTTP %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 2*1024*1024+1))
	if err != nil {
		return DiscoveryResult{}, err
	}
	if len(raw) > 2*1024*1024 {
		return DiscoveryResult{}, fmt.Errorf("search response exceeds 2 MiB")
	}
	retrieved := time.Now().UTC().Format(time.RFC3339Nano)
	archive := struct {
		Kind        string          `json:"kind"`
		Query       string          `json:"query"`
		RetrievedAt string          `json:"retrieved_at"`
		Response    json.RawMessage `json:"response"`
	}{"firecrawl_search_response", query, retrieved, raw}
	if !json.Valid(raw) {
		return DiscoveryResult{}, fmt.Errorf("search response is not JSON")
	}
	snapshot, err := c.store.PutPackage(archive)
	if err != nil {
		return DiscoveryResult{}, err
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Web []struct{ URL, Title, Description string } `json:"web"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &response) != nil || !response.Success {
		return DiscoveryResult{}, fmt.Errorf("search response unusable; source snapshot %s", snapshot)
	}
	out := Discovery{Kind: "property_source_discovery_v1", Query: query, Locality: canonical, Purpose: in.Purpose, RetrievedAt: retrieved, Candidates: []SourceCandidate{}, SearchSnapshotID: snapshot, Warnings: []string{"Search snippets are unverified discovery hints, never property-price evidence.", "An empty search can reflect provider failure, blocking or index coverage; it does not establish that no properties exist.", "Collection classification is heuristic. Individual page layout and source assertions require validation during ingestion."}}
	seen := map[string]bool{}
	for _, item := range response.Data.Web {
		u, err := publicCandidate(item.URL)
		if err != nil || seen[u.String()] {
			continue
		}
		seen[u.String()] = true
		path := strings.ToLower(u.Path)
		collection := collectionURL(path)
		out.Candidates = append(out.Candidates, SourceCandidate{ID: storage.Digest([]byte(u.String())), URL: u.String(), Domain: u.Hostname(), Title: item.Title, Description: item.Description, LikelyCollection: collection, AccessStatus: "discovered_public_url_usage_unreviewed", DiscoverySnapshotID: snapshot})
		if len(out.Candidates) == in.Limit {
			break
		}
	}
	if in.IndividualPages {
		for _, source := range out.Candidates {
			if !source.LikelyCollection {
				continue
			}
			links, archive, err := c.expandSourceLinks(ctx, source, in.Limit)
			if archive != "" {
				out.ExpansionSnapshotIDs = append(out.ExpansionSnapshotIDs, archive)
			}
			if err != nil {
				out.Warnings = append(out.Warnings, "Index link discovery failed: "+err.Error())
			} else if len(links) == 0 {
				out.Warnings = append(out.Warnings, "The source index returned no recognized individual listing links; source layout or access may be unsupported.")
			} else {
				combined := append(links, out.Candidates...)
				unique := []SourceCandidate{}
				urls := map[string]bool{}
				for _, candidate := range combined {
					if !urls[candidate.URL] {
						unique = append(unique, candidate)
						urls[candidate.URL] = true
					}
					if len(unique) == in.Limit {
						break
					}
				}
				out.Candidates = unique
			}
			break // At most one source-index request per discovery.
		}
	}
	id, err := c.store.PutPackage(out)
	if err != nil {
		return DiscoveryResult{}, err
	}
	status := "candidates_found"
	if len(out.Candidates) == 0 {
		status = "no_results_or_provider_unavailable"
	} else {
		allCollections := true
		for _, candidate := range out.Candidates {
			if !candidate.LikelyCollection {
				allCollections = false
				break
			}
		}
		if allCollections {
			status = "source_pages_found_no_individual_links"
		}
	}
	return DiscoveryResult{DiscoveryID: id, Status: status, Discovery: out}, nil
}

func (c *Client) IngestDiscovered(ctx context.Context, in DiscoveredInput) (Result, error) {
	raw, err := c.store.Get(in.DiscoveryID)
	if err != nil {
		return Result{}, err
	}
	var discovery Discovery
	if json.Unmarshal(raw, &discovery) != nil || discovery.Kind != "property_source_discovery_v1" {
		return Result{}, fmt.Errorf("not a saved source discovery")
	}
	for _, candidate := range discovery.Candidates {
		if candidate.ID != in.CandidateID {
			continue
		}
		u, err := publicCandidate(candidate.URL)
		if err != nil || storage.Digest([]byte(candidate.URL)) != candidate.ID {
			return Result{}, fmt.Errorf("invalid saved candidate")
		}
		if candidate.LikelyCollection {
			return Result{}, fmt.Errorf("candidate is a likely collection page, not an individual apartment listing")
		}
		config := c.config
		config.Sources = map[string]string{u.Hostname(): "Public source discovered by PRISM; discovery " + in.DiscoveryID + "; discovery snapshot " + candidate.DiscoverySnapshotID + "; source usage rights unreviewed, not a license assertion"}
		isolated := &Client{config: config, store: c.store, http: c.http}
		return isolated.Scrape(ctx, Input{URL: u.String(), Locality: discovery.Locality, expectedPurpose: discovery.Purpose})
	}
	return Result{}, fmt.Errorf("candidate does not belong to this saved discovery")
}
