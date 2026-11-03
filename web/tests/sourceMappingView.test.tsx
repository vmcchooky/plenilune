import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, expect, it } from 'vitest';
import type { Review } from '../src/hooks/useReview';
import { LocaleProvider, translate } from '../src/i18n';
import { quickOutcomeCause, quickOutcomeData } from '../src/components/QuickOutcomeData';
import { ChangeClasses, PacketProbe } from '../src/components/ChangeClasses';
import { WitnessDetail } from '../src/components/WitnessDetail';
import { compositionModel, decidingNumber, decidingText } from '../src/design/compositions/model';
import { changeClass, comparison, parsedResult, witness } from './fixtures';

afterEach(cleanup);

const localBefore = 'access-list 42 permit any\r\naccess-list 42 deny host 192.0.2.7\r\n';
const localAfter = 'access-list 42 deny any\r\naccess-list 42 permit host 192.0.2.7\r\n';
const point = { ...witness,
  before: { ...witness.before, line: 2, rule_ordinal: 0 },
  after: { ...witness.after, line: 2, rule_ordinal: 0 },
};

function reviewView(): Review {
  const report = { ...comparison.report, witnesses: [point], obligations: [] };
  const changes = { schema: 'plenilune.acl-changes/v1', engine: 'acl-exact/v0.4', complete: true,
    scope_declared: false, total_classes: 1, classes_truncated: false,
    classes: [{ ...changeClass, before: { ...changeClass.before, line: 2 }, after: { ...changeClass.after, line: 2 }, witness: point }],
    budget: { operations: 1, peak_boxes: 1, stop_reason: '', timing_limited: false } };
  return {
    comparison: { comparison: { ...comparison, report }, raw: '' }, accepted: null, analysis: null,
    parsed: { parsed: parsedResult, before: localBefore, after: localAfter, intent: '', firstLine: { before: 2, after: 3 }, sourceLines: { before: [2, 8], after: [3, 11] } },
    draft: { after: `header\n${localAfter}` }, aclChanges: changes, probeResult: null,
    selectedObject: { domain: 'acl', name: '42', kind: 'standard', status: 'changed', references: { before: [], after: [] } },
  } as unknown as Review;
}

it('quick and composition proofs map physical citations while looking up local rule bytes', () => {
  const review = reviewView();
  const data = quickOutcomeData(review)!;
  expect(quickOutcomeCause(data, point)).toEqual({ line: 11, implicit: false, text: 'access-list 42 permit host 192.0.2.7' });
  const model = compositionModel(review)!;
  expect(decidingNumber(model, 'before', point.before)).toBe(8);
  expect(decidingText(model, 'before', point.before)).toBe('access-list 42 deny host 192.0.2.7');
  expect(point.after.line).toBe(2);
  expect(point.after.rule_ordinal).toBe(0);
});

it('quick analysis fallback maps discontiguous extents before any object is opened', () => {
  const review = reviewView();
  review.parsed = null;
  review.draft.after = 'first\nsecond\naccess-list 42 deny any\nfourth\nfifth\nsixth\nseventh\neighth\nninth\ntenth\naccess-list 42 permit host 192.0.2.7\n';
  review.selectedObject!.after = { lines: [[3, 3], [11, 11]], sha256: 'a'.repeat(64), units: 2 };
  const data = quickOutcomeData(review)!;
  expect(quickOutcomeCause(data, point)).toEqual({ line: 11, implicit: false, text: 'access-list 42 permit host 192.0.2.7' });
});

it('change class labels cite original rows and leave local witness evidence intact', () => {
  localStorage.setItem('plenilune.locale', 'en');
  const review = reviewView();
  render(<LocaleProvider><ChangeClasses review={review} /></LocaleProvider>);
  expect(screen.getByText(translate('en', 'changes.rule', { line: 8 }), { exact: false })).toBeTruthy();
  expect(screen.getByText(translate('en', 'changes.rule', { line: 11 }), { exact: false })).toBeTruthy();
  expect(screen.getByText('access-list 42 permit host 192.0.2.7')).toBeTruthy();
  expect(review.aclChanges!.classes[0].after.line).toBe(2);
});

it('witness display maps labels and source gutter while raw witness numbers remain local', () => {
  localStorage.setItem('plenilune.locale', 'en');
  render(<LocaleProvider><WitnessDetail witness={point} title="Point" before={localBefore} after={localAfter}
    changedBefore={new Set()} changedAfter={new Set()} sourceLineMap={{ before: [2, 8], after: [3, 11] }} /></LocaleProvider>);
  expect(screen.getByText(translate('en', 'result.ruleLine', { line: 8 }))).toBeTruthy();
  expect(screen.getByText(translate('en', 'result.ruleLine', { line: 11 }))).toBeTruthy();
  expect(Array.from(document.querySelectorAll('.source-pane__number'), node => node.textContent)).toEqual(['2', '8', '3', '11']);
  expect(point.before.line).toBe(2);
});

it('point-probe results cite original rows without changing the trace decisions', () => {
  localStorage.setItem('plenilune.locale', 'en');
  const review = reviewView();
  review.probeResult = { packet: point.packet, before: point.before, after: point.after };
  render(<LocaleProvider><PacketProbe review={review} seed={null} /></LocaleProvider>);
  expect(screen.getByText(translate('en', 'changes.rule', { line: 8 }), { exact: false })).toBeTruthy();
  expect(screen.getByText(translate('en', 'changes.rule', { line: 11 }), { exact: false })).toBeTruthy();
  expect(review.probeResult.after.line).toBe(2);
});
