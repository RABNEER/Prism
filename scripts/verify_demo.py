"""Exercise the running local Next.js API through its real MCP bridge."""
import json
import pathlib
import urllib.error
import urllib.request

BASE = "http://127.0.0.1:3000"


def call(value, status=200, origin=BASE):
    request = urllib.request.Request(BASE + "/api/prism", data=json.dumps(value).encode(), headers={"Content-Type": "application/json", "Origin": origin})
    try:
        response = urllib.request.urlopen(request, timeout=230)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        result = json.load(response)
        assert response.status == status, (response.status, result)
        return result


def main():
    bootstrap = call({"action": "bootstrap"})["data"]
    assert bootstrap["tools"] == 14
    example = call({"action": "example"})["data"]
    package = example["result"]["package"]
    assert example["stored"] and example["example_mode"]
    assert len(package["records"]) == 5
    assert all(record["data_mode"] == "synthetic_fixture" for record in package["records"])
    assert len(package["reconciliation"]["duplicate_groups"]) == 1
    assert len(package["comparable_selection"]["candidates"]) == 2
    record_id = package["records"][0]["id"]
    assert call({"action": "evidence", "example_mode": True, "id": record_id})["data"]["record"]["id"] == record_id
    call({"action": "evidence", "id": record_id}, 422)
    real = call({"action": "investigate", "locality": "Baner"})["data"]
    assert real["stored"] and not real["example_mode"]
    assert all(record["data_mode"] != "synthetic_fixture" for record in real["result"]["package"]["records"])
    call({"action": "investigate", "locality": "Mumbai"}, 422)
    call({"action": "investigate", "area": 0}, 422)
    call({"action": "unknown"}, 400)
    call(None, 400)
    call({"action": "bootstrap"}, 403, "https://unrelated.example")
    report = {"verdict": "PASS", "tools": bootstrap["tools"], "synthetic_records": 5, "duplicate_groups": 1, "comparable_candidates": 2, "real_records": len(real["result"]["package"]["records"]), "example_store_isolated": True, "snapshot_readback": True, "invalid_requests_rejected": True, "live_firecrawl_verified": False}
    target = pathlib.Path(__file__).resolve().parents[1] / ".cache/demo-check-report.json"
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(report, indent=2), encoding="utf-8")
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()
