# Pune evidence desk

The current demo scope is residential apartments in Baner and Kharadi, Pune. Other-city expansion is deferred until this flow passes acceptance checks. The underlying jurisdiction-name registry remains reusable; it is not a claim of nationwide data coverage.

## Run

From the repository root, build the Go MCP and Rust worker:

```powershell
.\scripts\build.ps1
python -m pip install --target .cache/python -r python/requirements.txt
cd web
npm install
npm run dev -- --hostname 127.0.0.1
```

Visit http://127.0.0.1:3000. The Next.js server invokes a local Python bridge, which talks to the actual Go MCP server over stdio. Rust performs record matching. Real investigations share a persisted demo directory; synthetic examples use its separate `synthetic-examples` subdirectory. This viewer runs locally with the repository, Python and compiled binaries available; it is not a standalone serverless deployment.

## Demo sequence

1. Use **Explore synthetic example** to exercise the pipeline. All five artificial observations and prices are visibly labeled synthetic. They demonstrate a repeated source observation, distinct units and an area conflict.
2. Inspect Evidence, Reconciliation, Comparables and Audit trail. Conflicting source versions are excluded from comparable selection. Download the evidence package as JSON.
3. Start a real investigation for Baner or Kharadi. Synthetic records are excluded automatically. Empty real-data results remain empty.
4. Optionally retrieve public mapped amenities using Overture. Default coordinates are approximate locality points, not verified apartment positions. A first query can take a minute; subsequent queries use saved source snapshots.
5. The separate MahaRERA reference check demonstrates a real Pune-district registry lookup. It is not automatically attached to a Baner/Kharadi subject or presented as that subject's project. The portal has returned both a project and no-match for the reference registration; the viewer shows the actual response status, not a claim of registration validity.
6. Click **Discover listing sources**, then **Fetch and validate listing** on a candidate, and rerun the investigation. MCP finds and saves source URLs internally; no manual domain list is required for discovered candidates. Listings supply asking prices/rents, never completed-sale prices. See [SOURCE_DISCOVERY.md](SOURCE_DISCOVERY.md).

## Firecrawl setup

For the local Docker option, see [LOCAL_FIRECRAWL.md](LOCAL_FIRECRAWL.md). It uses explicit loopback self-hosted mode instead of hosted credentials.

For the optional direct-URL tool using hosted Firecrawl, set `FIRECRAWL_API_KEY` and set `PRISM_FIRECRAWL_SOURCES` to a JSON file mapping exact permitted hostnames to permission/source-use references:

```json
{
  "your-permitted-source.example": "Reference documenting your authority to retrieve and reuse this source"
}
```

`examples/firecrawl-sources.json` is deliberately empty. No listing portal is assumed to permit scraping. The source file is operator configuration, not a permission claim made by an AI tool caller. Credentials remain server-side; the viewer only displays readiness and domain names.

Firecrawl documents anonymous hosted trial requests, explicitly enabled with `PRISM_FIRECRAWL_ANONYMOUS=1`; the default is disabled. The current setup uses local self-hosted Firecrawl instead. Real discovery and one listing ingestion were verified without a model. Hosted/paid access was not tested.

Restart the viewer after changing environment configuration. Configure shell variables or `web/.env.local`; see `web/.env.example`. Next.js passes server-side environment configuration to the local MCP child.

Extraction archives the raw Firecrawl response (Markdown/raw HTML), requires a permitted origin and restricts ingestion to Pune apartments in the selected locality. Go parses supported single-Apartment JSON-LD or explicit labeled Markdown; no AI model is called. Monthly rent requires an explicit monthly period. Missing/ambiguous prices remain absent and unsupported area bases remain unknown. Publisher assertions, addresses and project identity still require review. Missing publication dates make listings ineligible for date-filtered comparable selection. See the local Firecrawl guide for supported parser contracts.

## Current data status

- Actual public MahaRERA source snapshots and Overture extracts exist from development verification.
- Demo price/identity examples are artificial and remain excluded from real investigations.
- No real completed-sale dataset is available in this workspace. There is no validated fair-market-value prediction or calibrated price interval in this viewer.
- Automatic discovery and one real listing ingestion have been verified through local Firecrawl without a model or manual domain list. A larger reviewed sample and broader source-layout coverage remain future work.
- Storage is currently local immutable files. PostgreSQL/PostGIS remains the proposed next storage step after the focused demo is stable.

## Tests

```powershell
.\scripts\build.ps1
python scripts/verify_pipeline.py
python scripts/verify_pipeline.py --live-amenities
python scripts/verify_firecrawl_mcp.py
cd web
npm run lint
npm run build
# With the viewer running in a second terminal, from the repository root:
python scripts/verify_demo.py
python scripts/verify_live_demo.py
```

The Go suite includes mocked Firecrawl extraction and error handling. Python checks use actual MCP tool calls and the compiled Rust worker; `verify_firecrawl_mcp.py` connects the real MCP to a local mock provider and does not verify live Firecrawl access. Live amenity checks use real public data, separate from synthetic price fixtures.

Optional browser acceptance check, using installed Chrome:

```powershell
npm install --prefix .cache/browser-check playwright-core --no-audit --no-fund
node scripts/verify_demo_browser.cjs
```

Reports and desktop/mobile screenshots are written under `.cache`.
