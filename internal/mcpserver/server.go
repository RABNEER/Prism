package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"prism/internal/adapters/firecrawl"
	"prism/internal/adapters/geography"
	"prism/internal/adapters/maharera"
	"prism/internal/services"
)

func New(service services.Service) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "prism-property-intelligence", Version: "0.3.0"}, nil)
	mcp.AddTool(s, &mcp.Tool{Name: "discover_property_sources", Description: "Search the web through Firecrawl for Baner/Kharadi apartment sources. Saves candidate URLs and raw provenance internally; no manual domain list required. With individual_pages true, fetches links from at most one source index to find recognized individual-listing URL patterns. Snippets and discovered links are unverified, not price evidence or usage-rights assertions."}, func(ctx context.Context, _ *mcp.CallToolRequest, in firecrawl.DiscoveryInput) (*mcp.CallToolResult, firecrawl.DiscoveryResult, error) {
		if service.Firecrawl == nil {
			return nil, firecrawl.DiscoveryResult{}, fmt.Errorf("Firecrawl disabled")
		}
		out, err := service.Firecrawl.Discover(ctx, in)
		return nil, out, err
	})
	mcp.AddTool(s, &mcp.Tool{Name: "ingest_discovered_listing", Description: "Scrape one candidate from a saved source discovery by its IDs, without manually adding a domain. Source usage remains unreviewed; caller must respect applicable source access restrictions. Known collection pages are rejected. Source text must establish the pilot locality, apartment and sale/rent purpose. May return unsupported layout; does not invent missing fields."}, func(ctx context.Context, _ *mcp.CallToolRequest, in firecrawl.DiscoveredInput) (*mcp.CallToolResult, firecrawl.Result, error) {
		if service.Firecrawl == nil {
			return nil, firecrawl.Result{}, fmt.Errorf("Firecrawl disabled")
		}
		out, err := service.Firecrawl.IngestDiscovered(ctx, in)
		return nil, out, err
	})
	mcp.AddTool(s, &mcp.Tool{Name: "get_market_coverage", Description: "Report India-wide jurisdiction routing, imported property evidence counts and configured geographic connectors. Does not check provider health or imply live property records."}, func(ctx context.Context, _ *mcp.CallToolRequest, in services.CoverageInput) (*mcp.CallToolResult, services.CoverageResult, error) {
		out, err := service.Coverage(ctx, in)
		return nil, out, err
	})
	mcp.AddTool(s, &mcp.Tool{Name: "resolve_property", Description: "Resolve an explicitly supplied Indian state/UT and normalize address whitespace. Does not geocode or verify physical property identity."}, func(ctx context.Context, _ *mcp.CallToolRequest, in services.ResolveInput) (*mcp.CallToolResult, services.Resolved, error) {
		out, err := service.Resolve(in)
		return nil, out, err
	})
	mcp.AddTool(s, &mcp.Tool{Name: "search_property_evidence", Description: "Search local imported source observations by state, district, locality and property type. Fixtures excluded by default. No live source search or comparable selection."}, func(ctx context.Context, _ *mcp.CallToolRequest, in services.SearchInput) (*mcp.CallToolResult, services.SearchResult, error) {
		out, err := service.Search(ctx, in)
		return nil, out, err
	})
	mcp.AddTool(s, &mcp.Tool{Name: "build_evidence_package", Description: "Snapshot matching imported evidence and explicit capability gaps into an immutable package. Does not estimate value or assert property identity."}, func(ctx context.Context, _ *mcp.CallToolRequest, in services.BuildInput) (*mcp.CallToolResult, services.PackageResult, error) {
		out, err := service.Build(ctx, in)
		return nil, out, err
	})
	type getInput struct {
		ID string `json:"id" jsonschema:"Evidence ev_... or package pkg_... ID"`
	}
	type getOutput struct {
		Record map[string]any `json:"record"`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "get_evidence", Description: "Read a stored evidence observation or package by ID."}, func(ctx context.Context, _ *mcp.CallToolRequest, in getInput) (*mcp.CallToolResult, getOutput, error) {
		b, err := service.Store.Get(in.ID)
		if err != nil {
			return nil, getOutput{}, err
		}
		var out getOutput
		err = json.Unmarshal(b, &out.Record)
		return nil, out, err
	})
	mcp.AddTool(s, &mcp.Tool{Name: "geocode_location", Description: "Retrieve current named-place coordinates from Open-Meteo/GeoNames, filtered to an Indian state/UT. Not rooftop/property geocoding; preserves candidate ambiguity."}, func(ctx context.Context, _ *mcp.CallToolRequest, in geography.GeocodeInput) (*mcp.CallToolResult, geography.GeocodeResult, error) {
		out, err := service.Geocode(ctx, in)
		return nil, out, err
	})
	mcp.AddTool(s, &mcp.Tool{Name: "get_location_context", Description: "Retrieve mapped education, healthcare and transit nearby using a bounded public Overture extract (when configured) or OSM/Overpass. Includes source release/snapshot and approximate distances; no historical property valuation."}, func(ctx context.Context, _ *mcp.CallToolRequest, in geography.ContextInput) (*mcp.CallToolResult, geography.ContextResult, error) {
		out, err := service.LocationContext(ctx, in)
		return nil, out, err
	})
	mcp.AddTool(s, &mcp.Tool{Name: "search_registered_projects", Description: "Look up an exact registration number through an integrated state registry. Maharashtra public project metadata implemented; other states explicitly unsupported. No transaction price, legal/title or unit verification."}, func(ctx context.Context, _ *mcp.CallToolRequest, in maharera.Input) (*mcp.CallToolResult, maharera.Output, error) {
		out, err := service.ProjectRegistry(ctx, in)
		return nil, out, err
	})
	mcp.AddTool(s, &mcp.Tool{Name: "reconcile_evidence", Description: "Use the Rust worker to compare up to 200 imported records, preserve ambiguous unit/project matches and identify safe repeated-source observations. Requires configured worker; scores are uncalibrated heuristics."}, func(ctx context.Context, _ *mcp.CallToolRequest, in services.ReconcileInput) (*mcp.CallToolResult, services.Reconciliation, error) {
		out, err := service.Reconcile(ctx, in)
		return nil, out, err
	})
	mcp.AddTool(s, &mcp.Tool{Name: "find_comparables", Description: "Rank imported unit-level evidence in a single price-type pool using explicit locality, area-basis, size and date rules. Requires evidence; no fabricated sales or valuation."}, func(ctx context.Context, _ *mcp.CallToolRequest, in services.ComparablesInput) (*mcp.CallToolResult, services.Comparables, error) {
		out, err := service.Comparables(ctx, in)
		return nil, out, err
	})
	mcp.AddTool(s, &mcp.Tool{Name: "get_listing_ingestion_status", Description: "Report Firecrawl configuration and permitted domains for the Baner/Kharadi Pune pilot without exposing credentials or making network requests."}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, firecrawl.Status, error) {
		if service.Firecrawl == nil {
			return nil, firecrawl.Status{Authentication: "disabled", PermittedDomains: []string{}, PilotLocalities: []string{"Baner", "Kharadi"}}, nil
		}
		return nil, service.Firecrawl.Status(), nil
	})
	mcp.AddTool(s, &mcp.Tool{Name: "ingest_listing", Description: "Fetch HTML/Markdown for one permitted Baner/Kharadi apartment listing through Firecrawl and parse supported single-Apartment JSON-LD or labeled fields in Go, without model calls. Prices are asking-price or rent evidence only. Requires permitted sources and hosted authentication, explicit anonymous trial or loopback self-hosted mode; hosted calls may consume credits."}, func(ctx context.Context, _ *mcp.CallToolRequest, in firecrawl.Input) (*mcp.CallToolResult, firecrawl.Result, error) {
		if service.Firecrawl == nil {
			return nil, firecrawl.Result{}, fmt.Errorf("Firecrawl disabled")
		}
		out, err := service.Firecrawl.Scrape(ctx, in)
		return nil, out, err
	})
	return s
}
