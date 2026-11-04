import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';

const base = process.env.QA_BASE_URL ?? 'http://127.0.0.1:8098';
const outputDir = '../out/u1-audit';
const screensDir = `${outputDir}/screenshots`;
mkdirSync(screensDir, { recursive: true });

const viewports = [360, 390, 480, 768, 1024, 1280, 1440, 1920];

const browser = await chromium.launch({
  headless: true,
  executablePath: 'C:/Program Files/BraveSoftware/Brave-Browser/Application/brave.exe'
});

const report = {
  timestamp: new Date().toISOString(),
  targetUrl: base,
  viewports,
  overflowResults: [],
  axeSummary: {},
  touchTargetViolations: [],
  p0Findings: {},
  screenInventories: {}
};

try {
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
    reducedMotion: 'reduce'
  });
  const page = await context.newPage();

  console.log('Navigating to', base);
  await page.goto(base);
  await page.locator('#quick-verdict-title').waitFor();
  await page.waitForLoadState('networkidle');

  // Audit 1: QUICK SCREEN across 8 viewports
  console.log('Auditing Quick Screen...');
  for (const width of viewports) {
    await page.setViewportSize({ width, height: 900 });
    await page.waitForTimeout(100);

    const overflow = await page.evaluate(() => {
      const doc = document.documentElement;
      return {
        innerWidth: window.innerWidth,
        scrollWidth: doc.scrollWidth,
        clientWidth: doc.clientWidth,
        diff: doc.scrollWidth - window.innerWidth
      };
    });

    const axe = await new AxeBuilder({ page }).analyze();
    const violations = axe.violations.map(v => ({
      id: v.id,
      impact: v.impact,
      description: v.description,
      nodesCount: v.nodes.length,
      selectors: v.nodes.map(n => n.target.join(' > '))
    }));

    report.overflowResults.push({
      screen: 'quick',
      width,
      overflow: overflow.diff > 0,
      diff: overflow.diff,
      scrollWidth: overflow.scrollWidth
    });

    report.axeSummary[`quick-${width}`] = violations;

    const screenPath = `${screensDir}/quick-${width}.png`;
    await page.screenshot({ path: screenPath, fullPage: true });

    // Touch targets check on mobile (<= 480)
    if (width <= 480) {
      const smallTargets = await page.evaluate(() => {
        const elements = Array.from(document.querySelectorAll('button, a, input, select, textarea, summary, [role="button"]'));
        return elements.map(el => {
          const rect = el.getBoundingClientRect();
          return {
            tag: el.tagName,
            text: (el.textContent || el.getAttribute('aria-label') || '').trim().slice(0, 30),
            width: Math.round(rect.width),
            height: Math.round(rect.height),
            visible: rect.width > 0 && rect.height > 0 && window.getComputedStyle(el).visibility !== 'hidden'
          };
        }).filter(t => t.visible && (t.width < 44 || t.height < 44));
      });
      report.touchTargetViolations.push({ screen: 'quick', width, smallTargets });
    }
  }

  // Audit 2: P0-1 ACL Long line editor check in Quick Screen
  console.log('Checking P0-1: Long line editor wrapping...');
  {
    await page.setViewportSize({ width: 1440, height: 900 });
    const editorSummary = page.locator('details summary').filter({ hasText: /Toàn bộ cấu hình|All configuration/ });
    if (await editorSummary.count() > 0) {
      await editorSummary.first().click();
      await page.waitForTimeout(200);
      const editorCheck = await page.evaluate(() => {
        const textareas = Array.from(document.querySelectorAll('textarea'));
        return textareas.map(ta => ({
          id: ta.id,
          scrollWidth: ta.scrollWidth,
          clientWidth: ta.clientWidth,
          hasHorizontalScroll: ta.scrollWidth > ta.clientWidth,
          wrap: ta.getAttribute('wrap') || window.getComputedStyle(ta).whiteSpace
        }));
      });
      report.p0Findings.p0_1_editor = editorCheck;
      await page.screenshot({ path: `${screensDir}/p0-1-editor-expanded.png` });
    }
  }

  // Audit 3: P0-2 Verdict Inbox check
  console.log('Checking P0-2: Verdict inbox whitespace and watch form...');
  {
    const drawer = page.locator('.watch-drawer');
    if (await drawer.count() > 0) {
      await drawer.locator('summary').click();
      await page.waitForTimeout(300);

      for (const width of [390, 768, 1440]) {
        await page.setViewportSize({ width, height: 900 });
        const inboxMetrics = await page.evaluate(() => {
          const panel = document.querySelector('.inbox-panel');
          const form = document.querySelector('.watch-form') || document.querySelector('.inbox-panel form');
          if (!panel) return null;
          const panelRect = panel.getBoundingClientRect();
          const formRect = form ? form.getBoundingClientRect() : null;
          return {
            panelWidth: Math.round(panelRect.width),
            panelHeight: Math.round(panelRect.height),
            formWidth: formRect ? Math.round(formRect.width) : null,
            formHeight: formRect ? Math.round(formRect.height) : null,
            entriesCount: document.querySelectorAll('.inbox-entry').length
          };
        });
        await page.screenshot({ path: `${screensDir}/inbox-${width}.png` });
        report.p0Findings[`p0_2_inbox_${width}`] = inboxMetrics;
      }
    }
  }

  // Audit 4: Chat Panel check
  console.log('Auditing Chat Panel...');
  {
    const chatSummary = page.locator('details summary').filter({ hasText: /Thẩm vấn|Ask about/ });
    if (await chatSummary.count() > 0) {
      await chatSummary.first().click();
      await page.waitForTimeout(300);

      for (const width of [390, 768, 1440]) {
        await page.setViewportSize({ width, height: 900 });
        await page.screenshot({ path: `${screensDir}/chat-${width}.png` });
      }
      const chatAxe = await new AxeBuilder({ page }).analyze();
      report.axeSummary['chat-1440'] = chatAxe.violations.map(v => ({
        id: v.id,
        impact: v.impact,
        description: v.description
      }));
    }
  }

  // Audit 5: Deep Review Mode across all 5 stages
  console.log('Auditing Deep Review Mode...');
  {
    await page.setViewportSize({ width: 1440, height: 900 });
    const deepSwitch = page.locator('.mode-switch button').filter({ hasText: /Duyệt sâu|Deep/ });
    if (await deepSwitch.count() > 0) {
      await deepSwitch.click();
      await page.waitForTimeout(300);

      const stages = ['source', 'conditions', 'result', 'repair', 'evidence'];
      for (const [index, stage] of stages.entries()) {
        const stepButton = page.locator('.rail-step').nth(index);
        if (await stepButton.isEnabled()) {
          await stepButton.click();
          await page.waitForTimeout(300);

          for (const width of [390, 768, 1440]) {
            await page.setViewportSize({ width, height: 900 });
            const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
            report.overflowResults.push({
              screen: `deep-${stage}`,
              width,
              overflow: overflow > 0,
              diff: overflow
            });
            await page.screenshot({ path: `${screensDir}/deep-${stage}-${width}.png`, fullPage: true });
          }

          const stageAxe = await new AxeBuilder({ page }).analyze();
          report.axeSummary[`deep-${stage}-1440`] = stageAxe.violations.map(v => ({
            id: v.id,
            impact: v.impact,
            description: v.description
          }));
        }
      }
    }
  }

  writeFileSync(`${outputDir}/audit-report.json`, JSON.stringify(report, null, 2));
  console.log('Audit completed successfully. Report written to', `${outputDir}/audit-report.json`);
} finally {
  await browser.close();
}
