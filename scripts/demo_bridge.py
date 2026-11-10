"""Local demo API bridge. Every investigation uses the actual stdio MCP tools."""
import json
import os
import pathlib
import subprocess
import sys
from mcp_client import MCP

sys.stdout.reconfigure(encoding="utf-8")
ROOT = pathlib.Path(__file__).resolve().parents[1]
DATA = pathlib.Path(os.getenv("PRISM_DEMO_DATA_DIR", str(ROOT / "data/pune-demo")))

def example_records():
    rows = []
    for number, area, price, source_id in [("101", 100, 12000000, "fixture-101"), ("102", 105, 12500000, "fixture-102"), ("103", 98, 11800000, "fixture-103")]:
        rows.append({"id":"","state":"Maharashtra","district":"Pune","locality":"Baner","property_type":"apartment","entity_level":"unit","address":f"Synthetic Example Building, unit {number}, Baner, Pune","project":"Synthetic Example Building","unit_id":number,"provider_record_id":source_id,"source":{"id":"prism-demo-fixtures","name":"Artificial demo examples","url":"https://example.org/prism-demo","access_reference":"Synthetic observations created for testing; no real property or price claim"},"category":"listing","data_mode":"synthetic_fixture","published_date":"2026-10-01","retrieved_at":"2026-11-01T00:00:00Z","area":{"value":area,"unit":"sq_m","basis":"carpet"},"price":{"amount":price,"currency":"INR","type":"asking_price"},"warnings":["SYNTHETIC example; not a real listing, sale or valuation"]})
    repeated=json.loads(json.dumps(rows[0]));repeated["retrieved_at"]="2026-11-02T00:00:00Z";rows.append(repeated)
    conflict=json.loads(json.dumps(rows[1]));conflict["area"]["value"]=125;conflict["retrieved_at"]="2026-11-03T00:00:00Z";rows.append(conflict)
    return rows

def main(request):
    action=request.get("action")
    if action not in {"bootstrap","investigate","ingest","example","evidence","registry_sample","discover","ingest_discovered"}:raise ValueError("unsupported action")
    example_mode=action=="example" or (action=="evidence" and request.get("example_mode") is True)
    data_dir=DATA/"synthetic-examples" if example_mode else DATA
    if action=="example":
        data_dir.mkdir(parents=True,exist_ok=True);fixture=data_dir/"demo-fixtures.json";fixture.write_text(json.dumps(example_records()),encoding="utf-8")
        subprocess.run([str(ROOT/"bin/prism-admin.exe"),"-import",str(fixture),"-data-dir",str(data_dir)],check=True,capture_output=True)
    extra=[]
    if os.getenv("PRISM_FIRECRAWL_URL"):extra += ["-firecrawl-url",os.environ["PRISM_FIRECRAWL_URL"]]
    if os.getenv("PRISM_FIRECRAWL_SELF_HOSTED")=="1":extra += ["-firecrawl-self-hosted"]
    if os.getenv("PRISM_FIRECRAWL_SOURCES"):extra += ["-firecrawl-sources",os.environ["PRISM_FIRECRAWL_SOURCES"]]
    if os.getenv("PRISM_FIRECRAWL_ANONYMOUS")=="1":extra += ["-firecrawl-anonymous"]
    client=MCP(ROOT/"bin/prism-mcp.exe",data_dir,extra,timeout=220)
    try:
        if action=="bootstrap":return {"tools":len(client.request("tools/list",{})["tools"]),"version":client.initialized["serverInfo"]["version"],"firecrawl":client.call("get_listing_ingestion_status",{}),"coverage":client.call("get_market_coverage",{"state":"MH","property_type":"apartment"})}
        if action=="evidence":return client.call("get_evidence",{"id":str(request.get("id",""))})
        if action=="registry_sample":return client.call("search_registered_projects",{"state":"MH","registration_number":"P52100012200"})
        locality=request.get("locality","Baner")
        if locality not in {"Baner","Kharadi"}:raise ValueError("pilot covers Baner and Kharadi")
        if action=="discover":return client.call("discover_property_sources",{"locality":locality,"purpose":request.get("purpose","sale"),"limit":5,"individual_pages":True})
        if action=="ingest_discovered":return client.call("ingest_discovered_listing",{"discovery_id":str(request.get("discovery_id","")),"candidate_id":str(request.get("candidate_id",""))})
        if action=="ingest":return client.call("ingest_listing",{"url":str(request.get("url","")),"locality":locality})
        query={"state":"Maharashtra","district":"Pune","locality":locality,"property_type":"apartment"}
        include=action=="example"
        area=float(request.get("area",100))
        if not 10<=area<=1000:raise ValueError("area must be 10–1000 square metres")
        basis=request.get("area_basis","carpet")
        if basis not in {"carpet","built_up","super_built_up"}:raise ValueError("unsupported area basis")
        subject={"query":query,"address":str(request.get("address","User-supplied apartment")),"project":str(request.get("project","")),"unit_id":str(request.get("unit_id","")),"area":{"value":area,"unit":"sq_m","basis":basis}}
        kind=request.get("price_type","asking_price")
        if kind not in {"asking_price","registered_consideration","monthly_rent"}:raise ValueError("unsupported price type")
        args={"query":query,"include_fixtures":include,"reconcile":True,"comparables":{"subject":subject,"price_type":kind,"as_of":request.get("as_of","2026-11-07"),"include_fixtures":include}}
        registration=str(request.get("registration","")).strip()
        if registration:args["project_registration_number"]=registration
        if request.get("with_location") is True:args["location"]={"latitude":float(request["latitude"]),"longitude":float(request["longitude"]),"radius_m":500,"provider":"overture"}
        result=client.call("build_evidence_package",args)
        return {"result":result,"example_mode":include,"subject":subject,"stored":client.call("get_evidence",{"id":result["id"]})["record"]==result["package"]}
    finally:client.close()

if __name__=="__main__":
    try:
        raw=sys.stdin.buffer.read(32769)
        if len(raw)>32768:raise ValueError("request too large")
        print(json.dumps({"ok":True,"data":main(json.loads(raw))},ensure_ascii=False))
    except Exception as error:
        print(json.dumps({"ok":False,"error":str(error)},ensure_ascii=False));sys.exit(1)
