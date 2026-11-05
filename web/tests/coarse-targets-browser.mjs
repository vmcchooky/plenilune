import assert from 'node:assert/strict';
import { mkdirSync } from 'node:fs';
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';

mkdirSync('../out/n2-touch-fixed', { recursive: true });
const browser = await chromium.launch({ executablePath: 'C:/Program Files/BraveSoftware/Brave-Browser/Application/brave.exe' });
try {
  for (const locale of ['vi', 'en']) {
    for (const width of [360, 390, 480, 768, 1024, 1280, 1440, 1920]) {
      const context = await browser.newContext({ viewport: { width, height: 900 }, hasTouch: true });
      try {
        const page = await context.newPage();
        await page.goto('http://127.0.0.1:8098');
        await page.locator('#quick-verdict-title').waitFor();
        if (locale === 'en') await page.getByRole('button', { name: /EN —/ }).click();
        await page.evaluate(() => window.scrollTo(0, 0));
        if (await page.locator('.watch-drawer').getAttribute('open') === null) {
          await page.locator('.watch-drawer > summary').click();
        }
        if (await page.locator('.inbox-watch__add').getAttribute('open') === null) {
          await page.locator('.inbox-watch__add > summary').click();
        }
        const watch = page.locator('.inbox-watch__form input:not([type])');
        assert.ok(await watch.count() > 0, 'watch text input exists');
        assert.ok((await watch.boundingBox()).height >= 44, `${locale}-${width}: watch input height`);
        await page.getByRole('button', { name: locale === 'vi' ? 'Duyệt sâu' : 'Deep review', exact: true }).click();
        await page.getByRole('button', { name: locale === 'vi' ? /Điều kiện & phạm vi/ : /Conditions & scope/ }).click();
        const conditionInputs = page.locator('.condition-list .region-fields input:not([type]):visible');
        assert.ok(await conditionInputs.count() > 0, 'condition text inputs exist');
        for (const input of await conditionInputs.all()) {
          assert.ok((await input.boundingBox()).height >= 44, `${locale}-${width}: condition input height`);
        }
        await page.locator('.scope-option input[value="regions"]').check();
        await page.locator('.scope-regions > button').click();
        const scopeInputs = page.locator('.scope-regions .region-fields input:not([type]):visible');
        assert.ok(await scopeInputs.count() > 0, 'scope text inputs exist');
        for (const input of await scopeInputs.all()) {
          assert.ok((await input.boundingBox()).height >= 44, `${locale}-${width}: scope input height`);
        }
        await page.locator('.scope-option input[value="undeclared"]').check();
        await page.getByRole('button', { name: locale === 'vi' ? 'Xem policy đã chuẩn hóa' : 'Preview normalized policy', exact: true }).click();
        await page.getByRole('checkbox', { name: locale === 'vi' ? /Tôi đã đọc và chấp nhận bộ điều kiện/ : /I have reviewed and accept these conditions/ }).check();
        await page.getByRole('button', { name: locale === 'vi' ? 'Chấp nhận & kiểm chứng' : 'Accept & verify', exact: true }).click();
        await page.getByRole('button', { name: locale === 'vi' ? 'Chuẩn bị bản sửa' : 'Prepare a repair', exact: true }).click();
        await page.locator('#append-rule').waitFor();
        const input = page.locator('#append-rule');
        const consent = page.locator('[aria-labelledby="repair-ai-title"] .ai-consent__controls .checkbox-row');
        assert.ok((await input.boundingBox()).height >= 44, `${locale}-${width}: repair input height`);
        assert.ok((await consent.boundingBox()).height >= 44, `${locale}-${width}: consent label height`);
        const checkbox = consent.locator('input[type="checkbox"]');
        await consent.click();
        assert.equal(await checkbox.isChecked(), true, 'clicking the label checks consent');
        await checkbox.focus();
        await page.keyboard.press('Space');
        assert.equal(await checkbox.isChecked(), false, 'keyboard toggles consent');
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth), 0, `${locale}-${width}: overflow`);
        const axe = await new AxeBuilder({ page }).analyze();
        assert.deepEqual(axe.violations.map(violation => violation.id), [], `${locale}-${width}: axe`);
        if ([390, 1440].includes(width)) await page.screenshot({ path: `../out/n2-touch-fixed/${locale}-${width}.png`, fullPage: true });
        console.log(`${locale}-${width}: PASS`);
      } finally {
        await context.close();
      }
    }
  }
} finally {
  await browser.close();
}
