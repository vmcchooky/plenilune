// Real local analysis; only inbox responses are controlled to reproduce arrival
// during typing. This is a UI regression, not live-provider/device qualification.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';

const base = 'http://127.0.0.1:8098';
const output = '../out/morning-review';
mkdirSync(output, { recursive: true });
const browser = await chromium.launch({ executablePath: 'C:/Program Files/BraveSoftware/Brave-Browser/Application/brave.exe' });
const results = [];
try {
  for (const locale of ['vi', 'en']) {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, reducedMotion: 'reduce' });
    const page = await context.newPage();
    const external = [];
    const models = [];
    page.on('request', request => {
      if (new URL(request.url()).origin !== base) external.push(request.url());
      if (request.url().includes('/api/agent/')) models.push(request.url());
    });
    let armed = false;
    let notify;
    let release;
    const requested = new Promise(resolve => { notify = resolve; });
    const held = new Promise(resolve => { release = resolve; });
    const entry = { id: 1, domain: 'acl', name: 'MGMT', condition: 'any_different', verdict: 'DIFFERENT',
      artifact_kind: 'analysis', content_sha256: 'a'.repeat(64), time: '2026-11-03T01:00:00Z', read: false, stale: false };
    await page.route(`${base}/api/inbox`, async route => {
      if (armed) { notify(); await held; }
      await route.fulfill({ json: { entries: armed ? [entry] : [], unread: armed ? 1 : 0 } });
    });
    await page.route(`${base}/api/inbox/read`, route => route.fulfill({ json: { entries: [{ ...entry, read: true }], unread: 0 } }));
    await page.goto(base);
    if (locale === 'en') await page.getByRole('button', { name: /EN —/ }).click();
    await page.locator('#quick-verdict-title').waitFor();
    await page.waitForLoadState('networkidle');
    const packet = await page.locator('.quick-outcome__example').innerText();
    assert.match(packet, /192\.0\.2\.10:0/);
    assert.match(packet, /198\.51\.100\.10:22/);
    assert.doesNotMatch(packet, /\(any\)/);
    const drawer = page.locator('.watch-drawer');
    await drawer.locator('summary').click();
    const name = drawer.getByRole('textbox');
    await name.fill('DRAFT-ACL');
    armed = true;
    await drawer.getByRole('button', { name: locale === 'vi' ? 'Làm mới' : 'Refresh', exact: true }).click();
    await requested;
    await name.focus();
    release();
    const jump = page.locator('.workspace-actions button').filter({ hasText: locale === 'vi' ? '1 chưa đọc' : '1 unread' });
    await jump.waitFor();
    assert.equal(await name.inputValue(), 'DRAFT-ACL');
    assert.equal(await name.evaluate(node => node === document.activeElement), true);
    assert.equal(await drawer.evaluate(node => node.open), true);
    await jump.click();
    assert.equal(await drawer.locator('summary').evaluate(node => node === document.activeElement), true);
    await drawer.locator('summary').click();
    for (const width of [320, 390, 768, 1440]) {
      await page.setViewportSize({ width, height: 1000 });
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
      const axe = await new AxeBuilder({ page }).analyze();
      assert.deepEqual(axe.violations.filter(v => ['serious', 'critical'].includes(v.impact)).map(v => v.id), []);
      await page.screenshot({ path: `${output}/${locale}-${width}.png`, fullPage: true });
      results.push(`${locale}-${width}: unread action, concrete ports, no overflow, axe`);
    }
    await jump.click();
    await drawer.getByRole('button', { name: locale === 'vi' ? 'Đánh dấu tất cả đã đọc' : 'Mark all read', exact: true }).click();
    await jump.waitFor({ state: 'detached' });
    assert.equal(await name.inputValue(), 'DRAFT-ACL');
    assert.equal(await drawer.evaluate(node => node.open), true);
    assert.deepEqual(external, []);
    assert.deepEqual(models, []);
    await context.close();
  }
  writeFileSync(`${output}/results.json`, JSON.stringify({ results, limitations: 'Inbox mocked; analysis local; no physical device, reader or live model qualification' }, null, 2));
  console.log(`${results.length} viewport checks plus notification draft/focus/read flow passed`);
} finally {
  await browser.close();
}
