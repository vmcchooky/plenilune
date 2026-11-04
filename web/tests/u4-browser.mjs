import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import AxeBuilder from '@axe-core/playwright';
import { mkdirSync, writeFileSync } from 'node:fs';
const phase=process.argv[2]; const out='../out/u4-evidence'; mkdirSync(out,{recursive:true});
const browser=await chromium.launch({executablePath:'C:/Program Files/BraveSoftware/Brave-Browser/Application/brave.exe'}); const results=[];
try { for (const locale of ['vi','en']) { const context=await browser.newContext({viewport:{width:1440,height:900},reducedMotion:'reduce'}); const page=await context.newPage();
await page.goto('http://127.0.0.1:8098'); if(locale==='en') await page.getByRole('button',{name:/EN —/}).click(); await page.locator('#quick-verdict-title').waitFor();
for(const width of [360,390,480,768,1024,1280,1440,1920]) {await page.setViewportSize({width,height:900}); const overflow=await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth); const axe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21a','wcag21aa','wcag22aa']).analyze(); results.push({locale,width,overflow,violations:axe.violations.map(x=>x.id)}); if (phase === 'after') { assert.equal(overflow, 0); assert.deepEqual(axe.violations.map(x=>x.id), []); } if([360,390,1440].includes(width)) await page.screenshot({path:`${out}/${phase}-${locale}-${width}.png`,fullPage:true}); } await context.close(); } writeFileSync(`${out}/${phase}.json`,JSON.stringify(results,null,2)); console.log(JSON.stringify(results)); } finally {await browser.close();}
