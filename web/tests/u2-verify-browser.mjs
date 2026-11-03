import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';

const base = process.env.QA_BASE_URL ?? 'http://127.0.0.1:8098';
const out = '../out/u2-verify';
mkdirSync(out, { recursive: true });

const viewports = [360, 390, 480, 768, 1024, 1280, 1440, 1920];
const browser = await chromium.launch({
  headless: true,
  executablePath: 'C:/Program Files/BraveSoftware/Brave-Browser/Application/brave.exe'
});

const results = {
  timestamp: new Date().toISOString(),
  overflowChecks: [],
  axeChecks: [],
  screenshots: []
};

try {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  const page = await context.newPage();

  await page.goto(base);
  await page.locator('#quick-verdict-title').waitFor();
  await page.waitForLoadState('networkidle');

  // Switch to Deep mode and click Step 0 (Source)
  const deepSwitch = page.locator('.mode-switch button').filter({ hasText: /Duyệt sâu|Deep/ });
  await deepSwitch.click();
  await page.locator('.rail-step').nth(0).click();
  await page.locator('.source-editor').first().waitFor();

  // Check 8 viewports for overflow on SourceStep
  for (const width of viewports) {
    await page.setViewportSize({ width, height: 900 });
    const overflowDiff = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    assert.equal(overflowDiff <= 0, true, `Overflow on width ${width}: diff=${overflowDiff}`);
    results.overflowChecks.push({ screen: 'deep-source', width, diff: overflowDiff, pass: overflowDiff <= 0 });

    if ([390, 1440].includes(width)) {
      const snapWrapped = `${out}/deep-source-wrapped-${width}.png`;
      await page.screenshot({ path: snapWrapped, fullPage: true });
      results.screenshots.push(snapWrapped);

      // Toggle wrap
      const toggle = page.locator('.source-editor__wrap-toggle').first();
      await toggle.click();
      await page.waitForTimeout(200);

      const snapUnwrapped = `${out}/deep-source-unwrapped-${width}.png`;
      await page.screenshot({ path: snapUnwrapped, fullPage: true });
      results.screenshots.push(snapUnwrapped);

      await toggle.click();
      await page.waitForTimeout(200);
    }
  }

  // Axe accessibility scan on the source editors
  const axe = await new AxeBuilder({ page }).include('.source-editor').analyze();
  assert.equal(axe.violations.length, 0, `Axe violations in editor: ${JSON.stringify(axe.violations)}`);
  results.axeChecks.push({ target: '.source-editor', violations: axe.violations.length });

  // Now switch back to Quick mode and check overflow across all 8 viewports
  const quickSwitch = page.locator('.mode-switch button').filter({ hasText: /Kết luận nhanh|Quick/ });
  await quickSwitch.click();
  await page.locator('#quick-verdict-title').waitFor();

  for (const width of viewports) {
    await page.setViewportSize({ width, height: 900 });
    const overflowDiff = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    assert.equal(overflowDiff <= 0, true, `Quick mode overflow on width ${width}: diff=${overflowDiff}`);
    results.overflowChecks.push({ screen: 'quick', width, diff: overflowDiff, pass: overflowDiff <= 0 });
  }

  writeFileSync(`${out}/results.json`, JSON.stringify(results, null, 2));
  console.log('U2 verification completed successfully. All 8 viewports overflow=0, axe=0 violations in editor.');
} finally {
  await browser.close();
}
