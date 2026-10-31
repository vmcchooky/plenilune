import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { LocaleProvider } from '../src/i18n';
import { PrefixListDetail } from '../src/components/PrefixListDetail';
import type { AnalysisObject, PrefixClass, PrefixReport } from '../src/lib/types';

afterEach(cleanup);

const extent = (line: number) => ({ lines: [[line, line]] as [number, number][], sha256: 'a'.repeat(64), units: 1 });

function object(report: PrefixReport, status: AnalysisObject['status'] = 'inconclusive'): AnalysisObject {
  return {
    domain: 'prefix-list',
    name: 'CUSTOMER',
    kind: 'ipv4',
    status,
    before: extent(3),
    after: extent(4),
    references: { before: [], after: [] },
    prefix_list: { parser: 'cisco-prefix-list/v0.2', engine: 'prefix-list-exact/v0.2', report },
  };
}

function classRow(routes = 3): PrefixClass {
  return {
    effect: 'newly_permitted',
    before: { action: 'deny', seq: 10, line: 3, implicit: false },
    after: { action: 'permit', seq: 5, line: 4, implicit: false },
    witness: {
      route: '203.0.113.0/25',
      before: { action: 'deny', seq: 10, line: 3, implicit: false },
      after: { action: 'permit', seq: 5, line: 4, implicit: false },
    },
    routes,
    lengths: [25, 26],
    ranges: [{ length: 25, first: '203.0.113.0/25', last: '203.0.113.0/25', routes: 1 }],
    ranges_truncated: false,
    total_ranges: 2,
  };
}

function renderObject(o: AnalysisObject) {
  window.localStorage.setItem('plenilune.locale', 'en');
  render(<LocaleProvider><PrefixListDetail object={o} /></LocaleProvider>);
}

describe('prefix-list result presentation', () => {
  it('renders no equivalence claim for an inconclusive report with no classes', () => {
    renderObject(object({
      verdict: 'INCONCLUSIVE',
      complete: false,
      reason: 'operations',
      classes: [],
      classes_truncated: false,
      total_classes: 0,
      operations: 2_000_000,
    }));
    const text = document.body.textContent ?? '';
    expect(text).toContain('No conclusion');
    expect(text).toContain('the operation budget ran out');
    expect(text).not.toContain('exactly the same routes');
    expect(text).toContain('not routes present on a network');
  });

  it('renders the equivalence sentence only for a complete equivalent report', () => {
    renderObject(object({
      verdict: 'EQUIVALENT',
      complete: true,
      classes: [],
      classes_truncated: false,
      total_classes: 0,
      operations: 12,
    }, 'equivalent'));
    expect(screen.getByText(/exactly the same routes in the model/)).toBeTruthy();
  });

  it('states that a truncated class list is not the whole change', () => {
    renderObject(object({
      verdict: 'DIFFERENT',
      complete: true,
      classes: [classRow(3)],
      classes_truncated: true,
      total_classes: 2,
      operations: 40,
    }, 'changed'));
    const text = document.body.textContent ?? '';
    expect(text).toContain('Showing 1/2 groups');
    expect(text).toContain('not the whole change');
    expect(text).toContain('route prefixes');
  });

  it('keeps the operation count as data and shows completeness', () => {
    renderObject(object({
      verdict: 'INCONCLUSIVE',
      complete: false,
      reason: 'operations',
      classes: [],
      classes_truncated: false,
      total_classes: 0,
      operations: 123,
    }));
    expect(screen.getByText('Operations: 123')).toBeTruthy();
    expect(screen.getByText('Compared the full prefix space')).toBeTruthy();
    expect(screen.getByText('No')).toBeTruthy();
  });
});
