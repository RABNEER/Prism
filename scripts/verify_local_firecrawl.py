"""Real local Firecrawl smoke check on IANA's example page, not property evidence."""
import json
import pathlib
import time
import http.client
import urllib.error
import urllib.request

def main():
    deadline=time.monotonic()+60
    while True:
        try:
            with urllib.request.urlopen('http://127.0.0.1:3002/health/readiness',timeout=2) as response:
                if response.status==200:break
        except (OSError,urllib.error.URLError,http.client.RemoteDisconnected):
            pass
        if time.monotonic()>=deadline:raise RuntimeError('Local Firecrawl API did not become ready; inspect docker compose logs')
        time.sleep(1)
    payload={"url":"https://example.org", "formats":["markdown","rawHtml"], "onlyMainContent":True, "timeout":30000}
    request=urllib.request.Request("http://127.0.0.1:3002/v2/scrape", data=json.dumps(payload).encode(), headers={"Content-Type":"application/json"})
    with urllib.request.urlopen(request,timeout=90) as response:
        result=json.load(response)
    assert result.get("success"),result
    assert "documentation examples" in result["data"]["markdown"],result
    assert "Example Domain" in result["data"].get("rawHtml",""),result
    report={"verdict":"PASS","endpoint":"http://127.0.0.1:3002/v2/scrape","real_scrape":True,"source":"https://example.org","markdown_verified":True,"html_verified":True,"model_requested":False,"property_data_collected":False,"structured_listing_extraction_verified":False}
    target=pathlib.Path(__file__).resolve().parents[1]/".cache/local-firecrawl-report.json"
    target.write_text(json.dumps(report,indent=2),encoding="utf-8")
    print(json.dumps(report,indent=2))

if __name__=="__main__":main()
