"""Small explicit public-source check through the running viewer's MCP bridge."""
import json
import pathlib
from verify_demo import call


def main():
    registry = call({"action": "registry_sample"})["data"]
    assert registry["status"] in {"found", "no_match"}
    assert all(project["registration_number"] == "P52100012200" for project in registry["projects"])
    assert registry["source_snapshot_ids"]
    result = call({"action": "investigate", "locality": "Baner", "with_location": True, "latitude": 18.5590, "longitude": 73.7868})["data"]
    package = result["result"]["package"]
    assert result["stored"] and not result["example_mode"]
    assert not package["source_errors"], package["source_errors"]
    context = package["location_context"]
    assert context["source"]["provider"] == "overture_places" and context["pois"]
    assert call({"action": "evidence", "id": context["source"]["snapshot_id"]})["data"]["record"]["provider"] == "overture_places"
    report = {"verdict": "PASS", "data_mode": "actual public project and map sources; no sale-price claims", "registry": {"status": registry["status"], "projects": registry["projects"], "registration": "P52100012200", "retrieved_at": registry["retrieved_at"], "cache_hit": registry["cache_hit"], "limit": "A no-match source response does not establish invalid registration; the public portal has returned inconsistent results."}, "baner_approximate_location": {"mapped_elements": len(context["pois"]), "counts": context["counts"], "release": context["dataset_release"], "retrieved_at": context["source"]["retrieved_at"], "snapshot_id": context["source"]["snapshot_id"]}, "source_snapshot_readback": True, "live_firecrawl_verified": False}
    target = pathlib.Path(__file__).resolve().parents[1] / ".cache/live-demo-report.json"
    target.write_text(json.dumps(report, indent=2), encoding="utf-8")
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()
