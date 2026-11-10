package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"prism/internal/adapters/firecrawl"
	"prism/internal/adapters/geography"
	"prism/internal/adapters/maharera"
	"prism/internal/coverage"
	"prism/internal/domain"
	"prism/internal/matching"
	"prism/internal/storage"
)

type Service struct {
	Store     storage.Store
	Geography *geography.Client
	Registry  *maharera.Client
	Matcher   *matching.Client
	Firecrawl *firecrawl.Client
}

type CoverageInput struct {
	State        string `json:"state,omitempty" jsonschema:"Omit for all Indian states and union territories"`
	PropertyType string `json:"property_type,omitempty"`
}
type CoverageEntry struct {
	Jurisdiction    coverage.Jurisdiction `json:"jurisdiction"`
	Status          string                `json:"status"`
	ImportedRecords int                   `json:"imported_records"`
	FixtureRecords  int                   `json:"fixture_records"`
	LiveConnectors  []string              `json:"live_connectors"`
}
type CoverageResult struct {
	Country       string          `json:"country"`
	PropertyTypes []string        `json:"property_types"`
	Entries       []CoverageEntry `json:"entries"`
	Warnings      []string        `json:"warnings"`
}

func (s Service) Coverage(ctx context.Context, in CoverageInput) (CoverageResult, error) {
	if in.PropertyType != "" && !domain.ValidPropertyType(in.PropertyType) {
		return CoverageResult{}, fmt.Errorf("unsupported property_type")
	}
	js := coverage.All()
	if in.State != "" {
		j, err := coverage.Resolve(in.State)
		if err != nil {
			return CoverageResult{}, err
		}
		js = []coverage.Jurisdiction{j}
	}
	records, err := s.Store.List()
	if err != nil {
		return CoverageResult{}, err
	}
	out := CoverageResult{Country: "India", PropertyTypes: domain.PropertyTypes, Entries: []CoverageEntry{}, Warnings: []string{"Jurisdiction routing is national; transaction-record availability depends on local imports. No live sale-transaction connector is enabled.", "Imported records are source observations, not independently verified property identities or market values."}}
	if s.Registry != nil {
		out.Warnings = append(out.Warnings, "Maharashtra exact-registration project search is configured, not health-verified by this call; other state registries are not integrated. Project metadata is not sale-price evidence.")
	}
	if s.Geography != nil {
		out.Warnings = append(out.Warnings, "Geographic connectors are configured, not health-verified by this coverage call. Named-place and OSM availability varies by location; they do not provide transaction or property coverage.")
	}
	for _, j := range js {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		entry := CoverageEntry{Jurisdiction: j, Status: "not_yet_integrated", LiveConnectors: []string{}}
		if s.Geography != nil {
			entry.LiveConnectors = s.Geography.Providers()
		}
		if s.Registry != nil && j.Name == "Maharashtra" {
			entry.LiveConnectors = append(entry.LiveConnectors, "maharera_public_project_search")
			entry.Status = "project_lookup_configured"
		}
		for _, e := range records {
			if e.State == j.Name && (in.PropertyType == "" || in.PropertyType == e.PropertyType) {
				if e.DataMode == "synthetic_fixture" {
					entry.FixtureRecords++
				} else {
					entry.ImportedRecords++
				}
			}
		}
		if entry.ImportedRecords > 0 {
			entry.Status = "import_only"
			if s.Registry != nil && j.Name == "Maharashtra" {
				entry.Status = "imports_and_project_lookup_configured"
			}
		}
		out.Entries = append(out.Entries, entry)
	}
	return out, nil
}

type ResolveInput struct {
	State        string `json:"state"`
	Address      string `json:"address"`
	PropertyType string `json:"property_type"`
	District     string `json:"district,omitempty"`
	Locality     string `json:"locality,omitempty"`
}
type Resolved struct {
	Query             domain.Query `json:"query"`
	OriginalAddress   string       `json:"original_address"`
	NormalizedAddress string       `json:"normalized_address"`
	Status            string       `json:"status"`
	EntityVerified    bool         `json:"entity_verified"`
	Warnings          []string     `json:"warnings"`
}

func (s Service) Resolve(in ResolveInput) (Resolved, error) {
	j, err := coverage.Resolve(in.State)
	if err != nil {
		return Resolved{}, err
	}
	if strings.TrimSpace(in.Address) == "" {
		return Resolved{}, fmt.Errorf("address is required")
	}
	q := domain.Query{State: j.Name, PropertyType: in.PropertyType, District: strings.TrimSpace(in.District), Locality: strings.TrimSpace(in.Locality)}
	if err = q.Validate(); err != nil {
		return Resolved{}, err
	}
	return Resolved{Query: q, OriginalAddress: in.Address, NormalizedAddress: strings.Join(strings.Fields(in.Address), " "), Status: "jurisdiction_resolved", Warnings: []string{"State resolved from explicit input only. Address whitespace normalized; address existence, district/locality, coordinates and physical property identity have not been verified."}}, nil
}

