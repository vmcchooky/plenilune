import { chromium } from 'playwright';
import AxeBuilder from '@axe-core/playwright';
import { mkdirSync, writeFileSync } from 'node:fs';
const phase=process.argv[2]; const out='../out/u3-evidence'; mkdirSync(out,{recursive:true});
const browser=await chromium.launch({executablePath:'C:/Program Files/BraveSoftware/Brave-Browser/Application/brave.exe'});
const results=[];
try { for(const width of [360,390,480,768,1024,1280,1440,1920]) {
 const context=await browser.newContext({viewport:{width,height:1000},reducedMotion:'reduce'}); const page=await context.newPage();
 await page.route('**/api/inbox',r=>r.fulfill({json:{entries:[{id:1,domain:'acl',name:'EDGE-IN',condition:'verdict_changed',previous_verdict:'EQUIVALENT',verdict:'DIFFERENT',artifact_kind:'analysis',content_sha256:'a'.repeat(64),time:'2026-11-03T01:00:00Z',read:false,stale:false},{id:2,domain:'prefix-list',name:'ROUTE-FILTER',condition:'obligation_failed',verdict:'INCONCLUSIVE',violated_obligations:['OB-1'],artifact_kind:'analysis',content_sha256:'b'.repeat(64),time:'2026-11-03T02:00:00Z',read:true,stale:true}],unread:1}}));
 await page.route('**/api/watches',r=>r.fulfill({json:{watches:[]}}));
 await page.goto('http://127.0.0.1:5173/'); await page.locator('.watch-drawer > summary').click(); await page.locator('.inbox-entry').first().waitFor();
 if(phase==='after') await page.locator('.inbox-watch__add > summary').click();
 const overflow=await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth);
 const axe=await new AxeBuilder({page}).include('.inbox-panel').withTags(['wcag2a','wcag2aa','wcag21a','wcag21aa','wcag22aa']).analyze();
 results.push({width,overflow,violations:axe.violations.map(v=>({id:v.id,impact:v.impact}))});
 if([390,1440].includes(width)) await page.locator('.watch-drawer').screenshot({path:`${out}/${phase}-${width}.png`});
 await context.close();
 } writeFileSync(`${out}/${phase}.json`,JSON.stringify(results,null,2)); console.log(JSON.stringify(results)); } finally { await browser.close(); }

