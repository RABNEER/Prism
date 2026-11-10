"""A few on-demand live checks through the built stdio MCP server.

No crawl, no private property data, no unit-test mocks. Saves a compact report.
Run from the repository root after building bin/prism-mcp.exe.
"""
import argparse
import json
import pathlib
import queue
import subprocess
import threading
import time


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--server", default="bin/prism-mcp.exe")
    parser.add_argument("--data-dir", default=".cache/live-registry-data")
    parser.add_argument("--report", default=".cache/live-mcp-report.json")
    args = parser.parse_args()
    data = str(pathlib.Path(args.data_dir).resolve())
    process = subprocess.Popen(
        [str(pathlib.Path(args.server).resolve()), "-data-dir", data],
        stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        text=True, encoding="utf-8", bufsize=1,
    )
    messages = queue.Queue()
    threading.Thread(target=lambda: [messages.put(line) for line in process.stdout], daemon=True).start()
    counter = 0
    report = {"checked_at_unix": time.time(), "data_dir": data, "checks": [], "limitations": []}

    def request(method, params):
        nonlocal counter
        counter += 1
        process.stdin.write(json.dumps({"jsonrpc": "2.0", "id": counter, "method": method, "params": params}) + "\n")
        process.stdin.flush()
        deadline = time.monotonic() + 80
        while True:
            message = json.loads(messages.get(timeout=max(.1, deadline-time.monotonic())))
            if message.get("id") == counter:
                if "error" in message:
                    raise RuntimeError(message["error"])
                return message["result"]

    def tool(name, arguments):
        result = request("tools/call", {"name": name, "arguments": arguments})
        if result.get("isError"):
            raise RuntimeError("; ".join(c.get("text", "") for c in result.get("content", [])))
        return result["structuredContent"]

    def check(name, details):
        report["checks"].append({"check": name, "details": details})

    try:
        init = request("initialize", {"protocolVersion": "2025-11-25", "capabilities": {}, "clientInfo": {"name": "prism-live-verification", "version": "1.0"}})
        process.stdin.write(json.dumps({"jsonrpc": "2.0", "method": "notifications/initialized"}) + "\n")
        process.stdin.flush()
        tools = request("tools/list", {})["tools"]
        assert len(tools) == 14
        check("stdio initialization and discovery", {"server": init["serverInfo"], "tools": [t["name"] for t in tools]})
        coverage = tool("get_market_coverage", {})
        assert len(coverage["entries"]) == 36
        check("national routing", {"jurisdictions": len(coverage["entries"])})
        for place, state in [("Pune", "Maharashtra"), ("Bengaluru", "Karnataka")]:
            geo = tool("geocode_location", {"place": place, "state": state})
            assert geo["candidates"] and all(c["state"] == state for c in geo["candidates"])
            check("live named-place search", {"place": place, "candidates": len(geo["candidates"]), "source": geo["source"], "cache_hit": geo["cache_hit"]})
        project = tool("search_registered_projects", {"state": "MH", "registration_number": "P52100012200"})
        assert project["status"] == "found" and project["projects"][0]["registration_number"] == "P52100012200"
        check("live government project metadata", {"projects": project["projects"], "source_snapshot_ids": project["source_snapshot_ids"], "retrieved_at": project["retrieved_at"], "cache_hit": project["cache_hit"]})
        location = {"latitude": 18.5204, "longitude": 73.8567, "radius_m": 500}
        try:
            context = tool("get_location_context", location)
            check("live amenity context", {"status": context["status"], "mapped_elements": len(context["pois"]), "source": context["source"]})
        except RuntimeError as error:
            report["limitations"].append({"source": "osm_overpass", "error": str(error)})
        package = tool("build_evidence_package", {"query": {"state": "MH", "property_type": "apartment", "district": "Pune"}, "project_registration_number": "P52100012200", "location": location})
        assert package["package"]["registry_project"]["projects"]
        stored = tool("get_evidence", {"id": package["id"]})["record"]
        assert stored == package["package"]
        check("combined package and persisted readback", {"id": package["id"], "status": stored["status"], "source_errors": stored["source_errors"], "missing_capabilities": stored["missing_capabilities"]})
        archive = tool("get_evidence", {"id": project["source_snapshot_ids"][-1]})["record"]
        assert archive["kind"] == "registry_source_page" and "P52100012200" in archive["html"]
        check("raw government source archive", {"kind": archive["kind"], "html_bytes": len(archive["html"].encode("utf-8"))})
        report["verdict"] = "PASS for live named-place and government project retrieval; see limitations for unavailable sources"
    except Exception as error:
        report["verdict"] = "FAIL"
        report["error"] = repr(error)
        raise
    finally:
        output = pathlib.Path(args.report)
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, indent=2, ensure_ascii=False), encoding="utf-8")
        process.stdin.close()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)
        print(json.dumps(report, indent=2, ensure_ascii=False))


if __name__ == "__main__":
    main()