type SearchInput struct {
	Query           domain.Query `json:"query"`
	Limit           int          `json:"limit,omitempty" jsonschema:"1-100; defaults to 25"`
	Offset          int          `json:"offset,omitempty" jsonschema:"Non-negative offset; pagination assumes unchanged imports"`
	IncludeFixtures bool         `json:"include_fixtures,omitempty" jsonschema:"Development only; synthetic fixtures are excluded by default"`
}
type SearchResult struct {
	Records    []domain.Evidence `json:"records"`
	Total      int               `json:"total"`
	NextOffset *int              `json:"next_offset,omitempty"`
	Warnings   []string          `json:"warnings"`
}

func (s Service) canonicalQuery(q domain.Query) (domain.Query, error) {
	j, err := coverage.Resolve(q.State)
	if err != nil {
		return q, err
	}
	q.State = j.Name
	q.District = strings.TrimSpace(q.District)
	q.Locality = strings.TrimSpace(q.Locality)
	return q, q.Validate()
}

func (s Service) matching(ctx context.Context, q domain.Query, fixtures bool) ([]domain.Evidence, error) {
	records, err := s.Store.List()
	if err != nil {
		return nil, err
	}
	out := []domain.Evidence{}
	for _, e := range records {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if e.State != q.State || e.PropertyType != q.PropertyType {
			continue
		}
		if q.District != "" && coverage.Normalize(q.District) != coverage.Normalize(e.District) {
			continue
		}
		if q.Locality != "" && coverage.Normalize(q.Locality) != coverage.Normalize(e.Locality) {
			continue
		}
		if !fixtures && e.DataMode == "synthetic_fixture" {
			continue
		}
		if q.AsOf != "" && (e.PublishedDate == "" || e.PublishedDate > q.AsOf || e.EventDate > q.AsOf) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func searchWarnings(q domain.Query) []string {
	out := []string{"The records list searches local imports only. Any requested live enrichment is returned in separate fields. An empty list does not establish absence of property records.", "Matching uses exact normalized district/locality filters; results are unverified source observations, not selected comparable sales."}
	if q.AsOf != "" {
		out = append(out, "Historical filtering uses declared publication/event dates, not independently verified availability. Records with unknown publication dates are excluded.")
	}
	return out
}

func (s Service) Search(ctx context.Context, in SearchInput) (SearchResult, error) {
	q, err := s.canonicalQuery(in.Query)
	if err != nil {
		return SearchResult{}, err
	}
	if in.Limit == 0 {
		in.Limit = 25
	}
	if in.Limit < 1 || in.Limit > 100 || in.Offset < 0 {
		return SearchResult{}, fmt.Errorf("limit must be 1-100 and offset non-negative")
	}
	records, err := s.matching(ctx, q, in.IncludeFixtures)
	if err != nil {
		return SearchResult{}, err
	}
	out := SearchResult{Records: []domain.Evidence{}, Total: len(records), Warnings: searchWarnings(q)}
	if in.Offset >= len(records) {
		return out, nil
	}
	end := in.Offset + in.Limit
	if end > len(records) {
		end = len(records)
	}
	out.Records = records[in.Offset:end]
	if end < len(records) {
		out.NextOffset = &end
	}
	return out, nil
}

type BuildInput struct {
	Query                     domain.Query            `json:"query"`
	IncludeFixtures           bool                    `json:"include_fixtures,omitempty"`
	Location                  *geography.ContextInput `json:"location,omitempty" jsonschema:"Optional current geographic context for supplied coordinates; not automatically matched to the query property"`
	ProjectRegistrationNumber string                  `json:"project_registration_number,omitempty" jsonschema:"Optional exact registration lookup through an integrated state registry; not automatically matched to property attributes"`
	Reconcile                 bool                    `json:"reconcile,omitempty"`
	Comparables               *ComparablesInput       `json:"comparables,omitempty"`
}
type Package struct {
	SchemaVersion       string                     `json:"schema_version"`
	PipelineVersion     string                     `json:"pipeline_version"`
	Query               domain.Query               `json:"query"`
	Records             []domain.Evidence          `json:"records"`
	Status              string                     `json:"status"`
	MissingCapabilities []string                   `json:"missing_capabilities"`
	Warnings            []string                   `json:"warnings"`
	LocationContext     *geography.ContextEvidence `json:"location_context,omitempty"`
	SourceErrors        []string                   `json:"source_errors"`
	RegistryProject     *maharera.Result           `json:"registry_project,omitempty"`
	Reconciliation      *Reconciliation            `json:"reconciliation,omitempty"`
	ComparableSelection *Comparables               `json:"comparable_selection,omitempty"`
}
type PackageResult struct {
	ID          string  `json:"id"`
	GeneratedAt string  `json:"generated_at"`
	Package     Package `json:"package"`
}

func (s Service) Build(ctx context.Context, in BuildInput) (PackageResult, error) {
	q, err := s.canonicalQuery(in.Query)
	if err != nil {
		return PackageResult{}, err
	}
	records, err := s.matching(ctx, q, in.IncludeFixtures)
	if err != nil {
		return PackageResult{}, err
	}
	if len(records) > 1000 {
		return PackageResult{}, fmt.Errorf("package exceeds 1000 records; narrow district/locality filters")
	}
	if in.Location != nil && q.AsOf != "" {
		return PackageResult{}, fmt.Errorf("live location context cannot be attached to historical packages")
	}
	if in.ProjectRegistrationNumber != "" && q.AsOf != "" {
		return PackageResult{}, fmt.Errorf("current project registry data cannot be attached to historical packages")
	}
	p := Package{SchemaVersion: domain.SchemaVersion, PipelineVersion: "0.3.0", Query: q, Records: records, Status: "no_evidence", MissingCapabilities: []string{"live_sale_transaction_retrieval", "physical_property_identity", "property_geocoding"}, Warnings: searchWarnings(q), SourceErrors: []string{}}
	if in.Location != nil {
		location, err := s.LocationContext(ctx, *in.Location)
		if err != nil {
			if ctx.Err() != nil {
				return PackageResult{}, ctx.Err()
			}
			p.SourceErrors = append(p.SourceErrors, err.Error())
			p.MissingCapabilities = append(p.MissingCapabilities, "derived_location_features")
		} else {
			p.LocationContext = &location.ContextEvidence
			p.Status = "partial"
			p.Warnings = append(p.Warnings, "Location context describes supplied coordinates; their association with the query property or jurisdiction has not been verified.")
		}
	} else {
		p.MissingCapabilities = append(p.MissingCapabilities, "derived_location_features")
	}
	if len(records) > 0 {
		p.Status = "partial"
	}
	if in.ProjectRegistrationNumber != "" {
		project, err := s.ProjectRegistry(ctx, maharera.Input{State: q.State, RegistrationNumber: in.ProjectRegistrationNumber})
		if err != nil {
			if ctx.Err() != nil {
				return PackageResult{}, ctx.Err()
			}
			p.SourceErrors = append(p.SourceErrors, err.Error())
		} else {
			p.RegistryProject = &project.Result
			if len(project.Projects) > 0 {
				p.Status = "partial"
			}
			p.Warnings = append(p.Warnings, "Registry data describes the supplied project registration; unit identity, district/locality and property type are not automatically reconciled with the query.")
		}
	}
	for _, e := range records {
		if e.DataMode == "synthetic_fixture" {
			p.Warnings = append(p.Warnings, "Contains synthetic fixtures; must not be used for valuation or accuracy claims.")
			break
		}
	}
	if in.Reconcile {
		result, err := s.reconcileRecords(ctx, records)
		if err != nil {
			p.SourceErrors = append(p.SourceErrors, err.Error())
			p.MissingCapabilities = append(p.MissingCapabilities, "reconciliation")
		} else {
			p.Reconciliation = &result
		}
	}
	if in.Comparables != nil {
		cq, err := s.canonicalQuery(in.Comparables.Subject.Query)
		if err != nil || cq.State != q.State || cq.PropertyType != q.PropertyType || coverage.Normalize(cq.District) != coverage.Normalize(q.District) || coverage.Normalize(cq.Locality) != coverage.Normalize(q.Locality) {
			return PackageResult{}, fmt.Errorf("comparable subject must match package state/property type/district/locality")
		}
		if q.AsOf != "" && in.Comparables.AsOf != q.AsOf {
			return PackageResult{}, fmt.Errorf("comparable as_of must match historical package")
		}
		if in.Comparables.IncludeFixtures && !in.IncludeFixtures {
			return PackageResult{}, fmt.Errorf("fixture comparables require package include_fixtures")
		}
		result, err := s.Comparables(ctx, *in.Comparables)
		if err != nil {
			p.SourceErrors = append(p.SourceErrors, err.Error())
			p.MissingCapabilities = append(p.MissingCapabilities, "comparable_selection")
		} else {
			p.ComparableSelection = &result
		}
	}
	id, err := s.Store.PutPackage(p)
	if err != nil {
		return PackageResult{}, err
	}
	return PackageResult{ID: id, GeneratedAt: time.Now().UTC().Format(time.RFC3339), Package: p}, nil
}

func (s Service) Geocode(ctx context.Context, in geography.GeocodeInput) (geography.GeocodeResult, error) {
	if s.Geography == nil {
		return geography.GeocodeResult{}, fmt.Errorf("live geographic sources disabled")
	}
	return s.Geography.Geocode(ctx, in)
}
func (s Service) LocationContext(ctx context.Context, in geography.ContextInput) (geography.ContextResult, error) {
	if s.Geography == nil {
		return geography.ContextResult{}, fmt.Errorf("live geographic sources disabled")
	}
	return s.Geography.Context(ctx, in)
}

func (s Service) ProjectRegistry(ctx context.Context, in maharera.Input) (maharera.Output, error) {
	if s.Registry == nil {
		return maharera.Output{}, fmt.Errorf("live project registry source disabled")
	}
	return s.Registry.Search(ctx, in)
}
