"""Actual MCP ingestion against a local Firecrawl mock; never a live listing claim."""
import http.server
import json
import pathlib
import tempfile
import threading
from mcp_client import MCP

ROOT = pathlib.Path(__file__).resolve().parents[1]


class Mock(http.server.BaseHTTPRequestHandler):
    calls = 0

    def log_message(self, *_):
        pass

    def do_POST(self):
        Mock.calls += 1
        request = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        url = request["url"]
        price = "₹50,000/month" if url.endswith("rent") else "₹1.2 Cr"
        assert request['formats']==['markdown','rawHtml'],request
        purpose='rent' if url.endswith('rent') else 'sale'
        markdown=f"# Mock apartment\nProperty type: apartment\nTransaction type: {purpose}\nAddress: Mock apartment, Baner, Pune\nLocality: Baner\nDistrict: Pune\nState: Maharashtra\nPrice: {price}\nCarpet area: 100 sqm\nPublished date: 2026-10-01"
        document={"success":True,"data":{"markdown":markdown,"metadata":{"sourceURL":url,"statusCode":200}}}
        if url.endswith('structured'):
            node={"@type":"Apartment","address":{"streetAddress":"Test Building, Baner","addressLocality":"Pune","addressRegion":"Maharashtra","addressCountry":"IN"},"offers":{"@type":"Offer","businessFunction":"http://purl.org/goodrelations/v1#Sell","price":12000000,"priceCurrency":"INR"},"floorSize":{"value":100,"unitCode":"MTK","name":"Carpet area"},"datePublished":"2026-10-01"}
            document['data']['markdown']='# Structured apartment'
            document['data']['rawHtml']='<script type="application/ld+json">'+json.dumps(node)+'</script>'
        body = json.dumps(document).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def main():
    base = ROOT / ".cache"
    base.mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="firecrawl-mcp-", dir=base) as directory:
        work = pathlib.Path(directory)
        sources = work / "sources.json"
        sources.write_text(json.dumps({"listing.example": "Local mock fixture, not a real provider permission"}), encoding="utf-8")
        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Mock)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        client = MCP(ROOT / "bin/prism-mcp.exe", work / "data", ["-firecrawl-url", f"http://127.0.0.1:{server.server_port}", "-firecrawl-sources", str(sources), "-firecrawl-self-hosted"])
        try:
            assert client.call("get_listing_ingestion_status", {})["configured"]
            assert client.call("get_listing_ingestion_status", {})["authentication"] == "self_hosted"
            records = []
            for slug, kind, amount in [("sale", "asking_price", 12000000), ("rent", "monthly_rent", 50000), ("structured", "asking_price", 12000000)]:
                result = client.call("ingest_listing", {"url": f"https://listing.example/{slug}", "locality": "Baner"})
                record = result["record"]
                assert record["price"]["type"] == kind and record["price"]["amount"] == amount
                assert record["category"] == "listing" and record["area"]["basis"] == "carpet"
                assert client.call("get_evidence", {"id": result["source_snapshot_id"]})["record"]["kind"] == "firecrawl_source_response"
                records.append(record)
            found = client.call("search_property_evidence", {"query": {"state": "MH", "district": "Pune", "locality": "Baner", "property_type": "apartment"}})
            assert len(found["records"]) == 3
            for url, locality in [("https://unpermitted.example/sale", "Baner"), ("https://listing.example/sale", "Mumbai")]:
                try:
                    client.call("ingest_listing", {"url": url, "locality": locality})
                except RuntimeError:
                    pass
                else:
                    raise AssertionError("unsupported input accepted")
            assert Mock.calls == 3
            report = {"verdict": "PASS", "source_mode": "LOCAL MOCK ONLY", "actual_mcp": True, "no_model_requested": True, "parsers": ["labeled_markdown_v1","jsonld_apartment_v1"], "listing_records": len(records), "asking_and_rent_pools_separate": True, "raw_source_readback": True, "unsupported_inputs_blocked_before_upstream": True, "live_firecrawl_verified": False}
            (base / "firecrawl-mcp-report.json").write_text(json.dumps(report, indent=2), encoding="utf-8")
            print(json.dumps(report, indent=2))
        finally:
            client.close()
            server.shutdown()
            server.server_close()


if __name__ == "__main__":
    main()
