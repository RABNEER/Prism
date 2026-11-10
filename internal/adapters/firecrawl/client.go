// Package firecrawl extracts permitted, single-property Pune listing pages.
package firecrawl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"prism/internal/coverage"
	"prism/internal/domain"
	"prism/internal/storage"
)

type Config struct {
	Endpoint       string
	APIKey         string
	AllowAnonymous bool
	SelfHosted     bool
	Sources        map[string]string
}
type Client struct {
	config Config
	store  storage.Store
	http   *http.Client
}

func New(config Config, store storage.Store) (*Client, error) {
	u, err := url.Parse(config.Endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && u.Hostname() == "127.0.0.1")) {
		return nil, fmt.Errorf("invalid Firecrawl endpoint")
	}
	if config.SelfHosted && u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1" {
		return nil, fmt.Errorf("self-hosted mode requires a loopback Firecrawl endpoint")
	}
	return &Client{config: config, store: store, http: &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

type Status struct {
	Configured       bool     `json:"configured"`
	Authentication   string   `json:"authentication"`
	PermittedDomains []string `json:"permitted_domains"`
	PilotLocalities  []string `json:"pilot_localities"`
}

func (c *Client) Status() Status {
	s := Status{Authentication: "missing", PermittedDomains: []string{}, PilotLocalities: []string{"Baner", "Kharadi"}}
	if c.config.SelfHosted {
		s.Authentication = "self_hosted"
	} else if c.config.APIKey != "" {
		s.Authentication = "api_key"
	} else if c.config.AllowAnonymous {
		s.Authentication = "anonymous_trial"
	}
	for host, reference := range c.config.Sources {
		if reference != "" {
			s.PermittedDomains = append(s.PermittedDomains, host)
		}
	}
	sort.Strings(s.PermittedDomains)
	s.Configured = s.Authentication != "missing" && len(s.PermittedDomains) > 0
	return s
}

type Input struct {
	URL             string `json:"url"`
	Locality        string `json:"locality" jsonschema:"Pune pilot: Baner or Kharadi"`
	expectedPurpose string
}
type Result struct {
	Record             domain.Evidence `json:"record"`
	SourceSnapshotID   string          `json:"source_snapshot_id"`
	ExtractionWarnings []string        `json:"extraction_warnings"`
}
type extracted struct {
	Address       string `json:"address"`
	District      string `json:"district"`
	State         string `json:"state"`
	Locality      string `json:"locality"`
	PropertyType  string `json:"property_type"`
	Project       string `json:"project"`
	UnitID        string `json:"unit_id"`
	Purpose       string `json:"purpose"`
	PriceText     string `json:"price_display_text"`
	AreaText      string `json:"area_display_text"`
	AreaBasis     string `json:"area_basis"`
	PublishedDate string `json:"published_date"`
	parsedPrice   *domain.Price
	parsedArea    *domain.Area
	sourceText    string
	method        string
	extraWarnings []string
}

func (c *Client) Scrape(ctx context.Context, in Input) (Result, error) {
	if !c.Status().Configured {
		return Result{}, fmt.Errorf("Firecrawl requires configured credentials, explicit anonymous trial or loopback self-hosted mode, and permitted source domains")
	}
	locality := coverage.Normalize(in.Locality)
	if locality != "baner" && locality != "kharadi" {
		return Result{}, fmt.Errorf("listing ingestion is limited to Baner and Kharadi, Pune")
	}
	u, err := url.Parse(in.URL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Port() != "" || u.Fragment != "" {
		return Result{}, fmt.Errorf("listing URL must be a public HTTPS URL")
	}
	permission := c.config.Sources[strings.ToLower(u.Hostname())]
	if permission == "" {
		return Result{}, fmt.Errorf("source domain is not in the operator's permitted-source list")
	}
	payload, _ := json.Marshal(map[string]any{"url": u.String(), "formats": []string{"markdown", "rawHtml"}, "onlyMainContent": true, "maxAge": 86400000, "timeout": 60000, "storeInCache": false})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" && !c.config.SelfHosted {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("Firecrawl unavailable: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return Result{}, fmt.Errorf("Firecrawl HTTP %d; no automatic retry", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 4*1024*1024+1))
	if err != nil {
		return Result{}, err
	}
	if len(b) > 4*1024*1024 {
		return Result{}, fmt.Errorf("Firecrawl response exceeds 4 MiB")
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			RawHTML  string `json:"rawHtml"`
			HTML     string `json:"html"`
			Markdown string `json:"markdown"`
			Metadata struct {
				SourceURL  string `json:"sourceURL"`
				StatusCode int    `json:"statusCode"`
			} `json:"metadata"`
		} `json:"data"`
	}
	if err = json.Unmarshal(b, &response); err != nil || !response.Success || (response.Data.Markdown == "" && response.Data.HTML == "" && response.Data.RawHTML == "") {
		return Result{}, fmt.Errorf("Firecrawl returned no usable source document")
	}
	if response.Data.Metadata.StatusCode != 0 && response.Data.Metadata.StatusCode != 200 {
		return Result{}, fmt.Errorf("target listing returned status %d", response.Data.Metadata.StatusCode)
	}
	final, err := url.Parse(response.Data.Metadata.SourceURL)
	if err != nil || final.Scheme != "https" || final.Host != u.Host {
		return Result{}, fmt.Errorf("Firecrawl source provenance missing or changed origin")
	}
	retrieved := time.Now().UTC().Format(time.RFC3339Nano)
	archive := struct {
		Kind        string          `json:"kind"`
		URL         string          `json:"url"`
		RetrievedAt string          `json:"retrieved_at"`
		Response    json.RawMessage `json:"response"`
	}{"firecrawl_source_response", u.String(), retrieved, b}
	snapshotID, err := c.store.PutPackage(archive)
	if err != nil {
		return Result{}, err
	}
	document := response.Data.RawHTML
	if document == "" {
		document = response.Data.HTML
	}
	fields, err := parseDocumentForSource(response.Data.Markdown, document, u.String())
	if err != nil {
		return Result{}, fmt.Errorf("listing parser: %w; source snapshot %s", err, snapshotID)
	}
	state, err := coverage.Resolve(fields.State)
	if err != nil || state.Name != "Maharashtra" || coverage.Normalize(fields.District) != "pune" || coverage.Normalize(fields.Locality) != locality || fields.PropertyType != "apartment" || strings.TrimSpace(fields.Address) == "" || (fields.Purpose != "sale" && fields.Purpose != "rent") {
		return Result{}, fmt.Errorf("page does not establish a supported Pune apartment listing; source snapshot %s", snapshotID)
	}
	if in.expectedPurpose != "" && fields.Purpose != in.expectedPurpose {
		return Result{}, fmt.Errorf("source purpose differs from discovery query; source snapshot %s", snapshotID)
	}
	markdown := fields.sourceText
	warnings := []string{"Asking-price or rent evidence only, not a completed sale or verified physical property.", "Deterministic parser " + fields.method + "; source assertions require review. Retrieval time is not the listing publication date."}
	warnings = append(warnings, fields.extraWarnings...)
	canonicalLocality := "Baner"
	if locality == "kharadi" {
		canonicalLocality = "Kharadi"
	}
	e := domain.Evidence{State: "Maharashtra", District: "Pune", Locality: canonicalLocality, PropertyType: "apartment", EntityLevel: "unit", Address: fields.Address, Project: fields.Project, UnitID: fields.UnitID, ProviderRecordID: storage.Digest([]byte(u.String())), Source: domain.Source{ID: "listing:" + u.Hostname(), Name: u.Hostname(), URL: u.String(), AccessReference: permission}, Category: "listing", DataMode: "imported", RetrievedAt: retrieved, Warnings: warnings, SourceSnapshotID: snapshotID}
	e.Price, e.Area = fields.parsedPrice, fields.parsedArea
	if fields.PublishedDate != "" {
		if date, err := time.Parse(time.DateOnly, fields.PublishedDate); err == nil && strings.Contains(markdown, fields.PublishedDate) && date.Before(time.Now().UTC().AddDate(0, 0, 1)) {
			e.PublishedDate = fields.PublishedDate
		} else {
			warnings = append(warnings, "Publication date was not verified in source text; left unknown.")
		}
	}
	if e.Price == nil && fields.PriceText != "" && strings.Contains(markdown, fields.PriceText) {
		if amount, ok := parseListingPrice(fields.PriceText, fields.Purpose); ok {
			kind := "asking_price"
			if fields.Purpose == "rent" {
				kind = "monthly_rent"
			}
			e.Price = &domain.Price{Amount: amount, Currency: "INR", Type: kind}
		}
	}
	if e.Price == nil {
		warnings = append(warnings, "No machine-verified asking-price text; price requires review.")
	}
	if e.Area == nil && fields.AreaText != "" && strings.Contains(markdown, fields.AreaText) {
		e.Area = parseArea(fields.AreaText, sourceAreaBasis(markdown, fields.AreaText, fields.AreaBasis))
	}
	if e.Area == nil {
		warnings = append(warnings, "Area could not be parsed; no area inferred.")
	}
	e.Warnings = warnings
	if err = e.Validate(); err != nil {
		return Result{}, err
	}
	e, err = c.store.PutEvidence(e)
	if err != nil {
		return Result{}, err
	}
	return Result{Record: e, SourceSnapshotID: snapshotID, ExtractionWarnings: warnings}, nil
}

