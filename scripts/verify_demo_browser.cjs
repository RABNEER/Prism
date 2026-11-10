// Optional browser check: install playwright-core in .cache/browser-check first.
const { chromium } = require('../.cache/browser-check/node_modules/playwright-core');
const path = require('node:path');
const fs = require('node:fs');

(async () => {
  const browser = await chromium.launch({ channel: 'chrome', headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1100 }, reducedMotion: 'reduce' });
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  const output = path.resolve(__dirname, '../.cache/demo-browser');
  fs.mkdirSync(output, { recursive: true });
  try {
    await page.goto('http://127.0.0.1:3000');
    await page.getByText('MCP connected · 14 tools').waitFor();
    await page.screenshot({ path: path.join(output, 'desktop-empty.png'), fullPage: true });
    await page.getByRole('button', { name: 'Explore synthetic example' }).click();
    await page.getByText('Synthetic mechanism demo').waitFor();
    await page.getByText('Evidence package saved and read back through MCP.').waitFor();
    if (await page.getByRole('button', { name: 'View source record' }).count() !== 5) throw new Error('Expected five synthetic records');
    await page.screenshot({ path: path.join(output, 'desktop-evidence.png'), fullPage: true });
    await page.getByRole('tab', { name: 'Reconciliation', exact: true }).click();
    await page.getByRole('heading', { name: 'Keep the evidence distinct.' }).waitFor();
    await page.getByRole('tab', { name: 'Comparables', exact: true }).click();
    if (await page.locator('.comparable').count() !== 2) throw new Error('Expected two comparable candidates');
    const downloadPromise = page.waitForEvent('download');
    await page.getByRole('button', { name: 'Download JSON' }).click();
    const download = await downloadPromise;
    await download.saveAs(path.join(output, 'downloaded-package.json'));
    const packet = JSON.parse(fs.readFileSync(path.join(output, 'downloaded-package.json'), 'utf8'));
    if (packet.package.records.length !== 5) throw new Error('Invalid package download');
    await page.getByRole('tab', { name: 'Evidence', exact: true }).click();
    await page.getByRole('button', { name: 'View source record' }).first().click();
    await page.locator('pre').waitFor();
    await page.getByRole('button', { name: 'Build evidence package' }).click();
    await page.getByText('Evidence package saved and read back through MCP.').waitFor();
    await page.getByRole('tab', { name: 'Evidence', exact: true }).waitFor();
    if (await page.getByText('Synthetic mechanism demo').count()) throw new Error('Synthetic banner leaked into real mode');
    await page.setViewportSize({ width: 390, height: 844 });
    await page.screenshot({ path: path.join(output, 'mobile.png'), fullPage: true });
    if (await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)) throw new Error('Mobile horizontal overflow');
    if (errors.length) throw new Error(errors.join('\n'));
    const report = { verdict: 'PASS', browser: 'Installed Chrome, headless', synthetic_records: 5, comparable_candidates: 2, source_inspection: true, json_download: true, real_mode_isolation: true, mobile_no_overflow: true, page_errors: errors };
    fs.writeFileSync(path.join(output, 'report.json'), JSON.stringify(report, null, 2));
    console.log(JSON.stringify(report, null, 2));
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
