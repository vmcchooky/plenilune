import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { ChangeClasses } from '../src/components/ChangeClasses';
import { PartialNotice } from '../src/components/PartialNotice';
import type { Review } from '../src/hooks/useReview';
import { LocaleProvider, translate } from '../src/i18n';
import { partialReportLines } from '../src/lib/partial';
import { buildTicketSummary } from '../src/lib/report';
import type { ReportInput } from '../src/lib/report';
import type { ChangeReport, PartialResult } from '../src/lib/types';
import { accepted, changeClass, comparisonSnapshot, repairSnapshot } from './fixtures';

// P1-2 web rendering: a partial result must stay unmistakably partial on
// every surface — badge, proven facts only, and the undecided remainder
// described as undecided (never as equivalent or different).

const swapPartial: PartialResult = {
  stage: 'permit_before',
  before_rule: 330,
  prefix_pairs: 256,
  decided_pairs: [],
  undecided: [{ direction: 'delta', before_rules: [256, 511], after_rules: [256, 511] }],
};

const decidedPartial: PartialResult = {
  stage: 'decide_gained',
  prefix_pairs: 0,
  decided_pairs: [
    { before: 3, after: 3, pieces: 2 },
    { before: -1, after: 5, pieces: 1 },
  ],
  undecided: [{ direction: 'gained', after_rules: [13, 511], work_boxes: 41 }],
};

function renderNotice(partial: PartialResult) {
  return render(
    <LocaleProvider>
      <PartialNotice partial={partial} />
    </LocaleProvider>,
  );
}

afterEach(() => {
  cleanup();
  window.localStorage.clear();
});

describe('PartialNotice', () => {
  it('shows the prefix proof and bounds the undecided remainder (vi)', () => {
    window.localStorage.setItem('plenilune.locale', 'vi');
    const { container } = renderNotice(swapPartial);
    const text = container.textContent ?? '';
    expect(text).toContain('Kết quả một phần');
    expect(text).toContain('256 cặp quy tắc đầu đã quyết định: giống hệt');
    expect(text).toContain('Phần chưa quyết định');
    expect(text).toContain('quy tắc trước 256–511 · quy tắc sau 256–511');
    expect(text).toContain('không tương đương, cũng không khác biệt');
  });

  it('lists decided pairs, with the implicit deny spelled out (en)', () => {
    window.localStorage.setItem('plenilune.locale', 'en');
    const { container } = renderNotice(decidedPartial);
    const text = container.textContent ?? '';
    expect(text).toContain('Partial result');
    expect(text).toContain('Rule pairs proven different: 2');
    expect(text).toContain('rule 3 ↔ rule 3 · 2 fragment(s)');
    expect(text).toContain('implicit deny ↔ rule 5 · 1 fragment(s)');
    expect(text).toContain('Gained behavior not yet decided');
    expect(text).toContain('41 undecided region box(es) remained');
  });
});

const partialChanges: ChangeReport = {
  schema: 'plenilune.acl-changes/v1',
  engine: 'acl-exact/v0.4',
  complete: false,
  reason: 'operations',
  partial: {
    stage: 'classes',
    prefix_pairs: 0,
    decided_pairs: [{ before: 0, after: 0, pieces: 1 }],
    undecided_pairs: [{ before: 7, after: 7, pieces: 3 }],
    undecided: [{ direction: 'classes', before_rules: [1, 9], after_rules: [1, 9] }],
  },
  scope_declared: false,
  classes: [changeClass],
  classes_truncated: false,
  total_classes: 2,
  budget: { operations: 10, peak_boxes: 2, stop_reason: 'operations', timing_limited: false },
};

function changesReview(report: ChangeReport): Review {
  return {
    aclChanges: report,
    accepted: null,
    parsed: {
      before: 'ip access-list extended MGMT\n permit tcp host 192.0.2.10 host 198.51.100.10 eq 22\n',
      after: 'ip access-list extended MGMT\n deny tcp host 192.0.2.10 host 198.51.100.10 eq 22\n',
      firstLine: { before: 1, after: 1 },
    },
  } as unknown as Review;
}

describe('ChangeClasses with a partial report', () => {
  it('keeps the decided classes listed and bounds the rest as undecided', () => {
    window.localStorage.setItem('plenilune.locale', 'vi');
    render(
      <LocaleProvider>
        <ChangeClasses review={changesReview(partialChanges)} />
      </LocaleProvider>,
    );
    // The decided class is rendered like any proven class…
    expect(document.querySelector('.change-class-list')).not.toBeNull();
    expect(screen.getByText(/chỉ là phần thay đổi đã quyết định/)).toBeTruthy();
    // …and the undecided pair is named, not dropped.
    expect(screen.getByText(/Cặp quy tắc chưa quyết định: 1/)).toBeTruthy();
    expect(screen.getByText(/quy tắc 7 ↔ quy tắc 7 · 3 mảnh/)).toBeTruthy();
  });

  it('keeps the notice-only rendering for pre-v0.4 incomplete reports', () => {
    window.localStorage.setItem('plenilune.locale', 'vi');
    const legacy: ChangeReport = { ...partialChanges, partial: undefined };
    render(
      <LocaleProvider>
        <ChangeClasses review={changesReview(legacy)} />
      </LocaleProvider>,
    );
    expect(document.querySelector('.change-class-list')).toBeNull();
    expect(document.querySelector('.partial-result')).toBeNull();
  });
});

describe('plain-text outputs', () => {
  it('ticket summary and report lines carry the partial block', () => {
    const t = (key: Parameters<typeof translate>[1], vars?: Record<string, string | number>) =>
      translate('en', key, vars);
    const lines = partialReportLines(t, swapPartial).join('\n');
    expect(lines).toContain('**Partial result**');
    expect(lines).toContain('First 256 rule pairs decided identical');
    expect(lines).toContain('Undecided remainder: Range not yet compared — before rules 256–511 · after rules 256–511');

    const comparison = structuredClone(comparisonSnapshot);
    comparison.comparison.report.verdict = 'INCONCLUSIVE';
    comparison.comparison.report.search_complete = false;
    comparison.comparison.report.reason = 'boxes';
    comparison.comparison.report.partial = swapPartial;
    const input: ReportInput = {
      locale: 'en',
      t,
      accepted,
      comparison,
      repair: repairSnapshot,
      comparisonReplay: null,
      repairReplay: null,
      lastAi: null,
      sourceKind: 'sample',
      generatedAt: new Date('2026-11-01T00:00:00Z'),
    };
    const summary = buildTicketSummary(input);
    expect(summary).toContain('**Partial result**');
    expect(summary).toContain('Search complete: No');
  });
});
