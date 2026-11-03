import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
const base = 'http://127.0.0.1:8098';
const out = '../out/compositions';
mkdirSync(out, { recursive: true });
const browser = await chromium.launch({ executablePath: 'C:/Program Files/BraveSoftware/Brave-Browser/Application/brave.exe' });
const results = [];
async function check(page, label) {
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, `${label}: overflow`);
  const axe = await new AxeBuilder({ page }).analyze();
  assert.deepEqual(axe.violations.filter(v => ['serious', 'critical'].includes(v.impact)).map(v => ({ id: v.id, nodes: v.nodes.map(n => n.target) })), [], `${label}: axe`);
  results.push(label);
}
try {
  for (const locale of ['vi', 'en']) {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, colorScheme: 'dark' });
    const page = await context.newPage();
    const requests = [];
    page.on('request', r => requests.push(r.url()));
    await page.goto(`${base}/?design=a`);
    await page.locator('.study-canvas').waitFor();
    if (locale === 'en') await page.getByRole('button', { name: /EN —/ }).click();
    const hash = await page.locator('.study-evidence > code').textContent();
    const initialAnalysisCalls = requests.filter(r => /\/api\/(analyze|compare)$/.test(r)).length;
    for (const [index, layout] of ['a', 'b', 'c'].entries()) {
      await page.locator('.study-choices button').nth(index).click();
      assert.equal(new URL(page.url()).searchParams.get('design'), layout);
      assert.equal(await page.locator('.study-evidence > code').textContent(), hash);
      for (const width of [320, 390, 768, 1440]) {
        await page.setViewportSize({ width, height: 1000 });
        await check(page, `${locale}-${layout}-${width}`);
        await page.screenshot({ path: `${out}/${locale}-${layout}-${width}.png`, fullPage: true });
      }
      await page.locator('.study-canvas').screenshot({ path: `${out}/${locale}-${layout}-composition.png` });
      if (layout === 'a') {
        await page.locator('.record-refined__proof-toggle').click();
        assert.equal(await page.locator('.record-refined__proof-toggle').getAttribute('aria-expanded'), 'true');
        assert.equal(await page.locator('#record-rule-proof').isVisible(), true);
        assert.match(await page.locator('.study-proof').innerText(), /permit tcp/);
        assert.match(await page.locator('.study-proof').innerText(), /deny tcp/);
        await page.locator('.record-refined__proof-toggle').click();
        assert.equal(await page.locator('#record-rule-proof').isVisible(), false);
      }
      if (layout === 'b') {
        await page.locator('.study-side-switch button').first().click();
        assert.match(await page.locator('.study-code .is-deciding').innerText(), /permit tcp/);
        await page.locator('.study-side-switch button').last().click();
        assert.match(await page.locator('.study-code .is-deciding').innerText(), /deny tcp/);
      }
      if (layout === 'c') {
        assert.equal(await page.locator('.study-queue button').count(), 1, 'only real change groups');
        await page.locator('.study-queue button').click();
        assert.match(await page.locator('.study-region').innerText(), /192\.0\.2\.10/);
      }
    }
    assert.equal(requests.filter(r => /\/api\/(analyze|compare)$/.test(r)).length, initialAnalysisCalls, 'layout switches reuse evidence');
    await page.goto(`${base}/?design=a&focus=1`);
    await page.locator('.record-refined').waitFor();
    assert.equal(await page.locator('.study-choices').count(), 0);
    for (const width of [320, 390, 768, 1440]) {
      await page.setViewportSize({ width, height: 1000 });
      await check(page, `${locale}-a-focused-${width}`);
      await page.screenshot({ path: `${out}/${locale}-a-focused-${width}.png`, fullPage: true });
    }
    await page.locator('.record-refined__proof-toggle').focus();
    await page.keyboard.press('Enter');
    assert.equal(await page.locator('#record-rule-proof').isVisible(), true);
    await page.keyboard.press('Enter');
    assert.equal(await page.locator('#record-rule-proof').isVisible(), false);
    await page.locator('.study-conditions').click();
    await page.locator('.composition-condition-panel').waitFor();
    await check(page, `${locale}-conditions`);
    await page.getByRole('button', { name: locale === 'vi' ? 'Xem policy đã chuẩn hóa' : 'Preview normalized policy', exact: true }).click();
    await page.locator('.canonical-preview').waitFor();
    const checkbox = page.locator('.accept-checkbox input');
    assert.equal(await checkbox.isChecked(), false, 'acceptance remains explicit');
    assert.deepEqual(requests.filter(r => new URL(r).origin !== base), []);
    assert.deepEqual(requests.filter(r => r.endsWith('/api/agent/session')), []);
    await context.close();
  }
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(base);
  await page.locator('.quick-outcome').waitFor();
  assert.equal(await page.locator('.composition-lab').count(), 0, 'default app unchanged');
  await context.close();
} finally {
  await browser.close();
  writeFileSync(`${out}/results.json`, JSON.stringify(results, null, 2));
  console.log(`${results.length} layout/state checks passed`);
}
