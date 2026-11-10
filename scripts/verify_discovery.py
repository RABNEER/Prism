"""Actual MCP discovery check. Default uses a mock; --live searches via local Firecrawl."""
import argparse
import http.server
import json
import pathlib
import threading
import time
from mcp_client import MCP
from verify_firecrawl_mcp import Mock

ROOT=pathlib.Path(__file__).resolve().parents[1]

class SearchMock(Mock):
    def do_POST(self):
        if self.path!='/v2/search':return super().do_POST()
        request=json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        assert request['sources']==['web'] and 'scrapeOptions' not in request
        body=json.dumps({'success':True,'data':{'web':[{'url':'https://listing.example/structured','title':'Mock single apartment'},{'url':'https://listing.example/structured#duplicate','title':'Duplicate'},{'url':'https://127.0.0.1/private'}]}}).encode()
        self.send_response(200);self.send_header('Content-Type','application/json');self.send_header('Content-Length',str(len(body)));self.end_headers();self.wfile.write(body)

def main():
    parser=argparse.ArgumentParser();parser.add_argument('--live',action='store_true');args=parser.parse_args()
    work=ROOT/'.cache'/f'discovery-check-{time.time_ns()}';work.mkdir(parents=True)
    server=None
    endpoint='http://127.0.0.1:3002/v2/scrape'
    if not args.live:
        server=http.server.ThreadingHTTPServer(('127.0.0.1',0),SearchMock)
        threading.Thread(target=server.serve_forever,daemon=True).start()
        endpoint=f'http://127.0.0.1:{server.server_port}/v2/scrape'
    client=MCP(ROOT/'bin/prism-mcp.exe',work/'data',['-firecrawl-url',endpoint,'-firecrawl-self-hosted'])
    try:
        assert len(client.request('tools/list',{})['tools'])==14
        assert not client.call('get_listing_ingestion_status',{})['permitted_domains']
        result=client.call('discover_property_sources',{'locality':'Baner','purpose':'sale','limit':5,'individual_pages':args.live})
        discovery=result['discovery']
        assert client.call('get_evidence',{'id':result['discovery_id']})['record']==discovery
        assert client.call('get_evidence',{'id':discovery['search_snapshot_id']})['record']['kind']=='firecrawl_search_response'
        assert discovery['candidates'],result
        ingestion_verified=False
        if not args.live:
            assert len(discovery['candidates'])==1
            record=client.call('ingest_discovered_listing',{'discovery_id':result['discovery_id'],'candidate_id':discovery['candidates'][0]['id']})['record']
            assert record['price']['amount']==12000000 and record['price']['type']=='asking_price'
            assert 'discovery '+result['discovery_id'] in record['source']['access_reference']
            ingestion_verified=True
        try:client.call('ingest_discovered_listing',{'discovery_id':result['discovery_id'],'candidate_id':'not-a-saved-candidate'})
        except RuntimeError:pass
        else:raise AssertionError('forged candidate accepted')
        report={'verdict':'PASS','source_mode':'REAL LOCAL FIRECRAWL SEARCH' if args.live else 'LOCAL MOCK ONLY','tools':14,'no_manual_domains':True,'discovery_verified':True,'discovery_id':result['discovery_id'],'candidate_count':len(discovery['candidates']),'candidates':discovery['candidates'],'search_source_readback':True,'mock_ingestion_verified':ingestion_verified,'real_listing_ingestion_verified':False,'search_snippets_used_as_prices':False}
        (work/'report.json').write_text(json.dumps(report,indent=2),encoding='utf-8')
        print(json.dumps({'report_path':str(work/'report.json'),**report},indent=2))
    finally:
        client.close()
        if server:server.shutdown();server.server_close()

if __name__=='__main__':main()
