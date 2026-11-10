# Live source integrations

## Status on 7 November 2026

| Adapter | Actual live observation | Scope |
| --- | --- | --- |
| GeoNames through Open-Meteo | Returned three Pune candidates and one Bengaluru candidate, restricted to the requested state | National named-place search; not rooftop geocoding |
| MahaRERA public project search | Returned NANDHA RESIDENCY, registration P52100012200, Pune district | Maharashtra exact-registration project lookup |
| OpenStreetMap through Overpass | Default public endpoint returned HTTP 406 to both POST and documented GET checks | Adapter implemented and mock-tested; live amenity retrieval unverified in this environment |
| Overture Maps public places | Actual MCP retrieval returned 179 mapped elements within 500 m of 18.5204, 73.8567; release 2026-10-21.0; source snapshot read back successfully | Bounded extract, source counts and approximate distances; mapping quality is not independently verified |
| Local Firecrawl discovery and listing extraction | Real search found individual Baner listing URLs; one publisher asking-price observation was ingested and read back, without a model or manually configured domains | Baner/Kharadi pilot; one real record in the demo store, not a reviewed valuation dataset. See [source discovery](SOURCE_DISCOVERY.md). |

One other published registration returned a project on one request and no-match on another. No-match therefore describes a source response, not a definitive finding that a registration does not exist. Successful responses cache for a day; no-match responses for five minutes.

Later focused-viewer verification also returned no-match for P52100012200, which had returned NANDHA RESIDENCY earlier. The viewer preserves this source variability. Its Baner amenity call returned 32 mapped elements (21 education, 11 healthcare) within 500 m of the approximate point 18.5590, 73.7868 and verified source snapshot readback. Reports: `.cache/live-demo-report.json` and `.cache/pipeline-check-1794069811177699400/report.json`.

These are real source results, separate from synthetic development fixtures. They do not validate a property valuation or establish nationwide transaction-record coverage.

## Usage conditions

Open-Meteo's free endpoint is for non-commercial use with provider quotas. This configuration is for a small educational prototype; a commercial service requires suitable licensed/self-hosted arrangements. Named-place data is attributed to GeoNames/Open-Meteo (CC BY 4.0). See [API documentation](https://open-meteo.com/en/docs/geocoding-api) and [terms](https://open-meteo.com/en/terms).

