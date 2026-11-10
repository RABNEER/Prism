# PRISM Property Intelligence MCP

Property evidence infrastructure for PRISM. The current hackathon demo covers residential apartments in **Baner and Kharadi, Pune**, with a Next.js evidence desk backed by the actual Go MCP and Rust matching worker. National jurisdiction routing remains available for future expansion; it does not imply nationwide source coverage.

Start with [the focused demo guide](docs/PUNE_DEMO.md) for setup, the presentation sequence, Firecrawl configuration and data limits.

Hero Housing Fintech Challenge, Problem Statement 1: [solution deck (PDF)](submission/PRISM%20-%20Hero%20Housing%20Solution%20Deck.pdf) · [editable PowerPoint](submission/PRISM%20-%20Hero%20Housing%20Solution%20Deck.pptx).

## Current implementation

- Official Go MCP SDK v1.8.0, local stdio transport and typed structured results.
- All 28 states and 8 union territories in the jurisdiction registry, with supported aliases.
- Residential/commercial evidence classifications: apartment, house, plot, office and shop.
- Controlled JSON imports with source references, price/area semantics and fixture labeling.
- Exact state/district/locality filtering and declared publication/event-date cutoffs.
- Immutable content-addressed evidence records and reproducible package snapshots in a local development file store.
- Live named-place coordinates from GeoNames through Open-Meteo, filtered to India and an explicit state.
- Live Maharashtra project metadata from MahaRERA's public exact-registration search.
- An OSM/Overpass nearby-amenity adapter with bounded queries, caching and approximate distances; the default public endpoint returned HTTP 406 in live verification.
- Python-backed bounded Overture public-place extracts, with dataset releases, source snapshots and real Pune amenity verification.
- Rust Unicode-preserving record matching, conservative repeated-observation grouping and explicit source conflicts.
- Rule-based comparable selection with separate sale, asking-price and rental pools; conflicting versions are excluded.
- Automatic Firecrawl source discovery, an immutable candidate registry, bounded index-link discovery and ingestion by saved candidate IDs. Discovered URLs do not require a manually curated domain list. Search snippets never become price labels.
- Model-free listing extraction from Apartment JSON-LD, URL-scoped RealEstateListing/Apartment structures and explicitly labeled pages. The Baner pilot ingested 10 asking-price observations from one portal. None qualified for the example comparison because area and publication date were missing in the extracted records. Raw source responses are stored locally.
- A Next.js/React evidence viewer with separate synthetic storage, source inspection and JSON package download.
- Archived source responses, attribution, timeouts, cancellation handling and explicit source failures.
- No AI API key or model call is required to run these tools.

**Coverage is source-specific.** Live place search is national; project-registry lookup currently covers Maharashtra only. The discovery/ingestion pilot covers Baner and Kharadi apartments. Other state registries, completed-sale datasets and satellite-image features are not integrated. Discovery records source access/reuse status as unreviewed, not a license assertion. `resolve_property` still performs conservative normalization; named-place centroids do not establish physical property identity.

PostGIS, Python imagery processing, further state/source adapters and authenticated remote MCP transport are subsequent milestones. There is no trained valuation model or calibrated price interval in this demo.

## Build and test (PowerShell)

Requires Go 1.26.3 or newer and Rust/Cargo. Build the matching worker and MCP together; the script uses workspace-local caches:

```powershell
.\scripts\build.ps1
python scripts/verify_pipeline.py
```

On other operating systems, omit `.exe`. The test suite includes a real stdio client/server handshake, tool discovery, invocation, error handling and snapshot readback.

## Run and connect

```powershell
.\bin\prism-mcp.exe -data-dir D:\Prism\data
```

