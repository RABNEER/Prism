"""Use actual MCP tools and Rust worker; fixture prices stay explicitly synthetic."""
import argparse
import copy
import json
import pathlib
import subprocess
import time
from mcp_client import MCP


def main():
    args = argparse.ArgumentParser()
    args.add_argument("--live-amenities", action="store_true")
    options = args.parse_args()
    root = pathlib.Path(__file__).resolve().parents[1]
    work = root / ".cache" / f"pipeline-check-{time.time_ns()}"
    work.mkdir(parents=True)
    template = {"id": "", "state": "MH", "district": "Pune", "locality": "Synthetic Locality", "property_type": "apartment", "entity_level": "unit", "address": "Synthetic Project apartment 101", "project": "Synthetic Project", "unit_id": "101", "provider_record_id": "fixture-sale-1", "source": {"id": "synthetic-intelligence", "name": "Synthetic development fixture", "url": "https://example.org/fixture", "access_reference": "Artificial test data; not real transactions"}, "category": "transaction", "data_mode": "synthetic_fixture", "event_date": "2026-01-01", "published_date": "2026-01-02", "retrieved_at": "2026-02-01T00:00:00Z", "area": {"value": 100, "unit": "sq_m", "basis": "carpet"}, "price": {"amount": 10000000, "currency": "INR", "type": "registered_consideration"}, "warnings": ["SYNTHETIC: never use for lending or accuracy claims"]}
    records = [template]
    repeated = copy.deepcopy(template)
    repeated["retrieved_at"] = "2026-02-02T00:00:00Z"
    records.append(repeated)
    another = copy.deepcopy(template)
    another.update(unit_id="102", address="Synthetic Project apartment 102", provider_record_id="fixture-sale-2")
    records.append(another)
    asking = copy.deepcopy(another)
    asking.update(unit_id="103", address="Synthetic Project apartment 103", provider_record_id="fixture-listing-1", category="listing")
    asking["price"]["type"] = "asking_price"
    records.append(asking)
    fixture = work / "fixtures.json"
    fixture.write_text(json.dumps(records), encoding="utf-8")
    subprocess.run([str(root/"bin/prism-admin.exe"), "-import", str(fixture), "-data-dir", str(work/"data")], check=True, capture_output=True)
    client = MCP(root/"bin/prism-mcp.exe", work/"data", ["-live-sources=false"])
    report = {"data_mode": "synthetic fixtures for intelligence; public data for optional amenities", "checks": []}
    try:
        tools = client.request("tools/list", {})["tools"]
        assert len(tools) == 14
        query = {"state": "MH", "district": "Pune", "locality": "Synthetic Locality", "property_type": "apartment"}
        reconciliation = client.call("reconcile_evidence", {"query": query, "include_fixtures": True})
        assert len(reconciliation["duplicate_groups"]) == 1 and len(reconciliation["retained_ids"]) == 3
        assert any("explicit_unit_mismatch" in d["conflicts"] for d in reconciliation["decisions"])
        report["checks"].append({"reconciliation": {"worker": reconciliation["worker_version"], "duplicate_groups": 1, "retained": 3}})
        comp_args = {"subject": {"query": query, "address":"Synthetic Project apartment 999", "project": "Synthetic Project", "unit_id": "999", "area": {"value": 100, "unit": "sq_m", "basis": "carpet"}}, "price_type": "registered_consideration", "as_of": "2026-06-01", "include_fixtures": True}
        comps = client.call("find_comparables", comp_args)
        assert len(comps["candidates"]) == 2 and all(c["evidence"]["price"]["type"] == "registered_consideration" for c in comps["candidates"])
        report["checks"].append({"comparable_sale_candidates": len(comps["candidates"]), "asking_prices_excluded": True})
        inputs = {"query": query, "include_fixtures": True, "reconcile": True, "comparables": comp_args}
        package = client.call("build_evidence_package", inputs)
        assert not package["package"]["source_errors"]
        assert client.call("get_evidence", {"id": package["id"]})["record"] == package["package"]
        assert client.call("build_evidence_package", inputs)["id"] == package["id"]
        report["checks"].append({"intelligence_package": package["id"], "reproducible": True})
    finally:
        client.close()
    if options.live_amenities:
        live = MCP(root/"bin/prism-mcp.exe", root/".cache/public-amenity-data")
        try:
            context = live.call("get_location_context", {"latitude": 18.5204, "longitude": 73.8567, "radius_m": 500, "provider": "overture"})
            assert context["source"]["provider"] == "overture_places" and context["pois"]
            assert live.call("get_evidence", {"id": context["source"]["snapshot_id"]})["record"]["provider"] == "overture_places"
            report["checks"].append({"real_amenities": {"count": len(context["pois"]), "counts": context["counts"], "release": context["dataset_release"], "snapshot_id": context["source"]["snapshot_id"], "cache_hit": context["cache_hit"]}})
        finally:
            live.close()
    report["verdict"] = "PASS"
    (work/"report.json").write_text(json.dumps(report, indent=2), encoding="utf-8")
    print(json.dumps({"report": str(work/"report.json"), **report}, indent=2))


if __name__ == "__main__":
    main()