Overpass public instances are shared resources rather than a production lender backend. Use a suitable provisioned instance/extract for a deployed service. Preserve OpenStreetMap attribution and ODbL obligations. See [provider usage guidance](https://dev.overpass-api.de/overpass-doc/en/preface/commons.html).

MahaRERA's [copyright policy](https://www.maharera.maharashtra.gov.in/copyright-policy) permits accurate, attributed reproduction of its material, excluding separately copyrighted third-party material. The adapter submits the public search form once per exact lookup. It performs no bulk crawling, authenticated document access or CAPTCHA bypass. See also [terms](https://www.maharera.maharashtra.gov.in/terms-conditions). The site itself cautions against treating its content as a legal statement; this connector provides metadata, not title/legal clearance.

Nominatim's public service is not integrated. A generic address-search service must not silently rely on that endpoint; its [usage policy](https://operations.osmfoundation.org/policies/nominatim/) requires specific developer responsibility and imposes additional restrictions.

## Behavior and limits

- Network requests happen only on explicit live-tool or live-package calls. Coverage reports configured connectors, not provider health or uniform record availability.
- Use `-live-sources=false` for offline operation. Endpoint flags are operator configuration, unavailable to tool callers. Redirects are rejected or restricted to the configured registry origin.
- Geographic cache misses are serialized with at least 1.1 seconds between requests. Use one prototype live process; process-local throttling does not enforce distributed provider quotas. Registry form lookups are independently serialized with a delay between GET and POST.
- Source responses are archived as content-addressed package records before parsing. Results contain snapshot IDs, retrieval times and attribution. Cache hits preserve the original retrieval time.
- Geocodes cache for seven days, OSM results for a day and registry results for a day (five minutes for no-match). No stale response is silently substituted after failure. Failed/partial geographic payloads are archived but evicted from the reusable cache.
- Geographic responses are limited to 2 MiB; registry pages to 4 MiB. OSM radius is 100–2,000 m and output is capped at 500 elements, marked truncated when the cap is reached.
- HTTP errors are explicit, with no automatic retries. Geographic providers enter a cooldown on HTTP errors, honoring a later `Retry-After` deadline.
- POI counts describe mapped elements, not verified unique facilities. Distances are straight-line to nodes or polygon bounding-box centers, not road distance, travel time or distance to a boundary. Large facilities with centers outside the radius are excluded.
- Coordinates receive a coarse India bounds check, not property/state boundary verification. The OSM query filters returned elements to India's mapped country area.
- Historical queries cannot use current map/registry results. Declared source modification dates do not establish historical data availability.
- A supplied project ID or coordinate is not automatically matched to the package subject, district, locality or property type. Warnings preserve that unresolved association.

## Reproduce verification

Routine tests are offline: local HTTP mocks exercise caching, failure handling, temporal guards, source filters and archived data. MCP discovery exposes 12 tools. `python scripts/verify_pipeline.py` exercises the compiled MCP and Rust worker with labeled synthetic prices; `--live-amenities` additionally verifies a real Overture query and source snapshot. See the focused demo guide for viewer checks.

For a few explicit live requests in PowerShell:

```powershell
$env:GOMODCACHE = Join-Path $PWD '.cache\gomod'
$env:GOCACHE = Join-Path $PWD '.cache\go-build'
$env:PRISM_LIVE_DATA_DIR = Join-Path $PWD '.cache\live-source-check'
$env:PRISM_LIVE_REGISTRY_TEST = '1'
$env:PRISM_REGISTRATION_NUMBER = 'P52100012200'
go test -count=1 -v ./internal/adapters/maharera -run TestLiveMahaRERA
$env:PRISM_LIVE_TEST = '1'
go test -count=1 -v ./internal/adapters/geography -run TestLiveGeographicSources
```

The geographic live test currently fails at Overpass after successful named-place searches because of the observed HTTP 406. That is an external-source failure rather than a passing end-to-end amenity retrieval. Do not repeatedly rerun the live tests or use the public endpoints for a crawl.

Local source archives from development verification are under `.cache/live-geographic-data` and `.cache/live-registry-data`; these are ignored generated files rather than checked-in datasets. Fetching again can yield different upstream data.

## Actual MCP client check

After building the binary, `python scripts/verify_live_mcp.py` launches it through stdio and runs a small real-source workflow. It checks tool discovery (currently 12 tools), 36-jurisdiction routing, Pune and Bengaluru geocodes, the saved live MahaRERA project response, a combined partial package, persisted readback and the raw government response archive. It reports unavailable sources separately rather than marking them successful. The earlier eight-tool run is saved in `.cache/live-mcp-report.json`; newer focused checks are listed above.

The recorded MCP run used fresh live geocodes and a cached government result previously retrieved from the actual portal; cache-hit and retrieval timestamps are retained. This distinction matters when measuring turnaround.

## Independent verification and regression testing

Two independent agents verified source provenance and tested actual MCP behavior. Source verification matched the saved GeoNames candidates and six MahaRERA project fields against raw responses. Testing found and reproduced two defects, subsequently fixed:

- An incomplete project card could inherit fields from a neighboring card. Parsing is now confined to one recognized result card, with structural field extraction and one registration/name required. Registry cache keys include the parser version to invalidate earlier parsed-cache results.
- An Overpass payload without an elements array or valid dataset timestamp could appear to be available empty context. Incomplete envelopes now fail explicitly and are not reused from cache.

The full uncached Go suite passed after the fixes. An independent fresh-binary MCP recheck completed 23 schema-validated success/expected-error checks across all eight tools, with neither defect reproduced. Generated reports are under `.cache/verification-1794048741519513900` and `.cache/testing-1794049098427405100`.