The process waits for an MCP client on stdin; it is not an interactive CLI. Configure any compatible MCP client to launch the binary. Example configuration shape (adapt to the client's configuration format):

```json
{
  "mcpServers": {
    "prism": {
      "command": "D:\\Prism\\bin\\prism-mcp.exe",
      "args": ["-data-dir", "D:\\Prism\\data"]
    }
  }
}
```

Logs go to stderr. Stdout is reserved for MCP protocol messages. The absolute data directory ensures the importer and MCP process use the same records regardless of their working directories.

Live sources are enabled for explicit on-demand tool calls. Starting the server and checking coverage do not make network requests. Use `-live-sources=false` for offline operation. Operator flags `-geocoding-url`, `-overpass-url`, `-maharera-url` and `-source-user-agent` control endpoints and identification; tool arguments cannot supply arbitrary URLs.

## Tools

| Tool | Purpose |
| --- | --- |
| `get_market_coverage` | National summary or one state; import/fixture counts and configured connectors are separate; does not probe provider health |
| `resolve_property` | Explicit jurisdiction resolution and conservative address normalization |
| `search_property_evidence` | Paginated local evidence search; no live fetching |
| `build_evidence_package` | Persist matching evidence and capability gaps; no valuation |
| `get_evidence` | Read an evidence record or saved package by ID |
| `geocode_location` | Named-place candidates through Open-Meteo/GeoNames, filtered by country/state |
| `get_location_context` | Current mapped education, healthcare and transit via bounded Overture extracts or OSM/Overpass |
| `search_registered_projects` | Exact project-registration lookup; Maharashtra currently implemented |
| `reconcile_evidence` | Rust record comparisons, duplicate groups and preserved identity/source conflicts |
| `find_comparables` | Rule-based candidates in one price pool with area/date/locality guards |
| `get_listing_ingestion_status` | Firecrawl readiness and permitted domains, without revealing credentials |
| `ingest_listing` | Archive and validate one permitted Baner/Kharadi apartment listing |
| `discover_property_sources` | Discover public source URLs and optionally inspect links from one index; save raw provenance and candidates |
| `ingest_discovered_listing` | Validate and ingest a candidate using saved discovery/candidate IDs, without a manual domain list |

Start with `get_market_coverage` using `{}`. Example search arguments:

```json
{
  "query": {
    "state": "Maharashtra",
    "property_type": "apartment",
    "district": "Pune",
    "as_of": "2026-11-07"
  },
  "limit": 25,
  "include_fixtures": false
}
```

State is required for evidence search. District/locality are optional exact filters after case/whitespace normalization. Local evidence search still searches imports; live registry and geography tools return separately attributed source results. Historical filters exclude unknown publication dates and later event/publication dates; they depend on importer-supplied dates. Imports are not a substitute for verifying historical source availability.

## Live-source workflow

1. `geocode_location`: `{"place":"Pune","state":"Maharashtra"}`. Select a candidate; it is a place centroid rather than the apartment location.
2. `search_registered_projects`: `{"state":"Maharashtra","registration_number":"P52100012200"}`.
3. `get_location_context`: `{"latitude":18.5204,"longitude":73.8567,"radius_m":500}`. This call can fail if the provider is unavailable or rejects the request.
4. `build_evidence_package` can attach an optional `project_registration_number` and/or `location` object alongside `query`. Source errors appear in `source_errors`; successful sources remain in a partial package. A historical `query.as_of` cannot be combined with either live source.
5. `get_evidence` reads package IDs and raw source snapshot IDs. Raw HTML/JSON is archived before interpretation and hashes are checked on read. Runtime cache-hit metadata is excluded from package identities.

Live results are not automatically inserted into the imported-record pool: project metadata does not establish property type, unit identity or sale value. Packages explicitly preserve that separation. Coordinates and project IDs are supplied by the caller, and their association to the subject remains unverified. Current map data is never silently substituted for historical features.

Source conditions, limits, live-test instructions and current verification results are in [LIVE_SOURCES.md](docs/LIVE_SOURCES.md).

## Import evidence

```powershell
.\bin\prism-admin.exe -import examples/synthetic-evidence.json -data-dir D:\Prism\data
```

The example contains three **synthetic** observations across Maharashtra, Karnataka and Rajasthan, including a rural query shape. They are excluded from search/packages by default and never count toward real coverage. Set `include_fixtures: true` only to exercise development flows. They contain no real property evidence or accuracy results.

For actual imports, supply authorized observations in the same shape with `data_mode: "imported"`, real source URLs, provider record IDs, permission references, and retrieval timestamps. Do not relabel the demo fixtures as real data. The importer validates the whole batch before writing; an I/O failure can still leave a partially imported batch. Re-running is idempotent for identical normalized records.

Limits: 4 MiB and 1,000 observations per import; 10,000 store entries; 100 observations per search response; 1,000 observations per package. Narrow queries when package limits are exceeded. Pagination assumes the imports do not change between pages. Local files are development storage, not a multi-tenant database. Keep data directories private; remote access controls come with the HTTP milestone.

Every observation preserves its own source attribution. Conflicting records remain separate; Rust reconciliation groups only repeated identical source observations and flags ambiguous relationships. Original source documents are not archived by the JSON importer; live connectors archive their responses separately. Snapshot IDs hash the canonical stored JSON, and reads reject altered records/packages; they prove snapshot identity, not source authenticity. Nonempty sale-transaction prices must be registered consideration; asking prices and rents cannot enter that category.

## Plan and next milestone

See [the full MCP build plan](docs/MCP_BUILD_PLAN.md) and [automatic discovery guide](docs/SOURCE_DISCOVERY.md). Next: review a larger Pune sample, improve area/date coverage and validate comparable selection before city expansion. PostgreSQL/PostGIS remains a future storage step. National source coverage remains a staged rollout.

Official SDK reference: https://go.sdk.modelcontextprotocol.io/
