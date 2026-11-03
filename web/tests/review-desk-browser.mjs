import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
const output = '../out/review-desk';
mkdirSync(output, { recursive: true });
const browser = await chromium.launch({ executablePath: 'C:/Program Files/BraveSoftware/Brave-Browser/Application/brave.exe' });
const results = [];
async function check(page, label) {
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, `${label}: overflow`);
  // The cream palette stays light under either OS appearance preference.
  assert.equal(await page.evaluate(() => getComputedStyle(document.body).backgroundColor), 'rgb(245, 241, 232)');
  assert.equal(await page.evaluate(() => getComputedStyle(document.documentElement).colorScheme), 'light only');
  const axe = await new AxeBuilder({ page }).analyze();
  assert.deepEqual(axe.violations.filter(v => ['serious', 'critical'].includes(v.impact)).map(v => ({ id: v.id, nodes: v.nodes.map(n => n.target) })), [], `${label}: axe`);
  results.push(label);
}
try {
  for (const locale of ['vi', 'en']) for (const colorScheme of ['light', 'dark']) {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, colorScheme });
    const page = await context.newPage();
    const external = [];
    page.on('request', r => { if (!r.url().startsWith('http://127.0.0.1:8098')) external.push(r.url()); });
    await page.goto('http://127.0.0.1:8098');
    if (locale === 'en') await page.getByRole('button', { name: /EN —/ }).click();
    await page.locator('#quick-verdict-title').waitFor();
    assert.equal(await page.locator('.quick-sources').getAttribute('open'), null);
    assert.equal(await page.locator('.quick-policy').getAttribute('open'), null);
    assert.equal(await page.locator('.quick-questions').getAttribute('open'), null);
    assert.match(await page.locator('.quick-outcome__rule').innerText(), /deny tcp host 192\.0\.2\.10 any eq 22/);
    assert.ok((await page.locator('main').innerText()).trim().split(/\s+/).length < 200, 'quick entry is under 200 visible words');
    for (const width of [320, 390, 768, 1440]) {
      await page.setViewportSize({ width, height: 1000 });
      await check(page, `${locale}-${colorScheme}-${width}-quick`);
      await page.screenshot({ path: `${output}/${locale}-${colorScheme}-${width}.png`, fullPage: true });
    }
    await page.locator('.quick-tools > summary').click();
    await page.locator('.quick-policy > summary').click();
    await check(page, `${locale}-${colorScheme}-conditions-open`);
    await page.locator('.watch-drawer > summary').click();
    await check(page, `${locale}-${colorScheme}-watches-open`);
    await page.locator('.watch-drawer > summary').click();
    await page.getByRole('button', { name: locale === 'vi' ? 'Duyệt sâu' : 'Deep review', exact: true }).click();
    await page.locator('.rail-step').nth(0).click();
    await check(page, `${locale}-${colorScheme}-deep-source`);
    await page.getByRole('button', { name: locale === 'vi' ? 'Xem hành vi thay đổi' : 'Explore behavior changes', exact: true }).click();
    await page.locator('.verdict-plate').waitFor();
    await check(page, `${locale}-${colorScheme}-deep-result`);
    await page.screenshot({ path: `${output}/${locale}-${colorScheme}-deep.png`, fullPage: true });
    assert.deepEqual(external, []);
    await context.close();
  }
  const context = await browser.newContext({ viewport: { width: 390, height: 900 } });
  const p = await context.newPage();
  await p.goto('http://127.0.0.1:8098');
  await p.locator('#quick-verdict-title').waitFor();
  await p.getByRole('button', { name: 'Duyệt sâu', exact: true }).click();
  await p.getByRole('button', { name: /Điều kiện & phạm vi/ }).click();
  await p.getByRole('button', { name: 'Xem policy đã chuẩn hóa', exact: true }).click();
  await p.getByRole('checkbox', { name: /Tôi đã đọc và chấp nhận bộ điều kiện/ }).check();
  await p.getByRole('button', { name: 'Chấp nhận & kiểm chứng', exact: true }).click();
  await p.getByRole('button', { name: 'Chuẩn bị bản sửa', exact: true }).click();
  await p.locator('.rule-builder__row').first().getByRole('button', { name: 'Xóa dòng', exact: true }).click();
  await p.getByRole('button', { name: 'Kiểm chứng bản sửa', exact: true }).click();
  await p.locator('.receipt').waitFor();
  await check(p, 'vi-mobile-repair');
  await p.getByRole('button', { name: 'Nhận để review', exact: true }).click();
  assert.match(await p.locator('.accept-section').innerText(), /Đã nhận để review/);
  await p.screenshot({ path: `${output}/vi-repair.png`, fullPage: true });
  await p.getByRole('button', { name: 'Kết luận nhanh', exact: true }).click();
  await p.locator('.quick-tools > summary').click();
  await p.locator('.quick-policy > summary').focus();
  await p.keyboard.press('Enter');
  assert.equal(await p.locator('.quick-policy').getAttribute('open'), '');
  await p.getByRole('button', { name: 'Xem / sửa cấu hình', exact: true }).click();
  await p.locator('#sample-select').selectOption('blank');
  await check(p, 'vi-empty');
  await p.locator('#before-acl').fill('ip access-list extended MGMT\n 10 permit tcp any');
  await p.locator('#after-acl').fill('ip access-list extended MGMT\n 10 permit tcp any');
  await p.locator('.object-errors').first().waitFor();
  assert.equal(await p.locator('#quick-verdict-title').count(), 0);
  await check(p, 'vi-invalid');
  await p.screenshot({ path: `${output}/vi-error.png`, fullPage: true });
  await p.close();
} finally {
  await browser.close();
  writeFileSync(`${output}/results.json`, JSON.stringify(results, null, 2));
  console.log(`${results.length} checks passed`);
}

