# Local Firecrawl for PRISM

## Start and stop

Start Docker Desktop, then run from the repository root:

```powershell
.\scripts\start-firecrawl.ps1
python scripts/verify_local_firecrawl.py
```

The first run downloads large container images. Only the API is published, at `http://127.0.0.1:3002`; Redis, RabbitMQ and PostgreSQL remain inside the Compose network. Concurrency is one to limit resource usage. The database here stores Firecrawl's jobs, not PRISM's property observations. PRISM still uses its immutable file store.

During this setup, the API and queue services started before the browser image finished downloading. A real basic-fetch scrape and an actual MCP rejection/archive check passed without a model. Reports: `.cache/local-firecrawl-report.json`, `.cache/local-mcp-firecrawl-check/report.json`. These do not verify a real property-listing layout or browser rendering. The pending host-side browser download can be completed with `scripts/complete-firecrawl-browser.ps1`; it waits for that download, imports the image, starts the browser service and records its health check in `.cache/firecrawl-browser-ready.json`. If that report is absent, browser health remains unverified.

Stop the stack without deleting saved volumes:

```powershell
docker compose -f infra/firecrawl/compose.yaml stop
```

`infra/firecrawl/compose.yaml` pins the three official Firecrawl images to immutable registry digests resolved during setup. The examined upstream source was tag `v2.11.702`, commit `bf6739d95caccfd6518d2bf1b5d07fad2b6bd34c`, saved under `.cache/firecrawl-src`. That tag had no published API image; the deployment instead uses the official published image digests. Do not assume those images represent that exact source tag. Redis and RabbitMQ use explicit version series. Future upgrades require compatibility checks.

## Connect the viewer

Set these server-side values in `web/.env.local` and restart the Next.js server:

```dotenv
PRISM_FIRECRAWL_URL=http://127.0.0.1:3002/v2/scrape
PRISM_FIRECRAWL_SELF_HOSTED=1
PRISM_FIRECRAWL_SOURCES=D:/Prism/examples/firecrawl-sources.json
```

Automatic source discovery and ingestion by saved candidate IDs do not require this manual source map; see [SOURCE_DISCOVERY.md](SOURCE_DISCOVERY.md). The map is retained for optional direct-URL ingestion. A running local scraper does not grant source reuse rights or provide a property database. MCP callers cannot choose the Firecrawl API endpoint; self-hosted mode requires a loopback endpoint. The dashboard's configured status describes configuration, not upstream health or extraction accuracy.

For a direct MCP client:

```powershell
.\bin\prism-mcp.exe -data-dir data/pune-demo -firecrawl-url http://127.0.0.1:3002/v2/scrape -firecrawl-self-hosted -firecrawl-sources examples/firecrawl-sources.json
```

## Model-free extraction

PRISM requests only `markdown` and `rawHtml` from Firecrawl. Raw HTML preserves JSON-LD scripts that main-content HTML can omit. Go performs deterministic extraction; no Firecrawl AI extraction format or prompt is requested. Supported contracts are deliberately narrow:

- One schema.org `Apartment` with a structured address and an associated `Offer` (including URL-scoped RealEstateListing/mainEntity relationships), explicit `Sell`/`LeaseOut` business function and INR currency. Monthly rent also requires `priceSpecification.unitText` to declare month/monthly. `floorSize` supports square feet/metres; its basis stays unknown unless its name supplies an unambiguous basis.
- Markdown with exactly one main heading and explicit labels: `Property type`, `Transaction type`, `Address`, `Locality`, `District`, `State`, and optional `Price`/`Rent`, one area measurement, `Published date`, `Project`, `Unit id`. Two-column Markdown tables are supported. See parser tests for examples.

The source must establish Maharashtra/Pune and Baner or Kharadi. Structured search/collection pages, multiple Apartment entities, repeated labels, unrelated offers, malformed JSON-LD and conflicting area fields are rejected. Unsupported layouts require a source-specific parser, not guessed values. JSON-LD is a publisher assertion, not independent verification. Missing/ambiguous prices, area bases and dates stay absent/unknown and may exclude the record from comparable selection. The full response is archived before parsing, including on unsupported-layout failures.

No local model, cloud model or paid API key is configured by this setup. Ollama is not required for Markdown scraping. The pre-existing host Ollama installation is left intact.

The public smoke check fetches `https://example.org`, verifies Markdown and raw HTML, and records `.cache/local-firecrawl-report.json`. It does not collect property data. `python scripts/verify_firecrawl_mcp.py` separately verifies both Go parser contracts through the actual MCP against a local mock provider, including asking/rent separation and source snapshot readback. Mock success does not establish compatibility with an arbitrary listing portal.

Upstream: [self-hosting documentation](https://github.com/firecrawl/firecrawl/blob/main/SELF_HOST.md). Firecrawl is distributed under AGPL-3.0; preserve its upstream license. This is a local educational deployment, not a production service.
