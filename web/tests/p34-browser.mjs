// Optional real-browser regression. Start the AI-off demo on loopback first.
// This is not a screen-reader, physical-mobile or live-model qualification.
import assert from 'node:assert/strict';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import en from '../src/i18n/messages.en.ts';
import vi from '../src/i18n/messages.vi.ts';

const base = process.env.QA_BASE_URL ?? 'http://127.0.0.1:8098';
const output = '../out/p34-browser';
mkdirSync(output, { recursive: true });
const browser = await chromium.launch({ headless: true,
  executablePath: process.env.BROWSER_PATH ?? 'C:/Program Files/BraveSoftware/Brave-Browser/Application/brave.exe' });
const results = [];
let activePage;
try {
  for (const locale of ['vi', 'en']) {
    const messages = locale === 'vi' ? vi : en;
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    const page = await context.newPage();
    activePage = page;
    const external = [];
    const modelCalls = [];
    page.on('request', request => {
      if (new URL(request.url()).origin !== new URL(base).origin) external.push(request.url());
      if (request.url().endsWith('/api/agent/session')) modelCalls.push(request.url());
    });
    await page.goto(base);
    await page.locator('#quick-verdict-title').waitFor();
    await page.getByRole('button', { name: 'Duyệt sâu', exact: true }).click();
    await page.getByRole('button', { name: /Điều kiện & phạm vi/ }).click();
    await page.getByRole('button', { name: 'Xem policy đã chuẩn hóa', exact: true }).click();
    await page.getByRole('checkbox', { name: /Tôi đã đọc và chấp nhận bộ điều kiện/ }).check();
    await page.getByRole('button', { name: 'Chấp nhận & kiểm chứng', exact: true }).click();
    await page.getByRole('button', { name: 'Chuẩn bị bản sửa', exact: true }).click();
    await page.locator('.rule-builder__row').first().getByRole('button', { name: 'Xóa dòng', exact: true }).click();
    const verifyResponse = () => page.waitForResponse(response => response.url() === `${base}/api/repair/verify` && response.request().method() === 'POST');
    const [firstResponse] = await Promise.all([verifyResponse(), page.getByRole('button', { name: 'Kiểm chứng bản sửa', exact: true }).click()]);
    assert.equal(firstResponse.ok(), true);
    const originalReceipt = await firstResponse.json();
    assert.equal(originalReceipt.status, 'VERIFIED');
    assert.match(originalReceipt.sha256, /^[a-f0-9]{64}$/);
    await page.locator('.receipt').waitFor();
    assert.equal(await page.getByRole('button', { name: 'Nhận để review', exact: true }).isEnabled(), true);
    await page.getByRole('button', { name: 'Nhận để review', exact: true }).click();
    assert.match(await page.locator('.accept-section').innerText(), /Đã nhận để review/);
    if (locale === 'en') await page.getByRole('button', { name: /EN —/ }).click();
    // Rebuild the same manual patch through actual UI controls. A verified
    // receipt replaces the verify CTA with evidence, so there is no direct
    // reverify button here. Returning to identical edits must not restore approval.
    await page.getByRole('button', { name: locale === 'vi' ? 'Bỏ chỉnh sửa' : 'Remove edit', exact: true }).click();
    await page.locator('.rule-builder__row').first().getByRole('button', { name: locale === 'vi' ? 'Xóa dòng' : 'Remove line', exact: true }).click();
    const verify = page.getByRole('button', { name: messages['repair.verify'], exact: true });
    assert.equal(await verify.isEnabled(), true);
    const [againResponse] = await Promise.all([verifyResponse(), verify.click()]);
    assert.equal(againResponse.ok(), true);
    const returnedReceipt = await againResponse.json();
    assert.equal(returnedReceipt.sha256, originalReceipt.sha256);
    await page.waitForFunction(hash => document.querySelector('.receipt')?.textContent?.includes(hash) && !document.querySelector('.busy-panel'), returnedReceipt.sha256);
    const warning = locale === 'vi' ? /Quyết định trước đã hết hiệu lực/ : /previous decision is stale/;
    assert.match(await page.locator('main').innerText(), warning);
    assert.match(await page.locator('.accept-section').innerText(), warning);
    const axe = await new AxeBuilder({ page }).analyze();
    assert.deepEqual(axe.violations.filter(v => ['serious', 'critical'].includes(v.impact)).map(v => v.id), []);
    await page.setViewportSize({ width: 390, height: 900 });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth), true);
    await page.screenshot({ path: `${output}/${locale}-stale.png`, fullPage: true });
    await page.locator('.rail-step').nth(4).click();
    const reportDownload = page.getByRole('button', { name: messages['evidence.downloadReport'].replace('{language}', locale === 'vi' ? 'Tiếng Việt' : 'English'), exact: true });
    assert.equal(await reportDownload.isEnabled(), true);
    const [download] = await Promise.all([
      page.waitForEvent('download'),
      reportDownload.click(),
    ]);
    const reportPath = `${output}/${locale}-stale.md`;
    await download.saveAs(reportPath);
    const markdown = readFileSync(reportPath, 'utf8');
    assert.match(markdown, locale === 'vi' ? /Quyết định trước đã hết hiệu lực/ : /previous decision is stale/);
    assert.doesNotMatch(markdown, locale === 'vi' ? /Đã nhận để review/ : /Received for review/);
    await page.locator('.rail-step').nth(3).click();
    await page.getByRole('button', { name: locale === 'vi' ? 'Nhận để review' : 'Receive for review', exact: true }).click();
    assert.match(await page.locator('.accept-section').innerText(), locale === 'vi' ? /Đã nhận để review/ : /Received for review/);
    await page.locator('.rail-step').nth(0).click();
    const after = page.locator('#after-acl');
    const originalAfter = await after.inputValue();
    await after.fill(`${originalAfter}\n`);
    await after.fill(originalAfter);
    assert.match(await page.locator('main').innerText(), warning);
    assert.deepEqual(external, []);
    assert.deepEqual(modelCalls, []);
    results.push({ locale, receipt: originalReceipt.sha256, sameReceiptStale: true, markdownStale: true, explicitRenewal: true, restoredSourceStale: true });
    console.log(`PASS ${locale}: real repair, completed same-receipt reverify, stale Markdown, explicit renewal, source restore, axe, 390px; AI off`);
    await context.close();
  }
  writeFileSync(`${output}/results.json`, JSON.stringify({ results, limitations: 'Local synthetic engine; no screen-reader, physical mobile or live model qualification' }, null, 2));
} catch (error) {
  if (activePage && !activePage.isClosed()) {
    await activePage.screenshot({ path: `${output}/failure.png`, fullPage: true });
    writeFileSync(`${output}/failure.json`, JSON.stringify({ message: String(error),
      buttons: await activePage.getByRole('button').evaluateAll(nodes => nodes.map(node => ({ text: node.textContent, disabled: node.disabled }))),
      errors: await activePage.locator('.error-panel').allTextContents(),
    }, null, 2));
  }
  throw error;
} finally {
  await browser.close();
}
