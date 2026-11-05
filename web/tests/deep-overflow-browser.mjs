import assert from 'node:assert/strict';
import { mkdirSync } from 'node:fs';
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';

mkdirSync('../out/n2-deep-fixed', { recursive: true });
const browser = await chromium.launch({ executablePath: 'C:/Program Files/BraveSoftware/Brave-Browser/Application/brave.exe' });
try {
  for (const locale of ['vi', 'en']) {
    for (const width of [360, 390, 480, 768, 1024, 1280, 1440, 1920]) {
      const context = await browser.newContext({ viewport: { width, height: 900 }, hasTouch: true });
      try {
        const page = await context.newPage();
        await page.goto('http://127.0.0.1:8098');
        if (locale === 'en') await page.getByRole('button', { name: /EN —/ }).click();
        await page.locator('#quick-verdict-title').waitFor();
        await page.locator('.mode-switch button').nth(1).click();
        for (const stage of [0, 1, 2]) await page.locator('.rail-step').nth(stage).click();
        const buttons = page.locator('.action-bar__buttons .button');
        assert.equal(await buttons.count(), 2, `${locale}-${width}: result actions`);
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth), 0, `${locale}-${width}: overflow`);
        for (const button of await buttons.all()) {
          const fits = await button.evaluate(element => element.scrollWidth <= element.clientWidth);
          assert.equal(fits, true, `${locale}-${width}: action label fits`);
          assert.ok((await button.boundingBox()).height >= 44, `${locale}-${width}: action target height`);
        }
        const axe = await new AxeBuilder({ page }).analyze();
        assert.deepEqual(axe.violations.map(violation => violation.id), [], `${locale}-${width}: axe`);
        if ([360, 390, 1440].includes(width)) {
          await page.screenshot({ path: `../out/n2-deep-fixed/${locale}-${width}.png`, fullPage: true });
        }
        console.log(`${locale}-${width}: PASS`);
      } finally {
        await context.close();
      }
    }
  }
} finally {
  await browser.close();
}
