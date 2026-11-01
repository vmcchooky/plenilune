import { cleanup, render } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { LocaleProvider, translate } from '../src/i18n';
import en from '../src/i18n/messages.en';
import vi from '../src/i18n/messages.vi';
import { ObjectDetail } from '../src/components/PrefixListDetail';
import { obligationStatusList } from '../src/lib/labels';
import { buildTicketSummary } from '../src/lib/report';
import type { ReportInput } from '../src/lib/report';
import type { AnalysisObject } from '../src/lib/types';
import {
  accepted,
  comparisonSnapshot,
  repairSnapshot,
} from './fixtures';

afterEach(cleanup);

// P0-3: references panel, per-obligation listing (no ratio framing),
// deterministic ticket summary, modeled-scope equivalent label.

function aclObject(references: AnalysisObject['references']): AnalysisObject {
  return {
    domain: 'acl',
    name: 'EDGE',
    kind: 'named-extended',
    status: 'changed',
    references,
  };
}

function renderDetail(object: AnalysisObject, locale: 'en' | 'vi') {
  window.localStorage.setItem('plenilune.locale', locale);
  render(<LocaleProvider><ObjectDetail object={object} /></LocaleProvider>);
  return document.body.textContent ?? '';
}

describe('references panel (P0-3 item 1)', () => {
  it('renders artifact references with kind, name, side and line', () => {
    const text = renderDetail(aclObject({
      before: [{ line: 12, kind: 'access-group', name: 'EDGE' }],
      after: [
        { line: 14, kind: 'access-group', name: 'EDGE' },
        { line: 21, kind: 'access-class', name: 'EDGE' },
      ],
    }), 'en');
    expect(text).toContain('Used at');
    expect(text).toContain('Structural references only');
    expect(text).toContain('does not model direction (in/out)');
    expect(text).toContain('access-group');
    expect(text).toContain('access-class');
    expect(text).toContain('EDGE');
    expect(text).toContain('Before line 12');
    expect(text).toContain('After line 14');
    expect(text).toContain('After line 21');
  });

  it('renders an explicit empty state when the artifact has no references', () => {
    const text = renderDetail(aclObject({ before: [], after: [] }), 'vi');
    expect(text).toContain('Được dùng tại');
    expect(text).toContain('Không tìm thấy tham chiếu cấu trúc nào trong cả hai cấu hình.');
  });
});

describe('per-obligation listing replaces the ratio framing (P0-3 item 2)', () => {
  const obligations = [
    { id: 'OB-1', status: 'VIOLATED' as const },
    { id: 'OB-2', status: 'HOLDS' as const },
    { id: 'OB-3', status: 'HOLDS' as const },
  ];

  it('lists each obligation id with its status in both locales', () => {
    expect(obligationStatusList((k, v) => translate('en', k, v), obligations))
      .toBe('OB-1 violated · OB-2 holds · OB-3 holds');
    expect(obligationStatusList((k, v) => translate('vi', k, v), obligations))
      .toBe('OB-1 không đạt · OB-2 đạt · OB-3 đạt');
  });

  it('retires the passed/total message key in both locales', () => {
    expect('result.conditionsSummary' in en).toBe(false);
    expect('result.conditionsSummary' in vi).toBe(false);
    expect(Object.values(en).join('\n')).not.toContain('Conditions passed');
    expect(Object.values(vi).join('\n')).not.toContain('Điều kiện đạt:');
  });
});

const generatedAt = new Date('2026-10-30T10:00:00Z');

function ticketInput(overrides: Partial<ReportInput> = {}): ReportInput {
  return {
    locale: 'vi',
    t: (key, vars) => translate('vi', key, vars),
    accepted,
    comparison: comparisonSnapshot,
    repair: repairSnapshot,
    comparisonReplay: null,
    repairReplay: null,
    lastAi: null,
    sourceKind: 'sample',
    generatedAt,
    ...overrides,
  };
}

describe('deterministic ticket summary (P0-3 item 3)', () => {
  it('covers verdict, witnesses, obligations, repair, limits, hash and replay commands', () => {
    const ticket = buildTicketSummary(ticketInput());
    expect(ticket).toContain('# Tóm tắt ticket Plenilune');
    expect(ticket).toContain('DIFFERENT');
    expect(ticket).toContain('OB-1 không đạt · OB-2 đạt · OB-3 đạt');
    expect(ticket).toContain('192.0.2.10:0 → 198.51.100.10:22');
    expect(ticket).toContain('Đã kiểm chứng');
    expect(ticket).toContain(comparisonSnapshot.comparison.sha256);
    expect(ticket).toContain('plenilune replay --before before.acl --after after.acl --evidence comparison.json');
    expect(ticket).toContain('plenilune repair-replay --before before.acl --after after.acl --evidence repair.json');
  });

  it('is deterministic: identical output regardless of generation time', () => {
    const first = buildTicketSummary(ticketInput());
    const second = buildTicketSummary(ticketInput({ generatedAt: new Date('2030-01-01T00:00:00Z') }));
    expect(second).toBe(first);
  });

  it('contains no raw configuration text and no ratio framing', () => {
    const ticket = buildTicketSummary(ticketInput());
    // Wildcard masks and section headers appear only in the raw source.
    expect(ticket).not.toContain('0.0.0.255');
    expect(ticket).not.toContain('ip access-list extended');
    expect(ticket).not.toContain('permit tcp');
    expect(ticket).not.toContain('Điều kiện đạt:');
    const english = buildTicketSummary(ticketInput({
      locale: 'en',
      t: (key, vars) => translate('en', key, vars),
    }));
    expect(english).toContain('# Plenilune ticket summary');
    expect(english).toContain('OB-1 violated · OB-2 holds · OB-3 holds');
    expect(english).not.toContain('Conditions passed');
  });

  it('omits the repair section content and repair-replay command without a receipt', () => {
    const ticket = buildTicketSummary(ticketInput({ repair: null }));
    expect(ticket).toContain('Không có biên bản sửa.');
    expect(ticket).not.toContain('repair-replay');
  });
});