var pricePattern = regexp.MustCompile(`(?i)^\s*(?:₹|rs\.?|inr)?\s*([0-9][0-9,]*(?:\.[0-9]+)?)\s*(crore|cr|lakh|lac|l|k)?\s*(?:/month|per month|monthly)?\s*$`)

func parseListingPrice(text, purpose string) (float64, bool) {
	monthly := strings.Contains(strings.ToLower(text), "/month") || strings.Contains(strings.ToLower(text), "per month") || strings.Contains(strings.ToLower(text), "monthly")
	if (purpose == "rent" && !monthly) || (purpose == "sale" && monthly) {
		return 0, false
	}
	return parsePrice(text)
}

var basisPattern = regexp.MustCompile(`(?i)\b(super\s+built[- ]?up|built[- ]?up|carpet)\b`)

func sourceAreaBasis(markdown, areaText, proposed string) string {
	i := strings.Index(strings.ToLower(markdown), strings.ToLower(areaText))
	if i < 0 || areaText == "" {
		return "unknown"
	}
	start, end := max(0, i-80), min(len(markdown), i+len(areaText)+80)
	seen := map[string]bool{}
	for _, match := range basisPattern.FindAllString(markdown[start:end], -1) {
		basis := "built_up"
		if strings.EqualFold(match, "carpet") {
			basis = "carpet"
		} else if strings.HasPrefix(strings.ToLower(match), "super") {
			basis = "super_built_up"
		}
		seen[basis] = true
	}
	if len(seen) == 1 && seen[proposed] {
		return proposed
	}
	return "unknown"
}

func parsePrice(text string) (float64, bool) {
	m := pricePattern.FindStringSubmatch(text)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	switch strings.ToLower(m[2]) {
	case "crore", "cr":
		n *= 10000000
	case "lakh", "lac", "l":
		n *= 100000
	case "k":
		n *= 1000
	}
	return n, err == nil && n > 0 && !math.IsInf(n, 0)
}

var areaPattern = regexp.MustCompile(`(?i)^\s*([0-9][0-9,]*(?:\.[0-9]+)?)\s*(sq\.?\s*ft\.?|sqft|sq\.?\s*m\.?|sqm|m²)\s*$`)

func parseArea(text, basis string) *domain.Area {
	m := areaPattern.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	n, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	if err != nil || n <= 0 {
		return nil
	}
	unit := "sq_ft"
	if strings.Contains(strings.ToLower(m[2]), "m") {
		unit = "sq_m"
	}
	if basis != "carpet" && basis != "built_up" && basis != "super_built_up" {
		basis = "unknown"
	}
	return &domain.Area{Value: n, Unit: unit, Basis: basis}
}
