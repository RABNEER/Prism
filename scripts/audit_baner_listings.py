"""Audit real Baner apartment evidence via the model-free local Firecrawl MCP."""
import argparse
from datetime import date
import json
from pathlib import Path
from time import time_ns

from mcp_client import MCP


ROOT = Path(__file__).resolve().parents[1]
QUERY = dict(state="Maharashtra", district="Pune", locality="Baner", property_type="apartment")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--saved-only", action="store_true", help="Audit existing evidence without network requests")
    parser.add_argument("--limit", type=int, choices=range(1, 11), default=10)
    args = parser.parse_args()
    run_dir = ROOT / ".cache" / f"baner-audit-{time_ns()}"
    run_dir.mkdir(parents=True)
    data_dir = ROOT / "data/pune-demo" if args.saved_only else run_dir / "data"
    client = MCP(ROOT / "bin/prism-mcp.exe", data_dir, [
        "-firecrawl-url", "http://127.0.0.1:3002/v2/scrape", "-firecrawl-self-hosted",
    ])
    report = {"mode": "saved_only" if args.saved_only else "live_local_firecrawl", "attempts": []}

    def save():
        (run_dir / "report.json").write_text(json.dumps(report, indent=2), encoding="utf-8")

    print(run_dir / "report.json", flush=True)
    try:
        if not args.saved_only:
            discovery = client.call("discover_property_sources", dict(
                locality="Baner", purpose="sale", limit=args.limit, individual_pages=True,
            ))
            report["discovery"] = discovery
            save()
            for candidate in discovery["discovery"]["candidates"]:
                attempt = {"url": candidate["url"], "domain": candidate["domain"]}
                if candidate["likely_collection"]:
                    attempt["status"] = "skipped_collection"
                else:
                    try:
                        record = client.call("ingest_discovered_listing", dict(
                            discovery_id=discovery["discovery_id"], candidate_id=candidate["id"],
                        ))["record"]
                        attempt.update(status="ingested", evidence_id=record["id"])
                    except RuntimeError as exc:
                        attempt.update(status="rejected", error=str(exc))
                report["attempts"].append(attempt)
                save()
                print(attempt["status"], attempt["domain"], flush=True)
        records = client.call("search_property_evidence", dict(query=QUERY, limit=100))["records"]
        report["records"] = []
        for record in records:
            if record["category"] != "listing":
                continue
            area = record.get("area") or {}
            price = record.get("price") or {}
            gaps = []
            if price.get("type") != "asking_price":
                gaps.append("asking_price_missing")
            if not area:
                gaps.append("area_missing")
            elif area.get("basis") not in {"carpet", "built_up", "super_built_up"}:
                gaps.append("area_basis_unknown")
            if not record.get("published_date"):
                gaps.append("publication_date_missing")
            snapshot = record.get("source_snapshot_id")
            if not snapshot or client.call("get_evidence", {"id": snapshot})["record"]["kind"] != "firecrawl_source_response":
                gaps.append("source_snapshot_missing")
            report["records"].append({
                "id": record["id"], "url": record["source"]["url"], "price": price,
                "area": area, "published_date": record.get("published_date"),
                "source_snapshot_id": snapshot, "gaps": gaps,
            })
        # A reproducible demo subject, not an actual property or a valuation.
        subject = {"query": QUERY, "address": "", "area": {"value": 1000, "unit": "sq_ft", "basis": "carpet"}}
        report["example_subject"] = subject
        report["comparables_as_of"] = date.today().isoformat()
        comps = client.call("find_comparables", dict(
            subject=subject, price_type="asking_price", as_of=report["comparables_as_of"],
        ))
        report["eligible_comparables"] = [row["evidence"]["id"] for row in comps["candidates"]]
        report["comparable_exclusions"] = comps["exclusions"]
        save()
        print(f"{len(report['records'])} listing records; {len(comps['candidates'])} eligible comparables.")
    except Exception as exc:
        report["error"] = str(exc)
        save()
        raise
    finally:
        client.close()


if __name__ == "__main__":
    main()
