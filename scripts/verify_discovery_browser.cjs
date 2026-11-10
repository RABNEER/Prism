// Bounded live acceptance: one source search, at most one source index, one listing.
const { chromium } = require('../.cache/browser-check/node_modules/playwright-core');
const fs = require('node:fs');
const path = require('node:path');
(async () => {
 const browser = await chromium.launch({channel:'chrome',headless:true});
 const page = await browser.newPage({viewport:{width:1440,height:1100}});
 page.setDefaultTimeout(120000);
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 const output=path.resolve(__dirname,'../.cache/discovery-browser');fs.mkdirSync(output,{recursive:true});
 const replay=process.argv.includes('--replay-saved');
 if(replay){
  const data=path.resolve(__dirname,'../data/pune-demo');
  const records=fs.readdirSync(path.join(data,'evidence')).filter(n=>n.endsWith('.json')).map(n=>JSON.parse(fs.readFileSync(path.join(data,'evidence',n),'utf8')));
  const record=records.find(r=>r.data_mode==='imported'&&r.source.access_reference.includes('discovery pkg_'));
  if(!record)throw new Error('No real saved discovery record to replay');
  const id=record.source.access_reference.match(/discovery (pkg_[a-f0-9]+)/)[1];
  const discovery=JSON.parse(fs.readFileSync(path.join(data,'packages',id+'.json'),'utf8'));
  const candidate=discovery.candidates.find(c=>c.id===record.provider_record_id);if(!candidate)throw new Error('Saved record/candidate mismatch');
  discovery.candidates=[candidate,...discovery.candidates.filter(c=>c.id!==candidate.id)];
  await page.route('**/api/prism',async route=>{
   const action=route.request().postDataJSON()?.action;
   if(action==='discover')return route.fulfill({json:{ok:true,data:{discovery_id:id,status:'candidates_found',discovery}}});
   if(action==='ingest_discovered')return route.fulfill({json:{ok:true,data:{record}}});
   return route.continue();
  });
 }
 try {
  await page.goto('http://127.0.0.1:3000');
  await page.getByText('MCP connected · 14 tools').waitFor();
  await page.getByRole('button',{name:'Discover listing sources'}).click();
  await page.locator('.discovery-candidate').first().waitFor();
  const candidates=await page.locator('.discovery-candidate').count();
  const responsePromise=page.waitForResponse(async response=>response.url().endsWith('/api/prism') && response.request().postDataJSON()?.action==='ingest_discovered');
  await page.getByRole('button',{name:'Fetch and validate listing'}).first().click();
  const response=await responsePromise;const result=await response.json();if(!result.ok)throw new Error(result.error);
  if(result.data.record.data_mode!=='imported'||result.data.record.price?.type!=='asking_price')throw new Error('Expected actual asking-price source observation');
  await page.getByRole('button',{name:'Build evidence package'}).click();
  await page.getByText('Evidence package saved and read back through MCP.').waitFor();
  await page.getByRole('button',{name:'View source record'}).first().click();await page.locator('pre').waitFor();
  await page.screenshot({path:path.join(output,'desktop.png'),fullPage:true});
  await page.setViewportSize({width:390,height:844});await page.screenshot({path:path.join(output,'mobile.png'),fullPage:true});
  if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw new Error('Horizontal overflow');
  if(errors.length)throw new Error(errors.join('\n'));
  const report={verdict:'PASS',mode:replay?'UI REPLAY OF SAVED REAL DISCOVERY/INGESTION; NO NEW SITE REQUESTS':'REAL DISCOVERY AND ONE REAL LISTING',candidate_count:candidates,manual_domains_required:false,ingested_record_id:result.data.record.id,asking_price:result.data.record.price,source_url:result.data.record.source.url,source_inspection:true,mobile_no_overflow:true,page_errors:errors,valuation_generated:false};
  fs.writeFileSync(path.join(output,'report.json'),JSON.stringify(report,null,2));console.log(JSON.stringify(report,null,2));
 }finally{await browser.close()}
})().catch(error=>{console.error(error);process.exitCode=1});
