# Automatic source discovery

The MCP discovers sources itself. The manual hostname map remains only for the older direct-URL ingestion tool. It is not required for this flow.

1. Call `discover_property_sources` with `{"locality":"Baner","purpose":"sale","limit":5,"individual_pages":true}`.
2. Firecrawl searches the web without an AI model or paid key when using the local stack. If results contain recognized collection pages and `individual_pages` is true, at most one index page is fetched for links. Recognized individual-listing URL patterns are prioritized; output remains bounded by `limit`.
3. The result contains `discovery_id` and candidate IDs. Raw search and index-link responses have immutable snapshot IDs. These records form the internal source registry.
4. Call `ingest_discovered_listing` with `discovery_id` and a saved `candidate_id`. Arbitrary or forged IDs fail. The tool fetches that URL, parses supported source structures, checks locality/property/purpose and saves the observation and raw response.
5. Build an evidence package normally. Discovery hints are never inserted as prices. A page that cannot establish the necessary fields returns an explicit parsing/access error.

The dashboard exposes this as **Discover listing sources → Fetch and validate listing → Build evidence package**. It uses the selected locality and sale/rental pool. A retained direct-URL input still uses the manual domain map; it is optional.

## What discovery establishes

Candidate pages are not verified transactions, equivalent comparables or assertions that reuse is licensed. `access_status` is recorded as unreviewed. Collection/detail URL classification is heuristic and currently recognizes a small set of portal patterns. Search operators may be ignored upstream. Empty results can reflect provider failure or coverage; index expansion can fail independently. The source text is still validated after fetching, with no CAPTCHA or access-block bypass.

Only pilot localities Baner/Kharadi and sale/rent purposes are accepted. One search requests at most ten results and index expansion fetches at most one page; ingestion processes one saved candidate per call. HTTPS URLs with credentials, nonstandard ports, literal IPs or local hostnames are rejected. Index links must remain on the same host. Firecrawl's own fetch protections remain necessary; this is not a production multi-tenant network boundary.

## Supported extraction and unknowns

The Go parser supports a single Apartment with an associated Offer; a RealEstateListing can associate its Offer with an Apartment through `mainEntity` only when the listing URL matches the requested page. Explicit publisher BuyAction/RentAction can establish sale/rental purpose. Unrelated publisher/company addresses and other product offers are not borrowed.

A locality-only address stays locality-only and is flagged. An explicit Baner/Kharadi + Pune tuple may be normalized to Maharashtra through the pilot administrative mapping; the record explicitly labels that derivation. No street address, unit ID, area or publication date is invented. Missing area/date information can make an ingested listing ineligible for comparable selection.

## Verification

```powershell
python scripts/verify_discovery.py
python scripts/verify_discovery.py --live
node scripts/verify_discovery_browser.cjs
# For layout-only rechecks without more website requests:
node scripts/verify_discovery_browser.cjs --replay-saved
```

The default Python check uses a local mock through the actual MCP; the live check discovers real source URLs but does not ingest them. The live browser check is bounded to one discovery and one listing. The replay mode reuses saved real discovery/ingestion responses and must not be described as a new live-source check.

On 7 November 2026, real MCP discovery found five individual URLs from an index page. One listing was ingested with an actual publisher asking price and saved in an evidence package; street address, area and publication date were not established. A live dashboard discovery/ingestion also populated the real demo store. No valuation or accuracy claim follows from these checks.

Backend discovery report: `.cache/discovery-check-1794075098693937700/report.json`; actual ingestion report: `.cache/discovery-check-1794075098693937700/real-ingestion-report.json`. Current browser verification artifacts are in `.cache/discovery-browser`.
