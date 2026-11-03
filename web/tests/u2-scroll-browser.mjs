import { chromium } from 'playwright';
const browser = await chromium.launch({ executablePath: 'C:/Program Files/BraveSoftware/Brave-Browser/Application/brave.exe' });
const page = await browser.newPage({ viewport: { width: 390, height: 900 } });
await page.goto('http://127.0.0.1:5174/');
await page.locator('.mode-switch button').filter({ hasText: /Duyệt sâu|Deep/ }).click();
await page.locator('.rail-step').first().click();
const textarea = page.locator('.source-editor__input').first();
await textarea.fill(Array.from({ length: 100 }, (_, i) => `line ${i + 1} ` + 'x'.repeat(150)).join('\n'));
for (const mode of ['wrapped', 'unwrapped']) {
  if (mode === 'unwrapped') await page.locator('.source-editor__wrap-toggle').first().click();
  const samples = await textarea.evaluate(async el => {
    const gutter = el.parentElement.querySelector('.source-editor__gutter'); const mirror = el.parentElement.querySelector('.source-editor__mirror'); if (!mirror) throw new Error('new build mirror missing');
    const samples = [];
    for (const fraction of [.1, .5, 1, .3]) {
      el.scrollTop = (el.scrollHeight - el.clientHeight) * fraction;
      await new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r)));
      samples.push({ text: el.scrollTop, gutter: gutter.scrollTop, inputClient: el.clientHeight, gutterClient: gutter.clientHeight, inputScroll: el.scrollHeight, gutterScroll: gutter.scrollHeight, mirrorTotal: mirror.getBoundingClientRect().height, mirrorChildren: mirror.children.length, textMax: el.scrollHeight - el.clientHeight, gutterMax: gutter.scrollHeight - gutter.clientHeight });
    }
    return samples;
  });
  console.log(mode, JSON.stringify(samples));
  if (mode === 'wrapped') { await textarea.evaluate(el => { el.scrollTop = (el.scrollHeight-el.clientHeight)/2; }); await page.waitForTimeout(100); await page.locator('.source-editor').first().screenshot({path:'../out/u2-mid-scroll-390.png'}); }
}
await browser.close();
